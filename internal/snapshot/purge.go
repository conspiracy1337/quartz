package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"quartz/internal/content"
	"quartz/internal/content/store"
	"quartz/internal/core/safe"
)

// dirs lists the snapshot folders of every instance, live and trashed
// (a trashed instance keeps its snapshots until it is purged with it).
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

// Refs yields the blobs of every snapshot that hasn't expired, for the
// store's garbage collection.
func (s *Service) Refs(yield func(sha1 string)) error {
	now := s.now()
	for _, dir := range s.dirs() {
		for _, sn := range list(dir) {
			if s.expired(sn, now) {
				continue
			}
			for _, it := range sn.State.Items {
				yield(it.SHA1)
			}
		}
	}
	return nil
}

// Purge deletes expired snapshots (all not kept when snapshots are turned
// off) and snapshots a crash left half written, then frees the old files
// only they kept.
func (s *Service) Purge(now time.Time) {
	var freed []content.StateItem
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
		s.d.Log.Info("snapshots: removed expired snapshots", "count", removed)
		s.emit()
	}
	s.arm()
}

// purgeDir purges one instance's snapshots, adding the items of the ones
// it removed to freed.
func (s *Service) purgeDir(dir string, now time.Time, freed *[]content.StateItem) (removed int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if base, ok := strings.CutSuffix(e.Name(), tmpExt); ok && ValidID(base) {
			if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > time.Hour {
				_ = os.RemoveAll(p)
			}
			continue
		}
		if !e.IsDir() || !ValidID(e.Name()) {
			continue
		}
		sn, err := read(dir, e.Name())
		if err != nil {
			_ = os.RemoveAll(p) // unreadable: it can't be rolled back to anyway
			continue
		}
		if s.expired(sn, now) && os.RemoveAll(p) == nil {
			*freed = append(*freed, sn.State.Items...)
			removed++
		}
	}
	return removed
}

// release deletes the blobs of items no snapshot or instance uses any more.
func (s *Service) release(items []content.StateItem) {
	if len(items) == 0 || s.d.Store == nil {
		return
	}
	sha1s := make([]string, len(items))
	for i, it := range items {
		sha1s[i] = it.SHA1
	}
	freed, n, err := s.d.Store.Release(store.Union(s.d.Content.Refs, s.Refs), sha1s, releaseAge)
	if err != nil {
		s.d.Log.Warn("snapshots: freeing old files failed", "err", err)
	} else if n > 0 {
		s.d.Log.Info("snapshots: freed old files", "files", n, "bytes", freed)
	}
}

// Start purges once and then whenever the next snapshot expires.
func (s *Service) Start() { safe.Go(s.d.Log, "purge snapshots", func() { s.Purge(s.now()) }) }

// Close stops the expiry timer.
func (s *Service) Close() {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
	}
}

// arm sets the timer for the next snapshot to expire (a timer, not polling).
func (s *Service) arm() {
	now := s.now()
	var next time.Time
	for _, in := range s.d.Instances.List() {
		for _, sn := range list(s.dir(in.ID)) {
			if sn.Keep {
				continue
			}
			if at := time.UnixMilli(sn.At).Add(Life); next.IsZero() || at.Before(next) {
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
		safe.Run(s.d.Log, "purge snapshots", func() { s.Purge(s.now()) })
	})
}
