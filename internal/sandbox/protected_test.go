package sandbox

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestProtectedFiles(t *testing.T) {
	root := t.TempDir()
	game, shared := filepath.Join(root, "game"), filepath.Join(root, "shared", "saves")
	write := func(p string, perm os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), perm); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, perm); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(game, "resourcepacks", "store.zip"), 0o400)
	write(filepath.Join(game, "resourcepacks", "own.zip"), 0o600)
	write(filepath.Join(game, "shaderpacks", "store.zip"), 0o444)
	write(filepath.Join(game, "shaderpacks", "store.zip.txt"), 0o600)
	// saves is a shared folder the view mounts at game/saves.
	write(filepath.Join(shared, "World", "datapacks", "store.zip"), 0o400)

	r := Run{GameDir: game, Binds: map[string]string{filepath.Join(game, "saves"): shared}}
	got := r.ProtectedFiles()
	want := []ProtectedFile{
		{Seen: filepath.Join(game, "resourcepacks", "store.zip"), Real: filepath.Join(game, "resourcepacks", "store.zip")},
		{Seen: filepath.Join(game, "shaderpacks", "store.zip"), Real: filepath.Join(game, "shaderpacks", "store.zip")},
		{Seen: filepath.Join(game, "saves", "World", "datapacks", "store.zip"), Real: filepath.Join(shared, "World", "datapacks", "store.zip")},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ProtectedFiles =\n%v\nwant\n%v", got, want)
	}
}
