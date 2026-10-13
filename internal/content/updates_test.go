package content

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // test fixtures
	"crypto/sha512"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quartz/internal/content/packwiz"
	"quartz/internal/core/download"
	"quartz/internal/core/tasks"
	"quartz/internal/instance"
	"quartz/internal/modplatform"
)

// cdn serves fixed bodies by URL, standing in for the platform CDNs.
type cdn map[string][]byte

func (c cdn) Do(req *http.Request) (*http.Response, error) {
	body, ok := c[req.URL.String()]
	if !ok {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Header: http.Header{}, Request: req}, nil
}

func jarBytes(t *testing.T, id string) []byte {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.jar")
	writeZip(t, p, map[string]string{"fabric.mod.json": `{"id":"` + id + `","version":"x"}`})
	b, _ := os.ReadFile(p) //nolint:gosec // test temp dir
	return b
}

func fileOf(name, url string, body []byte) modplatform.File {
	s1 := sha1.Sum(body) //nolint:gosec // test fixtures
	s5 := sha512.Sum512(body)
	return modplatform.File{Name: name, URL: url, Size: int64(len(body)), SHA1: hex.EncodeToString(s1[:]), SHA512: hex.EncodeToString(s5[:])}
}

// withTasks gives the env a real download engine over the fake CDN.
func (e *env) withTasks(files cdn) *tasks.Engine {
	e.t.Helper()
	root := filepath.Dir(filepath.Dir(e.svc.d.Store.Path("0000000000000000000000000000000000000000")))
	root = filepath.Dir(filepath.Dir(root)) // the data root holding store/content
	ver, err := download.OpenVerified(filepath.Join(root, "verified.json"), root, slog.New(slog.DiscardHandler))
	if err != nil {
		e.t.Fatal(err)
	}
	eng, err := tasks.New(tasks.Config{Downloader: download.New(files, ver), Root: root, HistoryPath: filepath.Join(root, "downloads.json"), Parallel: 4})
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = eng.Close() })
	e.svc.d.Tasks = eng
	return eng
}

var day = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

