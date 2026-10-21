// Package instance stores instances: one folder each, holding instance.json,
// the game folder and launch history. An instance's id is its identity and
// its folder name, so renaming never moves files (unlike Pandora, where the
// folder name was the identity).
package instance

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"quartz/internal/core/jsonfile"
	"quartz/internal/loader"
	"quartz/internal/sandbox/rules"
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
	Width           int   `json:"width"`
	Height          int   `json:"height"`
	Fullscreen      bool  `json:"fullscreen"`
	HideOnGameStart *bool `json:"hideOnGameStart,omitempty"` // nil follows the global setting
	// OpenInto skips the title screen when nothing else is asked for.
	OpenInto Into `json:"openInto"`
}

// CommandSettings run the player's own programs around the game. Each command
// is split into words like a shell would, but no shell ever runs it; $VAR
// references (the $INST_* values and the environment) are expanded per word.
type CommandSettings struct {
	PreLaunch string   `json:"preLaunch"` // must succeed before the game starts
	Wrapper   string   `json:"wrapper"`   // put in front of java, e.g. "prime-run"
	PostExit  string   `json:"postExit"`  // runs after the game closes, with $EXIT_CODE
	Env       []EnvVar `json:"env"`
	// Sig authenticates the commands above as set in this launcher (an HMAC
	// over them with the per-install key). Commands run programs, so a foreign
	// or hand-edited instance.json carrying them is ignored unless its Sig
	// verifies; see Store.verified and Store.signCommands.
	Sig          string `json:"sig,omitempty"`
	SystemGLFW   bool   `json:"systemGlfw"`
	SystemOpenAL bool   `json:"systemOpenal"`
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
	// Account is the account this instance always plays as ("" for the
	// default one), as Prism and Pandora let you pick; Play as still wins.
	Account string `json:"account,omitempty"`
}

// accountRe is an account id.
var accountRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Instance is one instance's record.
type Instance struct {
	Version       int    `json:"version"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Art           Art    `json:"art"`
	Icon          string `json:"icon,omitempty"`   // sha256 of a picture in the icon library; "" draws Art
	Banner        string `json:"banner,omitempty"` // sha256 of a picture in the banner library; "" colours the card cover
	Minecraft     string `json:"minecraft"`
	Loader        string `json:"loader,omitempty"` // "" is vanilla
	LoaderVersion string `json:"loaderVersion,omitempty"`
	// KeepLoaderUpdated installs the newest stable loader build at launch.
	KeepLoaderUpdated bool `json:"keepLoaderUpdated"`
	// FollowLatest moves to the newest Minecraft release once every mod
	// supports it.
	FollowLatest bool     `json:"followLatest"`
	Notes        string   `json:"notes,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Location     string   `json:"location,omitempty"` // a moved game folder; "" is instances/<id>/game
	// Sandbox is what the game may reach.
	Sandbox    rules.Settings `json:"sandbox"`
	Sync       Sync           `json:"sync"`
	Created    time.Time      `json:"created"`
	LastPlayed *time.Time     `json:"lastPlayed,omitempty"`
	// LastOpened is when its page was last opened (to the minute), so Browse
	// can add to the instance looked at last.
	LastOpened  *time.Time `json:"lastOpened,omitempty"`
	PlaytimeMin int        `json:"playtimeMin"`
	// PlaytimeSec carries the seconds below a whole minute, so short
	// sessions add up instead of rounding away (Prism counts seconds).
	PlaytimeSec int      `json:"playtimeSec,omitempty"`
	Settings    Settings `json:"settings"`
	// Pack is the modpack the instance was installed from (nil for none).
	Pack *PackRef `json:"pack,omitempty"`
	// Host is how the instance was last hosted as a local server (nil: never).
	Host *Host `json:"host,omitempty"`
	// InPlace marks a game folder that belongs to another launcher (imported
	// "in place"): it is never adopted into the store or given read-only
	// links, and purging never deletes it (it carries no marker).
	InPlace bool `json:"inPlace,omitempty"`
	// ImportedFrom is the other launcher's instance this one was brought
	// over from (nil: made here).
	ImportedFrom *ImportedFrom `json:"importedFrom,omitempty"`
	// HideDiscord keeps the instance out of Discord's "Playing" status (the
	// Overview switch; the global one is in Integrations).
	HideDiscord bool `json:"hideDiscord,omitempty"`
	// Shortcuts are its desktop and app-menu shortcuts.
	Shortcuts []Shortcut `json:"shortcuts,omitempty"`
	// Export is what the last pack export was called, offered again next time.
	Export *ExportMeta `json:"export,omitempty"`
	// ClientJar is a custom client jar that runs in place of vanilla's (nil:
	// vanilla). Such an instance never has a mod loader.
	ClientJar *ClientJar `json:"clientJar,omitempty"`
}

