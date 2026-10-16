package browse

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"quartz/internal/content"
	"quartz/internal/core/netx"
	"quartz/internal/modplatform"
)

const (
	pageSize    = 20
	sessionLife = 10 * time.Minute
	maxSessions = 8
)

// Query is what the Browse page asks for.
type Query struct {
	Type     string `json:"type"` // mod | modpack | resourcepack | shader | datapack | world
	Text     string `json:"text"`
	Sort     string `json:"sort"` // relevance | downloads | updated
	Instance string `json:"instance"`
	Version  bool   `json:"version"` // only the instance's Minecraft version
	// Game is a Minecraft version picked by hand (modpacks, which go into
	// no instance); it wins over Version.
	Game       string   `json:"game"`
	Loader     bool     `json:"loader"` // only the instance's loader
	Categories []string `json:"categories"`
	HideAI     bool     `json:"hideAi"`
	HideAds    bool     `json:"hideAds"`
	// Side keeps projects for one side only: "client", "server" or "".
	Side string `json:"side"`
	// HideInstalled leaves out what the instance already has.
	HideInstalled bool `json:"hideInstalled"`
}

// Stats is one platform's numbers for a row.
type Stats struct {
	Downloads int64 `json:"downloads"`
	Updated   int64 `json:"updated"` // unix ms
}

// Row is one project in the results.
type Row struct {
	ID         string `json:"id"`  // stable for the search, even when a twin merges in
	Ref        string `json:"ref"` // the project on every platform found so far
	Name       string `json:"name"`
	Author     string `json:"author"`
	Summary    string `json:"summary"`
	Icon       string `json:"icon,omitempty"` // same-origin picture
	Modrinth   *Stats `json:"modrinth,omitempty"`
	CurseForge *Stats `json:"curseforge,omitempty"`
	NoRisk     *Stats `json:"norisk,omitempty"`
	// Installed is true when the target instance already has the project.
	Installed bool `json:"installed,omitempty"`
}

// Results is one page: new rows to append, rows already shown that changed
// (a twin from the other platform merged in), and whether more pages exist.
type Results struct {
	SearchID string `json:"searchId"`
	Rows     []Row  `json:"rows"`
	Changed  []Row  `json:"changed"`
	More     bool   `json:"more"`
	// Failed names the platforms that couldn't be searched for this page.
	Failed []string `json:"failed"`
}

// gameRe is what a Minecraft version id looks like ("1.20.1", "24w14a", "1.21-pre1").
var gameRe = regexp.MustCompile(`^[0-9A-Za-z._-]{1,32}$`)

// Content disclosures Modrinth can filter on (Prism's list).
var (
	aiDisclosures  = []string{"ai_content", "ai_content_code", "ai_content_assets", "ai_content_text", "ai_content_functionality"}
	adsDisclosures = []string{"advertisements", "telemetry", "telemetry_always_active", "telemetry_opt_out"}
)

type source struct {
	plat   modplatform.Platform
	q      modplatform.SearchQuery
	offset int
	done   bool
}

type session struct {
	mu        sync.Mutex
	id        string
	sort      modplatform.Sort
	sources   []*source
	rows      []*row
	byKey     map[string]*row // match key → row
	byID      map[string]*row // platform key → row
	installed map[string]bool // platform keys the instance already has
	hide      bool            // leave installed projects out
	used      time.Time
}

type row struct {
	Row
	key   string
	ref   modplatform.Ref
	sent  bool
	dirty bool
}

