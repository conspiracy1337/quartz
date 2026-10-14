package content

import (
	"context"
	"path/filepath"
	"quartz/internal/core/fsx"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// watcher follows the content folders of the one instance the UI shows, so
// files added or removed outside the launcher appear without a refresh.
type watcher struct {
	mu    sync.Mutex
	w     *fsnotify.Watcher
	id    string
	dirs  map[string]Kind // each kind's folder (game or shared) → kind
	timer *time.Timer
	dirty map[Kind]bool
}

const watchDelay = 400 * time.Millisecond

// Watch follows instance id's content folders ("" stops watching).
func (s *Service) Watch(id string) {
	s.watch.mu.Lock()
	defer s.watch.mu.Unlock()
	if s.watch.id == id {
		return
	}
	if s.watch.w != nil {
		_ = s.watch.w.Close()
		s.watch.w = nil
	}
	s.watch.id = id
	if id == "" {
		return
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		s.d.Log.Warn("content: can't watch folders", "err", err)
		return
	}
	dirs := map[string]Kind{}
	for _, k := range Kinds {
		dir := s.abs(id, k.top())
		dirs[dir] = k
		_ = w.Add(dir) // may not exist yet: its parent's watch adds it once it does
		_ = w.Add(filepath.Dir(dir))
	}
	saves := s.base(id, DataPacks)
	for _, f := range worldsAt(saves) {
		_ = w.Add(filepath.Join(saves, filepath.FromSlash(f)))
	}
	s.watch.w, s.watch.dirs, s.watch.dirty = w, dirs, map[Kind]bool{}
	go s.watchLoop(w, id)
}

func worldsAt(game string) []string {
	var out []string
	matches, _ := filepath.Glob(filepath.Join(game, "saves", "*", "datapacks"))
	for _, m := range matches {
		if rel, err := filepath.Rel(game, m); err == nil {
			out = append(out, filepath.ToSlash(rel))
		}
	}
	return out
}

func (s *Service) watchLoop(w *fsnotify.Watcher, id string) {
	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			s.touched(id, ev.Name)
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			// Events were lost (the kernel's queue overflowed): look at everything.
			s.d.Log.Debug("content: folder watch lost events", "err", err)
			s.touchedAll(id)
		}
	}
}

// touched marks the kind a changed path belongs to and schedules a scan.
func (s *Service) touched(id, name string) {
	s.watch.mu.Lock()
	defer s.watch.mu.Unlock()
	if s.watch.id != id {
		return
	}
	k, ok := Kind(""), false
	for dir, kind := range s.watch.dirs {
		if fsx.Inside(dir, name) {
			k, ok = kind, true
		}
	}
	if !ok {
		return
	}
	if _, top := s.watch.dirs[name]; top && s.watch.w != nil {
		_ = s.watch.w.Add(name) // a content folder that was just created
	}
	if k == DataPacks && s.watch.w != nil {
		// A new world: start following its datapacks folder too.
		saves := s.base(id, DataPacks)
		for _, f := range worldsAt(saves) {
			_ = s.watch.w.Add(filepath.Join(saves, filepath.FromSlash(f)))
		}
	}
	s.watch.dirty[k] = true
	s.scheduleWatch(id)
}

// touchedAll schedules a scan of every kind.
func (s *Service) touchedAll(id string) {
	s.watch.mu.Lock()
	defer s.watch.mu.Unlock()
	if s.watch.id != id {
		return
	}
	for _, k := range Kinds {
		s.watch.dirty[k] = true
	}
	s.scheduleWatch(id)
}

// scheduleWatch (re)starts the debounce; the caller holds watch.mu.
func (s *Service) scheduleWatch(id string) {
	if s.watch.timer == nil {
		s.watch.timer = time.AfterFunc(watchDelay, func() { s.flushWatch(id) })
	} else {
		s.watch.timer.Reset(watchDelay)
	}
}

func (s *Service) flushWatch(id string) {
	s.watch.mu.Lock()
	kinds := s.watch.dirty
	s.watch.dirty = map[Kind]bool{}
	s.watch.timer = nil
	current := s.watch.id == id
	s.watch.mu.Unlock()
	if !current {
		return
	}
	for k := range kinds {
		_ = s.Scan(context.Background(), id, k)
	}
}