// ExportMeta names an exported pack (Prism's export fields).
type ExportMeta struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Summary string `json:"summary,omitempty"`
	Author  string `json:"author,omitempty"`
}

// Clean trims the fields, drops control characters and caps their lengths.
func (m ExportMeta) Clean() ExportMeta {
	clip := func(s string, n int) string {
		s = strings.TrimSpace(strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return -1
			}
			return r
		}, s))
		if r := []rune(s); len(r) > n {
			s = strings.TrimSpace(string(r[:n]))
		}
		return s
	}
	return ExportMeta{Name: clip(m.Name, 64), Version: clip(m.Version, 32), Summary: clip(m.Summary, 500), Author: clip(m.Author, 64)}
}

// Host is how to run an instance as a local server.
type Host struct {
	World        string `json:"world"`          // a saves folder; "" makes a new world
	Seed         string `json:"seed,omitempty"` // a new world's seed ("" random)
	Port         int    `json:"port"`
	MaxPlayers   int    `json:"maxPlayers"`
	MemoryMB     int    `json:"memoryMB"`
	ViewDistance int    `json:"viewDistance"`         // render distance in chunks
	MOTD         string `json:"motd"`                 // "" is the instance's name
	Difficulty   string `json:"difficulty,omitempty"` // peaceful | easy | normal | hard; "" keeps the world's
	Icon         string `json:"icon,omitempty"`       // a picture in the icon library; "" is Quartz's
	LAN          bool   `json:"lan"`                  // reachable from the network and announced there
	Offline      bool   `json:"offline"`              // players without a Microsoft account may join
	Sandbox      bool   `json:"sandbox"`              // run in the sandbox even when the instance isn't
	// Mods are the player's picks in Manage, by mod key (see
	// content.ServerMod): true runs a mod the launcher would leave out,
	// false leaves out one it would run. Start keeps them as saved.
	Mods map[string]bool `json:"mods,omitempty"`
}

// Sync is which shared targets the instance joins (see package syncing).
type Sync struct {
	Join []string `json:"join"` // target ids: "options", "saves", "c-…"
	Keys bool     `json:"keys"` // write the shared keybinds on launch
}

var syncIDRe = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// ValidSyncID reports whether s can name a sync target.
func ValidSyncID(s string) bool { return syncIDRe.MatchString(s) }

func (y Sync) normalize() Sync {
	join := y.Join
	y.Join = make([]string, 0, len(join)) // never nil: the UI reads sync.join as a list
	for _, id := range join {
		if ValidSyncID(id) && !slices.Contains(y.Join, id) && len(y.Join) < 128 {
			y.Join = append(y.Join, id)
		}
	}
	return y
}

// PackRef names an instance's modpack and its installed version.
type PackRef struct {
	Source      string `json:"source"` // modrinth | curseforge | ftb | technic | file
	Project     string `json:"project,omitempty"`
	Version     string `json:"version,omitempty"` // the platform's version id
	Name        string `json:"name"`
	VersionName string `json:"versionName,omitempty"` // "1.4.0"
}

// schema is the instance.json version this build writes.
const schema = 1

// Errors returned to the UI.
var (
	ErrUnknown         = errors.New("instance: no such instance")
	ErrBadName         = errors.New("instance: names need 1 to 64 characters")
	ErrBadLoader       = errors.New("instance: unknown mod loader")
	ErrInvalidSettings = errors.New("instance: invalid settings")
	ErrGameDirMissing  = errors.New("instance: the game folder isn't there. Is its drive connected?")
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
)

