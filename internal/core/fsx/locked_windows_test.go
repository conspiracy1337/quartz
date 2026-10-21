package fsx

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

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

// A folder a running program holds a file in (a game's loaded natives) can't
// be moved on Windows: ReplaceDir says it is in use and leaves it whole,
// rather than deleting what it can and failing halfway.
func TestReplaceDirInUse(t *testing.T) {
	dir := t.TempDir()
	dst, tmp := filepath.Join(dir, "natives"), filepath.Join(dir, "natives.tmp")
	for _, p := range []string{dst, tmp} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{filepath.Join(dst, ".complete"), filepath.Join(dst, "lwjgl.dll")}
	for _, f := range files {
		if err := os.WriteFile(f, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	held, err := os.Open(files[1]) // opened without FILE_SHARE_DELETE, as a loaded DLL is
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	if err := ReplaceDir(tmp, dst); !errors.Is(err, ErrInUse) {
		t.Fatalf("ReplaceDir = %v, want ErrInUse", err)
	}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("%s is gone: %v", filepath.Base(f), err)
		}
	}
	if _, err := os.Stat(tmp); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the new folder was left behind (%v)", err)
	}
}
