package curseforge

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"quartz/internal/modplatform"
)

var _ modplatform.Platform = (*Client)(nil)

// gameID is Minecraft on CurseForge.
const gameID = "432"

// classIDs maps our types to CurseForge's class ids.
var classIDs = map[modplatform.ProjectType]int{
	modplatform.TypeMod: 6, modplatform.TypeModpack: 4471, modplatform.TypeResourcePack: 12,
	modplatform.TypeShader: 6552, modplatform.TypeDataPack: 6945,
}

// classPaths are the website folders of each class, for page links.
var classPaths = map[modplatform.ProjectType]string{
	modplatform.TypeMod: "mc-mods", modplatform.TypeModpack: "modpacks", modplatform.TypeResourcePack: "texture-packs",
	modplatform.TypeShader: "shaders", modplatform.TypeDataPack: "data-packs",
}

// Site is CurseForge's website.
const Site = "https://www.curseforge.com/minecraft/"

// PageOf is a project's website page, built from its type and slug.
func PageOf(t modplatform.ProjectType, slug string) string {
	p, ok := classPaths[t]
	if !ok || !slugRe.MatchString(slug) {
		return ""
	}
	return Site + p + "/" + slug
}

var sortFields = map[modplatform.Sort]string{
	modplatform.SortRelevance: "2", modplatform.SortDownloads: "6", modplatform.SortUpdated: "3",
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`)

type mod struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug"`
	Summary       string    `json:"summary"`
	ClassID       int       `json:"classId"`
	DownloadCount float64   `json:"downloadCount"`
	DateModified  time.Time `json:"dateModified"`
	AllowDist     *bool     `json:"allowModDistribution"`
	Logo          *struct {
		ThumbnailURL string `json:"thumbnailUrl"`
		URL          string `json:"url"`
	} `json:"logo"`
	Screenshots []struct {
		Title        string `json:"title"`
		ThumbnailURL string `json:"thumbnailUrl"`
		URL          string `json:"url"`
	} `json:"screenshots"`
	Links struct {
		WebsiteURL string `json:"websiteUrl"`
		WikiURL    string `json:"wikiUrl"`
		IssuesURL  string `json:"issuesUrl"`
		SourceURL  string `json:"sourceUrl"`
	} `json:"links"`
	Authors []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Categories []struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
	} `json:"categories"`
	LatestIndexes []struct {
		GameVersion string `json:"gameVersion"`
		ModLoader   *int   `json:"modLoader"`
	} `json:"latestFilesIndexes"`
}

func (m mod) typ() modplatform.ProjectType {
	for t, id := range classIDs {
		if id == m.ClassID {
			return t
		}
	}
	return modplatform.TypeMod
}

