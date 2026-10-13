package runview

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"quartz/internal/core/fsx"
	"quartz/internal/core/linkx"
)

// A private view collects what the run changed; on exit the changes merge
// back into the real folder (the instance's, or a shared one), so another
// run's work there is never lost:
//   - a world is one unit: when both this run and the folder changed it since
//     the run started, the run's version is kept beside it as "W (copy 2)";
//   - any other file: the newer one wins;
//   - a deletion applies only to a file nobody changed since the start.

// change is one path the run added, changed or removed.
type change struct {
	Rel string `json:"rel"`           // slash-separated, inside the mount
	Src string `json:"src,omitempty"` // the run's file ("" for a deletion)
	Del bool   `json:"del,omitempty"`
}

// noMerge are files every run writes for itself; the run's own log is kept
// by the launcher instead.
var noMerge = []string{"logs/latest.log", "logs/debug.log", "logs/debug-1.log.gz"}

// stamp is a file's size and time in a snapshot.
type stamp struct {
	Size int64 `json:"s"`
	Mod  int64 `json:"m"` // UnixNano
}

// snapshot lists the regular files under dir, skipping the mount points of
// other mounts (skip) and anything that is a link.
func snapshot(dir string, skip []string) (map[string]stamp, error) {
	out := map[string]stamp{}
	root, err := os.OpenRoot(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != "." && slices.Contains(skip, p) {
			return fs.SkipDir
		}
		if fsx.IsLink(d.Type()) || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[p] = stamp{info.Size(), info.ModTime().UnixNano()}
		return nil
	})
	return out, err
}

// diff lists what changed under dir since before.
func diff(dir string, before map[string]stamp, skip []string) ([]change, error) {
	after, err := snapshot(dir, skip)
	if err != nil {
		return nil, err
	}
	var out []change
	for rel, st := range after {
		if old, ok := before[rel]; !ok || old != st {
			out = append(out, change{Rel: rel, Src: filepath.Join(dir, filepath.FromSlash(rel))})
		}
	}
	for rel := range before {
		if _, ok := after[rel]; !ok {
			out = append(out, change{Rel: rel, Del: true})
		}
	}
	return out, nil
}

// Copy is a world the run changed while something else changed it too.
type Copy struct {
	World string `json:"world"` // the original world's folder
	As    string `json:"as"`    // the folder the run's version went to
}

// merger applies changes to one real folder.
type merger struct {
	target  string
	worlds  string // where worlds are inside target ("saves", "" for a shared saves folder, "-" for none)
	start   time.Time
	retitle func(dir, suffix string) error
}

// copySuffix names a world kept from a run that clashed.
const copySuffix = " (copy 2)"

func (m merger) world(rel string) (string, bool) {
	if m.worlds == "-" {
		return "", false
	}
	rest := rel
	if m.worlds != "" {
		var ok bool
		if rest, ok = strings.CutPrefix(rel, m.worlds+"/"); !ok {
			return "", false
		}
	}
	w, _, ok := strings.Cut(rest, "/")
	return w, ok
}

func (m merger) worldDir(w string) string {
	if m.worlds == "" || m.worlds == "-" {
		return w
	}
	return path.Join(m.worlds, w)
}

// changedSinceStart reports whether rel in the target was written after the
// run started (by another run, or the player).
func (m merger) changedSinceStart(rel string) bool {
	info, err := os.Stat(filepath.Join(m.target, filepath.FromSlash(rel)))
	return err == nil && info.ModTime().After(m.start)
}

func (m merger) apply(changes []change) ([]Copy, error) {
	byWorld := map[string][]change{}
	var rest []change
	for _, c := range changes {
		if slices.Contains(noMerge, c.Rel) || path.Base(c.Rel) == "session.lock" {
			continue
		}
		if w, ok := m.world(c.Rel); ok {
			byWorld[w] = append(byWorld[w], c)
		} else {
			rest = append(rest, c)
		}
	}
	var copies []Copy
	var errs []error
	for _, w := range slices.Sorted(maps.Keys(byWorld)) {
		dir := m.worldDir(w)
		if m.changedSinceStart(path.Join(dir, "level.dat")) {
			as, err := m.keepCopy(dir, byWorld[w])
			if err == nil {
				copies = append(copies, Copy{World: w, As: path.Base(as)})
			}
			errs = append(errs, err)
			continue
		}
		for _, c := range byWorld[w] {
			errs = append(errs, m.put(c, true))
		}
	}
	for _, c := range rest {
		errs = append(errs, m.put(c, false))
	}
	return copies, errors.Join(errs...)
}

// keepCopy saves the run's version of a world both sides changed: the
// folder's current world, with every file the run changed on top.
func (m merger) keepCopy(dir string, changes []change) (string, error) {
	parent := filepath.Join(m.target, filepath.FromSlash(path.Dir(dir)))
	name := fsx.FreeName(parent, path.Base(dir)+copySuffix, true)
	to := filepath.Join(parent, name)
	if err := linkx.CloneTree(filepath.Join(m.target, filepath.FromSlash(dir)), to); err != nil {
		return "", err
	}
	copyMerger := m
	copyMerger.target = to
	for _, c := range changes {
		c.Rel = strings.TrimPrefix(strings.TrimPrefix(c.Rel, dir), "/")
		if err := copyMerger.put(c, true); err != nil {
			return to, err
		}
	}
	if m.retitle != nil {
		_ = m.retitle(to, strings.TrimPrefix(name, path.Base(dir)))
	}
	return to, nil
}

// put applies one change. force skips the newest-wins check (a world the
// run owns).
func (m merger) put(c change, force bool) error {
	root, err := os.OpenRoot(m.target)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	rel := filepath.FromSlash(c.Rel)
	cur, err := root.Lstat(rel)
	exists := err == nil
	if exists && fsx.IsLink(cur.Mode()) {
		return nil // never write through a link
	}
	if c.Del {
		if exists && (force || !cur.ModTime().After(m.start)) {
			return root.RemoveAll(rel)
		}
		return nil
	}
	src, err := os.Lstat(c.Src)
	if err != nil || !src.Mode().IsRegular() {
		return nil // only regular files travel
	}
	if exists && !force && cur.ModTime().After(src.ModTime()) {
		return nil // the folder's copy is newer
	}
	if dir := filepath.Dir(rel); dir != "." {
		if err := root.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	// The parent folders were made through the root, but one that already
	// existed could be a link: the file must really land inside the target.
	dst := filepath.Join(m.target, rel)
	realTarget, err1 := filepath.EvalSymlinks(m.target)
	realDir, err2 := filepath.EvalSymlinks(filepath.Dir(dst))
	if err1 != nil || err2 != nil || !fsx.Inside(realTarget, realDir) {
		return nil
	}
	if _, err := linkx.Place(c.Src, dst, linkx.Options{NoHardlink: true}); err != nil {
		return err
	}
	return os.Chtimes(dst, src.ModTime(), src.ModTime())
}
