package clientjar

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeJar(t *testing.T, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "client.jar")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for name, body := range entries {
		e, _ := w.Create(name)
		_, _ = e.Write([]byte(body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	return p
}

func TestInspect(t *testing.T) {
	modern := writeJar(t, map[string]string{"net/minecraft/client/main/Main.class": "", "version.json": `{"id":"1.21.1","name":"1.21.1"}`})
	if info, err := Inspect(modern); err != nil || info.Minecraft != "1.21.1" {
		t.Fatalf("modern = %+v, %v", info, err)
	}
	// Before 1.14 the jar doesn't name its version; a bad id is ignored.
	for _, entries := range []map[string]string{
		{"net/minecraft/client/Minecraft.class": ""},
		{"net/minecraft/client/main/Main.class": "", "version.json": `{"id":"../evil"}`},
	} {
		if info, err := Inspect(writeJar(t, entries)); err != nil || info.Minecraft != "" {
			t.Errorf("%v = %+v, %v", entries, info, err)
		}
	}
	notClient := writeJar(t, map[string]string{"fabric.mod.json": "{}", "com/example/Mod.class": ""})
	if _, err := Inspect(notClient); !errors.Is(err, ErrNotClient) {
		t.Errorf("a mod jar = %v", err)
	}
	text := filepath.Join(t.TempDir(), "notes.jar")
	_ = os.WriteFile(text, []byte("not a zip"), 0o600)
	if _, err := Inspect(text); !errors.Is(err, ErrNotClient) {
		t.Errorf("a text file = %v", err)
	}
	if _, err := Inspect(t.TempDir()); !errors.Is(err, ErrNotClient) {
		t.Errorf("a folder = %v", err)
	}
}

func TestCheckRefusesLargeFiles(t *testing.T) {
	big := filepath.Join(t.TempDir(), "big.jar")
	f, _ := os.Create(big)
	_ = f.Truncate(MaxFile + 1) // sparse: costs no disk
	_ = f.Close()
	if err := Check(big); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized = %v", err)
	}
}

func TestFits(t *testing.T) {
	p := writeJar(t, map[string]string{"net/minecraft/client/main/Main.class": ""})
	if err := Fits(p, "net.minecraft.client.main.Main"); err != nil {
		t.Errorf("own main class = %v", err)
	}
	if err := Fits(p, "net.minecraft.launchwrapper.Launch"); err != nil {
		t.Errorf("a library's main class = %v", err)
	}
	if err := Fits(p, "net.minecraft.client.Minecraft"); !errors.Is(err, ErrWrongVersion) {
		t.Errorf("missing main class = %v", err)
	}
}
