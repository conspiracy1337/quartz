package norisk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"quartz/internal/modplatform"
)

// plain is a Doer over a test server.
type plain struct{ c *http.Client }

func (p plain) Do(req *http.Request) (*http.Response, error) { return p.c.Do(req) }

func (p plain) JSON(ctx context.Context, method, u string, _ http.Header, _, out any) error {
	req, _ := http.NewRequestWithContext(ctx, method, u, nil)
	resp, err := p.c.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return json.NewDecoder(resp.Body).Decode(out)
}

const miniConfig = `{"displayName":"NRC Mini",
 "mods":[{"id":"nrc-mini","source":{"type":"maven","repositoryRef":"noriskproduction","groupId":"gg.norisk","artifactId":"nrc-mini"},
  "compatibility":{
   "1.8.9":{"forge":{"identifier":"26.4.1-working+forge.1.8.9"}},
   "1.21.4":{"fabric":{"identifier":"26.4.1342909-working+fabric.1.21.4"},"neoforge":{"identifier":"26.4.1330783-working+neoforge.1.21.4"}},
   "26.2":{"fabric":{"identifier":"26.4.1342909-working+fabric.26.2"},"bukkit":{"identifier":"x"}},
   "1.20.1":{"fabric":{"identifier":"../../../evil"}}
  }},
  {"id":"other","source":{"type":"maven","groupId":"gg.norisk","artifactId":"something-else"},"compatibility":{"1.21.4":{"fabric":{"identifier":"1"}}}}]}`

// frontPage is the part of norisk.gg's front page that holds the count.
const frontPage = `<span class="HeroStats_count__7pNhE">41,967</span>players in the last 24 hours.</span>` +
	`<script>self.__next_f.push([1,"2c:[\"$\",\"$L3f\",null,{\"locale\":\"en\",\"players\":41967,\"modules\":80}]"])</script>`

func setup(t *testing.T) (*Client, *atomic.Int32) {
	t.Helper()
	var heads atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/site/":
			_, _ = io.WriteString(w, frontPage)
		case r.URL.Path == "/packs/nrc-mini.json":
			_, _ = io.WriteString(w, miniConfig)
		case strings.HasSuffix(r.URL.Path, ".jar.sha1"):
			_, _ = io.WriteString(w, "4A53AC23A6D54B7066B7662B31C3115461E9EFEB  nrc-mini.jar\n")
		case strings.HasSuffix(r.URL.Path, ".jar") && r.Method == http.MethodHead:
			heads.Add(1)
			w.Header().Set("Content-Length", "39426660")
			w.Header().Set("Last-Modified", "Fri, 16 Oct 2026 13:14:10 GMT")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c := New(plain{srv.Client()})
	c.ConfigBase, c.MavenBase, c.SiteBase = srv.URL+"/packs/", srv.URL+"/maven/", srv.URL+"/site/"
	return c, &heads
}

