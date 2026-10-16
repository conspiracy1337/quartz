package exporting

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // fixture checksums
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
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
func (m *manifest) Identify(context.Context, string, content.Kind) (int, error)      { return 0, nil }

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
	sodium, own, zoom := []byte("sodium jar"), []byte("a jar from nowhere"), []byte("zoom jar")
	s1, s5 := sums(sodium)
	o1, o5 := sums(own)
	z1, z5 := sums(zoom)
	e.write(e.game, "mods/sodium.jar", sodium)
	e.write(e.game, "mods/zoom.jar.disabled", zoom)
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
		{Kind: content.Mods, Rel: "mods/sodium.jar", Enabled: true, Name: "Sodium", SHA1: s1, SHA512: s5, Size: int64(len(sodium)), URL: "https://cdn.modrinth.com/data/AANobbMI/versions/x/sodium.jar"},
		{Kind: content.Mods, Rel: "mods/zoom.jar.disabled", Name: "Zoomify", SHA1: z1, SHA512: z5, Size: int64(len(zoom)), URL: "https://cdn.modrinth.com/data/w7ThoJFB/versions/y/zoom.jar"},
		{Kind: content.Mods, Rel: "mods/own.jar", Enabled: true, SHA1: o1, SHA512: o5, Size: int64(len(own)), URL: "https://edge.forgecdn.net/files/1/2/own.jar"},
	}
	dest := filepath.Join(t.TempDir(), "Survival.mrpack")
	p := Plan{Format: Mrpack, Include: Include{Options: true, Config: true}}
	pv, err := e.svc.Preview(context.Background(), e.in.ID, p)
	if err != nil || len(pv.Linked) != 1 || pv.Linked[0].Name != "Sodium" || len(pv.Off) != 1 || pv.Off[0].Name != "Zoomify" {
		t.Fatalf("preview %+v %v", pv, err)
	}
	// own.jar and options.txt by name; config/ and the shared shaderpacks/ as folders.
	var bundled []string
	for _, b := range pv.Bundled {
		bundled = append(bundled, b.Name+" "+b.Detail)
	}
	if !slices.Equal(bundled, []string{"config/ 1 file", "options.txt ", "own.jar ", "shaderpacks/ 1 file"}) {
		t.Fatalf("bundled %q", bundled)
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
		for _, bad := range []string{"logs/", "saves/", "usercache", "session.lock", "/.index/", "sodium.jar", "zoom"} {
			if strings.Contains(name, bad) {
				t.Errorf("%s must not be exported", name)
			}
		}
	}
	pack, err := modpack.NewReader(modpack.Deps{}).File(context.Background(), dest)
	if err != nil {
		t.Fatal(err)
	}
	if pack.Minecraft != "1.21.1" || pack.Loader != "fabric" || pack.LoaderVersion != "0.16.9" || len(pack.Files) != 1 || pack.Files[0].Path != "mods/sodium.jar" {
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

func mr(id string) *content.Ident {
	return &content.Ident{Source: "modrinth", ProjectID: id, VersionID: "v" + id, URL: "https://cdn.modrinth.com/data/" + id + "/f.jar"}
}

func TestModList(t *testing.T) {
	e := newEnv(t)
	e.items = manifest{
		{Kind: content.Mods, Rel: "mods/a.jar", Enabled: true, Name: "Sodium | <b>fast</b>", Version: "0.6", Page: "https://modrinth.com/project/sodium", Authors: []string{"Caffeine_MC"}, SHA1: "s1", Modrinth: mr("AANobbMI")},
		{Kind: content.Mods, Rel: "mods/b.jar", Enabled: true, Name: "Bad link", Page: "javascript:alert(1)", Modrinth: mr("BBBBBBBB")},
		{Kind: content.Mods, Rel: "mods/c.jar.disabled", Name: "Off", Modrinth: mr("CCCCCCCC")},
		{Kind: content.Mods, Rel: "mods/own.jar", Enabled: true, Name: "Own"},
		{Kind: content.Mods, Rel: "mods/blocked.jar", Enabled: true, Name: "Blocked", CurseForge: &content.Ident{Source: "curseforge", ProjectID: "1", VersionID: "2"}},
		{Kind: content.ResourcePacks, Rel: "resourcepacks/p.zip", Enabled: true, Name: "Pack", Modrinth: mr("PPPPPPPP")},
	}
	dir := t.TempDir()
	if err := e.svc.Write(context.Background(), e.in.ID, Plan{Format: List, List: Markdown}, filepath.Join(dir, "mods.md")); err != nil {
		t.Fatal(err)
	}
	md, _ := os.ReadFile(filepath.Join(dir, "mods.md"))
	if !bytes.Contains(md, []byte(`| [Sodium \| &lt;b&gt;fast&lt;/b&gt;](https://modrinth.com/project/sodium) | 0.6 | Caffeine\_MC |`)) || bytes.Contains(md, []byte("javascript")) {
		t.Fatalf("md:\n%s", md)
	}
	for _, left := range []string{"Off", "Own", "Blocked", "Pack"} {
		if bytes.Contains(md, []byte("| "+left)) {
			t.Errorf("%s is in the list", left)
		}
	}
	if err := e.svc.Write(context.Background(), e.in.ID, Plan{Format: List, List: JSON}, filepath.Join(dir, "mods.json")); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "mods.json"))
	var l ModList
	if err := json.Unmarshal(raw, &l); err != nil || l.Schema != 1 || l.Minecraft != "1.21.1" || l.Loader == nil || l.Loader.Kind != "fabric" || len(l.Mods) != 2 {
		t.Fatalf("json %+v %v", l, err)
	}
	if m := l.Mods[1]; m.Name != "Sodium | <b>fast</b>" || m.Source != "modrinth" || m.Project != "AANobbMI" || m.File != "vAANobbMI" || m.SHA1 != "s1" {
		t.Fatalf("mod %+v", m)
	}
	pv, err := e.svc.Preview(context.Background(), e.in.ID, Plan{Format: List, List: JSON})
	if err != nil || len(pv.Listed) != 2 || len(pv.Unlisted) != 2 || len(pv.Off) != 1 || pv.Bytes != int64(len(raw)) {
		t.Fatalf("preview %+v %v", pv, err)
	}
	e.items = nil
	if err := e.svc.Write(context.Background(), e.in.ID, Plan{Format: List, List: Markdown}, filepath.Join(dir, "none.md")); !errors.Is(err, ErrNoMods) {
		t.Fatal(err)
	}
	if (Plan{Format: List, List: "csv"}).Valid() {
		t.Fatal("csv is still a list format")
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

func TestCurseForgeZip(t *testing.T) {
	e := newEnv(t)
	jei, own := []byte("jei jar"), []byte("own jar")
	j1, _ := sums(jei)
	o1, _ := sums(own)
	e.write(e.game, "mods/jei.jar", jei)
	e.write(e.game, "mods/own.jar", own)
	e.items = manifest{
		{Kind: content.Mods, Rel: "mods/jei.jar", Enabled: true, SHA1: j1, Name: "JEI <3", Version: "15.2", Page: "https://www.curseforge.com/minecraft/mc-mods/jei",
			CurseForge: &content.Ident{Source: "curseforge", ProjectID: "238222", VersionID: "5101366", URL: "https://edge.forgecdn.net/files/5101/366/jei.jar"}},
		{Kind: content.Mods, Rel: "mods/own.jar", Enabled: true, SHA1: o1, Name: "Own"},
	}
	if _, err := e.svc.d.Instances.SetVersion(e.in.ID, "1.20.1", "neoforge", "1.20.1-47.1.106"); err != nil {
		t.Fatal(err)
	}
	p := Plan{Format: CurseForge, Meta: instance.ExportMeta{Name: "Pack", Version: "1.0", Author: "me"}}
	if pv, err := e.svc.Preview(context.Background(), e.in.ID, p); err != nil || len(pv.Linked) != 1 || pv.Linked[0].Source != "curseforge" || len(pv.Bundled) != 1 {
		t.Fatalf("preview %+v %v", pv, err)
	}
	dest := filepath.Join(t.TempDir(), "Pack.zip")
	if err := e.svc.Write(context.Background(), e.in.ID, p, dest); err != nil {
		t.Fatal(err)
	}
	got := names(t, dest)
	if !slices.Equal(got, []string{"manifest.json", "modlist.html", "overrides/mods/own.jar"}) {
		t.Fatalf("zip %v", got)
	}
	zr, _ := zip.OpenReader(dest)
	defer func() { _ = zr.Close() }()
	read := func(name string) string {
		f, _ := zr.Open(name)
		var b bytes.Buffer
		_, _ = b.ReadFrom(f)
		return b.String()
	}
	m := read("manifest.json")
	for _, want := range []string{`"manifestType": "minecraftModpack"`, `"id": "neoforge-1.20.1-47.1.106"`, `"projectID": 238222`, `"fileID": 5101366`, `"author": "me"`, `"overrides": "overrides"`} {
		if !strings.Contains(m, want) {
			t.Errorf("manifest lacks %s:\n%s", want, m)
		}
	}
	if h := read("modlist.html"); !strings.Contains(h, `<a href="https://www.curseforge.com/minecraft/mc-mods/jei">JEI &lt;3</a>`) {
		t.Fatalf("modlist.html:\n%s", h)
	}
}
