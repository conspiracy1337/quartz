package content

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"quartz/internal/core/tasks"
	"quartz/internal/instance"
	"quartz/internal/manual"
	"quartz/internal/modplatform"
)

func TestPlanAndApplyInstallWithDependencies(t *testing.T) {
	e := newEnv(t)
	// Fabric API is installed, identified on CurseForge only.
	e.mod("fabric-api.jar", "fabric-api", "Fabric API", "")
	rows := e.list(Mods)
	ix, _ := e.svc.idx.get(e.id)
	apiSHA := ix.Get().Entries[rows[0].ID].SHA1
	e.svc.cat.update(apiSHA, func(k *Known) {
		k.CurseForge = &Ident{Source: "curseforge", ProjectID: "306612", Slug: "fabric-api", Title: "Fabric API"}
	})

	iris, sodium, indium := jarBytes(t, "iris"), jarBytes(t, "sodium"), jarBytes(t, "indium")
	irisF := fileOf("iris-1.8.jar", "https://cdn.modrinth.com/data/PI/iris-1.8.jar", iris)
	sodiumF := fileOf("sodium-0.6.jar", "https://cdn.modrinth.com/data/PS/sodium-0.6.jar", sodium)
	indiumF := fileOf("indium-1.0.jar", "https://cdn.modrinth.com/data/PO/indium-1.0.jar", indium)
	req := func(id string) modplatform.Dep { return modplatform.Dep{ProjectID: id, Kind: modplatform.DepRequired} }
	irisV := modplatform.Version{Source: modplatform.Modrinth, ProjectID: "PI", ID: "i18", Number: "1.8.1", Type: "release", Published: day, File: irisF,
		Loaders: []string{"fabric"}, Deps: []modplatform.Dep{req("PS"), {ProjectID: "PO", Kind: modplatform.DepOptional}}}
	sodiumV := modplatform.Version{Source: modplatform.Modrinth, ProjectID: "PS", ID: "s06", Number: "0.6", Type: "release", Published: day, File: sodiumF,
		Deps: []modplatform.Dep{req("PF"), req("PI")}} // a cycle back to Iris, and Fabric API under another id
	indiumV := modplatform.Version{Source: modplatform.Modrinth, ProjectID: "PO", ID: "o1", Number: "1.0", Type: "release", Published: day, File: indiumF}
	e.svc.d.Platforms = []modplatform.Provider{&fakePlatform{src: modplatform.Modrinth,
		projects: map[string]modplatform.Project{"PI": {Slug: "iris", Title: "Iris Shaders"}, "PS": {Slug: "sodium", Title: "Sodium"},
			"PO": {Slug: "indium", Title: "Indium", Summary: "Improves Sodium compatibility"}, "PF": {Slug: "fabric-api", Title: "Fabric API"}},
		versions: map[string][]modplatform.Version{"PI": {irisV}, "PS": {sodiumV}, "PO": {indiumV}}}}
	eng := e.withTasks(cdn{irisF.URL: iris, sodiumF.URL: sodium, indiumF.URL: indium})

	ctx := context.Background()
	pl, err := e.svc.PlanInstall(ctx, InstallRequest{Instance: e.id, Kind: Mods, Source: modplatform.Modrinth, Project: "PI"})
	if err != nil {
		t.Fatal(err)
	}
	// Fabric API is installed and up to date, so it isn't listed.
	if pl.Name != "Iris Shaders" || pl.Version != "1.8.1" || pl.Mods != 1 || len(pl.Deps) != 2 {
		t.Fatalf("plan %+v", pl)
	}
	want := []struct {
		name     string
		required bool
	}{{"Sodium", true}, {"Indium", false}}
	for i, w := range want {
		d := pl.Deps[i]
		if d.Name != w.name || d.Required != w.required || d.Installed {
			t.Fatalf("dep %d = %+v, want %+v", i, d, w)
		}
	}
	started, err := e.svc.ApplyInstall(pl.PlanID, []string{"dep-modrinth-PO"}) // Indium left out
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Wait(ctx, started.Job); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"iris-1.8.jar": true, "sodium-0.6.jar": true, "indium-1.0.jar": false} {
		if _, err := os.Stat(filepath.Join(e.game, "mods", name)); (err == nil) != want {
			t.Errorf("%s exists=%v", name, err == nil)
		}
	}
	if a := e.svc.Activity(e.id); len(a) == 0 || a[0].Title != "Added Iris Shaders" || a[0].Detail != "Sodium" {
		t.Fatalf("activity %+v", a)
	}

	// Installing it again plans a replacement of the installed file, and the
	// installed Sodium, now outdated, is offered as an update.
	sodiumNew := sodiumV
	sodiumNew.ID, sodiumNew.Number, sodiumNew.Published = "s07", "0.7", day.Add(24*time.Hour)
	fp := e.svc.d.Platforms[0].(*fakePlatform)
	fp.versions["PS"] = []modplatform.Version{sodiumNew, sodiumV}
	again, err := e.svc.PlanInstall(ctx, InstallRequest{Instance: e.id, Kind: Mods, Source: modplatform.Modrinth, Project: "PI"})
	if err != nil || again.Replaces != "1.8.1" {
		t.Fatalf("replace plan %+v %v", again, err)
	}
	if len(again.Deps) == 0 || again.Deps[0].Name != "Sodium" || !again.Deps[0].Installed || again.Deps[0].Update != "0.6" || again.Deps[0].Version != "0.7" {
		t.Fatalf("an outdated dependency must be offered as an update: %+v", again.Deps)
	}
	sp, _ := e.svc.plan(again.PlanID)
	if v := sp.items["dep-modrinth-PS"]; v == nil || v.item == "" {
		t.Fatal("the update must replace the installed item")
	}
}