func TestParseKeepsOnlyOwnSafeBuilds(t *testing.T) {
	c, _ := setup(t)
	vs, err := c.Versions(context.Background(), "nrcmini", modplatform.Query{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range vs {
		got = append(got, v.Game[0]+"/"+v.Loaders[0])
	}
	// Newest game first, fabric before neoforge; no bukkit, no path tricks, no other artifact.
	if strings.Join(got, " ") != "26.2/fabric 1.21.4/fabric 1.21.4/neoforge 1.8.9/forge" {
		t.Fatalf("builds %v", got)
	}
	v := vs[1]
	if v.Number != "26.4.1342909" || !modplatform.ValidID(v.ID) || v.File.Size != 39426660 || v.Published.IsZero() || v.File.SHA1 != "" {
		t.Fatalf("version %+v", v)
	}
	if want := c.MavenBase + "nrc-mini/26.4.1342909-working+fabric.1.21.4/nrc-mini-26.4.1342909-working+fabric.1.21.4.jar"; v.File.URL != want {
		t.Fatalf("url %s", v.File.URL)
	}
}

func TestInstallableVersion(t *testing.T) {
	c, heads := setup(t)
	ctx := context.Background()
	vs, err := c.Versions(ctx, "nrcmini", modplatform.Query{Game: []string{"1.21.4"}, Loaders: []string{"quilt", "fabric"}})
	if err != nil || len(vs) != 1 || vs[0].File.SHA1 != "4a53ac23a6d54b7066b7662b31c3115461e9efeb" {
		t.Fatalf("versions %+v %v", vs, err)
	}
	v, err := c.Version(ctx, "nrcmini", vs[0].ID)
	if err != nil || v.File.SHA1 == "" || v.File.Size == 0 {
		t.Fatalf("version %+v %v", v, err)
	}
	if n := heads.Load(); n != 1 {
		t.Fatalf("asked the repository %d times about one jar", n)
	}
	latest, err := c.Latest(ctx, []modplatform.Installed{{Key: "a", ProjectID: "nrcmini"}}, "neoforge", modplatform.Query{Game: []string{"1.21.4"}})
	if err != nil || latest["a"].Loaders[0] != "neoforge" {
		t.Fatalf("latest %+v %v", latest, err)
	}
	if _, err := c.Version(ctx, "nrcmini", "0000000000000000"); err == nil {
		t.Fatal("found an unknown build")
	}
}

func TestSearch(t *testing.T) {
	c, _ := setup(t)
	ctx := context.Background()
	for text, want := range map[string]string{
		"NRC Mini": "nrcmini", "norisk mini": "nrcmini", "NoRiskClient Mod Mini": "nrcmini",
		"no": "", "sodium": "", "NoRisk Mod": "nrcstandalone",
	} {
		q := modplatform.SearchQuery{Type: modplatform.TypeMod, Text: text, Game: "1.21.4", Loaders: []string{"fabric"}}
		if want == "nrcstandalone" {
			if !matches(&mods[0], text) || matches(&mods[1], text) {
				t.Errorf("%q matches the wrong mods", text)
			}
			continue
		}
		page, err := c.Search(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if want == "" && len(page.Hits) != 0 || want != "" && (len(page.Hits) != 1 || page.Hits[0].ID != want || page.Hits[0].Updated.IsZero() || page.Hits[0].Daily != 41967) {
			t.Errorf("%q: %+v", text, page.Hits)
		}
	}
	// "norisk" names both, but only Mini has a config here, and only for these games.
	page, _ := c.Search(ctx, modplatform.SearchQuery{Type: modplatform.TypeMod, Text: "norisk", Game: "1.12.2"})
	if len(page.Hits) != 0 {
		t.Fatalf("a build for 1.12.2: %+v", page.Hits)
	}
	if page, _ := c.Search(ctx, modplatform.SearchQuery{Type: modplatform.TypeShader, Text: "NRC Mini"}); len(page.Hits) != 0 {
		t.Fatal("found a mod among shaders")
	}
}

func TestProjectPage(t *testing.T) {
	c, _ := setup(t)
	d, err := c.Project(context.Background(), "nrcmini")
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "NoRiskClient Mod Mini" || d.Author != "NoRisk" || !strings.Contains(d.Body, "The social side of NoRiskClient") || d.Versions != 4 || d.Daily != 41967 {
		t.Fatalf("detail %+v", d)
	}
	// Mini's page shows what Mini has: the cosmetics, never the module menu.
	// The page draws the gallery's first picture, so the body holds none.
	if len(d.Gallery) != 1 || d.Gallery[0] != cosmetics || strings.Contains(d.Body, "](") {
		t.Fatalf("mini's pictures: %+v", d.Gallery)
	}
	for _, m := range mods {
		if !strings.HasSuffix(m.body, "\n") || strings.Contains(m.body, "Good to know") || len(m.gallery) == 0 {
			t.Errorf("%s: body or gallery", m.id)
		}
		for _, img := range append([]modplatform.Image{{URL: icon}}, m.gallery...) {
			if !strings.HasPrefix(img.URL, Site+"/images/") && img.URL != icon {
				t.Errorf("%s: picture %s is not on norisk.gg", m.id, img.URL)
			}
		}
	}
}

func TestDailyPlayers(t *testing.T) {
	var reads atomic.Int32
	page := frontPage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		_, _ = io.WriteString(w, page)
	}))
	t.Cleanup(srv.Close)
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	c := New(plain{srv.Client()})
	c.SiteBase, c.Now = srv.URL+"/", func() time.Time { return now }
	ctx := context.Background()
	if n := c.dailyPlayers(ctx); n != 41967 {
		t.Fatalf("players %d", n)
	}
	page = `"players":"lots"`
	now = now.Add(59 * time.Minute)
	if n := c.dailyPlayers(ctx); n != 41967 || reads.Load() != 1 {
		t.Fatalf("within the hour: %d after %d reads", n, reads.Load())
	}
	// An unreadable page is 0, and asked again only after playersRetry.
	now = now.Add(2 * time.Minute)
	if n := c.dailyPlayers(ctx); n != 0 || reads.Load() != 2 {
		t.Fatalf("unreadable: %d after %d reads", n, reads.Load())
	}
	page = `{"players":1234567890123}`
	now = now.Add(time.Minute)
	if n := c.dailyPlayers(ctx); n != 0 || reads.Load() != 2 {
		t.Fatalf("retried too soon: %d after %d reads", n, reads.Load())
	}
	now = now.Add(playersRetry)
	if n := c.dailyPlayers(ctx); n != 0 || reads.Load() != 3 {
		t.Fatalf("a 13-digit count: %d after %d reads", n, reads.Load())
	}
}

func TestDownloadsAreAllowed(t *testing.T) {
	c := New(plain{http.DefaultClient})
	u := c.jarURL(&mods[1], "26.4.1342909-working+fabric.26.2")
	if !modplatform.ValidURL(u) {
		t.Fatalf("%s is not a valid download", u)
	}
	if modplatform.ValidURL("https://maven.norisk.gg/repository/norisk-production/gg/norisk/../../other/x.jar") {
		t.Fatal("a path out of gg.norisk is valid")
	}
	r, err := modplatform.ParseRef("nr:nrcmini")
	if err != nil || r.NoRisk != "nrcmini" || r.String() != "nr:nrcmini" || r.Sources()[0] != modplatform.NoRisk {
		t.Fatalf("ref %+v %v", r, err)
	}
	if _, err := modplatform.ParseRef("nr:a+nr:b"); err == nil {
		t.Fatal("two NoRisk ids parsed")
	}
}