func TestCheckReviewApply(t *testing.T) {
	e := newEnv(t)
	e.mod("alpha.jar", "alpha", "Alpha", "")
	rows := e.list(Mods)
	if err := e.svc.SetEnabled(context.Background(), e.id, Mods, rows[0].ID, false); err != nil {
		t.Fatal(err)
	}
	ix, _ := e.svc.idx.get(e.id)
	oldSHA := ix.Get().Entries[rows[0].ID].SHA1

	newAlpha, beta := jarBytes(t, "alpha"), jarBytes(t, "betalib")
	alphaFile := fileOf("alpha-2.0.jar", "https://cdn.modrinth.com/data/PA/alpha-2.0.jar", newAlpha)
	betaFile := fileOf("betalib-1.0.jar", "https://cdn.modrinth.com/data/PB/betalib-1.0.jar", beta)
	a1 := modplatform.Version{Source: modplatform.Modrinth, ProjectID: "PA", ID: "a1", Number: "1.0", Type: "release", Published: day}
	a15 := modplatform.Version{Source: modplatform.Modrinth, ProjectID: "PA", ID: "a15", Number: "1.5", Type: "release", Published: day.AddDate(0, 0, 1)}
	a2 := modplatform.Version{Source: modplatform.Modrinth, ProjectID: "PA", ID: "a2", Number: "2.0", Type: "release", Published: day.AddDate(0, 0, 2),
		File: alphaFile, Deps: []modplatform.Dep{{ProjectID: "PB", Kind: modplatform.DepRequired}}}
	b1 := modplatform.Version{Source: modplatform.Modrinth, ProjectID: "PB", ID: "b1", Number: "1.0", Type: "release", Published: day, File: betaFile}
	e.svc.d.Platforms = []modplatform.Provider{&fakePlatform{src: modplatform.Modrinth,
		byHash:    map[string]modplatform.Version{oldSHA: a1},
		projects:  map[string]modplatform.Project{"PA": {Slug: "alpha", Title: "Alpha"}, "PB": {Slug: "betalib", Title: "Beta Lib"}},
		latest:    map[string]modplatform.Version{"PA": a2},
		versions:  map[string][]modplatform.Version{"PA": {a2, a15, a1}, "PB": {b1}},
		changelog: map[string]string{"a2": "- Faster", "a15": "- Fixes"}}}
	eng := e.withTasks(cdn{alphaFile.URL: newAlpha, betaFile.URL: beta})

	ctx := context.Background()
	rev, err := e.svc.CheckUpdates(ctx, e.id, Mods, modplatform.Release)
	if err != nil {
		t.Fatal(err)
	}
	if len(rev.Updates) != 2 || rev.Updates[0].From != "1.0" || rev.Updates[0].To != "2.0" || !rev.Updates[1].Dependency {
		t.Fatalf("%+v", rev.Updates)
	}
	if p := e.svc.Pending(e.id); len(p) != 1 || p[0].To != "2.0" {
		t.Fatalf("pending %+v", p)
	}
	if r := e.list(Mods)[0]; r.Status == nil || r.Status.Kind != "update" || r.Status.Version != "2.0" {
		t.Fatalf("badge %+v", r.Status)
	}
	notes, err := e.svc.Changelog(ctx, rev.CheckID, rev.Updates[0].ID)
	if err != nil || len(notes) != 2 || notes[0].ID != "2.0" || notes[0].Items[0] != "Faster" || notes[1].ID != "1.5" {
		t.Fatalf("%+v %v", notes, err)
	}
	job, err := e.svc.ApplyUpdates(rev.CheckID, []string{rev.Updates[0].ID, rev.Updates[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Wait(ctx, job); err != nil {
		t.Fatal(err)
	}
	mods := filepath.Join(e.game, "mods")
	for name, want := range map[string]bool{"alpha-2.0.jar.disabled": true, "alpha.jar.disabled": false, "betalib-1.0.jar": true} {
		if _, err := os.Stat(filepath.Join(mods, name)); (err == nil) != want {
			t.Errorf("%s exists=%v", name, err == nil)
		}
	}
	pws, _ := packwiz.ReadDir(filepath.Join(mods, ".index"))
	if pw, ok := pws["alpha-2.0.jar"]; !ok || pw.Modrinth.Version != "a2" {
		t.Fatalf("metadata %+v", pws)
	}
	if a := e.svc.Activity(e.id); len(a) == 0 || a[0].Title != "Updated 2 mods" {
		t.Fatalf("%+v", a)
	}
	rows = e.list(Mods)
	if len(rows) != 2 || byName(rows, "Alpha").Version != "2.0" || byName(rows, "Alpha").Enabled || byName(rows, "Alpha").Status != nil {
		t.Fatalf("%+v", rows)
	}
	if len(e.svc.Pending(e.id)) != 0 {
		t.Fatal("applied updates still pending")
	}
	if _, err := e.svc.ApplyUpdates("nope", nil); err == nil {
		t.Fatal("unknown check id")
	}
}

func TestLockedItemsAreSkipped(t *testing.T) {
	e := newEnv(t)
	e.mod("alpha.jar", "alpha", "Alpha", "")
	rows := e.list(Mods)
	ix, _ := e.svc.idx.get(e.id)
	sha := ix.Get().Entries[rows[0].ID].SHA1
	a1 := modplatform.Version{Source: modplatform.Modrinth, ProjectID: "PA", ID: "a1", Number: "1.0", Type: "release", Published: day}
	a2 := a1
	a2.ID, a2.Number, a2.Published = "a2", "2.0", day.AddDate(0, 0, 1)
	e.svc.d.Platforms = []modplatform.Provider{&fakePlatform{src: modplatform.Modrinth, byHash: map[string]modplatform.Version{sha: a1},
		projects: map[string]modplatform.Project{"PA": {Title: "Alpha"}}, latest: map[string]modplatform.Version{"PA": a2}}}
	if _, err := e.svc.Identify(context.Background(), e.id, Mods); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetLocked(e.id, rows[0].ID, true); err != nil {
		t.Fatal(err)
	}
	rev, err := e.svc.CheckUpdates(context.Background(), e.id, Mods, modplatform.Release)
	if err != nil || len(rev.Updates) != 0 || len(rev.Locked) != 1 || rev.Locked[0].Version != "1.0" {
		t.Fatalf("%+v %v", rev, err)
	}
}

func TestSwitchMovesAndTurnsOffMods(t *testing.T) {
	e := newEnv(t)
	e.mod("alpha.jar", "alpha", "Alpha", "")
	e.mod("gone.jar", "gone", "Gone", "")
	e.mod("local.jar", "local", "Local", "")
	rows := e.list(Mods)
	ix, _ := e.svc.idx.get(e.id)
	sha := func(name string) string { return ix.Get().Entries[byName(rows, name).ID].SHA1 }
	newAlpha := jarBytes(t, "alpha")
	f := fileOf("alpha-mc2.jar", "https://cdn.modrinth.com/data/PA/alpha-mc2.jar", newAlpha)
	e.svc.d.Platforms = []modplatform.Provider{&fakePlatform{src: modplatform.Modrinth,
		byHash: map[string]modplatform.Version{
			sha("Alpha"): {Source: modplatform.Modrinth, ProjectID: "PA", ID: "a1", Number: "1", Type: "release"},
			sha("Gone"):  {Source: modplatform.Modrinth, ProjectID: "PG", ID: "g1", Number: "1", Type: "release"}},
		projects: map[string]modplatform.Project{"PA": {Title: "Alpha"}, "PG": {Title: "Gone"}},
		latest:   map[string]modplatform.Version{"PA": {Source: modplatform.Modrinth, ProjectID: "PA", ID: "a9", Number: "9", Type: "release", File: f}}}}
	eng := e.withTasks(cdn{f.URL: newAlpha})
	ctx := context.Background()
	if _, err := e.svc.Identify(ctx, e.id, Mods); err != nil {
		t.Fatal(err)
	}
	c, err := e.svc.Compatibility(ctx, e.id, "1.21.5", "", modplatform.Release)
	if err != nil || c != (Compat{Matching: 1, Total: 3, Unknown: 1}) {
		t.Fatalf("%+v %v", c, err)
	}
	applied := false
	res, err := e.svc.Switch(ctx, e.id, "1.21.5", "fabric", modplatform.Release, func(context.Context) error { applied = true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 || len(res.Disabled) != 1 || res.Disabled[0] != "Gone" || len(res.Unknown) != 1 || res.Job == "" || applied {
		t.Fatalf("%+v applied=%v", res, applied)
	}
	if err := eng.Wait(ctx, res.Job); err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("the instance was not switched after the mods")
	}
	rows = e.list(Mods)
	if byName(rows, "Gone").Enabled || !byName(rows, "Alpha").Enabled || byName(rows, "Alpha").Version != "9" {
		t.Fatalf("%+v", rows)
	}
}

func TestFileNameIsSafe(t *testing.T) {
	cases := map[string]string{"sodium-0.6.jar": "sodium-0.6.jar", "../../evil.jar": "evil.jar", `a\b:c.jar`: "b_c.jar",
		"noext": "noext.jar", ".jar": "jar.jar", "..hidden.jar": "hidden.jar"}
	for in, want := range cases {
		if got := fileName(in, ".jar"); got != want {
			t.Errorf("fileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCheckAllSkipsInstancesWithoutMods(t *testing.T) {
	e := newEnv(t)
	e.mod("alpha.jar", "alpha", "Alpha", "")
	rows := e.list(Mods)
	ix, _ := e.svc.idx.get(e.id)
	oldSHA := ix.Get().Entries[rows[0].ID].SHA1
	newAlpha := jarBytes(t, "alpha")
	a1 := modplatform.Version{Source: modplatform.Modrinth, ProjectID: "PA", ID: "a1", Number: "1.0", Type: "release", Published: day}
	a2 := modplatform.Version{Source: modplatform.Modrinth, ProjectID: "PA", ID: "a2", Number: "2.0", Type: "release", Published: day.AddDate(0, 0, 2),
		File: fileOf("alpha-2.0.jar", "https://cdn.modrinth.com/data/PA/alpha-2.0.jar", newAlpha)}
	e.svc.d.Platforms = []modplatform.Provider{&fakePlatform{src: modplatform.Modrinth,
		byHash:   map[string]modplatform.Version{oldSHA: a1},
		projects: map[string]modplatform.Project{"PA": {Slug: "alpha", Title: "Alpha"}},
		latest:   map[string]modplatform.Version{"PA": a2},
		versions: map[string][]modplatform.Version{"PA": {a2, a1}}}}
	if _, err := e.svc.d.Instances.Create(instance.CreateRequest{Name: "Empty", Minecraft: "1.21.1"}, instance.JavaSettings{}); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.CheckAll(context.Background(), modplatform.Release)
	if err != nil || len(got) != 1 || got[0].InstanceID != e.id || got[0].Updates != 1 {
		t.Fatalf("check all = %+v, %v", got, err)
	}
}
