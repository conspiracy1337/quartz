package gamewatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"quartz/internal/core/events"
)

func TestTopics(t *testing.T) {
	game := "/g"
	for name, want := range map[string]events.Topic{"/g/servers.dat": events.ServersChanged, "/g/saves/New World": events.WorldsChanged, "/g/screenshots/a.png": events.ScreenshotsChanged} {
		if got, ok := topic(game, name); !ok || got != want {
			t.Errorf("%s: %v", name, got)
		}
	}
	if _, ok := topic(game, "/g/options.txt"); ok {
		t.Error("options.txt announced")
	}
}

func TestWatchEmits(t *testing.T) {
	game := t.TempDir()
	bus := events.NewBus()
	got := make(chan events.Topic, 8)
	for _, tp := range []events.Topic{events.WorldsChanged, events.ScreenshotsChanged, events.ServersChanged} {
		bus.Subscribe(tp, func(any) { got <- tp })
	}
	w := New(bus, func(string) string { return game })
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
