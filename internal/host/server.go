package host

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"quartz/assets"
	"quartz/internal/accounts"
	"quartz/internal/content"
	"quartz/internal/core/events"
	"quartz/internal/core/fsx"
	"quartz/internal/core/linkx"
	"quartz/internal/core/pngx"
	"quartz/internal/core/procx"
	"quartz/internal/core/slp"
	"quartz/internal/core/sysinfo"
	"quartz/internal/core/tasks"
	"quartz/internal/core/version"
	"quartz/internal/gamelog"
	"quartz/internal/icons"
	"quartz/internal/install"
	"quartz/internal/instance"
	"quartz/internal/java"
	"quartz/internal/loader"
	"quartz/internal/mojang"
	"quartz/internal/sandbox"
	"quartz/internal/sandbox/rules"
)

const (
	consoleLines = 20_000
	logBatch     = 100 * time.Millisecond
	// stopGrace is how long a server gets to save its world after "stop".
	stopGrace = 30 * time.Second
	// pingDelay lets a burst of joins settle before the count is asked for.
	pingDelay = 500 * time.Millisecond
)

// configDirs are folders of mod settings a server reads too; the instance's
// newer files are copied over the server's before each start.
var configDirs = []string{"config", "defaultconfigs", "kubejs", "scripts"}

// server is one hosted server, from the moment Start accepts it.
type server struct {
	id, runID, name string
	opts            Options
	dir             string // the server folder
	saves           string // where the instance's worlds are (--universe)
	world           string // the world's folder in saves (--world)
	uuid            string // the default account's player id, for its inventory
	player          string // the default account's name
	rules           rules.Settings
	cancel          context.CancelFunc
	jail            *sandbox.Jail

	mu      sync.Mutex
	st      State
	cmd     *procx.Child
	stdin   io.WriteCloser
	con     *gamelog.Console
	started time.Time

	stopping atomic.Bool
	killed   atomic.Bool
	ready    atomic.Bool
	done     chan struct{}
	ping     chan struct{}
}

func (srv *server) state() State {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.st
}

func (srv *server) console() *gamelog.Console {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.con
}

// write sends one console line.
func (srv *server) write(line string) error {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.stdin == nil {
		return ErrNotRunning
	}
	_, err := io.WriteString(srv.stdin, line+"\n")
	return err
}

// Start checks the options and starts hosting the instance; the rest
// (install, backup, start) runs in the background and reports on host:state.
func (s *Service) Start(id string, o Options) error {
	if err := Check(o); err != nil {
		return err
	}
	in, err := s.d.Instances.Get(id)
	if err != nil {
		return err
	}
	saves := s.d.Instances.Path(id, "saves")
	world := o.World
	if world == "" {
		world = fsx.FreeName(saves, "New world", true)
	} else if info, err := os.Stat(filepath.Join(saves, world)); err != nil || !info.IsDir() {
		return ErrNoWorld
	} else if s.d.WorldOpen(filepath.Join(saves, world)) {
		return ErrWorldOpen
	}
	if o.Sandbox && !in.Sandbox.On && !s.canSandbox() {
		return ErrNoSandbox
	}
	if o.Icon != "" && s.d.Icons != nil {
		if _, err := s.d.Icons.Path(o.Icon); err != nil {
			return ErrBadOption
		}
	}
	if err := s.free(); err != nil {
		return err
	}
	s.mu.Lock()
	if err := s.freeLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	if !portFree(bindHost(o), o.Port) {
		s.mu.Unlock()
		return fmt.Errorf("%w: %d", ErrPortInUse, o.Port)
	}
	ctx, cancel := context.WithCancel(context.Background())
	// The Manage picks are saved on their own (SaveMods), not by the dialog.
	o.Mods = s.options(in).Mods
	srv := &server{id: id, runID: time.Now().Format("20060102-150405.000"), name: in.Name, opts: o, dir: s.Dir(id), saves: saves, world: world,
		rules: hostRules(in.Sandbox, o.Sandbox), cancel: cancel, done: make(chan struct{}), ping: make(chan struct{}, 1)}
	srv.st = State{InstanceID: id, RunID: srv.runID, State: "starting", Stage: startStage(o.World, world), Port: o.Port, Max: o.MaxPlayers,
		Address: "localhost:" + strconv.Itoa(o.Port)}
	if o.LAN {
		srv.st.LAN = lanAddress()
		if srv.st.LAN != "" {
			srv.st.Address = srv.st.LAN + ":" + strconv.Itoa(o.Port)
		}
	}
	s.cur = srv
	s.mu.Unlock()

	if _, err := s.d.Instances.Update(id, func(in *instance.Instance) error { kept := o; in.Host = &kept; return nil }); err != nil {
		s.d.Log.Warn("host: remembering the options failed", "err", err)
	}
	s.emit(srv)
	go s.run(ctx, srv, in)
	return nil
}

