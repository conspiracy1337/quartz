package content

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"quartz/internal/modplatform"
)

func TestServerMods(t *testing.T) {
	e := newEnv(t)
	e.mod("sodium.jar", "sodium", "Sodium", `,"environment":"client"`)
	e.mod("lithium.jar", "lithium", "Lithium", `,"environment":"*"`)
	e.mod("ferrite.jar", "ferritecore", "FerriteCore", "")
	e.mod("zoomify.jar", "zoomify", "Zoomify", "")
	e.mod("old.jar", "old", "Old Client Mod", `,"environment":"client"`)
	e.mod("off.jar.disabled", "off", "Off", "")
	rows := e.list(Mods)

	// Zoomify is on Modrinth, identified before sides were recorded.
	zoom := byName(rows, "Zoomify")
	sha := map[string]string{}
	ix, _ := e.svc.idx.get(e.id)
	for _, en := range ix.Get().Entries {
		sha[filepath.Base(en.Rel)] = en.SHA1
	}
	e.svc.cat.update(sha["zoomify.jar"], func(k *Known) { k.Modrinth = &Ident{Source: "modrinth", ProjectID: "w7ThoJFB", Title: "Zoomify"} })
	e.svc.d.Platforms = []modplatform.Provider{&fakePlatform{src: modplatform.Modrinth,
		projects: map[string]modplatform.Project{"w7ThoJFB": {ServerSide: "unsupported"}}}}
	// An entry parsed before its side was known.
	e.svc.cat.update(sha["old.jar"], func(k *Known) { k.Info.Side, k.Rev = "", 0 })
	if zoom.ClientOnly {
		t.Fatal("Zoomify isn't known to be client-only before Modrinth is asked")
	}

	mods, err := e.svc.ServerMods(context.Background(), e.id)
	if err != nil {
		t.Fatal(err)
	}
	var names, clientOnly []string
	for _, m := range mods {
		names = append(names, m.Name)
		if m.ClientOnly {
			clientOnly = append(clientOnly, m.Name)
		}
	}
	if want := []string{"FerriteCore", "Lithium", "Old Client Mod", "Sodium", "Zoomify"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("enabled mods by name %v, want %v", names, want)
	}
	if want := []string{"Old Client Mod", "Sodium", "Zoomify"}; !reflect.DeepEqual(clientOnly, want) {
		t.Fatalf("client-only %v, want %v", clientOnly, want)
	}
	if mods[0].File != "ferrite.jar" || mods[0].Key != "id:ferritecore" {
		t.Fatalf("a mod is keyed by its own id: %+v", mods[0])
	}
	if _, err := os.Stat(e.svc.d.Store.Path(mods[0].SHA1)); err != nil {
		t.Fatalf("mods are blobs in the store: %v", err)
	}
	if k, _ := e.svc.cat.get(sha["zoomify.jar"]); k.Modrinth.ServerSide != "unsupported" {
		t.Fatal("Modrinth's answer is kept in the catalog")
	}
	if r := byName(e.list(Mods), "Zoomify"); !r.ClientOnly {
		t.Fatal("the Mods row is marked client-only once known")
	}
}
