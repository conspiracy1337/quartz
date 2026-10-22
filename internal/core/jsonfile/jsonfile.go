// Package jsonfile persists one JSON document per file: tolerant load with a size
// cap, field-by-field normalisation, debounced crash-safe saves. Every launcher
// state file (settings, caches, accounts, instances, history) is built on it.
package jsonfile

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"sync"
	"time"

	"quartz/internal/core/fsx"
)

const (
	defaultMaxSize = 1 << 20
	defaultDelay   = 250 * time.Millisecond
)

// Options configure a Store. Only Path and Default are required.
type Options[T any] struct {
	Path      string
	Default   func() T      // value for a missing or corrupt file; also the base the file is decoded onto
	Normalize func(T) T     // optional validation run after decoding and before every store
	MaxSize   int64         // read cap; defaults to 1 MB
	Delay     time.Duration // save debounce; defaults to 250 ms
	Compact   bool          // write without indentation (large machine-only files)
	Log       *slog.Logger
}

// Store holds one document in memory and writes it back after changes.
//
// Values returned by Get are shared: callers must treat slices and maps inside
// them as read-only. Update hands its function a deep copy, so writers can
// mutate freely without racing readers.
type Store[T any] struct {
	file
	def       func() T
	normalize func(T) T
	cur       T
}

// file is the part of a Store that doesn't depend on the document's type:
// reading, the debounced save and writing. Keeping it out of Store[T] keeps
// each document type from compiling its own copy.
type file struct {
	path    string
	maxSize int64
	delay   time.Duration
	compact bool
	log     *slog.Logger

	mu    sync.Mutex // guards the document (doc points at it) and timer
	doc   any
	timer *time.Timer
	wmu   sync.Mutex // serialises file writes so an older snapshot never lands last
}

// Open loads the document. A missing file yields Default(); a corrupt one is
// kept aside as <path>.corrupt and replaced by Default() instead of failing.
// A value of the wrong type (a slip in a hand edit) only resets that field:
// the rest is kept, and a copy of the file is kept aside.
func Open[T any](o Options[T]) (*Store[T], error) {
	s := newStore(o)
	data, err := s.read()
	switch {
	case errors.Is(err, fs.ErrNotExist):
		s.cur = s.norm(s.def())
		return s, nil
	case err != nil:
		return nil, err
	}
	cur, typeErr, err := s.decode(data)
	s.decoded(data, typeErr, err)
	if err != nil {
		cur = s.norm(s.def())
	}
	s.cur = cur
	return s, nil
}

func newStore[T any](o Options[T]) *Store[T] {
	s := &Store[T]{file: newFile(o.Path, o.MaxSize, o.Delay, o.Compact, o.Log), def: o.Default, normalize: o.Normalize}
	s.doc = &s.cur
	return s
}

