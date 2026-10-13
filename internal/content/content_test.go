package content

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"quartz/internal/content/packwiz"
	"quartz/internal/content/store"
	"quartz/internal/core/events"
	"quartz/internal/core/optfile"
	"quartz/internal/icons"
	"quartz/internal/instance"
	"quartz/internal/modplatform"
)

type env struct {
	t       *testing.T
	svc     *Service
	id      string
	game    string
	insts   *instance.Store
	running bool
	events  []Changed
	mu      sync.Mutex
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	log := slog.New(slog.DiscardHandler)
	insts, err := instance.Open(filepath.Join(root, "instances"), log)
	if err != nil {
		t.Fatal(err)
	}
	in, err := insts.Create(instance.CreateRequest{Name: "Test", Minecraft: "1.21.1", Loader: "fabric", LoaderVersion: "0.16.9"}, instance.JavaSettings{})
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(root, "store", "content"), nil, log)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, id: in.ID, game: insts.GameDir(in.ID), insts: insts}
	bus := events.NewBus()
	bus.Subscribe(events.ContentChanged, func(p any) {
		e.mu.Lock()
		e.events = append(e.events, p.(Changed))
		e.mu.Unlock()
	})
	clientJar := filepath.Join(root, "client.jar")
	writeZip(t, clientJar, map[string]string{"version.json": `{"world_version":3955,"pack_version":{"resource":34,"data":48}}`})
	e.svc, err = New(Deps{Instances: insts, Store: st, Icons: icons.OpenSized(filepath.Join(root, "icons"), 64), Bus: bus,
		Running: func(string) bool { return e.running }, ClientJar: func(string) string { return clientJar }, Log: log},
		filepath.Join(root, "store", "content"))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func writeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}
	_ = zw.Close()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (e *env) mod(file, id, name string, extra string) {
	e.t.Helper()
	writeZip(e.t, filepath.Join(e.game, "mods", file), map[string]string{"fabric.mod.json": `{"id":"` + id + `","name":"` + name + `","version":"1.0","authors":["Dev"]` + extra + `}`})
}

func (e *env) list(k Kind) []Row {
	e.t.Helper()
	rows, err := e.svc.List(context.Background(), e.id, k)
	if err != nil {
		e.t.Fatal(err)
	}
	return rows
}

func byName(rows []Row, name string) Row {
	for _, r := range rows {
		if r.Name == name {
			return r
		}
	}
	return Row{}
}