// free fails when a server already runs (one at a time).
func (s *Service) free() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.freeLocked()
}

func (s *Service) freeLocked() error {
	if s.cur != nil {
		return fmt.Errorf("%w: %s is hosting, stop it first", ErrRunning, s.cur.name)
	}
	return nil
}

// hostRules are the sandbox rules a server runs under: the instance's when
// it is sandboxed, else the Standard rules when the Host dialog asks.
func hostRules(own rules.Settings, sandbox bool) rules.Settings {
	if own.On || !sandbox {
		return own
	}
	return rules.Settings{On: true, Preset: rules.Standard}
}

// bindHost is the address the server listens on.
func bindHost(o Options) string {
	if o.LAN {
		return ""
	}
	return "127.0.0.1"
}

// startStage is what the starting panel says while the world is prepared.
func startStage(chosen, world string) string {
	if chosen == "" {
		return "Preparing spawn area"
	}
	return "Backing up " + world + ", preparing spawn area"
}

func (s *Service) emit(srv *server) { s.d.Bus.Emit(events.HostState, srv.state()) }

func (s *Service) setStage(srv *server, stage string) {
	srv.mu.Lock()
	srv.st.Stage = stage
	srv.mu.Unlock()
	s.emit(srv)
}

// run prepares the server and starts it.
func (s *Service) run(ctx context.Context, srv *server, in instance.Instance) {
	javaPath, argv, err := s.prepare(ctx, srv, in)
	if err == nil {
		err = s.spawn(ctx, srv, javaPath, argv)
	}
	if err != nil {
		result := "failed"
		if ctx.Err() != nil {
			result, err = "cancelled", nil
		}
		s.finish(srv, result, err)
	}
}

