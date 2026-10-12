// Package launch starts the game: an ordered pipeline of small steps
// (Prism's LaunchSteps) that maps one-to-one onto the launch panel, with the
// per-OS process details in procx (XMCL's middleware idea). Game arguments,
// including the session token, travel over stdin to the wrapper jar; they are
// never on the command line. The game outlives the launcher (no PDEATHSIG,
// unlike Pandora).
package launch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"quartz/internal/accounts"
	"quartz/internal/auth"
	"quartz/internal/core/events"
	"quartz/internal/core/logging"
	"quartz/internal/core/sysinfo"
	"quartz/internal/core/tasks"
	"quartz/internal/gamelog"
	"quartz/internal/install"
	"quartz/internal/instance"
	"quartz/internal/java"
	"quartz/internal/loader"
	"quartz/internal/mojang"
	"quartz/internal/settings"
)

// Stages in the order the launch panel lists them.
const (
	StageSignIn = iota
	StageMods
	StageJava
	StageFiles
	StageSandbox
	StageStart
	stageCount
)

// Errors returned to the UI.
var (
	ErrBusy       = errors.New("launch: this instance is already starting or running")
	ErrNotRunning = errors.New("launch: this instance is not running")
)

// Hooks let the app react to games starting and ending (hide or restore the
// window, or close the launcher).
type Hooks struct {
	GameStarted func(onGameStart string)
	AllExited   func()
}

// Sessions hands out the account and session to launch with (id "" means
// the default account), refreshing Microsoft sign-ins as needed.
type Sessions interface {
	Session(ctx context.Context, id string) (accounts.Account, accounts.Session, error)
}

// Config wires a Launcher.
type Config struct {
	Instances *instance.Store
	Accounts  Sessions
	Installer *install.Installer
	Loaders   *loader.Registry
	Java      *java.Manager
	Tasks     *tasks.Engine
	Settings  *settings.Store
	Bus       *events.Bus
	Redactor  *logging.Redactor
	BinDir    string
	Log       *slog.Logger
	Hooks     Hooks
	// Mods checks an instance's mods before launch (content); nil skips it.
	Mods ModChecker
}

// ModChecker summarises the mods a launch will load. It never blocks a
// launch: warn marks a summary that names a problem.
type ModChecker interface {
	Quick(ctx context.Context, id string) (count int, detail string, warn bool, err error)
}

// Launcher is safe for concurrent use.
type Launcher struct {
	cfg Config

	mu        sync.Mutex
	launching map[string]context.CancelFunc
	running   map[string]*run
	last      map[string]*gamelog.Buffer // the console of each instance's last run
}

// New returns a Launcher.
func New(c Config) *Launcher {
	if c.Log == nil {
		c.Log = slog.New(slog.DiscardHandler)
	}
	return &Launcher{cfg: c, launching: map[string]context.CancelFunc{}, running: map[string]*run{}, last: map[string]*gamelog.Buffer{}}
}

// Progress is what launch:progress carries.
type Progress struct {
	InstanceID string   `json:"instanceId"`
	Stage      int      `json:"stage"`
	Details    []string `json:"details"`
	Pct        int      `json:"pct"`
}

// Failure is what launch:failed carries. Code "no-account" asks the UI to
// open the add-account dialog, "sign-in-expired" to sign AccountID in again.
type Failure struct {
	InstanceID string `json:"instanceId"`
	Stage      int    `json:"stage"`
	Message    string `json:"message"`
	Code       string `json:"code,omitempty"`
	AccountID  string `json:"accountId,omitempty"`
}

// State is what game:state carries.
type State struct {
	InstanceID string `json:"instanceId"`
	RunID      string `json:"runId"`
	State      string `json:"state"` // running | exited
	ExitCode   int    `json:"exitCode"`
	Result     Result `json:"result,omitempty"`
}

// Status is the launcher's current state, for a UI that just loaded.
type Status struct {
	Running   []string `json:"running"`
	Launching []string `json:"launching"`
}

// Status lists running and starting instances.
func (l *Launcher) Status() Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := Status{Running: []string{}, Launching: []string{}}
	for id := range l.running {
		s.Running = append(s.Running, id)
	}
	for id := range l.launching {
		s.Launching = append(s.Launching, id)
	}
	return s
}

// AccountInUse reports whether a running game plays with an account.
func (l *Launcher) AccountInUse(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.running {
		if r.accountID == id {
			return true
		}
	}
	return false
}

// Running reports whether an instance's game is running.
func (l *Launcher) Running(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.running[id]
	return ok
}

// Busy reports whether an instance is starting or running, when its files
// must not move.
func (l *Launcher) Busy(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, launching := l.launching[id]
	_, running := l.running[id]
	return launching || running
}

