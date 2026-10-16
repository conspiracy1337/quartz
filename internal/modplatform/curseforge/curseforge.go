// Package curseforge is the CurseForge provider. Its API needs a key
// (brand.CurseForgeKey); the HTTP client adds it for the API host only.
package curseforge

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"quartz/internal/modplatform"
)

// Host is the API host the key is sent to.
const Host = "api.curseforge.com"

// API is CurseForge's API base.
const API = "https://" + Host

const (
	batch    = 100
	pageSize = 50   // CurseForge's largest page
	maxFiles = 2000 // a project's file list is read up to this many
	// Requests per check that may run at once (one per project).
	parallel = 6
)

// Client is the CurseForge provider.
type Client struct {
	Base  string // overridden in tests
	http  modplatform.Doer
	ready func() bool
}

// New returns a client; ready reports whether an API key is configured.
func New(d modplatform.Doer, ready func() bool) *Client {
	return &Client{Base: API, http: d, ready: ready}
}

var _ modplatform.Provider = (*Client)(nil)

func (c *Client) Source() modplatform.Source { return modplatform.CurseForge }
func (c *Client) Ready() bool                { return c.ready() }

// Loader names and their modLoaderType numbers.
var loaderType = map[string]int{"forge": 1, "fabric": 4, "quilt": 5, "neoforge": 6}

var loaderName = map[string]string{"forge": "Forge", "fabric": "Fabric", "quilt": "Quilt", "neoforge": "NeoForge"}

type cfFile struct {
	ID           int64     `json:"id"`
	ModID        int64     `json:"modId"`
	DisplayName  string    `json:"displayName"`
	FileName     string    `json:"fileName"`
	ReleaseType  int       `json:"releaseType"`
	FileDate     time.Time `json:"fileDate"`
	FileLength   int64     `json:"fileLength"`
	Downloads    int64     `json:"downloadCount"`
	DownloadURL  string    `json:"downloadUrl"`
	GameVersions []string  `json:"gameVersions"`
	Fingerprint  uint32    `json:"fileFingerprint"`
	Hashes       []struct {
		Value string `json:"value"`
		Algo  int    `json:"algo"`
	} `json:"hashes"`
	Dependencies []struct {
		ModID        int64 `json:"modId"`
		RelationType int   `json:"relationType"`
	} `json:"dependencies"`
}

var releaseTypes = map[int]string{1: "release", 2: "beta", 3: "alpha"}

var relations = map[int]modplatform.DepKind{1: modplatform.DepEmbedded, 2: modplatform.DepOptional, 3: modplatform.DepRequired, 5: modplatform.DepIncompatible}

func (f cfFile) to() modplatform.Version {
	v := modplatform.Version{Source: modplatform.CurseForge, ProjectID: id(f.ModID), ID: id(f.ID), Number: f.DisplayName,
		Type: releaseTypes[f.ReleaseType], Published: f.FileDate, Downloads: f.Downloads,
		File: modplatform.File{Name: f.FileName, URL: f.DownloadURL, Size: f.FileLength}}
	for _, h := range f.Hashes {
		if h.Algo == 1 {
			v.File.SHA1 = strings.ToLower(h.Value)
		}
	}
	for _, gv := range f.GameVersions {
		if l, ok := loaderOf(gv); ok {
			v.Loaders = append(v.Loaders, l)
		} else if gv != "" && (gv[0] >= '0' && gv[0] <= '9') {
			v.Game = append(v.Game, gv)
		}
	}
	for _, d := range f.Dependencies {
		if k, ok := relations[d.RelationType]; ok && d.ModID > 0 {
			v.Deps = append(v.Deps, modplatform.Dep{ProjectID: id(d.ModID), Kind: k})
		}
	}
	return v
}

func loaderOf(gameVersion string) (string, bool) {
	for l, name := range loaderName {
		if strings.EqualFold(gameVersion, name) {
			return l, true
		}
	}
	return "", false
}

func id(n int64) string { return strconv.FormatInt(n, 10) }