// prepare installs what the server needs and lays out its folder; it
// returns the Java to run and its arguments.
func (s *Service) prepare(ctx context.Context, srv *server, in instance.Instance) (string, []string, error) {
	stage := srv.state().Stage
	t := install.Target{Minecraft: in.Minecraft, LoaderVersion: in.LoaderVersion}
	if in.Loader != "" {
		k, err := loader.ParseKind(in.Loader)
		if err != nil {
			return "", nil, err
		}
		t.Loader = k
	}
	res, err := s.d.Installer.ResolveServer(ctx, t, mojang.CurrentEnv(sysinfo.OSVersion()))
	if err != nil {
		return "", nil, err
	}
	eff := in.Resolve(s.d.Settings.Get())
	if eff.JavaUnset {
		return "", nil, instance.ErrJavaNotChosen
	}
	own := java.Request{Major: res.Version.JavaMajor(), Component: res.Version.JavaComponent()}
	req := own
	req.Override = eff.JavaPath
	rt, plan, err := s.d.Java.Resolve(ctx, req)
	if err != nil {
		return "", nil, err
	}
	if plan == nil && req.Override != "" && res.Processing != nil && !res.Processing.Done() {
		if _, ownPlan, err := s.d.Java.Resolve(ctx, own); err == nil {
			plan = ownPlan
		}
	}
	spec, err := s.d.Installer.PlanServer(install.ServerRequest{Server: res, Java: plan, ProcessorJava: s.d.Java.Installed(own),
		Meta: tasks.Meta{Title: in.Name, Subtitle: "Server for " + gameLabel(in), Art: in.TaskArt(), Kind: "install"}})
	if err != nil {
		return "", nil, err
	}
	if spec != nil {
		s.setStage(srv, "Installing the server")
		if err := s.runJob(ctx, spec); err != nil {
			return "", nil, err
		}
		s.setStage(srv, stage)
	}
	if plan != nil {
		if rt, _, err = s.d.Java.Resolve(ctx, req); err != nil {
			return "", nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	world := filepath.Join(srv.saves, srv.world)
	if srv.opts.World != "" {
		// Installing can take a while, and the game may open the world meanwhile.
		if s.d.WorldOpen(world) {
			return "", nil, ErrWorldOpen
		}
		if err := backupWorld(world, filepath.Join(s.d.Instances.Dir(in.ID), "backups", "host", srv.world), time.Now()); err != nil {
			return "", nil, fmt.Errorf("host: backing up %s: %w", srv.world, err)
		}
		srv.uuid, srv.player = s.player(srv.opts)
		if srv.uuid != "" {
			if err := carryIn(world, srv.uuid); err != nil {
				s.d.Log.Warn("host: carrying the singleplayer inventory over failed", "err", err)
			}
		}
	}
	if err := s.layout(ctx, srv, in, res); err != nil {
		return "", nil, err
	}
	return rt.Path, serverArgs(res, srv), nil
}

// gameLabel is "Minecraft 1.21.4" or "Fabric 0.16.10 on Minecraft 1.21.4".
func gameLabel(in instance.Instance) string {
	if in.Loader == "" {
		return "Minecraft " + in.Minecraft
	}
	return loader.Label(loader.Kind(in.Loader), in.Minecraft, in.LoaderVersion) + " on Minecraft " + in.Minecraft
}

// runJob runs an install job to the end (cancelled with ctx).
func (s *Service) runJob(ctx context.Context, spec *tasks.Spec) error {
	id, err := s.d.Tasks.Submit(spec)
	if err != nil || id == "" {
		return err
	}
	if err := s.d.Tasks.Wait(ctx, id); err != nil {
		if ctx.Err() != nil {
			_ = s.d.Tasks.Cancel(id)
		}
		return err
	}
	return nil
}

// player is the default account's id as the server sees it (its offline
// id when offline accounts are allowed, else its Minecraft id) and name.
func (s *Service) player(o Options) (string, string) {
	if s.d.Accounts == nil {
		return "", ""
	}
	acc, err := s.d.Accounts.Default()
	if err != nil {
		return "", ""
	}
	if o.Offline || acc.Kind == accounts.Offline {
		return accounts.OfflineUUID(acc.Name), acc.Name
	}
	return dashed(acc.ID), acc.Name
}

// layout fills the server folder: the mods that run on servers, newer mod
// settings from the instance, the files the start expects, and the server's
// settings and EULA.
func (s *Service) layout(ctx context.Context, srv *server, in instance.Instance, res *install.Server) error {
	if err := os.MkdirAll(filepath.Join(srv.dir, ".quartz"), 0o700); err != nil {
		return err
	}
	var keep []content.ServerMod
	if in.Loader != "" {
		mods, err := s.d.Content.ServerMods(ctx, in.ID)
		if err != nil {
			return err
		}
		for _, m := range mods {
			if runs(m, srv.opts.Mods) {
				keep = append(keep, m)
			}
		}
	}
	if err := s.placeMods(filepath.Join(srv.dir, "mods"), keep); err != nil {
		return err
	}
	for _, name := range configDirs {
		src := s.d.Instances.Path(in.ID, name)
		if info, err := os.Lstat(src); err != nil || !info.IsDir() {
			continue
		}
		if err := linkx.UpdateTree(src, filepath.Join(srv.dir, name)); err != nil {
			return fmt.Errorf("host: copying %s: %w", name, err)
		}
	}
	if err := res.PlaceInto(srv.dir); err != nil {
		return err
	}
	rules, cheats := worldRules(filepath.Join(srv.saves, srv.world), in.Minecraft)
	if err := writeServerFiles(srv.dir, srv.opts, srv.world, in.Name, in.Minecraft, rules); err != nil {
		return err
	}
	if err := s.writeIcon(srv.dir, srv.opts.Icon); err != nil {
		return err
	}
	if cheats && srv.uuid != "" {
		if err := addOp(srv.dir, in.Minecraft, srv.uuid, srv.player); err != nil {
			s.d.Log.Warn("host: making the host an operator failed", "err", err)
		}
	}
	return nil
}

// placeMods makes the server's mods folder exactly the kept mods: it is the
// launcher's, so whatever else is in it goes.
func (s *Service) placeMods(dir string, keep []content.ServerMod) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	want := map[string]string{}
	for _, m := range keep {
		want[m.File] = m.SHA1
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if _, ok := want[e.Name()]; ok && e.Type().IsRegular() {
			continue
		}
		if e.Type().IsRegular() || fsx.IsLink(e.Type()) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	for file, sha1 := range want {
		dst, err := fsx.SafeJoin(dir, file)
		if err != nil {
			return err
		}
		if _, err := s.d.Store.Place(sha1, dst); err != nil {
			return fmt.Errorf("host: placing %s: %w", file, err)
		}
	}
	return nil
}

// writeServerFiles writes server.properties (keeping the player's own
// edits, except where the Host dialog or the world's rules decide) and
// accepts the EULA, as Start server does for the player.
func writeServerFiles(dir string, o Options, world, name, mc string, rules map[string]string) error {
	set, keep := dialogProps(o, world, name, mc, rules)
	if err := writeProps(filepath.Join(dir, "server.properties"), set, keep); err != nil {
		return err
	}
	return fsx.AtomicWrite(filepath.Join(dir, "eula.txt"), []byte("# Accepted by Quartz when you chose Start server.\neula=true\n"), fsx.FilePerm)
}

// dialogProps are the server.properties values a start writes: set ones
// always (the Host dialog's, then the world's rules, which the dialog's
// difficulty overrides unless the world is hardcore), keep ones only when
// the file has none yet.
func dialogProps(o Options, world, name, mc string, rules map[string]string) (set, keep map[string]string) {
	set = map[string]string{
		"server-port": strconv.Itoa(o.Port), "max-players": strconv.Itoa(o.MaxPlayers), "server-ip": bindHost(o),
		"online-mode": strconv.FormatBool(!o.Offline), "enable-rcon": "false", "enable-query": "false", "level-name": world,
		"view-distance": strconv.Itoa(o.ViewDistance), "motd": motd(cmp.Or(strings.TrimSpace(o.MOTD), name)),
	}
	if o.Offline {
		set["enforce-secure-profile"] = "false"
	}
	if o.World == "" {
		set["level-seed"] = o.Seed
	}
	maps.Copy(set, rules)
	if o.Difficulty != "" && set["hardcore"] != "true" {
		set["difficulty"] = o.Difficulty
		if version.Compare(mc, namedRules) < 0 {
			set["difficulty"] = strconv.Itoa(slices.Index(difficulties, o.Difficulty))
		}
	}
	// As on LAN, an operator host doesn't stop friends building at spawn.
	return set, map[string]string{"spawn-protection": "0"}
}

// writeIcon writes the server's server-icon.png: the picked picture at
// 64 px, else Quartz's.
func (s *Service) writeIcon(dir, icon string) error {
	data := assets.ServerIcon
	if icon != "" && s.d.Icons != nil {
		p, err := s.d.Icons.Path(icon)
		if err != nil {
			return ErrBadOption
		}
		raw, err := fsx.ReadCapped(p, icons.MaxFile)
		if err != nil {
			return err
		}
		img, err := pngx.Decode(raw, pngx.Limits{MaxBytes: icons.MaxFile, MaxSide: icons.Side})
		if err != nil {
			return err
		}
		if data, _, err = pngx.Encode(pngx.Square(img, serverIconSide)); err != nil {
			return err
		}
	}
	return fsx.AtomicWrite(filepath.Join(dir, "server-icon.png"), data, fsx.FilePerm)
}

// serverIconSide is the size Minecraft wants a server icon.
const serverIconSide = 64

// serverArgs is the java command line after the java binary: Aikar's
// flags for the chosen memory (the instance's own JVM arguments are for
// the game and would clash with them), then how this server starts.
func serverArgs(res *install.Server, srv *server) []string {
	argv := aikarFlags(srv.opts.MemoryMB)
	argv = append(argv, java.Properties...)
	if res.LogConfig != "" && res.LogArgument != "" {
		argv = append(argv, strings.ReplaceAll(res.LogArgument, "${path}", res.LogConfig))
	}
	argv = append(argv, res.Argv...)
	return append(argv, "nogui", "--universe", srv.saves, "--world", srv.world)
}

// spawn starts the server process and follows it until it exits.
func (s *Service) spawn(ctx context.Context, srv *server, javaPath string, argv []string) error {
	spec := procx.Spec{Argv: append([]string{javaPath}, argv...), Dir: srv.dir, Env: java.CleanEnv(os.Environ()), NoConsole: true}
	if srv.rules.On && s.d.Sandbox != nil && runtime.GOOS != "windows" {
		spec.Env = java.CleanEnv(sandbox.FilterEnv(os.Environ()))
		r := sandbox.Run{Instance: srv.id, Rules: srv.rules, GameDir: srv.dir, Headless: true,
			Scratch: filepath.Join(srv.dir, ".quartz", "sandbox"),
			Read:    append(slices.Clone(s.d.StoreRead), sandbox.JavaRoot(javaPath)), Write: []string{srv.saves}}
		if s.d.Instances != nil {
			r.Data = filepath.Join(s.d.Instances.Dir(srv.id), "sandbox")
		}
		var err error
		if spec, srv.jail, err = s.d.Sandbox.Wrap(r, spec); err != nil {
			return err
		}
	}
	con := gamelog.NewConsole(consoleLines, s.d.Redactor, logBatch, func(l gamelog.Line) { s.onLine(srv, l) }, func(lines []gamelog.Line) {
		s.d.Bus.Emit(events.HostLog, LogBatch{InstanceID: srv.id, RunID: srv.runID, Lines: lines})
	})
	srv.mu.Lock()
	if err := ctx.Err(); err != nil {
		srv.mu.Unlock()
		srv.jail.Close()
		return err
	}
	cmd, err := procx.Start(spec)
	if err != nil {
		srv.mu.Unlock()
		srv.jail.Close()
		return err
	}
	srv.cmd, srv.stdin, srv.con, srv.started = cmd, cmd.Stdin, con, time.Now()
	srv.mu.Unlock()
	srv.jail.Started(cmd.Process.Pid)
	if err := fsx.AtomicWrite(pidFile(srv.dir), []byte(pidLine(cmd.Process.Pid)), fsx.FilePerm); err != nil {
		s.d.Log.Warn("host: recording the server process failed", "err", err)
	}
	pings, stopPings := context.WithCancel(context.Background())
	go s.pinger(pings, srv)
	wait := con.Start(cmd.Stdout, cmd.Stderr)
	go func() {
		wait()
		err := cmd.Wait()
		stopPings()
		s.exited(srv, err)
	}()
	return nil
}

// onLine watches the console: "Done" means players can join; joins and
// leaves ask for a new player count.
func (s *Service) onLine(srv *server, l gamelog.Line) {
	switch {
	case !srv.ready.Load() && doneRe.MatchString(l.Message):
		srv.ready.Store(true)
		srv.mu.Lock()
		srv.st.State, srv.st.Stage = "running", ""
		srv.mu.Unlock()
		s.emit(srv)
		s.askPlayers(srv)
	case srv.ready.Load() && playersRe.MatchString(l.Message):
		s.askPlayers(srv)
	}
}

func (s *Service) askPlayers(srv *server) {
	select {
	case srv.ping <- struct{}{}:
	default:
	}
}

// pinger asks the server itself how many players it has whenever the
// console says the count changed; while it is shown on the network it also
// announces the server there.
func (s *Service) pinger(ctx context.Context, srv *server) {
	announcing := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-srv.ping:
		}
		if srv.opts.LAN && !announcing {
			announcing = true
			go announce(ctx, srv.name, srv.opts.Port)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pingDelay):
		}
		st, err := slp.Ping(ctx, slp.Address{Host: "127.0.0.1", Port: srv.opts.Port}, -1)
		if err != nil {
			continue
		}
		srv.mu.Lock()
		// Old servers report no maximum until their first status refresh.
		srv.st.Players, srv.st.Max = st.Online, cmp.Or(st.Max, srv.opts.MaxPlayers)
		srv.mu.Unlock()
		s.emit(srv)
	}
}

