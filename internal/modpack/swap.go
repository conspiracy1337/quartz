package modpack

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"

	"quartz/internal/instance"
)

// Suffixes of the files an update stages: the new version beside its
// target, and the old one set aside until the update is recorded.
const (
	newSuffix = ".quartz-new"
	oldSuffix = ".quartz-old"
)

// swap replaces files in an instance's folders so that all of them can be
// put back: new files are staged beside their targets, then each target is
// renamed aside and the staged file renamed in. Renames stay in one folder,
// so each is atomic.
type swap struct {
	instances *instance.Store
	id        string
	roots     map[string]*os.Root
	staged    map[string]bool // instance-relative names of staged files
	done      []renamed       // renames made, in order
}

type renamed struct {
	root     *os.Root
	from, to string
}

func newSwap(instances *instance.Store, id string) *swap {
	return &swap{instances: instances, id: id, roots: map[string]*os.Root{}, staged: map[string]bool{}}
}

// root opens the folder rel lives in: the game folder, or a shared one.
func (w *swap) root(rel string) (*os.Root, error) {
	b := w.instances.Base(w.id, rel)
	if r, ok := w.roots[b]; ok {
		return r, nil
	}
	r, err := os.OpenRoot(b)
	if err == nil {
		w.roots[b] = r
	}
	return r, err
}

// stage puts a new file at name+newSuffix through place, which writes the
// absolute path it is given.
func (w *swap) stage(name string, place func(dst string) error) error {
	r, err := w.root(name)
	if err != nil {
		return err
	}
	if dir := path.Dir(name); dir != "." {
		if err := r.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	staged := name + newSuffix
	if err := r.Remove(staged); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	w.staged[staged] = true
	return place(w.instances.Path(w.id, staged))
}

// isStaged reports whether name has a staged new file.
func (w *swap) isStaged(name string) bool { return w.staged[name+newSuffix] }

// replace sets whatever is at rel (on or off) aside and, with write, moves
// the staged file in as name.
func (w *swap) replace(rel, name string, write bool) error {
	r, err := w.root(rel)
	if err != nil {
		return err
	}
	for _, suffix := range []string{"", ".disabled"} {
		cur := rel + suffix
		if _, err := r.Lstat(cur); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err := w.rename(r, cur, cur+oldSuffix); err != nil {
			return err
		}
	}
	if !write {
		return nil
	}
	return w.rename(r, name+newSuffix, name)
}

func (w *swap) rename(r *os.Root, from, to string) error {
	if err := r.Rename(from, to); err != nil {
		return err
	}
	w.done = append(w.done, renamed{r, from, to})
	return nil
}

// rollback undoes every rename and removes the staged files: the folder is
// as it was before the update.
func (w *swap) rollback() {
	for i := len(w.done) - 1; i >= 0; i-- {
		d := w.done[i]
		_ = d.root.Rename(d.to, d.from)
	}
	w.done = nil
	for staged := range w.staged {
		if r, err := w.root(staged); err == nil {
			_ = r.Remove(staged)
		}
	}
}

// finish deletes the old files once the update is recorded.
func (w *swap) finish() {
	for _, d := range w.done {
		if strings.HasSuffix(d.to, oldSuffix) {
			_ = d.root.Remove(d.to)
		}
	}
	w.done = nil
}

func (w *swap) close() {
	for _, r := range w.roots {
		_ = r.Close()
	}
}