// cleanName trims and rejects control characters.
func cleanName(n string) (string, error) {
	n = strings.TrimSpace(n)
	if n == "" || len([]rune(n)) > maxName || strings.ContainsFunc(n, unicode.IsControl) {
		return "", ErrBadName
	}
	return n, nil
}

// freeName is name, or "name (2)", "name (3)" … when another instance is
// already called that (ignoring case), so menus that list instances by name
// never show two alike (XMCL does the same).
func (s *Store) freeName(name string) string {
	taken := map[string]bool{}
	for _, in := range s.List() {
		taken[strings.ToLower(in.Name)] = true
	}
	try := name
	for n := 2; taken[strings.ToLower(try)]; n++ {
		suffix := fmt.Sprintf(" (%d)", n)
		base := []rune(name)
		base = base[:min(len(base), maxName-len(suffix))]
		try = strings.TrimSpace(string(base)) + suffix
	}
	return try
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
	if in.Settings.Account != "" && !accountRe.MatchString(in.Settings.Account) {
		in.Settings.Account = ""
	}
	in.PlaytimeMin = max(in.PlaytimeMin, 0)
	in.PlaytimeSec = min(max(in.PlaytimeSec, 0), 59)
	in.Version = schema
	if _, err := loader.ParseKind(in.Loader); err != nil || !loader.ValidVersionID(in.LoaderVersion) {
		in.Loader, in.LoaderVersion = "", ""
	}
	if !iconRe.MatchString(in.Icon) {
		in.Icon = ""
	}
	if !iconRe.MatchString(in.Banner) {
		in.Banner = ""
	}
	if in.ClientJar = in.ClientJar.normalize(); in.ClientJar != nil {
		in.Loader, in.LoaderVersion, in.FollowLatest = "", "", false
	}
	in.Sync = in.Sync.normalize()
	in.Sandbox = in.Sandbox.Normalize()
	in.Notes = clip(in.Notes, maxNotes)
	in.Tags = cleanTags(in.Tags)
	in.Shortcuts = cleanShortcuts(in.Shortcuts)
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
	dir   string
	trash string // trash/, next to the instances folder
	log   *slog.Logger

	mu    sync.Mutex
	files map[string]*jsonfile.Store[Instance]

	keyOnce sync.Once
	key     []byte // per-install HMAC key authenticating command settings

	locate  atomic.Pointer[Locator]
	onPurge atomic.Pointer[func(id string)]
}

// OnPurge sets what runs after an instance is deleted for good (its
// sandbox's leftovers go too).
func (s *Store) OnPurge(fn func(id string)) { s.onPurge.Store(&fn) }

// Locator says under which folder an instance-relative path really lives:
// the game folder, or the shared folder when the instance shares it.
type Locator func(id, rel string) string

// SetLocator installs the shared layer's resolver (package syncing).
func (s *Store) SetLocator(l Locator) { s.locate.Store(&l) }

// Base is the folder an instance-relative path (such as "saves" or
// "resourcepacks/x.zip") lives under: the game folder, or the shared folder
// for a folder the instance shares. The relative path is the same in both.
func (s *Store) Base(id, rel string) string {
	if l := s.locate.Load(); l != nil {
		if b := (*l)(id, rel); b != "" {
			return b
		}
	}
	return s.GameDir(id)
}

// Path is where an instance-relative path really is: Base joined with rel.
func (s *Store) Path(id, rel string) string {
	return filepath.Join(s.Base(id, rel), filepath.FromSlash(rel))
}

// Open loads every instance folder in dir. Folders whose name is not a valid
// id, or whose instance.json names another id, are skipped.
func Open(dir string, log *slog.Logger) (*Store, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	s := &Store{dir: dir, trash: filepath.Join(filepath.Dir(dir), "trash"), log: log, files: map[string]*jsonfile.Store[Instance]{}}
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), stagePrefix) {
			_ = os.RemoveAll(filepath.Join(dir, e.Name()))
			continue
		}
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
	return jsonfile.Open(fileOptions(filepath.Join(s.dir, id, "instance.json"), s.log))
}

