package fsx

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSafeJoin(t *testing.T) {
	root := filepath.FromSlash("/data/root")
	ok := []string{"a", "a/b.txt", "a/../b", "./c"}
	bad := []string{"", "..", "../x", "a/../../x", "/etc/passwd", "a/../../../etc"}
	if runtime.GOOS == "windows" {
		bad = append(bad, `C:\x`, `\\server\share`, "NUL", `a\..\..\x`)
	}
	for _, rel := range ok {
		got, err := SafeJoin(root, rel)
		if err != nil {
			t.Errorf("SafeJoin(%q) unexpected error: %v", rel, err)
			continue
		}
		if !strings.HasPrefix(got, root+string(filepath.Separator)) {
			t.Errorf("SafeJoin(%q) = %q escapes root", rel, got)
		}
	}
	for _, rel := range bad {
		if _, err := SafeJoin(root, rel); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("SafeJoin(%q) = nil error, want ErrUnsafePath", rel)
		}
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.json")
	for _, content := range []string{"first", "second"} {
		if err := AtomicWrite(p, []byte(content), FilePerm); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(p)
		if err != nil || string(got) != content {
			t.Fatalf("got %q, %v; want %q", got, err, content)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(p)
		if info.Mode().Perm() != FilePerm {
			t.Fatalf("perm = %v, want %v", info.Mode().Perm(), os.FileMode(FilePerm))
		}
	}
}

func TestReadCapped(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := os.WriteFile(p, []byte("12345"), FilePerm); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadCapped(p, 5); err != nil || string(got) != "12345" {
		t.Fatalf("at limit: %q, %v", got, err)
	}
	if _, err := ReadCapped(p, 4); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("over limit: want ErrTooLarge, got %v", err)
	}
	if _, err := ReadCapped(dir, 100); err == nil {
		t.Fatal("directory: want error")
	}
}
