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

func TestMirror(t *testing.T) {
	stub(t, noClone, nil) // another drive: nothing can be cloned
	from, to := t.TempDir(), filepath.Join(t.TempDir(), "data")
	old := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	blob := filepath.Join(from, "store", "ab", "blob")
	for _, d := range []string{filepath.Dir(blob), filepath.Join(from, "instances", "a", "mods"), filepath.Join(from, "instances", "b", "mods"), filepath.Join(from, "java", "bin")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write(t, blob, "jar")
	_ = os.Chtimes(blob, old, old)
	if err := SetReadOnly(blob, true); err != nil {
		t.Fatal(err)
	}
	for _, in := range []string{"a", "b"} {
		if err := os.Link(blob, filepath.Join(from, "instances", in, "mods", "x.jar")); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(from, "instances", "a", "options.txt"), "opts")
	_ = os.Chtimes(filepath.Join(from, "instances", "a", "options.txt"), old, old)
	write(t, filepath.Join(from, "java", "bin", "java"), "exe")
	outside := filepath.Join(t.TempDir(), "elsewhere")
	write(t, outside, "x")
	links := map[string]string{
		filepath.Join(from, "java", "rel"):     filepath.Join("bin", "java"),
		filepath.Join(from, "java", "abs"):     filepath.Join(from, "java", "bin", "java"),
		filepath.Join(from, "java", "outside"): outside,
	}
	for at, target := range links {
		if err := os.Symlink(target, at); err != nil {
			t.Skip("no symlinks here:", err)
		}
	}

	m := &Mirror{From: from, To: to}
	for _, n := range []string{"store", "instances", "java"} {
		if err := m.Copy(filepath.Join(from, n), filepath.Join(to, n)); err != nil {
			t.Fatal(err)
		}
	}
	newBlob := filepath.Join(to, "store", "ab", "blob")
	info, err := os.Stat(newBlob)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(old) || info.Mode().Perm()&0o200 != 0 {
		t.Errorf("blob: mtime %v mode %v, want the original's", info.ModTime(), info.Mode())
	}
	for _, in := range []string{"a", "b"} {
		li, err := os.Stat(filepath.Join(to, "instances", in, "mods", "x.jar"))
		if err != nil || !os.SameFile(info, li) {
			t.Errorf("instance %s's mod is no longer the store file: %v", in, err)
		}
	}
	if oi, err := os.Stat(filepath.Join(to, "instances", "a", "options.txt")); err != nil || !oi.ModTime().Equal(old) || oi.Mode().Perm()&0o200 == 0 {
		t.Errorf("options.txt: %v %v", oi, err)
	}
	if got, err := os.Readlink(filepath.Join(to, "java", "rel")); err != nil || got != filepath.Join("bin", "java") {
		t.Errorf("relative link = %q, %v", got, err)
	}
	if got, err := os.Readlink(filepath.Join(to, "java", "abs")); err != nil || got != filepath.Join(to, "java", "bin", "java") {
		t.Errorf("absolute link = %q, %v (want it under the new folder)", got, err)
	}
	if _, err := os.Lstat(filepath.Join(to, "java", "outside")); err == nil {
		t.Error("a link leading outside the data folder was copied")
	}

	size, err := Size(filepath.Join(from, "store"), filepath.Join(from, "instances"))
	if err != nil || size != int64(len("jar")+len("opts")) {
		t.Errorf("Size = %d, %v: a file with hard links should count once", size, err)
	}
}
