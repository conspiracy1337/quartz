//go:build !windows

package fsx

import (
	"os"

	"golang.org/x/sys/unix"
)

// Locked reports whether another process holds a lock on the file at path.
// Minecraft locks each open world's session.lock with Java's
// FileChannel.tryLock, which on Unix is a POSIX record lock.
func Locked(path string) bool {
	f, err := os.Open(path) //nolint:gosec // a file the caller chose to check
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	lk := unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: 0, Len: 0}
	if err := unix.FcntlFlock(f.Fd(), unix.F_GETLK, &lk); err != nil {
		return false
	}
	return lk.Type != unix.F_UNLCK
}
