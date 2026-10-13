package meta

import (
	"archive/zip"
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"quartz/internal/core/archive"
	"quartz/internal/icons"
)

func zipBytes(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(body)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jar(t *testing.T, entries map[string]string) *archive.Reader {
	t.Helper()
	m := map[string][]byte{}
	for k, v := range entries {
		m[k] = []byte(v)
	}
	return jarOf(t, m)
}

func jarOf(t *testing.T, entries map[string][]byte) *archive.Reader {
	t.Helper()
	p := filepath.Join(t.TempDir(), "mod.jar")
	if err := os.WriteFile(p, zipBytes(t, entries), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := archive.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	img.Set(1, 1, color.NRGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFabric(t *testing.T) {
	r := jarOf(t, map[string][]byte{
		"fabric.mod.json": []byte("\xef\xbb\xbf" + `{"schemaVersion":1,"id":"sodium","version":"0.6.5","name":"§aSodium","description":"Fast
renderer","authors":["JellySquid",{"name":"IMS"}],"icon":{"16x16":"small.png","128x128":"assets/icon.png"},
"depends":{"minecraft":">=1.21","fabricloader":"*","fabric-api":"*"},"breaks":{"optifabric":"*"},
"custom":{"modmenu":{"badges":["library"]}}}`),
		"assets/icon.png":         pngBytes(t),
		"META-INF/jars/inner.jar": zipBytes(t, map[string][]byte{"fabric.mod.json": []byte(`{"id":"fabric-api-base","provides":["fabric-base"]}`)}),
	})
	lib := icons.OpenSized(t.TempDir(), 64)
	info, err := Mod(r, lib)
	if err != nil {
		t.Fatal(err)
	}
	want := Info{Format: FormatFabric, ID: "sodium", Name: "Sodium", Version: "0.6.5", Description: "Fast renderer",
		Authors: []string{"JellySquid", "IMS"}, Loaders: []string{"fabric"}, Library: true,
		Deps:     []Dep{{ID: "optifabric", Kind: Breaks}, {ID: "fabric-api", Kind: Required}},
		Provides: []string{"fabric-api-base", "fabric-base"}}
	if info.Icon == "" {
		t.Fatal("icon not stored")
	}
	info.Icon = ""
	if !reflect.DeepEqual(info, want) {
		t.Fatalf("got  %+v\nwant %+v", info, want)
	}
}

func TestQuilt(t *testing.T) {
	r := jar(t, map[string]string{"quilt.mod.json": `{"schema_version":1,"quilt_loader":{"id":"qsl_mod","version":"1.0",
"provides":[{"id":"alias"}],"depends":["minecraft",{"id":"qsl","optional":false},{"id":"maybe","optional":true}],
"metadata":{"name":"Q Mod","contributors":{"Bea":"Owner","Al":"Dev"}}}}`})
	info, err := Mod(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "qsl_mod" || info.Name != "Q Mod" || !reflect.DeepEqual(info.Authors, []string{"Al", "Bea"}) ||
		!reflect.DeepEqual(info.Deps, []Dep{{ID: "qsl", Kind: Required}}) || !reflect.DeepEqual(info.Provides, []string{"alias"}) {
		t.Fatalf("%+v", info)
	}
}

func TestForgeTOML(t *testing.T) {
	toml := `modLoader="javafml"
loaderVersion="[47,)"
license="MIT"
authors="Alice, Bob"
logoFile="logo.png"
[[mods]]
modId="jei"
version="${file.jarVersion}"
displayName="Just Enough Items"
description='''
Item list'''
[[dependencies.jei]]
modId="forge"
mandatory=true
[[dependencies.jei]]
modId="architectury"
mandatory=true
[[dependencies.jei]]
modId="optional_thing"
mandatory=false
`
	r := jar(t, map[string]string{"META-INF/mods.toml": toml, "META-INF/MANIFEST.MF": "Manifest-Version: 1.0\r\nImplementation-Version: 15.2.0.27\r\n"})
	info, err := Mod(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "jei" || info.Version != "15.2.0.27" || info.Name != "Just Enough Items" || info.Description != "Item list" ||
		!reflect.DeepEqual(info.Authors, []string{"Alice", "Bob"}) || !reflect.DeepEqual(info.Loaders, []string{"forge"}) ||
		!reflect.DeepEqual(info.Deps, []Dep{{ID: "architectury", Kind: Required}}) {
		t.Fatalf("%+v", info)
	}
}

func TestNeoForgeTypes(t *testing.T) {
	r := jar(t, map[string]string{"META-INF/neoforge.mods.toml": `[[mods]]
modId="create"
version="6.0"
[[dependencies.create]]
modId="ponder"
type="required"
[[dependencies.create]]
modId="optifine"
type="incompatible"
`})
	info, _ := Mod(r, nil)
	if info.Format != FormatNeoForge || !reflect.DeepEqual(info.Deps, []Dep{{ID: "optifine", Kind: Breaks}, {ID: "ponder", Kind: Required}}) {
		t.Fatalf("%+v", info)
	}
}

func TestInvalidTOMLIsSalvaged(t *testing.T) {
	r := jar(t, map[string]string{"META-INF/mods.toml": "[[mods]]\nmodId=\"broken\"\nversion=\"2.0\"\ndisplayName=\"Broken Mod\"\nkey = = oops\n"})
	info, err := Mod(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "broken" || info.Version != "2.0" || info.Name != "Broken Mod" || !info.Partial {
		t.Fatalf("%+v", info)
	}
}

func TestModInfoShapes(t *testing.T) {
	for name, body := range map[string]string{
		"array":   `[{"modid":"mod_ironchest","name":"Iron Chests","version":"6.0","authorList":["cpw"],"requiredMods":["required-after:Forge@[10,)","mod_codechicken"]}]`,
		"wrapped": `{"modListVersion":2,"modList":[{"modid":"ironchest","name":"Iron Chests","version":"6.0","authorList":["cpw"],"dependencies":["codechicken"]}]}`,
	} {
		info, err := Mod(jar(t, map[string]string{"mcmod.info": body}), nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if info.ID != "ironchest" || info.Name != "Iron Chests" || !reflect.DeepEqual(info.Loaders, []string{"forge"}) ||
			!reflect.DeepEqual(info.Deps, []Dep{{ID: "codechicken", Kind: Required}}) {
			t.Fatalf("%s: %+v", name, info)
		}
	}
}

func TestNotAMod(t *testing.T) {
	if _, err := Mod(jar(t, map[string]string{"a.class": "x"}), nil); !errors.Is(err, ErrNotContent) {
		t.Fatalf("err = %v", err)
	}
}

func TestPackFormats(t *testing.T) {
	cases := map[string]struct {
		mcmeta string
		want   *PackFormat
	}{
		"pack_format":         {`{"pack":{"pack_format":15,"description":"x"}}`, &PackFormat{Format{15}, Format{15, anyMinor}}},
		"supported number":    {`{"pack":{"pack_format":18,"supported_formats":22}}`, &PackFormat{Format{18}, Format{22, anyMinor}}},
		"supported array":     {`{"pack":{"pack_format":18,"supported_formats":[16,22]}}`, &PackFormat{Format{16}, Format{22, anyMinor}}},
		"supported object":    {`{"pack":{"pack_format":18,"supported_formats":{"min_inclusive":16,"max_inclusive":34}}}`, &PackFormat{Format{16}, Format{34, anyMinor}}},
		"min and max formats": {`{"pack":{"min_format":[88,0],"max_format":[97,1]}}`, &PackFormat{Format{88}, Format{97, 1}}},
		"min only":            {`{"pack":{"min_format":69}}`, &PackFormat{Format{69}, Format{69, anyMinor}}},
		"none":                {`{"pack":{"description":"x"}}`, nil},
	}
	for name, c := range cases {
		info, err := Pack(jar(t, map[string]string{"pack.mcmeta": c.mcmeta}), nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !reflect.DeepEqual(info.Pack, c.want) {
			t.Errorf("%s: got %+v want %+v", name, info.Pack, c.want)
		}
	}
}

func TestPackDescriptionComponent(t *testing.T) {
	info, _ := Pack(jar(t, map[string]string{"pack.mcmeta": `{"pack":{"pack_format":34,"description":[{"text":"§6Bushy "},{"text":"leaves","extra":["!"]}]}}`}), nil)
	if info.Description != "Bushy leaves!" {
		t.Fatalf("%q", info.Description)
	}
}

func TestLabel(t *testing.T) {
	cases := map[string]struct {
		pack *PackFormat
		game Format
		want string
	}{
		"fits":             {&PackFormat{Format{34}, Format{34, anyMinor}}, Format{34}, ""},
		"fits a minor":     {&PackFormat{Format{97}, Format{97, anyMinor}}, Format{97, 1}, ""},
		"older, known":     {&PackFormat{Format{15}, Format{15, anyMinor}}, Format{34}, "Made for 1.20"},
		"older, unknown":   {&PackFormat{Format{90}, Format{90, anyMinor}}, Format{97, 1}, "Made for an older version"},
		"newer, unknown":   {&PackFormat{Format{200}, Format{200, anyMinor}}, Format{34}, "Made for a newer version"},
		"no formats known": {nil, Format{34}, ""},
	}
	for name, c := range cases {
		if got := Label("resource", c.pack, c.game); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}

func TestShaderAndFolderPacks(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "shaders"), 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if _, err := Shader(Dir{root}); err != nil {
		t.Fatalf("folder shader: %v", err)
	}
	if _, err := Shader(jar(t, map[string]string{"pack.mcmeta": "{}"})); !errors.Is(err, ErrNotContent) {
		t.Fatal("a zip without shaders/ is no shader pack")
	}
	if err := os.WriteFile(filepath.Join(dir, "pack.mcmeta"), []byte(`{"pack":{"pack_format":34,"description":"Folder"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := Pack(Dir{root}, nil)
	if err != nil || info.Description != "Folder" {
		t.Fatalf("%+v %v", info, err)
	}
}

func TestClientPack(t *testing.T) {
	for name, c := range map[string]struct {
		body string
		want ClientFormats
	}{
		"1.16.5": {`{"pack_version":6}`, ClientFormats{Resource: Format{6}, Data: Format{6}}},
		"1.21.1": {`{"world_version":3955,"pack_version":{"resource":34,"data":48}}`, ClientFormats{Resource: Format{34}, Data: Format{48}, HasData: true, WorldVersion: 3955}},
		"26.3":   {`{"pack_version":{"resource_major":97,"resource_minor":1,"data_major":121,"data_minor":0}}`, ClientFormats{Resource: Format{97, 1}, Data: Format{121}, HasData: true}},
	} {
		p := filepath.Join(t.TempDir(), "client.jar")
		_ = os.WriteFile(p, zipBytes(t, map[string][]byte{"version.json": []byte(c.body)}), 0o600)
		got, ok := ClientPack(p)
		if !ok || got != c.want {
			t.Errorf("%s: %+v %v", name, got, ok)
		}
	}
}

func TestCleanText(t *testing.T) {
	if got := clean("  a\n\tb§lc  ", 10); got != "a bc" {
		t.Fatalf("%q", got)
	}
	if got := clean("abcdefghij", 5); got != "abcd…" {
		t.Fatalf("%q", got)
	}
}

func TestSide(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"fabric client", map[string]string{"fabric.mod.json": `{"id":"a","environment":"client"}`}, SideClient},
		{"fabric server", map[string]string{"fabric.mod.json": `{"id":"a","environment":"server"}`}, SideServer},
		{"fabric both", map[string]string{"fabric.mod.json": `{"id":"a","environment":"*"}`}, ""},
		{"fabric unset", map[string]string{"fabric.mod.json": `{"id":"a"}`}, ""},
		{"quilt client", map[string]string{"quilt.mod.json": `{"quilt_loader":{"id":"a","version":"1"},"minecraft":{"environment":"client"}}`}, SideClient},
		{"quilt server", map[string]string{"quilt.mod.json": `{"quilt_loader":{"id":"a","version":"1"},"minecraft":{"environment":"dedicated_server"}}`}, SideServer},
		{"forge root", map[string]string{"META-INF/mods.toml": "clientSideOnly=true\n[[mods]]\nmodId=\"a\"\n"}, SideClient},
		{"forge mod", map[string]string{"META-INF/mods.toml": "[[mods]]\nmodId=\"a\"\nclientSideOnly=true\n"}, SideClient},
		{"forge false", map[string]string{"META-INF/mods.toml": "clientSideOnly=false\n[[mods]]\nmodId=\"a\"\n"}, ""},
	}
	for _, c := range cases {
		info, err := Mod(jar(t, c.files), nil)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if info.Side != c.want {
			t.Errorf("%s: side %q, want %q", c.name, info.Side, c.want)
		}
	}
}
