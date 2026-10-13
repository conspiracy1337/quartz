//go:build linux

package linkx

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// clonePath clones src into a new file at dst with FICLONE (btrfs, XFS,
// bcachefs). dst must not exist.
func clonePath(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src) //nolint:gosec // callers pass paths they validated
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm) //nolint:gosec // a fresh temp name
	if err != nil {
		return err
	}
	err = unix.IoctlFileClone(int(out.Fd()), int(in.Fd())) //nolint:gosec // file descriptors fit an int
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(dst)
		return errors.Join(ErrUnsupported, err)
	}
	return nil
}

func cloneHandle(dst, src *os.File, _ int64) error {
	if err := unix.IoctlFileClone(int(dst.Fd()), int(src.Fd())); err != nil { //nolint:gosec // file descriptors fit an int
		return errors.Join(ErrUnsupported, err)
	}
	return nil
}

// cloneDir: only APFS clones a folder in one call.
func cloneDir(_, _ string) error { return ErrUnsupported }