func fileOptions(path string, log *slog.Logger) jsonfile.Options[Instance] {
	return jsonfile.Options[Instance]{
		Path: path, Default: func() Instance { return Instance{Version: schema, KeepLoaderUpdated: true} },
		Normalize: normalize, Log: log,
	}
}

// ReadAll reads every instance in dir without opening a Store or writing
// anything, for readers outside the running launcher (the command line).
// Instances are ordered as List orders them; unreadable folders are skipped.
func ReadAll(dir string) ([]Instance, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Instance
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), stagePrefix) {
			_ = os.RemoveAll(filepath.Join(dir, e.Name()))
			continue
		}
		if !e.IsDir() || !ValidID(e.Name()) {
			continue
		}
		path := filepath.Join(dir, e.Name(), "instance.json")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		in, err := jsonfile.Read(fileOptions(path, nil))
		if err != nil || in.ID != e.Name() {
			continue
		}
		out = append(out, in)
	}
	sort.Slice(out, func(i, j int) bool { return lastActive(out[j]).Before(lastActive(out[i])) })
	return out, nil
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

// CheckGameDir makes sure a game folder outside the data folder is really
// there before anything writes to it: a folder moved by the launcher must
// still carry its marker, another launcher's folder must exist. Otherwise an
// unplugged drive would get a fresh, empty game at its mount point.
func (s *Store) CheckGameDir(id string) error {
	in, err := s.Get(id)
	if err != nil || in.Location == "" {
		return err
	}
	ok := markedFor(in.Location, id)
	if in.InPlace {
		info, err := os.Stat(in.Location)
		ok = err == nil && info.IsDir()
	}
	if !ok {
		return fmt.Errorf("%w (%s)", ErrGameDirMissing, in.Location)
	}
	return nil
}

// RunsDir holds the instance's launch history.
func (s *Store) RunsDir(id string) string { return filepath.Join(s.dir, id, "runs") }

// List returns all instances, most recently played first, never-played last
// by creation date.
func (s *Store) List() []Instance {
	s.mu.Lock()
	out := make([]Instance, 0, len(s.files))
	for _, f := range s.files {
		out = append(out, s.verified(f.Get()))
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return lastActive(out[j]).Before(lastActive(out[i])) })
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
	return s.verified(f.Get()), nil
}

// commandKey is the per-install key that authenticates command settings, kept
// next to the trash folder (outside any instance, so a game's sandbox can't
// reach it). It is created once; if it can't be read or written the gate
// degrades to the prior behaviour (commands are trusted) rather than breaking
// the launcher.
func (s *Store) commandKey() []byte {
	s.keyOnce.Do(func() {
		path := filepath.Join(filepath.Dir(s.dir), "commands.key")
		if b, err := os.ReadFile(path); err == nil && len(b) == sha256.Size {
			s.key = b
			return
		}
		k := make([]byte, sha256.Size)
		if _, err := rand.Read(k); err != nil {
			s.log.Warn("can't make the command-settings key; commands aren't origin-checked", "err", err)
			return
		}
		if err := os.WriteFile(path, k, 0o600); err != nil {
			s.log.Warn("can't store the command-settings key; commands aren't origin-checked", "err", err)
			return
		}
		s.key = k
	})
	return s.key
}

// signCommands is the HMAC authenticating an instance's commands. The id is
// bound in so a signed instance.json can't be reused under another id. Returns
// "" when no key is available (the gate is then inactive).
func (s *Store) signCommands(id string, c CommandSettings) string {
	key := s.commandKey()
	if key == nil {
		return ""
	}
	var b bytes.Buffer
	for _, p := range []string{id, c.PreLaunch, c.Wrapper, c.PostExit} {
		b.WriteString(p)
		b.WriteByte(0) // commands reject NUL, so it's an unambiguous separator
	}
	for _, e := range c.Env {
		b.WriteString(e.Name)
		b.WriteByte(0)
		b.WriteString(e.Value)
		b.WriteByte(0)
	}
	m := hmac.New(sha256.New, key)
	m.Write(b.Bytes())
	return hex.EncodeToString(m.Sum(nil))
}

