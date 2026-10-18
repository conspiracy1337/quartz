// Package gamewatch follows the worlds, screenshots and server list of the
// one instance the UI shows, so changes made outside the launcher (a world
// copied in by hand, a screenshot the running game just took, the game
// saving its server list) reach the page without a refresh.
package gamewatch

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"quartz/internal/core/events"
	"quartz/internal/core/fsx"
)

const delay = 400 * time.Millisecond

// Watcher is safe for concurrent use.
type Watcher struct {
	bus  *events.Bus
	path func(id, rel string) string

	mu    sync.Mutex
	w     *fsnotify.Watcher
	id    string
	dirs  dirs
	timer *time.Timer
	dirty map[events.Topic]bool
}

// dirs are the watched places: saves/ and screenshots/ may live in the
// shared folder rather than the game folder.
type dirs struct{ game, saves, screenshots string }

// New returns a Watcher that finds an instance's folders through path
// (instance.Store.Path: the game folder, or the shared one).
func New(bus *events.Bus, path func(id, rel string) string) *Watcher {
	return &Watcher{bus: bus, path: path}
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
	d := dirs{game: w.path(id, ""), saves: w.path(id, "saves"), screenshots: w.path(id, "screenshots")}
	for _, p := range []string{d.game, d.saves, d.screenshots, filepath.Dir(d.saves), filepath.Dir(d.screenshots)} {
		_ = fw.Add(p)
	}
	w.w, w.dirs, w.dirty = fw, d, map[events.Topic]bool{}
	go w.loop(fw, id)
}

// Forget stops following instance id before its folder moves: an open
// watch holds the folders, and Windows can't move them then.
func (w *Watcher) Forget(id string) {
	w.mu.Lock()
	watching := w.id == id
	w.mu.Unlock()
	if watching {
		w.Watch("")
	}
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
func topic(d dirs, name string) (events.Topic, bool) {
	switch {
	case name == filepath.Join(d.game, "servers.dat"):
		return events.ServersChanged, true
	case fsx.Inside(d.saves, name):
		return events.WorldsChanged, true
	case fsx.Inside(d.screenshots, name):
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
	t, ok := topic(w.dirs, ev.Name)
	if !ok {
		return
	}
	// saves/ or screenshots/ appearing for the first time: follow them too.
	if (ev.Name == w.dirs.saves || ev.Name == w.dirs.screenshots) && ev.Has(fsnotify.Create) {
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
