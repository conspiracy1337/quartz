// Package instance stores instances: one folder each, holding instance.json,
// the game folder and launch history. An instance's id is its identity and
// its folder name, so renaming never moves files (unlike Pandora, where the
// folder name was the identity).
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

	"quartz/internal/core/jsonfile"
	"quartz/internal/loader"
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

// CommandSettings run the player's own programs around the game. Each command
// is split into words like a shell would, but no shell ever runs it; $VAR
// references (the $INST_* values and the environment) are expanded per word.
type CommandSettings struct {
	PreLaunch    string   `json:"preLaunch"` // must succeed before the game starts
	Wrapper      string   `json:"wrapper"`   // put in front of java, e.g. "prime-run"
	PostExit     string   `json:"postExit"`  // runs after the game closes, with $EXIT_CODE
	Env          []EnvVar `json:"env"`
	SystemGLFW   bool     `json:"systemGlfw"`
	SystemOpenAL bool     `json:"systemOpenal"`
}

// EnvVar is one extra environment variable for the game.
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// PerformanceSettings are the Linux performance helpers.
type PerformanceSettings struct {
	DedicatedGPU bool `json:"dedicatedGpu"`
	GameMode     bool `json:"gameMode"`
	MangoHud     bool `json:"mangoHud"`
}

// Settings are the per-instance settings. The Java card overrides the global
// Java settings as a whole when its Override is on (the design's per-card
// switch); the window falls back field by field; the rest is the instance's own.
type Settings struct {
	Java        JavaSettings        `json:"java"`
	Window      WindowSettings      `json:"window"`
	Commands    CommandSettings     `json:"commands"`
	Performance PerformanceSettings `json:"performance"`
}