// Play starts the launch pipeline in the background with an account
// (accountID "" means the default one; "Launch as" passes another, for this
// launch only). Progress and the outcome arrive as events.
func (l *Launcher) Play(id, accountID string) error {
	inst, err := l.cfg.Instances.Get(id)
	if err != nil {
		return err
	}
	l.mu.Lock()
	if _, ok := l.launching[id]; ok {
		l.mu.Unlock()
		return ErrBusy
	}
	if _, ok := l.running[id]; ok {
		l.mu.Unlock()
		return ErrBusy
	}
	ctx, cancel := context.WithCancel(context.Background())
	l.launching[id] = cancel
	l.mu.Unlock()
	go l.pipeline(ctx, inst, accountID)
	return nil
}

// CancelLaunch stops a launch that has not started the game yet.
func (l *Launcher) CancelLaunch(id string) {
	l.mu.Lock()
	cancel := l.launching[id]
	l.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Live returns the console lines of the running (or last) run.
func (l *Launcher) Live(id string) []gamelog.Line {
	l.mu.Lock()
	defer l.mu.Unlock()
	if r, ok := l.running[id]; ok {
		return r.buf.Lines()
	}
	if b, ok := l.last[id]; ok {
		return b.Lines()
	}
	return []gamelog.Line{}
}

// Runs returns an instance's launch history, newest first.
func (l *Launcher) Runs(id string) []Record { return listRecords(l.cfg.Instances.RunsDir(id)) }

// pipeline runs the launch steps; each failure ends it with launch:failed.
// details is also written from the task engine's event goroutine, so it is
// guarded.
type pipeline struct {
	l       *Launcher
	inst    instance.Instance
	eff     instance.Effective // inst's settings over the global ones
	account string
	// signedIn is the account the sign-in step resolved, for failure events.
	signedIn string
	// mods is how many enabled mods the launch loads.
	mods int

	mu      sync.Mutex
	details []string
}

func (p *pipeline) set(stage int, detail string) {
	p.mu.Lock()
	p.details[stage] = detail
	p.mu.Unlock()
}

func (p *pipeline) progress(stage, pct int) {
	p.mu.Lock()
	d := append([]string(nil), p.details...)
	p.mu.Unlock()
	p.l.cfg.Bus.Emit(events.LaunchProgress, Progress{InstanceID: p.inst.ID, Stage: stage, Details: d, Pct: pct})
}

func (l *Launcher) pipeline(ctx context.Context, inst instance.Instance, accountID string) {
	p := &pipeline{l: l, inst: inst, eff: inst.Resolve(l.cfg.Settings.Get()), account: accountID, details: make([]string, stageCount)}
	stage, err := p.run(ctx)
	l.mu.Lock()
	delete(l.launching, inst.ID)
	l.mu.Unlock()
	if err == nil {
		return
	}
	f := Failure{InstanceID: inst.ID, Stage: stage, Message: err.Error()}
	switch {
	case errors.Is(err, accounts.ErrNone):
		f.Code, f.Message = "no-account", "Add an account first"
	case errors.Is(err, accounts.ErrSignIn):
		f.Code, f.AccountID, f.Message = "sign-in-expired", p.signedIn, "Sign in again: "+auth.Describe(err).Message
	case ctx.Err() != nil:
		f.Code, f.Message = "cancelled", "Launch cancelled"
	}
	if f.Code != "cancelled" {
		l.cfg.Log.Warn("launch failed", "instance", inst.ID, "stage", stage, "err", err)
	}
	l.cfg.Bus.Emit(events.LaunchFailed, f)
}

// run executes the steps and returns the stage a failure happened in.
func (p *pipeline) run(ctx context.Context) (int, error) {
	l := p.l
	p.progress(StageSignIn, 0)
	acc, sess, err := l.cfg.Accounts.Session(ctx, p.account)
	p.signedIn = acc.ID
	if err != nil {
		return StageSignIn, err
	}
	switch {
	case sess.Fallback:
		p.set(StageSignIn, "offline, Microsoft unreachable")
	case sess.UserType == "msa":
		p.set(StageSignIn, "online")
	default:
		p.set(StageSignIn, "offline")
	}

	p.set(StageMods, p.checkMods(ctx))
	p.progress(StageJava, 10)

	gameDir := l.cfg.Instances.GameDir(p.inst.ID)
	if err := os.MkdirAll(gameDir, 0o700); err != nil {
		return StageJava, err
	}
	env := mojang.CurrentEnv(sysinfo.OSVersion())
	env.Features["has_custom_resolution"] = p.eff.Width > 0 && p.eff.Height > 0
	res, err := l.cfg.Installer.Resolve(ctx, target(p.inst), gameDir, env)
	if err != nil {
		return StageJava, err
	}
	// The game uses the instance's Java override if it has one; a loader's
	// installer always runs with the Java its Minecraft version needs.
	own := java.Request{Major: res.Version.JavaMajor(), Component: res.Version.JavaComponent()}
	req := own
	req.Override = p.eff.JavaPath
	rt, plan, err := l.cfg.Java.Resolve(ctx, req)
	if err != nil {
		return StageJava, err
	}
	if res.Processing != nil && req.Override != "" && plan == nil && !res.Processing.Done() {
		if _, ownPlan, err := l.cfg.Java.Resolve(ctx, own); err == nil {
			plan = ownPlan
		}
	}
	mem := p.memory()
	if plan != nil {
		p.set(StageJava, fmt.Sprintf("Java %d, downloading", plan.Major))
	} else {
		p.set(StageJava, "Java "+rt.Version)
	}
	if warn := l.lowMemory(mem.Max); warn != "" {
		p.mu.Lock()
		p.details[StageJava] += ", " + warn
		p.mu.Unlock()
	}
	p.progress(StageFiles, 20)

	if err := p.install(ctx, res, plan, l.processorJava(own)); err != nil {
		return StageFiles, err
	}
	if plan != nil {
		if rt, _, err = l.cfg.Java.Resolve(ctx, req); err != nil {
			return StageFiles, err
		}
		p.set(StageJava, "Java "+rt.Version)
	}
	p.set(StageFiles, "Ready")
	p.set(StageSandbox, "Off")
	p.progress(StageStart, 90)

	sp := spec{inst: p.inst, eff: p.eff, acc: acc, sess: sess, res: res, rt: rt, mem: mem, env: env, gameDir: gameDir, mods: p.mods}
	sp.vars = instVars(p.inst, l.cfg.Instances.Dir(p.inst.ID), gameDir, rt.Path)
	if pre := p.eff.Commands.PreLaunch; pre != "" {
		p.set(StageStart, "Before-launch command")
		p.progress(StageStart, 92)
		if err := runCommand(ctx, pre, gameDir, sp.vars, gameEnv(sp.vars, p.eff)); err != nil {
			return StageStart, fmt.Errorf("before-launch command: %w", err)
		}
		p.set(StageStart, "")
	}
	r, err := l.start(ctx, sp)
	if err != nil {
		return StageStart, err
	}
	select {
	case <-r.firstOutput:
	case <-r.done:
	case <-ctx.Done():
		// The game is already starting; cancelling now would only orphan it.
	}
	return StageStart, nil
}

// target is what an instance plays.
func target(in instance.Instance) install.Target {
	return install.Target{Minecraft: in.Minecraft, Loader: loader.Kind(in.Loader), LoaderVersion: in.LoaderVersion}
}

// subtitle names what an instance plays: "Fabric 0.16.10 for Minecraft 1.21.1".
func subtitle(in instance.Instance) string {
	if in.Loader == "" {
		return "Minecraft " + in.Minecraft
	}
	return loader.Label(loader.Kind(in.Loader), in.Minecraft, in.LoaderVersion) + " for Minecraft " + in.Minecraft
}

// checkMods is the "Checking mods" stage: the loader, kept up to date, and a
// summary of the mods (or the first missing dependency).
func (p *pipeline) checkMods(ctx context.Context) string {
	label := p.updateLoader(ctx)
	if p.inst.Loader == "" || p.l.cfg.Mods == nil {
		return label
	}
	n, detail, _, err := p.l.cfg.Mods.Quick(ctx, p.inst.ID)
	if err != nil {
		p.l.cfg.Log.Info("checking mods failed", "instance", p.inst.ID, "err", err)
		return label
	}
	p.mods = n
	return detail + ", " + label
}

// updateLoader moves the instance to its loader's newest stable build when
// it asks for that, and returns the mods step's detail. Being offline or the
// loader's site being down never stops a launch: the installed build is used.
func (p *pipeline) updateLoader(ctx context.Context) string {
	in := p.inst
	if in.Loader == "" {
		return "No mods"
	}
	k := loader.Kind(in.Loader)
	if in.KeepLoaderUpdated && p.l.cfg.Loaders != nil {
		latest, err := p.l.cfg.Loaders.Latest(ctx, k, in.Minecraft)
		cur := loader.Display(k, in.Minecraft, in.LoaderVersion)
		if err == nil && latest.ID != in.LoaderVersion && loader.Compare(latest.Name, cur) > 0 {
			if updated, err := p.l.cfg.Instances.Update(in.ID, func(i *instance.Instance) error { i.LoaderVersion = latest.ID; return nil }); err == nil {
				p.inst = updated
				p.l.cfg.Bus.Emit(events.InstancesChanged, nil)
				return "Updated to " + latest.Name
			}
		}
	}
	return loader.Label(k, in.Minecraft, p.inst.LoaderVersion)
}

// processorJava finds the Java a loader's installer runs with, after the
// install job's own Java download (if any) has finished.
func (l *Launcher) processorJava(req java.Request) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		rt, plan, err := l.cfg.Java.Resolve(ctx, req)
		if err != nil {
			return "", err
		}
		if plan != nil || rt.Path == "" {
			return "", fmt.Errorf("launch: Java %d is needed to finish installing the mod loader", req.Major)
		}
		return rt.Path, nil
	}
}

