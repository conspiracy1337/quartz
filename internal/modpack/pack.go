// Package modpack reads modpacks from every format the launcher imports
// (Modrinth .mrpack, CurseForge zips, Prism and MultiMC instance zips,
// Technic, FTB) into one Pack, installs a pack as a new instance and updates
// it later. Everything a pack says is untrusted: every path is checked
// before anything is written, downloads need https and a checksum, links
// and launcher folders are refused, and nothing a pack carries is run (an
// instance zip's commands and JVM arguments are never read).
package modpack

import (
	"errors"
	"path"
	"strings"
)

// Origin is where a pack comes from, for its updates.
type Origin struct {
	Source  string `json:"source"` // modrinth | curseforge | ftb | technic | file
	Project string `json:"project,omitempty"`
	Version string `json:"version,omitempty"`
	Slug    string `json:"slug,omitempty"`
}

// Updatable reports whether the pack's platform can be asked for newer versions.
func (o Origin) Updatable() bool {
	return (o.Source == "modrinth" || o.Source == "curseforge" || o.Source == "ftb") && o.Project != ""
}

// File is one file a pack downloads.
type File struct {
	Path   string   `json:"path"` // slash path in the game folder (optional files end in .disabled)
	SHA1   string   `json:"sha1,omitempty"`
	SHA512 string   `json:"sha512,omitempty"`
	MD5    string   `json:"md5,omitempty"` // Technic Solder publishes only md5
	Size   int64    `json:"size,omitempty"`
	URLs   []string `json:"urls,omitempty"`
	Name   string   `json:"name,omitempty"` // the project, when known
	// Project ("curseforge:238222") and Version ("15.2.0.27") identify the
	// file when the pack says (CurseForge and FTB list projects).
	Project string `json:"project,omitempty"`
	Version string `json:"version,omitempty"`
	Page   string   `json:"page,omitempty"` // blocked on CurseForge: where to get it by hand
	// Extract unpacks the download into the game folder (Technic Solder mods).
	Extract bool `json:"extract,omitempty"`
}

// Blocked reports whether the file must be downloaded by hand.
func (f File) Blocked() bool { return len(f.URLs) == 0 }

// Pack is a modpack read from any format.
type Pack struct {
	Origin Origin
	// Format names where the pack is from for people: "Modrinth",
	// "CurseForge", "Prism or MultiMC", "Technic", "FTB".
	Format        string
	Name          string
	Version       string
	Summary       string
	Minecraft     string
	Loader        string // "" for vanilla
	LoaderVersion string // as the pack names it; matched against the registry
	Files         []File
	// Archive is the zip holding the override layers ("" when none).
	Archive string
	// Layers are folders in Archive copied onto the game folder, in order
	// (later ones win): "overrides/", then "client-overrides/".
	Layers []string
	// Skip lists folders inside a layer that never reach the game folder.
	Skip     []string
	Icon     []byte
	IconURL  string
	Warnings []string
}

// Errors for broken or hostile packs.
var (
	ErrNotPack     = errors.New("modpack: this isn't a modpack the launcher can read")
	ErrUnsafePath  = errors.New("modpack: the pack names a file outside its instance")
	ErrNoMinecraft = errors.New("modpack: the pack doesn't say which Minecraft version it needs")
	ErrBlockedPack = errors.New("modpack: the pack's author only allows downloads from the CurseForge website; download it there and drop the zip here")
)

// Caps on what a pack may hold.
const (
	maxFiles     = 20_000
	maxFileSize  = 512 << 20
	maxLayerFile = 512 << 20
	maxLayers    = 4 << 30
	maxManifest  = 16 << 20
	maxIcon      = 4 << 20
)

// reserved are Windows device names no file may be called.
var reserved = map[string]bool{"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true, "com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true}

// CleanPath checks a path a pack names and returns it slash-separated and
// relative to the game folder. Absolute paths, "..", drive letters, links
// into the launcher's own folders (.quartz, packwiz .index) and names
// Windows can't hold are refused.
func CleanPath(p string) (string, error) {
	p = strings.ReplaceAll(p, "\\", "/")
	for strings.HasPrefix(p, "./") {
		p = p[2:]
	}
	if p == "" || len(p) > 1024 || strings.HasPrefix(p, "/") || strings.ContainsAny(p, ":*?\"<>|\x00") {
		return "", ErrUnsafePath
	}
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if s == "" && i == len(segs)-1 {
			return "", ErrUnsafePath // a folder, not a file
		}
		lower := strings.ToLower(s)
		base, _, _ := strings.Cut(lower, ".")
		switch {
		case s == "" || s == "." || s == ".." || len(s) > 255:
			return "", ErrUnsafePath
		case strings.HasSuffix(s, ".") || strings.HasSuffix(s, " "):
			return "", ErrUnsafePath
		case reserved[base]:
			return "", ErrUnsafePath
		case lower == ".quartz" || lower == ".index":
			return "", ErrUnsafePath
		}
		for _, r := range s {
			if r < 0x20 || r == 0x7f {
				return "", ErrUnsafePath
			}
		}
	}
	return path.Clean(p), nil
}

// Stats are the Detected card's numbers.
type Stats struct {
	Mods       int      `json:"mods"`
	ModBytes   int64    `json:"modBytes"`
	Files      int      `json:"files"` // override files
	Folders    []string `json:"folders"`
	Blocked    int      `json:"blocked"`
	Foreign    []string `json:"foreign"` // jars from outside the pack's platform
	NeedsWorld int      `json:"needsWorld,omitempty"`
}
