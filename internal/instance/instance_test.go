package instance

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"quartz/internal/settings"
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
		"loader":      {Name: "Modded", Minecraft: "1.21.4", Loader: "liteloader", LoaderVersion: "1"},
		"no build":    {Name: "Modded", Minecraft: "1.21.4", Loader: "fabric"},
		"bad build":   {Name: "Modded", Minecraft: "1.21.4", Loader: "fabric", LoaderVersion: "../x"},
		"long group":  {Name: "X", Minecraft: "1.21.4", Group: strings.Repeat("g", 33)},
		"bad icon":    {Name: "X", Minecraft: "1.21.4", Icon: "../icon.png"},
		"bad version": {Name: "X", Minecraft: "../../x"},
	}
	for name, req := range cases {
		if _, err := s.Create(req, JavaSettings{}); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := s.Create(cases["loader"], JavaSettings{}); !errors.Is(err, ErrBadLoader) {
		t.Fatal("loader error not specific")
	}
	in, err := s.Create(CreateRequest{Name: "Modded", Minecraft: "1.21.1", Loader: "fabric", LoaderVersion: "0.16.10", Group: " Mods "}, JavaSettings{})
	if err != nil || in.Loader != "fabric" || in.LoaderVersion != "0.16.10" || in.Group != "Mods" || !in.KeepLoaderUpdated || in.Version != schema {
		t.Fatalf("modded instance = %+v, %v", in, err)
	}
	if in, _ := s.Create(CreateRequest{Name: "Plain", Minecraft: "1.21.1", Loader: "none"}, JavaSettings{}); in.Loader != "" {
		t.Fatal(`"none" must mean vanilla`)
	}
}

func TestVersionOneFilesMigrate(t *testing.T) {
	dir := t.TempDir()
	id := "old-1a2b"
	if err := os.MkdirAll(filepath.Join(dir, id, "game"), 0o700); err != nil {
		t.Fatal(err)
	}
	v1 := `{"version":1,"id":"old-1a2b","name":"Old","art":{"background":"#3ecf8e"},"minecraft":"1.21.1","loader":"quilt-ish",
		"settings":{"java":{"override":true,"memoryMinMb":2048,"memoryMaxMb":6144,"jvmArgs":"-XX:+UseZGC"},"window":{}}}`
	if err := os.WriteFile(filepath.Join(dir, id, "instance.json"), []byte(v1), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	in, err := s.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if in.Version != schema || !in.KeepLoaderUpdated || in.Loader != "" || in.Settings.Java.MemoryMaxMB != 6144 {
		t.Fatalf("migrated = %+v", in)
	}
}

func TestResolveLayersOverGlobals(t *testing.T) {
	g := settings.Defaults()
	in := Instance{Settings: Settings{Java: JavaSettings{JavaPath: "/opt/jdk/bin/java", MemoryMinMB: 100, MemoryMaxMB: 8192, JVMArgs: "-Dx"}}}
	e := in.Resolve(g)
	if e.JavaPath != "" || e.MemoryMaxMB != g.Java.MemoryMaxMB || e.JVMArgs != g.Java.JVMArgs || e.OnGameStart != g.General.OnGameStart {
		t.Fatalf("without override the globals apply: %+v", e)
	}
	in.Settings.Java.Override, in.Settings.Window.OnGameStart = true, "close"
	e = in.Resolve(g)
	if e.JavaPath != "/opt/jdk/bin/java" || e.MemoryMinMB != 256 || e.MemoryMaxMB != 8192 || e.JVMArgs != "-Dx" || e.OnGameStart != "close" {
		t.Fatalf("override = %+v", e)
	}
}

func TestCommandValidation(t *testing.T) {
	s, _ := Open(t.TempDir(), nil)
	in, _ := s.Create(CreateRequest{Name: "C", Minecraft: "1.21.1"}, JavaSettings{})
	bad := []CommandSettings{
		{PreLaunch: "backup\nrm -rf /"},
		{Env: []EnvVar{{Name: "1BAD", Value: "x"}}},
		{Env: []EnvVar{{Name: "JAVA_TOOL_OPTIONS", Value: "-javaagent:evil.jar"}}},
		{Env: []EnvVar{{Name: "_java_options", Value: "x"}}},
	}
	for _, c := range bad {
		if _, err := s.SetSettings(in.ID, Settings{Commands: c}); err == nil {
			t.Errorf("accepted %+v", c)
		}
	}
	ok := CommandSettings{PreLaunch: `$HOME/scripts/backup.sh "$INST_DIR"`, Env: []EnvVar{{Name: "MESA_GL_VERSION_OVERRIDE", Value: "4.6"}}}
	if _, err := s.SetSettings(in.ID, Settings{Commands: ok}); err != nil {
		t.Fatal(err)
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

func TestSyncMigratesShareLabels(t *testing.T) {
	in := normalize(Instance{ID: "a", Name: "A", Shares: []string{"Keybinds", "Worlds", "Shaders", "Nonsense"},
		Sync: Sync{Join: []string{"saves", "Bad Id", "c-1a2b"}}})
	if in.Shares != nil || !in.Sync.Keys || !slices.Equal(in.Sync.Join, []string{"saves", "c-1a2b", "shaderpacks"}) {
		t.Fatalf("%+v %v", in.Sync, in.Shares)
	}
}
