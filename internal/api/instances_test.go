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
