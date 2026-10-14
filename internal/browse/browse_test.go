package browse

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"quartz/internal/content"
	"quartz/internal/core/netx"
	"quartz/internal/core/remoteimg"
	"quartz/internal/modplatform"
)

// fake is a platform with fixed hits, served in pages.
type fake struct {
	src      modplatform.Source
	hits     []modplatform.Hit
	details  map[string]modplatform.Detail
	versions map[string][]modplatform.Version
	cats     []modplatform.Category
	refuse   bool // answers 400 to disclosure filters
	down     bool // every search fails

	mu      sync.Mutex
	queries []modplatform.SearchQuery
}

func (f *fake) Source() modplatform.Source { return f.src }
func (f *fake) Ready() bool                { return true }
func (f *fake) Search(_ context.Context, q modplatform.SearchQuery) (modplatform.SearchPage, error) {
	f.mu.Lock()
	f.queries = append(f.queries, q)
	f.mu.Unlock()
	if f.down {
		return modplatform.SearchPage{}, &netx.StatusError{Code: 503}
	}
	if f.refuse && len(q.Exclude) > 0 {
		return modplatform.SearchPage{}, &netx.StatusError{Code: 400}
	}
	end := min(q.Offset+q.Limit, len(f.hits))
	var hits []modplatform.Hit
	for _, h := range f.hits[min(q.Offset, end):end] {
		if q.Text == "" || strings.Contains(strings.ToLower(h.Title), strings.ToLower(q.Text)) {
			hits = append(hits, h)
		}
	}
	return modplatform.SearchPage{Hits: hits, Total: len(f.hits)}, nil
}
func (f *fake) Project(_ context.Context, id string) (modplatform.Detail, error) {
	if d, ok := f.details[id]; ok {
		return d, nil
	}
	return modplatform.Detail{}, errors.New("no such project")
}
func (f *fake) Slug(context.Context, modplatform.ProjectType, string) (string, error) { return "", nil }
func (f *fake) Version(context.Context, string, string) (modplatform.Version, error) {
	return modplatform.Version{}, nil
}
func (f *fake) Files(context.Context, []string) (map[string]modplatform.Version, error) { return nil, nil }
func (f *fake) Categories(context.Context, modplatform.ProjectType) ([]modplatform.Category, error) {
	return f.cats, nil
}
func (f *fake) Identify(context.Context, []modplatform.Hashes) (map[string]modplatform.Version, error) {
	return nil, nil
}
func (f *fake) Projects(context.Context, []string) (map[string]modplatform.Project, error) {
	return nil, nil
}
func (f *fake) Latest(context.Context, []modplatform.Installed, string, modplatform.Query) (map[string]modplatform.Version, error) {
	return nil, nil
}
func (f *fake) Versions(_ context.Context, id string, _ modplatform.Query) ([]modplatform.Version, error) {
	return f.versions[id], nil
}
func (f *fake) Changelogs(_ context.Context, vs []modplatform.Version) error {
	for i := range vs {
		vs[i].Changelog = "- change in " + vs[i].Number
	}
	return nil
}

func hit(src modplatform.Source, id, title, author string, dl int64, daysAgo int) modplatform.Hit {
	return modplatform.Hit{Source: src, ID: id, Title: title, Author: author, Downloads: dl, Updated: time.Now().AddDate(0, 0, -daysAgo),
		Icon: "https://cdn.modrinth.com/data/" + id + "/icon.png"}
}

func newService(mr, cf *fake, tgt *Target) *Service {
	return New(Deps{
		Platforms: []modplatform.Platform{mr, cf},
		Pictures:  remoteimg.New("", nil, nil),
		Target: func(string) (Target, error) {
			if tgt == nil {
				return Target{}, errors.New("no instance")
			}
			return *tgt, nil
		},
	})
}

