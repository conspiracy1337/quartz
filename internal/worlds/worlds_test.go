package worlds

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"quartz/internal/core/localimg"
	"quartz/internal/core/nbt"
	"quartz/internal/instance"
	"quartz/internal/launch"
)

type env struct {
	svc   *Service
	insts *instance.Store
	id    string
	other string
	busy  bool
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	insts, err := instance.Open(filepath.Join(dir, "instances"), nil)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := insts.Create(instance.CreateRequest{Name: "A", Minecraft: "1.21.4"})
	b, _ := insts.Create(instance.CreateRequest{Name: "B", Minecraft: "1.21.4"})
	e := &env{insts: insts, id: a.ID, other: b.ID}
	e.svc = New(Deps{Instances: insts, Images: localimg.New(filepath.Join(dir, "thumbs")), Busy: func(string) bool { return e.busy }})
	return e
}

// level writes a 1.21-style level.dat.
func level(t *testing.T, dir, name string, gameType int32, hardcore bool, seed int64) {
	t.Helper()
	data := nbt.NewCompound()
	data.Comp.Set("LevelName", nbt.NewString(name))
	data.Comp.Set("GameType", nbt.NewInt(gameType))
	hc := int8(0)
	if hardcore {
		hc = 1
	}
	data.Comp.Set("hardcore", nbt.NewByte(hc))
	data.Comp.Set("LastPlayed", nbt.NewLong(time.Now().UnixMilli()))
	ver := nbt.NewCompound()
	ver.Comp.Set("Name", nbt.NewString("1.21.4"))
	data.Comp.Set("Version", ver)
	wg := nbt.NewCompound()
	wg.Comp.Set("seed", nbt.NewLong(seed))
	data.Comp.Set("WorldGenSettings", wg)
	root := nbt.NewCompound()
	root.Comp.Set("Data", data)
	_ = os.MkdirAll(filepath.Join(dir, "region"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "region", "r.0.0.mca"), make([]byte, 4096), 0o644)
	if err := nbt.WriteFile(filepath.Join(dir, "level.dat"), &nbt.File{Root: root, Gzip: true}); err != nil {
		t.Fatal(err)
	}
}

func (e *env) world(t *testing.T, folder, name string, gameType int32, hardcore bool) {
	level(t, filepath.Join(e.insts.GameDir(e.id), "saves", folder), name, gameType, hardcore, -4172144997902289642)
}

func TestListReadsLevelDat(t *testing.T) {
	e := newEnv(t)
	e.world(t, "New World", "Northwatch", 0, false)
	e.world(t, "c", "Creative Plots", 1, false)
	e.world(t, "hc", "The Long Winter", 0, true)
	_ = os.MkdirAll(filepath.Join(e.insts.GameDir(e.id), "saves", "not a world"), 0o755)
	ws, err := e.svc.List(context.Background(), e.id)
	if err != nil || len(ws) != 3 {
		t.Fatalf("%v %v", ws, err)
	}
	modes := map[string]string{}
	for _, w := range ws {
		modes[w.Name] = w.Mode
		if w.Bytes < 4096 || w.Version != "1.21.4" {
			t.Errorf("%+v", w)
		}
	}
	if modes["Northwatch"] != "Survival" || modes["Creative Plots"] != "Creative" || modes["The Long Winter"] != "Hardcore" {
		t.Fatalf("modes %v", modes)
	}
	if e.svc.Count(e.id) != 3 {
		t.Fatal("count")
	}
	if seed, _ := e.svc.Seed(e.id, "New World"); seed != "-4172144997902289642" {
		t.Fatalf("seed %q", seed)
	}
}

func TestRenameKeepsFolder(t *testing.T) {
	e := newEnv(t)
	e.world(t, "New World", "Northwatch", 0, false)
	if err := e.svc.Rename(e.id, "New World", "  North watch  "); err != nil {
		t.Fatal(err)
	}
	ws, _ := e.svc.List(context.Background(), e.id)
	if ws[0].Folder != "New World" || ws[0].Name != "North watch" {
		t.Fatalf("%+v", ws[0])
	}
	if seed, _ := e.svc.Seed(e.id, "New World"); seed != "-4172144997902289642" {
		t.Fatal("rename lost the seed")
	}
	e.busy = true
	if err := e.svc.Rename(e.id, "New World", "x"); !errors.Is(err, ErrBusy) {
		t.Fatal("rename while running")
	}
	e.busy = false
	for _, bad := range []string{"", "a\nb"} {
		if err := e.svc.Rename(e.id, "New World", bad); !errors.Is(err, ErrName) {
			t.Errorf("%q accepted", bad)
		}
	}
	if err := e.svc.Rename(e.id, "../New World", "x"); !errors.Is(err, ErrNoWorld) {
		t.Fatal("traversal accepted")
	}
}

func TestDeleteAndUndo(t *testing.T) {
	e := newEnv(t)
	e.world(t, "w", "W", 0, false)
	token, err := e.svc.Delete(e.id, "w")
	if err != nil || e.svc.Count(e.id) != 0 {
		t.Fatalf("delete: %v", err)
	}
	if err := e.svc.Undo(e.id, token); err != nil || e.svc.Count(e.id) != 1 {
		t.Fatalf("undo: %v", err)
	}
}

