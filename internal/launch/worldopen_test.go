package launch

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"quartz/internal/runview"
)

// A world a second copy opened in its private view is open: hosting or
// playing it elsewhere would fork it.
func TestWorldOpenAsksPrivateViews(t *testing.T) {
	dir := t.TempDir()
	game := filepath.Join(dir, "game")
	world := filepath.Join(game, "saves", "W")
	if err := os.MkdirAll(world, 0o700); err != nil {
		t.Fatal(err)
	}
	views := runview.New(runview.Config{Backend: "clone"})
	first, err := views.Assemble(runview.Request{Game: game, Scratch: filepath.Join(dir, "1.view")})
	if err != nil {
		t.Fatal(err)
	}
	second, err := views.Assemble(runview.Request{Game: game, Scratch: filepath.Join(dir, "2.view")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = second.Close(context.Background()); _, _ = first.Close(context.Background()) }()
	l := &Launcher{running: map[string][]*run{"i": {{sp: spec{view: first}}, {sp: spec{view: second}}}}}
	if l.WorldOpen(world) {
		t.Fatal("nothing opened the world")
	}
	if err := os.WriteFile(filepath.Join(second.GameDir(), "saves", "W", "session.lock"), []byte("☃"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !l.WorldOpen(world) {
		t.Fatal("the second copy opened its own copy of the world")
	}
}
