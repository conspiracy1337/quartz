package linkx

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// stub replaces the clone and link seams for one test.
func stub(t *testing.T, clone func(string, string, os.FileMode) error, link func(string, string) error) {
	t.Helper()
	oc, ol := cloneFn, linkFn
	t.Cleanup(func() { cloneFn, linkFn = oc, ol })
	if clone != nil {
		cloneFn = clone
	}
	if link != nil {
		linkFn = link
	}
}

func noClone(string, string, os.FileMode) error { return ErrUnsupported }
func noLink(string, string) error            { return &os.LinkError{Op: "link", Err: errors.New("EXDEV")} }

func TestPlaceFallbacks(t *testing.T) {
	cases := map[string]struct {
		clone func(string, string, os.FileMode) error
		link  func(string, string) error
		opts  Options
		want  Method
	}{
		"hardlink when cloning fails": {clone: noClone, want: Hardlink},
		"copy when both fail":         {clone: noClone, link: noLink, want: Copy},
		"copy when a private file is asked for": {clone: noClone, opts: Options{NoHardlink: true}, want: Copy},
	}
	t.Run("no copy refuses", func(t *testing.T) {
		stub(t, noClone, noLink)
		dir := t.TempDir()
		src, dst := filepath.Join(dir, "a"), filepath.Join(dir, "b")
		write(t, src, "x")
		write(t, dst, "keep")
		if _, err := Place(src, dst, Options{NoCopy: true}); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("err = %v", err)
		}
		if read(t, dst) != "keep" {
			t.Fatal("dst changed")
		}
	})
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			stub(t, c.clone, c.link)
			dir := t.TempDir()
			src, dst := filepath.Join(dir, "src.jar"), filepath.Join(dir, "dst.jar")
			write(t, src, "content")
			write(t, dst, "old")
			m, err := Place(src, dst, c.opts)
			if err != nil {
				t.Fatal(err)
			}
			if m != c.want {
				t.Fatalf("method = %v, want %v", m, c.want)
			}
			if got := read(t, dst); got != "content" {
				t.Fatalf("dst = %q", got)
			}
			a, _ := os.Stat(src)
			b, _ := os.Stat(dst)
			if os.SameFile(a, b) != (c.want == Hardlink) {
				t.Fatalf("same file = %v for %v", os.SameFile(a, b), m)
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 2 {
				t.Fatalf("temp files left behind: %v", entries)
			}
		})
	}
}

func TestPlaceRealFileSystem(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "a"), filepath.Join(dir, "sub", "b")
	write(t, src, "x")
	if err := os.Mkdir(filepath.Dir(dst), 0o700); err != nil {
		t.Fatal(err)
	}
	m, err := Place(src, dst, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if m == Copy {
		t.Fatalf("a same-volume place should clone or link, got %v", m)
	}
	if read(t, dst) != "x" {
		t.Fatal("wrong content")
	}
}

func TestPlaceRejectsNonRegular(t *testing.T) {
	dir := t.TempDir()
	if _, err := Place(dir, filepath.Join(dir, "x"), Options{}); err == nil {
		t.Fatal("placing a directory should fail")
	}
}

func TestReplaceReadOnlyTarget(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
	write(t, src, "new")
	write(t, dst, "old")
	if err := SetReadOnly(dst, true); err != nil {
		t.Fatal(err)
	}
	stub(t, noClone, noLink)
	if _, err := Place(src, dst, Options{}); err != nil {
		t.Fatal(err)
	}
	if read(t, dst) != "new" {
		t.Fatal("read-only target was not replaced")
	}
}

func TestSetReadOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	write(t, p, "x")
	if err := SetReadOnly(p, true); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(p); info.Mode().Perm()&0o222 != 0 {
			t.Fatalf("mode = %v", info.Mode())
		}
	}
	if err := SetReadOnly(p, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("y"), 0o600); err != nil {
		t.Fatalf("writable again: %v", err)
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	write(t, src, "stream me")
	in, _ := os.Open(src) //nolint:gosec // test temp dir
	defer func() { _ = in.Close() }()
	out, err := os.Create(filepath.Join(dir, "dst")) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	n, err := CopyFile(out, in)
	_ = out.Close()
	if err != nil || n != 9 || read(t, out.Name()) != "stream me" {
		t.Fatalf("n=%d err=%v", n, err)
	}
}
