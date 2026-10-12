package instance

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateListUpdateReload(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	def := JavaSettings{MemoryMinMB: 1024, MemoryMaxMB: 4096, JVMArgs: "-XX:+UseG1GC"}
	a, err := s.Create(CreateRequest{Name: "  Survival 1.21.4 ", Minecraft: "1.21.4", Art: Art{Background: "#6b8afd"}}, def)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(a.ID, "survival-1-21-4-") || a.Name != "Survival 1.21.4" || a.Art.Initial != "S" || a.Settings.Java.MemoryMaxMB != 4096 || a.Settings.Java.Override {
		t.Fatalf("created %+v", a)
	}
	if _, err := os.Stat(s.GameDir(a.ID)); err != nil {
		t.Fatal("game folder missing")
	}
	b, _ := s.Create(CreateRequest{Name: "Second", Minecraft: "26.3"}, def)
	now := time.Now()
	if _, err := s.Update(a.ID, func(in *Instance) error { in.LastPlayed = &now; in.PlaytimeMin += 30; in.ID = "hijack"; return nil }); err != nil {
		t.Fatal(err)
	}
	if list := s.List(); list[0].ID != a.ID || list[1].ID != b.ID {
		t.Fatalf("order %v, %v", list[0].ID, list[1].ID)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	re, err := Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := re.Get(a.ID)
	if err != nil || got.PlaytimeMin != 30 || got.ID != a.ID {
		t.Fatalf("reloaded %+v %v", got, err)
	}
}

func TestCreateRejects(t *testing.T) {
	s, _ := Open(t.TempDir(), nil)
	cases := map[string]CreateRequest{
		"empty name":  {Name: "   ", Minecraft: "1.21.4"},
		"long name":   {Name: strings.Repeat("x", 65), Minecraft: "1.21.4"},
		"control":     {Name: "a\x00b", Minecraft: "1.21.4"},
		"loader":      {Name: "Modded", Minecraft: "1.21.4", Loader: "fabric"},
		"bad version": {Name: "X", Minecraft: "../../x"},
	}
	for name, req := range cases {
		if _, err := s.Create(req, JavaSettings{}); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := s.Create(cases["loader"], JavaSettings{}); !errors.Is(err, ErrLoaderUnsupported) {
		t.Fatal("loader error not specific")
	}
}

func TestHostileFolders(t *testing.T) {
	dir := t.TempDir()
	// A folder whose file claims another id, and one with an invalid name.
	for name, body := range map[string]string{"real-1234": `{"id":"other-1234","name":"x"}`, "Bad Name": `{"id":"Bad Name"}`} {
		_ = os.MkdirAll(filepath.Join(dir, name), 0o700)
		_ = os.WriteFile(filepath.Join(dir, name, "instance.json"), []byte(body), 0o600)
	}
	s, err := Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.List()) != 0 {
		t.Fatalf("hostile folders loaded: %+v", s.List())
	}
	if _, err := s.GameDirSafe("../../etc"); err == nil {
		t.Fatal("traversal id accepted")
	}
}

func TestSettingsValidation(t *testing.T) {
	s, _ := Open(t.TempDir(), nil)
	a, _ := s.Create(CreateRequest{Name: "A", Minecraft: "1.21.4"}, JavaSettings{})
	if _, err := s.SetSettings(a.ID, Settings{Java: JavaSettings{JavaPath: "relative/java"}}); !errors.Is(err, ErrInvalidSettings) {
		t.Fatal("relative java path accepted")
	}
	got, err := s.SetSettings(a.ID, Settings{Java: JavaSettings{Override: true, MemoryMinMB: 4096, MemoryMaxMB: 1024}, Window: WindowSettings{Width: 1280, Height: 720, OnGameStart: "bogus"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Settings.Java.MemoryMaxMB != 4096 || got.Settings.Window.OnGameStart != "" {
		t.Fatalf("not normalised: %+v", got.Settings)
	}
}
