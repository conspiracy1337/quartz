// Package content manages what goes into an instance besides the game:
// mods, resource packs, shader packs and data packs. One scanner serves all
// four through a small per-kind strategy, so every tab behaves the same.
package content

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"

	"quartz/internal/content/meta"
	"quartz/internal/core/archive"
)

// Kind is one content tab.
type Kind string

const (
	Mods          Kind = "mods"
	ResourcePacks Kind = "resourcepacks"
	Shaders       Kind = "shaders"
	DataPacks     Kind = "datapacks"
)

// top is the instance folder a kind's items live under (data packs live in
// worlds); it decides whether they are in the game or the shared folder.
func (k Kind) top() string {
	switch k {
	case DataPacks:
		return "saves"
	case Shaders:
		return "shaderpacks"
	}
	return string(k)
}

// Kinds lists every kind, in tab order.
var Kinds = []Kind{Mods, ResourcePacks, Shaders, DataPacks}

// ErrKind is returned for an unknown kind.
var ErrKind = errors.New("content: unknown content kind")

// ParseKind validates a kind name.
func ParseKind(s string) (Kind, error) {
	for _, k := range Kinds {
		if string(k) == s {
			return k, nil
		}
	}
	return "", ErrKind
}

// Disabled is the suffix that hides a mod or data pack from the game.
const Disabled = ".disabled"

// folder is one place a kind keeps its items, relative to the game folder.
type folder struct {
	Rel   string // slash-separated
	Group string // the world, for data packs
}

// toggleKind says where an item's on/off state lives.
type toggleKind int

const (
	toggleSuffix   toggleKind = iota // .disabled (mods, data packs)
	togglePackList                   // options.txt resourcePacks (resource packs)
	toggleShader                     // iris/oculus properties (shaders)
)

// spec is everything that differs between kinds.
type spec struct {
	ext     string // the file extension items have (before .disabled)
	dirs    bool   // folders can be items (packs)
	toggle  toggleKind
	indexIn string // where .pw.toml files go, relative to the folder
	folders func(root *os.Root) []folder
	parse   func(src meta.Source, zip *archive.Reader, ic meta.Icons) (meta.Info, error)
}

var specs = map[Kind]spec{
	Mods: {ext: ".jar", toggle: toggleSuffix, indexIn: ".index",
		folders: fixed("mods"),
		parse: func(_ meta.Source, zip *archive.Reader, ic meta.Icons) (meta.Info, error) {
			if zip == nil {
				return meta.Info{}, meta.ErrNotContent
			}
			return meta.Mod(zip, ic)
		}},
	ResourcePacks: {ext: ".zip", dirs: true, toggle: togglePackList, indexIn: ".index",
		folders: fixed("resourcepacks"),
		parse:   func(src meta.Source, _ *archive.Reader, ic meta.Icons) (meta.Info, error) { return meta.Pack(src, ic) }},
	// Iris lists every subfolder of shaderpacks/ as a pack, so packwiz files
	// sit beside the packs there (as Prism does).
	Shaders: {ext: ".zip", dirs: true, toggle: toggleShader, indexIn: ".",
		folders: fixed("shaderpacks"),
		parse:   func(src meta.Source, _ *archive.Reader, _ meta.Icons) (meta.Info, error) { return meta.Shader(src) }},
	DataPacks: {ext: ".zip", dirs: true, toggle: toggleSuffix, indexIn: ".index",
		folders: worlds,
		parse:   func(src meta.Source, _ *archive.Reader, ic meta.Icons) (meta.Info, error) { return meta.Pack(src, ic) }},
}

func fixed(rel string) func(*os.Root) []folder {
	return func(*os.Root) []folder { return []folder{{Rel: rel}} }
}

// worlds lists saves/<world>/datapacks for every world.
func worlds(root *os.Root) []folder {
	f, err := root.Open("saves")
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	entries, _ := f.ReadDir(-1)
	var out []folder
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, folder{Rel: path.Join("saves", e.Name(), "datapacks"), Group: e.Name()})
		}
	}
	return out
}

// accepts reports whether a folder entry can be an item of this kind, and
// whether it is switched off by suffix.
func (sp spec) accepts(name string, mode fs.FileMode) (ok, off bool) {
	if strings.HasPrefix(name, ".") || mode&fs.ModeSymlink != 0 {
		return false, false
	}
	base, off := strings.CutSuffix(name, Disabled)
	if mode.IsDir() {
		// A folder pack is turned off like a file (see packManifest).
		return sp.dirs && (!off || sp.toggle == toggleSuffix), off && sp.toggle == toggleSuffix
	}
	if !mode.IsRegular() {
		return false, false
	}
	return strings.EqualFold(path.Ext(base), sp.ext), off && sp.toggle == toggleSuffix
}

// packManifest is a folder pack's pack.mcmeta. Minecraft loads every folder
// in datapacks/ that has one, whatever it is called, so turning a folder pack
// off also renames its manifest (offManifest): the game then skips it.
const (
	packManifest = "pack.mcmeta"
	offManifest  = packManifest + Disabled
)

// offPack reads a turned-off folder pack, whose manifest is offManifest.
type offPack struct{ meta.Dir }

func (o offPack) Read(name string, limit int64) ([]byte, error) {
	if name == packManifest {
		name = offManifest
	}
	return o.Dir.Read(name, limit)
}

func (o offPack) Has(name string) bool {
	if name == packManifest {
		name = offManifest
	}
	return o.Dir.Has(name)
}

// itemID names an item stably across toggles: its kind and path without the
// .disabled suffix, hashed so no path ever reaches the UI.
func itemID(k Kind, rel string) string {
	sum := sha256.Sum256([]byte(string(k) + "\x00" + strings.TrimSuffix(rel, Disabled)))
	return hex.EncodeToString(sum[:8])
}

// ValidItemID reports whether s can name an item.
func ValidItemID(s string) bool {
	if len(s) != 16 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil && strings.ToLower(s) == s
}