func TestPlanInstallEdges(t *testing.T) {
	e := newEnv(t)
	e.svc.d.Platforms = []modplatform.Provider{&fakePlatform{src: modplatform.Modrinth,
		projects: map[string]modplatform.Project{"PT": {Title: "Terralith"}},
		versions: map[string][]modplatform.Version{"PT": {{Source: modplatform.Modrinth, ProjectID: "PT", ID: "t", Number: "t1", Type: "beta"}}}}}
	ctx := context.Background()
	if _, err := e.svc.PlanInstall(ctx, InstallRequest{Instance: e.id, Kind: DataPacks, Source: modplatform.Modrinth, Project: "PT", World: "../x"}); !errors.Is(err, ErrPickWorld) {
		t.Fatalf("bad world: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(e.game, "saves", "New World"), 0o700); err != nil {
		t.Fatal(err)
	}
	pl, err := e.svc.PlanInstall(ctx, InstallRequest{Instance: e.id, Kind: DataPacks, Source: modplatform.Modrinth, Project: "PT", World: "New World"})
	if err != nil || pl.Version != "t1" {
		t.Fatalf("a pack's only beta not offered: %+v %v", pl, err)
	}
	if pl.Channel != "Beta" {
		t.Fatalf("a beta picked on the release channel must say so: %+v", pl)
	}
	// A mod with only a beta gets it too, named as such.
	pl, err = e.svc.PlanInstall(ctx, InstallRequest{Instance: e.id, Kind: Mods, Source: modplatform.Modrinth, Project: "PT"})
	if err != nil || pl.Version != "t1" || pl.Channel != "Beta" {
		t.Fatalf("a mod's only beta not offered: %+v %v", pl, err)
	}
	pl, err = e.svc.PlanInstall(ctx, InstallRequest{Instance: e.id, Kind: Mods, Source: modplatform.Modrinth, Project: "PT", Channel: modplatform.Beta})
	if err != nil || pl.Channel != "" {
		t.Fatalf("a beta on the beta channel needs no note: %+v %v", pl, err)
	}
	// No build at all: no version.
	e.svc.d.Platforms[0].(*fakePlatform).versions["PT"] = nil
	if pl, err = e.svc.PlanInstall(ctx, InstallRequest{Instance: e.id, Kind: Mods, Source: modplatform.Modrinth, Project: "PT"}); err != nil || pl.Version != "" {
		t.Fatalf("a build for another game version offered: %+v %v", pl, err)
	}
	if _, err := e.svc.ApplyInstall(pl.PlanID, nil); err == nil {
		t.Fatal("applied a plan without a version")
	}
	if _, err := e.svc.PlanInstall(ctx, InstallRequest{Instance: e.id, Kind: Mods, Source: modplatform.CurseForge, Project: "1"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing platform: %v", err)
	}
}

func TestBlockedFileWaitsForTheDownloadsFolder(t *testing.T) {
	e := newEnv(t)
	balm := jarBytes(t, "balm")
	f := fileOf("balm-1.0.jar", "", balm)
	f.Page = "https://www.curseforge.com/minecraft/mc-mods/balm/download/9"
	bv := modplatform.Version{Source: modplatform.CurseForge, ProjectID: "531761", ID: "9", Number: "1.0", Type: "release", Published: day, File: f}
	e.svc.d.Platforms = []modplatform.Provider{&fakePlatform{src: modplatform.CurseForge,
		projects: map[string]modplatform.Project{"531761": {Slug: "balm", Title: "Balm"}},
		versions: map[string][]modplatform.Version{"531761": {bv}}}}
	eng := e.withTasks(cdn{})
	folder := t.TempDir()
	e.svc.d.Manual = manual.New(manual.Deps{Store: e.svc.d.Store, Tasks: eng, Folder: folder, Watch: true})

	ctx := context.Background()
	pl, err := e.svc.PlanInstall(ctx, InstallRequest{Instance: e.id, Kind: Mods, Source: modplatform.CurseForge, Project: "531761"})
	if err != nil || !pl.Blocked {
		t.Fatalf("plan %+v %v", pl, err)
	}
	started, err := e.svc.ApplyInstall(pl.PlanID, nil)
	if err != nil {
		t.Fatal(err)
	}
	waiting := func() bool {
		for _, j := range eng.Snapshot().Active {
			for _, st := range j.Steps {
				if j.ID == started.Job && st.State == tasks.StepWarn && st.Name == "1 mod needs a manual download" {
					return true
				}
			}
		}
		return false
	}
	for deadline := time.Now().Add(5 * time.Second); !waiting(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("no waiting step")
		}
	}
	if v, err := e.svc.d.Manual.View(started.Job); err != nil || len(v.Items) != 1 || v.Items[0].Name != "Balm" {
		t.Fatalf("view %+v %v", v, err)
	}
	if !e.svc.Busy(e.id) {
		t.Fatal("an instance with a waiting install must count as busy")
	}
	if err := os.WriteFile(filepath.Join(folder, "balm-1.0.jar"), balm, 0o600); err != nil {
		t.Fatal(err)
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := eng.Wait(wctx, started.Job); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.game, "mods", "balm-1.0.jar")); err != nil {
		t.Fatal("the manually downloaded mod was not installed")
	}
	if e.svc.Busy(e.id) {
		t.Fatal("still busy after the job ended")
	}
}

