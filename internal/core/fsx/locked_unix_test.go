//go:build !windows

package fsx

import (
	"os"

	"golang.org/x/sys/unix"
)

// holdLock locks the whole file as Java's FileChannel.tryLock does.
func holdLock(p string) (func(), error) {
	f, err := os.OpenFile(p, os.O_RDWR, 0) //nolint:gosec // a test file
	if err != nil {
		return nil, err
	}
	lk := unix.Flock_t{Type: unix.F_WRLCK}
	if err := unix.FcntlFlock(f.Fd(), unix.F_SETLK, &lk); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}
