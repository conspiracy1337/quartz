package mojang

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func load(t *testing.T, id string) *Version {
	t.Helper()
	data, err := os.ReadFile("testdata/" + id + ".json")
	if err != nil {
		t.Fatal(err)
	}
	v, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

var (
	linux   = Env{OS: "linux", Arch: "x86_64", Features: map[string]bool{}}
	mac     = Env{OS: "osx", Arch: "arm64", Features: map[string]bool{}}
	windows = Env{OS: "windows", Arch: "x86_64", OSVersion: "10.0.19045", Features: map[string]bool{}}
)

func vars() Vars {
	return Vars{
		"auth_player_name": "Steve", "version_name": "x", "game_directory": "/g", "assets_root": "/a",
		"assets_index_name": "19", "auth_uuid": "u", "auth_access_token": "TOKEN", "clientid": "", "auth_xuid": "",
		"version_type": "release", "natives_directory": "/n", "launcher_name": "quartz", "launcher_version": "1",
		"classpath": "/cp", "user_type": "msa", "auth_session": "TOKEN", "game_assets": "/a/virtual/legacy",
		"user_properties": "{}", "resolution_width": "1280", "resolution_height": "720",
	}
}

func TestJavaAndAssetsFromJSON(t *testing.T) {
	cases := map[string]struct {
		java   int
		comp   string
		assets string
	}{
		"26.3": {25, "java-runtime-epsilon", "34"}, "1.21.4": {21, "java-runtime-delta", "19"},
		"1.16.5": {8, "jre-legacy", "1.16"}, "1.6.4": {8, "jre-legacy", "legacy"},
	}
	for id, c := range cases {
		v := load(t, id)
		if v.JavaMajor() != c.java || v.JavaComponent() != c.comp || v.AssetsID() != c.assets {
			t.Errorf("%s: java %d %s assets %s", id, v.JavaMajor(), v.JavaComponent(), v.AssetsID())
		}
	}
}

func TestModernArguments(t *testing.T) {
	v := load(t, "26.3")
	var unknown []string
	game := v.GameArgs(linux, vars(), func(n string) { unknown = append(unknown, n) })
	if !slices.Contains(game, "--accessToken") || slices.Contains(game, "--demo") || slices.Contains(game, "--width") {
		t.Fatalf("game args = %q", game)
	}
	if len(unknown) != 0 {
		t.Fatalf("unknown placeholders: %v", unknown)
	}
	env := linux
	env.Features = map[string]bool{"has_custom_resolution": true}
	if game := v.GameArgs(env, vars(), nil); !slices.Contains(game, "1280") {
		t.Fatal("custom resolution feature ignored")
	}
	jvm := v.JVMArgs(linux, vars(), nil)
	if slices.Contains(jvm, "-XstartOnFirstThread") || !slices.Contains(jvm, "-Djava.library.path=/n/java") || jvm[len(jvm)-1] != "/cp" {
		t.Fatalf("jvm args = %q", jvm)
	}
	if jvm := v.JVMArgs(mac, vars(), nil); !slices.Contains(jvm, "-XstartOnFirstThread") {
		t.Fatal("macOS rule ignored")
	}
}

func TestLegacyArguments(t *testing.T) {
	v := load(t, "1.6.4")
	game := v.GameArgs(linux, vars(), nil)
	want := []string{"--username", "Steve", "--session", "TOKEN", "--version", "x", "--gameDir", "/g", "--assetsDir", "/a/virtual/legacy"}
	if !slices.Equal(game, want) {
		t.Fatalf("game = %q", game)
	}
	jvm := v.JVMArgs(windows, vars(), nil)
	if !slices.Contains(jvm, "-Djava.library.path=/n") || !strings.HasPrefix(jvm[0], "-XX:HeapDumpPath") {
		t.Fatalf("legacy jvm = %q", jvm)
	}
}

func TestUnknownPlaceholderNeverPanics(t *testing.T) {
	var got []string
	out := Vars{"a": "1"}.Expand("${a}-${brand_new_thing}-${a}", func(n string) { got = append(got, n) })
	if out != "1--1" || !slices.Equal(got, []string{"brand_new_thing"}) {
		t.Fatalf("out=%q unknown=%v", out, got)
	}
}

func TestRules(t *testing.T) {
	zgc := []Rule{{Action: "allow", OS: &OSRule{Name: "windows", VersionRange: &VersionRange{Min: "10.0.17134"}}}}
	g1 := []Rule{{Action: "allow", OS: &OSRule{Name: "windows", VersionRange: &VersionRange{Max: "10.0.17134"}}}}
	old := windows
	old.OSVersion = "10.0.16299"
	if !Allowed(zgc, windows) || Allowed(g1, windows) || Allowed(zgc, old) || !Allowed(g1, old) {
		t.Fatal("versionRange wrong")
	}
	notMac := []Rule{{Action: "allow"}, {Action: "disallow", OS: &OSRule{Name: "osx"}}}
	if !Allowed(notMac, linux) || Allowed(notMac, mac) {
		t.Fatal("last matching rule must win")
	}
	if Allowed([]Rule{{Action: "allow", OS: &OSRule{Name: "windows", Version: "^10\\."}}}, Env{OS: "windows", OSVersion: "6.1"}) {
		t.Fatal("version regex ignored")
	}
	if Allowed([]Rule{{Action: "allow", OS: &OSRule{Version: "(bad"}}}, windows) {
		t.Fatal("invalid regex must not match")
	}
}

func TestLibrariesModernAndLegacyNatives(t *testing.T) {
	libs, err := load(t, "26.3").ResolveLibraries(linux)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, l := range libs {
		if l.Native {
			t.Fatalf("modern version produced a legacy native: %+v", l)
		}
		paths = append(paths, l.Path)
	}
	joined := strings.Join(paths, "\n")
	if !strings.Contains(joined, "natives-linux") || strings.Contains(joined, "natives-macos") || strings.Contains(joined, "java-objc-bridge") {
		t.Fatal("platform filtering wrong")
	}

	libs, err = load(t, "1.12.2").ResolveLibraries(windows)
	if err != nil {
		t.Fatal(err)
	}
	var natives []ResolvedLibrary
	for _, l := range libs {
		if l.Native {
			natives = append(natives, l)
		}
	}
	if len(natives) == 0 || !strings.Contains(natives[0].Path, "natives-windows") || !slices.Contains(natives[0].Exclude, "META-INF/") {
		t.Fatalf("legacy natives = %+v", natives)
	}
}

func TestMergeAndDedup(t *testing.T) {
	parent := load(t, "1.21.4")
	child := &Version{
		ID: "fabric-loader-0.16.9-1.21.4", InheritsFrom: "1.21.4", MainClass: "net.fabricmc.loader.impl.launch.knot.KnotClient",
		Arguments: &Arguments{JVM: []Argument{{Value: []string{"-DFabricMcEmu= net.minecraft.client.main.Main "}}}},
		Libraries: []Library{
			{Name: "org.ow2.asm:asm:9.9", URL: "https://maven.fabricmc.net/"},
			{Name: "net.fabricmc:fabric-loader:0.16.9", URL: "https://maven.fabricmc.net/"},
		},
	}
	m := Merge(parent, child)
	if m.ID != child.ID || m.MainClass != child.MainClass || m.AssetsID() != "19" || m.JavaMajor() != 21 {
		t.Fatalf("merged scalars wrong: %+v", m)
	}
	if jvm := m.JVMArgs(linux, vars(), nil); jvm[len(jvm)-1] != "-DFabricMcEmu= net.minecraft.client.main.Main " {
		t.Fatal("child JVM args not appended")
	}
	libs, err := m.ResolveLibraries(linux)
	if err != nil {
		t.Fatal(err)
	}
	if libs[0].URL != "https://maven.fabricmc.net/org/ow2/asm/asm/9.9/asm-9.9.jar" {
		t.Fatalf("first lib = %+v", libs[0])
	}
	if parent.ID != "1.21.4" || len(parent.Arguments.JVM) == len(m.Arguments.JVM) {
		t.Fatal("Merge mutated the parent")
	}
}

func TestHostileLibraries(t *testing.T) {
	bad := []Library{
		{Name: "../../evil:x:1"},
		{Name: "a:b:1", Downloads: &LibraryDownloads{Artifact: &Artifact{Path: "../../../.ssh/id_rsa", URL: "https://x"}}},
		{Name: "a:b:1", Downloads: &LibraryDownloads{Artifact: &Artifact{Path: "/etc/passwd"}}},
		{Name: "a:b:1", Downloads: &LibraryDownloads{Artifact: &Artifact{Path: "a/b.jar", SHA1: "nothex"}}},
		{Name: "a:b:1@../../x"},
	}
	for _, lib := range bad {
		v := &Version{Libraries: []Library{lib}}
		if _, err := v.ResolveLibraries(linux); err == nil {
			t.Errorf("accepted %+v", lib)
		}
	}
}

func TestAssetIndex(t *testing.T) {
	var ix AssetIndex
	data := `{"virtual":true,"objects":{"sound/step/grass1.ogg":{"hash":"227ab99bf7c6cf0b2002e0f7957d0ff7e5cb0c96","size":6407},"../../x":{"hash":"zz","size":1}}}`
	if err := json.Unmarshal([]byte(data), &ix); err != nil {
		t.Fatal(err)
	}
	o := ix.Objects["sound/step/grass1.ogg"]
	if !ix.Legacy() || !o.Valid() || o.ObjectPath() != "22/227ab99bf7c6cf0b2002e0f7957d0ff7e5cb0c96" {
		t.Fatal("asset object wrong")
	}
	if ix.Objects["../../x"].Valid() || ValidAssetName("../../x") || ValidAssetName("/abs") || !ValidAssetName("sound/step/grass1.ogg") {
		t.Fatal("hostile asset accepted")
	}
}

func TestRuntimeIndex(t *testing.T) {
	data, err := os.ReadFile("testdata/runtimes.json")
	if err != nil {
		t.Fatal(err)
	}
	var ix RuntimeIndex
	if err := json.Unmarshal(data, &ix); err != nil {
		t.Fatal(err)
	}
	if b, ok := ix.Build(RuntimePlatform("linux", "amd64"), "java-runtime-delta"); !ok || !strings.HasPrefix(b.Version.Name, "21") {
		t.Fatalf("linux delta = %+v", b)
	}
	if _, ok := ix.Build(RuntimePlatform("darwin", "arm64"), "jre-legacy"); !ok {
		t.Fatal("Apple silicon must fall back to the Intel runtime")
	}
	if RuntimePlatform("linux", "arm64") != "" {
		t.Fatal("Mojang publishes no linux arm64 runtime")
	}
}

func TestFamilies(t *testing.T) {
	data, err := os.ReadFile("testdata/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	fams := Families(m, ManifestEntry.EstimatedJava)
	if fams[0].ID != "26.x" || fams[0].Note != "Newest" || fams[0].Releases[0].ID != m.Latest.Release || fams[0].Releases[0].Java != 25 {
		t.Fatalf("first family = %+v", fams[0])
	}
	if !slices.ContainsFunc(fams[0].Snapshots, func(r Release) bool { return r.ID == m.Latest.Snapshot }) {
		t.Fatal("named snapshot not in its family")
	}
	var f121 *Family
	for i := range fams {
		if fams[i].ID == "1.21" {
			f121 = &fams[i]
		}
		for _, r := range fams[i].Releases {
			if r.ID == "1.5.2" {
				t.Fatal("unsupported version listed")
			}
		}
	}
	if last := fams[len(fams)-1]; last.ID != "1.6" || last.Releases[len(last.Releases)-1].ID != "1.6.1" {
		t.Fatalf("the oldest family must end at 1.6.1: %+v", last)
	}
	if f121 == nil || f121.Note != "2024 to 2025" || !slices.ContainsFunc(f121.Snapshots, func(r Release) bool { return r.ID == "25w14a" || strings.HasPrefix(r.ID, "1.21") }) {
		t.Fatalf("1.21 family = %+v", f121)
	}
	e, _ := m.Find("24w14a")
	if e.EstimatedJava() != 21 {
		t.Fatal("24w14a needs Java 21")
	}
}

func TestMergeLegacyForgeProfile(t *testing.T) {
	parent := load(t, "1.12.2")
	var lwjgl string
	for _, l := range parent.Libraries {
		if strings.HasPrefix(l.Name, "org.lwjgl.lwjgl:lwjgl:") {
			lwjgl = l.Name
		}
	}
	if lwjgl == "" {
		t.Fatal("fixture has no lwjgl")
	}
	child := &Version{
		ID: "1.12.2-forge-14.23.5.2860", InheritsFrom: "1.12.2", MainClass: "net.minecraft.launchwrapper.Launch",
		MinecraftArguments: "--username ${auth_player_name} --tweakClass net.minecraftforge.fml.common.launcher.FMLTweaker",
		Libraries: []Library{
			{Name: "net.minecraftforge:forge:1.12.2-14.23.5.2860"},
			{Name: lwjgl}, // listed again, bare
		},
	}
	m := Merge(parent, child)
	if m.MinecraftArguments != child.MinecraftArguments {
		t.Fatal("legacy arguments must replace the parent's")
	}
	libs, err := m.ResolveLibraries(linux)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range libs {
		if l.Name == lwjgl && l.SHA1 == "" {
			t.Fatalf("bare relisted library lost Mojang's checksum: %+v", l)
		}
	}
	if libs[0].Name != "net.minecraftforge:forge:1.12.2-14.23.5.2860" {
		t.Fatalf("child library order lost: %+v", libs[0])
	}
}
