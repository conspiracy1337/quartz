package store

import (
	"os"
	"path/filepath"
	"time"

	"quartz/internal/core/linkx"
)

// Refs calls yield with the sha1 of every blob something still uses.
type Refs func(yield func(sha1 string)) error

// Usage sums the store.
type Usage struct {
	Blobs       int   `json:"blobs"`
	Bytes       int64 `json:"bytes"`
	Unused      int   `json:"unused"`
	UnusedBytes int64 `json:"unusedBytes"`
}

func (s *Store) referenced(refs Refs) (map[string]bool, error) {
	used := map[string]bool{}
	if err := refs(func(h string) { used[h] = true }); err != nil {
		return nil, err
	}
	s.mu.Lock()
	for h := range s.holds {
		used[h] = true
	}
	s.mu.Unlock()
	return used, nil
}

// Usage reports the store's size and how much of it nothing uses.
func (s *Store) Usage(refs Refs) (Usage, error) {
	used, err := s.referenced(refs)
	if err != nil {
		return Usage{}, err
	}
	var u Usage
	s.idx.View(func(i index) {
		for h, b := range i.Blobs {
			u.Blobs++
			u.Bytes += b.Size
			if !used[h] {
				u.Unused++
				u.UnusedBytes += b.Size
			}
		}
	})
	return u, nil
}

// GC deletes blobs nothing has referenced for longer than grace, and stale
// temp files. A blob first seen unused is only marked, so a job between
// download and placement never loses it.
func (s *Store) GC(refs Refs, grace time.Duration) (freed int64, n int, err error) {
	return s.collect(refs, func(b Blob, now time.Time) bool {
		return b.Released != 0 && now.Sub(time.Unix(b.Released, 0)) >= grace
	})
}

// Clean deletes every blob nothing references now, except blobs added in the
// last minAge (a job may be about to place them). It is the Clean up button.
func (s *Store) Clean(refs Refs, minAge time.Duration) (freed int64, n int, err error) {
	return s.collect(refs, func(b Blob, now time.Time) bool { return now.Sub(time.Unix(b.Added, 0)) >= minAge })
}

func (s *Store) collect(refs Refs, deletable func(Blob, time.Time) bool) (freed int64, n int, err error) {
	used, err := s.referenced(refs)
	if err != nil {
		return 0, 0, err
	}
	now := s.now()
	var doomed []string
	s.idx.Mutate(func(i *index) {
		for h, b := range i.Blobs {
			switch {
			case used[h]:
				b.Released = 0
			case deletable(b, now):
				doomed = append(doomed, h)
				continue
			case b.Released == 0:
				b.Released = now.Unix()
			}
			i.Blobs[h] = b
		}
	})
	for _, h := range doomed {
		p := s.Path(h)
		size := int64(0)
		if info, err := os.Stat(p); err == nil {
			size = info.Size()
		}
		_ = linkx.SetReadOnly(p, false)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			_ = linkx.SetReadOnly(p, true)
			s.log.Warn("store: could not delete an unused file", "sha1", h, "err", err)
			continue
		}
		if s.forget != nil {
			s.forget.Forget(p)
		}
		s.idx.Mutate(func(i *index) { delete(i.Blobs, h) })
		freed += size
		n++
	}
	s.sweepTemp(now)
	return freed, n, nil
}

// sweepTemp deletes temp files a crash left behind.
func (s *Store) sweepTemp(now time.Time) {
	dir := filepath.Join(s.dir, "tmp")
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > time.Hour {
			p := filepath.Join(dir, e.Name())
			_ = linkx.SetReadOnly(p, false)
			_ = os.Remove(p)
		}
	}
}
