package modpack

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"quartz/internal/core/hashx"
	"quartz/internal/modplatform"
)

type entry struct {
	name, body string
	mode       fs.FileMode
}

func zipOf(t *testing.T, entries ...entry) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.mode != 0 {
			h.SetMode(e.mode)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(e.body))
	}
	_ = zw.Close()
	p := filepath.Join(t.TempDir(), "pack.zip")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func js(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

const (
	sha1A   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sha512A = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func mrFile(path string, url string, env map[string]string) map[string]any {
	f := map[string]any{"path": path, "hashes": map[string]string{"sha1": sha1A, "sha512": sha512A}, "downloads": []string{url}, "fileSize": 10}
	if env != nil {
		f["env"] = env
	}
	return f
}

func TestReadMrpack(t *testing.T) {
	index := js(map[string]any{"formatVersion": 1, "game": "minecraft", "versionId": "1.6.1", "name": "Cobblemon",
		"dependencies": map[string]string{"minecraft": "1.21.1", "fabric-loader": "0.16.9"},
		"files": []any{
			mrFile("mods/cobblemon.jar", "https://cdn.modrinth.com/data/x/cobblemon.jar", nil),
			mrFile("mods/extra.jar", "https://github.com/a/b/releases/extra.jar", map[string]string{"client": "optional"}),
			mrFile("mods/server.jar", "https://cdn.modrinth.com/s.jar", map[string]string{"client": "unsupported"}),
			mrFile("mods/evil.jar", "https://evil.example/e.jar", nil),
		}})
	file := zipOf(t, entry{name: "modrinth.index.json", body: index}, entry{name: "overrides/config/a.toml", body: "a=1"},
		entry{name: "overrides/mods/hotfix.jar", body: "jar"}, entry{name: "client-overrides/options.txt", body: "fov:1"},
		entry{name: "overrides/link", body: "/etc/passwd", mode: fs.ModeSymlink | 0o777})
	p, err := NewReader(Deps{}).File(context.Background(), file)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Cobblemon" || p.Version != "1.6.1" || p.Minecraft != "1.21.1" || p.Loader != "fabric" || p.LoaderVersion != "0.16.9" {
		t.Fatalf("pack %+v", p)
	}
	if len(p.Files) != 2 || p.Files[1].Path != "mods/extra.jar.disabled" || len(p.Warnings) != 1 {
		t.Fatalf("files %+v warnings %v", p.Files, p.Warnings)
	}
	if opt := p.Optional(); len(opt) != 1 || opt[0] != (OptionalFile{Path: "mods/extra.jar", Name: "extra.jar"}) {
		t.Fatalf("optional %+v", opt)
	}
	q := *p
	q.Files = slices.Clone(p.Files)
	q.Enable([]string{"mods/extra.jar", "mods/cobblemon.jar"})
	if q.Files[1].Path != "mods/extra.jar" || q.Files[0].Path != "mods/cobblemon.jar" || p.Files[1].Path != "mods/extra.jar.disabled" {
		t.Fatalf("enabled %+v", q.Files)
	}
	st, err := p.Stats()
	if err != nil || st.Mods != 3 || st.Files != 2 || !slices.Contains(st.Foreign, "mods/hotfix.jar") || !slices.Contains(st.Foreign, "extra.jar.disabled") {
		t.Fatalf("stats %+v %v", st, err)
	}
	dir := t.TempDir()
	root, _ := os.OpenRoot(dir)
	defer func() { _ = root.Close() }()
	got, err := p.Extract(root)
	if err != nil || len(got) != 3 || got["options.txt"] == "" {
		t.Fatalf("extracted %v %v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "link")); err == nil {
		t.Fatal("a link entry was written")
	}
}

func TestHostilePacksAreRefused(t *testing.T) {
	for name, path := range map[string]string{"parent": "../evil.jar", "absolute": "/etc/x", "drive": "C:/x.jar", "nested": "mods/../../x",
		"launcher": "mods/.index/x.pw.toml", "quartz": ".quartz/removed/x", "device": "mods/con.jar", "backslash": "..\\x"} {
		index := js(map[string]any{"formatVersion": 1, "game": "minecraft", "dependencies": map[string]string{"minecraft": "1.21.1"},
			"files": []any{mrFile(path, "https://cdn.modrinth.com/x.jar", nil)}})
		if _, err := NewReader(Deps{}).File(context.Background(), zipOf(t, entry{name: "modrinth.index.json", body: index})); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A hostile entry in the overrides is skipped, never written.
	index := js(map[string]any{"formatVersion": 1, "game": "minecraft", "dependencies": map[string]string{"minecraft": "1.21.1"}})
	p, err := NewReader(Deps{}).File(context.Background(), zipOf(t, entry{name: "modrinth.index.json", body: index},
		entry{name: "overrides/../../escape.txt", body: "x"}, entry{name: "overrides/ok.txt", body: "x"}))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	root, _ := os.OpenRoot(filepath.Join(dir))
	got, err := p.Extract(root)
	_ = root.Close()
	if err != nil || len(got) != 1 || got["ok.txt"] == "" {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := NewReader(Deps{}).File(context.Background(), zipOf(t, entry{name: "readme.txt", body: "hi"})); !errors.Is(err, ErrNotPack) {
		t.Fatalf("not a pack: %v", err)
	}
}

// curse is a CurseForge that knows a few files.
type curse struct {
	modplatform.Platform
	files map[string]modplatform.Version
}

func (c curse) Ready() bool { return true }
func (c curse) Files(_ context.Context, ids []string) (map[string]modplatform.Version, error) {
	out := map[string]modplatform.Version{}
	for _, id := range ids {
		if v, ok := c.files[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

func TestReadCurseForgeZip(t *testing.T) {
	manifest := js(map[string]any{"manifestType": "minecraftModpack", "manifestVersion": 1, "name": "ATM", "version": "1.4.0",
		"minecraft": map[string]any{"version": "1.20.1", "modLoaders": []any{map[string]any{"id": "neoforge-1.20.1-47.1.106", "primary": true}}},
		"files":     []any{map[string]any{"projectID": 1, "fileID": 10}, map[string]any{"projectID": 2, "fileID": 20, "required": false}, map[string]any{"projectID": 3, "fileID": 30}},
		"overrides": "overrides"})
	cf := curse{files: map[string]modplatform.Version{
		"10": {ProjectID: "1", Class: modplatform.TypeMod, File: modplatform.File{Name: "jei.jar", URL: "https://edge.forgecdn.net/files/1/0/jei.jar", SHA1: sha1A, Size: 5}},
		"20": {ProjectID: "2", Class: modplatform.TypeResourcePack, File: modplatform.File{Name: "pack.zip", SHA1: sha1A, Size: 5, Page: "https://www.curseforge.com/x/download/20"}},
		"30": {ProjectID: "3", Class: modplatform.TypeDataPack, File: modplatform.File{Name: "dp.zip", URL: "https://edge.forgecdn.net/dp.zip", SHA1: sha1A}},
	}}
	p, err := NewReader(Deps{CurseForge: cf}).File(context.Background(), zipOf(t, entry{name: "manifest.json", body: manifest}))
	if err != nil {
		t.Fatal(err)
	}
	if p.Loader != "neoforge" || p.LoaderVersion != "47.1.106" || p.Minecraft != "1.20.1" || len(p.Files) != 2 || len(p.Warnings) != 1 {
		t.Fatalf("pack %+v", p)
	}
	if f := p.Files[1]; f.Path != "resourcepacks/pack.zip.disabled" || !f.Blocked() || f.Page == "" {
		t.Fatalf("blocked file %+v", f)
	}
	for id, want := range map[string][2]string{"forge-47.2.0": {"forge", "47.2.0"}, "fabric-0.16.9": {"fabric", "0.16.9"}, "neoforge-21.1.77": {"neoforge", "21.1.77"}} {
		if k, v, ok := cfLoader(id, "1.20.1"); !ok || k != want[0] || v != want[1] {
			t.Errorf("%s: %s %s", id, k, v)
		}
	}
	if _, err := NewReader(Deps{}).File(context.Background(), zipOf(t, entry{name: "manifest.json", body: manifest})); err == nil {
		t.Fatal("read a CurseForge pack without CurseForge")
	}
}

func TestReadMMCZipIgnoresCommands(t *testing.T) {
	cfg := "InstanceType=OneSix\nname=My Pack\nPreLaunchCommand=rm -rf ~\nJvmArgs=-javaagent:evil.jar\n"
	pack := js(map[string]any{"components": []any{
		map[string]any{"uid": "org.lwjgl3", "version": "3.3.3"}, map[string]any{"uid": "net.minecraft", "version": "1.21.1"},
		map[string]any{"uid": "net.fabricmc.intermediary", "version": "1.21.1"}, map[string]any{"uid": "net.fabricmc.fabric-loader", "version": "0.16.9"},
		map[string]any{"uid": "custom.jarmod", "version": "1"}}})
	file := zipOf(t, entry{name: "My Pack/instance.cfg", body: cfg}, entry{name: "My Pack/mmc-pack.json", body: pack},
		entry{name: "My Pack/.minecraft/mods/a.jar", body: "jar"}, entry{name: "My Pack/.minecraft/options.txt", body: "x"})
	p, err := NewReader(Deps{}).File(context.Background(), file)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "My Pack" || p.Minecraft != "1.21.1" || p.Loader != "fabric" || len(p.Warnings) != 1 || p.Layers[0] != "My Pack/.minecraft/" {
		t.Fatalf("pack %+v", p)
	}
	st, _ := p.Stats()
	if st.Mods != 1 || st.Files != 1 {
		t.Fatalf("stats %+v", st)
	}
}

func TestReadTechnicZip(t *testing.T) {
	version := js(map[string]any{"id": "1.12.2-forge-14.23.5.2860", "inheritsFrom": "1.12.2",
		"libraries": []any{map[string]string{"name": "net.minecraftforge:forge:1.12.2-14.23.5.2860"}}})
	p, err := NewReader(Deps{}).File(context.Background(), zipOf(t, entry{name: "bin/version.json", body: version},
		entry{name: "bin/minecraft.jar", body: "x"}, entry{name: "mods/a.jar", body: "jar"}, entry{name: "config/b.cfg", body: "b"}))
	if err != nil {
		t.Fatal(err)
	}
	if p.Minecraft != "1.12.2" || p.Loader != "forge" || p.LoaderVersion != "14.23.5.2860" {
		t.Fatalf("pack %+v", p)
	}
	st, _ := p.Stats()
	if st.Mods != 1 || st.Files != 1 {
		t.Fatalf("bin/ was counted: %+v", st)
	}
	neo := js(map[string]any{"inheritsFrom": "1.21.1", "libraries": []any{map[string]string{"name": "net.neoforged.fancymodloader:loader:4.0.0"}},
		"arguments": map[string]any{"game": []any{"--fml.neoForgeVersion", "21.1.77"}}})
	if mc, k, v := technicLoader([]byte(neo)); mc != "1.21.1" || k != "neoforge" || v != "21.1.77" {
		t.Fatalf("%s %s %s", mc, k, v)
	}
}

// api answers JSON by URL.
type api map[string]any

func (a api) JSON(_ context.Context, _, rawURL string, _ http.Header, _, out any) error {
	v, ok := a[rawURL]
	if !ok {
		return errors.New("404")
	}
	b, _ := json.Marshal(v)
	return json.Unmarshal(b, out)
}

func TestLinks(t *testing.T) {
	r := NewReader(Deps{HTTP: api{
		ftbAPI + "119": map[string]any{"name": "FTB Skies", "versions": []any{map[string]any{"id": 1, "type": "release"}, map[string]any{"id": 5, "type": "release"},
			map[string]any{"id": 9, "type": "beta"}}},
		ftbAPI + "119/5": map[string]any{"name": "1.4", "targets": []any{map[string]any{"type": "game", "version": "1.20.1"}, map[string]any{"type": "modloader", "name": "neoforge", "version": "47.1.106"}},
			"files": []any{map[string]any{"path": "./mods/", "name": "a.jar", "url": "https://dist.modpacks.ch/a.jar", "sha1": sha1A, "size": 3},
				map[string]any{"path": "./mods/", "name": "s.jar", "url": "https://dist.modpacks.ch/s.jar", "sha1": sha1A, "serveronly": true}}},
	}})
	p, err := r.Link(context.Background(), "https://www.feed-the-beast.com/modpacks/119-ftb-skies")
	if err != nil {
		t.Fatal(err)
	}
	if p.Origin.Source != "ftb" || p.Origin.Version != "5" || p.Loader != "neoforge" || len(p.Files) != 1 || p.Files[0].Path != "mods/a.jar" {
		t.Fatalf("ftb %+v", p)
	}
	for _, bad := range []string{"http://modrinth.com/modpack/x", "https://modrinth.com/mod", "https://user@modrinth.com/modpack/x",
		"https://www.curseforge.com/minecraft/mc-mods/jei", "https://technicpack.net/modpack/../x", "not a link"} {
		if _, err := r.Link(context.Background(), bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	// A direct link is fetched into the cache and read.
	cache := t.TempDir()
	index := js(map[string]any{"formatVersion": 1, "game": "minecraft", "dependencies": map[string]string{"minecraft": "1.21.1"}})
	src := zipOf(t, entry{name: "modrinth.index.json", body: index})
	r = NewReader(Deps{Cache: cache, Fetch: func(_ context.Context, raw string, _ int64, _ hashx.Sum, dest string) error {
		if !strings.HasPrefix(raw, "https://example.com/") {
			t.Errorf("fetched %s", raw)
		}
		b, _ := os.ReadFile(src) //nolint:gosec // test
		return os.WriteFile(dest, b, 0o600)
	}})
	if p, err := r.Link(context.Background(), "https://example.com/packs/my.mrpack"); err != nil || p.Minecraft != "1.21.1" {
		t.Fatalf("direct %+v %v", p, err)
	}
}

func TestCleanPath(t *testing.T) {
	for in, want := range map[string]string{"./mods/a.jar": "mods/a.jar", "config\\b.toml": "config/b.toml", "a//b": ""} {
		got, err := CleanPath(in)
		if want == "" && err == nil || want != "" && got != want {
			t.Errorf("%q → %q %v", in, got, err)
		}
	}
}

func TestFlexInt(t *testing.T) {
	var v struct{ A, B flexInt }
	if err := json.Unmarshal([]byte(`{"A":12,"B":"34"}`), &v); err != nil || v.A != 12 || v.B != 34 {
		t.Fatalf("%+v %v", v, err)
	}
}

// twins is a Modrinth that has some files by sha1.
type twins struct {
	modplatform.Platform
	files map[string]modplatform.Version
}

func (m twins) Ready() bool { return true }
func (m twins) Identify(_ context.Context, hs []modplatform.Hashes) (map[string]modplatform.Version, error) {
	out := map[string]modplatform.Version{}
	for _, h := range hs {
		if v, ok := m.files[h.SHA1]; ok {
			out[h.SHA1] = v
		}
	}
	return out, nil
}

func TestCurseForgeGoneAndBlockedFiles(t *testing.T) {
	manifest := js(map[string]any{"manifestType": "minecraftModpack", "manifestVersion": 1, "name": "P", "version": "1",
		"minecraft": map[string]any{"version": "1.20.1"},
		"files":     []any{map[string]any{"projectID": 1, "fileID": 10}, map[string]any{"projectID": 2, "fileID": 20}, map[string]any{"projectID": 9, "fileID": 90}},
	})
	sha1B := strings.Repeat("b", 40)
	cf := curse{files: map[string]modplatform.Version{
		"10": {ProjectID: "1", Class: modplatform.TypeMod, File: modplatform.File{Name: "a.jar", SHA1: sha1A, Size: 5, Page: "https://www.curseforge.com/x/download/10"}},
		"20": {ProjectID: "2", Class: modplatform.TypeMod, File: modplatform.File{Name: "b.jar", SHA1: sha1B, Size: 5, Page: "https://www.curseforge.com/y/download/20"}},
	}}
	mr := twins{files: map[string]modplatform.Version{
		sha1A: {File: modplatform.File{URL: "https://cdn.modrinth.com/data/a/a.jar", SHA1: sha1A, SHA512: sha512A}},
		sha1B: {File: modplatform.File{URL: "https://evil.example/b.jar", SHA1: sha1B}},
	}}
	p, err := NewReader(Deps{CurseForge: cf, Modrinth: mr}).File(context.Background(), zipOf(t, entry{name: "manifest.json", body: manifest}))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 2 || !slices.Contains(p.Warnings, "1 file CurseForge no longer has was left out") {
		t.Fatalf("files %+v warnings %v", p.Files, p.Warnings)
	}
	if f := p.Files[0]; f.Blocked() || f.URLs[0] != "https://cdn.modrinth.com/data/a/a.jar" || f.SHA512 != sha512A {
		t.Fatalf("Modrinth copy not used: %+v", f)
	}
	if !p.Files[1].Blocked() {
		t.Fatalf("a copy from an address that isn't Modrinth's CDN was used: %+v", p.Files[1])
	}
}

func TestCurseLoaderIDRoundTrip(t *testing.T) {
	for _, c := range [][3]string{{"neoforge", "1.20.1", "47.1.106"}, {"neoforge", "1.21.1", "21.1.77"}, {"forge", "1.20.1", "47.3.0"}, {"fabric", "1.21.1", "0.16.9"}} {
		if k, v, ok := cfLoader(CurseLoaderID(c[0], c[1], c[2]), c[1]); !ok || k != c[0] || v != c[2] {
			t.Errorf("%v → %s %s", c, k, v)
		}
	}
}
