package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"quartz/internal/core/events"
)

// Files dropped on the window reach Go, never the UI: the UI only learns a
// drop id and the names, then asks for the drop to be added to a tab. The
// paths are still untrusted (page script can fake a drop event), so the
// content service checks every one as it would a file someone typed in.
const (
	dropTTL   = 2 * time.Minute
	dropFiles = 64
)

// ErrDropExpired means the drop id is unknown, used, or too old.
var ErrDropExpired = errors.New("api: drop the files again")

var dropIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Dropped is the content:dropped payload.
type Dropped struct {
	DropID string   `json:"dropId"`
	Count  int      `json:"count"`
	Names  []string `json:"names"`
}

// Drops holds dropped paths for a short while, each drop usable once.
type Drops struct {
	bus *events.Bus
	now func() time.Time

	mu    sync.Mutex
	drops map[string]drop
}

type drop struct {
	paths []string
	at    time.Time
}

// NewDrops returns an empty stash.
func NewDrops(bus *events.Bus) *Drops {
	return &Drops{bus: bus, now: time.Now, drops: map[string]drop{}}
}

// Receive is Wails' OnFileDrop callback.
func (d *Drops) Receive(_, _ int, paths []string) {
	if len(paths) == 0 {
		return
	}
	if len(paths) > dropFiles {
		paths = paths[:dropFiles]
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return
	}
	id := hex.EncodeToString(b[:])
	names := make([]string, len(paths))
	for i, p := range paths {
		names[i] = filepath.Base(p)
	}
	d.mu.Lock()
	for k, v := range d.drops {
		if d.now().Sub(v.at) > dropTTL {
			delete(d.drops, k)
		}
	}
	d.drops[id] = drop{paths: append([]string(nil), paths...), at: d.now()}
	d.mu.Unlock()
	d.bus.Emit(events.ContentDropped, Dropped{DropID: id, Count: len(paths), Names: names})
}

// Take returns a drop's paths once.
func (d *Drops) Take(id string) ([]string, error) {
	if !dropIDRe.MatchString(id) {
		return nil, ErrDropExpired
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	v, ok := d.drops[id]
	delete(d.drops, id)
	if !ok || d.now().Sub(v.at) > dropTTL {
		return nil, ErrDropExpired
	}
	return asListErr(v.paths, nil)
}