// install runs the install job (if anything is missing) and mirrors its
// progress into the launch panel.
func (p *pipeline) install(ctx context.Context, res *install.Resolved, plan *java.Plan, procJava func(context.Context) (string, error)) error {
	l := p.l
	spec, err := l.cfg.Installer.Plan(install.Request{
		Resolved: res, Java: plan, ProcessorJava: procJava,
		Meta: tasks.Meta{Title: p.inst.Name, Subtitle: subtitle(p.inst), Art: tasks.Art{Background: p.inst.Art.Background, Initial: p.inst.Art.Initial}, Kind: "install"},
	})
	if err != nil || spec == nil {
		return err
	}
	id, err := l.cfg.Tasks.Submit(spec)
	if err != nil || id == "" {
		return err
	}
	off := l.cfg.Bus.Subscribe(events.TasksChanged, func(payload any) {
		snap, ok := payload.(tasks.Snapshot)
		if !ok {
			return
		}
		for _, j := range snap.Active {
			if j.ID == id {
				p.set(StageFiles, filesDetail(j))
				pct := 20
				if j.TotalBytes > 0 {
					pct += int(65 * j.DoneBytes / j.TotalBytes)
				}
				p.progress(StageFiles, pct)
			}
		}
	})
	defer off()
	if err := l.cfg.Tasks.Wait(ctx, id); err != nil {
		if ctx.Err() != nil {
			_ = l.cfg.Tasks.Cancel(id)
		}
		return err
	}
	return nil
}

