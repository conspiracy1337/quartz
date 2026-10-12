// Package linkx puts one file's content at another path the cheapest way the
// file system allows: a copy-on-write clone (reflink), else a hardlink, else a
// real copy. It never creates symlinks: Windows needs privileges for them and
// Minecraft refuses worlds that contain them.
package linkx

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// Method says how a file was placed.
type Method uint8

const (
	Copy Method = iota
	Hardlink
	Reflink
)

func (m Method) String() string {
	switch m {
	case Reflink:
		return "reflink"
	case Hardlink:
		return "hardlink"
	}
	return "copy"
}

// ErrUnsupported means the file system can't clone this file.
var ErrUnsupported = errors.New("linkx: cloning is not supported here")

// Options tune Place.
type Options struct {
	// NoHardlink asks for a private file: a hardlink shares edits and the
	// read-only state with its source, a clone or copy doesn't.
	NoHardlink bool
	// Perm is the mode of a clone or copy; zero keeps the source's owner bits
	// plus read-write for the owner.
	Perm os.FileMode
}

// Seams for tests: they make each fallback reachable on any file system.
var (
	cloneFn = clonePath
	linkFn  = os.Link
)

// Place puts src's content at dst, replacing dst atomically. It tries a clone,
// then a hardlink (unless NoHardlink), then a copy, and reports which one won.
func Place(src, dst string, o Options) (Method, error) {
	info, err := os.Stat(src)
	if err != nil {
		return Copy, err
	}
	if !info.Mode().IsRegular() {
		return Copy, &os.PathError{Op: "place", Path: src, Err: errors.New("not a regular file")}
	}
	perm := o.Perm
	if perm == 0 {
		perm = info.Mode().Perm() | 0o600
	}
	tmp, err := tempName(dst)
	if err != nil {
		return Copy, err
	}
	m, err := place(src, tmp, perm, o.NoHardlink)
	if err != nil {
		_ = os.Remove(tmp)
		return m, err
	}
	if err := Replace(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return m, err
	}
	return m, nil
}

func place(src, tmp string, perm os.FileMode, noHardlink bool) (Method, error) {
	if err := cloneFn(src, tmp, perm); err == nil {
		return Reflink, nil
	}
	_ = os.Remove(tmp)
	if !noHardlink {
		if err := linkFn(src, tmp); err == nil {
			return Hardlink, nil
		}
	}
	return Copy, copyPath(src, tmp, perm)
}

// CopyFile copies src's content into dst (both open, dst empty), cloning the
// data when the file system can and streaming it otherwise. On Linux the
// stream uses copy_file_range, which still shares blocks on btrfs and XFS.
func CopyFile(dst, src *os.File) (int64, error) {
	info, err := src.Stat()
	if err != nil {
		return 0, err
	}
	if cloneHandle(dst, src, info.Size()) == nil {
		return info.Size(), nil
	}
	if _, err := dst.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	if err := dst.Truncate(0); err != nil {
		return 0, err
	}
	return io.Copy(dst, src)
}

// Replace renames tmp over dst. On Windows a read-only dst is made writable
// first, since a rename onto it is refused.
func Replace(tmp, dst string) error {
	if err := os.Rename(tmp, dst); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrPermission) {
		return err
	}
	if err := SetReadOnly(dst, false); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func copyPath(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src) //nolint:gosec // callers pass paths they validated
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm) //nolint:gosec // a fresh temp name next to dst
	if err != nil {
		return err
	}
	_, err = CopyFile(out, in)
	if err == nil {
		err = out.Sync()
	}
	return errors.Join(err, out.Close())
}

// tempName is a hidden, unused name in dst's folder, so the final rename never
// crosses a file system.
func tempName(dst string) (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(dst), "."+filepath.Base(dst)+"."+hex.EncodeToString(b[:])+".tmp"), nil
}
