package modpack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"quartz/internal/core/hashx"
	"quartz/internal/instance"
	"quartz/internal/modplatform"
)

func TestPlanThreeWay(t *testing.T) {
	old := Manifest{Files: map[string]ManifestFile{
		"mods/same.jar":     {SHA1: "s"},
		"mods/upd.jar":      {SHA1: "u1"},
		"mods/removed.jar":  {SHA1: "r1"},        // the player removed it
		"mods/dropped.jar":  {SHA1: "d1"},        // the pack drops it, unchanged on disk
		"config/edited.txt": {SHA1: "e1", Layer: true},
		"config/kept.txt":   {SHA1: "k1", Layer: true}, // the pack drops it, the player changed it
		"config/gone.txt":   {SHA1: "g1", Layer: true}, // missing on disk, still in the pack
	}}
	next := map[string]want{
		"mods/same.jar": {SHA1: "s"}, "mods/upd.jar": {SHA1: "u2"}, "mods/removed.jar": {SHA1: "r2"}, "mods/new.jar": {SHA1: "n1"},
		"config/edited.txt": {SHA1: "e2", Layer: true}, "config/gone.txt": {SHA1: "g2", Layer: true}, "config/mine.txt": {SHA1: "m1", Layer: true},
	}
	now := map[string]disk{
		"mods/same.jar": {true, "s", false}, "mods/upd.jar": {true, "u1", true}, "mods/dropped.jar": {true, "d1", false},
		"config/edited.txt": {true, "player", false}, "config/kept.txt": {true, "player", false}, "config/mine.txt": {true, "players own", false},
	}
	changes, kept := plan(old, next, func(rel string) disk { return now[rel] })
	got := map[string]change{}
	for _, c := range changes {
		got[c.rel] = c
	}
	if c := got["mods/upd.jar"]; c.act != actWrite || !c.off || c.w.SHA1 != "u2" {
		t.Errorf("update keeps it off: %+v", c)
	}
	if _, ok := got["mods/removed.jar"]; ok {
		t.Error("a removed mod came back")
	}
	if c := got["mods/new.jar"]; c.act != actWrite {
		t.Error("a new mod was not added")
	}
	if c := got["mods/dropped.jar"]; c.act != actDelete {
		t.Error("an unchanged dropped mod stayed")
	}
	if c := got["config/gone.txt"]; c.act != actWrite {
		t.Error("a missing config was not written back")
	}
	for _, p := range []string{"mods/same.jar", "config/edited.txt", "config/kept.txt", "config/mine.txt"} {
		if _, ok := got[p]; ok {
			t.Errorf("%s was touched", p)
		}
	}
	if !slices.Equal(kept, []string{"config/edited.txt", "config/mine.txt"}) {
		t.Errorf("kept %v", kept)
	}
}

// packs is a Modrinth with one modpack in two versions.
type packs struct {
	modplatform.Platform
	versions []modplatform.Version
}

func (p packs) Source() modplatform.Source { return modplatform.Modrinth }
func (p packs) Ready() bool                { return true }
func (p packs) Project(context.Context, string) (modplatform.Detail, error) {
	return modplatform.Detail{Project: modplatform.Project{ID: "PACK", Title: "Pack"}, Type: modplatform.TypeModpack}, nil
}
func (p packs) Versions(context.Context, string, modplatform.Query) ([]modplatform.Version, error) {
	return p.versions, nil
}
func (p packs) Identify(context.Context, []modplatform.Hashes) (map[string]modplatform.Version, error) {
	return nil, nil
}
func (p packs) Changelogs(_ context.Context, vs []modplatform.Version) error {
	vs[0].Changelog = "- Fixes quests\n- Updates Create"
	return nil
}

