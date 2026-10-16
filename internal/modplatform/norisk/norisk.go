// Package norisk is a small built-in platform for NoRiskClient's own mods,
// NoRiskClient Mod (NRC Standalone) and NoRiskClient Mod Mini (NRC Mini),
// which are on neither Modrinth nor CurseForge. Their versions come from
// NoRisk's public pack configs, and the jars download straight from NoRisk's
// Maven repository: they are All Rights Reserved, so nothing is mirrored.
package norisk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"quartz/internal/core/flightx"
	"quartz/internal/core/version"
	"quartz/internal/modplatform"
)

var _ modplatform.Platform = (*Client)(nil)

const (
	// ConfigBase serves NoRisk's pack configs (packs/<artifact>.json), the
	// same files NoRisk's own launcher reads.
	ConfigBase = "https://raw.githubusercontent.com/NoRiskClient/nrc-configs/HEAD/packs/"
	// MavenBase is NoRisk's Maven repository, at the gg.norisk group.
	MavenBase = "https://maven.norisk.gg/repository/norisk-production/gg/norisk/"
	// Site is NoRisk's website; its pictures show on the project pages.
	Site   = "https://norisk.gg"
	author = "NoRisk"
	// configLife is how long a pack config is used before it is read again
	// (NoRisk publishes builds several times a day).
	configLife = time.Hour
)

// mod is one of the built-in projects.
type mod struct {
	id       string // the project id: the mod id in the jar
	artifact string // the Maven artifact and the pack config's name
	title    string
	summary  string
	features []string
	aliases  []string // what a search may type for it
}

var mods = []mod{
	{
		id: "nrcstandalone", artifact: "nrc-standalone", title: "NoRiskClient Mod",
		summary:  "The full NoRiskClient as one mod: every module, HUD and feature, with no extra launcher.",
		features: []string{"Every NoRiskClient module and HUD", "Cosmetics, capes and emotes", "Friends and hosted worlds", "Fabric, Forge and NeoForge"},
		aliases:  []string{"NoRiskClient Mod", "NRC Mod", "NoRisk Mod", "NoRiskClient Standalone Mod"},
	},
	{
		id: "nrcmini", artifact: "nrc-mini", title: "NoRiskClient Mod Mini",
		summary:  "NoRiskClient's cosmetics and emotes as one small mod, without its HUD and quality-of-life modules.",
		features: []string{"Cosmetics, capes and emotes", "Friends and hosted worlds", "No HUD or quality-of-life modules", "Fabric, Forge and NeoForge"},
		aliases:  []string{"NoRiskClient Mod Mini", "NRC Mini", "NoRisk Mini"},
	},
}

func byID(id string) *mod {
	for i := range mods {
		if mods[i].id == id {
			return &mods[i]
		}
	}
	return nil
}

// Pictures on NoRisk's website (remoteimg allows norisk.gg).
var (
	icon    = Site + "/icon-192.png"
	gallery = []modplatform.Image{
		{URL: Site + "/images/hero-mod-menu.png", Title: "The NoRiskClient mod menu"},
		{URL: Site + "/images/hero-artwork.jpg", Title: "NoRiskClient"},
	}
)

// Doer is the HTTP client (netx.Client): JSON for configs, Do for the
// Maven files' checksums and sizes.
type Doer interface {
	modplatform.Doer
	Do(req *http.Request) (*http.Response, error)
}

// Client is safe for concurrent use.
type Client struct {
	http Doer
	// ConfigBase and MavenBase are the package's constants; tests point
	// them at a local server.
	ConfigBase, MavenBase string
	Now                   func() time.Time

	flight flightx.Group[*pack]
	mu     sync.Mutex
	packs  map[string]cachedPack
	files  map[string]fileInfo // by jar URL; a build never changes
}

type cachedPack struct {
	p  *pack
	at time.Time
}

