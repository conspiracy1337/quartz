package syncing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"quartz/internal/core/events"
	"quartz/internal/instance"
	"quartz/internal/settings"
)

type env struct {
	t        *testing.T
	dir      string
	svc      *Service
	insts    *instance.Store
	a, b     string
	running  map[string]bool
	hasMod   bool
	retitled []string
	client   string              // the client jar every version uses
	modJars  map[string][]ModJar // by instance
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	insts, err := instance.Open(filepath.Join(dir, "instances"), nil)
	if err != nil {
		t.Fatal(err)
	}
	st, err := settings.Open(filepath.Join(dir, "settings.json"), events.NewBus(), nil)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := insts.Create(instance.CreateRequest{Name: "A", Minecraft: "1.21.4"})
	b, _ := insts.Create(instance.CreateRequest{Name: "B", Minecraft: "1.12.2"})
	e := &env{t: t, dir: dir, insts: insts, a: a.ID, b: b.ID, running: map[string]bool{}}
	e.svc = New(Config{
		Root: filepath.Join(dir, "shared"), Trash: filepath.Join(dir, "trash"), Settings: st, Instances: insts,
		Running: func(id string) bool { return e.running[id] },
		HasMod:  func(...string) bool { return e.hasMod },
		Retitle: func(dir, suffix string) error {
			e.retitled = append(e.retitled, filepath.Base(dir)+"|"+suffix)
			return nil
		},
		ClientJar: func(string) string { return e.client },
		ModJars:   func(id string) []ModJar { return e.modJars[id] },
	})
	return e
}

func (e *env) game(id string, rel ...string) string {
	return filepath.Join(append([]string{e.insts.GameDir(id)}, rel...)...)
}

func (e *env) shared(rel ...string) string {
	return filepath.Join(append([]string{e.dir, "shared"}, rel...)...)
}

