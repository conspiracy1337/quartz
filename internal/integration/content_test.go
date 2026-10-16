//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"quartz/internal/content"
	"quartz/internal/content/meta"
	"quartz/internal/content/packwiz"
	"quartz/internal/content/store"
	"quartz/internal/core/download"
	"quartz/internal/core/hashx"
	"quartz/internal/icons"
	"quartz/internal/instance"
	"quartz/internal/modplatform"
	"quartz/internal/modplatform/modrinth"
	"quartz/internal/modplatform/norisk"
)

// fetch downloads one platform file into dir and returns its path.
func (h *harness) fetch(t *testing.T, dir string, f modplatform.File) string {
	t.Helper()
	dst := filepath.Join(dir, f.Name)
	sum := hashx.Of(hashx.SHA512, f.SHA512)
	if f.SHA512 == "" {
		sum = hashx.Of(hashx.SHA1, f.SHA1)
	}
	if err := h.dl.Fetch(h.ctx(), download.Item{URLs: []string{f.URL}, Dest: dst, Size: f.Size, Sum: sum}, nil); err != nil {
		t.Fatal(err)
	}
	return dst
}

// versions lists a Modrinth project's Fabric builds for 1.21.1, newest first.
func versions(t *testing.T, h *harness, mr *modrinth.Client, project string) []modplatform.Version {
	t.Helper()
	vs, err := mr.Versions(h.ctx(), project, modplatform.Query{Game: []string{"1.21.1"}, Loaders: []string{"fabric"}})
	if err != nil || len(vs) < 2 {
		t.Fatalf("%s versions: %d, %v", project, len(vs), err)
	}
	return vs
}

func TestModrinthIdentifyAndUpdate(t *testing.T) {
	h := newHarness(t)
	mr := modrinth.New(h.http)
	vs := versions(t, h, mr, "gvQqBUqZ") // Lithium
	old := vs[len(vs)-1]
	p := h.fetch(t, filepath.Join(h.root, "it-content"), old.File)
	m, err := hashx.MultiFile(p)
	if err != nil || m.SHA1 != old.File.SHA1 || m.SHA512 != old.File.SHA512 {
		t.Fatalf("hashes differ from the published ones: %+v", m)
	}
	got, err := mr.Identify(h.ctx(), []modplatform.Hashes{{SHA1: m.SHA1, SHA512: m.SHA512}})
	if err != nil || got[m.SHA1].ID != old.ID || got[m.SHA1].ProjectID != "gvQqBUqZ" {
		t.Fatalf("identify: %+v %v", got, err)
	}
	ps, err := mr.Projects(h.ctx(), []string{"gvQqBUqZ"})
	if err != nil || ps["gvQqBUqZ"].Author == "" || ps["gvQqBUqZ"].Slug != "lithium" {
		t.Fatalf("project: %+v %v", ps, err)
	}
	up, err := modplatform.Check(h.ctx(), mr, []modplatform.Installed{{Key: "k", ProjectID: old.ProjectID, Current: got[m.SHA1],
		Hashes: modplatform.Hashes{SHA1: m.SHA1, SHA512: m.SHA512}}}, modplatform.Query{Game: []string{"1.21.1"}, Loaders: []string{"fabric"}})
	if err != nil || up["k"].ID == "" || up["k"].ID == old.ID {
		t.Fatalf("no update found for %s: %+v %v", old.Number, up, err)
	}
	notes := []modplatform.Version{up["k"]}
	if err := mr.Changelogs(h.ctx(), notes); err != nil || len(modplatform.Items(notes[0].Changelog, false)) == 0 {
		t.Fatalf("changelog: %q %v", notes[0].Changelog, err)
	}
	t.Logf("Lithium %s → %s by %s", old.Number, up["k"].Number, ps["gvQqBUqZ"].Author)
}

