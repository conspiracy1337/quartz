package gamewatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"quartz/internal/core/events"
)

func TestTopics(t *testing.T) {
	// Worlds shared: saves/ lives in the shared folder.
	d := dirs{game: "/g", saves: "/shared/saves", screenshots: "/g/screenshots"}
	for name, want := range map[string]events.Topic{"/g/servers.dat": events.ServersChanged, "/shared/saves/New World": events.WorldsChanged, "/g/screenshots/a.png": events.ScreenshotsChanged} {
		if got, ok := topic(d, name); !ok || got != want {
			t.Errorf("%s: %v", name, got)
		}
	}
	for _, name := range []string{"/g/options.txt", "/g/saves/x", "/shared/servers.dat"} {
		if _, ok := topic(d, name); ok {
			t.Errorf("%s announced", name)
		}
	}
}

func TestWatchEmits(t *testing.T) {
	game := t.TempDir()
	bus := events.NewBus()
	got := make(chan events.Topic, 8)
	for _, tp := range []events.Topic{events.WorldsChanged, events.ScreenshotsChanged, events.ServersChanged} {
		bus.Subscribe(tp, func(any) { got <- tp })
	}
	w := New(bus, func(_, rel string) string { return filepath.Join(game, rel) })
	w.Watch("a")
	defer w.Watch("")
	_ = os.WriteFile(filepath.Join(game, "servers.dat"), []byte("x"), 0o600)
	_ = os.Mkdir(filepath.Join(game, "screenshots"), 0o700)
	time.Sleep(100 * time.Millisecond)
	_ = os.WriteFile(filepath.Join(game, "screenshots", "a.png"), []byte("x"), 0o600)
	seen := map[events.Topic]bool{}
	timeout := time.After(3 * time.Second)
	for len(seen) < 2 {
		select {
		case tp := <-got:
			seen[tp] = true
		case <-timeout:
			t.Fatalf("saw %v", seen)
		}
	}
}