// Stop asks the server to save and quit ("stop" on its console); after 30
// seconds, or on a second Stop, it is killed. A server still being prepared
// is just cancelled.
func (s *Service) Stop(id string) error {
	srv, err := s.current(id)
	if err != nil {
		return err
	}
	srv.mu.Lock()
	cmd := srv.cmd
	srv.mu.Unlock()
	if cmd == nil {
		srv.cancel()
		return nil
	}
	if srv.stopping.Swap(true) {
		srv.killed.Store(true)
		return procx.Kill(cmd.Process)
	}
	if err := srv.write("stop"); err != nil {
		srv.killed.Store(true)
		return procx.Kill(cmd.Process)
	}
	go func() {
		select {
		case <-srv.done:
		case <-time.After(stopGrace):
			srv.killed.Store(true)
			_ = procx.Kill(cmd.Process)
		}
	}()
	return nil
}

// Close stops a running server for the launcher quitting: gracefully, then
// forced when ctx ends.
func (s *Service) Close(ctx context.Context) {
	s.mu.Lock()
	srv := s.cur
	s.mu.Unlock()
	if srv == nil {
		return
	}
	_ = s.Stop(srv.id)
	select {
	case <-srv.done:
	case <-ctx.Done():
		srv.mu.Lock()
		cmd := srv.cmd
		srv.mu.Unlock()
		if cmd != nil {
			srv.killed.Store(true)
			_ = procx.Kill(cmd.Process)
		}
		select {
		case <-srv.done:
		case <-time.After(5 * time.Second):
		}
	}
}

