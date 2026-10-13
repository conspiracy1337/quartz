package content

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"quartz/internal/content/meta"
	"quartz/internal/content/store"
	"quartz/internal/core/jsonfile"
)

// Entry is one item an instance has, as last scanned.
type Entry struct {
	Kind  Kind   `json:"kind"`
	Rel   string `json:"rel"` // slash path from the game folder, as named now
	Group string `json:"group,omitempty"`
	Dir   bool   `json:"dir,omitempty"`
	Size  int64  `json:"size"`
	ModNS int64  `json:"modNs"`
	SHA1  string `json:"sha1,omitempty"` // files only: the blob in the store
	// Info is kept here for folders (which have no blob); files use the catalog.
	Info  *meta.Info `json:"info,omitempty"`
	Added int64      `json:"added"`
}

// Update is a newer version a check found for an item.
type Update struct {
	From    string `json:"from"`
	Version string `json:"version"`
	Source  string `json:"source"`
}

// Pending holds edits to files the running game owns (it rewrites them on
// exit), applied when it stops: the wanted resource pack list (top first)
// and the wanted shader pack ("" for none).
type Pending struct {
	Packs  []string `json:"packs,omitempty"`
	Shader *string  `json:"shader,omitempty"`
}

// removal is a set of items moved aside, restorable for a while.
type removal struct {
	At    int64          `json:"at"`
	Items []removedEntry `json:"items"`
}

// removedEntry keeps what the store needs about a held item (holdbin moves it).
type removedEntry struct {
	ID    string `json:"id"`
	Entry Entry  `json:"entry"`
	Held  string `json:"held"` // where it waits, relative to the game folder
}

// Index is instances/<id>/content.json: what the last scan found, so an
// unchanged file is only stat'ed, plus the state around it.
type Index struct {
	Entries   map[string]Entry   `json:"entries"`
	Updates   map[string]Update  `json:"updates"`
	CheckedAt int64              `json:"checkedAt,omitempty"`
	Pending   Pending            `json:"pending"`
	Removed   map[string]removal `json:"removed"`
	// Missing is the first missing dependency, for Home's alert.
	Missing *Missing `json:"missing,omitempty"`
}

// Missing names a mod and the dependency it lacks.
type Missing struct {
	Mod  string `json:"mod"`
	Dep  string `json:"dep"`
	More int    `json:"more,omitempty"`
}

func newIndex() Index {
	return Index{Entries: map[string]Entry{}, Updates: map[string]Update{}, Removed: map[string]removal{}}
}

const indexFile = "content.json"

// indexes caches each instance's open index.
type indexes struct {
	mu   sync.Mutex
	open map[string]*jsonfile.Store[Index]
	dir  func(id string) string
	log  *slog.Logger
}

func (x *indexes) get(id string) (*jsonfile.Store[Index], error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if s, ok := x.open[id]; ok {
		return s, nil
	}
	s, err := jsonfile.Open(jsonfile.Options[Index]{
		Path: filepath.Join(x.dir(id), indexFile), Default: newIndex, Normalize: normalizeIndex,
		MaxSize: 32 << 20, Delay: 500 * time.Millisecond, Compact: true, Log: x.log,
	})
	if err != nil {
		return nil, err
	}
	x.open[id] = s
	return s, nil
}

// forget flushes and drops a cached index (the instance went to the trash).
func (x *indexes) forget(id string) {
	x.mu.Lock()
	s := x.open[id]
	delete(x.open, id)
	x.mu.Unlock()
	if s != nil {
		_ = s.Flush()
	}
}

// prune drops cached indexes of instances that are gone (trashed or
// restored under another id), so nothing writes into a moved folder.
func (x *indexes) prune(alive interface{ Dir(string) string }) {
	x.mu.Lock()
	var gone []string
	for id := range x.open {
		if _, err := os.Stat(alive.Dir(id)); err != nil {
			gone = append(gone, id)
		}
	}
	x.mu.Unlock()
	for _, id := range gone {
		x.mu.Lock()
		delete(x.open, id)
		x.mu.Unlock()
	}
}

func (x *indexes) flush() {
	x.mu.Lock()
	defer x.mu.Unlock()
	for _, s := range x.open {
		_ = s.Flush()
	}
}

func normalizeIndex(i Index) Index {
	if i.Entries == nil {
		i.Entries = map[string]Entry{}
	}
	if i.Updates == nil {
		i.Updates = map[string]Update{}
	}
	if i.Removed == nil {
		i.Removed = map[string]removal{}
	}
	for id, e := range i.Entries {
		if !ValidItemID(id) || (e.SHA1 != "" && !store.ValidSHA1(e.SHA1)) {
			delete(i.Entries, id)
		}
	}
	return i
}
