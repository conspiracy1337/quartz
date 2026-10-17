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
	"strconv"
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
	// Site is NoRisk's website: its pictures show on the project pages, and
	// its front page says how many played in the last 24 hours.
	Site   = "https://norisk.gg"
	author = "NoRisk"
	// configLife is how long a pack config is used before it is read again
	// (NoRisk publishes builds several times a day).
	configLife = time.Hour
	// playersRetry is how long an unreadable player count waits before the
	// site is asked again.
	playersRetry = 5 * time.Minute
)

// mod is one of the built-in projects. Its page is written here: the pack
// configs' descriptions are short and their text is mis-encoded. The page
// shows the gallery's first picture above the body.
type mod struct {
	id       string // the project id: the mod id in the jar
	artifact string // the Maven artifact and the pack config's name
	title    string
	summary  string
	body     string // Markdown, before the shared footer
	gallery  []modplatform.Image
	aliases  []string // what a search may type for it
}

// Pictures on NoRisk's website (remoteimg allows norisk.gg).
var (
	icon      = Site + "/icon-192.png"
	menu      = modplatform.Image{URL: Site + "/images/hero-mod-menu.png", Title: "The NoRiskClient menu"}
	cosmetics = modplatform.Image{URL: Site + "/images/features/cosmetics-poster.webp", Title: "The cosmetics shop"}
)

var mods = []mod{
	{
		id: "nrcstandalone", artifact: "nrc-standalone", title: "NoRiskClient Mod",
		summary: "The whole NoRiskClient in one mod: modules, HUD, cosmetics and friends, without NoRisk's launcher.",
		body: "The full NoRiskClient packed into one mod. Add it to a Fabric, Forge or NeoForge instance and you get everything the client offers, with no separate launcher.\n\n" +
			"## What's inside\n\n" +
			"- 80+ modules: zoom, freelook, fullbright, keystrokes, coordinates, FPS, motion blur, waypoints and more\n" +
			"- HUD editor: 20+ HUD modules, placed and styled the way you like\n" +
			"- Cosmetics: 300+ cosmetics, emotes and free custom capes, seen by everyone on NoRiskClient\n" +
			"- Friends and chats: see who's online, join them in one click, message them in game\n" +
			"- Hosted Worlds: open your singleplayer world to friends without a server\n" +
			"- McReal and minigames: one screenshot a day with friends, plus Tetris and Pong\n",
		gallery: []modplatform.Image{menu,
			{URL: Site + "/images/features/hud-editor-poster.webp", Title: "HUD modules in game"},
			cosmetics,
			{URL: Site + "/images/features/mcreal-poster.webp", Title: "McReal, a daily screenshot with friends"},
			{URL: Site + "/images/features/minigames-poster.webp", Title: "Minigames like Tetris, in game"}},
		aliases: []string{"NoRiskClient Mod", "NRC Mod", "NoRisk Mod", "NoRiskClient Standalone Mod"},
	},
	{
		id: "nrcmini", artifact: "nrc-mini", title: "NoRiskClient Mod Mini",
		summary: "NoRiskClient's social side in one small mod: cosmetics, emotes, friends and hosted worlds. No modules, no HUD.",
		body: "The social side of NoRiskClient as one lightweight mod: look the part and play with friends, without changing anything else about your game.\n\n" +
			"## What's inside\n\n" +
			"- Cosmetics and capes: 300+ cosmetics and free custom capes, seen by everyone on NoRiskClient\n" +
			"- Emotes: emotes other NoRiskClient players see\n" +
			"- Friends: see who's online and join them in one click\n" +
			"- Hosted Worlds: open your singleplayer world to friends without a server\n\n" +
			"## What it leaves out\n\n" +
			"No modules, no HUD, no quality-of-life tweaks: the game plays exactly as before. Want those? Get NoRiskClient Mod instead.\n",
		gallery: []modplatform.Image{cosmetics},
		aliases: []string{"NoRiskClient Mod Mini", "NRC Mini", "NoRisk Mini"},
	},
}

