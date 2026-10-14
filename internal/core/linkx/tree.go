package linkx

import (
	"io/fs"
	"os"
	"path/filepath"

	"quartz/internal/core/fsx"
)

// CloneTree gives dst (which must not exist yet) a private copy of the tree
// under src, as cheaply as the file system allows: one directory clone on
// APFS; elsewhere a clone or copy per file, plus hardlinks for read-only
// files (store-placed content), since nothing can edit those through either
// name. Modification times are kept, so newest-wins comparisons stay true.
// Links and special files are skipped, as in CopyTree.
func CloneTree(src, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		return os.ErrExist
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	if cloneDir(src, dst) == nil {
		return nil
	}
	root, err := os.OpenRoot(src)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		from := filepath.Join(src, filepath.FromSlash(p))
		readOnly := info.Mode().Perm()&0o200 == 0
		if _, err := Place(from, target, Options{NoHardlink: !readOnly}); err != nil {
			return err
		}
		if readOnly {
			return nil // a hardlink shares the time already; a copy keeps it below
		}
		return os.Chtimes(target, info.ModTime(), info.ModTime())
	})
	if err != nil {
		_ = os.RemoveAll(dst)
	}
	return err
}

// UpdateTree brings dst up to date with src one way: every regular file
// under src that dst lacks, or has an older copy of, gets a private copy
// (times kept); files only dst has stay. A link or special file on either
// side is skipped, so nothing is written through one.
func UpdateTree(src, dst string) error {
	root, err := os.OpenRoot(src)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(p))
		cur, statErr := os.Lstat(target)
		if d.IsDir() {
			if statErr == nil && !cur.IsDir() {
				return fs.SkipDir
			}
			return os.MkdirAll(target, 0o700)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if statErr == nil && (!cur.Mode().IsRegular() || !info.ModTime().After(cur.ModTime())) {
			return nil
		}
		if _, err := Place(filepath.Join(src, filepath.FromSlash(p)), target, Options{NoHardlink: true}); err != nil {
			return err
		}
		return os.Chtimes(target, info.ModTime(), info.ModTime())
	})
}

// Mirror copies folders to another volume as they are, for moving the data
// folder: modification times and read-only marks are kept (the verified
// index and content scans trust them), files that were hardlinks of each
// other stay one file (a store file and every instance's link to it), and
// links that stay inside From are made again, pointing at the same place
// under To. Links leading elsewhere and special files are left out. One
// Mirror copies every folder of a move, so hardlinks between them survive.
type Mirror struct {
	From, To string            // the folder being moved and its new place
	copies   map[string]string // the identity of a file with hard links → its first copy
}

// Copy copies src (inside From: a folder, file or link) to dst, which must
// not exist. On failure the partial copy is removed.
func (m *Mirror) Copy(src, dst string) error {
	if m.copies == nil {
		m.copies = map[string]string{}
	}
	if _, err := os.Lstat(dst); err == nil {
		return os.ErrExist
	}
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o700)
		case d.Type()&fs.ModeSymlink != 0:
			return m.link(p, target)
		case d.Type().IsRegular():
			info, err := d.Info()
			if err != nil {
				return err
			}
			return m.file(p, target, info)
		}
		return nil
	})
	if err != nil {
		_ = os.RemoveAll(dst)
	}
	return err
}

func (m *Mirror) file(src, dst string, info fs.FileInfo) error {
	readOnly := info.Mode().Perm()&0o200 == 0
	id, linked := "", false
	if readOnly { // the files the launcher hardlinks are read-only
		id, linked = identity(src, info)
	}
	if first, ok := m.copies[id]; linked && ok && os.Link(first, dst) == nil {
		return nil
	}
	if _, err := Place(src, dst, Options{NoHardlink: true, Perm: info.Mode().Perm() | 0o200}); err != nil {
		return err
	}
	if err := os.Chtimes(dst, info.ModTime(), info.ModTime()); err != nil {
		return err
	}
	if readOnly {
		if err := SetReadOnly(dst, true); err != nil {
			return err
		}
	}
	if linked {
		m.copies[id] = dst
	}
	return nil
}

// link makes the link at src again at dst when it leads inside From; where
// the system refuses links (Windows without the privilege), the file it
// points at is copied instead.
func (m *Mirror) link(src, dst string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return err
	}
	resolved := target
	if !filepath.IsAbs(target) {
		resolved = filepath.Join(filepath.Dir(src), target)
	}
	if !fsx.Within(resolved, m.From) {
		return nil
	}
	if filepath.IsAbs(target) {
		rel, err := filepath.Rel(m.From, resolved)
		if err != nil {
			return err
		}
		target = filepath.Join(m.To, rel)
	}
	if os.Symlink(target, dst) == nil {
		return nil
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return nil //nolint:nilerr // a dangling or folder link is left out, as before
	}
	return m.file(resolved, dst, info)
}

// Size is how much room the files under paths take, each file with hard links
// counted once (as Mirror copies it).
func Size(paths ...string) (int64, error) {
	var total int64
	seen := map[string]bool{}
	for _, root := range paths {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.Type().IsRegular() {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Mode().Perm()&0o200 == 0 {
				if id, ok := identity(p, info); ok {
					if seen[id] {
						return nil
					}
					seen[id] = true
				}
			}
			total += info.Size()
			return nil
		})
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}
