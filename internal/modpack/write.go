package modpack

import (
	"encoding/json"
	"errors"
	"io"
)

// Index is what an exported .mrpack's modrinth.index.json describes.
type Index struct {
	Name          string
	Version       string
	Summary       string
	Minecraft     string
	Loader        string // fabric | quilt | forge | neoforge | "" (vanilla)
	LoaderVersion string // as the loader's own site names it ("0.16.9", "47.3.0")
	Files         []IndexFile
}

// IndexFile is a file the importer downloads instead of finding it bundled.
type IndexFile struct {
	Path     string // slash path in the game folder, without ".disabled"
	SHA1     string
	SHA512   string
	Size     int64
	URL      string
	Optional bool // a turned-off mod: imported as ".disabled"
	// ClientOnly is a mod a server built from the pack leaves out.
	ClientOnly bool
}

// ErrBadIndexFile refuses a file the format can't link (no allowed address,
// a missing checksum or an unsafe path); it belongs in the overrides.
var ErrBadIndexFile = errors.New("modpack: that file can't be linked in an .mrpack")

// Linkable reports whether f may be listed in an .mrpack: an https address
// on one of the format's hosts and both checksums. readMrpack accepts
// exactly these.
func Linkable(f IndexFile) bool {
	if _, err := CleanPath(f.Path); err != nil {
		return false
	}
	return allowedURL(f.URL, mrpackHosts) && hexOf(f.SHA1, 40) && hexOf(f.SHA512, 128) && f.Size >= 0 && f.Size <= maxFileSize
}

// mrLoaderKeys is mrLoaders the other way round.
var mrLoaderKeys = func() map[string]string {
	out := make(map[string]string, len(mrLoaders))
	for key, kind := range mrLoaders {
		out[kind] = key
	}
	return out
}()

type mrEnv struct {
	Client string `json:"client"`
	Server string `json:"server"`
}

type mrOutFile struct {
	Path      string            `json:"path"`
	Hashes    map[string]string `json:"hashes"`
	Env       *mrEnv            `json:"env,omitempty"`
	Downloads []string          `json:"downloads"`
	FileSize  int64             `json:"fileSize"`
}

type mrOut struct {
	FormatVersion int               `json:"formatVersion"`
	Game          string            `json:"game"`
	VersionID     string            `json:"versionId"`
	Name          string            `json:"name"`
	Summary       string            `json:"summary,omitempty"`
	Files         []mrOutFile       `json:"files"`
	Dependencies  map[string]string `json:"dependencies"`
}

// WriteIndex writes modrinth.index.json (format version 1).
func WriteIndex(w io.Writer, ix Index) error {
	if ix.Minecraft == "" {
		return ErrNoMinecraft
	}
	deps := map[string]string{"minecraft": ix.Minecraft}
	if ix.Loader != "" {
		key, ok := mrLoaderKeys[ix.Loader]
		if !ok || ix.LoaderVersion == "" {
			return errors.New("modpack: an .mrpack can't name that mod loader")
		}
		deps[key] = ix.LoaderVersion
	}
	out := mrOut{FormatVersion: 1, Game: "minecraft", VersionID: orElse(ix.Version, "1.0.0"), Name: ix.Name, Summary: ix.Summary,
		Files: make([]mrOutFile, 0, len(ix.Files)), Dependencies: deps}
	for _, f := range ix.Files {
		if !Linkable(f) {
			return ErrBadIndexFile
		}
		mf := mrOutFile{Path: f.Path, Hashes: map[string]string{"sha1": f.SHA1, "sha512": f.SHA512}, Downloads: []string{f.URL}, FileSize: f.Size}
		switch {
		case f.Optional:
			mf.Env = &mrEnv{Client: "optional", Server: "optional"}
		case f.ClientOnly:
			mf.Env = &mrEnv{Client: "required", Server: "unsupported"}
		}
		out.Files = append(out.Files, mf)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// CurseManifest is what an exported CurseForge zip's manifest.json says.
type CurseManifest struct {
	Name          string
	Version       string
	Author        string
	Minecraft     string
	Loader        string // fabric | quilt | forge | neoforge | "" (vanilla)
	LoaderVersion string // as the loader's own site names it ("47.3.0")
	MemoryMB      int    // recommendedRam; 0 leaves it out
	Files         []CurseFile
}

// CurseFile is a file the CurseForge app downloads by project and file id.
type CurseFile struct {
	Project, File int64
	Required      bool // false: a turned-off mod, optional in the pack
}

// CurseLoaderID is a manifest's loader id, readCurse's cfLoader the other
// way round ("forge-47.3.0"; NeoForge for 1.20.1 is "neoforge-1.20.1-47.1.106").
func CurseLoaderID(kind, mc, version string) string {
	if kind == "neoforge" && mc == "1.20.1" {
		return kind + "-" + mc + "-" + version
	}
	return kind + "-" + version
}

// WriteCurseManifest writes manifest.json (manifest version 1).
func WriteCurseManifest(w io.Writer, m CurseManifest) error {
	if m.Minecraft == "" {
		return ErrNoMinecraft
	}
	type loader struct {
		ID      string `json:"id"`
		Primary bool   `json:"primary"`
	}
	type file struct {
		ProjectID int64 `json:"projectID"`
		FileID    int64 `json:"fileID"`
		Required  bool  `json:"required"`
	}
	out := struct {
		Minecraft struct {
			Version        string   `json:"version"`
			ModLoaders     []loader `json:"modLoaders"`
			RecommendedRAM int      `json:"recommendedRam,omitempty"`
		} `json:"minecraft"`
		ManifestType    string `json:"manifestType"`
		ManifestVersion int    `json:"manifestVersion"`
		Name            string `json:"name"`
		Version         string `json:"version"`
		Author          string `json:"author"`
		Files           []file `json:"files"`
		Overrides       string `json:"overrides"`
	}{ManifestType: "minecraftModpack", ManifestVersion: 1, Name: m.Name, Version: orElse(m.Version, "1.0.0"), Author: m.Author,
		Files: make([]file, 0, len(m.Files)), Overrides: "overrides"}
	out.Minecraft.Version = m.Minecraft
	out.Minecraft.ModLoaders = []loader{}
	if m.Loader != "" {
		if m.LoaderVersion == "" {
			return errors.New("modpack: a CurseForge pack needs the loader's version")
		}
		out.Minecraft.ModLoaders = append(out.Minecraft.ModLoaders, loader{ID: CurseLoaderID(m.Loader, m.Minecraft, m.LoaderVersion), Primary: true})
	}
	out.Minecraft.RecommendedRAM = m.MemoryMB
	for _, f := range m.Files {
		if f.Project <= 0 || f.File <= 0 {
			return ErrBadIndexFile
		}
		out.Files = append(out.Files, file{ProjectID: f.Project, FileID: f.File, Required: f.Required})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
