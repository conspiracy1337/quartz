package browse

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"quartz/internal/content"
	"quartz/internal/core/docs"
	"quartz/internal/modplatform"
)

// Side is a project as one platform shows it.
type Side struct {
	Name      string       `json:"name"`
	Author    string       `json:"author"`
	Summary   string       `json:"summary"`
	Icon      string       `json:"icon,omitempty"`
	Downloads int64        `json:"downloads"`
	Updated   int64        `json:"updated"` // unix ms
	URL       string       `json:"url"`     // the project on the platform's website
	Versions  int          `json:"versions"`
	Badges    []string     `json:"badges"`
	Body      []docs.Block `json:"body"`
	Gallery   []Picture    `json:"gallery"`
	Links     []Link       `json:"links"`
	// Blocked is true when the author allows downloads only from the website.
	Blocked bool `json:"blocked,omitempty"`
}

// Picture is a gallery picture.
type Picture struct {
	Src   string `json:"src"`
	Title string `json:"title"`
}

// Link is one of a project's pages elsewhere.
type Link struct {
	Icon  string `json:"icon"`
	Label string `json:"label"`
	URL   string `json:"url"`
}

// InstalledVersion is the version of the project the target has.
type InstalledVersion struct {
	Item      string `json:"item"`
	Source    string `json:"source"`
	VersionID string `json:"versionId"`
	Version   string `json:"version"`
	Published int64  `json:"published"` // unix ms
}

// Page is the project page.
type Page struct {
	Ref        string `json:"ref"`
	Type       string `json:"type"`
	Name       string `json:"name"`
	Modrinth   *Side  `json:"modrinth,omitempty"`
	CurseForge *Side  `json:"curseforge,omitempty"`
	// Preferred is the platform shown and installed from by default, and
	// Reason why (see prefer).
	Preferred string            `json:"preferred"`
	Reason    string            `json:"reason"`
	Installed *InstalledVersion `json:"installed,omitempty"`
	// Channels are the release types the update channel offers ("release", "beta").
	Channels []string `json:"channels"`
}

// Project reads a project page from every platform that has it. A project
// known on one platform is looked for on the other by title and author.
func (s *Service) Project(ctx context.Context, ref modplatform.Ref, instance string) (Page, error) {
	ref = s.twin(ctx, ref)
	var mu sync.Mutex
	sides := map[modplatform.Source]modplatform.Detail{}
	var firstErr error
	var wg sync.WaitGroup
	var vers map[modplatform.Source][]modplatform.Version
	wg.Go(func() { vers = s.releases(ctx, ref) })
	for _, src := range ref.Sources() {
		wg.Go(func() {
			d, err := s.detail(ctx, src, ref.ID(src))
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			sides[src] = d
		})
	}
	wg.Wait()
	if len(sides) == 0 {
		if firstErr == nil {
			firstErr = errors.New("browse: no platform is available")
		}
		return Page{}, firstErr
	}
	page := Page{Ref: ref.String(), Channels: s.d.Channel().Types()}
	var have []modplatform.Source
	var updated time.Time
	for _, src := range []modplatform.Source{modplatform.Modrinth, modplatform.CurseForge} {
		d, ok := sides[src]
		if !ok {
			continue
		}
		have = append(have, src)
		side := s.side(d)
		if src == modplatform.CurseForge {
			page.CurseForge = &side
		} else {
			page.Modrinth = &side
		}
		if page.Name == "" || d.Updated.After(updated) {
			updated, page.Name, page.Type = d.Updated, d.Title, string(d.Type)
		}
	}
	tgt, err := s.target(instance)
	if err == nil && tgt != nil {
		page.Installed = installedOf(tgt, ref)
	}
	if page.Type == string(modplatform.TypeModpack) {
		tgt = nil // a pack becomes its own instance: the target's game does not narrow it
	}
	src, why := prefer(have, vers, s.d.Channel(), tgt)
	page.Preferred, page.Reason = string(src), why
	return page, nil
}

