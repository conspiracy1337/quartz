package curseforge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"quartz/internal/modplatform"
)

func TestSearchQuery(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		want := map[string]string{"gameId": "432", "classId": "6", "sortField": "6", "searchFilter": "sodium", "gameVersion": "1.21.1",
			"modLoaderTypes": "[5,4]", "categoryIds": "[423]", "index": "20", "pageSize": "20"}
		for k, v := range want {
			if q.Get(k) != v {
				t.Errorf("%s = %q, want %q", k, q.Get(k), v)
			}
		}
		_, _ = io.WriteString(w, `{"data":[{"id":394468,"name":"Sodium","slug":"sodium","summary":"Fast","downloadCount":4.12e7,
			"dateModified":"2025-02-01T00:00:00Z","logo":{"thumbnailUrl":"https://media.forgecdn.net/avatars/thumbnails/1/2/256/256/x.png"},
			"authors":[{"name":"jellysquid3_"}]},{"id":0}],"pagination":{"totalCount":31}}`)
	})
	page, err := c.Search(context.Background(), modplatform.SearchQuery{Type: modplatform.TypeMod, Text: "sodium", Sort: modplatform.SortDownloads,
		Game: "1.21.1", Loaders: []string{"quilt", "fabric", "datapack"}, Categories: []string{"423"}, Offset: 20, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 31 || len(page.Hits) != 1 {
		t.Fatalf("%+v", page)
	}
	h := page.Hits[0]
	if h.ID != "394468" || h.Author != "jellysquid3_" || h.Downloads != 41200000 || !strings.HasPrefix(h.Icon, "https://media.forgecdn.net/") {
		t.Fatalf("%+v", h)
	}
}

func TestFilesAnnotatesClassAndBlockedPage(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/mods/files":
			blocked := strings.Replace(file, `"downloadUrl":"https://edge.forgecdn.net/files/3509/43/stc.jar"`, `"downloadUrl":null`, 1)
			_, _ = io.WriteString(w, `{"data":[`+blocked+`]}`)
		case "/v1/mods":
			_, _ = io.WriteString(w, `{"data":[{"id":327154,"slug":"stc","classId":6,"links":{"websiteUrl":"https://www.curseforge.com/minecraft/mc-mods/stc"}}]}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	got, err := c.Files(context.Background(), []string{"3509043", "nope"})
	if err != nil {
		t.Fatal(err)
	}
	v := got["3509043"]
	if v.Class != modplatform.TypeMod || v.File.URL != "" || v.File.Page != "https://www.curseforge.com/minecraft/mc-mods/stc/download/3509043" {
		t.Fatalf("%+v", v)
	}
}

func TestFilesArePaged(t *testing.T) {
	var pages int
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		pages++
		index := r.URL.Query().Get("index")
		n := 50
		if index == "50" {
			n = 7
		}
		var items []string
		for i := range n {
			items = append(items, fmt.Sprintf(`{"id":%d,"modId":1,"releaseType":1,"fileDate":"2024-01-01T00:00:00Z"}`, i+1))
		}
		_, _ = fmt.Fprintf(w, `{"data":[%s],"pagination":{"totalCount":57}}`, strings.Join(items, ","))
	})
	vs, err := c.Versions(context.Background(), "1", modplatform.Query{})
	if err != nil || len(vs) != 57 || pages != 2 {
		t.Fatalf("got %d versions in %d pages: %v", len(vs), pages, err)
	}
}

func TestSlugAndPage(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("slug") != "all-the-mods-10" || r.URL.Query().Get("classId") != "4471" {
			t.Errorf("query %v", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"data":[{"id":925200,"slug":"all-the-mods-10"}]}`)
	})
	id, err := c.Slug(context.Background(), modplatform.TypeModpack, "all-the-mods-10")
	if err != nil || id != "925200" {
		t.Fatalf("%q %v", id, err)
	}
	if _, err := c.Slug(context.Background(), modplatform.TypeModpack, "../x"); err == nil {
		t.Fatal("bad slug accepted")
	}
	if PageOf(modplatform.TypeShader, "bsl") != "https://www.curseforge.com/minecraft/shaders/bsl" || PageOf(modplatform.TypeShader, "a/b") != "" {
		t.Fatal("page")
	}
}
