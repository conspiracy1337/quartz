package modrinth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"quartz/internal/core/netx"
	"quartz/internal/modplatform"
)

var _ modplatform.Platform = (*Client)(nil)

// Site is Modrinth's website, for project pages.
const Site = "https://modrinth.com"

var slugRe = regexp.MustCompile(`^[A-Za-z0-9!@$()` + "`" + `.+,"_-]{1,64}$`)

// projectTypes maps our types to Modrinth's project_type.
var projectTypes = map[modplatform.ProjectType]string{
	modplatform.TypeMod: "mod", modplatform.TypeModpack: "modpack", modplatform.TypeResourcePack: "resourcepack",
	modplatform.TypeShader: "shader", modplatform.TypeDataPack: "datapack",
}

var sortIndex = map[modplatform.Sort]string{
	modplatform.SortRelevance: "relevance", modplatform.SortDownloads: "downloads", modplatform.SortUpdated: "updated",
}

type hit struct {
	ProjectID    string    `json:"project_id"`
	Slug         string    `json:"slug"`
	Title        string    `json:"title"`
	Author       string    `json:"author"`
	Description  string    `json:"description"`
	IconURL      string    `json:"icon_url"`
	Downloads    int64     `json:"downloads"`
	DateModified time.Time `json:"date_modified"`
}

// facets builds the search filter: groups are AND'ed, items in a group OR'ed.
func facets(q modplatform.SearchQuery) [][]string {
	f := [][]string{{"project_type:" + projectTypes[q.Type]}}
	if len(q.Loaders) > 0 {
		var g []string
		for _, l := range q.Loaders {
			g = append(g, "categories:"+l)
		}
		f = append(f, g)
	}
	if q.Game != "" {
		f = append(f, []string{"versions:" + q.Game})
	}
	for _, group := range q.Categories {
		var g []string
		for _, c := range group {
			g = append(g, "categories:"+c)
		}
		if len(g) > 0 {
			f = append(f, g)
		}
	}
	// A client mod is needed on the client and optional or unsupported on a
	// server; a server mod the other way round (Prism's and XMCL's facets).
	switch q.Side {
	case modplatform.SideClient:
		f = append(f, []string{"client_side:required", "client_side:optional"}, []string{"server_side:optional", "server_side:unsupported"})
	case modplatform.SideServer:
		f = append(f, []string{"server_side:required", "server_side:optional"}, []string{"client_side:optional", "client_side:unsupported"})
	}
	for _, d := range q.Exclude {
		f = append(f, []string{"disclosure_types!=" + d})
	}
	return f
}

// Search runs GET /v2/search.
func (c *Client) Search(ctx context.Context, q modplatform.SearchQuery) (modplatform.SearchPage, error) {
	if _, ok := projectTypes[q.Type]; !ok {
		return modplatform.SearchPage{}, errors.New("modrinth: unknown project type")
	}
	fb, err := jsonOf(facets(q))
	if err != nil {
		return modplatform.SearchPage{}, err
	}
	v := url.Values{"facets": {fb}, "index": {sortIndex[q.Sort]}, "offset": {strconv.Itoa(q.Offset)}, "limit": {strconv.Itoa(max(q.Limit, 1))}}
	if v.Get("index") == "" {
		v.Set("index", "relevance")
	}
	if q.Text != "" {
		v.Set("query", q.Text)
	}
	var resp struct {
		Hits      []hit `json:"hits"`
		TotalHits int   `json:"total_hits"`
	}
	if err := c.http.JSON(ctx, http.MethodGet, c.Base+"/v2/search?"+v.Encode(), nil, nil, &resp); err != nil {
		return modplatform.SearchPage{}, err
	}
	page := modplatform.SearchPage{Total: resp.TotalHits}
	for _, h := range resp.Hits {
		if !modplatform.ValidID(h.ProjectID) {
			continue
		}
		page.Hits = append(page.Hits, modplatform.Hit{Source: modplatform.Modrinth, ID: h.ProjectID, Slug: h.Slug, Title: h.Title,
			Author: h.Author, Summary: h.Description, Icon: h.IconURL, Downloads: h.Downloads, Updated: h.DateModified})
	}
	return page, nil
}

