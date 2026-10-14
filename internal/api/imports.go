package api

import (
	"errors"
	"os"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/internal/instance"
	"quartz/internal/loader"
	"quartz/internal/modpack"
	"quartz/internal/modplatform"
)

// ImportsDeps are what the import facade works with.
type ImportsDeps struct {
	Reader  *modpack.Reader
	Packs   *modpack.Service
	Drops   *Drops
	Opens   *Opens
	Picture func(url string) string // same-origin address of a platform picture
}

// Imports reads modpacks from files and links for the Import screen.
type Imports struct {
	ctx *Ctx
	d   ImportsDeps
}

// NewImports returns the facade.
func NewImports(ctx *Ctx, d ImportsDeps) *Imports { return &Imports{ctx: ctx, d: d} }

// ImportPreview is the Import screen's Detected card.
type ImportPreview struct {
	Token     string        `json:"token"`
	Name      string        `json:"name"`
	Version   string        `json:"version"`
	Minecraft string        `json:"minecraft"`
	Loader    string        `json:"loader"` // "Fabric", or "" for vanilla
	Source    string        `json:"source"` // "Modrinth", "CurseForge", "FTB", …
	Icon      string        `json:"icon,omitempty"`
	Stats     modpack.Stats `json:"stats"`
	Warnings  []string      `json:"warnings"`
	// Optional are files the pack lets the player choose (off by default).
	Optional []modpack.OptionalFile `json:"optional"`
}

// ErrBadLink is returned for links that are too long to be real.
var ErrBadLink = errors.New("api: invalid link")

func (im *Imports) preview(p *modpack.Pack, err error) (*ImportPreview, error) {
	if err != nil {
		return nil, err
	}
	st, err := p.Stats()
	if err != nil {
		return nil, err
	}
	v := &ImportPreview{Token: im.d.Packs.Stage(p), Name: p.Name, Version: p.Version, Minecraft: p.Minecraft, Source: p.Format,
		Stats: st, Warnings: append([]string{}, p.Warnings...), Optional: p.Optional()}
	if k, err := loader.ParseKind(p.Loader); err == nil {
		v.Loader = k.Name()
	}
	if im.d.Picture != nil && p.IconURL != "" {
		v.Icon = im.d.Picture(p.IconURL)
	}
	if v.Name == "" {
		v.Name = "Imported pack"
	}
	return v, nil
}

// InspectLink reads the pack a pasted link names.
func (im *Imports) InspectLink(link string) (*ImportPreview, error) {
	link = strings.TrimSpace(link)
	if len(link) > 2048 {
		return nil, ErrBadLink
	}
	if err := checkHTTPS(link); err != nil {
		return nil, err
	}
	return im.preview(im.d.Reader.Link(im.ctx.Get(), link))
}

// InspectFile asks for a pack file and reads it (nil when cancelled).
func (im *Imports) InspectFile() (*ImportPreview, error) {
	path, err := openFileDialog(im.ctx.Get(), wruntime.OpenDialogOptions{
		Title:   "Choose a modpack",
		Filters: []wruntime.FileFilter{{DisplayName: "Modpacks (*.mrpack, *.zip)", Pattern: "*.mrpack;*.zip"}},
	})
	if err != nil || path == "" {
		return nil, err
	}
	return im.preview(im.d.Reader.File(im.ctx.Get(), path))
}

// InspectDropped reads a pack file dropped onto the Import screen.
func (im *Imports) InspectDropped(dropID string) (*ImportPreview, error) {
	paths, err := im.d.Drops.Take(dropID)
	if err != nil {
		return nil, err
	}
	if len(paths) != 1 {
		return nil, modpack.ErrNotPack
	}
	return im.preview(im.d.Reader.File(im.ctx.Get(), paths[0]))
}

// maxPackFile caps a pack file opened from outside.
const maxPackFile = 2 << 30

// InspectOpened reads a pack the launcher was asked to open from outside (a
// file, a link, or a platform project; see Opens).
func (im *Imports) InspectOpened(openID string) (*ImportPreview, error) {
	p, err := im.d.Opens.Take(openID)
	if err != nil {
		return nil, err
	}
	ctx := im.ctx.Get()
	switch {
	case p.Path != "":
		info, err := os.Lstat(p.Path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > maxPackFile {
			return nil, modpack.ErrNotPack
		}
		return im.preview(im.d.Reader.File(ctx, p.Path))
	case p.URL != "":
		return im.InspectLink(p.URL)
	}
	return im.preview(im.d.Reader.Project(ctx, p.Source, p.Project, p.Version))
}

// InspectProject reads a pack from Browse: the version picked on the project
// page, or "" for the newest release.
func (im *Imports) InspectProject(ref, source, version string) (*ImportPreview, error) {
	r, err := modplatform.ParseRef(ref)
	if err != nil {
		return nil, err
	}
	src, err := checkSource(source)
	if err != nil {
		return nil, err
	}
	if r.ID(src) == "" || len(version) > 64 {
		return nil, ErrBadSource
	}
	return im.preview(im.d.Reader.Project(im.ctx.Get(), src, r.ID(src), version))
}

// Install creates the instance from an inspected pack, with the optional
// files in enable turned on, and returns its job.
func (im *Imports) Install(token, name string, art instance.Art, enable []string) (string, error) {
	name = strings.TrimSpace(name)
	if len(name) > 64 || len(enable) > 20_000 {
		return "", instance.ErrBadName
	}
	return im.d.Packs.Install(im.ctx.Get(), token, name, art, enable)
}
