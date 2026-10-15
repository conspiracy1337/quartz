package holdbin

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"
)

func setup(t *testing.T) (*os.Root, string) {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"screenshots/a.png", "saves/World/level.dat", "mods/x.jar"} {
		p := filepath.Join(dir, filepath.FromSlash(f))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root, dir
}

func exists(root *os.Root, rel string) bool { _, err := root.Lstat(rel); return err == nil }

func TestHoldAndRestore(t *testing.T) {
	root, _ := setup(t)
	now := time.Now()
	token, items, err := Hold(root, []Move{{Rel: "screenshots/a.png"}, {Rel: "saves/World"}, {Rel: "mods/.index/x.pw.toml", Optional: true}}, now)
	if err != nil || len(items) != 2 || !ValidToken(token) {
		t.Fatalf("%v %v %q", items, err, token)
	}
	if exists(root, "screenshots/a.png") || exists(root, "saves/World") {
		t.Fatal("items still in place")
	}
	got, err := Restore(root, token)
	if err != nil || len(got) != 2 || !exists(root, "saves/World/level.dat") || !exists(root, "screenshots/a.png") {
		t.Fatalf("restore: %v", err)
	}
	if exists(root, path.Join(Dir, token)) {
		t.Fatal("hold folder left behind")
	}
	if _, err := Restore(root, token); !errors.Is(err, ErrExpired) {
		t.Fatal("second restore should be expired")
	}
	if _, err := Restore(root, "../../etc"); !errors.Is(err, ErrExpired) {
		t.Fatal("bad token accepted")
	}
}

func TestRequiredFailureRollsBack(t *testing.T) {
	root, _ := setup(t)
	if _, _, err := Hold(root, []Move{{Rel: "screenshots/a.png"}, {Rel: "missing.txt"}}, time.Now()); err == nil {
		t.Fatal("missing required item accepted")
	}
	if !exists(root, "screenshots/a.png") {
		t.Fatal("moved item not put back")
	}
}

func TestNameTaken(t *testing.T) {
	root, dir := setup(t)
	token, _, _ := Hold(root, []Move{{Rel: "screenshots/a.png"}}, time.Now())
	_ = os.WriteFile(filepath.Join(dir, "screenshots", "a.png"), []byte("new"), 0o644)
	if _, err := Restore(root, token); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("got %v", err)
	}
}

func TestPurge(t *testing.T) {
	root, dir := setup(t)
	now := time.Now()
	old, _, _ := Hold(root, []Move{{Rel: "screenshots/a.png"}}, now.Add(-2*Keep))
	fresh, _, _ := Hold(root, []Move{{Rel: "mods/x.jar"}}, now)
	// A hold whose manifest is missing: purged by the folder's age.
	legacy := filepath.Join(dir, ".quartz", "removed", "0123456789abcdef")
	_ = os.MkdirAll(legacy, 0o700)
	_ = os.Chtimes(legacy, now.Add(-2*Keep), now.Add(-2*Keep))
	var dropped []string
	Purge(root, now, func(token string, it Item) { dropped = append(dropped, token+":"+it.Rel) })
	if exists(root, path.Join(Dir, old)) || exists(root, path.Join(Dir, "0123456789abcdef")) {
		t.Fatal("expired holds kept")
	}
	if !exists(root, path.Join(Dir, fresh)) {
		t.Fatal("fresh hold purged")
	}
	if len(dropped) != 1 || dropped[0] != old+":screenshots/a.png" {
		t.Fatalf("dropped %v", dropped)
	}
}