type detail struct {
	project
	ProjectType string    `json:"project_type"`
	Body        string    `json:"body"`
	Downloads   int64     `json:"downloads"`
	Updated     time.Time `json:"updated"`
	Loaders     []string  `json:"loaders"`
	GameVers    []string  `json:"game_versions"`
	Versions    []string  `json:"versions"`
	License     struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"license"`
	SourceURL string `json:"source_url"`
	IssuesURL string `json:"issues_url"`
	WikiURL   string `json:"wiki_url"`
	Gallery   []struct {
		URL      string `json:"url"`
		Title    string `json:"title"`
		Featured bool   `json:"featured"`
	} `json:"gallery"`
}

// Project reads GET /v2/project/{id|slug}.
func (c *Client) Project(ctx context.Context, id string) (modplatform.Detail, error) {
	if !slugRe.MatchString(id) {
		return modplatform.Detail{}, errors.New("modrinth: invalid project id")
	}
	var d detail
	if err := c.http.JSON(ctx, http.MethodGet, c.Base+"/v2/project/"+url.PathEscape(id), nil, nil, &d); err != nil {
		return modplatform.Detail{}, err
	}
	if !modplatform.ValidID(d.ID) {
		return modplatform.Detail{}, errors.New("modrinth: invalid project")
	}
	authors := c.authors(ctx, []project{d.project})
	out := modplatform.Detail{
		Project: modplatform.Project{Source: modplatform.Modrinth, ID: d.ID, Slug: d.Slug, Title: d.Title, Author: authors[d.ID],
			Library: contains(d.Categories, "library")},
		Type: typeOf(d.ProjectType), Summary: d.Description, Body: d.Body, Icon: d.IconURL, Loaders: d.Loaders, Game: d.GameVers,
		Client: side(d.ClientSide), Server: side(d.ServerSide), Downloads: d.Downloads, Updated: d.Updated, Versions: len(d.Versions),
		Distributable: true,
		Links: modplatform.Links{Page: Site + "/" + projectTypes[typeOf(d.ProjectType)] + "/" + url.PathEscape(d.Slug),
			Source: d.SourceURL, Issues: d.IssuesURL, Wiki: d.WikiURL},
	}
	out.License = d.License.ID
	if out.License == "" || strings.HasPrefix(out.License, "LicenseRef") {
		out.License = d.License.Name
	}
	// Featured pictures first, as the website shows them.
	for _, featured := range []bool{true, false} {
		for _, g := range d.Gallery {
			if g.Featured == featured {
				out.Gallery = append(out.Gallery, modplatform.Image{URL: g.URL, Title: g.Title})
			}
		}
	}
	return out, nil
}

func typeOf(t string) modplatform.ProjectType {
	for k, v := range projectTypes {
		if v == t {
			return k
		}
	}
	return modplatform.TypeMod
}

func side(s string) string {
	switch s {
	case "required", "optional", "unsupported":
		return s
	}
	return ""
}

// serverSide keeps Modrinth's server_side answer; "unknown" (and anything
// unexpected) is recorded as SideUnknown, so "" still means "never asked".
func serverSide(s string) string {
	if v := side(s); v != "" {
		return v
	}
	return modplatform.SideUnknown
}

// Slug resolves a website slug; Modrinth answers /v2/project/{slug} directly.
func (c *Client) Slug(ctx context.Context, _ modplatform.ProjectType, slug string) (string, error) {
	d, err := c.Project(ctx, slug)
	return d.ID, err
}

// Version reads GET /v2/version/{id}.
func (c *Client) Version(ctx context.Context, project, id string) (modplatform.Version, error) {
	if !modplatform.ValidID(id) {
		return modplatform.Version{}, errors.New("modrinth: invalid version id")
	}
	var v version
	if err := c.http.JSON(ctx, http.MethodGet, c.Base+"/v2/version/"+id, nil, nil, &v); err != nil {
		return modplatform.Version{}, err
	}
	mv, ok := v.to(nil)
	if !ok || (project != "" && mv.ProjectID != project) {
		return modplatform.Version{}, errors.New("modrinth: version not found")
	}
	return mv, nil
}

// Files reads versions by id (GET /v2/versions?ids=).
func (c *Client) Files(ctx context.Context, ids []string) (map[string]modplatform.Version, error) {
	out := map[string]modplatform.Version{}
	var valid []string
	for _, id := range ids {
		if modplatform.ValidID(id) {
			valid = append(valid, id)
		}
	}
	for _, chunk := range modplatform.Chunk(valid, batch) {
		var resp []version
		if err := c.http.JSON(ctx, http.MethodGet, c.Base+"/v2/versions?ids="+jsonList(chunk), nil, nil, &resp); err != nil {
			return out, err
		}
		for _, v := range resp {
			if mv, ok := v.to(nil); ok {
				out[mv.ID] = mv
			}
		}
	}
	return out, nil
}

// Categories lists GET /v2/tag/category for one project type.
func (c *Client) Categories(ctx context.Context, t modplatform.ProjectType) ([]modplatform.Category, error) {
	var resp []struct {
		Name        string `json:"name"`
		ProjectType string `json:"project_type"`
		Header      string `json:"header"`
	}
	if err := c.http.JSON(ctx, http.MethodGet, c.Base+"/v2/tag/category", nil, nil, &resp); err != nil {
		return nil, err
	}
	var out []modplatform.Category
	for _, cat := range resp {
		if cat.ProjectType == projectTypes[t] && cat.Header == "categories" {
			out = append(out, modplatform.Category{ID: cat.Name, Slug: cat.Name, Name: cat.Name})
		}
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// versionsCtx raises the JSON cap: a long-lived project lists thousands of versions.
func versionsCtx(ctx context.Context) context.Context {
	return netx.WithBodyCap(ctx, modplatform.VersionListCap)
}
