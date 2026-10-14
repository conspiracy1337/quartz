package exporting

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // fixture checksums
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"quartz/internal/content"
	"quartz/internal/instance"
	"quartz/internal/modpack"
)

type manifest []content.ManifestItem

type env struct {
	t      *testing.T
	svc    *Service
	in     instance.Instance
	game   string
	shared string
	items  manifest
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	st, err := instance.Open(filepath.Join(root, "instances"), nil)
	if err != nil {
		t.Fatal(err)
	}
	in, err := st.Create(instance.CreateRequest{Name: "Survival", Minecraft: "1.21.1", Loader: "fabric", LoaderVersion: "0.16.9"})
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, in: in, game: st.GameDir(in.ID), shared: filepath.Join(root, "shared")}
	st.SetLocator(func(_, rel string) string {
		if rel == "shaderpacks" || strings.HasPrefix(rel, "shaderpacks/") {
			return e.shared
		}
		return ""
	})
	e.svc = New(Deps{Instances: st, Content: &e.items, Shared: func(string) []string { return []string{"shaderpacks"} }})
	return e
}

func (e *env) write(dir, rel string, body []byte) {
	e.t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, body, 0o600); err != nil {
		e.t.Fatal(err)
	}
}

func sums(b []byte) (string, string) {
	s1 := sha1.Sum(b) //nolint:gosec // fixture checksums
	s5 := sha512.Sum512(b)
	return hex.EncodeToString(s1[:]), hex.EncodeToString(s5[:])
}

func (m *manifest) Manifest(context.Context, string) ([]content.ManifestItem, error) { return *m, nil }
func (m *manifest) Identify(context.Context, string, content.Kind) (int, error)       { return 0, nil }

func names(t *testing.T, file string) []string {
	t.Helper()
	zr, err := zip.OpenReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	var out []string
	for _, f := range zr.File {
		out = append(out, f.Name)
	}
	return out
}

