//go:build integration

package integration

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"quartz/internal/browse"
	"quartz/internal/brand"
	"quartz/internal/content"
	"quartz/internal/content/store"
	"quartz/internal/core/download"
	"quartz/internal/core/hashx"
	"quartz/internal/core/netx"
	"quartz/internal/core/remoteimg"
	"quartz/internal/instance"
	"quartz/internal/manual"
	"quartz/internal/modpack"
	"quartz/internal/modplatform"
	"quartz/internal/modplatform/curseforge"
	"quartz/internal/modplatform/modrinth"
)

// platforms returns Modrinth and, when QUARTZ_CF_API_KEY is set, CurseForge.
func (h *harness) platforms(t *testing.T) []modplatform.Platform {
	t.Helper()
	out := []modplatform.Platform{modrinth.New(h.http)}
	if key := brand.CurseForgeKey(); key != "" {
		c, err := netx.New(netx.Proxy{Mode: netx.ProxySystem}, netx.Options{Injectors: []netx.HeaderInjector{
			netx.HostHeader{Host: curseforge.Host, Name: "x-api-key", Value: func() string { return key }},
		}})
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, curseforge.New(c, func() bool { return true }))
	}
	return out
}

func TestBrowseSearchAndProject(t *testing.T) {
	h := newHarness(t)
	plats := h.platforms(t)
	svc := browse.New(browse.Deps{Platforms: plats, Pictures: remoteimg.New(filepath.Join(h.root, "cache", "img"), h.http, nil),
		Target: func(string) (browse.Target, error) { return browse.Target{Minecraft: "1.21.1", Loader: "fabric"}, nil }})
	res, err := svc.Search(h.ctx(), browse.Query{Type: "mod", Text: "sodium", Instance: "x", Version: true, Loader: true, HideAI: true})
	if err != nil {
		t.Fatal(err)
	}
	var sodium *browse.Row
	for i, r := range res.Rows {
		if r.Name == "Sodium" {
			sodium = &res.Rows[i]
		}
	}
	if sodium == nil || sodium.Modrinth == nil || !strings.HasPrefix(sodium.Icon, "/tex/remote/") {
		t.Fatalf("no Sodium row: %+v", res.Rows)
	}
	if len(plats) == 2 && sodium.CurseForge == nil {
		t.Errorf("Sodium was not merged with its CurseForge twin: %+v", sodium)
	}
	t.Logf("Sodium: %s, %d rows, more %v", sodium.Ref, len(res.Rows), res.More)
	if res.More {
		if _, err := svc.More(h.ctx(), res.SearchID); err != nil {
			t.Fatal(err)
		}
	}
	ref, _ := modplatform.ParseRef(sodium.Ref)
	page, err := svc.Project(h.ctx(), ref, "x")
	if err != nil || page.Modrinth == nil || len(page.Modrinth.Body) == 0 || page.Modrinth.Versions == 0 {
		t.Fatalf("project page: %+v %v", page, err)
	}
	vs, err := svc.Versions(h.ctx(), ref, modplatform.Modrinth, "x", true)
	if err != nil || len(vs) == 0 || vs[0].Action != "Install" {
		t.Fatalf("versions: %d %v", len(vs), err)
	}
	notes, err := svc.Changelog(h.ctx(), ref, modplatform.Modrinth, nil)
	if err != nil || len(notes) == 0 {
		t.Fatalf("changelog: %v", err)
	}
	t.Logf("Sodium %s: %d versions, %d body blocks, first changelog items %v", vs[0].Number, len(vs), len(page.Modrinth.Body), notes[0].Items)
}

// Modrinth accepts the content disclosure filters Browse sends.
func TestModrinthDisclosureFacet(t *testing.T) {
	h := newHarness(t)
	_, err := modrinth.New(h.http).Search(h.ctx(), modplatform.SearchQuery{Type: modplatform.TypeMod, Limit: 5,
		Exclude: []string{"ai_content", "advertisements", "telemetry"}})
	if err != nil {
		t.Fatalf("Modrinth refused disclosure filters: %v", err)
	}
}

// packEnv is a pack service over a fresh instance store.
func packEnv(t *testing.T, h *harness) (*modpack.Service, *modpack.Reader, *instance.Store) {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	insts, err := instance.Open(filepath.Join(h.root, "it-packs", "instances"), log)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(h.root, "store", "content"), h.ver, log)
	if err != nil {
		t.Fatal(err)
	}
	plats := h.platforms(t)
	providers := []modplatform.Provider{plats[0]}
	deps := modpack.Deps{Modrinth: plats[0], HTTP: h.http, Cache: filepath.Join(h.root, "cache", "imports"),
		Fetch: func(ctx context.Context, raw string, size int64, sum hashx.Sum, dest string) error {
			return h.dl.Fetch(ctx, download.Item{URLs: []string{raw}, Dest: dest, Size: size, Sum: sum}, nil)
		}}
	if len(plats) == 2 {
		providers = append(providers, plats[1])
		deps.CurseForge = plats[1]
	}
	reader := modpack.NewReader(deps)
	cs, err := content.New(content.Deps{Instances: insts, Store: st, Platforms: providers, Tasks: h.tasks, Log: log}, filepath.Join(h.root, "store", "content"))
	if err != nil {
		t.Fatal(err)
	}
	svc := modpack.NewService(modpack.InstallDeps{
		Reader: reader, Instances: insts, Store: st, Tasks: h.tasks, Loaders: h.loaders(),
		Manual: manual.New(manual.Deps{Store: st, Tasks: h.tasks}), GetIcon: modpack.GetIcon(h.http),
		Staging: filepath.Join(h.root, "it-packs", "instances", ".staging"), Log: log,
	}, modpack.UpdateDeps{Known: cs.Known, Record: cs.Record})
	return svc, reader, insts
}

