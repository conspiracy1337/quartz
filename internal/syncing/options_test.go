package syncing

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quartz/internal/core/optfile"
	"quartz/internal/settings"
)

func TestFormatOf(t *testing.T) {
	legacyFile := optfile.Parse([]byte("version:1343\nkey_key.attack:-100\n"), ":")
	modernFile := optfile.Parse([]byte("key_key.attack:key.mouse.left\n"), ":")
	for _, c := range []struct {
		mc          string
		f           optfile.File
		keys, upper bool
	}{
		{"1.12.2", optfile.File{}, true, false},
		{"1.8.9", optfile.File{}, true, true},
		{"1.13", optfile.File{}, false, false},
		{"1.21.4", optfile.File{}, false, false},
		{"26.1", optfile.File{}, false, false},
		{"24w14a", optfile.File{}, false, false},
		{"1.21.4", legacyFile, true, false}, // the file knows best
		{"1.12.2", modernFile, false, false},
	} {
		if fm := formatOf(c.mc, c.f); fm.legacyKeys != c.keys || fm.upperLang != c.upper {
			t.Errorf("%s: %+v", c.mc, fm)
		}
	}
}

func get(t *testing.T, p, key string) string {
	t.Helper()
	f, err := optfile.Read(p, ":")
	if err != nil {
		t.Fatal(err)
	}
	v, _ := f.Get(key)
	return v
}

func TestOptionsMergeAcrossVersions(t *testing.T) {
	e := newEnv(t) // a is 1.21.4, b is 1.12.2
	ctx := context.Background()
	put(t, e.game(e.a, "options.txt"), "version:3955\nfov:0.5\nlang:en_us\nresourcePacks:[\"vanilla\",\"file/a.zip\"]\nkey_key.attack:key.mouse.left\nkey_key.sprint:key.keyboard.left.control\n")
	put(t, e.game(e.b, "options.txt"), "version:1343\r\nfov:0.0\r\nlang:en_GB\r\nresourcePacks:[]\r\nkey_key.attack:-100\r\nkey_key.sprint:29\r\nkey_key.socialInteractions:0\r\n")
	e.enable(OptionsID)
	for _, id := range []string{e.a, e.b} {
		if err := e.svc.Join(ctx, id, OptionsID); err != nil {
			t.Fatal(err)
		}
	}
	shared := e.shared("options.txt")
	if get(t, shared, "fov") != "0.5" || get(t, shared, "version") != "" || get(t, shared, "resourcePacks") != "" {
		t.Fatal("the shared copy is seeded from the first instance, without its own keys")
	}

	// A run of A changes two keys.
	se, err := e.svc.Prepare(e.a)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := optfile.Read(e.game(e.a, "options.txt"), ":")
	f.Set("fov", "0.7")
	f.Set("key_key.attack", "key.keyboard.f")
	f.Set("lang", "de_de")
	_ = f.Write(e.game(e.a, "options.txt"))
	if err := se.Finish(); err != nil {
		t.Fatal(err)
	}
	if get(t, shared, "fov") != "0.7" || get(t, shared, "key_key.attack") != "key.keyboard.f" || get(t, shared, "key_key.sprint") != "key.keyboard.left.control" {
		t.Fatalf("shared after A's run:\n%s", body(t, shared))
	}

	// B, a 1.12.2 instance, gets them in its own format.
	if _, err := e.svc.Prepare(e.b); err != nil {
		t.Fatal(err)
	}
	b := e.game(e.b, "options.txt")
	for key, want := range map[string]string{
		"fov": "0.7", "key_key.attack": "33", "key_key.sprint": "29", "lang": "de_de",
		"version": "1343", "resourcePacks": "[]", "key_key.socialInteractions": "0",
	} {
		if got := get(t, b, key); got != want {
			t.Errorf("B %s = %q, want %q", key, got, want)
		}
	}
	if !strings.Contains(body(t, b), "\r\n") {
		t.Fatal("B's line endings changed")
	}
	// A's resource packs are its own.
	if get(t, e.game(e.a, "options.txt"), "resourcePacks") != `["vanilla","file/a.zip"]` {
		t.Fatal("resource packs synced")
	}
}

