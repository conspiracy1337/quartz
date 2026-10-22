package instance

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"
)

// Suffixes of the files a swap stages: the new version beside its target,
// and the old one set aside until the change is recorded. Scans skip both.
const (
	NewSuffix = ".quartz-new"
	OldSuffix = ".quartz-old"
)

// Swap replaces files in an instance's folders so that all of them can be
// put back: new files are staged beside their targets, then each target is
// set aside and the staged file renamed in. Renames stay in one folder, so
// each is atomic. Modpack updates and rollbacks both go through it.
type Swap struct {
	instances *Store
	id        string
	roots     map[string]*os.Root
	staged    map[string]bool // instance-relative names of staged files
	done      []renamed       // renames made, in order
}

type renamed struct {
	root     *os.Root
	from, to string
}

// NewSwap starts a swap in an instance's folders; Close it when done.
func NewSwap(s *Store, id string) *Swap {
	return &Swap{instances: s, id: id, roots: map[string]*os.Root{}, staged: map[string]bool{}}
}

// root opens the folder rel lives in: the game folder, or a shared one.
func (w *Swap) root(rel string) (*os.Root, error) {
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

// Stage puts a new file at name+NewSuffix through place, which writes the
// absolute path it is given.
func (w *Swap) Stage(name string, place func(dst string) error) error {
	r, err := w.root(name)
	if err != nil {
		return err
	}
	if dir := path.Dir(name); dir != "." {
		if err := r.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	staged := name + NewSuffix
	if err := r.Remove(staged); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	w.staged[staged] = true
	return place(w.instances.Path(w.id, staged))
}

// IsStaged reports whether name has a staged new file.
func (w *Swap) IsStaged(name string) bool { return w.staged[name+NewSuffix] }

// Aside sets whatever is at rel aside; nothing there is fine. Set every
// target aside before putting staged files in, so two changes to one name
// never collide.
func (w *Swap) Aside(rel string) error {
	r, err := w.root(rel)
	if err != nil {
		return err
	}
	if _, err := r.Lstat(rel); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return w.rename(r, rel, rel+OldSuffix)
}

// Put moves the staged file in as name.
func (w *Swap) Put(name string) error {
	r, err := w.root(name)
	if err != nil {
		return err
	}
	return w.rename(r, name+NewSuffix, name)
}

// Replace sets whatever is at rel (on or off) aside and, with write, puts
// the staged file in as name.
func (w *Swap) Replace(rel, name string, write bool) error {
	for _, cur := range []string{rel, rel + ".disabled"} {
		if err := w.Aside(cur); err != nil {
			return err
		}
	}
	if !write {
		return nil
	}
	return w.Put(name)
}

func (w *Swap) rename(r *os.Root, from, to string) error {
	if err := r.Rename(from, to); err != nil {
		return err
	}
	w.done = append(w.done, renamed{r, from, to})
	return nil
}

// Rollback undoes every rename and removes the staged files: the folders
// are as they were before the swap.
func (w *Swap) Rollback() {
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

// Finish deletes the old files once the change is recorded.
func (w *Swap) Finish() {
	for _, d := range w.done {
		if strings.HasSuffix(d.to, OldSuffix) {
			_ = d.root.RemoveAll(d.to)
		}
	}
	w.done = nil
}

// Close releases the folders the swap opened.
func (w *Swap) Close() {
	for _, r := range w.roots {
		_ = r.Close()
	}
}
