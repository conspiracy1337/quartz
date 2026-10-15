package settings

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"

	"quartz/internal/core/events"
	"quartz/internal/core/jsonfile"
	"quartz/internal/core/sysinfo"
)

// Store holds the current settings, persists changes with a short debounce, and
// announces every change on the bus.
type Store struct {
	file *jsonfile.Store[Settings]
	bus  *events.Bus
}

// Open loads settings from path. A missing file yields defaults; a corrupt file is
// kept aside as path.corrupt and replaced with defaults instead of failing startup.
// A new data folder (no settings file yet) gets a memory limit suited to
// the computer.
func Open(path string, bus *events.Bus, log *slog.Logger) (*Store, error) {
	_, statErr := os.Stat(path)
	f, err := jsonfile.Open(jsonfile.Options[Settings]{
		Path: path, Default: Defaults, Normalize: Settings.Normalize, Log: log,
	})
	if err != nil {
		return nil, err
	}
	if errors.Is(statErr, fs.ErrNotExist) {
		s := f.Get()
		if mem, err := sysinfo.ReadMemory(); err == nil {
			s.Java.MemoryMaxMB = SuggestedMaxMemoryMB(mem.Total, s.Java.MemoryMaxMB)
		}
		f.Set(s)
	}
	return &Store{file: f, bus: bus}, nil
}

// SuggestedMaxMemoryMB is the default most memory a game may use on a computer
// with total bytes of RAM: two thirds of it, in steps of 512 MB, between 1 GB
// and def (Prism's rule; a 4 GB laptop gets 2.5 GB instead of all of it). An
// unknown total keeps def.
func SuggestedMaxMemoryMB(total uint64, def int) int {
	if total == 0 {
		return def
	}
	mb := int(total/(1<<20)) * 2 / 3 / 512 * 512 //nolint:gosec // RAM in MB fits an int
	return min(max(mb, 1024), def)
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
