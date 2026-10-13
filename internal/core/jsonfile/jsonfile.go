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
	o     Options[T]
	mu    sync.Mutex
	cur   T
	timer *time.Timer
	wmu   sync.Mutex // serialises file writes so an older snapshot never lands last
}

// Open loads the document. A missing file yields Default(); a corrupt one is
// kept aside as <path>.corrupt and replaced by Default() instead of failing.
func Open[T any](o Options[T]) (*Store[T], error) {
	s := newStore(o)
	o = s.o
	data, err := fsx.ReadCapped(o.Path, o.MaxSize)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		s.cur = s.normalize(o.Default())
		return s, nil
	case err != nil:
		return nil, err
	}
	cur, err := s.decode(data)
	if err != nil {
		o.Log.Warn("state file unreadable, starting fresh", "path", o.Path, "err", err)
		_ = os.Rename(o.Path, o.Path+".corrupt")
		cur = s.normalize(o.Default())
	}
	s.cur = cur
	return s, nil
}

func newStore[T any](o Options[T]) *Store[T] {
	if o.MaxSize == 0 {
		o.MaxSize = defaultMaxSize
	}
	if o.Delay == 0 {
		o.Delay = defaultDelay
	}
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	return &Store[T]{o: o}
}

// Read decodes the document without keeping a store and without touching the
// file, for readers outside the running launcher (the command line). A
// missing file yields Default(); a corrupt one is an error.
func Read[T any](o Options[T]) (T, error) {
	s := newStore(o)
	data, err := fsx.ReadCapped(o.Path, s.o.MaxSize)
	if errors.Is(err, fs.ErrNotExist) {
		return s.normalize(o.Default()), nil
	}
	if err != nil {
		var zero T
		return zero, err
	}
	return s.decode(data)
}

// decode starts from the defaults, so missing fields keep default values and
// unknown fields are ignored.
func (s *Store[T]) decode(data []byte) (T, error) {
	v := s.o.Default()
	if err := json.Unmarshal(data, &v); err != nil {
		var zero T
		return zero, err
	}
	return s.normalize(v), nil
}

func (s *Store[T]) normalize(v T) T {
	if s.o.Normalize != nil {
		return s.o.Normalize(v)
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
	next = s.normalize(next)
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
	cp, err := clone(s.cur)
	if err != nil {
		return s.cur, err
	}
	if err := fn(&cp); err != nil {
		return s.cur, err
	}
	s.cur = s.normalize(cp)
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

// schedule (re)arms the debounced save; s.mu must be held.
func (s *Store[T]) schedule() {
	if s.timer == nil {
		s.timer = time.AfterFunc(s.o.Delay, s.saveNow)
	} else {
		s.timer.Reset(s.o.Delay)
	}
}

// Flush writes any pending change immediately. Call it on shutdown.
func (s *Store[T]) Flush() error {
	s.mu.Lock()
	pending := s.timer != nil && s.timer.Stop()
	s.mu.Unlock()
	if !pending {
		return nil
	}
	return s.write()
}

func (s *Store[T]) saveNow() {
	if err := s.write(); err != nil {
		s.o.Log.Error("saving state file failed", "path", s.o.Path, "err", err)
	}
}

func (s *Store[T]) write() error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	s.mu.Lock()
	var (
		data []byte
		err  error
	)
	if s.o.Compact {
		data, err = json.Marshal(s.cur)
	} else {
		data, err = json.MarshalIndent(s.cur, "", "  ")
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return fsx.AtomicWrite(s.o.Path, data, fsx.FilePerm)
}

func clone[T any](v T) (T, error) {
	var out T
	data, err := json.Marshal(v)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(data, &out)
	return out, err
}