// inEvery reports whether m has a category of every group. CurseForge's
// categoryIds is one "any of" list, so with two groups or more the answer is
// narrowed here, as Modrinth does it.
func (m mod) inEvery(groups [][]string) bool {
	if len(groups) < 2 {
		return true
	}
	for _, g := range groups {
		found := false
		for _, c := range m.Categories {
			if slices.Contains(g, id(c.ID)) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (m mod) author() string {
	if len(m.Authors) > 0 {
		return m.Authors[0].Name
	}
	return ""
}

func (m mod) icon() string {
	if m.Logo == nil {
		return ""
	}
	if m.Logo.ThumbnailURL != "" {
		return m.Logo.ThumbnailURL
	}
	return m.Logo.URL
}

// page is the project's website page; the API's websiteUrl when it is on
// curseforge.com, else built from the class and slug.
func (m mod) page() string {
	if u := m.Links.WebsiteURL; strings.HasPrefix(u, Site) {
		return strings.TrimRight(u, "/")
	}
	return PageOf(m.typ(), m.Slug)
}

func jsonInts(v []string) string {
	b, _ := json.Marshal(ids(v))
	return string(b)
}

// Search runs GET /v1/mods/search.
func (c *Client) Search(ctx context.Context, q modplatform.SearchQuery) (modplatform.SearchPage, error) {
	class, ok := classIDs[q.Type]
	if !ok {
		return modplatform.SearchPage{}, errors.New("curseforge: unknown project type")
	}
	v := url.Values{"gameId": {gameID}, "classId": {strconv.Itoa(class)}, "sortOrder": {"desc"},
		"index": {strconv.Itoa(q.Offset)}, "pageSize": {strconv.Itoa(min(max(q.Limit, 1), pageSize))}}
	if f := sortFields[q.Sort]; f != "" {
		v.Set("sortField", f)
	} else {
		v.Set("sortField", "2")
	}
	if q.Text != "" {
		v.Set("searchFilter", q.Text)
	}
	if q.Game != "" {
		v.Set("gameVersion", q.Game)
	}
	var loaders []string
	for _, l := range q.Loaders {
		if t, ok := loaderType[l]; ok {
			loaders = append(loaders, strconv.Itoa(t))
		}
	}
	if len(loaders) > 0 {
		v.Set("modLoaderTypes", jsonInts(loaders))
	}
	var cats []string
	for _, group := range q.Categories {
		cats = append(cats, group...)
	}
	if len(cats) > 0 {
		v.Set("categoryIds", jsonInts(cats))
	}
	var resp struct {
		Data       []mod `json:"data"`
		Pagination struct {
			TotalCount int `json:"totalCount"`
		} `json:"pagination"`
	}
	if err := c.http.JSON(ctx, http.MethodGet, c.Base+"/v1/mods/search?"+v.Encode(), nil, nil, &resp); err != nil {
		return modplatform.SearchPage{}, err
	}
	page := modplatform.SearchPage{Total: resp.Pagination.TotalCount, Read: len(resp.Data)}
	for _, m := range resp.Data {
		if m.ID <= 0 || !m.inEvery(q.Categories) {
			continue
		}
		page.Hits = append(page.Hits, modplatform.Hit{Source: modplatform.CurseForge, ID: id(m.ID), Slug: m.Slug, Title: m.Name,
			Author: m.author(), Summary: m.Summary, Icon: m.icon(), Downloads: int64(m.DownloadCount), Updated: m.DateModified})
	}
	return page, nil
}

// Project reads GET /v1/mods/{id} and its description.
func (c *Client) Project(ctx context.Context, project string) (modplatform.Detail, error) {
	if _, err := strconv.ParseInt(project, 10, 64); err != nil {
		return modplatform.Detail{}, errors.New("curseforge: invalid project id")
	}
	var resp struct {
		Data mod `json:"data"`
	}
	if err := c.http.JSON(ctx, http.MethodGet, c.Base+"/v1/mods/"+project, nil, nil, &resp); err != nil {
		return modplatform.Detail{}, err
	}
	m := resp.Data
	if m.ID <= 0 {
		return modplatform.Detail{}, errors.New("curseforge: invalid project")
	}
	var body struct {
		Data string `json:"data"`
	}
	if err := c.http.JSON(ctx, http.MethodGet, c.Base+"/v1/mods/"+project+"/description", nil, nil, &body); err != nil {
		return modplatform.Detail{}, err
	}
	d := modplatform.Detail{
		Project: modplatform.Project{Source: modplatform.CurseForge, ID: id(m.ID), Slug: m.Slug, Title: m.Name, Author: m.author()},
		Type:    m.typ(), Summary: m.Summary, Body: body.Data, BodyHTML: true, Icon: m.icon(),
		Downloads: int64(m.DownloadCount), Updated: m.DateModified, Distributable: m.AllowDist == nil || *m.AllowDist,
		Links: modplatform.Links{Page: m.page(), Source: m.Links.SourceURL, Issues: m.Links.IssuesURL, Wiki: m.Links.WikiURL},
	}
	for _, cat := range m.Categories {
		if strings.Contains(cat.Slug, "library") || strings.Contains(cat.Slug, "api") {
			d.Library = true
		}
	}
	seen := map[string]bool{}
	for _, li := range m.LatestIndexes {
		if li.ModLoader != nil {
			for name, t := range loaderType {
				if t == *li.ModLoader && !seen[name] {
					seen[name] = true
					d.Loaders = append(d.Loaders, name)
				}
			}
		}
		if li.GameVersion != "" && !seen["v"+li.GameVersion] {
			seen["v"+li.GameVersion] = true
			d.Game = append(d.Game, li.GameVersion)
		}
	}
	for _, s := range m.Screenshots {
		u := s.URL
		if u == "" {
			u = s.ThumbnailURL
		}
		d.Gallery = append(d.Gallery, modplatform.Image{URL: u, Title: s.Title})
	}
	return d, nil
}

// Slug finds a project by its website slug within a class.
func (c *Client) Slug(ctx context.Context, t modplatform.ProjectType, slug string) (string, error) {
	class, ok := classIDs[t]
	if !ok || !slugRe.MatchString(slug) {
		return "", errors.New("curseforge: invalid slug")
	}
	v := url.Values{"gameId": {gameID}, "classId": {strconv.Itoa(class)}, "slug": {slug}}
	var resp struct {
		Data []mod `json:"data"`
	}
	if err := c.http.JSON(ctx, http.MethodGet, c.Base+"/v1/mods/search?"+v.Encode(), nil, nil, &resp); err != nil {
		return "", err
	}
	for _, m := range resp.Data {
		if m.Slug == slug && m.ID > 0 {
			return id(m.ID), nil
		}
	}
	return "", errors.New("curseforge: no project " + slug)
}

// Version reads one file (GET /v1/mods/{id}/files/{fileId}).
func (c *Client) Version(ctx context.Context, project, file string) (modplatform.Version, error) {
	if _, err := strconv.ParseInt(project, 10, 64); err != nil {
		return modplatform.Version{}, errors.New("curseforge: invalid project id")
	}
	if _, err := strconv.ParseInt(file, 10, 64); err != nil {
		return modplatform.Version{}, errors.New("curseforge: invalid file id")
	}
	var resp struct {
		Data cfFile `json:"data"`
	}
	if err := c.http.JSON(ctx, http.MethodGet, c.Base+"/v1/mods/"+project+"/files/"+file, nil, nil, &resp); err != nil {
		return modplatform.Version{}, err
	}
	vs := []modplatform.Version{resp.Data.to()}
	if err := c.annotate(ctx, vs); err != nil {
		return modplatform.Version{}, err
	}
	return vs[0], nil
}

// Files reads many files at once (POST /v1/mods/files), with each file's
// class and, for blocked files, the page to download them by hand.
func (c *Client) Files(ctx context.Context, fileIDs []string) (map[string]modplatform.Version, error) {
	out := map[string]modplatform.Version{}
	var all []modplatform.Version
	for _, chunk := range modplatform.Chunk(ids(fileIDs), batch) {
		var resp struct {
			Data []cfFile `json:"data"`
		}
		if err := c.http.JSON(ctx, http.MethodPost, c.Base+"/v1/mods/files", nil, map[string]any{"fileIds": chunk}, &resp); err != nil {
			return out, err
		}
		for _, f := range resp.Data {
			all = append(all, f.to())
		}
	}
	if err := c.annotate(ctx, all); err != nil {
		return out, err
	}
	for _, v := range all {
		out[v.ID] = v
	}
	return out, nil
}

// annotate fills each version's class and, when blocked, its download page
// from the projects (POST /v1/mods).
func (c *Client) annotate(ctx context.Context, vs []modplatform.Version) error {
	var pids []string
	seen := map[string]bool{}
	for _, v := range vs {
		if !seen[v.ProjectID] {
			seen[v.ProjectID] = true
			pids = append(pids, v.ProjectID)
		}
	}
	mods := map[string]mod{}
	for _, chunk := range modplatform.Chunk(ids(pids), batch) {
		var resp struct {
			Data []mod `json:"data"`
		}
		if err := c.http.JSON(ctx, http.MethodPost, c.Base+"/v1/mods", nil, map[string]any{"modIds": chunk}, &resp); err != nil {
			return err
		}
		for _, m := range resp.Data {
			mods[id(m.ID)] = m
		}
	}
	for i := range vs {
		m, ok := mods[vs[i].ProjectID]
		if !ok {
			continue
		}
		vs[i].Class = m.typ()
		if vs[i].File.URL == "" {
			if p := m.page(); p != "" {
				vs[i].File.Page = p + "/download/" + vs[i].ID
			}
		}
	}
	return nil
}

// Categories lists GET /v1/categories for one class.
func (c *Client) Categories(ctx context.Context, t modplatform.ProjectType) ([]modplatform.Category, error) {
	class, ok := classIDs[t]
	if !ok {
		return nil, errors.New("curseforge: unknown project type")
	}
	var resp struct {
		Data []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Slug string `json:"slug"`
		} `json:"data"`
	}
	if err := c.http.JSON(ctx, http.MethodGet, c.Base+"/v1/categories?gameId="+gameID+"&classId="+strconv.Itoa(class), nil, nil, &resp); err != nil {
		return nil, err
	}
	out := make([]modplatform.Category, 0, len(resp.Data))
	for _, cat := range resp.Data {
		out = append(out, modplatform.Category{ID: id(cat.ID), Slug: cat.Slug, Name: cat.Name})
	}
	return out, nil
}