func TestSearchMergesTwins(t *testing.T) {
	mr := &fake{src: modplatform.Modrinth, hits: []modplatform.Hit{
		hit(modplatform.Modrinth, "AANobbMI", "Sodium", "jellysquid3", 58_700_000, 5),
		hit(modplatform.Modrinth, "uXXizFIs", "FerriteCore", "malte0811", 41_000_000, 14),
	}}
	cf := &fake{src: modplatform.CurseForge, hits: []modplatform.Hit{
		hit(modplatform.CurseForge, "394468", "Sodium", "jellysquid3_", 41_200_000, 12),
		hit(modplatform.CurseForge, "263420", "Xaero's Minimap", "xaero96", 88_100_000, 6),
	}}
	s := newService(mr, cf, nil)
	res, err := s.Search(context.Background(), Query{Type: "mod", Text: ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 3 || res.More {
		t.Fatalf("rows %+v more %v", res.Rows, res.More)
	}
	sodium := res.Rows[0]
	if sodium.Ref != "mr:AANobbMI+cf:394468" || sodium.ID != "mr:AANobbMI" || sodium.Modrinth == nil || sodium.CurseForge == nil ||
		sodium.CurseForge.Downloads != 41_200_000 || !strings.HasPrefix(sodium.Icon, "/tex/remote/") {
		t.Fatalf("sodium %+v", sodium)
	}
	if res.Rows[1].Name != "FerriteCore" || res.Rows[2].Name != "Xaero's Minimap" {
		t.Fatalf("relevance order: %s, %s", res.Rows[1].Name, res.Rows[2].Name)
	}
}

func TestMoreMergesIntoShownRowsWithoutMovingThem(t *testing.T) {
	var mrHits, cfHits []modplatform.Hit
	for i := range pageSize {
		mrHits = append(mrHits, hit(modplatform.Modrinth, "mr"+string(rune('a'+i)), "Mod "+string(rune('A'+i)), "dev", 100, 1))
	}
	for i := range pageSize {
		cfHits = append(cfHits, hit(modplatform.CurseForge, "9"+string(rune('0'+i%10))+string(rune('0'+i/10)), "Other "+string(rune('A'+i)), "x", 5, 1))
	}
	// The twin of Modrinth's first hit comes on CurseForge's second page.
	cfHits = append(cfHits, hit(modplatform.CurseForge, "777", "Mod A", "dev", 50, 1))
	mr := &fake{src: modplatform.Modrinth, hits: mrHits}
	cf := &fake{src: modplatform.CurseForge, hits: cfHits}
	s := newService(mr, cf, nil)
	first, err := s.Search(context.Background(), Query{Type: "mod"})
	if err != nil || len(first.Rows) != 2*pageSize || !first.More {
		t.Fatalf("first page: %d rows, more %v, %v", len(first.Rows), first.More, err)
	}
	next, err := s.More(context.Background(), first.SearchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Rows) != 0 || len(next.Changed) != 1 || next.Changed[0].ID != first.Rows[0].ID || next.Changed[0].Ref != "mr:mra+cf:777" || next.More {
		t.Fatalf("second page %+v", next)
	}
	if _, err := s.More(context.Background(), "nope"); !errors.Is(err, ErrUnknownSearch) {
		t.Fatal(err)
	}
}

func TestSearchFiltersFromTheTarget(t *testing.T) {
	mr := &fake{src: modplatform.Modrinth, refuse: true, hits: []modplatform.Hit{
		hit(modplatform.Modrinth, "AANobbMI", "Sodium", "jellysquid3", 1, 1),
		hit(modplatform.Modrinth, "YL57xq9U", "Iris Shaders", "coderbot", 1, 1),
	}}
	cf := &fake{src: modplatform.CurseForge, cats: []modplatform.Category{{ID: "423", Slug: "performance"}}}
	tgt := &Target{Minecraft: "1.21.1", Loader: "quilt", Installed: []content.Installed{{Modrinth: &content.Ident{ProjectID: "AANobbMI"}}}}
	s := newService(mr, cf, tgt)
	res, err := s.Search(context.Background(), Query{Type: "mod", Instance: "x", Version: true, Loader: true, HideInstalled: true, HideAI: true,
		Categories: []string{"Optimization"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || res.Rows[0].Name != "Iris Shaders" {
		t.Fatalf("installed project shown: %+v", res.Rows)
	}
	q := mr.queries[0]
	if q.Game != "1.21.1" || !slices.Equal(q.Loaders, []string{"quilt", "fabric"}) || len(q.Categories) != 1 || q.Categories[0][0] != "optimization" || len(q.Exclude) == 0 {
		t.Fatalf("modrinth query %+v", q)
	}
	if last := mr.queries[len(mr.queries)-1]; len(last.Exclude) != 0 || !s.noDisclose.Load() {
		t.Fatal("refused disclosure filters were not dropped")
	}
	if cq := cf.queries[0]; len(cq.Categories) != 1 || cq.Categories[0][0] != "423" {
		t.Fatalf("curseforge query %+v", cq)
	}
	// A category CurseForge lacks leaves CurseForge out.
	before := len(cf.queries)
	if _, err := s.Search(context.Background(), Query{Type: "shader", Categories: []string{"Cartoon"}}); err != nil {
		t.Fatal(err)
	}
	if len(cf.queries) != before {
		t.Fatal("CurseForge asked for a category it can't filter")
	}
}

func TestProjectPage(t *testing.T) {
	now := time.Now()
	mr := &fake{src: modplatform.Modrinth,
		hits: []modplatform.Hit{},
		details: map[string]modplatform.Detail{"YL57xq9U": {
			Project: modplatform.Project{Source: modplatform.Modrinth, ID: "YL57xq9U", Title: "Iris Shaders", Author: "coderbot"},
			Type:    modplatform.TypeMod, Body: "Needs `Sodium`.\n\n<script>x</script>", Updated: now.AddDate(0, 0, -5), Loaders: []string{"fabric", "quilt"},
			Client: "required", Server: "unsupported", License: "LGPL-3.0-only", Distributable: true,
			Gallery: []modplatform.Image{{URL: "https://cdn.modrinth.com/g.png", Title: "Sunset"}, {URL: "https://evil.example/x.png"}},
			Links:   modplatform.Links{Page: "https://modrinth.com/mod/iris", Source: "https://github.com/IrisShaders/Iris", Issues: "javascript:alert(1)"},
		}},
		versions: map[string][]modplatform.Version{"YL57xq9U": {
			{ID: "v3", Number: "1.8.3", Type: "release", Game: []string{"1.21.4"}, Loaders: []string{"fabric"}, Published: now.AddDate(0, 0, -5)},
			{ID: "v1", Number: "1.8.1", Type: "release", Game: []string{"1.21.4"}, Loaders: []string{"fabric"}, Published: now.AddDate(0, 0, -60)},
			{ID: "v0", Number: "1.7.6", Type: "beta", Game: []string{"1.21.1"}, Loaders: []string{"fabric"}, Published: now.AddDate(0, 0, -90)},
		}},
	}
	cf := &fake{src: modplatform.CurseForge,
		hits: []modplatform.Hit{hit(modplatform.CurseForge, "455508", "Iris Shaders", "coderbot", 1, 12)},
		details: map[string]modplatform.Detail{"455508": {
			Project: modplatform.Project{Source: modplatform.CurseForge, ID: "455508", Title: "Iris Shaders", Author: "coderbot"},
			Type:    modplatform.TypeMod, Body: "<p>Shaders</p>", BodyHTML: true, Updated: now.AddDate(0, 0, -12),
		}},
	}
	tgt := &Target{Minecraft: "1.21.4", Loader: "fabric", Installed: []content.Installed{{Item: "abc", Modrinth: &content.Ident{
		ProjectID: "YL57xq9U", VersionID: "v1", Version: "1.8.1", Published: now.AddDate(0, 0, -60).Unix()}}}}
	s := newService(mr, cf, tgt)
	ref, _ := modplatform.ParseRef("mr:YL57xq9U")
	p, err := s.Project(context.Background(), ref, "x")
	if err != nil {
		t.Fatal(err)
	}
	if p.Ref != "mr:YL57xq9U+cf:455508" || p.Fresher != "modrinth" || p.CurseForge == nil || p.Installed == nil || p.Installed.VersionID != "v1" {
		t.Fatalf("page %+v", p)
	}
	m := p.Modrinth
	if !slices.Equal(m.Badges, []string{"Client", "Fabric", "Quilt", "LGPL-3.0-only"}) || len(m.Gallery) != 1 || len(m.Links) != 1 ||
		len(m.Body) != 1 || m.Body[0].Spans[1].Code != true {
		t.Fatalf("modrinth side %+v", m)
	}
	vs, err := s.Versions(context.Background(), modplatform.Ref{Modrinth: "YL57xq9U"}, modplatform.Modrinth, "x", true)
	if err != nil || len(vs) != 2 || vs[0].Action != "Update" || vs[1].Action != "" {
		t.Fatalf("compatible versions %+v %v", vs, err)
	}
	all, _ := s.Versions(context.Background(), modplatform.Ref{Modrinth: "YL57xq9U"}, modplatform.Modrinth, "x", false)
	if len(all) != 3 || all[2].Action != "Switch" || all[2].Channel != "Beta" {
		t.Fatalf("all versions %+v", all)
	}
	notes, err := s.Changelog(context.Background(), modplatform.Ref{Modrinth: "YL57xq9U"}, modplatform.Modrinth, []string{"v1"})
	if err != nil || len(notes) != 1 || notes[0].Items[0] != "change in 1.8.1" {
		t.Fatalf("notes %+v %v", notes, err)
	}
}

func TestMatchKey(t *testing.T) {
	if matchKey("Xaero’s Minimap", "xaero96") != matchKey("Xaeros Minimap", "Xaero96") {
		t.Fatal("punctuation and case should not matter")
	}
	if matchKey("Sodium", "jellysquid3") == matchKey("Sodium", "someone") {
		t.Fatal("different authors matched")
	}
	if matchKey("!!!", "x") != "" {
		t.Fatal("empty title matched")
	}
}

func TestModpackSearchIgnoresTarget(t *testing.T) {
	mr := &fake{src: modplatform.Modrinth}
	cf := &fake{src: modplatform.CurseForge}
	s := newService(mr, cf, &Target{Minecraft: "1.20.1", Loader: "fabric"})
	if _, err := s.Search(context.Background(), Query{Type: "modpack", Instance: "x", Version: true, Loader: true}); err != nil {
		t.Fatal(err)
	}
	if q := mr.queries[0]; q.Game != "" || len(q.Loaders) != 0 {
		t.Fatalf("a pack search took the instance's filters: %+v", q)
	}
	if _, err := s.Search(context.Background(), Query{Type: "modpack", Instance: "x", Version: true, Game: "1.21.1"}); err != nil {
		t.Fatal(err)
	}
	if q := mr.queries[1]; q.Game != "1.21.1" {
		t.Fatalf("picked version not used: %+v", q)
	}
	if _, err := s.Search(context.Background(), Query{Type: "modpack", Game: "1.21&x=1"}); err != nil {
		t.Fatal(err)
	}
	if q := mr.queries[2]; q.Game != "" {
		t.Fatalf("bad version passed through: %+v", q)
	}
}

func TestSearchNamesFailedPlatformsAndMarksInstalled(t *testing.T) {
	mr := &fake{src: modplatform.Modrinth, hits: []modplatform.Hit{
		hit(modplatform.Modrinth, "AANobbMI", "Sodium", "jellysquid3", 58_700_000, 5),
		hit(modplatform.Modrinth, "uXXizFIs", "FerriteCore", "malte0811", 41_000_000, 14),
	}}
	cf := &fake{src: modplatform.CurseForge, down: true}
	tgt := &Target{Minecraft: "1.21.1", Loader: "fabric", Installed: []content.Installed{{Modrinth: &content.Ident{ProjectID: "AANobbMI"}}}}
	s := newService(mr, cf, tgt)
	res, err := s.Search(context.Background(), Query{Type: "mod", Instance: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Failed, []string{"curseforge"}) || len(res.Rows) != 2 || !res.Rows[0].Installed || res.Rows[1].Installed {
		t.Fatalf("failed %v rows %+v", res.Failed, res.Rows)
	}
	res, err = s.Search(context.Background(), Query{Type: "mod", Instance: "x", HideInstalled: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || res.Rows[0].Name != "FerriteCore" {
		t.Fatalf("rows %+v", res.Rows)
	}
	cf.down, cf.queries = false, nil
	if _, err := s.Search(context.Background(), Query{Type: "mod", Side: "client"}); err != nil {
		t.Fatal(err)
	}
	if n := len(cf.queries); n != 0 {
		t.Fatalf("CurseForge asked %d times for a side it can't filter", n)
	}
}