func TestListModsWithMetadataAndProblems(t *testing.T) {
	e := newEnv(t)
	e.mod("sodium.jar", "sodium", "Sodium", "")
	e.mod("extra.jar", "sodium-extra", "Sodium Extra", `,"depends":{"sodium":"*","reeses-sodium-options":"*"}`)
	e.mod("dup-a.jar", "lithium", "Lithium", "")
	e.mod("dup-b.jar", "lithium", "Lithium", "")
	e.mod("lib.jar", "unusedlib", "Unused Lib", `,"custom":{"modmenu":{"badges":["library"]}}`)
	writeZip(t, filepath.Join(e.game, "mods", "notamod.jar"), map[string]string{"a.class": "x"})
	_ = os.WriteFile(filepath.Join(e.game, "mods", "downloading.jar.part"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(e.game, "mods", "readme.txt"), []byte("x"), 0o600)
	rows := e.list(Mods)
	if len(rows) != 6 {
		t.Fatalf("%d rows: %+v", len(rows), rows)
	}
	if r := byName(rows, "Sodium"); r.Sub != "Dev" || r.Version != "1.0" || r.Source != "file" || !r.Enabled || r.Status != nil {
		t.Fatalf("sodium %+v", r)
	}
	if r := byName(rows, "Sodium Extra"); r.Status == nil || r.Status.Label != "Needs reeses-sodium-options" {
		t.Fatalf("missing dep %+v", r.Status)
	}
	if r := byName(rows, "Lithium"); r.Status == nil || r.Status.Label != "Duplicate" {
		t.Fatalf("dup %+v", r.Status)
	}
	if r := byName(rows, "Unused Lib"); r.Status == nil || r.Status.Tone != "info" {
		t.Fatalf("library %+v", r.Status)
	}
	if r := byName(rows, "notamod"); r.ID == "" {
		t.Fatal("a jar without metadata is still listed by file name")
	}
	if m := e.svc.Missing(); len(m) != 1 || m[0].Mod != "Sodium Extra" || m[0].Dep != "reeses-sodium-options" {
		t.Fatalf("alerts %+v", m)
	}
	if c := e.svc.Counts(e.id); c[Mods] != 6 {
		t.Fatalf("counts %v", c)
	}
}

func TestScanAdoptsIntoTheStoreAndReusesStats(t *testing.T) {
	e := newEnv(t)
	e.mod("a.jar", "a", "A", "")
	rows := e.list(Mods)
	ix, _ := e.svc.idx.get(e.id)
	entry := ix.Get().Entries[rows[0].ID]
	blob := e.svc.d.Store.Path(entry.SHA1)
	a, _ := os.Stat(filepath.Join(e.game, "mods", "a.jar"))
	b, _ := os.Stat(blob)
	if !os.SameFile(a, b) {
		t.Fatal("the mod was not adopted into the store")
	}
	e.events = nil
	e.list(Mods)
	if len(e.events) != 0 {
		t.Fatal("an unchanged folder must not emit")
	}
}

func TestInPlaceFilesAreLeftAlone(t *testing.T) {
	e := newEnv(t)
	if _, err := e.insts.Update(e.id, func(in *instance.Instance) error { in.InPlace = true; return nil }); err != nil {
		t.Fatal(err)
	}
	e.mod("a.jar", "a", "A", "")
	rows := e.list(Mods)
	if len(rows) != 1 {
		t.Fatalf("%+v", rows)
	}
	ix, _ := e.svc.idx.get(e.id)
	blob := e.svc.d.Store.Path(ix.Get().Entries[rows[0].ID].SHA1)
	a, _ := os.Stat(filepath.Join(e.game, "mods", "a.jar"))
	if b, err := os.Stat(blob); err == nil && os.SameFile(a, b) {
		t.Fatal("another launcher's file was linked into the store")
	}
	if a.Mode().Perm()&0o200 == 0 {
		t.Fatal("another launcher's file was made read-only")
	}
}

func TestToggleModBySuffix(t *testing.T) {
	e := newEnv(t)
	e.mod("a.jar", "a", "A", "")
	r := e.list(Mods)[0]
	ctx := context.Background()
	if err := e.svc.SetEnabled(ctx, e.id, Mods, r.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.game, "mods", "a.jar.disabled")); err != nil {
		t.Fatal("not renamed")
	}
	rows := e.list(Mods)
	if len(rows) != 1 || rows[0].ID != r.ID || rows[0].Enabled {
		t.Fatalf("%+v", rows)
	}
	if err := e.svc.SetEnabled(ctx, e.id, Mods, r.ID, true); err != nil {
		t.Fatal(err)
	}
	if !e.list(Mods)[0].Enabled {
		t.Fatal("not enabled again")
	}
}

func TestResourcePackOrderWritesOptions(t *testing.T) {
	e := newEnv(t)
	for _, n := range []string{"a.zip", "b.zip", "old.zip"} {
		format := "34"
		if n == "old.zip" {
			format = "15"
		}
		writeZip(t, filepath.Join(e.game, "resourcepacks", n), map[string]string{"pack.mcmeta": `{"pack":{"pack_format":` + format + `,"description":"Pack ` + n + `"}}`})
	}
	opts := "version:3955\nkeyAttack:key.mouse.left\nresourcePacks:[\"vanilla\",\"fabric\",\"file/a.zip\"]\nlang:en_us\n"
	_ = os.WriteFile(filepath.Join(e.game, "options.txt"), []byte(opts), 0o600)
	rows := e.list(ResourcePacks)
	id := map[string]string{}
	for _, r := range rows {
		id[strings.TrimPrefix(r.Sub, "Pack ")] = r.ID
	}
	if !byName(rows, "a").Enabled || byName(rows, "b").Enabled {
		t.Fatalf("%+v", rows)
	}
	if r := byName(rows, "old"); r.Status == nil || r.Status.Label != "Made for 1.20" {
		t.Fatalf("format label %+v", r.Status)
	}
	if err := e.svc.SetPackOrder(context.Background(), e.id, []string{id["old.zip"], id["a.zip"]}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(e.game, "options.txt")) //nolint:gosec // test temp dir
	want := "version:3955\nkeyAttack:key.mouse.left\nresourcePacks:[\"vanilla\",\"fabric\",\"file/a.zip\",\"file/old.zip\"]\nlang:en_us\nincompatibleResourcePacks:[\"file/old.zip\"]\n"
	if string(got) != want {
		t.Fatalf("options.txt:\n%s\nwant:\n%s", got, want)
	}
	rows = e.list(ResourcePacks)
	if rows[0].Name != "old" || rows[1].Name != "a" || rows[2].Enabled {
		t.Fatalf("order %+v", rows)
	}
}

