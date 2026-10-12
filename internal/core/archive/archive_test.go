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

func TestZipPrefixAndReadFile(t *testing.T) {
	src := writeZip(t, map[string]string{
		"maven/net/minecraftforge/forge/1/forge-1.jar": "jar",
		"maven/../../escape":                           "x",
		"data/client.lzma":                             "lzma",
		"install_profile.json":                         `{"minecraft":"1.20.1"}`,
	})
	dest := t.TempDir()
	// Traversal inside the prefix is still refused.
	if err := ExtractZip(src, dest, Options{Prefix: "maven/"}); !errors.Is(err, fsx.ErrUnsafePath) {
		t.Fatalf("traversal under prefix: %v", err)
	}
	src = writeZip(t, map[string]string{"maven/a/b/c.jar": "jar", "data/client.lzma": "lzma"})
	if err := ExtractZip(src, dest, Options{Prefix: "maven"}); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dest, "a", "b", "c.jar")) != "jar" {
		t.Fatal("prefixed entry missing")
	}
	if _, err := os.Stat(filepath.Join(dest, "data")); err == nil {
		t.Fatal("entry outside the prefix extracted")
	}

	data, err := ReadFile(src, "data/client.lzma", 16)
	if err != nil || string(data) != "lzma" {
		t.Fatalf("ReadFile = %q, %v", data, err)
	}
	if _, err := ReadFile(src, "data/client.lzma", 2); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("over the limit: %v", err)
	}
	if _, err := ReadFile(src, "missing.json", 16); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing entry: %v", err)
	}
}

func TestReaderOneOpenManyEntries(t *testing.T) {
	src := writeZip(t, map[string]string{"fabric.mod.json": "{}", "assets/icon.png": "png", "META-INF/jars/inner.jar": "jar"})
	r, err := Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	if b, err := r.Read("fabric.mod.json", 8); err != nil || string(b) != "{}" {
		t.Fatalf("Read = %q, %v", b, err)
	}
	if !r.Has("assets/icon.png") || r.Has("assets/") {
		t.Fatal("Has")
	}
	if got := r.Glob("META-INF/jars/", ".jar"); len(got) != 1 {
		t.Fatalf("Glob = %v", got)
	}
	if !r.HasPrefix("assets/") || r.HasPrefix("shaders/") {
		t.Fatal("HasPrefix")
	}
	if _, err := r.Read("assets/icon.png", 2); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("limit: %v", err)
	}
	if _, err := r.Read("nope", 2); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
}