// Search starts a search and returns its first page.
func (s *Service) Search(ctx context.Context, q Query) (Results, error) {
	t := modplatform.ProjectType(q.Type)
	if !t.Valid() {
		return Results{}, errors.New("browse: unknown project type")
	}
	srt := modplatform.Sort(q.Sort)
	if srt != modplatform.SortDownloads && srt != modplatform.SortUpdated {
		srt = modplatform.SortRelevance
	}
	tgt, err := s.target(q.Instance)
	if err != nil {
		return Results{}, err
	}
	base := modplatform.SearchQuery{Type: t, Text: strings.TrimSpace(q.Text), Sort: srt, Limit: pageSize}
	if q.Side == modplatform.SideClient || q.Side == modplatform.SideServer {
		base.Side = q.Side
	}
	if len(base.Text) > 200 {
		base.Text = base.Text[:200]
	}
	ss := &session{id: newID(), sort: srt, byKey: map[string]*row{}, byID: map[string]*row{}, installed: map[string]bool{}, hide: q.HideInstalled, used: s.d.Now()}
	if t == modplatform.TypeModpack {
		tgt = nil // a pack becomes a new instance: the target's version, loader and content don't apply
	}
	if gameRe.MatchString(q.Game) {
		base.Game = q.Game
	}
	if tgt != nil {
		if q.Version && base.Game == "" {
			base.Game = tgt.Minecraft
		}
		if q.Loader && t == modplatform.TypeMod {
			base.Loaders = content.LoaderChain(content.Mods, tgt.Loader, tgt.Minecraft)
		}
		for _, in := range tgt.Installed {
			for _, src := range modplatform.Sources {
				if id := in.Ident(src); id != nil {
					ss.installed[modplatform.Key(src, id.ProjectID)] = true
				}
			}
		}
	}
	mrCats, cfCats, modrinth, curse := s.categoryFilter(ctx, t, q.Categories)
	for _, p := range s.d.Platforms {
		if !p.Ready() {
			continue
		}
		sq := base
		switch p.Source() {
		case modplatform.Modrinth:
			if t == modplatform.TypeWorld || !modrinth {
				continue // Modrinth hosts no worlds, or can't filter by this
			}
			sq.Categories = mrCats
			if q.HideAI {
				sq.Exclude = append(sq.Exclude, aiDisclosures...)
			}
			if q.HideAds {
				sq.Exclude = append(sq.Exclude, adsDisclosures...)
			}
		case modplatform.CurseForge:
			if !curse || base.Side != "" {
				continue // CurseForge can't filter by this
			}
			sq.Categories = cfCats
		case modplatform.NoRisk:
			sq.Categories = mrCats // any category leaves its mods out
		}
		ss.sources = append(ss.sources, &source{plat: p, q: sq})
	}
	s.keep(ss)
	return s.next(ctx, ss)
}

// More returns the next page of a search.
func (s *Service) More(ctx context.Context, searchID string) (Results, error) {
	s.mu.Lock()
	ss, ok := s.sessions[searchID]
	s.mu.Unlock()
	if !ok {
		return Results{}, ErrUnknownSearch
	}
	return s.next(ctx, ss)
}

func (s *Service) keep(ss *session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.d.Now()
	var oldest *session
	for id, o := range s.sessions {
		if now.Sub(o.used) > sessionLife {
			delete(s.sessions, id)
			continue
		}
		if oldest == nil || o.used.Before(oldest.used) {
			oldest = o
		}
	}
	if len(s.sessions) >= maxSessions && oldest != nil {
		delete(s.sessions, oldest.id)
	}
	s.sessions[ss.id] = ss
}

// next fetches one page from every platform with results left and merges it.
func (s *Service) next(ctx context.Context, ss *session) (Results, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.used = s.d.Now()
	pages := make([][]modplatform.Hit, len(ss.sources))
	errs := make([]error, len(ss.sources))
	var wg sync.WaitGroup
	for i, src := range ss.sources {
		if src.done {
			continue
		}
		wg.Go(func() {
			q := src.q
			q.Offset = src.offset
			page, err := s.search(ctx, src.plat, q)
			if err != nil {
				errs[i] = err
				return
			}
			pages[i] = page.Hits
			src.offset += page.Used()
			src.done = page.Used() == 0 || src.offset >= page.Total
		})
	}
	wg.Wait()
	failed := []string{}
	for i, err := range errs {
		if err != nil {
			failed = append(failed, string(ss.sources[i].plat.Source()))
			ss.sources[i].done = true
			s.d.Log.Warn("browse: search failed", "source", ss.sources[i].plat.Source(), "err", err)
		}
	}
	if len(failed) > 0 && len(failed) == len(ss.sources) {
		return Results{}, errs[0]
	}
	res := Results{SearchID: ss.id, Rows: []Row{}, Changed: []Row{}, Failed: failed}
	for _, h := range order(pages, ss.sort) {
		s.merge(ss, h)
	}
	for _, r := range ss.rows {
		switch {
		case !r.sent:
			r.sent, r.dirty = true, false
			res.Rows = append(res.Rows, r.Row)
		case r.dirty:
			r.dirty = false
			res.Changed = append(res.Changed, r.Row)
		}
	}
	for _, src := range ss.sources {
		res.More = res.More || !src.done
	}
	return res, nil
}