// packUpdate installs version 1.0 of a two-version pack, then turns mod a
// off and edits config/y.toml as a player would.
func packUpdate(t *testing.T) (context.Context, *installEnv, instance.Instance, string) {
	t.Helper()
	jars := map[string][]byte{"a1": []byte("A1"), "a2": []byte("A2"), "b1": []byte("B1"), "c1": []byte("C1")}
	served := cdn{}
	file := func(name, key string) map[string]any {
		s1, s5 := hashes(jars[key])
		url := "https://cdn.modrinth.com/data/x/" + key + ".jar"
		served[url] = jars[key]
		return map[string]any{"path": "mods/" + name, "hashes": map[string]string{"sha1": s1, "sha512": s5}, "downloads": []string{url}, "fileSize": len(jars[key])}
	}
	mrpack := func(version string, files []any, overrides ...entry) string {
		index := js(map[string]any{"formatVersion": 1, "game": "minecraft", "versionId": version, "name": "Pack",
			"dependencies": map[string]string{"minecraft": "1.21.1"}, "files": files})
		return zipOf(t, append([]entry{{name: "modrinth.index.json", body: index}}, overrides...)...)
	}
	v1 := mrpack("1.0", []any{file("a.jar", "a1"), file("b.jar", "b1")},
		entry{name: "overrides/config/x.toml", body: "x1"}, entry{name: "overrides/config/y.toml", body: "y1"})
	v2 := mrpack("2.0", []any{file("a.jar", "a2"), file("c.jar", "c1")},
		entry{name: "overrides/config/x.toml", body: "x2"}, entry{name: "overrides/config/y.toml", body: "y2"}, entry{name: "overrides/config/z.toml", body: "z"})
	served["https://cdn.modrinth.com/data/PACK/v2.mrpack"], _ = os.ReadFile(v2) //nolint:gosec // test

	env := newInstallEnv(t, served)
	plat := packs{versions: []modplatform.Version{
		{ID: "V2", Number: "2.0", Type: "release", File: modplatform.File{Name: "v2.mrpack", URL: "https://cdn.modrinth.com/data/PACK/v2.mrpack", SHA1: sumOf(served["https://cdn.modrinth.com/data/PACK/v2.mrpack"])}},
		{ID: "V1", Number: "1.0", Type: "release"},
	}}
	env.svc.d.Reader = NewReader(Deps{Modrinth: plat, Cache: filepath.Join(env.root, "cache"), Fetch: func(_ context.Context, raw string, _ int64, _ hashx.Sum, dest string) error {
		return os.WriteFile(dest, served[raw], 0o600)
	}})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	p, err := env.svc.d.Reader.File(ctx, v1)
	if err != nil {
		t.Fatal(err)
	}
	p.Origin = Origin{Source: "modrinth", Project: "PACK", Version: "V1"}
	job, err := env.svc.Install(ctx, env.svc.Stage(p), "Pack", instance.Art{Background: "#3ecf8e"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.eng.Wait(ctx, job); err != nil {
		t.Fatal(err)
	}
	in := env.insts.List()[0]
	game := env.insts.GameDir(in.ID)
	// The player edits a config and turns mod a off.
	_ = os.WriteFile(filepath.Join(game, "config", "y.toml"), []byte("mine"), 0o600)
	_ = os.Rename(filepath.Join(game, "mods", "a.jar"), filepath.Join(game, "mods", "a.jar.disabled"))

	return ctx, env, in, game
}

func TestUpdateKeepsEdits(t *testing.T) {
	ctx, env, in, game := packUpdate(t)
	if _, err := env.svc.Diff(ctx, in.ID, ""); err == nil {
		t.Fatal("diff before a check")
	}
	if _, err := env.svc.Diff(ctx, in.ID, "V1"); !errors.Is(err, ErrUpToDate) {
		t.Fatalf("diff to the installed version: %v", err)
	}
	if d, err := env.svc.Diff(ctx, in.ID, "V2"); err != nil || d.To != "2.0" {
		t.Fatalf("diff to a picked version: %+v %v", d, err)
	}
	if av := env.svc.CheckUpdates(ctx); len(av) != 1 || av[0].Version != "2.0" {
		t.Fatalf("available %+v", av)
	}
	d, err := env.svc.Diff(ctx, in.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if d.From != "1.0" || d.To != "2.0" || !slices.Equal(d.KeptEdits, []string{"config/y.toml"}) || d.Summary != "Fixes quests Updates Create" {
		t.Fatalf("diff %+v", d)
	}
	want := []DiffRow{{Name: "c", Kind: "added"}, {Name: "a", Kind: "updated"}, {Name: "b", Kind: "removed"}}
	if !slices.Equal(d.Rows, want) {
		t.Fatalf("rows %+v", d.Rows)
	}
	saves := filepath.Join(game, "saves", "World")
	_ = os.MkdirAll(saves, 0o700)
	_ = os.WriteFile(filepath.Join(saves, "level.dat"), []byte("lvl"), 0o600)
	job, err := env.svc.Update(ctx, in.ID, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.eng.Wait(ctx, job); err != nil {
		t.Fatal(err)
	}
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(game, rel)) //nolint:gosec // test
		if err != nil {
			return "<missing>"
		}
		return string(b)
	}
	for rel, want := range map[string]string{"mods/a.jar.disabled": "A2", "mods/a.jar": "<missing>", "mods/b.jar": "<missing>", "mods/c.jar": "C1",
		"config/x.toml": "x2", "config/y.toml": "mine", "config/z.toml": "z"} {
		if got := read(rel); got != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
	if in, _ := env.insts.Get(in.ID); in.Pack.VersionName != "2.0" || in.Pack.Version != "V2" {
		t.Fatalf("pack %+v", in.Pack)
	}
	if backups, _ := os.ReadDir(filepath.Join(env.insts.Dir(in.ID), "backups")); len(backups) != 1 {
		t.Fatal("no world backup")
	}
	if len(env.svc.Updates()) != 0 {
		t.Fatal("the update is still offered")
	}
}

func sumOf(b []byte) string {
	s, _ := hashes(b)
	return s
}

func TestStem(t *testing.T) {
	for in, want := range map[string]string{"mods/create-1.20.1-0.5.1.f.jar": "create", "mods/jei-1.20.1-forge-15.2.0.27.jar.disabled": "jei",
		"mods/Xaeros_Minimap_24.0_Forge_1.20.jar": "xaeros_minimap", "mods/sodium.jar": "sodium"} {
		if got := stem(in); got != want {
			t.Errorf("%s → %s", in, got)
		}
	}
}

func TestUpdateRollsBackOnFailure(t *testing.T) {
	ctx, env, in, game := packUpdate(t)
	// b.jar can't be set aside: a folder sits where its old copy would go.
	_ = os.MkdirAll(filepath.Join(game, "mods", "b.jar"+oldSuffix, "x"), 0o700)
	if av := env.svc.CheckUpdates(ctx); len(av) != 1 {
		t.Fatalf("available %+v", av)
	}
	job, err := env.svc.Update(ctx, in.ID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.eng.Wait(ctx, job); err == nil {
		t.Fatal("the update succeeded")
	}
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(game, rel)) //nolint:gosec // test
		if err != nil {
			return "<missing>"
		}
		return string(b)
	}
	for rel, want := range map[string]string{"mods/a.jar.disabled": "A1", "mods/b.jar": "B1", "mods/c.jar": "<missing>",
		"config/x.toml": "x1", "config/y.toml": "mine", "config/z.toml": "<missing>", "config/x.toml" + newSuffix: "<missing>"} {
		if got := read(rel); got != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
	m, err := ReadManifest(env.insts.Dir(in.ID))
	if err != nil || m.Version != "1.0" {
		t.Fatalf("manifest %+v %v", m, err)
	}
	if in, _ := env.insts.Get(in.ID); in.Pack.Version != "V1" {
		t.Fatalf("pack %+v", in.Pack)
	}
}

func TestNewestStaysOnTheInstancesLine(t *testing.T) {
	plat := packs{versions: []modplatform.Version{
		{ID: "V4", Number: "7.0 (1.21)", Type: "release", Game: []string{"1.21.1"}, Loaders: []string{"fabric"}},
		{ID: "V3", Number: "6.2 beta", Type: "beta", Game: []string{"1.20.1"}, Loaders: []string{"fabric"}},
		{ID: "V2", Number: "6.1", Type: "release", Game: []string{"1.20.1"}, Loaders: []string{"fabric"}},
		{ID: "V1", Number: "6.0", Type: "release", Game: []string{"1.20.1"}, Loaders: []string{"fabric"}},
	}}
	s := &Service{d: InstallDeps{Reader: NewReader(Deps{Modrinth: plat})}}
	in := instance.Instance{Minecraft: "1.20.1", Loader: "fabric"}
	id, name, err := s.newest(context.Background(), Origin{Source: "modrinth", Project: "PACK", Version: "V1"}, in)
	if err != nil || id != "V2" || name != "6.1" {
		t.Fatalf("offered %q %q %v", id, name, err)
	}
	if id, _, _ := s.newest(context.Background(), Origin{Source: "modrinth", Project: "PACK", Version: "V2"}, in); id != "" {
		t.Fatalf("offered %q to an instance on the newest release of its line", id)
	}
	if onLine(modplatform.Version{Game: []string{"1.20.1"}, Loaders: []string{"forge"}}, in) {
		t.Fatal("a Forge build is on a Fabric instance's line")
	}
	if !onLine(modplatform.Version{Game: []string{"1.20.1"}, Loaders: []string{"mrpack"}}, in) {
		t.Fatal("a version naming no loader was ruled out")
	}
}