func TestPackEditsWaitForTheGame(t *testing.T) {
	e := newEnv(t)
	writeZip(t, filepath.Join(e.game, "resourcepacks", "a.zip"), map[string]string{"pack.mcmeta": `{"pack":{"pack_format":34}}`})
	r := e.list(ResourcePacks)[0]
	e.running = true
	if err := e.svc.SetEnabled(context.Background(), e.id, ResourcePacks, r.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.game, "options.txt")); err == nil {
		t.Fatal("options.txt written while the game runs")
	}
	if !e.list(ResourcePacks)[0].Enabled {
		t.Fatal("the queued state should show")
	}
	e.running = false
	e.svc.GameExited(e.id)
	got, _ := os.ReadFile(filepath.Join(e.game, "options.txt")) //nolint:gosec // test temp dir
	if !strings.Contains(string(got), `resourcePacks:["vanilla","file/a.zip"]`) || !strings.Contains(string(got), "version:3955") {
		t.Fatalf("%s", got)
	}
}

func TestActiveShader(t *testing.T) {
	e := newEnv(t)
	writeZip(t, filepath.Join(e.game, "shaderpacks", "Complementary r5.zip"), map[string]string{"shaders/composite.fsh": "x"})
	writeZip(t, filepath.Join(e.game, "shaderpacks", "BSL.zip"), map[string]string{"shaders/composite.fsh": "x"})
	writeZip(t, filepath.Join(e.game, "shaderpacks", "notashader.zip"), map[string]string{"x": "x"})
	_ = os.MkdirAll(filepath.Join(e.game, "config"), 0o700)
	_ = os.WriteFile(filepath.Join(e.game, "config", "iris.properties"), []byte("#Iris\nmaxShadowRenderDistance=32\nshaderPack=BSL.zip\nenableShaders=true\n"), 0o600)
	rows := e.list(Shaders)
	if len(rows) != 3 || !byName(rows, "BSL").Enabled || byName(rows, "Complementary r5").Enabled {
		t.Fatalf("%+v", rows)
	}
	if err := e.svc.SetActiveShader(context.Background(), e.id, byName(rows, "Complementary r5").ID); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(e.game, "config", "iris.properties")) //nolint:gosec // test temp dir
	if string(got) != "#Iris\nmaxShadowRenderDistance=32\nshaderPack=Complementary\\ r5.zip\nenableShaders=true\n" &&
		string(got) != "#Iris\nmaxShadowRenderDistance=32\nshaderPack=Complementary r5.zip\nenableShaders=true\n" {
		t.Fatalf("%q", got)
	}
	if !byName(e.list(Shaders), "Complementary r5").Enabled {
		t.Fatal("not active")
	}
	_ = e.svc.SetActiveShader(context.Background(), e.id, "")
	for _, r := range e.list(Shaders) {
		if r.Enabled {
			t.Fatal("shaders should be off")
		}
	}
}

func TestDataPacksByWorld(t *testing.T) {
	e := newEnv(t)
	writeZip(t, filepath.Join(e.game, "saves", "North", "datapacks", "terralith.zip"), map[string]string{"pack.mcmeta": `{"pack":{"pack_format":48,"description":"Terrain"}}`})
	_ = os.MkdirAll(filepath.Join(e.game, "saves", "Plots", "datapacks", "graves", "data"), 0o700)
	_ = os.WriteFile(filepath.Join(e.game, "saves", "Plots", "datapacks", "graves", "pack.mcmeta"), []byte(`{"pack":{"pack_format":48,"description":"Graves"}}`), 0o600)
	rows := e.list(DataPacks)
	if len(rows) != 2 || rows[0].Group != "North" || rows[1].Group != "Plots" || rows[1].Status.World != "Plots" {
		t.Fatalf("%+v", rows)
	}
	if info := e.svc.Info(e.id); len(info.Worlds) != 2 {
		t.Fatalf("%+v", info)
	}
}

