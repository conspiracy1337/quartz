package launch

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"quartz/internal/accounts"
	"quartz/internal/brand"
	"quartz/internal/core/events"
	"quartz/internal/core/fsx"
	"quartz/internal/core/procx"
	"quartz/internal/core/shlex"
	"quartz/internal/core/slp"
	"quartz/internal/gamelog"
	"quartz/internal/install"
	"quartz/internal/instance"
	"quartz/internal/java"
	"quartz/internal/mojang"
	"quartz/internal/runview"
	"quartz/internal/sandbox"
	"quartz/internal/syncing"
	"quartz/internal/wrapperjar"
)

const (
	consoleLines = 20_000
	stopGrace    = 3 * time.Second
	logBatch     = 100 * time.Millisecond
)

// run is one running game.
type run struct {
	id         string
	instanceID string
	child      *procx.Child
	started    time.Time
	con        *gamelog.Console
	logFile    *os.File
	accountID  string

	stopping atomic.Bool
	killed   atomic.Bool
	// loaded is set once the game got far enough to log its user: a
	// non-zero exit before that is "Failed to start", after it a crash.
	loaded atomic.Bool

	done chan struct{}

	pmu    sync.Mutex
	server string // the last "Connecting to host, port" (guarded by pmu)

	record Record
	// postExit is the player's after-exit command, with the variables it gets.
	postExit string
	vars     map[string]string
	env      []string
	// sp holds the run's view and shared-files session until it exits.
	sp spec
}

// spec is everything one game start needs.
type spec struct {
	inst    instance.Instance
	eff     instance.Effective
	acc     accounts.Account
	sess    accounts.Session
	res     *install.Resolved
	rt      java.Runtime
	mem     MemoryRange
	env     mojang.Env
	gameDir string
	vars    map[string]string // $INST_* values
	mods    int
	into    instance.Into // a world or server to open straight into
	runID   string
	view    *runview.View    // nil runs in the game folder itself
	session *syncing.Session // nil shares nothing
	// sandboxed runs the game in the instance's sandbox; jail is it once built.
	sandboxed bool
	jail      *sandbox.Jail
	// cmd, script and shown are the prepared start (see command).
	cmd    procx.Spec
	script []byte
	shown  string
}

// moduleExports are the named-module main classes the wrapper jar calls
// reflectively (Forge's BootstrapLauncher sits on the module path and does
// not export its package to unnamed code; Pandora hit the same).
var moduleExports = map[string]string{ //nolint:gosec // module names, not credentials
	"cpw.mods.bootstraplauncher.BootstrapLauncher": "cpw.mods.bootstraplauncher/cpw.mods.bootstraplauncher=ALL-UNNAMED",
}

