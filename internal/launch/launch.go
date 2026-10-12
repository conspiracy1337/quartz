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
	"quartz/internal/core/events"
	"quartz/internal/core/logging"
	"quartz/internal/core/sysinfo"
	"quartz/internal/core/tasks"
	"quartz/internal/gamelog"
	"quartz/internal/install"
	"quartz/internal/instance"
	"quartz/internal/java"
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

// Config wires a Launcher.
type Config struct {
	Instances *instance.Store
	Accounts  *accounts.Store
	Installer *install.Installer
	Java      *java.Manager
	Tasks     *tasks.Engine
	Settings  *settings.Store
	Bus       *events.Bus
	Redactor  *logging.Redactor
	BinDir    string
	Log       *slog.Logger
	Hooks     Hooks
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
// open the add-account dialog.
type Failure struct {
	InstanceID string `json:"instanceId"`
	Stage      int    `json:"stage"`
	Message    string `json:"message"`
	Code       string `json:"code,omitempty"`
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

// Running reports whether an instance's game is running.
func (l *Launcher) Running(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.running[id]
	return ok
}

// Play starts the launch pipeline in the background. Progress and the
// outcome arrive as events.
func (l *Launcher) Play(id string) error {
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
	go l.pipeline(ctx, inst)
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
	l    *Launcher
	inst instance.Instance

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

func (l *Launcher) pipeline(ctx context.Context, inst instance.Instance) {
	p := &pipeline{l: l, inst: inst, details: make([]string, stageCount)}
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
	acc, err := l.cfg.Accounts.Default()
	if err != nil {
		return StageSignIn, err
	}
	p.set(StageSignIn, acc.Name)

	p.set(StageMods, "No mods")
	p.progress(StageJava, 10)

	gameDir := l.cfg.Instances.GameDir(p.inst.ID)
	if err := os.MkdirAll(gameDir, 0o700); err != nil {
		return StageJava, err
	}
	win := p.inst.Settings.Window
	env := mojang.CurrentEnv(sysinfo.OSVersion())
	env.Features["has_custom_resolution"] = win.Width > 0 && win.Height > 0
	res, err := l.cfg.Installer.Resolve(ctx, p.inst.Minecraft, gameDir, env)
	if err != nil {
		return StageJava, err
	}
	req := java.Request{Major: res.Version.JavaMajor(), Component: res.Version.JavaComponent()}
	if j := p.inst.Settings.Java; j.Override && j.JavaPath != "" {
		req.Override = j.JavaPath
	}
	rt, plan, err := l.cfg.Java.Resolve(ctx, req)
	if err != nil {
		return StageJava, err
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

	if err := p.install(ctx, res, plan); err != nil {
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

	r, err := l.start(ctx, p.inst, acc, res, rt, mem, env)
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

// install runs the install job (if anything is missing) and mirrors its
// progress into the launch panel.
func (p *pipeline) install(ctx context.Context, res *install.Resolved, plan *java.Plan) error {
	l := p.l
	spec, err := l.cfg.Installer.Plan(install.Request{
		Resolved: res, Java: plan,
		Meta: tasks.Meta{Title: p.inst.Name, Subtitle: "Minecraft " + p.inst.Minecraft, Art: tasks.Art{Background: p.inst.Art.Background, Initial: p.inst.Art.Initial}, Kind: "install"},
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

// memory resolves the instance's memory, falling back to the global setting.
func (p *pipeline) memory() MemoryRange {
	g := p.l.cfg.Settings.Get().Java
	m := MemoryRange{Min: g.MemoryMinMB, Max: g.MemoryMaxMB}
	if j := p.inst.Settings.Java; j.Override && j.MemoryMaxMB > 0 {
		m = MemoryRange{Min: max(j.MemoryMinMB, 256), Max: j.MemoryMaxMB}
	}
	m.Min = min(m.Min, m.Max)
	return m
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
	res, err := l.cfg.Installer.Resolve(ctx, inst.Minecraft, l.cfg.Instances.GameDir(id), mojang.CurrentEnv(sysinfo.OSVersion()))
	if err != nil {
		return "", err
	}
	spec, err := l.cfg.Installer.Plan(install.Request{
		Resolved: res, Force: true,
		Meta: tasks.Meta{Title: inst.Name, Subtitle: "Repairing game files", Art: tasks.Art{Background: inst.Art.Background, Initial: inst.Art.Initial}, Kind: "repair"},
	})
	if err != nil || spec == nil {
		return "", err
	}
	return l.cfg.Tasks.Submit(spec)
}
