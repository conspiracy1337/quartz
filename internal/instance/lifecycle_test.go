package instance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := t.TempDir()
	s, err := Open(filepath.Join(root, "instances"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return s, root
}

func create(t *testing.T, s *Store, name string) Instance {
	t.Helper()
	in, err := s.Create(CreateRequest{Name: name, Minecraft: "1.21.1"})
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestEdits(t *testing.T) {
	s, _ := newStore(t)
	in := create(t, s, "Survival")
	if got, err := s.Rename(in.ID, "  Hardcore  "); err != nil || got.Name != "Hardcore" || got.ID != in.ID {
		t.Fatalf("rename = %+v, %v", got, err)
	}
	if _, err := s.Rename(in.ID, ""); !errors.Is(err, ErrBadName) {
		t.Fatal("empty name accepted")
	}
	got, _ := s.SetTags(in.ID, []string{" pvp ", "PvP", "", strings.Repeat("x", 25), "builds"})
	if strings.Join(got.Tags, ",") != "pvp,builds" {
		t.Fatalf("tags = %q", got.Tags)
	}
	if _, err := s.SetNotes(in.ID, strings.Repeat("n", maxNotes+1)); err == nil {
		t.Fatal("overlong notes accepted")
	}
	if _, err := s.SetLook(in.ID, Look{Art: Art{Background: "red"}}); err == nil {
		t.Fatal("bad colour accepted")
	}
	pic, banner := strings.Repeat("a", 64), strings.Repeat("b", 64)
	got, _ = s.SetLook(in.ID, Look{Art: Art{Background: "#e2a03f"}, Icon: pic, Banner: banner})
	got, _ = s.SetLook(got.ID, Look{Art: Art{Background: "#3ecf8e"}, Icon: got.Icon, Banner: got.Banner})
	if got.Icon != pic || got.Banner != banner || got.Art.Background != "#3ecf8e" || got.Art.Initial != "H" {
		t.Fatalf("a new colour keeps the picture, the banner and a letter: %+v", got)
	}
	if got, _ = s.SetBanner(in.ID, ""); got.Banner != "" || got.Icon != pic {
		t.Fatalf("removing the banner keeps the picture: %+v", got)
	}
	for _, l := range []Look{{Art: Art{Background: "#3ecf8e"}, Icon: "../x.png"}, {Art: Art{Background: "#3ecf8e"}, Banner: "../x.png"}, {Art: Art{Background: "#3ecf8e", Initial: "abc"}}} {
		if _, err := s.SetLook(in.ID, l); err == nil {
			t.Fatalf("bad look accepted: %+v", l)
		}
	}
	if _, err := s.SetIcon(in.ID, "../x.png"); err == nil {
		t.Fatal("bad icon hash accepted")
	}
}

func TestTrashRestoreAndPurge(t *testing.T) {
	s, root := newStore(t)
	in := create(t, s, "World")
	_ = os.WriteFile(filepath.Join(s.GameDir(in.ID), "options.txt"), []byte("fov:90"), 0o600)
	now := time.Now()
	e, err := s.Trash(in.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(in.ID); !errors.Is(err, ErrUnknown) {
		t.Fatal("trashed instance still listed")
	}
	if list := s.TrashList(); len(list) != 1 || list[0].Name != "World" {
		t.Fatalf("trash = %+v", list)
	}
	// Someone takes the id meanwhile: the restore gets a new one.
	if err := os.MkdirAll(filepath.Join(root, "instances", in.ID), 0o700); err != nil {
		t.Fatal(err)
	}
	back, err := s.Restore(e.Key)
	if err != nil {
		t.Fatal(err)
	}
	if back.ID == in.ID || back.Name != "World" {
		t.Fatalf("restored = %+v", back)
	}
	if b, _ := os.ReadFile(filepath.Join(s.GameDir(back.ID), "options.txt")); string(b) != "fov:90" {
		t.Fatal("game files lost in the trash")
	}
	if again, _ := s.Get(back.ID); again.ID != back.ID {
		t.Fatal("restored instance not registered")
	}
	if len(s.TrashList()) != 0 {
		t.Fatal("restored entry still in the trash")
	}
	// Reopening proves the new id was written to instance.json.
	if s2, _ := Open(filepath.Join(root, "instances"), nil); s2 != nil {
		if _, err := s2.Get(back.ID); err != nil {
			t.Fatal("restored instance doesn't survive a restart")
		}
	}

	// Expired entries are purged; recent ones stay.
	var purged []string
	s.OnPurge(func(id string) { purged = append(purged, id) })
	old, _ := s.Trash(back.ID, now.Add(-31*24*time.Hour))
	recent := create(t, s, "Recent")
	_, _ = s.Trash(recent.ID, now)
	if n := s.PurgeExpired(now); n != 1 || len(purged) != 1 || purged[0] != back.ID {
		t.Fatalf("purged %d %v", n, purged)
	}
	if _, err := s.readEntry(old.Key); err == nil {
		t.Fatal("expired entry kept")
	}
	if len(s.TrashList()) != 1 {
		t.Fatal("recent entry purged")
	}
	for _, bad := range []string{"../../etc", "x~1", "", "a~0000000000000/../../"} {
		if err := s.Purge(bad); err == nil {
			t.Errorf("purge accepted %q", bad)
		}
	}
}

func TestPurgeOnlyDeletesMarkedLocations(t *testing.T) {
	s, root := newStore(t)
	in := create(t, s, "Moved")
	elsewhere := filepath.Join(root, "other-drive")
	_ = os.MkdirAll(elsewhere, 0o700)
	p, err := s.PlanMove(in.ID, elsewhere)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Move(context.Background(), p, nil); err != nil {
		t.Fatal(err)
	}
	moved := s.GameDir(in.ID)
	// A hand-edited location pointing at a folder we never created is never deleted.
	foreign := filepath.Join(root, "precious")
	_ = os.MkdirAll(foreign, 0o700)
	other := create(t, s, "Foreign")
	_, _ = s.Update(other.ID, func(i *Instance) error { i.Location = foreign; return nil })

	e1, _ := s.Trash(in.ID, time.Now())
	e2, _ := s.Trash(other.ID, time.Now())
	if _, err := os.Stat(moved); err != nil {
		t.Fatal("a moved game folder must stay while in the trash")
	}
	_ = s.Purge(e1.Key)
	_ = s.Purge(e2.Key)
	if _, err := os.Stat(moved); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("our moved folder survived the purge")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal("a folder without our marker was deleted")
	}
}

func TestMove(t *testing.T) {
	s, root := newStore(t)
	in := create(t, s, "Big")
	world := filepath.Join(s.GameDir(in.ID), "saves", "World", "level.dat")
	_ = os.MkdirAll(filepath.Dir(world), 0o700)
	_ = os.WriteFile(world, []byte("level"), 0o600)

	full := filepath.Join(root, "full")
	_ = os.MkdirAll(filepath.Join(full, in.ID, "something"), 0o700)
	if _, err := s.PlanMove(in.ID, full); !errors.Is(err, ErrMoveTarget) {
		t.Fatal("a non-empty target must be refused")
	}
	if _, err := s.PlanMove(in.ID, "relative/dir"); !errors.Is(err, ErrMoveTarget) {
		t.Fatal("a relative target must be refused")
	}
	if _, err := s.PlanMove(in.ID, s.GameDir(in.ID)); !errors.Is(err, ErrMoveTarget) {
		t.Fatal("moving into itself must be refused")
	}
	if _, err := s.PlanMove(in.ID, ""); !errors.Is(err, ErrMoveSame) {
		t.Fatal("already in the default place")
	}

	// A copy (as across drives) is checked, then switched to.
	dst := filepath.Join(root, "drive2")
	_ = os.MkdirAll(dst, 0o700)
	var last int64
	if err := copyTree(context.Background(), s.GameDir(in.ID), filepath.Join(dst, "copy"), copyOpts{progress: func(d, _ int64) { last = d }}); err != nil || last != 5 {
		t.Fatalf("copy: %v (%d bytes)", err, last)
	}
	p, _ := s.PlanMove(in.ID, dst)
	got, err := s.Move(context.Background(), p, nil)
	if err != nil || got.Location != filepath.Join(dst, in.ID) || s.GameDir(in.ID) != got.Location {
		t.Fatalf("move = %+v, %v", got, err)
	}
	if b, _ := os.ReadFile(filepath.Join(got.Location, "saves", "World", "level.dat")); string(b) != "level" {
		t.Fatal("world lost in the move")
	}
	if !markedFor(got.Location, in.ID) {
		t.Fatal("moved folder not marked")
	}
	// And back.
	p, _ = s.PlanMove(in.ID, "")
	got, err = s.Move(context.Background(), p, nil)
	if err != nil || got.Location != "" {
		t.Fatalf("move back = %+v, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(s.GameDir(in.ID), markerName)); err == nil {
		t.Fatal("the default folder needs no marker")
	}
}

func TestHandEditOfTheWrongTypeKeepsTheInstance(t *testing.T) {
	s, root := newStore(t)
	in := create(t, s, "Survival")
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Dir(in.ID), "instance.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), `"playtimeMin": 0`, `"playtimeMin": "120"`, 1)
	if edited == string(data) {
		t.Fatal("test setup: playtimeMin not found")
	}
	data = []byte(edited)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	again, err := Open(filepath.Join(root, "instances"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := again.Get(in.ID); err != nil || got.Name != "Survival" {
		t.Fatalf("instance lost after a hand edit: %+v %v", got, err)
	}
}

func TestMoveKeepsTheSourceWhenSavingFails(t *testing.T) {
	s, root := newStore(t)
	in := create(t, s, "Kept")
	world := filepath.Join(s.GameDir(in.ID), "saves", "World")
	_ = os.MkdirAll(world, 0o700)
	_ = os.WriteFile(filepath.Join(world, "level.dat"), []byte("level"), 0o600)
	drive := filepath.Join(root, "drive2")
	_ = os.MkdirAll(drive, 0o700)
	p, err := s.PlanMove(in.ID, drive)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	f := s.files[in.ID]
	delete(s.files, in.ID) // saving the new location now fails
	s.mu.Unlock()
	if _, err := s.Move(context.Background(), p, nil); err == nil {
		t.Fatal("move succeeded without saving")
	}
	if b, _ := os.ReadFile(filepath.Join(p.From, "saves", "World", "level.dat")); string(b) != "level" {
		t.Fatal("the source was lost although the instance still points at it")
	}
	if _, err := os.Stat(p.To); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the half-moved copy was left behind")
	}
	s.mu.Lock()
	s.files[in.ID] = f
	s.mu.Unlock()
}

func TestCopyTreeRepointsLinksIntoTheSource(t *testing.T) {
	root := t.TempDir()
	src, dst := filepath.Join(root, "src"), filepath.Join(root, "dst")
	_ = os.MkdirAll(filepath.Join(src, "shared"), 0o700)
	_ = os.WriteFile(filepath.Join(src, "shared", "a"), []byte("a"), 0o600)
	if err := os.Symlink(filepath.Join(src, "shared"), filepath.Join(src, "link")); err != nil {
		t.Skip("no symlinks here")
	}
	if err := copyTree(context.Background(), src, dst, copyOpts{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(filepath.Join(dst, "link")); got != filepath.Join(dst, "shared") {
		t.Fatalf("link = %q", got)
	}
}

func TestCheckGameDirNoticesAMissingDrive(t *testing.T) {
	s, root := newStore(t)
	in := create(t, s, "Away")
	if err := s.CheckGameDir(in.ID); err != nil {
		t.Fatalf("default folder: %v", err)
	}
	drive := filepath.Join(root, "drive2")
	_ = os.MkdirAll(drive, 0o700)
	p, _ := s.PlanMove(in.ID, drive)
	moved, err := s.Move(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CheckGameDir(in.ID); err != nil {
		t.Fatalf("moved folder: %v", err)
	}
	// Unplugged: the mount point is there, empty, without our marker.
	if err := os.RemoveAll(moved.Location); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(moved.Location, 0o700)
	if err := s.CheckGameDir(in.ID); !errors.Is(err, ErrGameDirMissing) {
		t.Fatalf("missing drive: %v", err)
	}
}

func TestDuplicate(t *testing.T) {
	s, _ := newStore(t)
	in := create(t, s, "Survival")
	game := s.GameDir(in.ID)
	for rel, body := range map[string]string{"saves/World/level.dat": "level", "options.txt": "fov:90", "mods/a.jar": "jar", "logs/latest.log": "log", ".quartz/removed/x": "held"} {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(game, rel)), 0o700)
		_ = os.WriteFile(filepath.Join(game, rel), []byte(body), 0o600)
	}
	_ = os.WriteFile(filepath.Join(s.Dir(in.ID), "modpack.json"), []byte("{}"), 0o600)
	_, _ = s.Update(in.ID, func(i *Instance) error { i.PlaytimeMin = 300; return nil })
	var linked []string
	place := func(rel, dst string) (bool, error) {
		if rel != "mods/a.jar" {
			return false, nil
		}
		linked = append(linked, rel)
		return true, os.WriteFile(dst, []byte("jar"), 0o600)
	}
	cp, err := s.Duplicate(context.Background(), in.ID, place, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cp.ID == in.ID || cp.Name != "Survival (copy)" || cp.PlaytimeMin != 0 || cp.Minecraft != in.Minecraft {
		t.Fatalf("%+v", cp)
	}
	got := s.GameDir(cp.ID)
	for rel, want := range map[string]bool{"saves/World/level.dat": true, "options.txt": true, "mods/a.jar": true, "logs/latest.log": false, ".quartz": false} {
		if _, err := os.Stat(filepath.Join(got, rel)); (err == nil) != want {
			t.Errorf("%s exists=%v", rel, err == nil)
		}
	}
	if len(linked) != 1 {
		t.Fatal("store files must be placed, not copied")
	}
	if _, err := os.Stat(filepath.Join(s.Dir(cp.ID), "modpack.json")); err != nil {
		t.Fatal("the modpack record must come along")
	}
	again, _ := s.Duplicate(context.Background(), in.ID, nil, nil)
	if again.Name != "Survival (copy) (2)" {
		t.Fatalf("second copy: %q", again.Name)
	}

	// A failure leaves nothing behind.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Duplicate(ctx, in.ID, nil, nil); err == nil {
		t.Fatal("cancelled duplicate succeeded")
	}
	entries, _ := os.ReadDir(filepath.Dir(s.Dir(in.ID)))
	if len(entries) != 3 {
		t.Fatalf("left behind: %v", entries)
	}
}
