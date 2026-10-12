package download

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"quartz/internal/core/hashx"
	"quartz/internal/core/jsonfile"
)

// record is what was true about a file when it last passed its checksum.
type record struct {
	Size    int64     `json:"s"`
	ModTime int64     `json:"m"` // unix nanoseconds
	Sum     hashx.Sum `json:"h"`
}

type verifiedDoc struct {
	Files map[string]record `json:"files"` // keyed by slash path relative to the data root
}

// Verified remembers files that passed verification, so an unchanged file is
// trusted after a stat instead of a re-hash (XMCL's trick). Thousands of game
// files are then checked in milliseconds. A file whose metadata names no
// checksum at all (some loader libraries) is recorded "unhashed": trusted by
// size and modification time only, and never offered to FindBySum.
type Verified struct {
	root  string
	store *jsonfile.Store[verifiedDoc]

	mu    sync.Mutex
	bySum map[hashx.Sum]map[string]struct{} // reverse index for FindBySum
}

// OpenVerified loads the index at path for files under root.
func OpenVerified(path, root string, log *slog.Logger) (*Verified, error) {
	s, err := jsonfile.Open(jsonfile.Options[verifiedDoc]{
		Path:    path,
		Default: func() verifiedDoc { return verifiedDoc{Files: map[string]record{}} },
		Normalize: func(d verifiedDoc) verifiedDoc {
			if d.Files == nil {
				d.Files = map[string]record{}
			}
			return d
		},
		MaxSize: 128 << 20, Compact: true, Delay: 2 * time.Second, Log: log,
	})
	if err != nil {
		return nil, err
	}
	v := &Verified{root: root, store: s, bySum: map[hashx.Sum]map[string]struct{}{}}
	s.View(func(d verifiedDoc) {
		for k, rec := range d.Files {
			if !rec.Sum.IsZero() {
				v.link(rec.Sum, k)
			}
		}
	})
	return v, nil
}

func (v *Verified) link(sum hashx.Sum, k string) {
	v.mu.Lock()
	if v.bySum[sum] == nil {
		v.bySum[sum] = map[string]struct{}{}
	}
	v.bySum[sum][k] = struct{}{}
	v.mu.Unlock()
}

func (v *Verified) unlink(sum hashx.Sum, k string) {
	v.mu.Lock()
	delete(v.bySum[sum], k)
	if len(v.bySum[sum]) == 0 {
		delete(v.bySum, sum)
	}
	v.mu.Unlock()
}

// Flush writes pending changes; call on shutdown.
func (v *Verified) Flush() error { return v.store.Flush() }

func (v *Verified) key(path string) (string, bool) {
	rel, err := filepath.Rel(v.root, path)
	if err != nil || !filepath.IsLocal(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// Has reports whether path is on disk and unchanged since it was verified with
// sum. A zero sum accepts any record, hashed or not.
func (v *Verified) Has(path string, size int64, sum hashx.Sum) bool {
	k, ok := v.key(path)
	if !ok {
		return false
	}
	var rec record
	var found bool
	v.store.View(func(d verifiedDoc) { rec, found = d.Files[k] })
	if !found || (!sum.IsZero() && rec.Sum != sum) || (size > 0 && rec.Size != size) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Size() == rec.Size && info.ModTime().UnixNano() == rec.ModTime
}

// Add records path as verified against sum (zero: unhashed, see Verified).
func (v *Verified) Add(path string, sum hashx.Sum) {
	k, ok := v.key(path)
	if !ok {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	rec := record{Size: info.Size(), ModTime: info.ModTime().UnixNano(), Sum: sum}
	var old record
	var had bool
	v.store.Mutate(func(d *verifiedDoc) { old, had = d.Files[k]; d.Files[k] = rec })
	if had {
		v.unlink(old.Sum, k)
	}
	if !sum.IsZero() {
		v.link(sum, k)
	}
}

// Forget drops path, for example before a forced repair.
func (v *Verified) Forget(path string) {
	k, ok := v.key(path)
	if !ok {
		return
	}
	var old record
	var had bool
	v.store.Mutate(func(d *verifiedDoc) { old, had = d.Files[k]; delete(d.Files, k) })
	if had {
		v.unlink(old.Sum, k)
	}
}

// FindBySum returns another verified, unchanged file with the same content.
func (v *Verified) FindBySum(sum hashx.Sum, size int64) (string, bool) {
	if sum.IsZero() {
		return "", false
	}
	v.mu.Lock()
	candidates := make([]string, 0, len(v.bySum[sum]))
	for k := range v.bySum[sum] {
		candidates = append(candidates, k)
	}
	v.mu.Unlock()
	for _, k := range candidates {
		p := filepath.Join(v.root, filepath.FromSlash(k))
		if v.Has(p, size, sum) {
			return p, true
		}
	}
	return "", false
}