func newFile(path string, maxSize int64, delay time.Duration, compact bool, log *slog.Logger) file {
	if maxSize == 0 {
		maxSize = defaultMaxSize
	}
	if delay == 0 {
		delay = defaultDelay
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return file{path: path, maxSize: maxSize, delay: delay, compact: compact, log: log}
}

// Read decodes the document without keeping a store and without touching the
// file, for readers outside the running launcher (the command line). A
// missing file yields Default(); a corrupt one is an error.
func Read[T any](o Options[T]) (T, error) {
	s := newStore(o)
	data, err := s.read()
	if errors.Is(err, fs.ErrNotExist) {
		return s.norm(s.def()), nil
	}
	if err != nil {
		var zero T
		return zero, err
	}
	v, _, err := s.decode(data)
	return v, err
}

// decode starts from the defaults, so missing fields keep default values and
// unknown fields are ignored. A field of the wrong type keeps its default and
// is reported as typeErr; encoding/json still fills every other field. Only
// broken JSON is an error.
func (s *Store[T]) decode(data []byte) (v T, typeErr, err error) {
	v = s.def()
	if typeErr, err = unmarshal(data, &v); err != nil {
		var zero T
		return zero, nil, err
	}
	return s.norm(v), typeErr, nil
}

func (s *Store[T]) norm(v T) T {
	if s.normalize != nil {
		return s.normalize(v)
	}
	return v
}

// Get returns the current document (see the read-only note on Store).
func (s *Store[T]) Get() T {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// Set normalises and stores next, schedules a save, and returns what was stored.
func (s *Store[T]) Set(next T) T {
	next = s.norm(next)
	s.mu.Lock()
	s.cur = next
	s.schedule()
	s.mu.Unlock()
	return next
}

// Update applies fn to a deep copy of the document and stores the result
// atomically with respect to other updates. If fn returns an error nothing changes.
func (s *Store[T]) Update(fn func(*T) error) (T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cp T
	if err := clone(s.doc, &cp); err != nil {
		return s.cur, err
	}
	if err := fn(&cp); err != nil {
		return s.cur, err
	}
	s.cur = s.norm(cp)
	s.schedule()
	return s.cur, nil
}

// Mutate changes the document in place under the store's lock, skipping the
// deep copy Update makes and the Normalize hook. It is meant for large,
// machine-managed documents (indexes with thousands of entries) whose readers
// all go through View; never mix it with Get on the same store.
func (s *Store[T]) Mutate(fn func(*T)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.cur)
	s.schedule()
}

// View runs fn with the document under the store's lock. fn must not retain
// references into the document after it returns.
func (s *Store[T]) View(fn func(T)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(s.cur)
}

func (f *file) read() ([]byte, error) { return fsx.ReadCapped(f.path, f.maxSize) }

// decoded reports how Open's decode went: broken JSON moves the file aside,
// a field of the wrong type keeps a copy of it.
func (f *file) decoded(data []byte, typeErr, err error) {
	switch {
	case err != nil:
		f.log.Warn("state file unreadable, starting fresh", "path", f.path, "err", err)
		_ = os.Rename(f.path, f.path+".corrupt")
	case typeErr != nil:
		f.log.Warn("state file has a field of the wrong type, keeping the rest", "path", f.path, "err", typeErr)
		_ = fsx.AtomicWrite(f.path+".corrupt", data, fsx.FilePerm)
	}
}

// schedule (re)arms the debounced save; f.mu must be held.
func (f *file) schedule() {
	if f.timer == nil {
		f.timer = time.AfterFunc(f.delay, f.saveNow)
	} else {
		f.timer.Reset(f.delay)
	}
}

// Relocate runs move, which puts the file's folder somewhere else, and keeps
// the document at path from then on. Saves wait while move runs, so none
// lands in the old place; one still pending is written to the new one.
func (f *file) Relocate(path string, move func() error) error {
	f.wmu.Lock()
	defer f.wmu.Unlock()
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := move(); err != nil {
		return err
	}
	f.path = path
	return nil
}

// Flush writes any pending change immediately. Call it on shutdown.
func (f *file) Flush() error {
	f.mu.Lock()
	pending := f.timer != nil && f.timer.Stop()
	f.mu.Unlock()
	if !pending {
		return nil
	}
	return f.write()
}

func (f *file) saveNow() {
	if err := f.write(); err != nil {
		f.log.Error("saving state file failed", "path", f.path, "err", err)
	}
}

func (f *file) write() error {
	f.wmu.Lock()
	defer f.wmu.Unlock()
	f.mu.Lock()
	var (
		data []byte
		err  error
	)
	if f.compact {
		data, err = json.Marshal(f.doc)
	} else {
		data, err = json.MarshalIndent(f.doc, "", "  ")
	}
	f.mu.Unlock()
	if err != nil {
		return err
	}
	return fsx.AtomicWrite(f.path, data, fsx.FilePerm)
}

// unmarshal decodes data onto v, telling a field of the wrong type (typeErr)
// apart from broken JSON (err).
func unmarshal(data []byte, v any) (typeErr, err error) {
	if err := json.Unmarshal(data, v); err != nil {
		if _, ok := errors.AsType[*json.UnmarshalTypeError](err); !ok {
			return nil, err
		}
		return err, nil
	}
	return nil, nil
}

// clone deep-copies *from into *to through JSON.
func clone(from, to any) error {
	data, err := json.Marshal(from)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, to)
}