func TestExportImportCopy(t *testing.T) {
	e := newEnv(t)
	e.world(t, "w", "Northwatch", 0, false)
	zipPath := filepath.Join(t.TempDir(), "Northwatch.zip")
	if err := e.svc.Export(e.id, "w", zipPath); err != nil {
		t.Fatal(err)
	}
	// Importing the export twice gives "w" and "w (2)".
	for _, want := range []string{"w (2)", "w (3)"} {
		got, err := e.svc.Import(e.id, zipPath)
		if err != nil || got != want {
			t.Fatalf("import %q %v", got, err)
		}
	}
	got, err := e.svc.CopyTo(e.id, "w", e.other)
	if err != nil || got != "w" {
		t.Fatalf("copy %q %v", got, err)
	}
	if e.svc.Count(e.other) != 1 {
		t.Fatal("copy missing")
	}
	// The copy is private: editing it leaves the original alone.
	_ = os.WriteFile(filepath.Join(e.insts.GameDir(e.other), "saves", "w", "region", "r.0.0.mca"), []byte("x"), 0o644)
	if fi, _ := os.Stat(filepath.Join(e.insts.GameDir(e.id), "saves", "w", "region", "r.0.0.mca")); fi.Size() != 4096 {
		t.Fatal("copy shares data with the original")
	}
	// A folder import.
	src := filepath.Join(t.TempDir(), "Island")
	level(t, src, "Island", 0, false, 1)
	if got, err := e.svc.Import(e.id, src); err != nil || got != "Island" {
		t.Fatalf("folder import %q %v", got, err)
	}
}

func TestImportRefusesHostileZips(t *testing.T) {
	e := newEnv(t)
	mk := func(entries map[string]string) string {
		p := filepath.Join(t.TempDir(), "w.zip")
		f, _ := os.Create(p)
		zw := zip.NewWriter(f)
		for n, body := range entries {
			w, _ := zw.Create(n)
			_, _ = w.Write([]byte(body))
		}
		_ = zw.Close()
		_ = f.Close()
		return p
	}
	if _, err := e.svc.Import(e.id, mk(map[string]string{"readme.txt": "x"})); !errors.Is(err, ErrNotWorld) {
		t.Fatalf("no level.dat: %v", err)
	}
	if _, err := e.svc.Import(e.id, mk(map[string]string{"a/b/c/d/level.dat": "x"})); !errors.Is(err, ErrNotWorld) {
		t.Fatalf("buried level.dat: %v", err)
	}
	if _, err := e.svc.Import(e.id, mk(map[string]string{"W/level.dat": "x", "W/../../../evil.txt": "x"})); err == nil {
		t.Fatal("traversal accepted")
	}
	if _, err := os.Stat(filepath.Join(e.insts.GameDir(e.id), "evil.txt")); err == nil {
		t.Fatal("file escaped")
	}
	if entries, _ := os.ReadDir(filepath.Join(e.insts.GameDir(e.id), ".quartz")); len(entries) != 0 {
		t.Fatalf("staging left behind: %v", entries)
	}
}

func TestFinishRunNamesTheWorld(t *testing.T) {
	e := newEnv(t)
	started := time.Now().Add(-time.Minute)
	e.world(t, "old", "Old", 0, false)
	old := filepath.Join(e.insts.GameDir(e.id), "saves", "old", "level.dat")
	_ = os.Chtimes(old, started.Add(-time.Hour), started.Add(-time.Hour))
	e.world(t, "nw", "Northwatch", 0, false)
	rec := launch.Record{}
	e.svc.FinishRun(&launch.Finished{InstanceID: e.id, GameDir: e.insts.GameDir(e.id), Started: started, Record: &rec})
	if rec.World != "Northwatch" {
		t.Fatalf("world %q", rec.World)
	}
}

// Newer versions keep the seed in data/minecraft/world_gen_settings.dat.
func TestSeedFromWorldGenSettingsFile(t *testing.T) {
	e := newEnv(t)
	dir := filepath.Join(e.insts.GameDir(e.id), "saves", "New World")
	e.world(t, "New World", "Northwatch", 0, false)
	f, err := nbt.ReadFile(filepath.Join(dir, "level.dat"))
	if err != nil {
		t.Fatal(err)
	}
	f.Root.Get("Data").Comp.Delete("WorldGenSettings")
	if err := nbt.WriteFile(filepath.Join(dir, "level.dat"), f); err != nil {
		t.Fatal(err)
	}
	data := nbt.NewCompound()
	data.Comp.Set("seed", nbt.NewLong(8675309))
	root := nbt.NewCompound()
	root.Comp.Set("data", data)
	gen := filepath.Join(dir, "data", "minecraft", "world_gen_settings.dat")
	_ = os.MkdirAll(filepath.Dir(gen), 0o755)
	if err := nbt.WriteFile(gen, &nbt.File{Root: root, Gzip: true}); err != nil {
		t.Fatal(err)
	}
	if seed, err := e.svc.Seed(e.id, "New World"); err != nil || seed != "8675309" {
		t.Fatalf("seed %q %v", seed, err)
	}
}
