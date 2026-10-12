package settings

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"sync"
	"time"

	"quartz/internal/core/events"
	"quartz/internal/core/fsx"
)

const (
	maxFileSize = 1 << 20
	saveDelay   = 250 * time.Millisecond
)

// Store holds the current settings, persists changes with a short debounce, and
// announces every change on the bus.
type Store struct {
	mu    sync.Mutex
	path  string
	cur   Settings
	timer *time.Timer
	bus   *events.Bus
	log   *slog.Logger
}

// Open loads settings from path. A missing file yields defaults; a corrupt file is
// kept aside as path.corrupt and replaced with defaults instead of failing startup.
func Open(path string, bus *events.Bus, log *slog.Logger) (*Store, error) {
	s := &Store{path: path, bus: bus, log: log}
	data, err := fsx.ReadCapped(path, maxFileSize)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		s.cur = Defaults()
		return s, nil
	case err != nil:
		return nil, err
	}
	cur, err := decode(data)
	if err != nil {
		log.Warn("settings file unreadable, using defaults", "err", err)
		_ = os.Rename(path, path+".corrupt")
		cur = Defaults()
	}
	s.cur = cur
	return s, nil
}

// decode starts from the defaults so missing fields keep their default values and
// unknown fields are ignored, then validates every field.
func decode(data []byte) (Settings, error) {
	s := Defaults()
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, err
	}
	return s.Normalize(), nil
}

// Get returns a copy of the current settings.
func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// Replace validates next, stores it, schedules a save and emits SettingsChanged.
// It returns the normalized settings that were actually stored.
func (s *Store) Replace(next Settings) Settings {
	next = next.Normalize()
	s.mu.Lock()
	s.cur = next
	if s.timer == nil {
		s.timer = time.AfterFunc(saveDelay, s.saveNow)
	} else {
		s.timer.Reset(saveDelay)
	}
	s.mu.Unlock()
	s.bus.Emit(events.SettingsChanged, next)
	return next
}

// Flush writes any pending change immediately. Call it on shutdown.
func (s *Store) Flush() error {
	s.mu.Lock()
	pending := s.timer != nil && s.timer.Stop()
	s.mu.Unlock()
	if !pending {
		return nil
	}
	return s.write()
}

func (s *Store) saveNow() {
	if err := s.write(); err != nil {
		s.log.Error("saving settings failed", "err", err)
	}
}

func (s *Store) write() error {
	s.mu.Lock()
	data, err := json.MarshalIndent(s.cur, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return fsx.AtomicWrite(s.path, data, fsx.FilePerm)
}
