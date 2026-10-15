package main

import (
	"archive/zip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"quartz/internal/core/minisign"
)

// goProgram builds a tiny Go program with the given tags into dir/name, for
// tests that need a real executable to inspect.
func goProgram(t *testing.T, dir, name, tags string) string {
	t.Helper()
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command")
	}
	src := t.TempDir()
	for f, body := range map[string]string{"go.mod": "module p\n\ngo 1.22\n", "main.go": "package main\n\nfunc main() {}\n"} {
		if err := os.WriteFile(filepath.Join(src, f), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, name)
	cmd := exec.Command(gobin, "build", "-trimpath", "-tags", tags, "-o", out, ".")
	cmd.Dir = src
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, b)
	}
	return out
}

func TestSealRoundTrip(t *testing.T) {
	_, sec, err := minisign.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	box, err := seal(sec, []byte("correct horse battery"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := unseal(box, []byte("correct horse battery"))
	if err != nil || got.ID != sec.ID || !got.Key.Equal(sec.Key) {
		t.Fatal("round trip", err)
	}
	if _, err := unseal(box, []byte("wrong passphrase!")); err == nil {
		t.Fatal("a wrong passphrase must fail")
	}
	if strings.Contains(string(box), string(sec.Key.Seed())) {
		t.Fatal("the seed must not be stored in the clear")
	}
}

func TestManifestCollectsAndExtends(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bin := goProgram(t, dir, "quartz-linux-amd64", "production")
	write("notes.md", "not a release file")
	if err := stamp(dir, "quartz-linux-amd64", "1.2.3"); err != nil {
		t.Fatal(err)
	}
	m, _ := readManifest(dir, "1.2.3")
	if err := addFiles(&m, dir, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(bin)
	if len(m.Files) != 1 || m.Files["linux-amd64"].Size != st.Size() || len(m.Files["linux-amd64"].SHA256) != 64 {
		t.Fatalf("%+v", m.Files)
	}
	write("update.json", `{"version":"1.2.3","files":{"darwin-universal":{"name":"Quartz-macos.zip","size":1,"sha256":"`+strings.Repeat("a", 64)+`"}}}`)
	m, _ = readManifest(dir, "1.2.3")
	if err := addFiles(&m, dir, "1.2.3"); err != nil || len(m.Files) != 2 {
		t.Fatal("a later Mac build extends the same version's manifest", err)
	}
	if m2, _ := readManifest(dir, "1.2.4"); len(m2.Files) != 0 {
		t.Fatal("another version starts fresh")
	}
}

func TestManifestRefusesStaleBuilds(t *testing.T) {
	dir := t.TempDir()
	goProgram(t, dir, "quartz-linux-amd64", "production")
	m, _ := readManifest(dir, "1.0.1")
	if err := addFiles(&m, dir, "1.0.1"); err == nil || !strings.Contains(err.Error(), "unknown version") {
		t.Fatal("an unstamped build must be refused:", err)
	}
	_ = stamp(dir, "quartz-linux-amd64", "1.0.0")
	if err := addFiles(&m, dir, "1.0.1"); err == nil || !strings.Contains(err.Error(), "built for 1.0.0") {
		t.Fatal("an older version's build must be refused:", err)
	}
	_ = stamp(dir, "quartz-linux-amd64", "1.0.1")
	if err := addFiles(&m, dir, "1.0.1"); err != nil {
		t.Fatal(err)
	}
}

func TestManifestRefusesDevBuilds(t *testing.T) {
	dir := t.TempDir()
	goProgram(t, dir, "quartz-linux-amd64", "production,fakeauth")
	_ = stamp(dir, "quartz-linux-amd64", "1.0.0")
	m, _ := readManifest(dir, "1.0.0")
	if err := addFiles(&m, dir, "1.0.0"); err == nil || !strings.Contains(err.Error(), "fakeauth") {
		t.Fatal("a development build must never be signed:", err)
	}
}

func TestVerifyArtifact(t *testing.T) {
	dir := t.TempDir()
	if _, err := verifyArtifact(goProgram(t, dir, "ok", "production,webkit2_41")); err != nil {
		t.Fatal(err)
	}
	for _, tags := range []string{"webkit2_41", "production,e2e", "production,pprof"} {
		if _, err := verifyArtifact(goProgram(t, dir, "bad", tags)); err == nil {
			t.Errorf("tags %q must be refused", tags)
		}
	}
	big := filepath.Join(dir, "big")
	data, _ := os.ReadFile(goProgram(t, dir, "big", "production"))
	data = append(data, make([]byte, maxBinary)...)
	if err := os.WriteFile(big, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyArtifact(big); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatal("a binary over the size limit must be refused:", err)
	}
}

func TestVerifyMacZip(t *testing.T) {
	dir := t.TempDir()
	zipped := func(name, tags string) string {
		bin, err := os.Open(goProgram(t, dir, "Quartz", tags)) //nolint:gosec // test output
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = bin.Close() }()
		path := filepath.Join(dir, name)
		f, err := os.Create(path) //nolint:gosec // test output
		if err != nil {
			t.Fatal(err)
		}
		z := zip.NewWriter(f)
		w, _ := z.Create("Quartz.app/Contents/MacOS/Quartz")
		_, _ = io.Copy(w, bin)
		_ = z.Close()
		_ = f.Close()
		return path
	}
	if _, err := verifyArtifact(zipped("good.zip", "production")); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyArtifact(zipped("dev.zip", "production,e2e")); err == nil {
		t.Fatal("a zipped development build must be refused")
	}
}

func TestCheckIDs(t *testing.T) {
	t.Setenv("QUARTZ_MSA_CLIENT_ID", "")
	t.Setenv("QUARTZ_CF_API_KEY", "")
	t.Setenv("QUARTZ_DISCORD_CLIENT_ID", "")
	if err := checkIDs(false); err == nil {
		t.Fatal("a release without the Microsoft ID must be refused")
	}
	if err := checkIDs(true); err != nil {
		t.Fatal("-allow-missing accepts missing IDs:", err)
	}
	t.Setenv("QUARTZ_MSA_CLIENT_ID", "11111111-2222-3333-4444-555555555555")
	t.Setenv("QUARTZ_CF_API_KEY", "$2a$10$abcdefghijklmnopqrstuv")
	if err := checkIDs(false); err != nil {
		t.Fatal("Discord is optional:", err)
	}
	t.Setenv("QUARTZ_DISCORD_CLIENT_ID", "123 -X quartz/internal/brand.Version=6.6.6")
	if err := checkIDs(true); err == nil {
		t.Fatal("a malformed ID must be refused even with -allow-missing")
	}
}

func TestLdflags(t *testing.T) {
	t.Setenv("QUARTZ_DISCORD_CLIENT_ID", "123456789012345678")
	f := ldflags("1.0.0", "RWQkey")
	for _, want := range []string{"-s -w", "brand.Version=1.0.0", "brand.UpdatePublicKey=RWQkey", "brand.DiscordClientID=123456789012345678"} {
		if !strings.Contains(f, want) {
			t.Errorf("missing %q in %s", want, f)
		}
	}
}

func TestReleaseBuildsCarryNoDevTags(t *testing.T) {
	for w := range targets {
		args := strings.Join(wailsArgs(w, "", "out", false), " ")
		for _, dev := range devTags {
			if strings.Contains(args, dev) {
				t.Fatalf("%s builds with the development tag %q: %s", w, dev, args)
			}
		}
	}
}
