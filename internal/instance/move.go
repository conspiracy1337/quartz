package instance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"quartz/internal/core/fsx"
	"quartz/internal/core/linkx"
)

// Errors a move reports to the UI.
var (
	ErrMoveTarget = errors.New("instance: pick an empty folder outside the instance")
	ErrMoveSame   = errors.New("instance: the game folder is already there")
)

// MovePlan is a checked move of one instance's game folder.
type MovePlan struct {
	ID       string
	From, To string
	Default  bool // moving back to instances/<id>/game
}

// PlanMove checks a move of id's game folder into parent (a folder the user
// picked; the game folder becomes parent/<id>). parent "" moves it back to the
// default place. The target must not exist or be empty.
func (s *Store) PlanMove(id, parent string) (MovePlan, error) {
	if _, err := s.Get(id); err != nil {
		return MovePlan{}, err
	}
	p := MovePlan{ID: id, From: s.GameDir(id)}
	if parent == "" {
		p.To, p.Default = s.defaultGameDir(id), true
	} else {
		if !filepath.IsAbs(parent) {
			return MovePlan{}, ErrMoveTarget
		}
		if info, err := os.Stat(parent); err != nil || !info.IsDir() {
			return MovePlan{}, ErrMoveTarget
		}
		p.To = filepath.Join(filepath.Clean(parent), id)
	}
	if filepath.Clean(p.To) == filepath.Clean(p.From) {
		return MovePlan{}, ErrMoveSame
	}
	if fsx.Inside(p.From, p.To) {
		return MovePlan{}, ErrMoveTarget // a folder can't move into itself
	}
	if entries, err := os.ReadDir(p.To); err == nil && len(entries) > 0 {
		return MovePlan{}, ErrMoveTarget
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return MovePlan{}, err
	}
	return p, nil
}

// Move carries out a plan: a rename when both places share a volume, else a
// copy that is checked (every file, same size) before the instance switches
// to it and the old folder is deleted. The caller makes sure the instance
// isn't running. progress, if set, gets bytes copied so far and the total.
func (s *Store) Move(ctx context.Context, p MovePlan, progress func(done, total int64)) (Instance, error) {
	if err := os.MkdirAll(filepath.Dir(p.To), 0o700); err != nil {
		return Instance{}, err
	}
	_ = os.Remove(p.To) // an empty target folder is fine
	renamed := os.Rename(p.From, p.To) == nil
	if !renamed {
		if err := copyTree(ctx, p.From, p.To, copyOpts{progress: progress}); err != nil {
			_ = os.RemoveAll(p.To)
			return Instance{}, err
		}
	}
	loc := p.To
	if p.Default {
		loc = ""
		_ = os.Remove(filepath.Join(p.To, markerName))
	} else {
		writeMarker(p.To, p.ID)
	}
	in, err := s.Update(p.ID, func(in *Instance) error { in.Location = loc; return nil })
	if err != nil {
		// The instance still points at the old place: put it back as it was.
		if renamed {
			_ = os.Rename(p.To, p.From)
			if p.Default {
				writeMarker(p.From, p.ID)
			} else {
				_ = os.Remove(filepath.Join(p.From, markerName))
			}
		} else {
			_ = os.RemoveAll(p.To)
		}
		return Instance{}, err
	}
	if !renamed {
		_ = os.RemoveAll(p.From) // only once the instance points at the copy
	}
	return in, nil
}

// copyOpts tune copyTree.
type copyOpts struct {
	// skip leaves a file or folder (and all in it) out.
	skip func(rel string) bool
	// place may put a file in place another way (a link to its stored copy);
	// it reports false to have it copied.
	place    func(rel, dst string) (bool, error)
	progress func(done, total int64)
}

// copyTree copies src into dst, keeping links as links (one pointing into src
// now points to the same place in dst), and checks that dst ends up with
// every file at the same size. Both folders are opened as roots,
// so a link swapped in during the copy can't lead it outside either one.
func copyTree(ctx context.Context, src, dst string, o copyOpts) error {
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	from, err := os.OpenRoot(src)
	if err != nil {
		return err
	}
	defer func() { _ = from.Close() }()
	to, err := os.OpenRoot(dst)
	if err != nil {
		return err
	}
	defer func() { _ = to.Close() }()
	skipped := func(rel string, d fs.DirEntry) error {
		if rel == "." || o.skip == nil || !o.skip(rel) {
			return nil
		}
		if d.IsDir() {
			return fs.SkipDir
		}
		return errSkip
	}

	var total, done int64
	var files, copied int
	// Counting only: an unreadable entry fails the copy below instead.
	_ = fs.WalkDir(from.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err == nil {
			if serr := skipped(rel, d); serr != nil {
				return ignoreSkip(serr)
			}
			if info, ierr := d.Info(); ierr == nil && d.Type().IsRegular() {
				total += info.Size()
				files++
			}
		}
		return nil
	})
	err = fs.WalkDir(from.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := skipped(rel, d); err != nil {
			return ignoreSkip(err)
		}
		switch {
		case rel == ".":
			return nil
		case d.IsDir():
			return to.Mkdir(rel, 0o700)
		case d.Type()&fs.ModeSymlink != 0:
			link, err := from.Readlink(rel)
			if err != nil {
				return err
			}
			// A link into the old folder would dangle once it is deleted.
			if r, err := filepath.Rel(src, link); err == nil && filepath.IsAbs(link) && filepath.IsLocal(r) {
				link = filepath.Join(dst, r)
			}
			return to.Symlink(link, rel)
		case d.Type().IsRegular():
			n, err := placeOrCopy(from, to, rel, filepath.Join(dst, filepath.FromSlash(rel)), o.place)
			done += n
			copied++
			if o.progress != nil {
				o.progress(done, total)
			}
			return err
		}
		return nil // sockets and devices don't belong in a game folder
	})
	if err != nil {
		return err
	}
	if copied != files || done != total {
		return fmt.Errorf("instance: the copy is incomplete (%d of %d files)", copied, files)
	}
	return nil
}

// errSkip leaves one file out of a walk.
var errSkip = errors.New("skip")

func ignoreSkip(err error) error {
	if errors.Is(err, errSkip) {
		return nil
	}
	return err
}

// placeOrCopy lets place put a file in place, else copies it; it returns the
// size now at dst.
func placeOrCopy(from, to *os.Root, rel, dst string, place func(rel, dst string) (bool, error)) (int64, error) {
	if place != nil {
		ok, err := place(rel, dst)
		if err != nil {
			return 0, err
		}
		if ok {
			info, err := to.Lstat(rel)
			if err != nil {
				return 0, err
			}
			want, err := from.Lstat(rel)
			if err == nil && want.Size() != info.Size() {
				return info.Size(), io.ErrShortWrite
			}
			return info.Size(), err
		}
	}
	return copyFile(from, to, rel)
}

func copyFile(from, to *os.Root, rel string) (int64, error) {
	in, err := from.Open(rel)
	if err != nil {
		return 0, err
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return 0, err
	}
	out, err := to.OpenFile(rel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm()|0o600)
	if err != nil {
		return 0, err
	}
	n, err := linkx.CopyFile(out, in)
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n != info.Size() {
		err = io.ErrShortWrite
	}
	return n, err
}