// filesDetail is the launch panel's counter, e.g. "2,318 / 3,604".
func filesDetail(j tasks.JobView) string {
	for _, s := range j.Steps {
		if s.Unit == tasks.Count && s.Name == "Assets" && s.State != tasks.StepDone {
			return thousands(s.Done) + " / " + thousands(s.Total)
		}
	}
	if j.TotalBytes > 0 {
		return fmt.Sprintf("%d of %d MB", j.DoneBytes>>20, j.TotalBytes>>20)
	}
	return "Preparing"
}

func thousands(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// memory is the heap the game gets.
func (p *pipeline) memory() MemoryRange {
	return MemoryRange{Min: p.eff.MemoryMinMB, Max: p.eff.MemoryMaxMB}
}

// lowMemory warns when free RAM is below the maximum heap (if enabled).
func (l *Launcher) lowMemory(maxMB int) string {
	if !l.cfg.Settings.Get().Java.WarnLowMemory {
		return ""
	}
	mem, err := sysinfo.ReadMemory()
	if err != nil || mem.Available == 0 || mem.Available>>20 >= uint64(maxMB) { //nolint:gosec // maxMB is a validated positive setting
		return ""
	}
	return fmt.Sprintf("only %.1f GB free", float64(mem.Available)/(1<<30))
}

// Close stops nothing: running games outlive the launcher. It only cancels
// launches that have not started a game yet.
func (l *Launcher) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, cancel := range l.launching {
		cancel()
	}
}

// Repair verifies every file of an instance's version again by hashing and
// re-downloads anything damaged. It returns the job id ("" if nothing to do).
func (l *Launcher) Repair(ctx context.Context, id string) (string, error) {
	inst, err := l.cfg.Instances.Get(id)
	if err != nil {
		return "", err
	}
	res, err := l.cfg.Installer.Resolve(ctx, target(inst), l.cfg.Instances.GameDir(id), mojang.CurrentEnv(sysinfo.OSVersion()))
	if err != nil {
		return "", err
	}
	spec, err := l.cfg.Installer.Plan(install.Request{
		Resolved: res, Force: true, ProcessorJava: l.processorJava(java.Request{Major: res.Version.JavaMajor(), Component: res.Version.JavaComponent()}),
		Meta: tasks.Meta{Title: inst.Name, Subtitle: "Repairing game files", Art: tasks.Art{Background: inst.Art.Background, Initial: inst.Art.Initial}, Kind: "repair"},
	})
	if err != nil || spec == nil {
		return "", err
	}
	return l.cfg.Tasks.Submit(spec)
}