// Instance is one instance's record.
type Instance struct {
	Version       int    `json:"version"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Art           Art    `json:"art"`
	Icon          string `json:"icon,omitempty"` // sha256 of a picture in the icon library; "" draws Art
	Minecraft     string `json:"minecraft"`
	Loader        string `json:"loader,omitempty"` // "" is vanilla
	LoaderVersion string `json:"loaderVersion,omitempty"`
	// KeepLoaderUpdated installs the newest stable loader build at launch.
	KeepLoaderUpdated bool `json:"keepLoaderUpdated"`
	// FollowLatest moves to the newest Minecraft release once every mod
	// supports it (checked from M5 on; stored now).
	FollowLatest bool       `json:"followLatest"`
	Group        string     `json:"group,omitempty"`
	Notes        string     `json:"notes,omitempty"`
	Tags         []string   `json:"tags,omitempty"`
	Location     string     `json:"location,omitempty"` // a moved game folder; "" is instances/<id>/game
	Sandbox      bool       `json:"sandbox"`
	Shares       []string   `json:"shares,omitempty"` // stored now, used by syncing (M8)
	Created      time.Time  `json:"created"`
	LastPlayed   *time.Time `json:"lastPlayed,omitempty"`
	PlaytimeMin  int        `json:"playtimeMin"`
	Settings     Settings   `json:"settings"`
}

// schema is the instance.json version this build writes. Version 1 files
// (M2 and M3) load unchanged: every field added since defaults sensibly.
const schema = 2

// Errors returned to the UI.
var (
	ErrUnknown         = errors.New("instance: no such instance")
	ErrBadName         = errors.New("instance: names need 1 to 64 characters")
	ErrBadLoader       = errors.New("instance: unknown mod loader")
	ErrInvalidSettings = errors.New("instance: invalid settings")
	ErrBadGroup        = errors.New("instance: group names need at most 32 characters")
)

var (
	idRe      = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)
	hexColor  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	versionRe = regexp.MustCompile(`^[A-Za-z0-9._ +-]{1,64}$`)
	iconRe    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
)

// ValidID reports whether id can name an instance.
func ValidID(id string) bool { return idRe.MatchString(id) }

const (
	maxName    = 64
	maxJVMArgs = 4096
	maxCommand = 4096
	maxEnv     = 64
	maxEnvVal  = 4096
	maxMemory  = 1 << 20
	maxNotes   = 4000
	maxTags    = 12
	maxTag     = 24
	maxGroup   = 32
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
	if in.Version < schema {
		in.Version, in.KeepLoaderUpdated = schema, true
	}
	if _, err := loader.ParseKind(in.Loader); err != nil || !loader.ValidVersionID(in.LoaderVersion) {
		in.Loader, in.LoaderVersion = "", ""
	}
	if !iconRe.MatchString(in.Icon) {
		in.Icon = ""
	}
	in.Notes = clip(in.Notes, maxNotes)
	in.Tags = cleanTags(in.Tags)
	in.Group, _ = cleanGroup(in.Group)
	if in.Location != "" && !filepath.IsAbs(in.Location) {
		in.Location = ""
	}
	c := &in.Settings.Commands
	for _, p := range []*string{&c.PreLaunch, &c.Wrapper, &c.PostExit} {
		if len(*p) > maxCommand || strings.ContainsAny(*p, "\x00\n\r") {
			*p = ""
		}
	}
	c.Env = slices.DeleteFunc(slices.Clone(c.Env), func(e EnvVar) bool { return validEnv(e) != nil })
	if len(c.Env) > maxEnv {
		c.Env = c.Env[:maxEnv]
	}
	return in
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// cleanTags trims, drops empty or overlong tags and duplicates (ignoring
// case), and keeps at most maxTags.
func cleanTags(tags []string) []string {
	var out []string
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || len([]rune(t)) > maxTag || strings.ContainsFunc(t, unicode.IsControl) ||
			slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, t) }) {
			continue
		}
		if out = append(out, t); len(out) == maxTags {
			break
		}
	}
	return out
}

// cleanGroup validates a group name ("" is no group).
func cleanGroup(g string) (string, error) {
	g = strings.TrimSpace(g)
	if len([]rune(g)) > maxGroup || strings.ContainsFunc(g, unicode.IsControl) {
		return "", ErrBadGroup
	}
	return g, nil
}

// envBlocked are variables that could inject JVM flags; they are stripped
// from the game's environment (java.CleanEnv) and may not be set back here.
var envBlocked = []string{"JAVA_TOOL_OPTIONS", "_JAVA_OPTIONS", "JDK_JAVA_OPTIONS", "CLASSPATH", "JAVA_HOME", "JRE_HOME"}

func validEnv(e EnvVar) error {
	if !envNameRe.MatchString(e.Name) || len(e.Value) > maxEnvVal || strings.ContainsAny(e.Value, "\x00") ||
		slices.Contains(envBlocked, strings.ToUpper(e.Name)) {
		return fmt.Errorf("%w: environment variable %q", ErrInvalidSettings, e.Name)
	}
	return nil
}

// Store holds every instance.
type Store struct {
	dir    string
	trash  string // trash/, next to the instances folder
	log    *slog.Logger
	groups *jsonfile.Store[groupsDoc]

	mu    sync.Mutex
	files map[string]*jsonfile.Store[Instance]
}

// Open loads every instance folder in dir. Folders whose name is not a valid
// id, or whose instance.json names another id, are skipped.
func Open(dir string, log *slog.Logger) (*Store, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	s := &Store{dir: dir, trash: filepath.Join(filepath.Dir(dir), "trash"), log: log, files: map[string]*jsonfile.Store[Instance]{}}
	var err error
	if s.groups, err = openGroups(dir, s); err != nil {
		return nil, err
	}
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
		Path: filepath.Join(s.dir, id, "instance.json"), Default: func() Instance { return Instance{Version: schema, KeepLoaderUpdated: true} },
		Normalize: normalize, Log: s.log,
	})
}

// Dir is the instance's folder.
func (s *Store) Dir(id string) string { return filepath.Join(s.dir, id) }

// GameDir is the folder the game runs in (.minecraft): instances/<id>/game,
// or wherever the player moved it.
func (s *Store) GameDir(id string) string {
	if in, err := s.Get(id); err == nil && in.Location != "" {
		return in.Location
	}
	return s.defaultGameDir(id)
}

func (s *Store) defaultGameDir(id string) string { return filepath.Join(s.dir, id, "game") }

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
	Name          string   `json:"name"`
	Art           Art      `json:"art"`
	Icon          string   `json:"icon"`
	Minecraft     string   `json:"minecraft"`
	Loader        string   `json:"loader"` // "" or "none" for vanilla
	LoaderVersion string   `json:"loaderVersion"`
	Group         string   `json:"group"`
	Sandbox       bool     `json:"sandbox"`
	Shares        []string `json:"shares"`
}

// Create makes a new instance folder. The Minecraft version and loader build
// must already be checked upstream by the caller; defaults seed the Java card.
func (s *Store) Create(req CreateRequest, defaults JavaSettings) (Instance, error) {
	name, err := cleanName(req.Name)
	if err != nil {
		return Instance{}, err
	}
	if req.Loader == "none" {
		req.Loader, req.LoaderVersion = "", ""
	}
	if err := checkLoader(req.Loader, req.LoaderVersion); err != nil {
		return Instance{}, err
	}
	if !versionRe.MatchString(req.Minecraft) {
		return Instance{}, fmt.Errorf("%w: version", ErrInvalidSettings)
	}
	group, err := cleanGroup(req.Group)
	if err != nil {
		return Instance{}, err
	}
	if req.Icon != "" && !iconRe.MatchString(req.Icon) {
		return Instance{}, fmt.Errorf("%w: icon", ErrInvalidSettings)
	}
	shares := make([]string, 0, len(req.Shares))
	for _, sh := range req.Shares {
		if len(sh) <= 64 && !strings.ContainsFunc(sh, unicode.IsControl) {
			shares = append(shares, sh)
		}
	}
	defaults.Override = false
	in := Instance{
		Version: schema, ID: s.newID(name), Name: name, Art: req.Art, Icon: req.Icon, Minecraft: req.Minecraft,
		Loader: req.Loader, LoaderVersion: req.LoaderVersion, KeepLoaderUpdated: true,
		Group: group, Sandbox: req.Sandbox, Shares: shares, Created: time.Now().UTC(),
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

// checkLoader validates a loader name and build id ("" for vanilla).
func checkLoader(kind, ver string) error {
	if kind == "" {
		if ver != "" {
			return ErrBadLoader
		}
		return nil
	}
	if _, err := loader.ParseKind(kind); err != nil || !loader.ValidVersionID(ver) {
		return ErrBadLoader
	}
	return nil
}

// SetSettings replaces an instance's settings after validation.
func (s *Store) SetSettings(id string, st Settings) (Instance, error) {
	if st.Java.MemoryMinMB < 0 || st.Java.MemoryMaxMB < 0 || len(st.Java.JVMArgs) > maxJVMArgs ||
		(st.Java.JavaPath != "" && !filepath.IsAbs(st.Java.JavaPath)) {
		return Instance{}, ErrInvalidSettings
	}
	c := st.Commands
	for _, cmd := range []string{c.PreLaunch, c.Wrapper, c.PostExit} {
		if len(cmd) > maxCommand || strings.ContainsAny(cmd, "\x00\n\r") {
			return Instance{}, fmt.Errorf("%w: command", ErrInvalidSettings)
		}
	}
	if len(c.Env) > maxEnv {
		return Instance{}, fmt.Errorf("%w: too many environment variables", ErrInvalidSettings)
	}
	for _, e := range c.Env {
		if err := validEnv(e); err != nil {
			return Instance{}, err
		}
	}
	return s.Update(id, func(in *Instance) error { in.Settings = st; return nil })
}

// SetVersion moves an instance to another Minecraft version and loader build
// (checked upstream by the caller).
func (s *Store) SetVersion(id, minecraft, kind, ver string) (Instance, error) {
	if !versionRe.MatchString(minecraft) {
		return Instance{}, fmt.Errorf("%w: version", ErrInvalidSettings)
	}
	if err := checkLoader(kind, ver); err != nil {
		return Instance{}, err
	}
	return s.Update(id, func(in *Instance) error {
		in.Minecraft, in.Loader, in.LoaderVersion = minecraft, kind, ver
		return nil
	})
}

// Flush writes pending changes of every instance.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	errs := []error{s.groups.Flush()}
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
	if _, err := s.Get(id); err != nil {
		return "", err
	}
	return s.GameDir(id), nil
}