func TestRemoveAndUndo(t *testing.T) {
	e := newEnv(t)
	e.mod("a.jar", "a", "A", "")
	r := e.list(Mods)[0]
	ctx := context.Background()
	token, err := e.svc.Remove(ctx, e.id, Mods, []string{r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.list(Mods)) != 0 {
		t.Fatal("still listed")
	}
	if act := e.svc.Activity(e.id); len(act) != 1 || act[0].Title != "Removed A" {
		t.Fatalf("%+v", act)
	}
	if err := e.svc.Undo(ctx, e.id, token); err != nil {
		t.Fatal(err)
	}
	if rows := e.list(Mods); len(rows) != 1 || rows[0].ID != r.ID {
		t.Fatalf("%+v", rows)
	}
	if err := e.svc.Undo(ctx, e.id, token); err == nil {
		t.Fatal("a second undo must fail")
	}
}

func TestSymlinksAreNeverFollowed(t *testing.T) {
	e := newEnv(t)
	outside := t.TempDir()
	writeZip(t, filepath.Join(outside, "evil.jar"), map[string]string{"fabric.mod.json": `{"id":"evil"}`})
	_ = os.MkdirAll(filepath.Join(e.game, "mods"), 0o700)
	if err := os.Symlink(filepath.Join(outside, "evil.jar"), filepath.Join(e.game, "mods", "evil.jar")); err != nil {
		t.Skip("no symlinks here")
	}
	if rows := e.list(Mods); len(rows) != 0 {
		t.Fatalf("followed a link: %+v", rows)
	}
	_ = os.RemoveAll(filepath.Join(e.game, "resourcepacks"))
	if err := os.Symlink(outside, filepath.Join(e.game, "resourcepacks")); err == nil {
		if rows := e.list(ResourcePacks); len(rows) != 0 {
			t.Fatal("followed a linked folder out of the instance")
		}
	}
}

func TestIdentifyLockAndMetadata(t *testing.T) {
	e := newEnv(t)
	e.mod("sodium.jar", "sodium", "Sodium", "")
	e.mod("local.jar", "local", "Local", "")
	rows := e.list(Mods)
	ix, _ := e.svc.idx.get(e.id)
	sodiumSHA := ix.Get().Entries[byName(rows, "Sodium").ID].SHA1
	e.svc.d.Platforms = []modplatform.Provider{&fakePlatform{src: modplatform.Modrinth,
		byHash:   map[string]modplatform.Version{sodiumSHA: {Source: modplatform.Modrinth, ProjectID: "AANobbMI", ID: "v1", Number: "0.6.5", Type: "release"}},
		projects: map[string]modplatform.Project{"AANobbMI": {Slug: "sodium", Title: "Sodium", Author: "CaffeineMC"}}}}
	n, err := e.svc.Identify(context.Background(), e.id, Mods)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	rows = e.list(Mods)
	s := byName(rows, "Sodium")
	if s.Source != "modrinth" || s.Sub != "CaffeineMC" || s.Version != "0.6.5" || s.Project != "https://modrinth.com/project/sodium" {
		t.Fatalf("%+v", s)
	}
	if l := byName(rows, "Local"); l.Status == nil || l.Status.Kind != "unknown" {
		t.Fatalf("unknown %+v", l)
	}
	pws, _ := packwiz.ReadDir(filepath.Join(e.game, "mods", ".index"))
	if pw, ok := pws["sodium.jar"]; !ok || pw.Modrinth == nil || pw.Modrinth.ModID != "AANobbMI" || pw.Download.HashFormat != "sha512" {
		t.Fatalf("metadata %+v", pws)
	}
	if err := e.svc.SetLocked(e.id, s.ID, true); err != nil {
		t.Fatal(err)
	}
	if r := byName(e.list(Mods), "Sodium"); !r.Locked || r.Status == nil || r.Status.Kind != "locked" {
		t.Fatalf("%+v", r)
	}
	if err := e.svc.SetLocked(e.id, byName(rows, "Local").ID, true); err == nil {
		t.Fatal("unidentified items can't be locked")
	}
}

func TestRefsCoverEntriesAndRemovals(t *testing.T) {
	e := newEnv(t)
	e.mod("a.jar", "a", "A", "")
	e.mod("b.jar", "b", "B", "")
	rows := e.list(Mods)
	if _, err := e.svc.Remove(context.Background(), e.id, Mods, []string{rows[0].ID}); err != nil {
		t.Fatal(err)
	}
	n := 0
	if err := e.svc.Refs(func(string) { n++ }); err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestQuick(t *testing.T) {
	e := newEnv(t)
	e.mod("a.jar", "a", "A", "")
	e.mod("b.jar", "b", "B", `,"depends":{"fabric-api":"*"}`)
	n, detail, warn, err := e.svc.Quick(context.Background(), e.id)
	if err != nil || n != 2 || detail != "B needs Fabric API" || !warn {
		t.Fatalf("%d %q %v %v", n, detail, warn, err)
	}
}

func TestConfigEditsKeepOtherBytes(t *testing.T) {
	l := optfile.File{Sep: "=", Text: []string{"# comment", "a=1", "shaderPack=old"}, CRLF: true, Exist: true}
	setShader(&l, "Ünï code.zip")
	if got, _ := l.Get("shaderPack"); got != "Ünï code.zip" {
		t.Fatalf("%q", got)
	}
	if l.Text[0] != "# comment" || l.Text[1] != "a=1" || l.Text[2] != "shaderPack=\\u00dcn\\u00ef code.zip" {
		t.Fatalf("%q", l.Text)
	}
}

func TestAddDropped(t *testing.T) {
	e := newEnv(t)
	src := t.TempDir()
	mod := filepath.Join(src, "sodium.jar")
	writeZip(t, mod, map[string]string{"fabric.mod.json": `{"id":"sodium","name":"Sodium"}`})
	notMod := filepath.Join(src, "random.jar")
	writeZip(t, notMod, map[string]string{"a.class": "x"})
	pack := filepath.Join(src, "pack.mrpack")
	_ = os.WriteFile(pack, []byte("PK\x03\x04"), 0o600)
	txt := filepath.Join(src, "notes.txt")
	_ = os.WriteFile(txt, []byte("x"), 0o600)
	link := filepath.Join(src, "link.jar")
	_ = os.Symlink(mod, link)
	res, err := e.svc.Add(context.Background(), e.id, Mods, "", []string{mod, notMod, pack, txt, link, "relative.jar"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Added) != 1 || res.Added[0] != "sodium.jar" || len(res.Rejected) != 5 {
		t.Fatalf("%+v", res)
	}
	if res.Rejected[1].Reason != "It's a modpack: import it from the Library" {
		t.Fatalf("%+v", res.Rejected)
	}
	if rows := e.list(Mods); len(rows) != 1 || rows[0].Name != "Sodium" {
		t.Fatalf("%+v", rows)
	}
	again, _ := e.svc.Add(context.Background(), e.id, Mods, "", []string{mod})
	if len(again.Rejected) != 1 || again.Rejected[0].Reason != "A file with that name is already there" {
		t.Fatalf("%+v", again)
	}
	// The dropped file itself was never linked into the store.
	a, _ := os.Stat(mod)
	b, _ := os.Stat(filepath.Join(e.game, "mods", "sodium.jar"))
	if os.SameFile(a, b) {
		t.Fatal("the user's own file was linked")
	}
}

func TestAddDataPackFolderToAWorld(t *testing.T) {
	e := newEnv(t)
	_ = os.MkdirAll(filepath.Join(e.game, "saves", "North"), 0o700)
	src := filepath.Join(t.TempDir(), "graves")
	_ = os.MkdirAll(filepath.Join(src, "data"), 0o700)
	_ = os.WriteFile(filepath.Join(src, "pack.mcmeta"), []byte(`{"pack":{"pack_format":48}}`), 0o600)
	if _, err := e.svc.Add(context.Background(), e.id, DataPacks, "", []string{src}); !errors.Is(err, ErrNoWorld) {
		t.Fatalf("err = %v", err)
	}
	res, err := e.svc.Add(context.Background(), e.id, DataPacks, "North", []string{src})
	if err != nil || len(res.Added) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(e.game, "saves", "North", "datapacks", "graves", "pack.mcmeta")); err != nil {
		t.Fatal(err)
	}
}

func TestSharedFolderItems(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	shared := t.TempDir()
	// The instance shares its resource packs (package syncing's resolver).
	e.insts.SetLocator(func(_, rel string) string {
		if rel == "resourcepacks" || strings.HasPrefix(rel, "resourcepacks/") {
			return shared
		}
		return ""
	})
	writeZip(t, filepath.Join(shared, "resourcepacks", "faithful.zip"), map[string]string{"pack.mcmeta": `{"pack":{"pack_format":34,"description":"Faithful"}}`})
	e.mod("sodium.jar", "sodium", "Sodium", "")
	rows := e.list(ResourcePacks)
	if len(rows) != 1 || len(e.list(Mods)) != 1 {
		t.Fatalf("packs %v", rows)
	}
	token, err := e.svc.Remove(ctx, e.id, ResourcePacks, []string{rows[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(shared, "resourcepacks", "faithful.zip")); !os.IsNotExist(err) {
		t.Fatal("not removed from the shared folder")
	}
	if held, _ := filepath.Glob(filepath.Join(shared, ".quartz", "removed", "*")); len(held) == 0 {
		t.Fatal("held outside the shared folder")
	}
	if err := e.svc.Undo(ctx, e.id, token); err != nil {
		t.Fatal(err)
	}
	if len(e.list(ResourcePacks)) != 1 {
		t.Fatal("undo")
	}
	if !e.svc.AnyHasMod("SODIUM") || e.svc.AnyHasMod("xaerominimap") {
		t.Fatal("AnyHasMod")
	}
}