// search asks one platform, dropping Modrinth's disclosure filters for good
// if Modrinth refuses them.
func (s *Service) search(ctx context.Context, p modplatform.Platform, q modplatform.SearchQuery) (modplatform.SearchPage, error) {
	if s.noDisclose.Load() {
		q.Exclude = nil
	}
	page, err := p.Search(ctx, q)
	var se *netx.StatusError
	if err != nil && len(q.Exclude) > 0 && errors.As(err, &se) && se.Code == 400 {
		if !s.noDisclose.Swap(true) {
			s.d.Log.Warn("browse: Modrinth refused content disclosure filters; searching without them")
		}
		q.Exclude = nil
		return p.Search(ctx, q)
	}
	return page, err
}

// order lays one page of each platform out in the session's order:
// alternating by rank for relevance, else by the sort's number.
func order(pages [][]modplatform.Hit, srt modplatform.Sort) []modplatform.Hit {
	var out []modplatform.Hit
	for i := 0; ; i++ {
		added := false
		for _, p := range pages {
			if i < len(p) {
				out = append(out, p[i])
				added = true
			}
		}
		if !added {
			break
		}
	}
	switch srt {
	case modplatform.SortDownloads:
		sort.SliceStable(out, func(a, b int) bool { return out[a].Downloads > out[b].Downloads })
	case modplatform.SortUpdated:
		sort.SliceStable(out, func(a, b int) bool { return out[a].Updated.After(out[b].Updated) })
	}
	// A built-in mod is only found by its own name, so it leads whatever the sort.
	sort.SliceStable(out, func(a, b int) bool { return out[a].Source == modplatform.NoRisk && out[b].Source != modplatform.NoRisk })
	return out
}

// merge adds a hit to the session: into its twin's row when the other
// platform already listed the project, else as a new row at the end.
func (s *Service) merge(ss *session, h modplatform.Hit) {
	id := modplatform.Key(h.Source, h.ID)
	if _, dup := ss.byID[id]; dup {
		return
	}
	stats := &Stats{Downloads: h.Downloads, Updated: h.Updated.UnixMilli()}
	key := matchKey(h.Title, h.Author)
	if r, ok := ss.byKey[key]; ok && key != "" && r.ref.ID(h.Source) == "" {
		ss.byID[id] = r
		r.ref = r.ref.With(h.Source, h.ID)
		r.Ref = r.ref.String()
		setStats(&r.Row, h.Source, stats)
		if r.Icon == "" {
			r.Icon = s.d.Pictures.Src(h.Icon)
		}
		if ss.installed[id] {
			if ss.hide {
				r.dirty = false
				ss.removeRow(r)
				return
			}
			r.Installed = true
		}
		r.dirty = r.sent
		return
	}
	ref := modplatform.RefOf(h.Source, h.ID)
	r := &row{key: key, ref: ref, Row: Row{ID: ref.String(), Ref: ref.String(), Name: h.Title, Author: h.Author, Summary: h.Summary, Icon: s.d.Pictures.Src(h.Icon)}}
	setStats(&r.Row, h.Source, stats)
	ss.byID[id] = r
	if _, taken := ss.byKey[key]; !taken && key != "" {
		ss.byKey[key] = r
	}
	if ss.installed[id] {
		if ss.hide {
			return
		}
		r.Installed = true
	}
	ss.rows = append(ss.rows, r)
}

// removeRow drops a row not yet shown (one shown stays, so nothing moves).
func (ss *session) removeRow(r *row) {
	if r.sent {
		return
	}
	for i, o := range ss.rows {
		if o == r {
			ss.rows = append(ss.rows[:i], ss.rows[i+1:]...)
			return
		}
	}
}

func setStats(r *Row, src modplatform.Source, st *Stats) {
	switch src {
	case modplatform.CurseForge:
		r.CurseForge = st
	case modplatform.NoRisk:
		r.NoRisk = st
	default:
		r.Modrinth = st
	}
}

// matchKey is how a project is recognised on both platforms: its title and
// author, lower case, letters and digits only ("Xaero's Minimap" by
// "xaero96" → "xaerosminimap/xaero96").
func matchKey(title, author string) string {
	t := modplatform.Normalize(title)
	if t == "" {
		return "" // nothing to match on
	}
	return t + "/" + modplatform.Normalize(author)
}