// exited sorts out how the server ended.
func (s *Service) exited(srv *server, waitErr error) {
	srv.jail.Close()
	code, reaped := srv.cmd.Result()
	failed := (reaped && code != 0) || (waitErr != nil && !reaped)
	result := "ok"
	var err error
	switch {
	case srv.stopping.Load():
	case !srv.ready.Load():
		result, err = "failed", errors.New(lastError(srv.con.Lines()))
	case failed:
		result = "crash"
	}
	if srv.uuid != "" {
		if err := carryOut(filepath.Join(srv.saves, srv.world), srv.uuid, srv.started); err != nil {
			s.d.Log.Warn("host: writing the inventory back to the singleplayer world failed", "err", err)
		}
	}
	_ = os.Remove(pidFile(srv.dir))
	s.finish(srv, result, err)
}

// finish ends a server's life: the console stays readable, the slot frees
// up, and the UI hears the result.
func (s *Service) finish(srv *server, result string, err error) {
	srv.mu.Lock()
	srv.st.State, srv.st.Stage, srv.st.Result, srv.st.Players = "stopped", "", result, 0
	if err != nil {
		srv.st.Message = err.Error()
	}
	srv.stdin = nil
	con := srv.con
	srv.mu.Unlock()
	srv.cancel()
	s.mu.Lock()
	if s.cur == srv {
		s.cur = nil
	}
	if con != nil {
		s.last[srv.id] = con.Buffer()
	}
	s.mu.Unlock()
	s.emit(srv)
	s.d.Bus.Emit(events.WorldsChanged, map[string]string{"instanceId": srv.id})
	close(srv.done)
}

// lastError is the newest error line of a console, for a server that never
// got going.
func lastError(lines []gamelog.Line) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i].Level == gamelog.Error && lines[i].Message != "" {
			return lines[i].Message
		}
	}
	return "The server stopped before it was ready."
}
