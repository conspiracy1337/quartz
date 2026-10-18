//go:build integration

package docs

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"quartz/internal/core/netx"
)

// TestMarkdownMatchesGoldmarkOnModrinth holds Markdown to goldmark on the
// descriptions of Modrinth's 300 most downloaded projects.
func TestMarkdownMatchesGoldmarkOnModrinth(t *testing.T) {
	get := func(u string, v any) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("User-Agent", netx.UserAgent)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %s", u, resp.Status)
		}
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			t.Fatal(err)
		}
	}
	checked := 0
	for offset := 0; offset < 300; offset += 100 {
		var search struct {
			Hits []struct {
				ID string `json:"project_id"`
			} `json:"hits"`
		}
		get("https://api.modrinth.com/v2/search?index=downloads&limit=100&offset="+strconv.Itoa(offset), &search)
		var ids []string
		for _, h := range search.Hits {
			ids = append(ids, h.ID)
		}
		list, _ := json.Marshal(ids)
		var projects []struct {
			Slug string `json:"slug"`
			Body string `json:"body"`
		}
		get("https://api.modrinth.com/v2/projects?ids="+url.QueryEscape(string(list)), &projects)
		for _, p := range projects {
			checked++
			if got, want, ok := matchesGoldmark(p.Body); !ok {
				t.Errorf("%s\n got %.300s\nwant %.300s", p.Slug, got, want)
			}
		}
	}
	if checked < 200 {
		t.Fatalf("only %d descriptions", checked)
	}
}