func TestCurseForgeFingerprintOfARealFile(t *testing.T) {
	h := newHarness(t)
	// Prism's packwiz fixture names this file with its fingerprint; no API key is needed.
	f := modplatform.File{Name: "screenshot-to-clipboard-1.0.7-fabric.jar", URL: "https://edge.forgecdn.net/files/3509/43/screenshot-to-clipboard-1.0.7-fabric.jar",
		SHA1: "760b3c0be40a3860bee1860bbee92cf25109e484", Size: 15066}
	m, err := hashx.MultiFile(h.fetch(t, filepath.Join(h.root, "it-content"), f))
	if err != nil || m.Murmur2 != 1781245820 {
		t.Fatalf("murmur2 = %d, %v", m.Murmur2, err)
	}
}

func TestClientPackFormats(t *testing.T) {
	h := newHarness(t)
	man, err := h.meta.Manifest(h.ctx())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"1.21.1", man.Latest.Release} {
		v, err := h.meta.Version(h.ctx(), id)
		if err != nil {
			t.Fatal(err)
		}
		c := v.Downloads["client"]
		jar := filepath.Join(h.root, "it-content", "client-"+id+".jar")
		if err := h.dl.Fetch(h.ctx(), download.Item{URLs: []string{c.URL}, Dest: jar, Size: c.Size, Sum: hashx.Of(hashx.SHA1, c.SHA1)}, nil); err != nil {
			t.Fatal(err)
		}
		f, ok := meta.ClientPack(jar)
		if !ok || f.Resource[0] == 0 || !f.HasData || f.WorldVersion == 0 {
			t.Fatalf("%s: %+v %v", id, f, ok)
		}
		t.Logf("%s: resource %v, data %v, world %d", id, f.Resource, f.Data, f.WorldVersion)
	}
}

