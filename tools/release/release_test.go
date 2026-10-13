package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quartz/internal/core/minisign"
)

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
	write("quartz-linux-amd64", "linux")
	write("notes.md", "not a release file")
	m, _ := readManifest(dir, "1.2.3")
	if err := addFiles(&m, dir); err != nil {
		t.Fatal(err)
	}
	if len(m.Files) != 1 || m.Files["linux-amd64"].Size != 5 || len(m.Files["linux-amd64"].SHA256) != 64 {
		t.Fatalf("%+v", m.Files)
	}
	write("update.json", `{"version":"1.2.3","files":{"darwin-universal":{"name":"Quartz-macos.zip","size":1,"sha256":"`+strings.Repeat("a", 64)+`"}}}`)
	m, _ = readManifest(dir, "1.2.3")
	_ = addFiles(&m, dir)
	if len(m.Files) != 2 {
		t.Fatal("a later Mac build extends the same version's manifest")
	}
	if m2, _ := readManifest(dir, "1.2.4"); len(m2.Files) != 0 {
		t.Fatal("another version starts fresh")
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