func TestMrpackRoundTrip(t *testing.T) {
	e := newEnv(t)
	sodium, own := []byte("sodium jar"), []byte("a jar from nowhere")
	s1, s5 := sums(sodium)
	o1, o5 := sums(own)
	e.write(e.game, "mods/sodium.jar.disabled", sodium)
	e.write(e.game, "mods/own.jar", own)
	e.write(e.game, "mods/.index/sodium.pw.toml", []byte("x"))
	e.write(e.game, "config/sodium.json", []byte("{}"))
	e.write(e.game, "options.txt", []byte("fov:0.5"))
	e.write(e.game, "saves/World/level.dat", []byte("nbt"))
	e.write(e.game, "logs/latest.log", []byte("token"))
	e.write(e.game, "usercache.json", []byte("[]"))
	e.write(e.game, "session.lock", nil)
	e.write(e.shared, "shaderpacks/Complementary.zip", []byte("shader"))
	e.items = manifest{
		{Kind: content.Mods, Rel: "mods/sodium.jar.disabled", SHA1: s1, SHA512: s5, Size: int64(len(sodium)), URL: "https://cdn.modrinth.com/data/AANobbMI/versions/x/sodium.jar"},
		{Kind: content.Mods, Rel: "mods/own.jar", Enabled: true, SHA1: o1, SHA512: o5, Size: int64(len(own)), URL: "https://edge.forgecdn.net/files/1/2/own.jar"},
	}
	dest := filepath.Join(t.TempDir(), "Survival.mrpack")
	p := Plan{Format: Mrpack, Include: Include{Options: true, Config: true}}
	pv, err := e.svc.Preview(context.Background(), e.in.ID, p)
	if err != nil || pv.Linked != 1 || pv.Bundled != 4 {
		t.Fatalf("preview %+v %v", pv, err)
	}
	if err := e.svc.Write(context.Background(), e.in.ID, p, dest); err != nil {
		t.Fatal(err)
	}
	got := names(t, dest)
	for _, want := range []string{"modrinth.index.json", "overrides/mods/own.jar", "overrides/config/sodium.json", "overrides/options.txt", "overrides/shaderpacks/Complementary.zip"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	for _, name := range got {
		for _, bad := range []string{"logs/", "saves/", "usercache", "session.lock", "/.index/", "sodium.jar"} {
			if strings.Contains(name, bad) {
				t.Errorf("%s must not be exported", name)
			}
		}
	}
	pack, err := modpack.NewReader(modpack.Deps{}).File(context.Background(), dest)
	if err != nil {
		t.Fatal(err)
	}
	if pack.Minecraft != "1.21.1" || pack.Loader != "fabric" || pack.LoaderVersion != "0.16.9" || len(pack.Files) != 1 || pack.Files[0].Path != "mods/sodium.jar.disabled" {
		t.Fatalf("read back %+v", pack)
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Fatal("the partial file stayed")
	}
}

func TestFullZipBundlesEverything(t *testing.T) {
	e := newEnv(t)
	jar := []byte("sodium")
	s1, s5 := sums(jar)
	e.write(e.game, "mods/sodium.jar", jar)
	e.write(e.game, "saves/World/level.dat", []byte("nbt"))
	e.items = manifest{{Kind: content.Mods, Rel: "mods/sodium.jar", Enabled: true, SHA1: s1, SHA512: s5, Size: 6, URL: "https://cdn.modrinth.com/a/sodium.jar"}}
	dest := filepath.Join(t.TempDir(), "Survival.zip")
	if err := e.svc.Write(context.Background(), e.in.ID, Plan{Format: Zip, Include: Include{Worlds: true}}, dest); err != nil {
		t.Fatal(err)
	}
	got := names(t, dest)
	if !slices.Contains(got, "overrides/mods/sodium.jar") || !slices.Contains(got, "overrides/saves/World/level.dat") {
		t.Fatalf("zip %v", got)
	}
}

func TestDestinationAndLinks(t *testing.T) {
	e := newEnv(t)
	e.write(e.game, "mods/a.jar", []byte("a"))
	if err := os.Symlink("/etc/passwd", filepath.Join(e.game, "mods", "passwd.jar")); err != nil {
		t.Fatal(err)
	}
	for _, dest := range []string{filepath.Join(e.game, "out.mrpack"), filepath.Join(e.shared, "shaderpacks", "x.mrpack"), "relative.mrpack"} {
		if err := e.svc.Write(context.Background(), e.in.ID, Plan{Format: Zip}, dest); !errors.Is(err, ErrDestination) {
			t.Errorf("%s: %v", dest, err)
		}
	}
	dest := filepath.Join(t.TempDir(), "x.zip")
	if err := e.svc.Write(context.Background(), e.in.ID, Plan{Format: Zip}, dest); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(names(t, dest), "overrides/mods/passwd.jar") {
		t.Fatal("followed a link")
	}
	if err := e.svc.Write(context.Background(), e.in.ID, Plan{Format: "tar"}, dest); !errors.Is(err, ErrPlan) {
		t.Fatal(err)
	}
}

func TestModListEscapes(t *testing.T) {
	e := newEnv(t)
	e.items = manifest{
		{Kind: content.Mods, Rel: "mods/b.jar", Enabled: true, Name: "=HYPERLINK(\"http://x\")", Version: "-1", Authors: []string{"@me"}},
		{Kind: content.Mods, Rel: "mods/a.jar", Enabled: true, Name: "Sodium | <b>fast</b>", Version: "0.6", Page: "https://modrinth.com/project/sodium", Authors: []string{"Caffeine_MC"}},
		{Kind: content.Mods, Rel: "mods/c.jar.disabled", Name: "Off"},
		{Kind: content.ResourcePacks, Rel: "resourcepacks/p.zip", Enabled: true, Name: "Pack"},
		{Kind: content.Mods, Rel: "mods/d.jar", Enabled: true, Name: "Bad link", Page: "javascript:alert(1)"},
	}
	dir := t.TempDir()
	if err := e.svc.Write(context.Background(), e.in.ID, Plan{Format: List, List: CSV}, filepath.Join(dir, "mods.csv")); err != nil {
		t.Fatal(err)
	}
	csv, _ := os.ReadFile(filepath.Join(dir, "mods.csv"))
	want := "Name,Version,Link,Authors\n\"'=HYPERLINK(\"\"http://x\"\")\",'-1,,'@me\n"
	if !strings.HasPrefix(string(csv), want) || strings.Contains(string(csv), "Off") || strings.Contains(string(csv), "Pack") || strings.Contains(string(csv), "javascript") {
		t.Fatalf("csv:\n%s", csv)
	}
	if err := e.svc.Write(context.Background(), e.in.ID, Plan{Format: List, List: Markdown}, filepath.Join(dir, "mods.md")); err != nil {
		t.Fatal(err)
	}
	md, _ := os.ReadFile(filepath.Join(dir, "mods.md"))
	if !bytes.Contains(md, []byte(`| [Sodium \| &lt;b&gt;fast&lt;/b&gt;](https://modrinth.com/project/sodium) | 0.6 | Caffeine\_MC |`)) {
		t.Fatalf("md:\n%s", md)
	}
	e.items = nil
	if err := e.svc.Write(context.Background(), e.in.ID, Plan{Format: List, List: CSV}, filepath.Join(dir, "none.csv")); !errors.Is(err, ErrNoMods) {
		t.Fatal(err)
	}
}

func TestMrpackMetaSidesAndLinkedFolder(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.d.Instances.SetNotes(e.in.ID, "my private notes"); err != nil {
		t.Fatal(err)
	}
	// mods/ is a link to another launcher's folder (Prism's copy with links).
	other := filepath.Join(t.TempDir(), "elsewhere")
	sodium, menu := []byte("sodium jar"), []byte("a client mod from nowhere")
	s1, _ := sums(sodium)
	_, m5 := sums(menu)
	m1, _ := sums(menu)
	e.write(other, "sodium.jar", sodium)
	e.write(other, "menu.jar", menu)
	if err := os.Symlink(other, filepath.Join(e.game, "mods")); err != nil {
		t.Fatal(err)
	}
	e.items = manifest{
		// The store never recorded sodium's sha512: it is hashed for the index.
		{Kind: content.Mods, Rel: "mods/sodium.jar", Enabled: true, SHA1: s1, Size: int64(len(sodium)), URL: "https://cdn.modrinth.com/data/AANobbMI/versions/x/sodium.jar", ClientOnly: true},
		{Kind: content.Mods, Rel: "mods/menu.jar", Enabled: true, SHA1: m1, SHA512: m5, Size: int64(len(menu)), ClientOnly: true},
	}
	dest := filepath.Join(t.TempDir(), "x.mrpack")
	p := Plan{Format: Mrpack, Meta: instance.ExportMeta{Name: " My Pack ", Version: "2.1", Summary: "For friends"}}
	if err := e.svc.Write(context.Background(), e.in.ID, p, dest); err != nil {
		t.Fatal(err)
	}
	if got := names(t, dest); !slices.Contains(got, "client-overrides/mods/menu.jar") {
		t.Fatalf("zip %v", got)
	}
	zr, _ := zip.OpenReader(dest)
	defer func() { _ = zr.Close() }()
	f, _ := zr.Open("modrinth.index.json")
	var b bytes.Buffer
	_, _ = b.ReadFrom(f)
	index := b.String()
	for _, want := range []string{`"name": "My Pack"`, `"versionId": "2.1"`, `"summary": "For friends"`, `"server": "unsupported"`, `mods/sodium.jar`} {
		if !strings.Contains(index, want) {
			t.Errorf("index lacks %s:\n%s", want, index)
		}
	}
	if strings.Contains(index, "private") {
		t.Fatal("the instance's notes were exported")
	}
}