// command builds the java command line and the stdin script. Nothing secret
// goes on the command line: the account's token is a game argument and game
// arguments only travel through the script. It also returns both as text for
// the run's cmd.txt (the caller redacts it). A sandboxed run is wrapped in
// its jail here (sp.jail), so the sandbox's own failures happen before
// anything starts.
func (l *Launcher) command(sp *spec) (procx.Spec, []byte, string, error) {
	jar, err := wrapperjar.Install(l.cfg.BinDir)
	if err != nil {
		return procx.Spec{}, nil, "", err
	}
	sess, res, eff := sp.sess, sp.res, sp.eff
	v := res.Version
	natives, err := l.natives(sp)
	if err != nil {
		return procx.Spec{}, nil, "", err
	}
	vars := mojang.Vars{
		"auth_player_name": sess.Name, "auth_uuid": sess.UUID, "auth_access_token": sess.AccessToken,
		"auth_session": sess.LegacySession(), "auth_xuid": sess.XUID, "clientid": sess.ClientID, "user_type": sess.UserType,
		"user_properties": "{}", "version_name": v.ID, "version_type": v.Type,
		"game_directory": sp.gameDir, "assets_root": res.AssetsRoot, "assets_index_name": v.AssetsID(),
		"game_assets": res.GameAssets, "natives_directory": natives, "library_directory": res.LibrariesDir,
		"classpath":           strings.Join(append(append([]string(nil), res.Classpath...), jar), string(os.PathListSeparator)),
		"classpath_separator": string(os.PathListSeparator),
		"launcher_name":       brand.Slug, "launcher_version": brand.Version,
		"resolution_width": strconv.Itoa(eff.Width), "resolution_height": strconv.Itoa(eff.Height),
		"quickPlayPath": filepath.Join(sp.gameDir, "quickPlay", "log.json"),
	}
	quickPlay(sp.into, sp.env, vars)
	unknown := func(name string) {
		l.cfg.Log.Warn("unknown placeholder in version metadata", "version", v.ID, "name", name)
	}

	args := v.JVMArgs(sp.env, vars, unknown)
	args = append(args, fmt.Sprintf("-Xms%dM", sp.mem.Min), fmt.Sprintf("-Xmx%dM", sp.mem.Max))
	user, err := shlex.Split(eff.JVMArgs)
	if err != nil {
		return procx.Spec{}, nil, "", fmt.Errorf("JVM arguments: %w", err)
	}
	args = append(args, user...)
	args = append(args, nativeLibraryArgs(eff)...)
	// Defence in depth against Log4Shell on old versions, whatever the config says.
	args = append(args, "-Dlog4j2.formatMsgNoLookups=true")
	if res.LogConfig != "" && res.LogArgument != "" {
		args = append(args, mojang.Vars{"path": res.LogConfig}.Expand(res.LogArgument, unknown))
	}
	if exp, ok := moduleExports[v.MainClass]; ok {
		args = append(args, "--add-exports", exp)
	}
	args = append(args, wrapperjar.MainClass)

	game := v.GameArgs(sp.env, vars, unknown)
	game = append(game, legacyServer(sp.into, game)...)
	if eff.Fullscreen {
		game = append(game, "--fullscreen")
	}
	script := new(wrapperjar.Script)
	for _, a := range game {
		script.Arg(a)
	}
	script.Launch(v.MainClass)

	argv, err := wrappers(eff, sp.vars, sp.sandboxed)
	if err != nil {
		return procx.Spec{}, nil, "", err
	}
	argv = append(argv, java.LaunchBinary(sp.rt.Path))
	argv = append(argv, args...)
	cmd := procx.Spec{Argv: argv, Dir: sp.gameDir, Env: gameEnv(sp.vars, eff, sp.sandboxed)}
	switch {
	case sp.sandboxed:
		if cmd, sp.jail, err = l.cfg.Sandbox.Wrap(l.sandboxRun(sp, argv), cmd); err != nil {
			return procx.Spec{}, nil, "", err
		}
	case sp.view != nil:
		cmd.Argv = sp.view.Wrap(argv)
	}
	shown := strings.Join(cmd.Argv, " ") + "\n\nGame arguments (sent to the game over stdin, never on the command line):\n" + strings.Join(game, " ") + "\n"
	return cmd, script.Bytes(), shown, nil
}

// quickPlay turns on the version's Quick Play arguments (1.20 and later).
func quickPlay(into instance.Into, env mojang.Env, vars mojang.Vars) {
	switch into.Kind {
	case "world":
		env.Features["is_quick_play_singleplayer"] = true
		vars["quickPlaySingleplayer"] = into.Target
	case "server":
		env.Features["is_quick_play_multiplayer"] = true
		vars["quickPlayMultiplayer"] = into.Target
	}
}

// legacyServer joins a server on versions without Quick Play through the
// older --server and --port arguments (Prism does the same). A world can't
// be opened that way; those versions start at the title screen.
func legacyServer(into instance.Into, game []string) []string {
	if into.Kind != "server" || slices.Contains(game, "--quickPlayMultiplayer") {
		return nil
	}
	a, err := slp.Parse(into.Target)
	if err != nil {
		return nil
	}
	return []string{"--server", a.Host, "--port", strconv.Itoa(cmp.Or(a.Port, slp.DefaultPort))}
}

