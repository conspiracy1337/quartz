package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"quartz/internal/core/fsx"
)

type entry struct {
	name, body, link string
	typ              byte // tar type; 0 = regular
	mode             int64
}

func writeTarGz(t *testing.T, entries []entry) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: 0o644, Typeflag: tar.TypeReg, Size: int64(len(e.body))}
		if e.mode != 0 {
			h.Mode = e.mode
		}
		if e.typ != 0 {
			h.Typeflag = e.typ
			h.Size = 0
			h.Linkname = e.link
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "a.tar.gz")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "a.zip")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTarStripExecAndLinks(t *testing.T) {
	src := writeTarGz(t, []entry{
		{name: "jdk-21/", typ: tar.TypeDir},
		{name: "jdk-21/bin/java", body: "ELF", mode: 0o755},
		{name: "jdk-21/lib/a.so", body: "lib"},
		{name: "jdk-21/lib/b.so", typ: tar.TypeSymlink, link: "a.so"},
		{name: "jdk-21/lib/c.so", typ: tar.TypeLink, link: "jdk-21/lib/a.so"},
	})
	dest := t.TempDir()
	if err := ExtractTarGz(src, dest, Options{Strip: 1}); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dest, "lib", "b.so")) != "lib" || read(t, filepath.Join(dest, "lib", "c.so")) != "lib" {
		t.Fatal("links not resolved")
	}
	info, err := os.Stat(filepath.Join(dest, "bin", "java"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("exec bit lost: %v", info.Mode())
	}
}

func TestTarRejectsHostileEntries(t *testing.T) {
	cases := map[string][]entry{
		"traversal":    {{name: "../evil", body: "x"}},
		"absolute":     {{name: "/etc/evil", body: "x"}},
		"symlink out":  {{name: "l", typ: tar.TypeSymlink, link: "../../etc/passwd"}},
		"symlink abs":  {{name: "l", typ: tar.TypeSymlink, link: "/etc/passwd"}},
		"hardlink out": {{name: "l", typ: tar.TypeLink, link: "../outside"}},
		"device":       {{name: "dev", typ: tar.TypeChar}},
		"symlink chain": {
			{name: "d", typ: tar.TypeSymlink, link: "."},
			{name: "d/l", typ: tar.TypeSymlink, link: "../x"},
		},
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			dest := filepath.Join(parent, "dest")
			if err := os.Mkdir(dest, 0o700); err != nil {
				t.Fatal(err)
			}
			err := ExtractTarGz(writeTarGz(t, entries), dest, Options{})
			if err == nil {
				t.Fatal("hostile archive accepted")
			}
			if !errors.Is(err, fsx.ErrUnsafePath) && !errors.Is(err, ErrBadLink) && !errors.Is(err, ErrBadEntry) {
				t.Fatalf("unexpected error %v", err)
			}
			if _, err := os.Lstat(filepath.Join(parent, "x")); err == nil {
				t.Fatal("something was written outside dest")
			}
		})
	}
}

func TestSizeCaps(t *testing.T) {
	src := writeTarGz(t, []entry{{name: "big", body: strings.Repeat("a", 2048)}})
	if err := ExtractTarGz(src, t.TempDir(), Options{MaxFileSize: 1024}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("per-file cap: %v", err)
	}
	src = writeTarGz(t, []entry{{name: "a", body: strings.Repeat("a", 600)}, {name: "b", body: strings.Repeat("b", 600)}})
	if err := ExtractTarGz(src, t.TempDir(), Options{MaxTotalSize: 1000}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("total cap: %v", err)
	}
	src = writeTarGz(t, []entry{{name: "a"}, {name: "b"}, {name: "c"}})
	if err := ExtractTarGz(src, t.TempDir(), Options{MaxFiles: 2}); !errors.Is(err, ErrTooMany) {
		t.Fatalf("file count cap: %v", err)
	}
}

func TestZipIncludeAndTraversal(t *testing.T) {
	src := writeZip(t, map[string]string{"liblwjgl.so": "so", "META-INF/MANIFEST.MF": "m"})
	dest := t.TempDir()
	err := ExtractZip(src, dest, Options{Include: func(n string) bool { return !strings.HasPrefix(n, "META-INF/") }})
	if err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dest, "liblwjgl.so")) != "so" {
		t.Fatal("file missing")
	}
	if _, err := os.Stat(filepath.Join(dest, "META-INF")); err == nil {
		t.Fatal("excluded entry extracted")
	}
	if err := ExtractZip(writeZip(t, map[string]string{"../../zip-slip": "x"}), t.TempDir(), Options{}); !errors.Is(err, fsx.ErrUnsafePath) {
		t.Fatalf("zip slip: %v", err)
	}
}
