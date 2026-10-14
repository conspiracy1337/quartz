// Package meta reads what mods and packs say about themselves: ids, names,
// versions, authors, dependencies, icons and pack formats. Every input is
// untrusted: reads are size-capped, text is cleaned and truncated, and a
// malformed file degrades to what could be read instead of failing the scan.
package meta

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	"quartz/internal/core/archive"
	"quartz/internal/core/version"
)

// Read limits for entries inside a jar or pack.
const (
	maxText = 1 << 20 // fabric.mod.json, mods.toml, pack.mcmeta, MANIFEST.MF
	maxIcon = 4 << 20
)

// Formats a file's metadata came from.
const (
	FormatFabric   = "fabric"
	FormatQuilt    = "quilt"
	FormatForge    = "forge"
	FormatNeoForge = "neoforge"
	FormatModInfo  = "mcmod.info"
	FormatPack     = "pack"
	FormatShader   = "shader"
)

// DepKind says how a mod relates to another.
type DepKind string

const (
	Required DepKind = "required"
	Breaks   DepKind = "breaks"
)

// Sides a mod can be limited to; "" means both (or unknown).
const (
	SideClient = "client"
	SideServer = "server"
)

// Dep is a relation to another mod, by mod id. Range limits the versions it
// applies to, written in the Info's RangeStyle ("" applies to every version).
type Dep struct {
	ID    string  `json:"id"`
	Kind  DepKind `json:"kind"`
	Range string  `json:"range,omitempty"`
}

// Info is what a file says about itself.
type Info struct {
	Format      string      `json:"format,omitempty"`
	ID          string      `json:"id,omitempty"`
	Name        string      `json:"name,omitempty"`
	Version     string      `json:"version,omitempty"`
	Description string      `json:"description,omitempty"`
	Authors     []string    `json:"authors,omitempty"`
	Loaders     []string    `json:"loaders,omitempty"`
	Deps        []Dep       `json:"deps,omitempty"`
	Provides    []string    `json:"provides,omitempty"`
	Library     bool        `json:"library,omitempty"`
	Icon        string      `json:"icon,omitempty"` // sha256 of the stored icon
	Pack        *PackFormat `json:"pack,omitempty"`
	// Game is the range of Minecraft versions the mod says it runs on.
	Game string `json:"game,omitempty"`
	// Side is SideClient or SideServer when the mod runs on only one side.
	Side string `json:"side,omitempty"`
	// Partial is set when the metadata was malformed and only partly read.
	Partial bool `json:"partial,omitempty"`
}

// RangeStyle is how the file writes its version ranges: Fabric and Quilt
// predicates, or Forge's Maven ranges.
func (i Info) RangeStyle() version.Style {
	if i.Format == FormatFabric || i.Format == FormatQuilt {
		return version.Fabric
	}
	return version.Maven
}

// ErrNotContent means the file isn't a mod, pack or shader pack at all.
var ErrNotContent = errors.New("meta: not a recognised mod or pack")

// Source is a zip (archive.Reader) or a folder (Dir) to read entries from.
type Source interface {
	Read(name string, limit int64) ([]byte, error)
	Has(name string) bool
	HasPrefix(prefix string) bool
}

// Icons stores icon images and names them; icons.Library implements it.
type Icons interface {
	Add(data []byte) (string, error)
}

var _ Source = (*archive.Reader)(nil)

// Dir reads entries of a pack folder through an os.Root, so a link inside
// the folder can't reach outside it.
type Dir struct{ Root *os.Root }

// Read returns one file, capped at limit bytes.
func (d Dir) Read(name string, limit int64) ([]byte, error) {
	f, err := d.Root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fs.ErrNotExist
	}
	if info.Size() > limit {
		return nil, archive.ErrTooLarge
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err == nil && int64(len(b)) > limit {
		err = archive.ErrTooLarge
	}
	return b, err
}

// Has reports whether a regular file exists at name.
func (d Dir) Has(name string) bool {
	info, err := d.Root.Lstat(name)
	return err == nil && info.Mode().IsRegular()
}

// HasPrefix reports whether a folder (prefix ends in "/") or file exists.
func (d Dir) HasPrefix(prefix string) bool {
	info, err := d.Root.Lstat(strings.TrimSuffix(prefix, "/"))
	return err == nil && (info.IsDir() || info.Mode().IsRegular())
}

// icon reads, normalises and stores an icon; a bad icon is just left out.
func icon(src Source, name string, ic Icons) string {
	name = strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(name, "\\", "/")), "/")
	if ic == nil || name == "" || name == "." {
		return ""
	}
	data, err := src.Read(name, maxIcon)
	if err != nil {
		return ""
	}
	hash, err := ic.Add(data)
	if err != nil {
		return ""
	}
	return hash
}