// start spawns the game and wires its output, events and exit handling.
func (l *Launcher) start(ctx context.Context, sp spec) (*run, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	inst, acc, sess, rt, mem := sp.inst, sp.acc, sp.sess, sp.rt, sp.mem
	cmd, script, shown := sp.cmd, sp.script, sp.shown
	now := time.Now()
	r := &run{
		id: sp.runID, instanceID: inst.ID, started: now, accountID: acc.ID, done: make(chan struct{}),
		record:   Record{StartedAt: now.UnixMilli(), Java: rt.Name(), Memory: mem, Account: acc.Name, AccountID: acc.ID, Mods: sp.mods},
		server:   serverOf(sp.into),
		postExit: sp.eff.Commands.PostExit, vars: sp.vars, env: gameEnv(sp.vars, sp.eff, false), sp: sp,
	}
	r.record.ID = r.id
	if sp.jail != nil {
		r.record.Sandbox = sp.jail.Name
	}
	r.con = gamelog.NewConsole(consoleLines, l.cfg.Redactor, logBatch, r.add, func(lines []gamelog.Line) {
		l.cfg.Bus.Emit(events.GameLog, LogBatch{InstanceID: r.instanceID, RunID: r.id, Lines: lines})
	})
	runsDir := l.cfg.Instances.RunsDir(inst.ID)
	if err := os.MkdirAll(runsDir, 0o700); err != nil {
		return nil, err
	}
	var err error
	if r.logFile, err = os.OpenFile(filepath.Join(runsDir, r.id+".log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err != nil { //nolint:gosec // our own runs folder
		return nil, err
	}
	// The token stays a secret for the launcher's lifetime: another run may
	// share it, and the account manager holds it too.
	l.cfg.Redactor.AddSecret(sess.AccessToken)
	if err := fsx.AtomicWrite(filepath.Join(runsDir, r.id+evidenceExt[EvidenceCommand]), []byte(l.cfg.Redactor.Redact(shown)), fsx.FilePerm); err != nil {
		l.cfg.Log.Warn("saving the launch command failed", "err", err)
	}
	child, err := procx.Start(cmd)
	if err != nil {
		_ = r.logFile.Close()
		return nil, err
	}
	r.child = child
	if sp.view != nil {
		sp.view.Started(child.Process.Pid)
	}
	sp.jail.Started(child.Process.Pid)
	_, werr := child.Stdin.Write(script)
	_ = child.Stdin.Close()
	if werr != nil {
		l.cfg.Log.Warn("writing launch arguments failed", "err", werr)
	}

	l.mu.Lock()
	l.running[inst.ID] = append(l.running[inst.ID], r)
	copies := len(l.running[inst.ID])
	l.mu.Unlock()
	l.cfg.Bus.Emit(events.GameState, State{InstanceID: inst.ID, RunID: r.id, State: "running", Copies: copies})
	if l.cfg.Hooks.GameStarted != nil {
		l.cfg.Hooks.GameStarted(sp.eff.OnGameStart)
	}

	wait := r.con.Start(child.Stdout, child.Stderr)
	go func() {
		wait()
		l.exited(r, child.Wait())
	}()
	return r, nil
}

var connectingRe = regexp.MustCompile(`^Connecting to (\S+), (\d{1,5})$`)

// add sees every console line as it is parsed.
func (r *run) add(line gamelog.Line) {
	if !r.loaded.Load() && (strings.HasPrefix(line.Message, "Setting user: ") || strings.HasPrefix(line.Message, "Backend library: LWJGL") || strings.HasPrefix(line.Message, "LWJGL Version")) {
		r.loaded.Store(true)
	}
	if m := connectingRe.FindStringSubmatch(line.Message); m != nil {
		host := m[1]
		if m[2] != strconv.Itoa(slp.DefaultPort) {
			host += ":" + m[2]
		}
		r.pmu.Lock()
		r.server = host
		r.pmu.Unlock()
	}
	thread := ""
	if line.Thread != "" {
		thread = line.Thread + "/"
	}
	_, _ = fmt.Fprintf(r.logFile, "[%s] [%s%s]: %s\n", line.Time, thread, line.Level, line.Message)
}

// tailLines is how much of the console finishers see.
const tailLines = 400

func tail(lines []gamelog.Line, n int) []gamelog.Line {
	if len(lines) > n {
		return lines[len(lines)-n:]
	}
	return lines
}

// serverOf is the server a launch opens into, for its record.
func serverOf(into instance.Into) string {
	if into.Kind != "server" {
		return ""
	}
	if a, err := slp.Parse(into.Target); err == nil {
		return a.String()
	}
	return ""
}

// LogBatch is what game:log carries.
type LogBatch struct {
	InstanceID string         `json:"instanceId"`
	RunID      string         `json:"runId"`
	Lines      []gamelog.Line `json:"lines"`
}

// Stop asks every copy of the game to quit; after 3 seconds, or on a second
// Stop, they are killed.
func (l *Launcher) Stop(id string) error {
	l.mu.Lock()
	copies := slices.Clone(l.running[id])
	l.mu.Unlock()
	if len(copies) == 0 {
		return ErrNotRunning
	}
	var errs []error
	for _, r := range copies {
		errs = append(errs, l.stop(r))
	}
	return errors.Join(errs...)
}

func (l *Launcher) stop(r *run) error {
	if r.stopping.Swap(true) {
		r.killed.Store(true)
		return procx.Kill(r.child.Process)
	}
	if err := procx.Terminate(r.child.Process); err != nil {
		r.killed.Store(true)
		return procx.Kill(r.child.Process)
	}
	go func() {
		select {
		case <-r.done:
		case <-time.After(stopGrace):
			r.killed.Store(true)
			_ = procx.Kill(r.child.Process)
		}
	}()
	return nil
}

// exited records the run, updates playtime and tells the UI.
func (l *Launcher) exited(r *run, waitErr error) {
	close(r.done)
	r.con.Done()
	_ = r.logFile.Close()

	code, reaped := r.child.Result()
	if !reaped {
		code = 0
	}
	failed := code != 0 || (waitErr != nil && !reaped)
	result := ResultOK
	switch {
	case r.killed.Load():
		result = ResultKilled
	case r.stopping.Load():
	case failed && !r.loaded.Load():
		result = ResultFailed
	case failed:
		result = ResultCrash
	}
	ended := time.Now()
	r.record.LengthMin = int(math.Round(ended.Sub(r.started).Minutes()))
	r.record.ExitCode, r.record.Result = code, result
	r.pmu.Lock()
	r.record.Server = r.server
	r.pmu.Unlock()
	// The run's changes go back into the instance (and shared folders)
	// before anything reads them.
	kept := l.disassemble(&r.sp)
	gameDir := l.cfg.Instances.GameDir(r.instanceID)
	runsDir := l.cfg.Instances.RunsDir(r.instanceID)
	f := &Finished{InstanceID: r.instanceID, GameDir: gameDir, RunsDir: runsDir, Started: r.started, Ended: ended, Record: &r.record, Tail: tail(r.con.Lines(), tailLines)}
	for _, fin := range l.cfg.Finishers {
		fin.FinishRun(f)
	}
	if err := saveRecord(runsDir, r.record); err != nil {
		l.cfg.Log.Error("saving run record failed", "err", err)
	}
	started := r.started.UTC()
	if _, err := l.cfg.Instances.Update(r.instanceID, func(in *instance.Instance) error {
		in.LastPlayed = &started
		in.AddPlaytime(ended.Sub(r.started))
		return nil
	}); err != nil {
		l.cfg.Log.Error("updating playtime failed", "err", err)
	}

	l.mu.Lock()
	rest := slices.DeleteFunc(l.running[r.instanceID], func(o *run) bool { return o == r })
	if len(rest) == 0 {
		delete(l.running, r.instanceID)
	} else {
		l.running[r.instanceID] = rest
	}
	l.last[r.instanceID] = r.con.Buffer()
	none := len(l.running) == 0
	l.mu.Unlock()
	l.cfg.Bus.Emit(events.InstancesChanged, nil)
	l.cfg.Bus.Emit(events.GameState, State{InstanceID: r.instanceID, RunID: r.id, State: "exited", ExitCode: code, Result: result, Copies: len(rest), Kept: kept})
	if none && l.cfg.Hooks.AllExited != nil {
		l.cfg.Hooks.AllExited()
	}
	if r.postExit != "" {
		vars := maps.Clone(r.vars)
		vars["EXIT_CODE"] = strconv.Itoa(code)
		if err := runCommand(context.Background(), r.postExit, gameDir, vars, append(r.env, "EXIT_CODE="+vars["EXIT_CODE"])); err != nil {
			l.cfg.Log.Warn("after-exit command failed", "instance", r.instanceID, "err", err)
			if l.cfg.Hooks.AfterExitFailed != nil {
				name := r.instanceID
				if in, err := l.cfg.Instances.Get(r.instanceID); err == nil {
					name = in.Name
				}
				l.cfg.Hooks.AfterExitFailed(name, err)
			}
		}
	}
}
