package launch

import (
	"context"
	"path/filepath"
	"runtime"
	"slices"

	"quartz/internal/java"
	"quartz/internal/runview"
	"quartz/internal/sandbox"
	"quartz/internal/sandbox/rules"
)

// assemble readies a run's files: shared files copied in, then the view of
// the game folder (shared folders in place; a private view when another run
// uses it). sp.gameDir becomes the folder the game runs in.
func (l *Launcher) assemble(sp *spec) error {
	if l.cfg.Syncing != nil {
		se, err := l.cfg.Syncing.Prepare(sp.inst.ID)
		if err != nil {
			return err
		}
		sp.session = se
	}
	if l.cfg.Views == nil {
		return nil
	}
	req := runview.Request{Game: sp.gameDir, Scratch: filepath.Join(l.cfg.Instances.RunsDir(sp.inst.ID), sp.runID+".view")}
	mode := runview.Shared
	if sp.sandboxed {
		switch sp.inst.Sandbox.Level(rules.Synced, runtime.GOOS) {
		case rules.RO:
			mode = runview.Private
		case rules.Blocked:
			mode = runview.Empty
		}
	}
	if l.cfg.Syncing != nil {
		for _, f := range l.cfg.Syncing.RunFolders(sp.inst.ID) {
			req.Folders = append(req.Folders, runview.Folder{Rel: f.Rel, Shared: f.Shared, Mode: mode})
		}
	}
	v, err := l.cfg.Views.Assemble(req)
	if err != nil {
		return err
	}
	sp.view = v
	sp.gameDir = v.GameDir()
	l.cfg.Log.Info("run view", "instance", sp.inst.ID, "run", sp.runID, "backend", v.Backend(), "folders", len(req.Folders))
	return nil
}

// sandboxRun is what a sandboxed run may reach: its game folder and view,
// the store, its Java and the player's own wrapper program.
func (l *Launcher) sandboxRun(sp *spec, argv []string) sandbox.Run {
	id := sp.inst.ID
	r := sandbox.Run{
		Instance: id, Rules: sp.inst.Sandbox, GameDir: sp.gameDir,
		Scratch:  filepath.Join(l.cfg.Instances.RunsDir(id), sp.runID+".view"),
		Data:     filepath.Join(l.cfg.Instances.Dir(id), "sandbox"),
		Read:     append(slices.Clone(l.cfg.StoreRead), sandbox.JavaRoot(sp.rt.Path)),
		GameMode: sp.eff.Performance.GameMode && runtime.GOOS == "linux",
	}
	if sp.view != nil {
		r.Mounts = sp.view.Mounts()
	}
	// Absolute programs in front of java (the player's wrapper, MangoHud).
	for _, a := range argv {
		if a == java.LaunchBinary(sp.rt.Path) {
			break
		}
		if filepath.IsAbs(a) {
			r.Read = append(r.Read, a)
		}
	}
	if l.cfg.Syncing != nil {
		for _, f := range l.cfg.Syncing.RunFolders(id) {
			r.Shared = append(r.Shared, f.Shared)
		}
	}
	return r
}

// disassemble merges a run's changes back and copies shared files out; it
// returns the worlds kept as copies.
func (l *Launcher) disassemble(sp *spec) []runview.Copy {
	sp.jail.Close()
	sp.jail = nil
	var kept []runview.Copy
	if sp.view != nil {
		var err error
		if kept, err = sp.view.Close(context.Background()); err != nil {
			l.cfg.Log.Warn("merging a run back failed", "instance", sp.inst.ID, "run", sp.runID, "err", err)
		}
		sp.view = nil
	}
	if sp.session != nil {
		if err := sp.session.Finish(); err != nil {
			l.cfg.Log.Warn("copying shared files out failed", "instance", sp.inst.ID, "err", err)
		}
		sp.session = nil
	}
	return kept
}

// Recover merges back run views left by a launcher that quit or crashed
// while games ran; it runs once at startup.
func (l *Launcher) Recover() {
	if l.cfg.Views == nil {
		return
	}
	var dirs []string
	for _, in := range l.cfg.Instances.List() {
		m, _ := filepath.Glob(filepath.Join(l.cfg.Instances.RunsDir(in.ID), "*.view"))
		dirs = append(dirs, m...)
	}
	if len(dirs) > 0 {
		l.cfg.Views.Recover(dirs)
	}
}
