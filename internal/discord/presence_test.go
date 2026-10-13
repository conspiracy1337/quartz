package discord

import (
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type fakeConn struct {
	mu     sync.Mutex
	shown  []*Activity
	closed int
}

func (f *fakeConn) SetActivity(_ int, a *Activity) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shown = append(f.shown, a)
	return nil
}

func (f *fakeConn) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
	return nil
}

func (f *fakeConn) last() (*Activity, int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.shown) == 0 {
		return nil, 0, f.closed
	}
	return f.shown[len(f.shown)-1], len(f.shown), f.closed
}

func eventually(t *testing.T, ok func() bool) {
	t.Helper()
	for range 200 {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out")
}

func newTestPresence(enabled func(string) bool, connect func(string) (conn, error)) *Presence {
	p := &Presence{clientID: "1", enabled: enabled, connect: connect, log: slog.New(slog.DiscardHandler), games: map[string]Game{}, wake: make(chan struct{}, 1),
		first: 5 * time.Millisecond, most: 10 * time.Millisecond}
	go p.run()
	return p
}

func TestPresenceShowsTheNewestGameAndClears(t *testing.T) {
	fc := &fakeConn{}
	var mu sync.Mutex
	connects := 0
	p := newTestPresence(func(id string) bool { return id != "hidden" }, func(string) (conn, error) {
		mu.Lock()
		connects++
		mu.Unlock()
		return fc, nil
	})
	now := time.Now()
	p.Started("r1", Game{Instance: "a", Name: "Survival", Minecraft: "1.21.4", Loader: "fabric", Started: now})
	eventually(t, func() bool { a, _, _ := fc.last(); return a != nil && a.Details == "Survival" })
	if a, _, _ := fc.last(); a.State != "Minecraft 1.21.4 with Fabric" || a.Assets.SmallImage != "fabric" || a.Timestamps.Start != now.UnixMilli() {
		t.Fatalf("%+v %+v", a, a.Assets)
	}
	p.Started("r2", Game{Instance: "b", Name: "Creative", Minecraft: "1.20.1", Started: now.Add(time.Second)})
	eventually(t, func() bool { a, _, _ := fc.last(); return a != nil && a.Details == "Creative" })
	p.Started("r3", Game{Instance: "hidden", Name: "Secret", Started: now.Add(2 * time.Second)})
	time.Sleep(20 * time.Millisecond)
	if a, _, _ := fc.last(); a.Details != "Creative" {
		t.Fatal("a hidden instance is never shown")
	}
	for _, r := range []string{"r1", "r2", "r3"} {
		p.Exited(r)
	}
	eventually(t, func() bool { a, n, closed := fc.last(); return n > 0 && a == nil && closed == 1 })
	mu.Lock()
	defer mu.Unlock()
	if connects != 1 {
		t.Fatalf("connected %d times", connects)
	}
}

func TestPresenceRetriesOnlyWhileAGameRuns(t *testing.T) {
	var mu sync.Mutex
	tries := 0
	p := newTestPresence(func(string) bool { return true }, func(string) (conn, error) {
		mu.Lock()
		tries++
		mu.Unlock()
		return nil, errors.New("not running")
	})
	p.Started("r1", Game{Instance: "a", Name: "x", Started: time.Now()})
	eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return tries >= 3 })
	p.Exited("r1")
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	n := tries
	mu.Unlock()
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if tries != n {
		t.Fatalf("kept retrying with no game: %d then %d", n, tries)
	}
}
