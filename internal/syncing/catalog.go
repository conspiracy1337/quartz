// Package syncing is the shared layer: files and folders that instances share
// through one real copy in the launcher's shared/ folder, which mirrors an
// instance folder's layout. The Syncing page turns targets on; each instance
// joins the ones it wants. Folders are assembled into each run per OS (see
// package runview); files are copied in before launch and out after exit,
// newest wins, with options.txt merged key by key.
package syncing

import (
	"slices"

	"quartz/internal/settings"
)

// Group is a section of the Syncing page.
type Group string

const (
	Files   Group = "files"
	Folders Group = "folders"
	Mods    Group = "mods"
	Custom  Group = "custom"
)

// Target is one thing that can be shared.
type Target struct {
	ID    string
	Name  string
	Path  string // instance-relative, slash-separated, no trailing slash
	Group Group
	Dir   bool
	// ModIDs are the mods that write this folder (Mods only); the row is
	// greyed out while no instance has one of them.
	ModIDs []string
}

// OptionsID is the target merged key by key instead of copied whole.
const OptionsID = "options"

// KeysID is what New instance sends for "Keybinds": the shared keybinds,
// which are a per-instance switch rather than a target.
const KeysID = "keys"

// builtins are the targets every launcher has, in the page's order.
var builtins = []Target{
	{ID: OptionsID, Name: "Game options", Path: "options.txt", Group: Files},
	{ID: "servers", Name: "Server list", Path: "servers.dat", Group: Files},
	{ID: "history", Name: "Command history", Path: "command_history.txt", Group: Files},
	{ID: "hotbar", Name: "Hotbars", Path: "hotbar.nbt", Group: Files},

	{ID: "saves", Name: "Worlds", Path: "saves", Group: Folders, Dir: true},
	{ID: "config", Name: "Configs", Path: "config", Group: Folders, Dir: true},
	{ID: "screenshots", Name: "Screenshots", Path: "screenshots", Group: Folders, Dir: true},
	{ID: "resourcepacks", Name: "Resource packs", Path: "resourcepacks", Group: Folders, Dir: true},
	{ID: "shaderpacks", Name: "Shader packs", Path: "shaderpacks", Group: Folders, Dir: true},
	{ID: "datapacks", Name: "Data packs", Path: "datapacks", Group: Folders, Dir: true},

	{ID: "xaero", Name: "Xaero's Minimap", Path: "xaero", Group: Mods, Dir: true, ModIDs: []string{"xaerominimap", "xaeroworldmap", "xaerominimapfair"}},
	{ID: "dh", Name: "Distant Horizons", Path: "Distant_Horizons_server_data", Group: Mods, Dir: true, ModIDs: []string{"distanthorizons"}},
	{ID: "voxy", Name: "Voxy", Path: ".voxy", Group: Mods, Dir: true, ModIDs: []string{"voxy"}},
	{ID: "litematica", Name: "Litematica", Path: "schematics", Group: Mods, Dir: true, ModIDs: []string{"litematica"}},
	{ID: "flashback", Name: "Flashback", Path: "flashback", Group: Mods, Dir: true, ModIDs: []string{"flashback"}},
	{ID: "bobby", Name: "Bobby", Path: ".bobby", Group: Mods, Dir: true, ModIDs: []string{"bobby"}},
}

// catalog is every target: the built-in ones, then the custom paths.
func catalog(y settings.Syncing) []Target {
	out := slices.Clone(builtins)
	for _, c := range y.Custom {
		out = append(out, Target{ID: c.ID, Name: c.Path, Path: c.Path, Group: Custom, Dir: c.Dir})
	}
	return out
}

func find(y settings.Syncing, id string) (Target, bool) {
	for _, t := range catalog(y) {
		if t.ID == id {
			return t, true
		}
	}
	return Target{}, false
}

// covers reports whether rel is t's path or lies inside its folder.
func (t Target) covers(rel string) bool {
	if rel == t.Path {
		return true
	}
	return t.Dir && len(rel) > len(t.Path) && rel[len(t.Path)] == '/' && rel[:len(t.Path)] == t.Path
}
