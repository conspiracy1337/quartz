// Package gamewatch follows the worlds, screenshots and server list of the
// one instance the UI shows, so changes made outside the launcher (a world
// copied in by hand, a screenshot the running game just took, the game
// saving its server list) reach the page without a refresh.
package gamewatch

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"quartz/internal/core/events"
)

const delay = 400 * time.Millisecond

// Watcher is safe for concurrent use.
type Watcher struct {
	bus     *events.Bus
	gameDir func(id string) string

	mu    sync.Mutex
	w     *fsnotify.Watcher
	id    string
	game  string
	timer *time.Timer
	dirty map[events.Topic]bool
}

// New returns a Watcher that finds game folders through gameDir.
func New(bus *events.Bus, gameDir func(id string) string) *Watcher {
	return &Watcher{bus: bus, gameDir: gameDir}
}

// Watch follows instance id ("" stops).
func (w *Watcher) Watch(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.id == id {
		return
	}
	if w.w != nil {
		_ = w.w.Close()
		w.w = nil
	}
	w.id = id
	if id == "" {
		return
	}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	game := w.gameDir(id)
	for _, d := range []string{game, filepath.Join(game, "saves"), filepath.Join(game, "screenshots")} {
		_ = fw.Add(d)
	}
	w.w, w.game, w.dirty = fw, game, map[events.Topic]bool{}
	go w.loop(fw, id)
}

func (w *Watcher) loop(fw *fsnotify.Watcher, id string) {
	for {
		select {
		case ev, ok := <-fw.Events:
			if !ok {
				return
			}
			w.touched(fw, id, ev)
		case _, ok := <-fw.Errors:
			if !ok {
				return
			}
		}
	}
}

// topic maps a changed path to the event that announces it.
func topic(game, name string) (events.Topic, bool) {
	rel, err := filepath.Rel(game, name)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	switch {
	case rel == "servers.dat":
		return events.ServersChanged, true
	case rel == "saves" || strings.HasPrefix(rel, "saves/"):
		return events.WorldsChanged, true
	case rel == "screenshots" || strings.HasPrefix(rel, "screenshots/"):
		return events.ScreenshotsChanged, true
	}
	return "", false
}

func (w *Watcher) touched(fw *fsnotify.Watcher, id string, ev fsnotify.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.id != id || w.w != fw {
		return
	}
	t, ok := topic(w.game, ev.Name)
	if !ok {
		return
	}
	// saves/ or screenshots/ appearing for the first time: follow them too.
	if base := filepath.Base(ev.Name); filepath.Dir(ev.Name) == w.game && (base == "saves" || base == "screenshots") && ev.Has(fsnotify.Create) {
		_ = fw.Add(ev.Name)
	}
	w.dirty[t] = true
	if w.timer == nil {
		w.timer = time.AfterFunc(delay, func() { w.flush(id) })
	} else {
		w.timer.Reset(delay)
	}
}

func (w *Watcher) flush(id string) {
	w.mu.Lock()
	topics := w.dirty
	w.dirty = map[events.Topic]bool{}
	w.timer = nil
	current := w.id == id
	w.mu.Unlock()
	if !current {
		return
	}
	for t := range topics {
		w.bus.Emit(t, map[string]string{"instanceId": id})
	}
}
