package packwiz

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Prism's own fixtures (tests/testdata/Packwiz).
const borderless = `name = "Borderless Mining"
filename = "borderless-mining-1.1.1+1.18.jar"
side = "client"

[download]
url = "https://cdn.modrinth.com/data/kYq5qkSL/versions/1.1.1+1.18/borderless-mining-1.1.1+1.18.jar"
hash-format = "sha512"
hash = "c8fe6e15ddea32668822dddb26e1851e5f03834be4bcb2eff9c0da7fdc086a9b6cead78e31a44d3bc66335cba11144ee0337c6d5346f1ba63623064499b3188d"

[update]
[update.modrinth]
mod-id = "kYq5qkSL"
version = "ug2qKTPR"
`

const clipboard = `name = "Screenshot to Clipboard (Fabric)"
filename = "screenshot-to-clipboard-1.0.7-fabric.jar"
side = "both"
x-custom = "kept"

[download]
url = "https://edge.forgecdn.net/files/3509/43/screenshot-to-clipboard-1.0.7-fabric.jar"
hash-format = "murmur2"
hash = "1781245820"

[update]
[update.curseforge]
file-id = 3509043
project-id = 327154
`

func fixture(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name+Ext), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestReadPrismFixtures(t *testing.T) {
	m, err := Read(filepath.Join(fixture(t, "borderless-mining", borderless), "borderless-mining"+Ext))
	if err != nil {
		t.Fatal(err)
	}
	if m.Slug != "borderless-mining" || m.Name != "Borderless Mining" || m.Side != "client" || m.Download.HashFormat != "sha512" ||
		m.Modrinth == nil || *m.Modrinth != (Modrinth{ModID: "kYq5qkSL", Version: "ug2qKTPR"}) || m.CurseForge != nil {
		t.Fatalf("%+v", m)
	}
	m, err = Read(filepath.Join(fixture(t, "stc", clipboard), "stc"+Ext))
	if err != nil {
		t.Fatal(err)
	}
	if m.CurseForge == nil || *m.CurseForge != (CurseForge{ProjectID: 327154, FileID: 3509043}) || m.Download.Hash != "1781245820" {
		t.Fatalf("%+v", m)
	}
}

func TestRoundTripKeepsUnknownKeys(t *testing.T) {
	dir := fixture(t, "stc", clipboard)
	m, err := Read(filepath.Join(dir, "stc"+Ext))
	if err != nil {
		t.Fatal(err)
	}
	m.Pin = true
	m.Modrinth = &Modrinth{ModID: "abc", Version: "def"}
	if err := Write(dir, m); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "stc"+Ext)) //nolint:gosec // test temp dir
	for _, want := range []string{"x-custom = 'kept'", "pin = true", "x-prismlauncher-lock-update = true", "project-id = 327154"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %q in\n%s", want, raw)
		}
	}
	back, _ := Read(filepath.Join(dir, "stc"+Ext))
	if !back.Pin || back.Modrinth == nil || back.CurseForge == nil || back.Name != m.Name {
		t.Fatalf("%+v", back)
	}
	m.Pin = false
	_ = Write(dir, m)
	back, _ = Read(filepath.Join(dir, "stc"+Ext))
	if back.Pin {
		t.Fatal("unpin lost")
	}
}

func TestPrismLockKeyIsRead(t *testing.T) {
	dir := fixture(t, "x", "filename = \"x.jar\"\nx-prismlauncher-lock-update = true\n[download]\n[update]\n")
	m, _ := Read(filepath.Join(dir, "x"+Ext))
	if !m.Pin {
		t.Fatal("Prism's lock key ignored")
	}
}

func TestReadDirByFilenameIgnoringCase(t *testing.T) {
	dir := fixture(t, "stc", clipboard)
	_ = os.WriteFile(filepath.Join(dir, "junk.pw.toml"), []byte("= not toml"), 0o600)
	got, err := ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["screenshot-to-clipboard-1.0.7-fabric.jar"]; !ok || len(got) != 1 {
		t.Fatalf("%v", got)
	}
	if got, err := ReadDir(filepath.Join(dir, "missing")); err != nil || len(got) != 0 {
		t.Fatal("a missing folder is empty, not an error")
	}
}

func TestWriteRejectsBadSlugs(t *testing.T) {
	for _, s := range []string{"", "../x", "a/b", "UPPER", "..", strings.Repeat("a", 70)} {
		if err := Write(t.TempDir(), Meta{Slug: s, Filename: "x.jar"}); !errors.Is(err, ErrBadSlug) {
			t.Errorf("%q: %v", s, err)
		}
	}
}

func TestSlugAndRemove(t *testing.T) {
	cases := map[string]string{"Sodium Extra": "sodium-extra", "My Mod!!": "my-mod", "": "item", "../../etc": "etc", "a..b": "a.b", strings.Repeat("x", 80): strings.Repeat("x", 64)}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
	dir := fixture(t, "Mixed", clipboard)
	if err := Remove(dir, "mixed"); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("left %v", entries)
	}
	if !reflect.DeepEqual(strs(map[string]any{"k": []any{"a", 1, ""}}, "k"), []string{"a"}) {
		t.Fatal("strs")
	}
}
