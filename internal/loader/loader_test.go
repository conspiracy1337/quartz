package loader

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // test fixtures use the same checksums as maven
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"quartz/internal/core/download"
	"quartz/internal/core/httpcache"
	"quartz/internal/meta"
	"quartz/internal/mojang"
)

// fakeWeb answers requests from a map of URL to body; anything else is a 404.
type fakeWeb struct {
	mu    sync.Mutex
	files map[string][]byte
	hits  map[string]int
}

func (w *fakeWeb) put(url string, body []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.files[url] = body
}

func (w *fakeWeb) Do(r *http.Request) (*http.Response, error) {
	w.mu.Lock()
	body, ok := w.files[r.URL.String()]
	w.hits[r.URL.String()]++
	w.mu.Unlock()
	if !ok {
		return &http.Response{StatusCode: 404, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	}
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: r}, nil
}

type fixture struct {
	web  *fakeWeb
	deps Deps
	root string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	web := &fakeWeb{files: map[string][]byte{}, hits: map[string]int{}}
	root := t.TempDir()
	log := slog.New(slog.DiscardHandler)
	cache, err := httpcache.Open(filepath.Join(root, "cache"), web, log)
	if err != nil {
		t.Fatal(err)
	}
	ver, err := download.OpenVerified(filepath.Join(root, "verified.json"), root, log)
	if err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(root, "store")
	return &fixture{web: web, root: root, deps: Deps{
		Meta: meta.New(cache), Downloader: download.New(web, ver), Verified: ver, Log: log,
		Dirs: Dirs{Libraries: filepath.Join(store, "libraries"), Versions: filepath.Join(store, "versions")},
	}}
}

func sum(b []byte) string {
	h := sha1.Sum(b) //nolint:gosec // matches maven's checksums
	return hex.EncodeToString(h[:])
}

func TestPrerelease(t *testing.T) {
	if !prerelease("0.27.0-beta.1") || prerelease("0.16.10") || !prerelease("26.3.0.75-beta") {
		t.Fatal("prerelease detection")
	}
}

func TestNeoForgeMapping(t *testing.T) {
	cases := []struct {
		id, mc string
		want   bool
	}{
		{"21.1.77", "1.21.1", true},
		{"21.10.3", "1.21.1", false}, // numeric, not a string prefix
		{"21.0.167", "1.21", true},
		{"20.4.237", "1.20.4", true},
		{"26.3.0.76", "26.3", true},
		{"26.3.0.75-beta", "26.3", true},
		{"26.1.2.4", "26.1.2", true},
		{"26.1.0.4", "26.1.2", false},
		{"21.1.77", "25w14a", false},
	}
	for _, c := range cases {
		if got := neoForgeFor(c.id, c.mc); got != c.want {
			t.Errorf("neoForgeFor(%s, %s) = %v", c.id, c.mc, got)
		}
	}
	if neoForgeArtifact("1.20.1-47.1.106") != "net.neoforged:forge" || neoForgeArtifact("21.1.77") != "net.neoforged:neoforge" {
		t.Fatal("artifact mapping")
	}
}

func TestFabricAndQuiltVersions(t *testing.T) {
	f := newFixture(t)
	// Fabric flags only its newest stable build; the older ones are stable too.
	f.web.put("https://meta.fabricmc.net/v2/versions/loader/1.21.1", []byte(`[
		{"loader":{"version":"0.16.9","stable":false}},{"loader":{"version":"0.16.10","stable":true}},
		{"loader":{"version":"0.17.0-beta.1","stable":false}},{"loader":{"version":"../evil","stable":true}}]`))
	f.web.put("https://meta.quiltmc.org/v3/versions/loader/1.21.1", []byte(`[{"loader":{"version":"0.27.0"}},{"loader":{"version":"0.28.0-beta.2"}}]`))
	r := New(f.deps)
	ctx := context.Background()
	vs, err := r.Versions(ctx, Fabric, "1.21.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 3 || vs[0].ID != "0.17.0-beta.1" || vs[0].Stable || !vs[2].Stable {
		t.Fatalf("fabric versions = %+v", vs)
	}
	if v, _ := r.Latest(ctx, Fabric, "1.21.1"); v.ID != "0.16.10" {
		t.Fatalf("latest stable = %+v", v)
	}
	q, err := r.Versions(ctx, Quilt, "1.21.1")
	if err != nil || q[0].ID != "0.28.0-beta.2" || q[0].Stable || !q[1].Stable {
		t.Fatalf("quilt versions = %+v, %v", q, err)
	}
	if _, err := r.Latest(ctx, Fabric, "1.13.2"); !errors.Is(err, ErrNoBuild) && err == nil {
		t.Fatalf("no build: %v", err)
	}
	if _, err := ParseKind("liteloader"); !errors.Is(err, ErrUnknownKind) {
		t.Fatal("unknown kinds must be refused")
	}
}

