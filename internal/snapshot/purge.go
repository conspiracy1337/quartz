package snapshot

import (
	"os"
	"path/filepath"
	"time"

	"quartz/internal/content/store"
	"quartz/internal/core/safe"
)

// dirs lists the history folders of every instance, live and trashed (a
// trashed instance keeps its history until it is purged with it).
func (s *Service) dirs() []string {
	var out []string
	for _, in := range s.d.Instances.List() {
		out = append(out, s.dir(in.ID))
	}
	for _, d := range s.d.Instances.TrashDirs() {
		out = append(out, root(d))
	}
	return out
}

// Refs yields the blobs of every layer of a point that hasn't expired, for
// the store's garbage collection.
func (s *Service) Refs(yield func(sha1 string)) error {
	now := s.now()
	for _, dir := range s.dirs() {
		list := points(dir)
		newest := newestSealed(list)
		live := map[string]bool{}
		for _, p := range list {
			live[p.ID] = !s.expired(p, newest, now)
		}
		for _, l := range layers(dir) {
			if live[l.Point] {
				for _, it := range l.State.Items {
					yield(it.SHA1)
				}
			}
		}
	}
	return nil
}

// Purge deletes expired points (every unkept one when the history is
// turned off), layers no point owns and objects no layer names, then frees
// the old files only they kept.
func (s *Service) Purge(now time.Time) {
	var freed []string
	removed := 0
	for _, in := range s.d.Instances.List() {
		unlock := s.lock(in.ID)
		removed += s.purgeDir(s.dir(in.ID), now, &freed)
		unlock()
	}
	for _, d := range s.d.Instances.TrashDirs() {
		removed += s.purgeDir(root(d), now, &freed)
	}
	s.release(freed)
	if removed > 0 {
		s.d.Log.Info("history: removed expired restore points", "count", removed)
		s.emit()
	}
	s.arm()
}

// purgeDir purges one instance's history, adding the blobs of what it
// removed to freed.
func (s *Service) purgeDir(dir string, now time.Time, freed *[]string) (removed int) {
	list := points(dir)
	newest := newestSealed(list)
	live := map[string]bool{}
	for _, p := range list {
		if !s.expired(p, newest, now) {
			live[p.ID] = true
			continue
		}
		if f, err := removePoint(dir, p); err == nil {
			*freed = append(*freed, f...)
			removed++
		}
	}
	// Layers whose point is gone or unreadable can't be rolled back to.
	for _, l := range layers(dir) {
		if !live[l.Point] && os.Remove(filepath.Join(dir, layersDir, l.ID+jsonExt)) == nil {
			for _, it := range l.State.Items {
				*freed = append(*freed, it.SHA1)
			}
		}
	}
	sweep(dir)
	return removed
}

// release deletes the blobs no history or instance uses any more.
func (s *Service) release(sha1s []string) {
	if len(sha1s) == 0 || s.d.Store == nil {
		return
	}
	freed, n, err := s.d.Store.Release(store.Union(s.d.Content.Refs, s.Refs), sha1s, releaseAge)
	if err != nil {
		s.d.Log.Warn("history: freeing old files failed", "err", err)
	} else if n > 0 {
		s.d.Log.Info("history: freed old files", "files", n, "bytes", freed)
	}
}

// Start purges once and then whenever the next point expires.
func (s *Service) Start() { safe.Go(s.d.Log, "purge history", func() { s.Purge(s.now()) }) }

// Close stops the expiry timer.
func (s *Service) Close() {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
	}
}

// arm sets the timer for the next point to expire (a timer, not polling).
func (s *Service) arm() {
	now := s.now()
	var next time.Time
	for _, in := range s.d.Instances.List() {
		list := points(s.dir(in.ID))
		newest := newestSealed(list)
		for _, p := range list {
			if at := expires(p, newest); !at.IsZero() && (next.IsZero() || at.Before(next)) {
				next = at
			}
		}
	}
	s.timerMu.Lock()
	defer s.timerMu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if s.closed || next.IsZero() {
		return
	}
	s.timer = time.AfterFunc(max(next.Sub(now), time.Minute), func() {
		safe.Run(s.d.Log, "purge history", func() { s.Purge(s.now()) })
	})
}
