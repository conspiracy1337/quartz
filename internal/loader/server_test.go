package loader

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"quartz/internal/mojang"
)

func TestFabricServerProfile(t *testing.T) {
	f := newFixture(t)
	f.web.put("https://meta.quiltmc.org/v3/versions/loader/1.21.1/0.27.0/server/json",
		[]byte(`{"id":"quilt-loader-0.27.0-1.21.1","inheritsFrom":"1.21.1","mainClass":"org.quiltmc.loader.impl.launch.knot.KnotServer","libraries":[]}`))
	p, err := New(f.deps).Profile(context.Background(), Quilt, Request{Minecraft: "1.21.1", Server: true}, "0.27.0")
	if err != nil {
		t.Fatal(err)
	}
	if p.Version.MainClass != "org.quiltmc.loader.impl.launch.knot.KnotServer" || p.Server == nil || p.Server.GameJarProperty != "loader.gameJarPath" {
		t.Fatalf("server profile = %+v %+v", p.Version, p.Server)
	}
}

func TestArgsFile(t *testing.T) {
	libs := filepath.Join(string(filepath.Separator)+"q", "libraries")
	abs := filepath.ToSlash(libs)
	got, err := argsFile([]byte("-p libraries/a.jar:libraries/b.jar\n--add-modules ALL-MODULE-PATH\n-DlibraryDirectory=libraries\n"+
		"-DlegacyClassPath=libraries/c.jar;libraries/d.jar\n-DignoreList=a.jar,b.jar\nmain.Class\n--fml.mcVersion 1.21.1\n"), libs)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-p", abs + "/a.jar:" + abs + "/b.jar", "--add-modules", "ALL-MODULE-PATH", "-DlibraryDirectory=" + abs,
		"-DlegacyClassPath=" + abs + "/c.jar;" + abs + "/d.jar", "-DignoreList=a.jar,b.jar", "main.Class", "--fml.mcVersion", "1.21.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	for _, bad := range []string{`-p "libraries/a b.jar"`, "-p libraries/../../evil.jar", "   "} {
		if _, err := argsFile([]byte(bad), libs); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

// serverForge builds a Forge installer with both sides' data, an argument
// file when args is set, and a jar the server starts with when path is set.
func serverForge(t *testing.T, args string, path bool) *forgeSetup {
	t.Helper()
	f := newFixture(t)
	tool := zipOf(t, map[string][]byte{"META-INF/MANIFEST.MF": []byte("Manifest-Version: 1.0\r\nMain-Class: net.minecraftforge.Fake\r\n")})
	f.web.put(forgeRepo+"net/minecraftforge/tool/1.0/tool-1.0.jar", tool)
	prof := map[string]any{
		"minecraft": "1.20.1", "json": "/version.json",
		"data": map[string]any{
			"PATCHED":  map[string]string{"client": "[net.minecraftforge:forge:1.20.1-47.3.0:client]", "server": "[net.minecraftforge:forge:1.20.1-47.3.0:server]"},
			"BINPATCH": map[string]string{"client": "/data/client.lzma", "server": "/data/server.lzma"},
			"MOJMAPS":  map[string]string{"client": "[net.minecraft:client:1.20.1:mappings@txt]", "server": "[net.minecraft:server:1.20.1:mappings@txt]"},
		},
		"processors": []map[string]any{
			{"sides": []string{"server"}, "jar": "net.minecraftforge:tool:1.0", "args": []string{"--task", "EXTRACT_FILES", "--to", "{ROOT}/run.sh"}},
			{"sides": []string{"server"}, "jar": "net.minecraftforge:tool:1.0", "args": []string{"--task", "BUNDLER_EXTRACT", "--input", "{MINECRAFT_JAR}", "--side", "{SIDE}"}},
			{"sides": []string{"client"}, "jar": "net.minecraftforge:tool:1.0", "args": []string{"--client-only"}},
			{"jar": "net.minecraftforge:tool:1.0", "args": []string{"--task", "DOWNLOAD_MOJMAPS", "--output", "{MOJMAPS}"}},
			{"jar": "net.minecraftforge:tool:1.0", "args": []string{"--apply", "{BINPATCH}", "--output", "{PATCHED}"}},
		},
		"libraries": []map[string]any{{"name": "net.minecraftforge:tool:1.0", "downloads": map[string]any{"artifact": map[string]any{
			"path": "net/minecraftforge/tool/1.0/tool-1.0.jar", "url": forgeRepo + "net/minecraftforge/tool/1.0/tool-1.0.jar", "sha1": sum(tool), "size": len(tool)}}}},
	}
	files := map[string][]byte{
		"version.json":      []byte(`{"id":"1.20.1-forge-47.3.0","inheritsFrom":"1.20.1","mainClass":"x","libraries":[{"name":"org.example:lib:1","downloads":{"artifact":{"path":"org/example/lib/1/lib-1.jar","url":"https://example.org/lib-1.jar"}}}]}`),
		"data/client.lzma":  []byte("client patch"),
		"data/server.lzma":  []byte("server patch"),
		"maven/net/x/y.jar": []byte("embedded"),
	}
	if args != "" {
		files["data/unix_args.txt"], files["data/win_args.txt"] = []byte(args), []byte(args)
	}
	if path {
		prof["path"] = "net.minecraftforge:forge:1.20.1-47.3.0"
		files["maven/net/minecraftforge/forge/1.20.1-47.3.0/forge-1.20.1-47.3.0.jar"] = zipOf(t, map[string][]byte{"META-INF/MANIFEST.MF": []byte(
			"Manifest-Version: 1.0\r\nClass-Path: libraries/org/example/lib/1/lib-1.jar libraries/net/x/y.jar\r\n  minecraft_server.1.20.1.jar\r\nMain-Class: net.minecraftforge.server.ServerMain\r\n")})
	}
	raw, _ := json.Marshal(prof)
	files["install_profile.json"] = raw
	installer := zipOf(t, files)
	url := forgeRepo + "net/minecraftforge/forge/1.20.1-47.3.0/forge-1.20.1-47.3.0-installer.jar"
	f.web.put(url, installer)
	f.web.put(url+".sha1", []byte(sum(installer)))
	parent := &mojang.Version{ID: "1.20.1", Downloads: map[string]mojang.Artifact{
		"client_mappings": {URL: "https://piston-data.mojang.com/client.txt", SHA1: sum([]byte("c")), Size: 1},
		"server_mappings": {URL: "https://piston-data.mojang.com/server.txt", SHA1: sum([]byte("s")), Size: 1},
	}}
	return &forgeSetup{fixture: f, req: Request{Minecraft: "1.20.1", Parent: parent,
		ClientJar: filepath.Join(f.root, "store", "versions", "1.20.1", "1.20.1.jar"),
		ServerJar: filepath.Join(f.deps.Dirs.Libraries, "net", "minecraft", "server", "1.20.1", "server-1.20.1.jar")}}
}

func TestForgeServerProcessing(t *testing.T) {
	s := serverForge(t, "-p libraries/a.jar\nmain.Class\n--launchTarget forgeserver\n", false)
	ctx := context.Background()
	// The client is prepared first, as it is when the instance was played.
	if _, err := New(s.deps).Profile(ctx, Forge, s.req, "1.20.1-47.3.0"); err != nil {
		t.Fatal(err)
	}
	req := s.req
	req.Server = true
	p, err := New(s.deps).Profile(ctx, Forge, req, "1.20.1-47.3.0")
	if err != nil {
		t.Fatal(err)
	}
	if p.Processing == nil || p.Processing.Steps() != 2 {
		t.Fatalf("want the bundler and patch steps only (EXTRACT_FILES, client-only and mojmaps dropped): %+v", p.Processing)
	}
	bundler := p.Processing.steps[0].args
	if bundler[3] != req.ServerJar || bundler[5] != "server" {
		t.Fatalf("server values: %q", bundler)
	}
	patch := p.Processing.steps[1].args
	if !strings.HasSuffix(patch[1], filepath.Join("data", "server.lzma")) || !strings.HasSuffix(patch[3], "forge-1.20.1-47.3.0-server.jar") {
		t.Fatalf("server data: %q", patch)
	}
	if b, err := os.ReadFile(patch[1]); err != nil || string(b) != "server patch" {
		t.Fatalf("the server's data file is unpacked on demand: %q %v", b, err)
	}
	var mappings bool
	for _, l := range p.Extra {
		mappings = mappings || l.URL == "https://piston-data.mojang.com/server.txt"
	}
	if !mappings {
		t.Fatal("the server takes the server mappings")
	}
	if p.Processing.marker == filepath.Join(filepath.Dir(p.Processing.marker), ".processed") {
		t.Fatal("the server's processing has its own marker")
	}
	libs := filepath.ToSlash(s.deps.Dirs.Libraries)
	if want := []string{"-p", libs + "/a.jar", "main.Class", "--launchTarget", "forgeserver"}; p.Server == nil || !reflect.DeepEqual(p.Server.Argv, want) {
		t.Fatalf("argv %+v", p.Server)
	}
}

func TestForgeServerJarStart(t *testing.T) {
	s := serverForge(t, "", true)
	req := s.req
	req.Server = true
	p, err := New(s.deps).Profile(context.Background(), Forge, req, "1.20.1-47.3.0")
	if err != nil {
		t.Fatal(err)
	}
	st := p.Server
	if st == nil || !reflect.DeepEqual(st.Argv, []string{"-jar", "forge-1.20.1-47.3.0.jar"}) {
		t.Fatalf("start %+v", st)
	}
	lib := func(rel string) string { return filepath.Join(s.deps.Dirs.Libraries, filepath.FromSlash(rel)) }
	for rel, src := range map[string]string{
		"forge-1.20.1-47.3.0.jar":                            lib("net/minecraftforge/forge/1.20.1-47.3.0/forge-1.20.1-47.3.0.jar"),
		"libraries/net/x/y.jar":                              lib("net/x/y.jar"),
		"libraries/org/example/lib/1/lib-1.jar":              lib("org/example/lib/1/lib-1.jar"),
		"minecraft_server.1.20.1.jar":                        req.ServerJar,
		"libraries/net/minecraftforge/tool/1.0/tool-1.0.jar": lib("net/minecraftforge/tool/1.0/tool-1.0.jar"),
		"libraries/net/minecraftforge/forge/1.20.1-47.3.0/forge-1.20.1-47.3.0-server.jar": lib("net/minecraftforge/forge/1.20.1-47.3.0/forge-1.20.1-47.3.0-server.jar"),
	} {
		if st.Place[rel] != src {
			t.Errorf("place %s = %q, want %q", rel, st.Place[rel], src)
		}
	}
}

func TestLegacyForgeServer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("paths in the fixture are slash-separated")
	}
	f := newFixture(t)
	info := `{"id":"1.7.10-Forge","inheritsFrom":"1.7.10","mainClass":"net.minecraft.launchwrapper.Launch","libraries":[
		{"name":"net.minecraftforge:forge:1.7.10-10.13.4.1614-1.7.10","url":"https://maven.minecraftforge.net/"},
		{"name":"net.minecraft:launchwrapper:1.12","serverreq":true},
		{"name":"org.lwjgl:lwjgl:2.9.1"},
		{"name":"client:only:1","clientreq":true}]}`
	universal := zipOf(t, map[string][]byte{"META-INF/MANIFEST.MF": []byte("Manifest-Version: 1.0\r\nClass-Path: libraries/net/minecraft/launchwrapper/1.12/launchwrapper-1.12.jar minecraft_server.1.7.10.jar\r\n")})
	profile := `{"install":{"path":"net.minecraftforge:forge:1.7.10-10.13.4.1614-1.7.10","filePath":"forge-universal.jar","minecraft":"1.7.10"},"versionInfo":` + info + `}`
	installer := zipOf(t, map[string][]byte{"install_profile.json": []byte(profile), "forge-universal.jar": universal})
	url := forgeRepo + "net/minecraftforge/forge/1.7.10-10.13.4.1614-1.7.10/forge-1.7.10-10.13.4.1614-1.7.10-installer.jar"
	f.web.put(url, installer)
	f.web.put(url+".sha1", []byte(sum(installer)))
	jar := filepath.Join(f.deps.Dirs.Libraries, "net/minecraft/server/1.7.10/server-1.7.10.jar")
	p, err := New(f.deps).Profile(context.Background(), Forge, Request{Minecraft: "1.7.10", Server: true, ServerJar: jar}, "1.7.10-10.13.4.1614-1.7.10")
	if err != nil {
		t.Fatal(err)
	}
	libs, _ := p.Version.ResolveLibraries(mojang.Env{OS: "linux", Arch: "x86_64"})
	if len(libs) != 2 {
		t.Fatalf("a server takes only serverreq libraries and Forge itself: %+v", libs)
	}
	if p.Server == nil || p.Server.Argv[1] != "forge-1.7.10-10.13.4.1614-1.7.10.jar" || p.Server.Place["minecraft_server.1.7.10.jar"] != jar ||
		p.Server.Place["libraries/net/minecraft/launchwrapper/1.12/launchwrapper-1.12.jar"] == "" {
		t.Fatalf("start %+v", p.Server)
	}
}

func TestManifestContinuation(t *testing.T) {
	jar := filepath.Join(t.TempDir(), "x.jar")
	if err := os.WriteFile(jar, zipOf(t, map[string][]byte{"META-INF/MANIFEST.MF": []byte("Manifest-Version: 1.0\r\nClass-Path: libraries/a\r\n .jar libraries/b.jar\r\nMain-Class: M\r\n")}), 0o600); err != nil {
		t.Fatal(err)
	}
	if cp, err := manifestAttr(jar, "Class-Path"); err != nil || cp != "libraries/a.jar libraries/b.jar" {
		t.Fatalf("%q %v", cp, err)
	}
	if m, err := mainClass(jar); err != nil || m != "M" {
		t.Fatalf("%q %v", m, err)
	}
}