func put(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func body(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func (e *env) enable(id string) {
	e.t.Helper()
	if err := e.svc.SetEnabled(context.Background(), id, true); err != nil {
		e.t.Fatal(err)
	}
}

func TestJoinMergesItemsIn(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable("saves")
	put(t, e.shared("saves", "Same", "level.dat"), "same")
	put(t, e.shared("saves", "Clash", "level.dat"), "theirs")
	put(t, e.game(e.a, "saves", "Mine", "level.dat"), "mine")
	put(t, e.game(e.a, "saves", "Clash", "level.dat"), "ours")
	// An identical copy (same files, sizes and times) is merged once.
	put(t, e.game(e.a, "saves", "Same", "level.dat"), "same")
	at := time.Now().Add(-time.Hour)
	_ = os.Chtimes(e.shared("saves", "Same", "level.dat"), at, at)
	_ = os.Chtimes(e.game(e.a, "saves", "Same", "level.dat"), at, at)
	_ = os.MkdirAll(e.game(e.a, "saves", "Empty"), 0o700)

	if err := e.svc.Join(ctx, e.a, "saves"); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{
		e.shared("saves", "Mine", "level.dat"):      "mine",
		e.shared("saves", "Clash", "level.dat"):     "theirs",
		e.shared("saves", "Clash (2)", "level.dat"): "ours",
		e.shared("saves", "Same", "level.dat"):      "same",
	} {
		if got := body(t, p); got != want {
			t.Errorf("%s: %q", p, got)
		}
	}
	if exists(e.shared("saves", "Same (2)")) || exists(e.shared("saves", "Empty")) {
		t.Fatal("an identical or empty folder was kept twice")
	}
	if entries, _ := os.ReadDir(e.game(e.a, "saves")); len(entries) != 0 {
		t.Fatalf("the instance kept %v", entries)
	}
	if !slices.Equal(e.retitled, []string{"Clash (2)| (2)"}) {
		t.Fatalf("retitled %v", e.retitled)
	}
	// The instance now finds its worlds in the shared folder.
	if got := e.insts.Path(e.a, "saves/Mine"); got != e.shared("saves", "Mine") {
		t.Fatalf("Path = %s", got)
	}
	if got := e.insts.Path(e.b, "saves/Mine"); got != e.game(e.b, "saves", "Mine") {
		t.Fatalf("B isn't joined: %s", got)
	}
	if got := e.insts.Base(e.a, "resourcepacks"); got != e.insts.GameDir(e.a) {
		t.Fatalf("resourcepacks aren't shared: %s", got)
	}
	g := e.svc.Groups()
	if g.Total != 2 || g.Folders[0].ID != "saves" || g.Folders[0].Users != 1 || !g.Folders[0].Enabled || g.Folders[0].Path != "saves/" {
		t.Fatalf("%+v", g.Folders[0])
	}
}

func TestLeaveCopiesOutAndOffTrashes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable("resourcepacks")
	put(t, e.game(e.a, "resourcepacks", "a.zip"), "A")
	put(t, e.game(e.b, "resourcepacks", "b.zip"), "B")
	for _, id := range []string{e.a, e.b} {
		if err := e.svc.Join(ctx, id, "resourcepacks"); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.svc.Leave(ctx, e.a, "resourcepacks"); err != nil {
		t.Fatal(err)
	}
	if body(t, e.game(e.a, "resourcepacks", "b.zip")) != "B" || !exists(e.shared("resourcepacks", "a.zip")) {
		t.Fatal("leaving must copy, not move")
	}
	put(t, e.game(e.a, "resourcepacks", "b.zip"), "private now")
	if body(t, e.shared("resourcepacks", "b.zip")) != "B" {
		t.Fatal("the copy shares writes")
	}

	e.running[e.b] = true
	if err := e.svc.SetEnabled(ctx, "resourcepacks", false); !errors.Is(err, ErrRunning) {
		t.Fatalf("turned off under a running game: %v", err)
	}
	e.running[e.b] = false
	if err := e.svc.SetEnabled(ctx, "resourcepacks", false); err != nil {
		t.Fatal(err)
	}
	if body(t, e.game(e.b, "resourcepacks", "a.zip")) != "A" || exists(e.shared("resourcepacks")) {
		t.Fatal("turning off must leave each instance a copy and clear the shared one")
	}
	trashed, _ := filepath.Glob(filepath.Join(e.dir, "trash", "shared", "resourcepacks~*", "resourcepacks", "a.zip"))
	if len(trashed) != 1 {
		t.Fatal("the shared copy wasn't trashed")
	}
	if in, _ := e.insts.Get(e.b); len(in.Sync.Join) != 0 {
		t.Fatalf("joins kept: %v", in.Sync.Join)
	}
	e.svc.PurgeTrash(time.Now().Add(TrashKeep + time.Hour))
	if exists(filepath.Join(e.dir, "trash", "shared", filepath.Base(filepath.Dir(filepath.Dir(trashed[0]))))) {
		t.Fatal("not purged")
	}
}

func TestFilesNewestWins(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable("servers")
	put(t, e.shared("servers.dat"), "old")
	_ = os.Chtimes(e.shared("servers.dat"), time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	put(t, e.game(e.a, "servers.dat"), "new")
	if err := e.svc.Join(ctx, e.a, "servers"); err != nil {
		t.Fatal(err)
	}
	if body(t, e.shared("servers.dat")) != "new" {
		t.Fatal("the newer instance copy didn't win")
	}
	if err := e.svc.Join(ctx, e.b, "servers"); err != nil {
		t.Fatal(err)
	}
	if body(t, e.game(e.b, "servers.dat")) != "new" {
		t.Fatal("a missing file must take the shared one")
	}
}

func TestJoinRefusals(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.svc.Join(ctx, e.a, "saves"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("joined a target that is off: %v", err)
	}
	e.enable("saves")
	e.running[e.a] = true
	if err := e.svc.Join(ctx, e.a, "saves"); !errors.Is(err, ErrRunning) {
		t.Fatalf("joined while running: %v", err)
	}
	if err := e.svc.SetEnabled(ctx, "nope", true); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}
}

func TestCustomPaths(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	c, err := e.svc.AddCustom("config/jei/", true)
	if err != nil || c.Path != "config/jei/" || !c.Enabled {
		t.Fatalf("%+v %v", c, err)
	}
	for _, bad := range []string{"config/jei", "config/jei/sub", "config", "options.txt", "../x", "mods/a"} {
		if _, err := e.svc.AddCustom(bad, true); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	// A custom folder inside a shared folder is shared through it.
	e.enable("config")
	if err := e.svc.Join(ctx, e.a, "config"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Join(ctx, e.a, c.ID); err != nil {
		t.Fatal(err)
	}
	if f := e.svc.Folders(e.a); len(f) != 1 || f[0].ID != "config" {
		t.Fatalf("folders %+v", f)
	}
	put(t, e.shared("config", "jei", "x.cfg"), "x")
	if err := e.svc.Leave(ctx, e.a, "config"); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(e.game(e.a, "config", "jei")); len(entries) != 0 {
		t.Fatal("the still-shared folder kept a copy")
	}
	if got := e.insts.Base(e.a, "config/jei/x.cfg"); got != e.shared() {
		t.Fatalf("jei should stay shared: %s", got)
	}
	if err := e.svc.RemoveCustom(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if body(t, e.game(e.a, "config", "jei", "x.cfg")) != "x" || len(e.svc.Groups().Custom) != 0 {
		t.Fatal("removing a custom path must leave a copy")
	}
}

func TestModAvailabilityAndNewInstanceShares(t *testing.T) {
	e := newEnv(t)
	if g := e.svc.Groups(); g.Mods[0].Available {
		t.Fatal("no instance has Xaero's")
	}
	e.hasMod = true
	if g := e.svc.Groups(); !g.Mods[0].Available {
		t.Fatal("availability")
	}
	if err := e.svc.JoinAll(context.Background(), e.a, []string{KeysID, "servers", "saves", "unknown"}); err != nil {
		t.Fatal(err)
	}
	in, _ := e.insts.Get(e.a)
	if !in.Sync.Keys || !slices.Equal(in.Sync.Join, []string{"servers", "saves"}) {
		t.Fatalf("%+v", in.Sync)
	}
	if g := e.svc.Groups(); !g.Files[1].Enabled || !g.Folders[0].Enabled {
		t.Fatal("New instance shares turn their targets on")
	}
}

func TestBusyCountsSharers(t *testing.T) {
	e := newEnv(t)
	e.enable("saves")
	for _, id := range []string{e.a, e.b} {
		if err := e.svc.Join(context.Background(), id, "saves"); err != nil {
			t.Fatal(err)
		}
	}
	e.running[e.b] = true
	if !e.svc.Busy(e.a, "saves") || e.svc.Busy(e.a, "resourcepacks") {
		t.Fatal("A shares worlds with B, which runs; its packs are its own")
	}
}

// A server list the launcher edits in one instance reaches the next launch
// of another.
func TestTouchedSharesLauncherEdits(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable("servers")
	for _, id := range []string{e.a, e.b} {
		if err := e.svc.Join(ctx, id, "servers"); err != nil {
			t.Fatal(err)
		}
	}
	put(t, e.game(e.a, "servers.dat"), "edited")
	_ = os.Chtimes(e.game(e.a, "servers.dat"), time.Now().Add(time.Second), time.Now().Add(time.Second))
	if err := e.svc.Touched(e.a, "servers.dat"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Prepare(e.b); err != nil {
		t.Fatal(err)
	}
	if body(t, e.game(e.b, "servers.dat")) != "edited" {
		t.Fatal("B didn't get A's edit")
	}
	if err := e.svc.Touched(e.a, "hotbar.nbt"); err != nil { // not shared: nothing to do
		t.Fatal(err)
	}
}
