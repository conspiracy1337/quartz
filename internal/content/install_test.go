package content

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

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
	if pl.Name != "Iris Shaders" || pl.Version != "1.8.1" || pl.Mods != 1 || len(pl.Deps) != 3 {
		t.Fatalf("plan %+v", pl)
	}
	want := []struct {
		name                string
		required, installed bool
	}{{"Sodium", true, false}, {"Fabric API", true, true}, {"Indium", false, false}}
	for i, w := range want {
		d := pl.Deps[i]
		if d.Name != w.name || d.Required != w.required || d.Installed != w.installed {
			t.Fatalf("dep %d = %+v, want %+v", i, d, w)
		}
	}
	job, err := e.svc.ApplyInstall(pl.PlanID, nil) // Indium left out
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Wait(ctx, job); err != nil {
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

	// Installing it again plans a replacement of the installed file.
	again, err := e.svc.PlanInstall(ctx, InstallRequest{Instance: e.id, Kind: Mods, Source: modplatform.Modrinth, Project: "PI"})
	if err != nil || again.Replaces != "1.8.1" {
		t.Fatalf("replace plan %+v %v", again, err)
	}
}

func TestPlanInstallEdges(t *testing.T) {
	e := newEnv(t)
	e.svc.d.Platforms = []modplatform.Provider{&fakePlatform{src: modplatform.Modrinth,
		projects: map[string]modplatform.Project{"PT": {Title: "Terralith"}},
		versions: map[string][]modplatform.Version{"PT": {{Source: modplatform.Modrinth, ProjectID: "PT", ID: "t", Type: "beta"}}}}}
	ctx := context.Background()
	if _, err := e.svc.PlanInstall(ctx, InstallRequest{Instance: e.id, Kind: DataPacks, Source: modplatform.Modrinth, Project: "PT", World: "../x"}); !errors.Is(err, ErrPickWorld) {
		t.Fatalf("bad world: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(e.game, "saves", "New World"), 0o700); err != nil {
		t.Fatal(err)
	}
	pl, err := e.svc.PlanInstall(ctx, InstallRequest{Instance: e.id, Kind: DataPacks, Source: modplatform.Modrinth, Project: "PT", World: "New World"})
	if err != nil || pl.Version != "" {
		t.Fatalf("a beta offered on the release channel: %+v %v", pl, err)
	}
	if _, err := e.svc.ApplyInstall(pl.PlanID, nil); err == nil {
		t.Fatal("applied a plan without a version")
	}
	if _, err := e.svc.PlanInstall(ctx, InstallRequest{Instance: e.id, Kind: Mods, Source: modplatform.CurseForge, Project: "1"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing platform: %v", err)
	}
}
