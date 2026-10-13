package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlaceInto(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "store", "a.jar")
	if err := os.MkdirAll(filepath.Dir(src), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("jar"), 0o400); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "server")
	s := &Server{Place: map[string]string{"libraries/x/a.jar": src, "a.jar": src}}
	if err := s.PlaceInto(dir); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"libraries/x/a.jar", "a.jar"} {
		info, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("%s placed as %v, %v", rel, info, err)
		}
	}
	if err := s.PlaceInto(dir); err != nil {
		t.Fatal("placing again keeps what is there:", err)
	}
	s.Place = map[string]string{"../escape.jar": src}
	if err := s.PlaceInto(dir); err == nil {
		t.Fatal("a path outside the server folder was accepted")
	}
	s.Place = map[string]string{"libraries/missing.jar": filepath.Join(root, "nope.jar")}
	if err := s.PlaceInto(dir); err == nil {
		t.Fatal("a missing store file must be reported")
	}
}