// twin completes a one-platform reference with the other platform's
// project when one matches by title and author (cached).
func (s *Service) twin(ctx context.Context, ref modplatform.Ref) modplatform.Ref {
	if ref.Modrinth != "" && ref.CurseForge != "" {
		return ref
	}
	have := ref.Sources()[0]
	other := modplatform.CurseForge
	if have == modplatform.CurseForge {
		other = modplatform.Modrinth
	}
	p := s.platform(other)
	if p == nil {
		return ref
	}
	key := ref.String()
	id, err := cached(s.twins, key, 30*time.Minute, func() (string, error) {
		d, err := s.detail(ctx, have, ref.ID(have))
		if err != nil {
			return "", err
		}
		page, err := p.Search(ctx, modplatform.SearchQuery{Type: d.Type, Text: d.Title, Sort: modplatform.SortRelevance, Limit: 10})
		if err != nil {
			return "", err
		}
		want := matchKey(d.Title, d.Author)
		for _, h := range page.Hits {
			if want != "" && matchKey(h.Title, h.Author) == want {
				return h.ID, nil
			}
		}
		return "", nil
	})
	if err != nil || id == "" {
		return ref
	}
	return withSource(ref, other, id)
}

var loaderNames = map[string]string{
	"fabric": "Fabric", "quilt": "Quilt", "forge": "Forge", "neoforge": "NeoForge",
	"iris": "Iris", "optifine": "OptiFine", "canvas": "Canvas",
}

func (s *Service) side(d modplatform.Detail) Side {
	pic := s.d.Pictures.Src
	out := Side{Name: d.Title, Author: d.Author, Summary: d.Summary, Icon: pic(d.Icon), Downloads: d.Downloads,
		Updated: d.Updated.UnixMilli(), URL: d.Links.Page, Versions: d.Versions, Blocked: !d.Distributable,
		Badges: []string{}, Gallery: []Picture{}, Links: []Link{}}
	switch {
	case d.Client != "" && d.Client != "unsupported" && d.Server == "unsupported":
		out.Badges = append(out.Badges, "Client")
	case d.Server != "" && d.Server != "unsupported" && d.Client == "unsupported":
		out.Badges = append(out.Badges, "Server")
	}
	for _, l := range d.Loaders {
		if n, ok := loaderNames[l]; ok && !slices.Contains(out.Badges, n) {
			out.Badges = append(out.Badges, n)
		}
	}
	if d.License != "" && len(d.License) <= 32 {
		out.Badges = append(out.Badges, d.License)
	}
	if d.BodyHTML {
		out.Body = docs.HTML(d.Body, docs.Picture(pic))
	} else {
		out.Body = docs.Markdown(d.Body, docs.Picture(pic))
	}
	if out.Body == nil {
		out.Body = []docs.Block{}
	}
	linkProjects(out.Body)
	for _, g := range d.Gallery {
		if src := pic(g.URL); src != "" {
			out.Gallery = append(out.Gallery, Picture{Src: src, Title: g.Title})
		}
	}
	for _, l := range []Link{{"github", "Source code", d.Links.Source}, {"issue", "Issues", d.Links.Issues}, {"globe", "Wiki", d.Links.Wiki}} {
		if strings.HasPrefix(l.URL, "https://") && len(l.URL) <= 2048 {
			out.Links = append(out.Links, l)
		}
	}
	return out
}

// installedOf finds the target's copy of a project.
func installedOf(t *Target, ref modplatform.Ref) *InstalledVersion {
	for _, in := range t.Installed {
		for _, src := range ref.Sources() {
			if id := in.Ident(src); id != nil && id.ProjectID == ref.ID(src) {
				return &InstalledVersion{Item: in.Item, Source: string(src), VersionID: id.VersionID, Version: id.Version, Published: id.Published * 1000}
			}
		}
	}
	return nil
}

// VersionRow is one version in the Versions tab.
type VersionRow struct {
	ID        string `json:"id"`
	Number    string `json:"number"`
	Channel   string `json:"channel"`   // Release | Beta | Alpha
	Minecraft string `json:"minecraft"` // "1.21, 1.21.1"
	Published int64  `json:"published"` // unix ms
	Bytes     int64  `json:"bytes"`
	// Action is what the row's button does for the target: Install, Update,
	// Switch (to an older version), or "" for the installed one.
	Action string `json:"action"`
}

