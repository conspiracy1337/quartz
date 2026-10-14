package launch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"quartz/internal/core/linkx"
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
	req := runview.Request{Game: sp.gameDir, Scratch: l.scratch(sp)}
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

// natives is the folder the game loads and unpacks native libraries in.
// Minecraft also points JNA's and LWJGL's temporary files there, so a
// sandboxed run gets its own copy (a clone where the file system can,
// never a hardlink): the shared one stays read-only, so no run can plant a
// library another loads.
func (l *Launcher) natives(sp *spec) (string, error) {
	if !sp.sandboxed || sp.res.NativesDir == "" {
		return sp.res.NativesDir, nil
	}
	dst := filepath.Join(l.scratch(sp), "natives")
	if err := os.RemoveAll(dst); err != nil {
		return "", err
	}
	if _, err := os.Stat(sp.res.NativesDir); err != nil {
		return dst, os.MkdirAll(dst, 0o700)
	}
	return dst, linkx.CopyTree(sp.res.NativesDir, dst, linkx.Options{NoHardlink: true})
}

// scratch is the run's own folder.
func (l *Launcher) scratch(sp *spec) string {
	return filepath.Join(l.cfg.Instances.RunsDir(sp.inst.ID), sp.runID+".view")
}

// sandboxRun is what a sandboxed run may reach: its game folder and view,
// the store, its Java and the player's own wrapper program.
func (l *Launcher) sandboxRun(sp *spec, argv []string) sandbox.Run {
	id := sp.inst.ID
	r := sandbox.Run{
		Instance: id, Rules: sp.inst.Sandbox, GameDir: sp.gameDir,
		Scratch:  l.scratch(sp),
		Write:    []string{filepath.Join(l.scratch(sp), "natives")},
		Data:     filepath.Join(l.cfg.Instances.Dir(id), "sandbox"),
		Read:     append(slices.Clone(l.cfg.StoreRead), sandbox.JavaRoot(sp.rt.Path)),
		GameMode: sp.eff.Performance.GameMode && runtime.GOOS == "linux",
	}
	if sp.inst.ClientJar != nil {
		r.Read = append(r.Read, l.cfg.Instances.ClientJarPath(id))
	}
	if sp.view != nil {
		r.Mounts = sp.view.Mounts()
		r.Binds = sp.view.Binds()
	}
	// Before 1.21.4 the game saves a skin under the assets and reads it back
	// from there; the shared assets are read-only, so each instance gets its
	// own skin cache.
	if sp.res != nil && sp.res.AssetsRoot != "" {
		r.Redirects = []sandbox.Redirect{{From: filepath.Join(r.Data, "skins"), To: filepath.Join(sp.res.AssetsRoot, "skins")}}
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