// Fabulously Optimized: an older version installs from its Modrinth link,
// then the review of the newest one keeps a config the player edited.
func TestModrinthPackInstallAndUpdate(t *testing.T) {
	h := newHarness(t)
	svc, r, insts := packEnv(t, h)
	mr := modrinth.New(h.http)
	d, err := mr.Project(h.ctx(), "fabulously-optimized")
	if err != nil {
		t.Fatal(err)
	}
	vs, err := mr.Versions(h.ctx(), d.ID, modplatform.Query{})
	if err != nil || len(vs) < 3 {
		t.Fatalf("versions: %d %v", len(vs), err)
	}
	// The release before the newest release.
	var releases []modplatform.Version
	for _, v := range vs {
		if v.Type == "release" {
			releases = append(releases, v)
		}
	}
	if len(releases) < 2 {
		t.Fatal("fewer than two releases")
	}
	old := releases[1]
	p, err := r.Link(h.ctx(), "https://modrinth.com/modpack/fabulously-optimized/version/"+old.ID)
	if err != nil {
		t.Fatal(err)
	}
	st, err := p.Stats()
	if err != nil || p.Loader != "fabric" || st.Mods < 10 {
		t.Fatalf("pack %s %s: %+v %v", p.Name, p.Version, st, err)
	}
	t.Logf("%s %s for %s: %d mods, %d override files", p.Name, p.Version, p.Minecraft, st.Mods, st.Files)
	job, err := svc.Install(h.ctx(), svc.Stage(p), "FO old", instance.Art{Background: "#3ecf8e", Initial: "F"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.tasks.Wait(h.ctx(), job); err != nil {
		t.Fatal(err)
	}
	var in instance.Instance
	for _, i := range insts.List() {
		if i.Name == "FO old" {
			in = i
		}
	}
	if in.Pack == nil || in.Pack.Version != old.ID {
		t.Fatalf("instance %+v", in)
	}
	mods, _ := os.ReadDir(filepath.Join(insts.GameDir(in.ID), "mods"))
	if len(mods) < 10 {
		t.Fatalf("only %d files in mods/", len(mods))
	}
	// Edit a config the newest version changes, so the update must keep it.
	m, _ := modpack.ReadManifest(insts.Dir(in.ID))
	edited := ""
	for rel, f := range m.Files {
		if f.Layer && strings.HasPrefix(rel, "config/") {
			edited = rel
			_ = os.WriteFile(filepath.Join(insts.GameDir(in.ID), filepath.FromSlash(rel)), []byte("# edited\n"), 0o600)
			break
		}
	}
	if av := svc.CheckUpdates(h.ctx()); len(av) == 0 {
		t.Fatal("no update offered")
	}
	diff, err := svc.Diff(h.ctx(), in.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s to %s: %d rows, kept %v, game %q", diff.From, diff.To, len(diff.Rows), diff.KeptEdits, diff.Game)
	kinds := map[string]int{}
	for _, r := range diff.Rows {
		kinds[r.Kind]++
	}
	t.Logf("rows by kind %v, e.g. %+v", kinds, diff.Rows[min(len(diff.Rows)-1, kinds["added"])])
	if kinds["updated"] == 0 || kinds["updated"] < kinds["added"]+kinds["removed"] {
		t.Errorf("mods were not paired across versions: %v", kinds)
	}
	for _, k := range diff.KeptEdits {
		if k != edited {
			t.Errorf("kept %s, which the player didn't edit", k)
		}
	}
}

func TestFTBAndTechnicPacksParse(t *testing.T) {
	h := newHarness(t)
	_, r, _ := packEnv(t, h)
	var all struct {
		Packs []int64 `json:"packs"`
	}
	if err := h.http.JSON(h.ctx(), http.MethodGet, "https://api.feed-the-beast.com/v1/modpacks/public/modpack/all", nil, nil, &all); err != nil {
		t.Fatal(err)
	}
	read := 0
	for _, id := range all.Packs {
		if read == 2 {
			break
		}
		p, err := r.Link(h.ctx(), "https://www.feed-the-beast.com/modpacks/"+strconv.FormatInt(id, 10))
		if err != nil {
			t.Logf("FTB %d: %v", id, err)
			continue
		}
		read++
		t.Logf("FTB %d: %s %s, Minecraft %s, %s %s, %d files", id, p.Name, p.Version, p.Minecraft, p.Loader, p.LoaderVersion, len(p.Files))
	}
	if read == 0 {
		t.Error("no FTB pack could be read")
	}
	read = 0
	for _, slug := range []string{"tekkitmain", "tekkit", "hexxit", "attack-of-the-bteam"} {
		p, err := r.Link(h.ctx(), "https://www.technicpack.net/modpack/"+slug)
		if err != nil {
			t.Logf("Technic %s: %v", slug, err)
			continue
		}
		read++
		t.Logf("Technic %s: %s %s, Minecraft %s, %s %s, %d files", slug, p.Name, p.Version, p.Minecraft, p.Loader, p.LoaderVersion, len(p.Files))
	}
	if read == 0 {
		t.Error("no Technic pack could be read")
	}
}

func TestCurseForgePack(t *testing.T) {
	if brand.CurseForgeKey() == "" {
		t.Skip("QUARTZ_CF_API_KEY is not set")
	}
	h := newHarness(t)
	_, r, _ := packEnv(t, h)
	p, err := r.Link(h.ctx(), "https://www.curseforge.com/minecraft/modpacks/all-the-mods-10")
	if err != nil {
		t.Fatal(err)
	}
	st, _ := p.Stats()
	t.Logf("%s %s: %d mods, %d blocked", p.Name, p.Version, st.Mods, st.Blocked)
}
