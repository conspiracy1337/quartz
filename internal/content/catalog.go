package content

import (
	"log/slog"
	"path/filepath"
	"time"

	"quartz/internal/content/meta"
	"quartz/internal/core/jsonfile"
	"quartz/internal/modplatform"
)

// Ident is a file found on a mod platform.
type Ident struct {
	Source      string   `json:"source"` // modrinth | curseforge
	ProjectID   string   `json:"projectId"`
	VersionID   string   `json:"versionId"`
	Slug        string   `json:"slug,omitempty"`
	Title       string   `json:"title,omitempty"`
	Author      string   `json:"author,omitempty"`
	Version     string   `json:"version,omitempty"` // the version number shown
	ReleaseType string   `json:"releaseType,omitempty"`
	FileName    string   `json:"fileName,omitempty"`
	URL         string   `json:"url,omitempty"`
	Loaders     []string `json:"loaders,omitempty"`
	Game        []string `json:"game,omitempty"`
	Library     bool     `json:"library,omitempty"`
	Published   int64    `json:"published,omitempty"` // unix seconds
}

// identOf records a platform's version and project.
func identOf(v modplatform.Version, p modplatform.Project) *Ident {
	return &Ident{Source: string(v.Source), ProjectID: v.ProjectID, VersionID: v.ID, Slug: p.Slug, Title: p.Title, Author: p.Author,
		Version: v.Number, ReleaseType: v.Type, FileName: v.File.Name, URL: v.File.URL, Loaders: v.Loaders, Game: v.Game,
		Library: p.Library, Published: v.Published.Unix()}
}

// Known is what the launcher knows about one blob, shared by every
// instance that has it: parsed once, identified once.
type Known struct {
	Info       *meta.Info `json:"info,omitempty"`
	Bad        bool       `json:"bad,omitempty"` // not a valid item of its kind
	Modrinth   *Ident     `json:"modrinth,omitempty"`
	CurseForge *Ident     `json:"curseforge,omitempty"`
	IdentAt    int64      `json:"identAt,omitempty"` // when platforms were last asked
}

// Ref names the blob's project on every platform that knows it.
func (k Known) Ref() modplatform.Ref {
	var r modplatform.Ref
	if k.Modrinth != nil {
		r.Modrinth = k.Modrinth.ProjectID
	}
	if k.CurseForge != nil {
		r.CurseForge = k.CurseForge.ProjectID
	}
	return r
}

// Ident returns the preferred identification: Modrinth, else CurseForge.
func (k Known) Ident() *Ident {
	if k.Modrinth != nil {
		return k.Modrinth
	}
	return k.CurseForge
}

type catalogDoc struct {
	Blobs map[string]Known `json:"blobs"`
}

// catalog is store/content/catalog.json.
type catalog struct {
	doc *jsonfile.Store[catalogDoc]
}

func openCatalog(dir string, log *slog.Logger) (*catalog, error) {
	doc, err := jsonfile.Open(jsonfile.Options[catalogDoc]{
		Path:    filepath.Join(dir, "catalog.json"),
		Default: func() catalogDoc { return catalogDoc{Blobs: map[string]Known{}} },
		Normalize: func(d catalogDoc) catalogDoc {
			if d.Blobs == nil {
				d.Blobs = map[string]Known{}
			}
			return d
		},
		MaxSize: 128 << 20, Delay: 2 * time.Second, Compact: true, Log: log,
	})
	if err != nil {
		return nil, err
	}
	return &catalog{doc: doc}, nil
}

func (c *catalog) get(sha1 string) (Known, bool) {
	var k Known
	var ok bool
	c.doc.View(func(d catalogDoc) { k, ok = d.Blobs[sha1] })
	return k, ok
}

func (c *catalog) many(sha1s []string) map[string]Known {
	out := make(map[string]Known, len(sha1s))
	c.doc.View(func(d catalogDoc) {
		for _, h := range sha1s {
			if k, ok := d.Blobs[h]; ok {
				out[h] = k
			}
		}
	})
	return out
}

func (c *catalog) update(sha1 string, fn func(*Known)) {
	c.doc.Mutate(func(d *catalogDoc) {
		k := d.Blobs[sha1]
		fn(&k)
		d.Blobs[sha1] = k
	})
}