// A new instance joining shared options gets them in a file that also names
// the game's data version, which Minecraft needs to read it.
func TestNewOptionsFileGetsDataVersion(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.client = jar(t, filepath.Join(e.dir, "client.jar"), map[string]string{"version.json": `{"world_version":4189,"protocol_version":769}`})
	put(t, e.game(e.b, "options.txt"), "version:1343\nfov:0.5\n")
	e.enable(OptionsID)
	if err := e.svc.Join(ctx, e.b, OptionsID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Join(ctx, e.a, OptionsID); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(e.game(e.a, "options.txt"))
	if _, err := e.svc.Prepare(e.a); err != nil {
		t.Fatal(err)
	}
	if got := get(t, e.game(e.a, "options.txt"), "version"); got != "4189" || get(t, e.game(e.a, "options.txt"), "fov") != "0.5" {
		t.Fatalf("version %q in:\n%s", got, body(t, e.game(e.a, "options.txt")))
	}
}

func TestSharedKeybinds(t *testing.T) {
	e := newEnv(t)
	put(t, e.game(e.b, "options.txt"), "key_key.sprint:29\nkey_key.attack:-100\n")
	editKeys(t, e, []settings.SharedKey{
		{ID: "key.sprint", Key: "key.keyboard.left.shift"},
		{ID: "key.zoomify.zoom", Key: "key.keyboard.c"}, // B has no Zoomify
		{ID: "key.attack", Key: "key.keyboard.world.1"}, // no 1.12 code
	})
	if _, err := e.svc.Prepare(e.b); err != nil {
		t.Fatal(err)
	}
	if got := get(t, e.game(e.b, "options.txt"), "key_key.sprint"); got != "29" {
		t.Fatal("keys are written only for instances that turn them on")
	}
	if err := e.svc.SetKeys(e.b, true); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Prepare(e.b); err != nil {
		t.Fatal(err)
	}
	opts := e.game(e.b, "options.txt")
	if get(t, opts, "key_key.sprint") != "42" || get(t, opts, "key_key.attack") != "-100" || strings.Contains(body(t, opts), "zoomify") {
		t.Fatalf("%s", body(t, opts))
	}
	// A fresh instance gets every shared key.
	_ = e.svc.SetKeys(e.a, true)
	if _, err := e.svc.Prepare(e.a); err != nil {
		t.Fatal(err)
	}
	if get(t, e.game(e.a, "options.txt"), "key_key.zoomify.zoom") != "key.keyboard.c" {
		t.Fatal(body(t, e.game(e.a, "options.txt")))
	}
}

func editKeys(t *testing.T, e *env, keys []settings.SharedKey) {
	t.Helper()
	e.svc.editSettings(func(y *settings.Syncing) { y.Keys = keys })
}

func TestFilesCopyInAndOut(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable("hotbar")
	if err := e.svc.Join(ctx, e.a, "hotbar"); err != nil {
		t.Fatal(err)
	}
	put(t, e.shared("hotbar.nbt"), "shared")
	se, err := e.svc.Prepare(e.a)
	if err != nil || body(t, e.game(e.a, "hotbar.nbt")) != "shared" {
		t.Fatal("copy in", err)
	}
	put(t, e.game(e.a, "hotbar.nbt"), "played")
	future := time.Now().Add(time.Minute)
	_ = os.Chtimes(e.game(e.a, "hotbar.nbt"), future, future)
	if err := se.Finish(); err != nil || body(t, e.shared("hotbar.nbt")) != "played" {
		t.Fatal("copy out", err)
	}
}

func jar(t *testing.T, p string, files map[string]string) string {
	t.Helper()
	f, err := os.Create(p) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}
	_ = zw.Close()
	_ = f.Close()
	return p
}

func TestKeyCatalog(t *testing.T) {
	e := newEnv(t)
	e.client = jar(t, filepath.Join(e.dir, "client.jar"), map[string]string{"assets/minecraft/lang/en_us.json": `{"key.attack":"Attack/Destroy","key.categories.gameplay":"Gameplay"}`})
	e.modJars = map[string][]ModJar{e.a: {{Path: jar(t, filepath.Join(e.dir, "zoomify.jar"), map[string]string{"assets/zoomify/lang/en_us.json": `{"key.zoomify.zoom":"Zoom"}`}), Name: "Zoomify"}}}
	put(t, e.game(e.a, "options.txt"), "key_key.attack:key.mouse.left\nkey_key.zoomify.zoom:key.keyboard.c\nkey_key.mystery:key.keyboard.m\n")
	cat := e.svc.KeyCatalog()
	if len(cat) != 3 || cat[0].Name != "Attack/Destroy" || cat[0].Category != "Minecraft" || cat[0].Key != "Left Button" ||
		cat[1].Name != "Zoom (Zoomify)" || cat[1].Category != "Zoomify" || cat[2].Name != "key.mystery" {
		t.Fatalf("%+v", cat)
	}
	k, err := e.svc.AddKey("key.zoomify.zoom")
	if err != nil || k.Key != "C" || k.Name != "Zoom (Zoomify)" {
		t.Fatalf("%+v %v", k, err)
	}
	if _, err := e.svc.AddKey("key.nothing"); err == nil {
		t.Fatal("added a key no instance has")
	}
	if k, err := e.svc.SetKey("key.zoomify.zoom", "ControlLeft"); err != nil || k.Key != "Left Ctrl" {
		t.Fatalf("%+v %v", k, err)
	}
	if _, err := e.svc.SetKey("key.zoomify.zoom", "Nope"); err == nil {
		t.Fatal("unknown browser code")
	}
	if list := e.svc.Keybinds(); len(list) != 1 || list[0].Key != "Left Ctrl" || !e.svc.KeyCatalog()[1].Shared {
		t.Fatalf("%+v", list)
	}
	_ = e.svc.RemoveKey("key.zoomify.zoom")
	if len(e.svc.Keybinds()) != 0 {
		t.Fatal("remove")
	}
}