// footer ends both pages: who makes the mod and what it talks to.
const footer = "\n## Good to know\n\nMade by NoRisk. Builds download straight from NoRisk's repository at maven.norisk.gg, " +
	"and the mod signs in to NoRisk's servers when the game starts. More at norisk.gg.\n"

func byID(id string) *mod {
	for i := range mods {
		if mods[i].id == id {
			return &mods[i]
		}
	}
	return nil
}

// Doer is the HTTP client (netx.Client): JSON for configs, Do for the
// Maven files' checksums and sizes.
type Doer interface {
	modplatform.Doer
	Do(req *http.Request) (*http.Response, error)
}

// Client is safe for concurrent use.
type Client struct {
	http Doer
	// ConfigBase, MavenBase and SiteBase are the package's constants (the
	// last with a slash); tests point them at a local server.
	ConfigBase, MavenBase, SiteBase string
	Now                             func() time.Time

	flight flightx.Group[*pack]
	mu     sync.Mutex
	packs  map[string]cachedPack
	files  map[string]fileInfo // by jar URL; a build never changes

	// playersMu is held while the site is read, so callers at once share
	// one read; players is the last count (0: unreadable) and playersAt when.
	playersMu sync.Mutex
	players   int64
	playersAt time.Time
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
	return &Client{http: d, ConfigBase: ConfigBase, MavenBase: MavenBase, SiteBase: Site + "/", Now: time.Now, packs: map[string]cachedPack{}, files: map[string]fileInfo{}}
}

func (c *Client) Source() modplatform.Source { return modplatform.NoRisk }
func (c *Client) Ready() bool                { return true }

// ---- pack config ----

// config is the part of a pack config the launcher reads.
type config struct {
	DisplayName string `json:"displayName"`
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

// pack is a mod's config read: every build, newest Minecraft version first.
type pack struct {
	builds []build
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
	p := &pack{}
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
		h := modplatform.Hit{Source: modplatform.NoRisk, ID: m.id, Slug: m.id, Title: m.title, Author: author, Summary: m.summary, Icon: icon, Daily: c.dailyPlayers(ctx)}
		if fi, err := c.info(ctx, c.jarURL(m, newest.identifier), false); err == nil {
			h.Updated = fi.date
		}
		page.Hits = append(page.Hits, h)
	}
	page.Total = len(page.Hits)
	return page, nil
}

// Project is a mod's page, written here, with NoRisk's pictures.
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
		Client: "required", Server: "unsupported", Gallery: m.gallery, Distributable: true, Versions: len(p.builds),
		Daily: c.dailyPlayers(ctx), Body: m.body + footer, Links: modplatform.Links{Page: Site, Website: Site}}
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
	return d, nil
}

// ---- players ----

// playersRe finds the count in the front page's server-rendered data, where
// it shows as "players":41967 (quotes escaped inside a script string).
var playersRe = regexp.MustCompile(`\\?"players\\?":([0-9]{1,9})[,}]`)

// dailyPlayers is how many played NoRiskClient in the last 24 hours, as
// norisk.gg's front page says, or 0 when it can't be read. It is read at
// most once per configLife (playersRetry after a failure).
func (c *Client) dailyPlayers(ctx context.Context) int64 {
	c.playersMu.Lock()
	defer c.playersMu.Unlock()
	life := configLife
	if c.players == 0 {
		life = playersRetry
	}
	if !c.playersAt.IsZero() && c.Now().Sub(c.playersAt) < life {
		return c.players
	}
	n := c.readPlayers(ctx)
	if ctx.Err() != nil {
		return n // a cancelled read says nothing about the site
	}
	c.players, c.playersAt = n, c.Now()
	return n
}

func (c *Client) readPlayers(ctx context.Context) int64 {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.SiteBase, nil)
	if err != nil {
		return 0
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0
	}
	m := playersRe.FindSubmatch(b)
	if m == nil {
		return 0
	}
	n, _ := strconv.ParseInt(string(m[1]), 10, 64)
	return n
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