// fileInfo is what the Maven repository says about one jar.
type fileInfo struct {
	size int64
	date time.Time
	sha1 string // "" until a download needs it
}

// New returns a client.
func New(d Doer) *Client {
	return &Client{http: d, ConfigBase: ConfigBase, MavenBase: MavenBase, Now: time.Now, packs: map[string]cachedPack{}, files: map[string]fileInfo{}}
}

func (c *Client) Source() modplatform.Source { return modplatform.NoRisk }
func (c *Client) Ready() bool                { return true }

// ---- pack config ----

// config is the part of a pack config the launcher reads.
type config struct {
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Mods        []struct {
		Source struct {
			Type       string `json:"type"`
			GroupID    string `json:"groupId"`
			ArtifactID string `json:"artifactId"`
		} `json:"source"`
		// Compatibility maps a Minecraft version and a loader to a build.
		Compatibility map[string]map[string]struct {
			Identifier string `json:"identifier"`
		} `json:"compatibility"`
	} `json:"mods"`
}

// pack is a mod's config read: its description and every build, newest
// Minecraft version first.
type pack struct {
	description string
	builds      []build
}

type build struct {
	game, loader, identifier string
}

var (
	gameRe  = regexp.MustCompile(`^[0-9A-Za-z._-]{1,32}$`)
	buildRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+_-]{0,95}$`)
	loaders = []string{"fabric", "forge", "neoforge", "quilt"}
)

// pack reads a mod's config, cached for configLife.
func (c *Client) pack(ctx context.Context, m *mod) (*pack, error) {
	c.mu.Lock()
	if cp, ok := c.packs[m.artifact]; ok && c.Now().Sub(cp.at) < configLife {
		c.mu.Unlock()
		return cp.p, nil
	}
	c.mu.Unlock()
	return c.flight.Do(ctx, m.artifact, func(ctx context.Context) (*pack, error) {
		var cfg config
		if err := c.http.JSON(ctx, http.MethodGet, c.ConfigBase+m.artifact+".json", nil, nil, &cfg); err != nil {
			return nil, err
		}
		p, err := parse(cfg, m)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.packs[m.artifact] = cachedPack{p: p, at: c.Now()}
		c.mu.Unlock()
		return p, nil
	})
}

var errConfig = errors.New("norisk: the pack config names no builds of this mod")

// parse keeps only builds of m's own Maven artifact with safe names, so a
// changed config can never point a download anywhere else.
func parse(cfg config, m *mod) (*pack, error) {
	p := &pack{description: strings.TrimSpace(cfg.Description)}
	if len(p.description) > 600 {
		p.description = ""
	}
	for _, md := range cfg.Mods {
		s := md.Source
		if s.Type != "maven" || s.GroupID != "gg.norisk" || s.ArtifactID != m.artifact {
			continue
		}
		for game, byLoader := range md.Compatibility {
			if !gameRe.MatchString(game) {
				continue
			}
			for l, b := range byLoader {
				if slices.Contains(loaders, l) && buildRe.MatchString(b.Identifier) && !strings.Contains(b.Identifier, "..") {
					p.builds = append(p.builds, build{game: game, loader: l, identifier: b.Identifier})
				}
			}
		}
	}
	if len(p.builds) == 0 {
		return nil, errConfig
	}
	// sort.Slice, not slices.SortFunc: a generic sort costs ~15 KB of binary per element type.
	sort.Slice(p.builds, func(i, j int) bool {
		a, b := p.builds[i], p.builds[j]
		if n := version.Compare(a.game, b.game); n != 0 {
			return n > 0
		}
		return slices.Index(loaders, a.loader) < slices.Index(loaders, b.loader)
	})
	return p, nil
}

// ---- versions ----

// versionID is a build's id: its identifier holds dots and plus signs, so
// the id is a short hash of it (stable, and valid on every API path).
func versionID(identifier string) string {
	sum := sha256.Sum256([]byte(identifier))
	return hex.EncodeToString(sum[:8])
}

// number is how a build shows: "26.4.1342909" for "26.4.1342909-working+fabric.26.2".
func number(identifier string) string {
	n, _, _ := strings.Cut(identifier, "+")
	return strings.TrimSuffix(n, "-working")
}

func (c *Client) jarURL(m *mod, identifier string) string {
	return c.MavenBase + m.artifact + "/" + identifier + "/" + m.artifact + "-" + identifier + ".jar"
}

func (c *Client) version(m *mod, b build) modplatform.Version {
	u := c.jarURL(m, b.identifier)
	v := modplatform.Version{Source: modplatform.NoRisk, ProjectID: m.id, ID: versionID(b.identifier), Number: number(b.identifier), Type: "release",
		Loaders: []string{b.loader}, Game: []string{b.game}, File: modplatform.File{Name: m.artifact + "-" + b.identifier + ".jar", URL: u}}
	c.mu.Lock()
	if fi, ok := c.files[u]; ok {
		v.File.Size, v.File.SHA1, v.Published = fi.size, fi.sha1, fi.date
	}
	c.mu.Unlock()
	return v
}

// fits reports whether a build runs on the query's game and loaders.
func fits(b build, q modplatform.Query) bool {
	return (len(q.Game) == 0 || slices.Contains(q.Game, b.game)) && (len(q.Loaders) == 0 || slices.Contains(q.Loaders, b.loader))
}

// Versions lists a mod's builds, newest Minecraft version first, with each
// jar's size and date.
func (c *Client) Versions(ctx context.Context, project string, q modplatform.Query) ([]modplatform.Version, error) {
	m := byID(project)
	if m == nil {
		return nil, modplatform.ErrRef
	}
	p, err := c.pack(ctx, m)
	if err != nil {
		return nil, err
	}
	var out []modplatform.Version
	for _, b := range p.builds {
		if fits(b, q) {
			out = append(out, c.version(m, b))
		}
	}
	// A query for one instance names a build or two: those are ready to install.
	full := len(out) <= 2
	c.describe(ctx, out, full)
	for i := range out {
		out[i] = c.refresh(out[i])
	}
	return out, nil
}

// describe asks the Maven repository about the jars it doesn't know yet:
// size and date, and with full the checksum a download needs. Up to six at
// once; a jar that can't be asked about stays without.
func (c *Client) describe(ctx context.Context, vs []modplatform.Version, full bool) {
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for _, v := range vs {
		if v.File.Size > 0 && (!full || v.File.SHA1 != "") {
			continue
		}
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			_, _ = c.info(ctx, v.File.URL, full)
		})
	}
	wg.Wait()
}

func (c *Client) refresh(v modplatform.Version) modplatform.Version {
	c.mu.Lock()
	defer c.mu.Unlock()
	if fi, ok := c.files[v.File.URL]; ok {
		v.File.Size, v.File.SHA1, v.Published = fi.size, fi.sha1, fi.date
	}
	return v
}

// info is a jar's size and date (HEAD) and, with sum, its sha1 (the .sha1
// file beside it), cached for good: a build never changes. Two callers
// asking at once may both ask; describe never asks twice for one jar.
func (c *Client) info(ctx context.Context, u string, sum bool) (fileInfo, error) {
	c.mu.Lock()
	fi, ok := c.files[u]
	c.mu.Unlock()
	if ok && (!sum || fi.sha1 != "") {
		return fi, nil
	}
	if !ok {
		var err error
		if fi, err = c.head(ctx, u); err != nil {
			return fi, err
		}
	}
	if sum {
		s, err := c.sha1(ctx, u+".sha1")
		if err != nil {
			return fi, err
		}
		fi.sha1 = s
	}
	c.mu.Lock()
	c.files[u] = fi
	c.mu.Unlock()
	return fi, nil
}

func (c *Client) head(ctx context.Context, u string) (fileInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	if err != nil {
		return fileInfo{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fileInfo{}, err
	}
	_ = resp.Body.Close()
	fi := fileInfo{size: resp.ContentLength}
	if t, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
		fi.date = t
	}
	if fi.size <= 0 {
		return fileInfo{}, errors.New("norisk: the repository gave no size for " + u)
	}
	return fi, nil
}

var sha1Re = regexp.MustCompile(`^[0-9a-f]{40}$`)

func (c *Client) sha1(ctx context.Context, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", err
	}
	fields := strings.Fields(strings.ToLower(string(b)))
	if len(fields) == 0 || !sha1Re.MatchString(fields[0]) {
		return "", errors.New("norisk: no checksum for " + u)
	}
	return fields[0], nil
}

// find is one build of a mod by version id, ready to download.
func (c *Client) find(ctx context.Context, m *mod, id string) (modplatform.Version, error) {
	p, err := c.pack(ctx, m)
	if err != nil {
		return modplatform.Version{}, err
	}
	for _, b := range p.builds {
		if versionID(b.identifier) == id {
			return c.ready(ctx, c.version(m, b))
		}
	}
	return modplatform.Version{}, modplatform.ErrRef
}

// ready fills in what a download needs: size, date and checksum.
func (c *Client) ready(ctx context.Context, v modplatform.Version) (modplatform.Version, error) {
	if _, err := c.info(ctx, v.File.URL, true); err != nil {
		return v, err
	}
	return c.refresh(v), nil
}

// ---- Provider ----

// Identify finds nothing: installs from here record what they are.
func (c *Client) Identify(context.Context, []modplatform.Hashes) (map[string]modplatform.Version, error) {
	return map[string]modplatform.Version{}, nil
}

func (c *Client) Projects(_ context.Context, ids []string) (map[string]modplatform.Project, error) {
	out := map[string]modplatform.Project{}
	for _, id := range ids {
		if m := byID(id); m != nil {
			out[id] = c.project(m)
		}
	}
	return out, nil
}

func (c *Client) project(m *mod) modplatform.Project {
	return modplatform.Project{Source: modplatform.NoRisk, ID: m.id, Slug: m.id, Title: m.title, Author: author, Icon: icon, Summary: m.summary, ServerSide: "unsupported"}
}

// Latest is the config's current build for each item's game and loader.
func (c *Client) Latest(ctx context.Context, items []modplatform.Installed, loader string, q modplatform.Query) (map[string]modplatform.Version, error) {
	out := map[string]modplatform.Version{}
	var errs []error
	for _, it := range items {
		vs, err := c.Versions(ctx, it.ProjectID, modplatform.Query{Game: q.Game, Loaders: []string{loader}})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if len(vs) > 0 && vs[0].File.SHA1 != "" {
			out[it.Key] = vs[0]
		}
	}
	return out, errors.Join(errs...)
}

// Changelogs: NoRisk publishes none per build.
func (c *Client) Changelogs(context.Context, []modplatform.Version) error { return nil }

// ---- Catalog ----

// matches reports whether a search names m: a query of three letters or
// more that is part of one of its names, spaces and case ignored.
func matches(m *mod, text string) bool {
	q := modplatform.Normalize(text)
	if len(q) < 3 {
		return false
	}
	for _, a := range m.aliases {
		if strings.Contains(modplatform.Normalize(a), q) {
			return true
		}
	}
	return false
}

// Search lists the mods a search names that have a build for the query's
// game and loaders. Only mods, only the first page.
func (c *Client) Search(ctx context.Context, q modplatform.SearchQuery) (modplatform.SearchPage, error) {
	page := modplatform.SearchPage{}
	if q.Type != modplatform.TypeMod || q.Offset > 0 || len(q.Categories) > 0 || q.Side == modplatform.SideServer {
		return page, nil
	}
	vq := modplatform.Query{Loaders: q.Loaders}
	if q.Game != "" {
		vq.Game = []string{q.Game}
	}
	for i := range mods {
		m := &mods[i]
		if !matches(m, q.Text) {
			continue
		}
		p, err := c.pack(ctx, m)
		if err != nil {
			continue // a built-in extra: its mod is left out, the search goes on
		}
		var newest *build
		for j := range p.builds {
			if fits(p.builds[j], vq) {
				newest = &p.builds[j]
				break
			}
		}
		if newest == nil {
			continue
		}
		h := modplatform.Hit{Source: modplatform.NoRisk, ID: m.id, Slug: m.id, Title: m.title, Author: author, Summary: m.summary, Icon: icon}
		if fi, err := c.info(ctx, c.jarURL(m, newest.identifier), false); err == nil {
			h.Updated = fi.date
		}
		page.Hits = append(page.Hits, h)
	}
	page.Total = len(page.Hits)
	return page, nil
}

// Project is a mod's page: the config's description with NoRisk's pictures.
func (c *Client) Project(ctx context.Context, id string) (modplatform.Detail, error) {
	m := byID(id)
	if m == nil {
		return modplatform.Detail{}, modplatform.ErrRef
	}
	p, err := c.pack(ctx, m)
	if err != nil {
		return modplatform.Detail{}, err
	}
	d := modplatform.Detail{Project: c.project(m), Type: modplatform.TypeMod, Summary: m.summary, Icon: icon, License: "All rights reserved",
		Client: "required", Server: "unsupported", Gallery: gallery, Distributable: true, Versions: len(p.builds),
		Links: modplatform.Links{Page: Site, Website: Site}}
	for _, b := range p.builds {
		if !slices.Contains(d.Loaders, b.loader) {
			d.Loaders = append(d.Loaders, b.loader)
		}
		if !slices.Contains(d.Game, b.game) {
			d.Game = append(d.Game, b.game)
		}
	}
	if fi, err := c.info(ctx, c.jarURL(m, p.builds[0].identifier), false); err == nil {
		d.Updated = fi.date
	}
	d.Body = body(m, p)
	return d, nil
}

// body is the page's description in Markdown: NoRisk's own words from the
// config, what the mod brings, and where it comes from.
func body(m *mod, p *pack) string {
	var b strings.Builder
	b.WriteString("![" + m.title + "](" + gallery[1].URL + ")\n\n")
	b.WriteString(orElse(p.description, m.summary) + "\n\n## What it brings\n\n")
	for _, f := range m.features {
		b.WriteString("- " + f + "\n")
	}
	b.WriteString("\n## Where it comes from\n\nBuilt and published by NoRisk. Every build downloads straight from NoRisk's own repository at maven.norisk.gg, " +
		"and the mod signs in to NoRisk's servers when the game starts. More at norisk.gg.\n")
	return b.String()
}

func orElse(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (c *Client) Slug(_ context.Context, t modplatform.ProjectType, slug string) (string, error) {
	if m := byID(slug); m != nil && t == modplatform.TypeMod {
		return m.id, nil
	}
	return "", modplatform.ErrRef
}

func (c *Client) Version(ctx context.Context, project, id string) (modplatform.Version, error) {
	m := byID(project)
	if m == nil {
		return modplatform.Version{}, modplatform.ErrRef
	}
	return c.find(ctx, m, id)
}

// Files finds builds of either mod by version id.
func (c *Client) Files(ctx context.Context, ids []string) (map[string]modplatform.Version, error) {
	out := map[string]modplatform.Version{}
	for _, id := range ids {
		for i := range mods {
			if v, err := c.find(ctx, &mods[i], id); err == nil {
				out[id] = v
				break
			}
		}
	}
	return out, nil
}

func (c *Client) Categories(context.Context, modplatform.ProjectType) ([]modplatform.Category, error) {
	return nil, nil
}
