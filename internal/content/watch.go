package content

import (
	"context"
	"path/filepath"
	"strings"
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
	game  string
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
	game := s.d.Instances.GameDir(id)
	for _, dir := range []string{"mods", "resourcepacks", "shaderpacks", "saves"} {
		_ = w.Add(filepath.Join(game, dir))
	}
	for _, f := range worldsAt(game) {
		_ = w.Add(filepath.Join(game, filepath.FromSlash(f)))
	}
	s.watch.w, s.watch.game, s.watch.dirty = w, game, map[Kind]bool{}
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
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
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
	rel, err := filepath.Rel(s.watch.game, name)
	if err != nil {
		return
	}
	first := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
	k, ok := map[string]Kind{"mods": Mods, "resourcepacks": ResourcePacks, "shaderpacks": Shaders, "saves": DataPacks}[first]
	if !ok {
		return
	}
	if k == DataPacks && s.watch.w != nil {
		// A new world: start following its datapacks folder too.
		for _, f := range worldsAt(s.watch.game) {
			_ = s.watch.w.Add(filepath.Join(s.watch.game, filepath.FromSlash(f)))
		}
	}
	s.watch.dirty[k] = true
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