func TestFabricProfileIsChecked(t *testing.T) {
	f := newFixture(t)
	base := "https://meta.fabricmc.net/v2/versions/loader/1.21.1/0.16.10/profile/json"
	f.web.put(base, []byte(`{"id":"fabric-loader-0.16.10-1.21.1","inheritsFrom":"1.21.1","mainClass":"net.fabricmc.loader.impl.launch.knot.KnotClient","libraries":[]}`))
	p, err := New(f.deps).Profile(context.Background(), Fabric, Request{Minecraft: "1.21.1"}, "0.16.10")
	if err != nil || p.Version.MainClass == "" || p.Processing != nil {
		t.Fatalf("profile = %+v, %v", p, err)
	}
	f.web.put(strings.Replace(base, "0.16.10", "0.16.11", 1), []byte(`{"id":"fabric-loader-0.16.11-1.20.4","inheritsFrom":"1.20.4"}`))
	if _, err := New(f.deps).Profile(context.Background(), Fabric, Request{Minecraft: "1.21.1"}, "0.16.11"); err == nil {
		t.Fatal("a profile for another game version must be refused")
	}
}

func TestForgeVersions(t *testing.T) {
	f := newFixture(t)
	f.web.put(forgeRepo+"net/minecraftforge/forge/maven-metadata.xml", []byte(`<metadata><versioning><versions>
		<version>1.20.1-47.3.0</version><version>1.20.1-47.4.10</version><version>1.20.1-47.4.26</version>
		<version>1.7.10-10.13.4.1614-1.7.10</version><version>1.7.10_pre4-10.12.2.1139-prerelease</version>
	</versions></versioning></metadata>`))
	f.web.put("https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json", []byte(`{"promos":{"1.20.1-recommended":"47.4.10","1.20.1-latest":"47.4.26","1.7.10-latest":"10.13.4.1614"}}`))
	r := New(f.deps)
	vs, err := r.Versions(context.Background(), Forge, "1.20.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 3 || vs[0].Name != "47.4.26" || vs[0].Stable || !vs[1].Stable || vs[1].ID != "1.20.1-47.4.10" {
		t.Fatalf("forge 1.20.1 = %+v", vs)
	}
	old, _ := r.Versions(context.Background(), Forge, "1.7.10")
	if len(old) != 1 || old[0].Name != "10.13.4.1614" || old[0].ID != "1.7.10-10.13.4.1614-1.7.10" {
		t.Fatalf("forge 1.7.10 = %+v", old)
	}
}

// --- the installer -------------------------------------------------------

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
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

// The fake processor writes "patched:<input>" to its --output and exits.
const patched = "patched!"

func TestMain(m *testing.M) {
	if os.Getenv("QUARTZ_FAKE_JAVA") == "1" {
		fakeJava()
		return
	}
	os.Exit(m.Run())
}

