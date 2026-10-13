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
		t.Fatal("the read-only file got its own copy instead of a hard link")
	}
	if err := CloneTree(src, dst); !os.IsExist(err) {
		t.Fatalf("existing dst: %v", err)
	}
}

func TestUpdateTree(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	old := time.Now().Add(-time.Hour)
	write := func(p, body string, at time.Time) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(src, "a.toml"), "new", time.Now())
	write(filepath.Join(dst, "a.toml"), "old", old)
	write(filepath.Join(src, "sub", "b.json"), "src", old)
	write(filepath.Join(dst, "sub", "b.json"), "server edit", time.Now())
	write(filepath.Join(src, "c.txt"), "added", old)
	write(filepath.Join(dst, "server-only.txt"), "kept", old)
	if err := UpdateTree(src, dst); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"a.toml": "new", "sub/b.json": "server edit", "c.txt": "added", "server-only.txt": "kept"} {
		if got, _ := os.ReadFile(filepath.Join(dst, filepath.FromSlash(name))); string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}
