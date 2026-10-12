package settings

import (
	"log/slog"

	"quartz/internal/core/events"
	"quartz/internal/core/jsonfile"
)

// Store holds the current settings, persists changes with a short debounce, and
// announces every change on the bus.
type Store struct {
	file *jsonfile.Store[Settings]
	bus  *events.Bus
}

// Open loads settings from path. A missing file yields defaults; a corrupt file is
// kept aside as path.corrupt and replaced with defaults instead of failing startup.
func Open(path string, bus *events.Bus, log *slog.Logger) (*Store, error) {
	f, err := jsonfile.Open(jsonfile.Options[Settings]{
		Path: path, Default: Defaults, Normalize: Settings.Normalize, Log: log,
	})
	if err != nil {
		return nil, err
	}
	return &Store{file: f, bus: bus}, nil
}

// Get returns a copy of the current settings.
func (s *Store) Get() Settings { return s.file.Get() }

// Replace validates next, stores it, schedules a save and emits SettingsChanged.
// It returns the normalized settings that were actually stored.
func (s *Store) Replace(next Settings) Settings {
	next = s.file.Set(next)
	s.bus.Emit(events.SettingsChanged, next)
	return next
}

// Flush writes any pending change immediately. Call it on shutdown.
func (s *Store) Flush() error { return s.file.Flush() }
