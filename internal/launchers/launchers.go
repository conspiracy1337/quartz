// Package launchers finds other Minecraft launchers on this computer and
// reads their instances (Prism Launcher, MultiMC, the Modrinth App, the
// CurseForge app, ATLauncher), so first-run setup can bring them over. It
// only reads: every file is opened read-only, and an instance becomes a
// modpack.Pack whose game folder is copied, or used where it is.
//
// Their files are untrusted input: JSON, INI and SQLite are size-capped and
// parsed strictly, and the only things taken are the name, the game and
// loader versions, the icon, the playtime and the memory setting (never
// commands, JVM arguments or wrappers, which would run another app's code).
package launchers

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"quartz/internal/core/fsx"
	"quartz/internal/modpack"
)

// Env is where to look.
type Env struct {
	OS   string
	Home string
	// Data is the per-user application data folder: ~/.local/share,
	// ~/Library/Application Support or %APPDATA%.
	Data string
}

// SystemEnv describes this computer.
func SystemEnv() Env {
	home, _ := os.UserHomeDir()
	e := Env{OS: runtime.GOOS, Home: home}
	switch runtime.GOOS {
	case "windows":
		e.Data = os.Getenv("APPDATA")
	case "darwin":
		e.Data = filepath.Join(home, "Library", "Application Support")
	default:
		e.Data = os.Getenv("XDG_DATA_HOME")
		if !filepath.IsAbs(e.Data) {
			e.Data = filepath.Join(home, ".local", "share")
		}
	}
	return e
}

// Source is one instance of another launcher.
type Source struct {
	Name          string
	Minecraft     string
	Loader        string // "fabric", "quilt", "forge", "neoforge" or "" for vanilla
	LoaderVersion string
	GameDir       string // the folder the game runs in; also its identity
	Icon          string // a picture file ("" for none)
	PlaytimeMin   int
	LastPlayed    *time.Time
	MemoryMB      int
	Skip          []string // top-level names in GameDir that belong to the launcher
	Disabled      string   // a folder of disabled mods (ATLauncher)
	Warnings      []string
	// Pack is the modpack the launcher says the instance came from.
	Managed *modpack.Managed
}

// Found is a launcher with instances.
type Found struct {
	ID        string // "prism", "multimc", "modrinth", "curseforge", "atlauncher"
	Name      string // "Prism Launcher"
	Root      string
	Instances []Source
}

type launcher struct {
	id, name string
	roots    func(Env) []string
	read     func(root string) ([]Source, error)
}

var all = []launcher{
	{"prism", "Prism Launcher", prismRoots, readPrism},
	{"multimc", "MultiMC", multiMCRoots, readMultiMC},
	{"modrinth", "Modrinth App", modrinthRoots, readModrinth},
	{"curseforge", "CurseForge", curseForgeRoots, readCurseForge},
	{"atlauncher", "ATLauncher", atLauncherRoots, readATLauncher},
}

// Detect lists the launchers that have instances, in a fixed order. A
// launcher whose files can't be read is left out.
func Detect(env Env) []Found {
	var out []Found
	for _, l := range all {
		for _, root := range l.roots(env) {
			if root == "" || !isDir(root) {
				continue
			}
			list, err := l.read(root)
			if err != nil || len(list) == 0 {
				continue
			}
			slices.SortFunc(list, func(a, b Source) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
			out = append(out, Found{ID: l.id, Name: l.name, Root: root, Instances: list})
			break
		}
	}
	return out
}

// maxIcon caps an instance picture read from another launcher.
const maxIcon = 4 << 20

// Pack turns a source of launcher f into a pack to install: its game folder
// is copied, or with inPlace played where it is.
func (s Source) Pack(f Found, inPlace bool) *modpack.Pack {
	launcherID := f.ID
	p := &modpack.Pack{
		Origin: modpack.Origin{Source: "launcher"}, Format: f.Name,
		Name: s.Name, Minecraft: s.Minecraft, Loader: s.Loader, LoaderVersion: s.LoaderVersion,
		Folder:   &modpack.Folder{Path: s.GameDir, Skip: s.Skip, Disabled: s.Disabled, InPlace: inPlace},
		Imported: &modpack.Imported{Launcher: launcherID, Path: s.GameDir, PlaytimeMin: s.PlaytimeMin, LastPlayed: s.LastPlayed, MemoryMB: s.MemoryMB},
		Warnings: s.Warnings, Copied: true,
	}
	if s.Managed.Valid() {
		p.Managed = s.Managed
	}
	if s.Icon != "" {
		if b, err := fsx.ReadCapped(s.Icon, maxIcon); err == nil {
			p.Icon = b
		}
	}
	return p
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

// maxConfig caps a launcher's config and instance files.
const maxConfig = 4 << 20

func readCapped(p string) ([]byte, error) {
	b, err := fsx.ReadCapped(p, maxConfig)
	if errors.Is(err, fsx.ErrTooLarge) {
		return nil, errors.New("launchers: file too large")
	}
	return b, err
}

// subdirs lists the folders inside dir.
func subdirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if p := filepath.Join(dir, e.Name()); !strings.HasPrefix(e.Name(), ".") && isDir(p) {
			out = append(out, p)
		}
	}
	return out
}

// minutes turns seconds into whole minutes.
func minutes(seconds int64) int {
	if seconds <= 0 || seconds > 1<<40 {
		return 0
	}
	return int(seconds / 60)
}

// clean trims a name and caps it at the instance name length.
func clean(name, fallback string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(name))
	if r := []rune(name); len(r) > 64 {
		name = strings.TrimSpace(string(r[:64]))
	}
	if name == "" {
		return fallback
	}
	return name
}

// loaderOf maps a launcher's loader name to ours ("" for vanilla, false for
// one that can't be installed here).
func loaderOf(name string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "vanilla", "none":
		return "", true
	case "fabric":
		return "fabric", true
	case "quilt":
		return "quilt", true
	case "forge":
		return "forge", true
	case "neoforge", "neoforged":
		return "neoforge", true
	}
	return "", false
}