// TestContentServiceWithRealMods scans an instance holding real mods: every
// file is adopted into the store, identified on Modrinth and given packwiz
// metadata, an update check finds the newer builds, and nothing is unused.
func TestContentServiceWithRealMods(t *testing.T) {
	h := newHarness(t)
	mr := modrinth.New(h.http)
	insts, err := instance.Open(filepath.Join(h.root, "it-instances"), h.log)
	if err != nil {
		t.Fatal(err)
	}
	in, err := insts.Create(instance.CreateRequest{Name: "Content IT", Minecraft: "1.21.1", Loader: "fabric", LoaderVersion: "0.16.9"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(insts.Dir(in.ID)) })
	mods := filepath.Join(insts.GameDir(in.ID), "mods")
	for _, project := range []string{"gvQqBUqZ", "AANobbMI", "P7dR8mSH"} { // Lithium, Sodium, Fabric API
		vs := versions(t, h, mr, project)
		h.fetch(t, mods, vs[len(vs)-1].File)
	}
	st, err := store.Open(filepath.Join(h.root, "store", "content"), h.ver, h.log)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := content.New(content.Deps{Instances: insts, Store: st, Icons: icons.OpenSized(filepath.Join(h.root, "cache", "content-icons"), 64),
		Platforms: []modplatform.Provider{mr}, Tasks: h.tasks, Log: h.log}, filepath.Join(h.root, "store", "content"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := h.ctx()
	if _, err := svc.List(ctx, in.ID, content.Mods); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.Identify(ctx, in.ID, content.Mods); err != nil || n != 3 {
		t.Fatalf("identified %d: %v", n, err)
	}
	rows, err := svc.List(ctx, in.ID, content.Mods)
	if err != nil || len(rows) != 3 {
		t.Fatalf("%+v %v", rows, err)
	}
	for _, r := range rows {
		if r.Source != "modrinth" || r.Sub == "" || r.Project == "" {
			t.Errorf("not identified: %+v", r)
		}
	}
	entries, _ := os.ReadDir(mods)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m, _ := hashx.MultiFile(filepath.Join(mods, e.Name()))
		a, _ := os.Stat(filepath.Join(mods, e.Name()))
		b, err := os.Stat(st.Path(m.SHA1))
		if err != nil || !os.SameFile(a, b) {
			t.Errorf("%s was not adopted into the store", e.Name())
		}
	}
	if pws, _ := packwiz.ReadDir(filepath.Join(mods, ".index")); len(pws) != 3 {
		t.Errorf("packwiz metadata for %d mods", len(pws))
	}
	rev, err := svc.CheckUpdates(ctx, in.ID, content.Mods, modplatform.Release)
	if err != nil || len(rev.Updates) == 0 {
		t.Fatalf("no updates for old builds: %+v %v", rev, err)
	}
	u, err := st.Usage(svc.Refs)
	if err != nil || u.Unused != 0 {
		t.Fatalf("usage %+v %v", u, err)
	}
	ids := make([]string, len(rev.Updates))
	for i, up := range rev.Updates {
		ids[i] = up.ID
	}
	job, err := svc.ApplyUpdates(rev.CheckID, ids)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.tasks.Wait(ctx, job); err != nil {
		t.Fatal(err)
	}
	after, err := svc.List(ctx, in.ID, content.Mods)
	if err != nil || len(after) != 3 {
		t.Fatalf("%+v %v", after, err)
	}
	for _, r := range after {
		if r.Status != nil && r.Status.Kind == "update" {
			t.Errorf("%s still shows an update", r.Name)
		}
	}
	t.Logf("%d updates applied, store holds %d files", len(rev.Updates), u.Blobs)
}

// TestNoRiskInstall installs NoRiskClient Mod Mini from NoRisk's Maven
// repository into a Fabric instance and checks it shows as a NoRisk mod.
func TestNoRiskInstall(t *testing.T) {
	h := newHarness(t)
	nr := norisk.New(h.http)
	insts, err := instance.Open(filepath.Join(h.root, "it-instances"), h.log)
	if err != nil {
		t.Fatal(err)
	}
	in, err := insts.Create(instance.CreateRequest{Name: "NoRisk IT", Minecraft: "1.21.4", Loader: "fabric", LoaderVersion: "0.16.9"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(insts.Dir(in.ID)) })
	st, err := store.Open(filepath.Join(h.root, "store", "content"), h.ver, h.log)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := content.New(content.Deps{Instances: insts, Store: st, Icons: icons.OpenSized(filepath.Join(h.root, "cache", "content-icons"), 64),
		Platforms: []modplatform.Provider{nr}, Tasks: h.tasks, Log: h.log}, filepath.Join(h.root, "store", "content"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := h.ctx()
	page, err := nr.Search(ctx, modplatform.SearchQuery{Type: modplatform.TypeMod, Text: "NRC Mini", Game: "1.21.4", Loaders: []string{"fabric"}})
	if err != nil || len(page.Hits) != 1 {
		t.Fatalf("search %+v %v", page, err)
	}
	plan, err := svc.PlanInstall(ctx, content.InstallRequest{Instance: in.ID, Kind: content.Mods, Source: modplatform.NoRisk, Project: page.Hits[0].ID})
	if err != nil || plan.Version == "" || plan.Blocked || len(plan.Deps) != 0 {
		t.Fatalf("plan %+v %v", plan, err)
	}
	job, err := svc.ApplyInstall(plan.PlanID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.tasks.Wait(ctx, job); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.List(ctx, in.ID, content.Mods)
	if err != nil || len(rows) != 1 || rows[0].Source != "norisk" || rows[0].Ref != "nr:nrcmini" {
		t.Fatalf("rows %+v %v", rows, err)
	}
	if rev, err := svc.CheckUpdates(ctx, in.ID, content.Mods, modplatform.Release); err != nil || len(rev.Updates) != 0 {
		t.Fatalf("a fresh install has updates: %+v %v", rev, err)
	}
	t.Logf("installed %s %s", plan.Name, plan.Version)
}
