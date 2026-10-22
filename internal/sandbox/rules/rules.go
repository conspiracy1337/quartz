// Package rules holds what a sandboxed instance may reach: one level per
// rule row of the Sandbox page, the Standard and Strict presets, and the
// player's own allowed folders. It is pure data, so instance.json can carry
// it and every OS backend reads the same answers.
package rules

import (
	"path/filepath"
	"regexp"
	"slices"
)

// Level is how far the game reaches one thing. Folders take rw, ro or no;
// Discord and graphics take on or no; internet takes on or lan.
type Level string

// Levels, named as the UI shows them.
const (
	RW      Level = "rw"     // Read and write
	RO      Level = "ro"     // Read only: writes stay private to the run and are dropped
	Blocked Level = "no"     // Blocked
	Allowed Level = "on"     // Allowed
	LAN     Level = "lan"    // The local network (and VPN) but not the internet
	Remove  Level = "remove" // takes one of the player's folders out
)

// ID names a rule row.
type ID string

// The rows of the Sandbox page, in its order.
const (
	Instance  ID = "instance"
	Mods      ID = "mods"
	Libraries ID = "libraries"
	Java      ID = "java"
	Synced    ID = "synced"
	Internet  ID = "internet"
	Discord   ID = "discord"
	Graphics  ID = "graphics"
	Home      ID = "home"
)

// Preset names a set of levels.
type Preset string

// Presets.
const (
	Standard Preset = "standard"
	Strict   Preset = "strict"
)

// Rule is one row: its level under each preset, and the level an OS holds it
// at whatever the preset (what that OS can enforce).
type Rule struct {
	ID       ID
	Standard Level
	Strict   Level
	fixed    map[string]Level // by GOOS
}

// Table is every row, in page order.
var Table = []Rule{
	{ID: Instance, Standard: RW, Strict: RW},
	// The instance's mods are store files linked into every instance that
	// uses them; a game that could write them would reach the others.
	{ID: Mods, Standard: RO, Strict: RO},
	{ID: Libraries, Standard: RO, Strict: RO},
	{ID: Java, Standard: RO, Strict: RO},
	{ID: Synced, Standard: RW, Strict: RO},
	// Internet: Allowed is the full network; LAN keeps the local network and
	// VPNs (so a Strict instance can still join or host a LAN/Radmin game) but
	// not the internet. LAN is enforced only where the OS can filter by
	// destination (Windows app container capabilities); Linux and macOS keep
	// the full network for it (see each backend).
	{ID: Internet, Standard: Allowed, Strict: LAN},
	// An app container can't open Discord's named pipe.
	{ID: Discord, Standard: Allowed, Strict: Blocked, fixed: map[string]Level{"windows": Blocked}},
	{ID: Graphics, Standard: Allowed, Strict: Allowed},
	{ID: Home, Standard: Blocked, Strict: Blocked},
}

// Level is the rule's level under preset p on goos.
func (r Rule) Level(p Preset, goos string) Level {
	if l, ok := r.fixed[goos]; ok {
		return l
	}
	if p == Strict {
		return r.Strict
	}
	return r.Standard
}

// Lookup finds a rule by id.
func Lookup(id ID) (Rule, bool) {
	i := slices.IndexFunc(Table, func(r Rule) bool { return r.ID == id })
	if i < 0 {
		return Rule{}, false
	}
	return Table[i], true
}

// FolderChoices are the levels of a player's own folder.
var FolderChoices = []Level{RW, RO}

// MaxFolders caps the player's own folders.
const MaxFolders = 32

// Folder is a folder the player allowed.
type Folder struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Level Level  `json:"level"`
}

// Settings are an instance's sandbox choices.
type Settings struct {
	On      bool     `json:"on"`
	Preset  Preset   `json:"preset"`
	Folders []Folder `json:"folders,omitempty"`
}

// Default is a new instance's sandbox: on, Standard.
func Default() Settings { return Settings{On: true, Preset: Standard} }

// Level is the rule's level on goos under the chosen preset.
func (s Settings) Level(id ID, goos string) Level {
	r, ok := Lookup(id)
	if !ok {
		return Blocked
	}
	return r.Level(s.Preset, goos)
}

// Apply switches to a preset.
func (s Settings) Apply(p Preset) Settings {
	s.Preset = p
	return s
}

var folderIDRe = regexp.MustCompile(`^[a-z0-9]{1,16}$`)

// ValidFolderID reports whether s can name one of the player's folders.
func ValidFolderID(s string) bool { return folderIDRe.MatchString(s) }

// Normalize repairs a hand-edited file: an unknown preset is Standard and
// malformed folders are dropped.
func (s Settings) Normalize() Settings {
	if s.Preset != Strict {
		s.Preset = Standard
	}
	var folders []Folder
	for _, f := range s.Folders {
		if ValidFolderID(f.ID) && filepath.IsAbs(f.Path) && len(f.Path) <= 4096 && slices.Contains(FolderChoices, f.Level) &&
			!slices.ContainsFunc(folders, func(o Folder) bool { return o.ID == f.ID || o.Path == f.Path }) && len(folders) < MaxFolders {
			folders = append(folders, f)
		}
	}
	s.Folders = folders
	return s
}
