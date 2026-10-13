// Package disklru keeps a cache folder under a size: once it grows past its
// limit, the least recently used files go until it is at 3/4 of it. A file's
// modification time is its "last used" mark, refreshed at most hourly so a
// busy cache doesn't rewrite metadata on every hit.
package disklru

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const touchEvery = time.Hour

// LRU is safe for concurrent use.
type LRU struct {
	dir string
	max int64

	mu    sync.Mutex
	total int64 // bytes on disk, -1 until first counted
}

// New limits dir to max bytes.
func New(dir string, max int64) *LRU { return &LRU{dir: dir, max: max, total: -1} }

// Dir is the cache folder.
func (l *LRU) Dir() string { return l.dir }

// Hit returns path when it exists, marking it used ("" when missing).
func (l *LRU) Hit(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return ""
	}
	if now := time.Now(); now.Sub(fi.ModTime()) > touchEvery {
		_ = os.Chtimes(path, now, now)
	}
	return path
}

// Added counts n new bytes and evicts the least recently used files once the
// folder is over its limit.
func (l *LRU) Added(n int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.total < 0 {
		l.total = 0
		for _, f := range l.files() {
			l.total += f.size
		}
	} else {
		l.total += n
	}
	if l.total <= l.max {
		return
	}
	files := l.files()
	sort.Slice(files, func(i, j int) bool { return files[i].used.Before(files[j].used) })
	for _, f := range files {
		if l.total <= l.max*3/4 {
			break
		}
		if os.Remove(f.path) == nil {
			l.total -= f.size
		}
	}
}

type entry struct {
	path string
	size int64
	used time.Time
}

func (l *LRU) files() []entry {
	var out []entry
	_ = filepath.WalkDir(l.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // an unreadable entry is skipped, not fatal
		}
		if fi, err := d.Info(); err == nil {
			out = append(out, entry{p, fi.Size(), fi.ModTime()})
		}
		return nil
	})
	return out
}
