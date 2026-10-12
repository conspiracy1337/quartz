// Package instance stores instances: one folder each, holding instance.json,
// the game folder and launch history. An instance's id is its identity and
// its folder name, so renaming never moves files (unlike Pandora, where the
// folder name was the identity). This milestone covers the vanilla lifecycle;
// groups, trash, import and loaders follow in M4.
package instance

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"quartz/internal/core/fsx"
	"quartz/internal/core/jsonfile"
)

// Art is the instance tile.
type Art struct {
	Background string `json:"background"`
	Initial    string `json:"initial,omitempty"`
}

// JavaSettings override the global Java settings when Override is set; the
// values are kept while off so the dimmed card still shows them.
type JavaSettings struct {
	Override    bool   `json:"override"`
	JavaPath    string `json:"javaPath"` // "" picks automatically
	MemoryMinMB int    `json:"memoryMinMb"`
	MemoryMaxMB int    `json:"memoryMaxMb"`
	JVMArgs     string `json:"jvmArgs"`
}

// WindowSettings configure the game window. Zero size keeps the game's default.
type WindowSettings struct {
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Fullscreen  bool   `json:"fullscreen"`
	OnGameStart string `json:"onGameStart"` // "" follows the global setting
}

// Settings are the per-instance settings.
type Settings struct {
	Java   JavaSettings   `json:"java"`
	Window WindowSettings `json:"window"`
}

// Instance is one instance's record.
type Instance struct {
	Version     int        `json:"version"`
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Art         Art        `json:"art"`
	Minecraft   string     `json:"minecraft"`
	Loader      string     `json:"loader,omitempty"` // "" is vanilla
	Group       string     `json:"group,omitempty"`
	Sandbox     bool       `json:"sandbox"`
	Shares      []string   `json:"shares,omitempty"` // stored now, used by syncing (M8)
	Created     time.Time  `json:"created"`
	LastPlayed  *time.Time `json:"lastPlayed,omitempty"`
	PlaytimeMin int        `json:"playtimeMin"`
	Settings    Settings   `json:"settings"`
}

// Errors returned to the UI.
var (
	ErrUnknown           = errors.New("instance: no such instance")
	ErrBadName           = errors.New("instance: names need 1 to 64 characters")
	ErrLoaderUnsupported = errors.New("instance: mod loaders arrive in a later update; pick None for now")
	ErrInvalidSettings   = errors.New("instance: invalid settings")
)

var (
	idRe      = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)
	hexColor  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	versionRe = regexp.MustCompile(`^[A-Za-z0-9._ +-]{1,64}$`)
)

// ValidID reports whether id can name an instance.
func ValidID(id string) bool { return idRe.MatchString(id) }

const (
	maxName    = 64
	maxJVMArgs = 4096
	maxMemory  = 1 << 20
)

// cleanName trims and rejects control characters.
func cleanName(n string) (string, error) {
	n = strings.TrimSpace(n)
	if n == "" || len([]rune(n)) > maxName || strings.ContainsFunc(n, unicode.IsControl) {
		return "", ErrBadName
	}
	return n, nil
}

// normalize repairs a hand-edited file field by field.
func normalize(in Instance) Instance {
	if !hexColor.MatchString(in.Art.Background) {
		in.Art.Background = "#6b8afd"
	}
	if r := []rune(in.Art.Initial); len(r) > 2 {
		in.Art.Initial = string(r[:2])
	}
	if n, err := cleanName(in.Name); err == nil {
		in.Name = n
	} else {
		in.Name = in.ID
	}
	j := &in.Settings.Java
	j.MemoryMinMB = min(max(j.MemoryMinMB, 0), maxMemory)
	j.MemoryMaxMB = min(max(j.MemoryMaxMB, 0), maxMemory)
	if j.MemoryMaxMB != 0 && j.MemoryMaxMB < j.MemoryMinMB {
		j.MemoryMaxMB = j.MemoryMinMB
	}
	if len(j.JVMArgs) > maxJVMArgs {
		j.JVMArgs = ""
	}
	if j.JavaPath != "" && (!filepath.IsAbs(j.JavaPath) || len(j.JavaPath) > 4096) {
		j.JavaPath = ""
	}
	w := &in.Settings.Window
	w.Width, w.Height = min(max(w.Width, 0), 16384), min(max(w.Height, 0), 16384)
	if !slices.Contains([]string{"", "keep-open", "hide", "close"}, w.OnGameStart) {
		w.OnGameStart = ""
	}
	in.PlaytimeMin = max(in.PlaytimeMin, 0)
	return in
}

// Store holds every instance.
type Store struct {
	dir string
	log *slog.Logger

	mu    sync.Mutex
	files map[string]*jsonfile.Store[Instance]
}

// Open loads every instance folder in dir. Folders whose name is not a valid
// id, or whose instance.json names another id, are skipped.
func Open(dir string, log *slog.Logger) (*Store, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	s := &Store{dir: dir, log: log, files: map[string]*jsonfile.Store[Instance]{}}
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() || !ValidID(e.Name()) {
			continue
		}
		f, err := s.open(e.Name())
		if err != nil || f.Get().ID != e.Name() {
			log.Warn("skipping instance folder", "folder", e.Name(), "err", err)
			continue
		}
		s.files[e.Name()] = f
	}
	return s, nil
}

