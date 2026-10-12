//go:build darwin

package linkx

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// clonePath clones src to dst with APFS clonefile (copy-on-write, instant).
// dst must not exist; the clone keeps src's mode, so perm is applied after.
func clonePath(src, dst string, perm os.FileMode) error {
	if err := unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW|unix.CLONE_NOOWNERCOPY); err != nil {
		return errors.Join(ErrUnsupported, err)
	}
	return os.Chmod(dst, perm)
}

// cloneHandle can't clone into an already open file on macOS.
func cloneHandle(_, _ *os.File, _ int64) error { return ErrUnsupported }