func TestInstallDependencyEdges(t *testing.T) {
	e := newEnv(t)
	// A hand-added, unidentified Cloth Config jar (mod id cloth_config), turned off.
	e.mod("cloth.jar.disabled", "cloth_config", "Cloth Config", "")
	mainJar, betaJar, pinnedJar := jarBytes(t, "main"), jarBytes(t, "beta"), jarBytes(t, "pinned")
	main := modplatform.Version{Source: modplatform.Modrinth, ProjectID: "PM", ID: "m1", Number: "1.0", Type: "release", Published: day,
		File: fileOf("main.jar", "https://cdn.modrinth.com/data/PM/main.jar", mainJar), Loaders: []string{"fabric"},
		Deps: []modplatform.Dep{
			{ProjectID: "PC", Kind: modplatform.DepRequired},     // present by mod id, but off
			{ProjectID: "PB", Kind: modplatform.DepRequired},     // only a beta exists
			{ProjectID: "PN", Kind: modplatform.DepRequired},     // nothing fits at all
			{VersionID: "pinned", Kind: modplatform.DepRequired}, // named by version only
		}}
	beta := modplatform.Version{Source: modplatform.Modrinth, ProjectID: "PB", ID: "b1", Number: "2.0-beta", Type: "beta", Published: day,
		File: fileOf("beta.jar", "https://cdn.modrinth.com/data/PB/beta.jar", betaJar)}
	pinned := modplatform.Version{Source: modplatform.Modrinth, ProjectID: "PV", ID: "pinned", Number: "0.9", Type: "release", Published: day,
		File: fileOf("pinned.jar", "https://cdn.modrinth.com/data/PV/pinned.jar", pinnedJar)}
	newer := pinned
	newer.ID, newer.Number = "newer", "1.0"
	fp := &fakePlatform{src: modplatform.Modrinth,
		projects: map[string]modplatform.Project{"PM": {Title: "Main"}, "PC": {Slug: "cloth-config", Title: "Cloth Config API"},
			"PB": {Title: "Beta Lib"}, "PN": {Title: "Nowhere"}, "PV": {Title: "Pinned Lib"}},
		versions: map[string][]modplatform.Version{"PM": {main}, "PB": {beta}, "PV": {newer, pinned}}}
	e.svc.d.Platforms = []modplatform.Provider{fakeCatalog{fp}}

	ctx := context.Background()
	pl, err := e.svc.PlanInstall(ctx, InstallRequest{Instance: e.id, Kind: Mods, Source: modplatform.Modrinth, Project: "PM"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]InstallDep{}
	for _, d := range pl.Deps {
		got[d.Name] = d
	}
	if d := got["Cloth Config API"]; !d.Installed || !d.Off {
		t.Errorf("a turned-off jar with the dependency's mod id counts as installed, to be turned on: %+v", d)
	}
	if d := got["Beta Lib"]; d.Installed || d.Unavailable || d.Size == 0 {
		t.Errorf("a beta-only required dependency must still be installed: %+v", d)
	}
	if d := got["Nowhere"]; !d.Unavailable {
		t.Errorf("a dependency with no build must be shown, not dropped: %+v", d)
	}
	if d := got["Pinned Lib"]; d.Size == 0 {
		t.Errorf("a version-only dependency must be resolved: %+v", d)
	}
	sp, _ := e.svc.plan(pl.PlanID)
	if v := sp.items["dep-modrinth-PV"]; v == nil || v.v.ID != "pinned" {
		t.Errorf("the pinned version must be used, not the newest")
	}
	eng := e.withTasks(cdn{main.File.URL: mainJar, beta.File.URL: betaJar, pinned.File.URL: pinnedJar})
	started, err := e.svc.ApplyInstall(pl.PlanID, []string{"dep-modrinth-PV"}) // a required one left out
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Wait(ctx, started.Job); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.game, "mods", "pinned.jar")); err == nil {
		t.Error("a skipped required dependency was installed")
	}
	if _, err := os.Stat(filepath.Join(e.game, "mods", "cloth.jar")); err != nil {
		t.Error("the turned-off dependency was not turned on")
	}

	vanilla, err := e.insts.Create(instance.CreateRequest{Name: "Plain", Minecraft: "1.21.1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.PlanInstall(ctx, InstallRequest{Instance: vanilla.ID, Kind: Mods, Source: modplatform.Modrinth, Project: "PM"}); !errors.Is(err, ErrNeedsLoader) {
		t.Fatalf("a mod into vanilla: %v", err)
	}
}
