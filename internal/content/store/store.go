// Package store is the content layer of the layered instance design: a
// content-addressed, read-only folder of mods and packs, each kept once by
// its sha1 however many instances use it. Instances get their files from it
// by reflink or hardlink (a copy only across volumes), never by symlink.
// Blobs are only ever added or deleted, never edited, which is what makes a
// hardlink safe: no instance can change the bytes another one reads.
package store

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"quartz/internal/core/hashx"
	"quartz/internal/core/jsonfile"
	"quartz/internal/core/linkx"
)

// ErrCollision means two different files share a sha1; the newcomer is
// refused rather than deduplicated.
var ErrCollision = errors.New("store: a different file with the same sha1 is already stored")

// Blob is what the index knows about one stored file.
type Blob struct {
	Size    int64  `json:"size"`
	SHA512  string `json:"sha512"`
	Murmur2 uint32 `json:"murmur2"`
	Added   int64  `json:"added"`
	// Released is when the blob was first seen unreferenced (unix seconds);
	// GC deletes it once that is older than the grace period.
	Released int64 `json:"released,omitempty"`
}

type index struct {
	Blobs map[string]Blob `json:"blobs"`
}

// Forgetter drops a deleted path from the download verified index.
type Forgetter interface{ Forget(path string) }

// Store is the content store in one folder.
type Store struct {
	dir    string
	idx    *jsonfile.Store[index]
	forget Forgetter
	log    *slog.Logger
	now    func() time.Time

	mu    sync.Mutex
	holds map[string]int
}

// Seams for tests, which simulate another volume by failing them.
var (
	linkFn  = os.Link
	shareFn = func(blob, dst string) (linkx.Method, error) {
		return linkx.Place(blob, dst, linkx.Options{NoCopy: true, Perm: 0o600})
	}
)

var sha1Re = regexp.MustCompile(`^[0-9a-f]{40}$`)

// ValidSHA1 reports whether s names a blob.
func ValidSHA1(s string) bool { return sha1Re.MatchString(s) }

// Open opens (creating) the store in dir. forget may be nil.
func Open(dir string, forget Forgetter, log *slog.Logger) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	idx, err := jsonfile.Open(jsonfile.Options[index]{
		Path:    filepath.Join(dir, "index.json"),
		Default: func() index { return index{Blobs: map[string]Blob{}} },
		Normalize: func(i index) index {
			if i.Blobs == nil {
				i.Blobs = map[string]Blob{}
			}
			for k := range i.Blobs {
				if !ValidSHA1(k) {
					delete(i.Blobs, k)
				}
			}
			return i
		},
		MaxSize: 64 << 20, Delay: 2 * time.Second, Compact: true, Log: log,
	})
	if err != nil {
		return nil, err
	}
	return &Store{dir: dir, idx: idx, forget: forget, log: log, now: time.Now, holds: map[string]int{}}, nil
}

// Flush writes the index now.
func (s *Store) Flush() error { return s.idx.Flush() }

// Path is where the blob with this sha1 lives (whether or not it exists).
func (s *Store) Path(sha1 string) string { return filepath.Join(s.dir, sha1[:2], sha1) }

// Blob returns what the index knows about a stored blob.
func (s *Store) Blob(sha1 string) (Blob, bool) {
	var b Blob
	var ok bool
	s.idx.View(func(i index) { b, ok = i.Blobs[sha1] })
	if ok {
		if _, err := os.Stat(s.Path(sha1)); err != nil {
			ok = false
		}
	}
	return b, ok
}

func (s *Store) record(h hashx.Multi) {
	s.idx.Mutate(func(i *index) {
		b, ok := i.Blobs[h.SHA1]
		if !ok {
			b = Blob{Size: h.Size, SHA512: h.SHA512, Murmur2: h.Murmur2, Added: s.now().Unix()}
		}
		b.Released = 0
		i.Blobs[h.SHA1] = b
	})
}

// Adopt takes a file a player put into an instance into the store. If the
// content is stored already, the file is replaced by a link to the blob;
// otherwise the file itself becomes the blob (a hardlink, so nothing is
// copied) and is sealed read-only. On another volume the file is left as
// it is, still recorded, so its blob exists for every other instance.
// h must be the hashes of the file as it is now.
func (s *Store) Adopt(path string, h hashx.Multi) (linkx.Method, error) {
	if !ValidSHA1(h.SHA1) {
		return linkx.Copy, fmt.Errorf("store: bad sha1 %q", h.SHA1)
	}
	if b, ok := s.Blob(h.SHA1); ok {
		if b.SHA512 != "" && h.SHA512 != "" && b.SHA512 != h.SHA512 {
			return linkx.Copy, ErrCollision
		}
		s.record(h)
		return s.share(h.SHA1, path)
	}
	if err := s.create(h, func(tmp string) (linkx.Method, error) {
		if err := linkFn(path, tmp); err == nil {
			return linkx.Hardlink, nil
		}
		return linkx.Place(path, tmp, linkx.Options{NoHardlink: true})
	}); err != nil {
		return linkx.Copy, err
	}
	// Linking failed (another volume) or another adopter won the race.
	return s.share(h.SHA1, path)
}

