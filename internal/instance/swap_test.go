package instance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSwapIsAllOrNothing(t *testing.T) {
	s, _ := newStore(t)
	in := create(t, s, "Swap")
	game := s.GameDir(in.ID)
	write := func(rel, body string) {
		p := filepath.Join(game, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o700)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(game, filepath.FromSlash(rel)))
		if err != nil {
			return "<missing>"
		}
		return string(b)
	}
	stage := func(sw *Swap, rel, body string) {
		if err := sw.Stage(rel, func(dst string) error { return os.WriteFile(dst, []byte(body), 0o600) }); err != nil {
			t.Fatal(err)
		}
	}
	write("config/a.toml", "old")
	write("config/b.toml.disabled", "a real file named so")
	write("config/gone.toml", "gone")

	sw := NewSwap(s, in.ID)
	stage(sw, "config/a.toml", "new")
	stage(sw, "config/b.toml", "b")
	for _, rel := range []string{"config/a.toml", "config/b.toml", "config/gone.toml"} {
		if err := sw.Aside(rel); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range []string{"config/a.toml", "config/b.toml"} {
		if err := sw.Put(rel); err != nil {
			t.Fatal(err)
		}
	}
	sw.Rollback()
	sw.Close()
	if read("config/a.toml") != "old" || read("config/b.toml") != "<missing>" || read("config/gone.toml") != "gone" || read("config/a.toml"+NewSuffix) != "<missing>" {
		t.Fatal("a rolled back swap left changes")
	}

	sw = NewSwap(s, in.ID)
	defer sw.Close()
	stage(sw, "config/a.toml", "new")
	for _, rel := range []string{"config/a.toml", "config/gone.toml"} {
		if err := sw.Aside(rel); err != nil {
			t.Fatal(err)
		}
	}
	if err := sw.Put("config/a.toml"); err != nil {
		t.Fatal(err)
	}
	sw.Finish()
	if read("config/a.toml") != "new" || read("config/gone.toml") != "<missing>" || read("config/a.toml"+OldSuffix) != "<missing>" {
		t.Fatal("a finished swap didn't apply")
	}
	if read("config/b.toml.disabled") != "a real file named so" {
		t.Fatal("setting a file aside moved its .disabled namesake")
	}
}
