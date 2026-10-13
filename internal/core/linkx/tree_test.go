package linkx

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCloneTree(t *testing.T) {
	stub(t, noClone, nil)
	src, dst := t.TempDir(), filepath.Join(t.TempDir(), "copy")
	if err := os.MkdirAll(filepath.Join(src, "w", "region"), 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(src, "w", "level.dat"), "lvl")
	old := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	_ = os.Chtimes(filepath.Join(src, "w", "level.dat"), old, old)
	write(t, filepath.Join(src, "mod.jar"), "jar")
	if err := SetReadOnly(filepath.Join(src, "mod.jar"), true); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink("/etc", filepath.Join(src, "escape"))

	if err := CloneTree(src, dst); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dst, "w", "level.dat")) != "lvl" {
		t.Fatal("content")
	}
	if info, _ := os.Stat(filepath.Join(dst, "w", "level.dat")); !info.ModTime().Equal(old) {
		t.Fatalf("mtime %v", info.ModTime())
	}
	if _, err := os.Stat(filepath.Join(dst, "w", "region")); err != nil {
		t.Fatal("empty folder dropped")
	}
	if _, err := os.Lstat(filepath.Join(dst, "escape")); !os.IsNotExist(err) {
		t.Fatal("a link was copied")
	}
	// The writable file is private; the read-only one may be shared.
	write(t, filepath.Join(dst, "w", "level.dat"), "changed")
	if read(t, filepath.Join(src, "w", "level.dat")) != "lvl" {
		t.Fatal("the clone shares writes")
	}
	a, _ := os.Stat(filepath.Join(src, "mod.jar"))
	b, _ := os.Stat(filepath.Join(dst, "mod.jar"))
	if !os.SameFile(a, b) {
		t.Fatal("read-only file wasn't hardlinked")
	}
	if err := CloneTree(src, dst); !os.IsExist(err) {
		t.Fatalf("existing dst: %v", err)
	}
}
