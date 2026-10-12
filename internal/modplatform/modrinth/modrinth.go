// Package modrinth is the Modrinth provider (api.modrinth.com, no key).
package modrinth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"quartz/internal/modplatform"
)

// API is Modrinth's API base.
const API = "https://api.modrinth.com"

// batch is how many hashes or ids go into one request.
const batch = 100

// Client is the Modrinth provider.
type Client struct {
	Base string // overridden in tests
	http modplatform.Doer
}

// New returns a client for the real API.
func New(d modplatform.Doer) *Client { return &Client{Base: API, http: d} }

var _ modplatform.Provider = (*Client)(nil)

func (c *Client) Source() modplatform.Source { return modplatform.Modrinth }
func (c *Client) Ready() bool                { return true }

type file struct {
	Hashes   map[string]string `json:"hashes"`
	URL      string            `json:"url"`
	Filename string            `json:"filename"`
	Primary  bool              `json:"primary"`
	Size     int64             `json:"size"`
}

type version struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"project_id"`
	VersionNumber string    `json:"version_number"`
	VersionType   string    `json:"version_type"`
	DatePublished time.Time `json:"date_published"`
	Loaders       []string  `json:"loaders"`
	GameVersions  []string  `json:"game_versions"`
	Files         []file    `json:"files"`
	Changelog     string    `json:"changelog"`
	Dependencies  []struct {
		ProjectID      string `json:"project_id"`
		VersionID      string `json:"version_id"`
		DependencyType string `json:"dependency_type"`
	} `json:"dependencies"`
}

// to converts a version; match picks the file (by hash), else the primary.
func (v version) to(match func(file) bool) (modplatform.Version, bool) {
	if !modplatform.ValidID(v.ID) || !modplatform.ValidID(v.ProjectID) || len(v.Files) == 0 {
		return modplatform.Version{}, false
	}
	f := v.Files[0]
	for _, c := range v.Files {
		if (match != nil && match(c)) || (match == nil && c.Primary) {
			f = c
			break
		}
	}
	out := modplatform.Version{Source: modplatform.Modrinth, ProjectID: v.ProjectID, ID: v.ID, Number: v.VersionNumber,
		Type: v.VersionType, Published: v.DatePublished, Loaders: v.Loaders, Game: v.GameVersions, Changelog: v.Changelog,
		File: modplatform.File{Name: f.Filename, URL: f.URL, Size: f.Size, SHA1: f.Hashes["sha1"], SHA512: f.Hashes["sha512"]}}
	for _, d := range v.Dependencies {
		if d.ProjectID == "" || !modplatform.ValidID(d.ProjectID) {
			continue
		}
		out.Deps = append(out.Deps, modplatform.Dep{ProjectID: d.ProjectID, VersionID: d.VersionID, Kind: modplatform.DepKind(d.DependencyType)})
	}
	return out, true
}

// hashOf picks the strongest hash a file has.
func hashOf(h modplatform.Hashes) (string, string) {
	if h.SHA512 != "" {
		return h.SHA512, "sha512"
	}
	return h.SHA1, "sha1"
}

// Identify looks files up by hash (POST /v2/version_files).
func (c *Client) Identify(ctx context.Context, files []modplatform.Hashes) (map[string]modplatform.Version, error) {
	out := map[string]modplatform.Version{}
	for algo, group := range byAlgo(files) {
		for _, chunk := range modplatform.Chunk(group, batch) {
			hashes := make([]string, len(chunk))
			back := map[string]string{}
			for i, f := range chunk {
				hashes[i], _ = hashOf(f)
				back[hashes[i]] = f.SHA1
			}
			var resp map[string]version
			if err := c.http.JSON(ctx, http.MethodPost, c.Base+"/v2/version_files", nil, map[string]any{"hashes": hashes, "algorithm": algo}, &resp); err != nil {
				return out, err
			}
			for h, v := range resp {
				sha1, ok := back[strings.ToLower(h)]
				if !ok {
					continue
				}
				if mv, ok := v.to(func(f file) bool { return f.Hashes[algo] == h }); ok {
					out[sha1] = mv
				}
			}
		}
	}
	return out, nil
}

func byAlgo(files []modplatform.Hashes) map[string][]modplatform.Hashes {
	out := map[string][]modplatform.Hashes{}
	for _, f := range files {
		_, algo := hashOf(f)
		out[algo] = append(out[algo], f)
	}
	return out
}

// Latest asks for the newest version of every file at once
// (POST /v2/version_files/update), for one loader.
func (c *Client) Latest(ctx context.Context, items []modplatform.Installed, loader string, q modplatform.Query) (map[string]modplatform.Version, error) {
	out := map[string]modplatform.Version{}
	byHash := map[string][]string{}
	var hashes []string
	for _, it := range items {
		h := it.Hashes.SHA512
		if h == "" {
			continue // identified on Modrinth means its sha512 is known
		}
		if _, dup := byHash[h]; !dup {
			hashes = append(hashes, h)
		}
		byHash[h] = append(byHash[h], it.Key)
	}
	for _, chunk := range modplatform.Chunk(hashes, batch) {
		body := map[string]any{"hashes": chunk, "algorithm": "sha512", "version_types": q.Channel.Types()}
		if loader != "" {
			body["loaders"] = []string{loader}
		}
		if len(q.Game) > 0 {
			body["game_versions"] = q.Game
		}
		var resp map[string]version
		if err := c.http.JSON(ctx, http.MethodPost, c.Base+"/v2/version_files/update", nil, body, &resp); err != nil {
			return out, err
		}
		for h, v := range resp {
			mv, ok := v.to(nil)
			if !ok {
				continue
			}
			for _, key := range byHash[strings.ToLower(h)] {
				out[key] = mv
			}
		}
	}
	return out, nil
}

func jsonList(v []string) string {
	b, _ := json.Marshal(v)
	return url.QueryEscape(string(b))
}

// Versions lists a project's versions, newest first, without changelogs.
func (c *Client) Versions(ctx context.Context, project string, q modplatform.Query) ([]modplatform.Version, error) {
	if !modplatform.ValidID(project) {
		return nil, nil
	}
	u := c.Base + "/v2/project/" + project + "/version?include_changelog=false"
	if len(q.Loaders) > 0 && q.Loaders[0] != "" {
		u += "&loaders=" + jsonList(q.Loaders)
	}
	if len(q.Game) > 0 {
		u += "&game_versions=" + jsonList(q.Game)
	}
	var resp []version
	if err := c.http.JSON(ctx, http.MethodGet, u, nil, nil, &resp); err != nil {
		return nil, err
	}
	out := make([]modplatform.Version, 0, len(resp))
	for _, v := range resp {
		if mv, ok := v.to(nil); ok {
			out = append(out, mv)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Published.After(out[b].Published) })
	return out, nil
}

// Changelogs fills changelogs in (GET /v2/versions?ids=…).
func (c *Client) Changelogs(ctx context.Context, vs []modplatform.Version) error {
	idx := map[string][]int{}
	var ids []string
	for i, v := range vs {
		if modplatform.ValidID(v.ID) {
			if _, ok := idx[v.ID]; !ok {
				ids = append(ids, v.ID)
			}
			idx[v.ID] = append(idx[v.ID], i)
		}
	}
	for _, chunk := range modplatform.Chunk(ids, batch) {
		var resp []version
		if err := c.http.JSON(ctx, http.MethodGet, c.Base+"/v2/versions?ids="+jsonList(chunk), nil, nil, &resp); err != nil {
			return err
		}
		for _, v := range resp {
			for _, i := range idx[v.ID] {
				vs[i].Changelog = v.Changelog
			}
		}
	}
	return nil
}

type project struct {
	ID           string   `json:"id"`
	Slug         string   `json:"slug"`
	Title        string   `json:"title"`
	Team         string   `json:"team"`
	Organization string   `json:"organization"`
	Categories   []string `json:"categories"`
}

// Projects looks projects up, with their author: the organization when a
// project belongs to one, else the team's owner.
func (c *Client) Projects(ctx context.Context, ids []string) (map[string]modplatform.Project, error) {
	out := map[string]modplatform.Project{}
	var valid []string
	for _, id := range ids {
		if modplatform.ValidID(id) {
			valid = append(valid, id)
		}
	}
	var all []project
	for _, chunk := range modplatform.Chunk(valid, batch) {
		var resp []project
		if err := c.http.JSON(ctx, http.MethodGet, c.Base+"/v2/projects?ids="+jsonList(chunk), nil, nil, &resp); err != nil {
			return out, err
		}
		all = append(all, resp...)
	}
	authors := c.authors(ctx, all)
	for _, p := range all {
		if !modplatform.ValidID(p.ID) {
			continue
		}
		out[p.ID] = modplatform.Project{Source: modplatform.Modrinth, ID: p.ID, Slug: p.Slug, Title: p.Title,
			Author: authors[p.ID], Library: slices.Contains(p.Categories, "library")}
	}
	return out, nil
}

// authors resolves organizations and team owners; failures leave names empty.
func (c *Client) authors(ctx context.Context, ps []project) map[string]string {
	out := map[string]string{}
	var orgs, teams []string
	for _, p := range ps {
		if modplatform.ValidID(p.Organization) {
			orgs = append(orgs, p.Organization)
		} else if modplatform.ValidID(p.Team) {
			teams = append(teams, p.Team)
		}
	}
	orgName := map[string]string{}
	for _, chunk := range modplatform.Chunk(orgs, batch) {
		var resp []struct{ ID, Name string }
		if c.http.JSON(ctx, http.MethodGet, c.Base+"/v3/organizations?ids="+jsonList(chunk), nil, nil, &resp) == nil {
			for _, o := range resp {
				orgName[o.ID] = o.Name
			}
		}
	}
	teamOwner := map[string]string{}
	for _, chunk := range modplatform.Chunk(teams, batch) {
		var resp [][]struct {
			TeamID  string `json:"team_id"`
			Role    string `json:"role"`
			IsOwner bool   `json:"is_owner"`
			User    struct {
				Username string `json:"username"`
			} `json:"user"`
		}
		if c.http.JSON(ctx, http.MethodGet, c.Base+"/v2/teams?ids="+jsonList(chunk), nil, nil, &resp) != nil {
			continue
		}
		for _, members := range resp {
			for i, m := range members {
				if i == 0 || m.IsOwner || strings.EqualFold(m.Role, "owner") {
					teamOwner[m.TeamID] = m.User.Username
				}
				if m.IsOwner {
					break
				}
			}
		}
	}
	for _, p := range ps {
		if n := orgName[p.Organization]; n != "" {
			out[p.ID] = n
		} else {
			out[p.ID] = teamOwner[p.Team]
		}
	}
	return out
}