// share makes path a link or clone of the blob; on another volume, where
// only a copy would be possible, the player's file is kept as it is.
func (s *Store) share(sha1, path string) (linkx.Method, error) {
	if same(path, s.Path(sha1)) {
		return linkx.Hardlink, nil
	}
	m, err := shareFn(s.Path(sha1), path)
	if errors.Is(err, linkx.ErrUnsupported) {
		return linkx.Copy, nil
	}
	return m, err
}

// Ingest copies a file from outside the launcher (a dropped file) into the
// store and returns its hashes. The source is never linked: it isn't ours.
func (s *Store) Ingest(src string) (hashx.Multi, error) {
	tmp, err := s.temp()
	if err != nil {
		return hashx.Multi{}, err
	}
	defer func() { _ = os.Remove(tmp) }()
	if _, err := linkx.Place(src, tmp, linkx.Options{NoHardlink: true, Perm: 0o600}); err != nil {
		return hashx.Multi{}, err
	}
	h, err := hashx.MultiFile(tmp)
	if err != nil {
		return hashx.Multi{}, err
	}
	if b, ok := s.Blob(h.SHA1); ok {
		if b.SHA512 != h.SHA512 {
			return hashx.Multi{}, ErrCollision
		}
		s.record(h)
		return h, nil
	}
	return h, s.create(h, func(t string) (linkx.Method, error) { return linkx.Hardlink, os.Rename(tmp, t) })
}

// Seal records a blob a download just wrote at Path(sha1) and makes it
// read-only. The download already verified it against its published hash.
func (s *Store) Seal(sha1 string) (hashx.Multi, error) {
	if !ValidSHA1(sha1) {
		return hashx.Multi{}, fmt.Errorf("store: bad sha1 %q", sha1)
	}
	p := s.Path(sha1)
	h, err := hashx.MultiFile(p)
	if err != nil {
		return hashx.Multi{}, err
	}
	if h.SHA1 != sha1 {
		return hashx.Multi{}, fmt.Errorf("store: %s has sha1 %s: %w", sha1, h.SHA1, hashx.ErrMismatch)
	}
	if err := linkx.SetReadOnly(p, true); err != nil {
		return hashx.Multi{}, err
	}
	s.record(h)
	return h, nil
}

// Place puts a blob at dst (reflink, hardlink, or a copy across volumes).
func (s *Store) Place(sha1, dst string) (linkx.Method, error) {
	if !ValidSHA1(sha1) {
		return linkx.Copy, fmt.Errorf("store: bad sha1 %q", sha1)
	}
	return linkx.Place(s.Path(sha1), dst, linkx.Options{Perm: 0o600})
}

// PlacePrivate puts a blob at dst as a private clone or copy, never a
// hardlink: for folders other programs own, which must not get read-only
// files.
func (s *Store) PlacePrivate(sha1, dst string) (linkx.Method, error) {
	if !ValidSHA1(sha1) {
		return linkx.Copy, fmt.Errorf("store: bad sha1 %q", sha1)
	}
	return linkx.Place(s.Path(sha1), dst, linkx.Options{Perm: 0o600, NoHardlink: true})
}

// Unlink deletes an instance's file. On Windows deleting a read-only file
// clears the attribute every hardlink shares, so the blob is sealed again.
func (s *Store) Unlink(path, sha1 string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return s.Reseal(sha1)
}

// Reseal makes a blob read-only again after a link to it was removed or
// replaced (hardlinks share the flag, so clearing it on one cleared it on all).
func (s *Store) Reseal(sha1 string) error {
	if ValidSHA1(sha1) {
		if _, err := os.Stat(s.Path(sha1)); err == nil {
			return linkx.SetReadOnly(s.Path(sha1), true)
		}
	}
	return nil
}

// Hold keeps blobs from being collected until release is called (a job
// that downloaded them but hasn't placed them yet).
func (s *Store) Hold(sha1s ...string) (release func()) {
	s.mu.Lock()
	for _, h := range sha1s {
		s.holds[h]++
	}
	s.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			for _, h := range sha1s {
				if s.holds[h]--; s.holds[h] <= 0 {
					delete(s.holds, h)
				}
			}
			s.mu.Unlock()
		})
	}
}

// create makes the blob for h from a temp file fill writes, sealing it and
// linking it into place only if no blob exists yet (create-if-absent).
func (s *Store) create(h hashx.Multi, fill func(tmp string) (linkx.Method, error)) error {
	dst := s.Path(h.SHA1)
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	tmp, err := s.temp()
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp) }()
	if _, err := fill(tmp); err != nil {
		return err
	}
	// A copy must still be the file that was hashed.
	if got, err := hashx.MultiFile(tmp); err != nil {
		return err
	} else if got.SHA1 != h.SHA1 || got.SHA512 != h.SHA512 {
		return fmt.Errorf("store: the file changed while it was stored: %w", hashx.ErrMismatch)
	}
	if err := linkx.SetReadOnly(tmp, true); err != nil {
		return err
	}
	if err := os.Link(tmp, dst); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	s.record(h)
	return nil
}

func (s *Store) temp() (string, error) {
	dir := filepath.Join(s.dir, "tmp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "blob-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	_ = f.Close()
	return name, os.Remove(name)
}

func same(a, b string) bool {
	ia, err := os.Stat(a)
	if err != nil {
		return false
	}
	ib, err := os.Stat(b)
	return err == nil && os.SameFile(ia, ib)
}
