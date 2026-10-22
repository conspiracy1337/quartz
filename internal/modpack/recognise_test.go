package modpack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"quartz/internal/core/hashx"
	"quartz/internal/instance"
	"quartz/internal/modplatform"
)

// found is a Modrinth whose search finds one pack.
type found struct {
	packs
	title string
}

func (f found) Search(context.Context, modplatform.SearchQuery) (modplatform.SearchPage, error) {
	return modplatform.SearchPage{Hits: []modplatform.Hit{{Source: modplatform.Modrinth, ID: "PACK", Title: f.title}}, Total: 1}, nil
}

func TestRecogniseFindsTheInstalledVersion(t *testing.T) {
	jars := map[string][]byte{}
	for _, k := range []string{"a1", "a2", "a3", "b", "c", "d"} {
		jars[k] = []byte("jar " + k)
	}
	served := cdn{}
	file := func(key string) map[string]any {
		s1, s5 := hashes(jars[key])
		url := "https://cdn.modrinth.com/data/x/" + key + ".jar"
		served[url] = jars[key]
		return map[string]any{"path": "mods/" + key + ".jar", "hashes": map[string]string{"sha1": s1, "sha512": s5}, "downloads": []string{url}, "fileSize": len(jars[key])}
	}
	version := func(id, number string, mods ...string) modplatform.Version {
		var files []any
		for _, m := range mods {
			files = append(files, file(m))
		}
		index := js(map[string]any{"formatVersion": 1, "game": "minecraft", "versionId": number, "name": "Cool Pack",
			"dependencies": map[string]string{"minecraft": "1.21.1"}, "files": files})
		zip := zipOf(t, entry{name: "modrinth.index.json", body: index}, entry{name: "overrides/config/x.toml", body: "x"})
		url := "https://cdn.modrinth.com/data/PACK/" + id + ".mrpack"
		served[url], _ = os.ReadFile(zip) //nolint:gosec // test
		return modplatform.Version{ID: id, Number: number, Type: "release", Game: []string{"1.21.1"},
			File: modplatform.File{Name: id + ".mrpack", URL: url, SHA1: sumOf(served[url])}}
	}
	plat := found{title: "Cool Pack", packs: packs{versions: []modplatform.Version{
		version("V3", "3.0", "a3", "b", "c", "d"),
		version("V2", "2.0", "a2", "b", "c", "d"),
		version("V1", "1.0", "a1", "b", "c"),
		version("V0", "0.9", "a1", "b"),
	}}}
	env := newInstallEnv(t, served)
	env.svc.d.Reader = NewReader(Deps{Modrinth: plat, Cache: filepath.Join(env.root, "cache"), Fetch: func(_ context.Context, raw string, _ int64, _ hashx.Sum, dest string) error {
		return os.WriteFile(dest, served[raw], 0o600)
	}})
	// The player renamed it on import; the pack's own name still finds it.
	in, err := env.insts.Create(instance.CreateRequest{Name: "My stuff", Minecraft: "1.21.1"})
	if err != nil {
		t.Fatal(err)
	}
	mods := env.insts.Path(in.ID, "mods")
	_ = os.MkdirAll(mods, 0o700)
	for name, key := range map[string]string{"a2.jar": "a2", "b.jar": "b", "c.jar.disabled": "c", "d.jar": "d", "mine.jar": "a1"} {
		_ = os.WriteFile(filepath.Join(mods, name), jars[key], 0o600)
	}
	ctx := context.Background()
	if err := env.svc.Recognise(ctx, in.ID); !errors.Is(err, ErrNotRecognised) {
		t.Fatalf("matched a pack by a name it doesn't have: %v", err)
	}
	if err := env.svc.Recognise(ctx, in.ID, "Cool Pack 2.0"); err != nil {
		t.Fatal(err)
	}
	in, _ = env.insts.Get(in.ID)
	if in.Pack == nil || in.Pack.Source != "modrinth" || in.Pack.Project != "PACK" || in.Pack.Version != "V2" || in.Pack.VersionName != "2.0" {
		t.Fatalf("linked %+v", in.Pack)
	}
	m, err := ReadManifest(env.insts.Dir(in.ID))
	if err != nil || len(m.Files) != 5 || !m.Files["config/x.toml"].Layer {
		t.Fatalf("manifest %+v %v", m, err)
	}
	if left, _ := os.ReadDir(filepath.Join(env.root, "cache")); len(left) != 0 {
		t.Fatalf("%d fetched packs left in the cache", len(left))
	}
	// A pack linked without a manifest (a recorded link) gets one from its
	// installed version at the first review.
	_ = os.Remove(filepath.Join(env.insts.Dir(in.ID), ManifestName))
	d, err := env.svc.Diff(ctx, in.ID, "V3")
	if err != nil || d.To != "3.0" || len(d.Rows) != 2 {
		t.Fatalf("diff %+v %v", d, err)
	}
	if _, err := ReadManifest(env.insts.Dir(in.ID)); err != nil {
		t.Fatalf("baseline not written: %v", err)
	}

	// Another launcher's folder played in place is never linked, and a
	// pack the instance has too few mods of isn't taken.
	other, _ := env.insts.Create(instance.CreateRequest{Name: "Cool Pack", Minecraft: "1.21.1"})
	_, _ = env.insts.Update(other.ID, func(i *instance.Instance) error { i.InPlace = true; return nil })
	if err := env.svc.Recognise(ctx, other.ID); !errors.Is(err, ErrNotRecognised) {
		t.Fatalf("in place: %v", err)
	}
	third, _ := env.insts.Create(instance.CreateRequest{Name: "Cool Pack", Minecraft: "1.21.1"})
	_ = os.MkdirAll(env.insts.Path(third.ID, "mods"), 0o700)
	for _, k := range []string{"a1", "x", "y"} {
		_ = os.WriteFile(filepath.Join(env.insts.Path(third.ID, "mods"), k+".jar"), []byte("other "+k), 0o600)
	}
	if err := env.svc.Recognise(ctx, third.ID); !errors.Is(err, ErrNotRecognised) {
		t.Fatalf("unrelated mods: %v", err)
	}
}
