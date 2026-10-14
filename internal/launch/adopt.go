package launch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"quartz/internal/core/events"
	"quartz/internal/core/fsx"
	"quartz/internal/core/procx"
	"quartz/internal/gamelog"
	"quartz/internal/instance"
	"quartz/internal/syncing"
)

// A game outlives the launcher: "when a game starts: close" quits it on
// purpose, and it may crash. Each running game therefore leaves a pending
// file beside its record, and the next launcher finishes the run: at once
// when the game is gone, or by adopting a game that still runs. Without it
// the run's option changes were overwritten by the shared ones on the next
// launch, its record and playtime were lost, and nothing counted it as
// running, so its worlds and shared folders could be changed under it.

const (
	pendingExt = ".pending.json"
	// adoptPoll is how often an adopted game is checked for having exited.
	adoptPoll = 2 * time.Second
)

// pending is what a running game leaves in its runs folder.
type pending struct {
	Record  Record    `json:"record"`
	Started time.Time `json:"started"`
	PID     int       `json:"pid"`
	Ident   uint64    `json:"ident"`
	// View says the run has a view, in <runs>/<id>.view.
	View bool                  `json:"view,omitempty"`
	Sync *syncing.SessionState `json:"sync,omitempty"`
}

func pendingPath(runsDir, runID string) string { return filepath.Join(runsDir, runID+pendingExt) }

// savePending records a started game for a launcher that doesn't see it end.
func (l *Launcher) savePending(r *run) {
	p := pending{Record: r.record, Started: r.started, PID: r.pid, Ident: r.ident, View: r.sp.view != nil}
	p.Record.Server = r.server
	if r.sp.session != nil {
		st := r.sp.session.State()
		p.Sync = &st
	}
	data, err := json.Marshal(p)
	if err == nil {
		err = fsx.AtomicWrite(pendingPath(l.cfg.Instances.RunsDir(r.instanceID), r.id), data, fsx.FilePerm)
	}
	if err != nil {
		l.cfg.Log.Warn("recording the running game failed", "instance", r.instanceID, "err", err)
	}
}

// Recover finishes the runs a launcher left behind when it quit or crashed
// while their games ran. A game that still runs is adopted: it counts as
// running, Stop works, and the run is finished when it exits. Views without
// a pending run (left by launchers before this) are merged back once their
// game has gone. It runs once at startup.
func (l *Launcher) Recover() {
	handled := map[string]bool{}
	var views []string
	for _, in := range l.cfg.Instances.List() {
		dir := l.cfg.Instances.RunsDir(in.ID)
		files, _ := filepath.Glob(filepath.Join(dir, "*"+pendingExt))
		for _, f := range files {
			if id, ok := l.adopt(in, f); ok {
				handled[filepath.Join(dir, id+".view")] = true
			}
		}
		m, _ := filepath.Glob(filepath.Join(dir, "*.view"))
		views = append(views, m...)
	}
	if l.cfg.Views == nil {
		return
	}
	var rest []string
	for _, v := range views {
		if !handled[v] {
			rest = append(rest, v)
		}
	}
	if len(rest) > 0 {
		l.cfg.Views.Recover(rest)
	}
}

// adopt takes over one pending run and returns its id.
func (l *Launcher) adopt(in instance.Instance, file string) (string, bool) {
	runsDir := l.cfg.Instances.RunsDir(in.ID)
	var p pending
	data, err := fsx.ReadCapped(file, 1<<20)
	if err != nil || json.Unmarshal(data, &p) != nil || !ValidRunID(p.Record.ID) || file != pendingPath(runsDir, p.Record.ID) {
		l.cfg.Log.Warn("an unreadable left-behind run was dropped", "file", file)
		_ = os.Remove(file)
		return "", false
	}
	id := p.Record.ID
	r := &run{
		id: id, instanceID: in.ID, started: p.Started, accountID: p.Record.AccountID, done: make(chan struct{}),
		record: p.Record, server: p.Record.Server, pid: p.PID, ident: p.Ident,
		con: gamelog.NewConsole(consoleLines, l.cfg.Redactor, logBatch, nil, nil),
		sp:  spec{inst: in, runID: id},
	}
	if p.View && l.cfg.Views != nil {
		v, err := l.cfg.Views.Adopt(filepath.Join(runsDir, id+".view"))
		if err != nil {
			l.cfg.Log.Warn("a left-behind run view couldn't be read", "instance", in.ID, "run", id, "err", err)
		} else {
			r.sp.view = v
		}
	}
	if p.Sync != nil && p.Sync.ID == in.ID && l.cfg.Syncing != nil {
		r.sp.session = l.cfg.Syncing.Resume(*p.Sync)
	}
	r.con.Done()
	if !procx.Same(p.PID, p.Ident) {
		l.cfg.Log.Info("finishing a run that ended while the launcher was closed", "instance", in.ID, "run", id)
		l.finish(r, l.endedAt(r), 0, ResultOK)
		return id, true
	}
	l.cfg.Log.Info("adopting a game that outlived the launcher", "instance", in.ID, "run", id, "pid", p.PID)
	l.mu.Lock()
	l.running[in.ID] = append(l.running[in.ID], r)
	copies := len(l.running[in.ID])
	l.mu.Unlock()
	l.cfg.Bus.Emit(events.GameState, State{InstanceID: in.ID, RunID: id, State: "running", Copies: copies})
	l.cfg.Bus.Emit(events.InstancesChanged, nil)
	go l.watch(r)
	return id, true
}

// watch finishes an adopted run once its game has exited. Its exit code is
// unknown; a crash still shows through the report it left.
func (l *Launcher) watch(r *run) {
	t := time.NewTicker(adoptPoll)
	for range t.C {
		if !procx.Same(r.pid, r.ident) {
			break
		}
	}
	t.Stop()
	result := ResultOK
	if r.killed.Load() {
		result = ResultKilled
	}
	l.finish(r, time.Now(), 0, result)
}

// endedAt guesses when a game that exited unseen stopped: the last write to
// its log, else now.
func (l *Launcher) endedAt(r *run) time.Time {
	dir := l.cfg.Instances.GameDir(r.instanceID)
	if r.sp.view != nil {
		dir = r.sp.view.GameDir()
	}
	now := time.Now()
	if fi, err := os.Stat(filepath.Join(dir, "logs", "latest.log")); err == nil && fi.ModTime().After(r.started) && fi.ModTime().Before(now) {
		return fi.ModTime()
	}
	return now
}