// Versions lists a project's versions on one platform, newest first. With
// compatible, only versions for the target's Minecraft version and loader.
func (s *Service) Versions(ctx context.Context, ref modplatform.Ref, src modplatform.Source, instance string, compatible bool) ([]VersionRow, error) {
	id := ref.ID(src)
	if id == "" {
		return nil, modplatform.ErrRef
	}
	all, err := s.allVersions(ctx, src, id)
	if err != nil {
		return nil, err
	}
	tgt, err := s.target(instance)
	if err != nil {
		return nil, err
	}
	var inst *InstalledVersion
	if tgt != nil {
		inst = installedOf(tgt, modplatform.RefOf(src, id))
	}
	out := []VersionRow{}
	for _, v := range all {
		if compatible && tgt != nil && !fits(v, tgt) {
			continue
		}
		out = append(out, versionRow(v, inst))
	}
	return out, nil
}

// fits reports whether a version runs on the target's game and loader.
func fits(v modplatform.Version, t *Target) bool {
	if len(v.Game) > 0 && !slices.Contains(v.Game, t.Minecraft) {
		return false
	}
	chain := content.LoaderChain(content.Mods, t.Loader, t.Minecraft)
	if len(v.Loaders) == 0 || len(chain) == 0 {
		return true
	}
	modLoader := false
	for _, l := range v.Loaders {
		if slices.Contains(chain, l) {
			return true
		}
		if _, ok := loaderNames[l]; ok && l != "iris" && l != "optifine" && l != "canvas" {
			modLoader = true
		}
	}
	// Shader and resource packs list no mod loader; they fit any.
	return !modLoader
}

var channels = map[string]string{"release": "Release", "beta": "Beta", "alpha": "Alpha"}

func versionRow(v modplatform.Version, inst *InstalledVersion) VersionRow {
	r := VersionRow{ID: v.ID, Number: v.Number, Channel: channels[v.Type], Minecraft: strings.Join(v.Game, ", "),
		Published: v.Published.UnixMilli(), Bytes: v.File.Size, Action: "Install"}
	if r.Channel == "" {
		r.Channel = "Release"
	}
	if inst != nil {
		switch {
		case inst.VersionID == v.ID:
			r.Action = ""
		case r.Published > inst.Published:
			r.Action = "Update"
		default:
			r.Action = "Switch"
		}
	}
	return r
}

// Notes are one version's changes.
type Notes struct {
	ID        string   `json:"id"`
	Number    string   `json:"number"`
	Published int64    `json:"published"`
	Items     []string `json:"items"`
}

// Changelog reads the changes of the given versions (at most 5), or of the
// newest 4 when ids is empty (the Changelog tab).
func (s *Service) Changelog(ctx context.Context, ref modplatform.Ref, src modplatform.Source, ids []string) ([]Notes, error) {
	id := ref.ID(src)
	p := s.platform(src)
	if id == "" || p == nil {
		return nil, modplatform.ErrRef
	}
	all, err := s.allVersions(ctx, src, id)
	if err != nil {
		return nil, err
	}
	limit := 5
	if len(ids) == 0 {
		limit = 4
	}
	var pick []modplatform.Version
	for _, v := range all {
		if len(ids) == 0 || slices.Contains(ids, v.ID) {
			pick = append(pick, v)
		}
		if len(pick) == limit {
			break
		}
	}
	if err := p.Changelogs(ctx, pick); err != nil {
		return nil, err
	}
	out := make([]Notes, 0, len(pick))
	for _, v := range pick {
		items := modplatform.Items(v.Changelog, src == modplatform.CurseForge)
		if items == nil {
			items = []string{}
		}
		out = append(out, Notes{ID: v.ID, Number: v.Number, Published: v.Published.UnixMilli(), Items: items})
	}
	return out, nil
}
