// Package rules holds what a sandboxed instance may reach: one level per
// rule row of the Sandbox page, the Standard and Strict presets, and the
// player's own allowed folders. It is pure data, so instance.json can carry
// it and every OS backend reads the same answers.
package rules

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"slices"
)

// Level is how far the game reaches one thing. Folders take rw, ro or no;
// switches (internet, Discord, graphics) take on or no.
type Level string

// Levels, named as the UI shows them.
const (
	RW      Level = "rw" // Read and write
	RO      Level = "ro" // Read only: writes stay private to the run and are dropped
	Blocked Level = "no" // Blocked
	Allowed Level = "on" // Allowed
	Remove  Level = "remove"
)

// ID names a rule row.
type ID string

// The rows of the Sandbox page, in its order.
const (
	Instance  ID = "instance"
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
	Custom   Preset = "custom"
)

// Rule is one row: its levels under each preset and the levels an OS can
// enforce for it (the first is the fallback).
type Rule struct {
	ID       ID
	Standard Level
	Strict   Level
	choices  []Level
	// fixed lists the OSes that only offer the first choice ("windows").
	fixed []string
}

// Table is every row, in page order.
var Table = []Rule{
	{ID: Instance, Standard: RW, Strict: RW, choices: []Level{RW}},
	{ID: Libraries, Standard: RO, Strict: RO, choices: []Level{RO}},
	{ID: Java, Standard: RO, Strict: RO, choices: []Level{RO}},
	{ID: Synced, Standard: RW, Strict: RO, choices: []Level{RW, RO, Blocked}},
	{ID: Internet, Standard: Allowed, Strict: Blocked, choices: []Level{Allowed, Blocked}},
	// An app container can't open Discord's named pipe.
	{ID: Discord, Standard: Allowed, Strict: Blocked, choices: []Level{Allowed, Blocked}, fixed: []string{"windows"}},
	{ID: Graphics, Standard: Allowed, Strict: Allowed, choices: []Level{Allowed}},
	// Granting a container the whole user profile would rewrite its ACLs.
	{ID: Home, Standard: Blocked, Strict: Blocked, choices: []Level{Blocked, RO}, fixed: []string{"windows"}},
}

// Choices are the levels goos can enforce for the rule.
func (r Rule) Choices(goos string) []Level {
	if slices.Contains(r.fixed, goos) {
		return []Level{r.preset(Strict)}
	}
	return r.choices
}

func (r Rule) preset(p Preset) Level {
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
	On      bool         `json:"on"`
	Preset  Preset       `json:"preset"`
	Levels  map[ID]Level `json:"levels,omitempty"` // custom choices; missing rows follow the preset
	Folders []Folder     `json:"folders,omitempty"`
}

// Default is a new instance's sandbox: on, Standard.
func Default() Settings { return Settings{On: true, Preset: Standard} }

// UnmarshalJSON also reads the bool instance.json held before the sandbox
// had rules (true: on, Standard).
func (s *Settings) UnmarshalJSON(b []byte) error {
	var on bool
	if json.Unmarshal(b, &on) == nil {
		*s = Settings{On: on, Preset: Standard}
		return nil
	}
	type plain Settings
	return json.Unmarshal(b, (*plain)(s))
}

// Level is the rule's level on goos: the custom choice if it is one goos can
// enforce, else the preset's.
func (s Settings) Level(id ID, goos string) Level {
	r, ok := Lookup(id)
	if !ok {
		return Blocked
	}
	choices := r.Choices(goos)
	if s.Preset == Custom {
		if l, ok := s.Levels[id]; ok && slices.Contains(choices, l) {
			return l
		}
	}
	if l := r.preset(s.Preset); slices.Contains(choices, l) {
		return l
	}
	return choices[0]
}

// Apply switches to a preset. Standard and Strict reset every row; Custom
// starts from the levels showing now.
func (s Settings) Apply(p Preset, goos string) Settings {
	if p == Custom {
		levels := map[ID]Level{}
		for _, r := range Table {
			levels[r.ID] = s.Level(r.ID, goos)
		}
		s.Levels = levels
	} else {
		s.Levels = nil
	}
	s.Preset = p
	return s
}

// SetLevel changes one row, which makes the choices Custom. It reports false
// for a level goos can't enforce for that row.
func (s Settings) SetLevel(id ID, l Level, goos string) (Settings, bool) {
	r, ok := Lookup(id)
	if !ok || !slices.Contains(r.Choices(goos), l) {
		return s, false
	}
	if s.Level(id, goos) == l {
		return s, true
	}
	s = s.Apply(Custom, goos)
	s.Levels[id] = l
	return s, true
}

var folderIDRe = regexp.MustCompile(`^[a-z0-9]{1,16}$`)

// ValidFolderID reports whether s can name one of the player's folders.
func ValidFolderID(s string) bool { return folderIDRe.MatchString(s) }

// Normalize repairs a hand-edited file: unknown presets, rows, levels and
// malformed folders are dropped.
func (s Settings) Normalize() Settings {
	if !slices.Contains([]Preset{Standard, Strict, Custom}, s.Preset) {
		s.Preset = Standard
	}
	levels := map[ID]Level{}
	for id, l := range s.Levels {
		if r, ok := Lookup(id); ok && slices.Contains(r.choices, l) {
			levels[id] = l
		}
	}
	s.Levels = nil
	if s.Preset == Custom && len(levels) > 0 {
		s.Levels = levels
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
