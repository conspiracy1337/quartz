package api

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"quartz/internal/instance"
)

// A tab can only pick one of the page's own folders, never a path.
func TestOpenTabFolderRejectsUnknownTabs(t *testing.T) {
	s := &Instances{}
	for _, tab := range []string{"", "..", "../..", "mods/../..", "/etc", "Mods", "config"} {
		if err := s.OpenTabFolder("abc", tab); !errors.Is(err, ErrBadTab) {
			t.Fatalf("tab %q: %v", tab, err)
		}
	}
}

// A moved game folder that's gone (an unplugged drive) is never recreated by opening a tab's folder.
func TestOpenTabFolderLeavesAMissingGameFolder(t *testing.T) {
	store, err := instance.Open(filepath.Join(t.TempDir(), "instances"), nil)
	if err != nil {
		t.Fatal(err)
	}
	in, err := store.Create(instance.CreateRequest{Name: "T", Minecraft: "1.21.4"})
	if err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(t.TempDir(), "drive", "game")
	if _, err := store.Update(in.ID, func(i *instance.Instance) error { i.Location = gone; return nil }); err != nil {
		t.Fatal(err)
	}
	s := NewInstances(&Ctx{}, InstanceDeps{Store: store})
	if err := s.OpenTabFolder(in.ID, "mods"); !errors.Is(err, instance.ErrGameDirMissing) {
		t.Fatal(err)
	}
	if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the missing game folder was created")
	}
}

// Rename moves the folder, so it waits for the game, and follows a watched instance again afterwards.
func TestRenameGuardsAndRewatches(t *testing.T) {
	store, err := instance.Open(filepath.Join(t.TempDir(), "instances"), nil)
	if err != nil {
		t.Fatal(err)
	}
	in, _ := store.Create(instance.CreateRequest{Name: "T", Minecraft: "1.21.4"})
	busy, rewatched := true, ""
	s := NewInstances(&Ctx{}, InstanceDeps{Store: store, Busy: func(string) bool { return busy },
		Unwatch: func(string) bool { return true }, Rewatch: func(id string) { rewatched = id }})
	if _, err := s.Rename(in.ID, "Other"); !errors.Is(err, ErrBusy) || rewatched != "" || filepath.Base(store.Dir(in.ID)) != "t" {
		t.Fatalf("renamed while running: %v %q %s", err, rewatched, store.Dir(in.ID))
	}
	busy = false
	if _, err := s.Rename(in.ID, " "); !errors.Is(err, instance.ErrBadName) || rewatched != in.ID {
		t.Fatalf("%v, rewatched %q", err, rewatched)
	}
}