func ids(in []string) []int64 {
	var out []int64
	for _, s := range in {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}

// Identify looks files up by their murmur2 fingerprint (POST /v1/fingerprints).
func (c *Client) Identify(ctx context.Context, files []modplatform.Hashes) (map[string]modplatform.Version, error) {
	out := map[string]modplatform.Version{}
	byPrint := map[uint32][]modplatform.Hashes{}
	var prints []uint32
	for _, f := range files {
		if _, dup := byPrint[f.Murmur2]; !dup {
			prints = append(prints, f.Murmur2)
		}
		byPrint[f.Murmur2] = append(byPrint[f.Murmur2], f)
	}
	for _, chunk := range modplatform.Chunk(prints, batch) {
		var resp struct {
			Data struct {
				ExactMatches []struct {
					File cfFile `json:"file"`
				} `json:"exactMatches"`
			} `json:"data"`
		}
		if err := c.http.JSON(ctx, http.MethodPost, c.Base+"/v1/fingerprints", nil, map[string]any{"fingerprints": chunk}, &resp); err != nil {
			return out, err
		}
		for _, m := range resp.Data.ExactMatches {
			v := m.File.to()
			for _, f := range byPrint[m.File.Fingerprint] {
				// A fingerprint ignores whitespace; the sha1 must agree too when known.
				if v.File.SHA1 != "" && f.SHA1 != "" && v.File.SHA1 != f.SHA1 {
					continue
				}
				out[f.SHA1] = v
			}
		}
	}
	return out, nil
}

// Projects looks mods up (POST /v1/mods).
func (c *Client) Projects(ctx context.Context, projectIDs []string) (map[string]modplatform.Project, error) {
	out := map[string]modplatform.Project{}
	for _, chunk := range modplatform.Chunk(ids(projectIDs), batch) {
		var resp struct {
			Data []mod `json:"data"`
		}
		if err := c.http.JSON(ctx, http.MethodPost, c.Base+"/v1/mods", nil, map[string]any{"modIds": chunk}, &resp); err != nil {
			return out, err
		}
		for _, m := range resp.Data {
			p := modplatform.Project{Source: modplatform.CurseForge, ID: id(m.ID), Slug: m.Slug, Title: m.Name, Author: m.author(),
				Icon: m.icon(), Summary: m.Summary}
			for _, cat := range m.Categories {
				if strings.Contains(cat.Slug, "library") || strings.Contains(cat.Slug, "api") {
					p.Library = true
				}
			}
			out[p.ID] = p
		}
	}
	return out, nil
}

// files lists a project's files for a game version and loader, newest
// first (GET /v1/mods/{id}/files, 50 per page), reading up to limit files.
func (c *Client) files(ctx context.Context, project, game, loader string, limit int) ([]modplatform.Version, error) {
	if _, err := strconv.ParseInt(project, 10, 64); err != nil {
		return nil, fmt.Errorf("curseforge: %q is no project id", project)
	}
	q := url.Values{"pageSize": {strconv.Itoa(pageSize)}}
	if game != "" {
		q.Set("gameVersion", game)
	}
	if t, ok := loaderType[loader]; ok {
		q.Set("modLoaderType", strconv.Itoa(t))
	}
	var out []modplatform.Version
	for index := 0; index < limit; index += pageSize {
		q.Set("index", strconv.Itoa(index))
		var resp struct {
			Data       []cfFile `json:"data"`
			Pagination struct {
				TotalCount int `json:"totalCount"`
			} `json:"pagination"`
		}
		if err := c.http.JSON(ctx, http.MethodGet, c.Base+"/v1/mods/"+project+"/files?"+q.Encode(), nil, nil, &resp); err != nil {
			return nil, err
		}
		for _, f := range resp.Data {
			out = append(out, f.to())
		}
		if len(resp.Data) < pageSize || index+pageSize >= resp.Pagination.TotalCount {
			break
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Published.After(out[b].Published) })
	return out, nil
}

// Latest picks, per project, the newest file for the game version, the
// loader and the channel (Prism's FlameAPI::getLatestVersion).
func (c *Client) Latest(ctx context.Context, items []modplatform.Installed, loader string, q modplatform.Query) (map[string]modplatform.Version, error) {
	game := ""
	if len(q.Game) > 0 {
		game = q.Game[0]
	}
	byProject := map[string][]string{}
	for _, it := range items {
		byProject[it.ProjectID] = append(byProject[it.ProjectID], it.Key)
	}
	out := map[string]modplatform.Version{}
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for project, keys := range byProject {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			vs, err := c.files(ctx, project, game, loader, pageSize)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			for _, v := range vs {
				if !q.Channel.Allows(v.Type) || (loader != "" && len(v.Loaders) > 0 && !contains(v.Loaders, loader)) {
					continue
				}
				for _, k := range keys {
					out[k] = v
				}
				return
			}
		})
	}
	wg.Wait()
	return out, firstErr
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Versions lists a project's files, newest first.
func (c *Client) Versions(ctx context.Context, project string, q modplatform.Query) ([]modplatform.Version, error) {
	game, loader := "", ""
	if len(q.Game) > 0 {
		game = q.Game[0]
	}
	if len(q.Loaders) > 0 {
		loader = q.Loaders[0]
	}
	return c.files(ctx, project, game, loader, maxFiles)
}

// Changelogs fetches each file's changelog (HTML), a few at a time.
func (c *Client) Changelogs(ctx context.Context, vs []modplatform.Version) error {
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for i := range vs {
		if _, err := strconv.ParseInt(vs[i].ProjectID, 10, 64); err != nil {
			continue
		}
		if _, err := strconv.ParseInt(vs[i].ID, 10, 64); err != nil {
			continue
		}
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			var resp struct {
				Data string `json:"data"`
			}
			err := c.http.JSON(ctx, http.MethodGet, c.Base+"/v1/mods/"+vs[i].ProjectID+"/files/"+vs[i].ID+"/changelog", nil, nil, &resp)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			vs[i].Changelog = resp.Data
		})
	}
	wg.Wait()
	return firstErr
}
