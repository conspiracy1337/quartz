package backup

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestWorlds(t *testing.T) {
	game := t.TempDir()
	if _, err := Worlds(game, filepath.Join(game, "b"), time.Now()); !errors.Is(err, ErrNoWorlds) {
		t.Fatalf("no saves: %v", err)
	}
	w := filepath.Join(game, "saves", "New World")
	_ = os.MkdirAll(filepath.Join(w, "region"), 0o700)
	_ = os.WriteFile(filepath.Join(w, "level.dat"), []byte("lvl"), 0o600)
	_ = os.WriteFile(filepath.Join(w, "region", "r.0.0.mca"), []byte("r"), 0o600)
	_ = os.WriteFile(filepath.Join(w, "session.lock"), []byte("x"), 0o600)
	_ = os.Symlink("/etc/passwd", filepath.Join(w, "link"))
	out, err := Worlds(game, filepath.Join(game, "backups"), time.Date(2026, 10, 13, 9, 30, 0, 0, time.UTC))
	if err != nil || filepath.Base(out) != "2026-10-13_09-30-00.zip" {
		t.Fatalf("%q %v", out, err)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	if len(names) != 2 || names[0] != "saves/New World/level.dat" || names[1] != "saves/New World/region/r.0.0.mca" {
		t.Fatalf("%v", names)
	}
}