func fakeJava() {
	args := os.Args[1:]
	if len(args) < 3 || args[0] != "-cp" || args[2] != "net.minecraftforge.Fake" {
		os.Exit(3)
	}
	if out := argAfter(args, "--fail"); out != "" {
		_, _ = os.Stderr.WriteString("Exception in processor: " + out + "\n")
		os.Exit(1)
	}
	if out := argAfter(args, "--output"); out != "" {
		_ = os.MkdirAll(filepath.Dir(out), 0o700)
		_ = os.WriteFile(out, []byte(patched), 0o600)
	}
	if n := argAfter(args, "--count"); n != "" {
		f, _ := os.OpenFile(n, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		_, _ = f.WriteString("x")
		_ = f.Close()
	}
}

type forgeSetup struct {
	*fixture
	req   Request
	count string
}

func setupForge(t *testing.T, processorArgs []string, outputs map[string]string) *forgeSetup {
	t.Helper()
	f := newFixture(t)
	tool := zipOf(t, map[string][]byte{"META-INF/MANIFEST.MF": []byte("Manifest-Version: 1.0\r\nMain-Class: net.minecraftforge.Fake\r\n")})
	f.web.put(forgeRepo+"net/minecraftforge/tool/1.0/tool-1.0.jar", tool)
	profile, _ := json.Marshal(map[string]any{
		"minecraft": "1.20.1", "json": "/version.json",
		"data": map[string]any{
			"PATCHED":     map[string]string{"client": "[net.minecraftforge:forge:1.20.1-47.3.0:client]"},
			"PATCHED_SHA": map[string]string{"client": "'" + sum([]byte(patched)) + "'"},
			"BINPATCH":    map[string]string{"client": "/data/client.lzma"},
			"MOJMAPS":     map[string]string{"client": "[net.minecraft:client:1.20.1:mappings@txt]"},
		},
		"processors": []map[string]any{
			{"sides": []string{"server"}, "jar": "net.minecraftforge:tool:1.0", "args": []string{"--server-only"}},
			{"jar": "net.minecraftforge:tool:1.0", "args": []string{"--task", "DOWNLOAD_MOJMAPS", "--output", "{MOJMAPS}"}},
			{"jar": "net.minecraftforge:tool:1.0", "classpath": []string{}, "args": processorArgs, "outputs": outputs},
		},
		"libraries": []map[string]any{{"name": "net.minecraftforge:tool:1.0", "downloads": map[string]any{"artifact": map[string]any{
			"path": "net/minecraftforge/tool/1.0/tool-1.0.jar", "url": forgeRepo + "net/minecraftforge/tool/1.0/tool-1.0.jar", "sha1": sum(tool), "size": len(tool)}}}},
	})
	installer := zipOf(t, map[string][]byte{
		"install_profile.json": profile,
		"version.json":         []byte(`{"id":"1.20.1-forge-47.3.0","inheritsFrom":"1.20.1","mainClass":"cpw.mods.bootstraplauncher.BootstrapLauncher","libraries":[]}`),
		"maven/net/minecraftforge/forge/1.20.1-47.3.0/forge-1.20.1-47.3.0-universal.jar": []byte("universal"),
		"data/client.lzma": []byte("binpatch"),
	})
	url := forgeRepo + "net/minecraftforge/forge/1.20.1-47.3.0/forge-1.20.1-47.3.0-installer.jar"
	f.web.put(url, installer)
	f.web.put(url+".sha1", []byte(sum(installer)))
	mappings := []byte("mappings!")
	f.web.put("https://piston-data.mojang.com/mappings.txt", mappings)
	parent := &mojang.Version{ID: "1.20.1", Downloads: map[string]mojang.Artifact{"client_mappings": {URL: "https://piston-data.mojang.com/mappings.txt", SHA1: sum(mappings), Size: int64(len(mappings))}}}
	return &forgeSetup{fixture: f, count: filepath.Join(f.root, "runs"), req: Request{Minecraft: "1.20.1", Parent: parent, ClientJar: filepath.Join(f.root, "store", "versions", "1.20.1", "1.20.1.jar")}}
}

func fakeJavaPath(t *testing.T) string {
	t.Helper()
	t.Setenv("QUARTZ_FAKE_JAVA", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

// fetchExtras stands in for the install job's download step.
func fetchExtras(t *testing.T, s *forgeSetup, p *Profile) {
	t.Helper()
	for _, l := range p.Extra {
		if l.URL == "" {
			continue
		}
		it := download.Item{URLs: []string{l.URL}, Dest: filepath.Join(s.deps.Dirs.Libraries, filepath.FromSlash(l.Path)), Size: l.Size}
		if err := s.deps.Downloader.Fetch(context.Background(), it, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestForgeInstallerProcessesOnceAndSkips(t *testing.T) {
	java := fakeJavaPath(t)
	s := setupForge(t, []string{"--input", "{MINECRAFT_JAR}", "--apply", "{BINPATCH}", "--output", "{PATCHED}", "--count", "COUNT"}, map[string]string{"{PATCHED}": "{PATCHED_SHA}"})
	s.count = filepath.Join(s.root, "count")
	// Point --count at a real file (args are expanded, so use a literal path).
	r := New(s.deps)
	ctx := context.Background()
	p, err := r.Profile(ctx, Forge, s.req, "1.20.1-47.3.0")
	if err != nil {
		t.Fatal(err)
	}
	if p.Processing == nil || p.Processing.Steps() != 1 || !p.NoClientJar {
		t.Fatalf("server-only and mojmaps steps must be dropped: %+v", p.Processing)
	}
	args := p.Processing.steps[0].args
	if args[1] != s.req.ClientJar || !strings.HasSuffix(args[3], filepath.Join("versions", "forge-1.20.1-47.3.0", "data", "client.lzma")) {
		t.Fatalf("built-in values must pass through as they are, data files resolve into the build folder: %q", args)
	}
	var mappings bool
	for _, l := range p.Extra {
		mappings = mappings || l.URL == "https://piston-data.mojang.com/mappings.txt"
	}
	if !mappings {
		t.Fatal("DOWNLOAD_MOJMAPS must become a verified download")
	}
	if !p.Produced["net/minecraftforge/forge/1.20.1-47.3.0/forge-1.20.1-47.3.0-client.jar"] {
		t.Fatalf("processor output not marked produced: %v", p.Produced)
	}
	if _, err := os.Stat(filepath.Join(s.deps.Dirs.Libraries, "net/minecraftforge/forge/1.20.1-47.3.0/forge-1.20.1-47.3.0-universal.jar")); err != nil {
		t.Fatal("embedded maven files must be unpacked into the libraries")
	}
	p.Processing.steps[0].args[len(p.Processing.steps[0].args)-1] = s.count
	fetchExtras(t, s, p)
	if p.Processing.Done() {
		t.Fatal("done before running")
	}
	if err := p.Processing.Run(ctx, java); err != nil {
		t.Fatal(err)
	}
	if !p.Processing.Done() {
		t.Fatal("not done after a successful run")
	}

	// A second launch reads the build from disk: no installer download, and
	// the step is skipped because its pinned output still matches.
	hits := s.web.hits[forgeRepo+"net/minecraftforge/forge/1.20.1-47.3.0/forge-1.20.1-47.3.0-installer.jar"]
	p2, err := New(s.deps).Profile(ctx, Forge, s.req, "1.20.1-47.3.0")
	if err != nil {
		t.Fatal(err)
	}
	if s.web.hits[forgeRepo+"net/minecraftforge/forge/1.20.1-47.3.0/forge-1.20.1-47.3.0-installer.jar"] != hits {
		t.Fatal("prepared build downloaded its installer again")
	}
	if !p2.Processing.Done() {
		t.Fatal("prepared build must be done without running anything")
	}
	p2.Processing.steps[0].args[len(p2.Processing.steps[0].args)-1] = s.count
	if err := p2.Processing.Run(ctx, java); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(s.count); string(b) != "x" {
		t.Fatalf("a step whose output matches must be skipped (ran %d times)", len(b))
	}
}

func TestForgeProcessorFailuresAreReported(t *testing.T) {
	java := fakeJavaPath(t)
	s := setupForge(t, []string{"--fail", "mixin", "--output", "{PATCHED}"}, map[string]string{"{PATCHED}": "{PATCHED_SHA}"})
	p, err := New(s.deps).Profile(context.Background(), Forge, s.req, "1.20.1-47.3.0")
	if err != nil {
		t.Fatal(err)
	}
	fetchExtras(t, s, p)
	if err := p.Processing.Run(context.Background(), java); err == nil || !strings.Contains(err.Error(), "Exception in processor: mixin") {
		t.Fatalf("failure must carry the tool's output: %v", err)
	}
	if p.Processing.Done() {
		t.Fatal("a failed run must not be marked done")
	}
}

func TestForgeOutputMismatchFails(t *testing.T) {
	java := fakeJavaPath(t)
	s := setupForge(t, []string{"--output", "{PATCHED}"}, map[string]string{"{PATCHED}": "'" + strings.Repeat("b", 40) + "'"})
	p, err := New(s.deps).Profile(context.Background(), Forge, s.req, "1.20.1-47.3.0")
	if err != nil {
		t.Fatal(err)
	}
	fetchExtras(t, s, p)
	if err := p.Processing.Run(context.Background(), java); err == nil || !strings.Contains(err.Error(), "different") {
		t.Fatalf("mismatched output: %v", err)
	}
}

func TestForgeInstallerRefusesBadInput(t *testing.T) {
	for name, args := range map[string][]string{
		"unknown key": {"--output", "{NOPE}"},
		"traversal":   {"--output", "[../../evil:x:1]"},
	} {
		t.Run(name, func(t *testing.T) {
			s := setupForge(t, args, nil)
			if _, err := New(s.deps).Profile(context.Background(), Forge, s.req, "1.20.1-47.3.0"); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	s := setupForge(t, []string{"--output", "{PATCHED}"}, nil)
	req := s.req
	req.Minecraft = "1.20.4"
	if _, err := New(s.deps).Profile(context.Background(), Forge, req, "1.20.1-47.3.0"); err == nil {
		t.Fatal("a build for another game version must be refused")
	}
	// An installer without a published checksum is never run.
	s = setupForge(t, []string{"--output", "{PATCHED}"}, nil)
	delete(s.web.files, forgeRepo+"net/minecraftforge/forge/1.20.1-47.3.0/forge-1.20.1-47.3.0-installer.jar.sha1")
	if _, err := New(s.deps).Profile(context.Background(), Forge, s.req, "1.20.1-47.3.0"); err == nil {
		t.Fatal("installer without a checksum accepted")
	}
}

func TestProcessingTimeout(t *testing.T) {
	p := &Processing{Timeout: time.Millisecond}
	err := p.run(context.Background(), "/bin/sleep", t.TempDir(), step{name: "slow", jar: "/nonexistent.jar"})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestLegacyForgeProfile(t *testing.T) {
	f := newFixture(t)
	info := `{"id":"1.7.10-Forge10.13.4.1614-1.7.10","inheritsFrom":"1.7.10","mainClass":"net.minecraft.launchwrapper.Launch",
		"minecraftArguments":"--tweakClass cpw.mods.fml.common.launcher.FMLTweaker",
		"libraries":[
			{"name":"net.minecraftforge:forge:1.7.10-10.13.4.1614-1.7.10","url":"https://maven.minecraftforge.net/"},
			{"name":"net.minecraft:launchwrapper:1.12","serverreq":true},
			{"name":"org.scala-lang:scala-library:2.11.1","url":"http://files.minecraftforge.net/maven/","clientreq":true},
			{"name":"server:only:1","clientreq":false}]}`
	profile := `{"install":{"path":"net.minecraftforge:forge:1.7.10-10.13.4.1614-1.7.10","filePath":"forge-universal.jar","minecraft":"1.7.10"},"versionInfo":` + info + `}`
	installer := zipOf(t, map[string][]byte{"install_profile.json": []byte(profile), "forge-universal.jar": []byte("universal")})
	url := forgeRepo + "net/minecraftforge/forge/1.7.10-10.13.4.1614-1.7.10/forge-1.7.10-10.13.4.1614-1.7.10-installer.jar"
	f.web.put(url, installer)
	f.web.put(url+".sha1", []byte(sum(installer)))
	p, err := New(f.deps).Profile(context.Background(), Forge, Request{Minecraft: "1.7.10"}, "1.7.10-10.13.4.1614-1.7.10")
	if err != nil {
		t.Fatal(err)
	}
	if p.Processing != nil || p.NoClientJar {
		t.Fatal("legacy Forge has no processors")
	}
	libs, err := p.Version.ResolveLibraries(mojang.Env{OS: "linux", Arch: "x86_64"})
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 3 {
		t.Fatalf("client libraries = %+v", libs)
	}
	if libs[0].URL != "" {
		t.Fatalf("the universal jar comes from the installer, not a download: %+v", libs[0])
	}
	if got, _ := os.ReadFile(filepath.Join(f.deps.Dirs.Libraries, filepath.FromSlash(libs[0].Path))); string(got) != "universal" {
		t.Fatal("universal jar not placed")
	}
	scala := libs[2]
	if !strings.HasPrefix(scala.URL, forgeRepo) || len(scala.Mirrors) != 2 || !strings.HasPrefix(scala.Mirrors[0], mavenCentral) {
		t.Fatalf("legacy repository not rewritten with fallbacks: %+v", scala)
	}
	if libs[1].URL != mojang.DefaultLibraryRepo+"net/minecraft/launchwrapper/1.12/launchwrapper-1.12.jar" {
		t.Fatalf("library without a repository: %+v", libs[1])
	}
}