func (s *Store) open(id string) (*jsonfile.Store[Instance], error) {
	return jsonfile.Open(jsonfile.Options[Instance]{
		Path: filepath.Join(s.dir, id, "instance.json"), Default: func() Instance { return Instance{Version: 1} },
		Normalize: normalize, Log: s.log,
	})
}

// Dir is the instance's folder.
func (s *Store) Dir(id string) string { return filepath.Join(s.dir, id) }

// GameDir is the folder the game runs in (.minecraft).
func (s *Store) GameDir(id string) string { return filepath.Join(s.dir, id, "game") }

// RunsDir holds the instance's launch history.
func (s *Store) RunsDir(id string) string { return filepath.Join(s.dir, id, "runs") }

// List returns all instances, most recently played first, never-played last
// by creation date.
func (s *Store) List() []Instance {
	s.mu.Lock()
	out := make([]Instance, 0, len(s.files))
	for _, f := range s.files {
		out = append(out, f.Get())
	}
	s.mu.Unlock()
	slices.SortFunc(out, func(a, b Instance) int { return lastActive(b).Compare(lastActive(a)) })
	return out
}

func lastActive(i Instance) time.Time {
	if i.LastPlayed != nil {
		return *i.LastPlayed
	}
	return i.Created
}

// Get returns one instance.
func (s *Store) Get(id string) (Instance, error) {
	s.mu.Lock()
	f, ok := s.files[id]
	s.mu.Unlock()
	if !ok {
		return Instance{}, ErrUnknown
	}
	return f.Get(), nil
}

// CreateRequest is what the New instance screen sends.
type CreateRequest struct {
	Name      string   `json:"name"`
	Art       Art      `json:"art"`
	Minecraft string   `json:"minecraft"`
	Loader    string   `json:"loader"`
	Group     string   `json:"group"`
	Sandbox   bool     `json:"sandbox"`
	Shares    []string `json:"shares"`
}

// Create makes a new instance folder. The Minecraft version must already be
// checked against the manifest by the caller; defaults seed the Java card.
func (s *Store) Create(req CreateRequest, defaults JavaSettings) (Instance, error) {
	name, err := cleanName(req.Name)
	if err != nil {
		return Instance{}, err
	}
	if req.Loader != "" && req.Loader != "none" {
		return Instance{}, ErrLoaderUnsupported
	}
	if !versionRe.MatchString(req.Minecraft) {
		return Instance{}, fmt.Errorf("%w: version", ErrInvalidSettings)
	}
	shares := make([]string, 0, len(req.Shares))
	for _, sh := range req.Shares {
		if len(sh) <= 64 && !strings.ContainsFunc(sh, unicode.IsControl) {
			shares = append(shares, sh)
		}
	}
	defaults.Override = false
	in := Instance{
		Version: 1, ID: s.newID(name), Name: name, Art: req.Art, Minecraft: req.Minecraft,
		Group: strings.TrimSpace(req.Group), Sandbox: req.Sandbox, Shares: shares, Created: time.Now().UTC(),
		Settings: Settings{Java: defaults},
	}
	if in.Art.Initial == "" {
		in.Art.Initial = strings.ToUpper(string([]rune(name)[:1]))
	}
	if err := os.MkdirAll(s.GameDir(in.ID), 0o700); err != nil {
		return Instance{}, err
	}
	f, err := s.open(in.ID)
	if err != nil {
		return Instance{}, err
	}
	in = f.Set(in)
	if err := f.Flush(); err != nil {
		return Instance{}, err
	}
	s.mu.Lock()
	s.files[in.ID] = f
	s.mu.Unlock()
	return in, nil
}

// newID builds "<slug>-<4 hex>" from the name; it never changes afterwards.
func (s *Store) newID(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 40 {
			break
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "instance"
	}
	for {
		var r [2]byte
		_, _ = rand.Read(r[:])
		id := slug + "-" + hex.EncodeToString(r[:])
		if _, err := os.Stat(s.Dir(id)); errors.Is(err, os.ErrNotExist) {
			return id
		}
	}
}

// Update changes an instance atomically; fn may not change the id.
func (s *Store) Update(id string, fn func(*Instance) error) (Instance, error) {
	s.mu.Lock()
	f, ok := s.files[id]
	s.mu.Unlock()
	if !ok {
		return Instance{}, ErrUnknown
	}
	return f.Update(func(in *Instance) error {
		if err := fn(in); err != nil {
			return err
		}
		in.ID = id
		return nil
	})
}

// SetSettings replaces an instance's settings after validation.
func (s *Store) SetSettings(id string, st Settings) (Instance, error) {
	if st.Java.MemoryMinMB < 0 || st.Java.MemoryMaxMB < 0 || len(st.Java.JVMArgs) > maxJVMArgs ||
		(st.Java.JavaPath != "" && !filepath.IsAbs(st.Java.JavaPath)) {
		return Instance{}, ErrInvalidSettings
	}
	return s.Update(id, func(in *Instance) error { in.Settings = st; return nil })
}

// Flush writes pending changes of every instance.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	for _, f := range s.files {
		errs = append(errs, f.Flush())
	}
	return errors.Join(errs...)
}

// GameDirSafe is GameDir for an id that came from outside, checked first.
func (s *Store) GameDirSafe(id string) (string, error) {
	if !ValidID(id) {
		return "", ErrUnknown
	}
	return fsx.SafeJoin(s.dir, filepath.Join(id, "game"))
}