// verified blanks an instance's commands when their signature doesn't match
// this install's key, so a command added by a foreign or hand-edited
// instance.json never runs. Commands set through the launcher (Store.SetSettings)
// carry a valid signature; everything else is dropped. Pure reads (List, Get)
// return the sanitised copy; the file on disk is left as is.
func (s *Store) verified(in Instance) Instance {
	c := &in.Settings.Commands
	if c.PreLaunch == "" && c.Wrapper == "" && c.PostExit == "" && len(c.Env) == 0 {
		return in
	}
	want := s.signCommands(in.ID, *c)
	if want == "" || (c.Sig != "" && hmac.Equal([]byte(c.Sig), []byte(want))) {
		return in // verified, or no key so the gate is inactive
	}
	s.log.Warn("ignoring an instance's commands: not set in this launcher", "instance", in.ID)
	c.PreLaunch, c.Wrapper, c.PostExit, c.Env, c.Sig = "", "", "", nil, ""
	return in
}

// CreateRequest is what the New instance screen sends.
type CreateRequest struct {
	Name          string `json:"name"`
	Art           Art    `json:"art"`
	Icon          string `json:"icon"`
	Banner        string `json:"banner"`
	Minecraft     string `json:"minecraft"`
	Loader        string `json:"loader"` // "" or "none" for vanilla
	LoaderVersion string `json:"loaderVersion"`
	Sandbox       bool   `json:"sandbox"`
	// MemoryMB gives the instance its own memory (0 follows the global
	// setting); the caller sets it once the instance exists.
	MemoryMB int `json:"memoryMB"`
	// Shares are the sync targets to join ("keys" for the shared keybinds);
	// the caller joins them once the instance exists (syncing.JoinAll).
	Shares []string `json:"shares"`
	// ClientJar is the sha1 of a picked client jar ("" for vanilla); the
	// caller places it once the instance exists (SetClientJar).
	ClientJar string `json:"clientJar"`
}

// Create makes a new instance folder. The Minecraft version and loader build
// must already be checked upstream by the caller. Its Java card starts empty
// and inherits the global settings live until the player overrides them.
func (s *Store) Create(req CreateRequest) (Instance, error) {
	name, err := cleanName(req.Name)
	if err != nil {
		return Instance{}, err
	}
	name = s.freeName(name)
	if req.Loader == "none" {
		req.Loader, req.LoaderVersion = "", ""
	}
	if err := checkLoader(req.Loader, req.LoaderVersion); err != nil {
		return Instance{}, err
	}
	if req.ClientJar != "" && req.Loader != "" {
		return Instance{}, ErrClientJarLocked
	}
	if !versionRe.MatchString(req.Minecraft) {
		return Instance{}, fmt.Errorf("%w: version", ErrInvalidSettings)
	}
	if !validLook(req.Art, req.Icon, req.Banner) {
		return Instance{}, fmt.Errorf("%w: icon", ErrInvalidSettings)
	}
	in := Instance{
		Version: schema, ID: s.newID(name), Name: name, Art: req.Art, Icon: req.Icon, Banner: req.Banner, Minecraft: req.Minecraft,
		Loader: req.Loader, LoaderVersion: req.LoaderVersion, KeepLoaderUpdated: true,
		Sandbox: rules.Settings{On: req.Sandbox, Preset: rules.Standard}, Created: time.Now().UTC(),
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
	slug := idSlug(name)
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
		(st.Java.JavaPath != "" && !filepath.IsAbs(st.Java.JavaPath)) || !st.Window.OpenInto.Valid() ||
		(st.Account != "" && !accountRe.MatchString(st.Account)) {
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
	// Authenticate the commands as set in this launcher, so a later foreign or
	// hand-edited instance.json carrying different ones is ignored (verified).
	st.Commands.Sig = s.signCommands(id, st.Commands)
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
		if in.ClientJar != nil && (minecraft != in.Minecraft || kind != "") {
			return ErrClientJarLocked
		}
		in.Minecraft, in.Loader, in.LoaderVersion = minecraft, kind, ver
		return nil
	})
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
	if _, err := s.Get(id); err != nil {
		return "", err
	}
	return s.GameDir(id), nil
}
