package modrinth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"quartz/internal/modplatform"
)

func TestSearchFacets(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var f [][]string
		if err := json.Unmarshal([]byte(q.Get("facets")), &f); err != nil {
			t.Fatal(err)
		}
		want := `[["project_type:mod"],["categories:quilt","categories:fabric"],["versions:1.21.1"],["categories:optimization"],["disclosure_types!=ai_content"]]`
		if b, _ := json.Marshal(f); string(b) != want {
			t.Errorf("facets %s", b)
		}
		if q.Get("index") != "updated" || q.Get("query") != "sodium" || q.Get("offset") != "20" || q.Get("limit") != "20" {
			t.Errorf("query %v", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"hits":[{"project_id":"AANobbMI","slug":"sodium","title":"Sodium","author":"jellysquid3",
			"description":"Fast","icon_url":"https://cdn.modrinth.com/data/AANobbMI/icon.png","downloads":58700000,
			"date_modified":"2025-02-03T00:00:00Z"},{"project_id":"../x"}],"total_hits":2}`)
	})
	page, err := c.Search(context.Background(), modplatform.SearchQuery{Type: modplatform.TypeMod, Text: "sodium", Sort: modplatform.SortUpdated,
		Game: "1.21.1", Loaders: []string{"quilt", "fabric"}, Categories: []string{"optimization"}, Exclude: []string{"ai_content"}, Offset: 20, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 || page.Hits[0].ID != "AANobbMI" || page.Hits[0].Downloads != 58700000 || page.Total != 2 {
		t.Fatalf("%+v", page)
	}
}

func TestProjectDetail(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/project/iris":
			_, _ = io.WriteString(w, `{"id":"YL57xq9U","slug":"iris","title":"Iris Shaders","team":"T1","project_type":"mod",
				"description":"Shaders","body":"# Iris","icon_url":"https://cdn.modrinth.com/i.webp","downloads":31600000,
				"updated":"2025-02-28T00:00:00Z","client_side":"required","server_side":"unsupported","loaders":["fabric","quilt"],
				"versions":["a","b","c"],"license":{"id":"LGPL-3.0-only","name":"GNU LGPL"},"source_url":"https://github.com/IrisShaders/Iris",
				"gallery":[{"url":"https://cdn.modrinth.com/g1.png","title":"Plain"},{"url":"https://cdn.modrinth.com/g2.png","title":"Best","featured":true}]}`)
		case "/v2/teams":
			_, _ = io.WriteString(w, `[[{"team_id":"T1","is_owner":true,"user":{"username":"coderbot"}}]]`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	d, err := c.Project(context.Background(), "iris")
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "YL57xq9U" || d.Author != "coderbot" || d.Versions != 3 || d.License != "LGPL-3.0-only" || d.Client != "required" ||
		d.Server != "unsupported" || d.Links.Page != "https://modrinth.com/mod/iris" || d.Gallery[0].Title != "Best" || d.BodyHTML {
		t.Fatalf("%+v", d)
	}
	if _, err := c.Project(context.Background(), "a/b"); err == nil {
		t.Fatal("slug with a slash accepted")
	}
}
