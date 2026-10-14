package fsx

import (
	"math"
	"os"

	"golang.org/x/sys/windows"
)

// holdLock locks the file as Java's FileChannel.tryLock does: 0 to MaxInt64.
func holdLock(p string) (func(), error) {
	f, err := os.OpenFile(p, os.O_RDWR, 0) //nolint:gosec // a test file
	if err != nil {
		return nil, err
	}
	var ol windows.Overlapped
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, math.MaxUint32, math.MaxInt32, &ol); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}
