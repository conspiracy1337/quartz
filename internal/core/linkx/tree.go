package linkx

import (
	"io/fs"
	"os"
	"path/filepath"
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
