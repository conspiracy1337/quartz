package launch

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"quartz/internal/accounts"
	"quartz/internal/core/events"
	"quartz/internal/gamelog"
	"quartz/internal/install"
	"quartz/internal/instance"
	"quartz/internal/java"
	"quartz/internal/mojang"
	"quartz/internal/settings"
)

// gameArgs builds a launch for version file ver opening into, and returns
// the game arguments the script carries plus the command text.
func gameArgs(t *testing.T, ver string, into instance.Into) ([]string, string) {
	t.Helper()
	dir := t.TempDir()
	data, err := os.ReadFile("../mojang/testdata/" + ver + ".json")
	if err != nil {
		t.Fatal(err)
	}
	v, err := mojang.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	insts, _ := instance.Open(filepath.Join(dir, "instances"), nil)
	inst, _ := insts.Create(instance.CreateRequest{Name: "T", Minecraft: ver})
	st, _ := settings.Open(filepath.Join(dir, "settings.json"), events.NewBus(), nil)
	l := New(Config{Instances: insts, Settings: st, BinDir: filepath.Join(dir, "bin")})
	res := &install.Resolved{Version: v, Classpath: []string{"/a.jar"}, AssetsRoot: "/assets", GameAssets: "/assets"}
	env := mojang.Env{OS: "linux", Arch: "x86_64", Features: map[string]bool{}}
	sp := spec{inst: inst, eff: inst.Resolve(st.Get()), sess: accounts.Session{Name: "Steve", AccessToken: "TOKEN"}, res: res, rt: java.Runtime{Path: "/java"}, mem: MemoryRange{Min: 512, Max: 1024}, env: env, gameDir: insts.GameDir(inst.ID), into: into}
	sp.vars = instVars(inst, insts.Dir(inst.ID), sp.gameDir, sp.rt.Path)
	_, script, shown, err := l.command(&sp)
	if err != nil {
		t.Fatal(err)
	}
	var game []string
	for _, line := range strings.Split(strings.TrimSpace(string(script)), "\n") {
		if c, val, _ := strings.Cut(line, " "); c == "arg" {
			b, _ := base64.StdEncoding.DecodeString(val)
			game = append(game, string(b))
		}
	}
	return game, shown
}

func after(args []string, flag string) string {
	if i := slices.Index(args, flag); i >= 0 && i+1 < len(args) {
		return args[i+1]
	}
	return ""
}

func TestQuickPlay(t *testing.T) {
	game, shown := gameArgs(t, "1.21.4", instance.Into{Kind: "world", Target: "Northwatch"})
	if after(game, "--quickPlaySingleplayer") != "Northwatch" || slices.Contains(game, "--quickPlayMultiplayer") {
		t.Fatalf("world: %v", game)
	}
	if !strings.Contains(shown, "Game arguments") || !strings.Contains(shown, "--quickPlaySingleplayer Northwatch") {
		t.Fatalf("command text: %s", shown)
	}
	game, _ = gameArgs(t, "1.21.4", instance.Into{Kind: "server", Target: "smp.kestrel.gg:25570"})
	if after(game, "--quickPlayMultiplayer") != "smp.kestrel.gg:25570" || slices.Contains(game, "--server") {
		t.Fatalf("server: %v", game)
	}
	game, _ = gameArgs(t, "1.21.4", instance.Into{})
	if slices.ContainsFunc(game, func(a string) bool { return strings.HasPrefix(a, "--quickPlay") && a != "--quickPlayPath" }) {
		t.Fatalf("quick play without a target: %v", game)
	}
}

func TestLegacyServerArgs(t *testing.T) {
	game, _ := gameArgs(t, "1.16.5", instance.Into{Kind: "server", Target: "mc.hypixel.net"})
	if after(game, "--server") != "mc.hypixel.net" || after(game, "--port") != "25565" {
		t.Fatalf("legacy server: %v", game)
	}
	game, _ = gameArgs(t, "1.12.2", instance.Into{Kind: "world", Target: "Northwatch"})
	if slices.Contains(game, "--server") || slices.Contains(game, "Northwatch") {
		t.Fatalf("legacy world must open the title screen: %v", game)
	}
}

func TestPlayRejectsBadTargets(t *testing.T) {
	l := New(Config{})
	for _, into := range []instance.Into{{Kind: "world", Target: "../x"}, {Kind: "server", Target: "a b"}, {Kind: "realm", Target: "x"}} {
		if err := l.Play("x", "", into); !errors.Is(err, ErrBadTarget) {
			t.Errorf("%+v: %v", into, err)
		}
	}
}

func TestLogDetection(t *testing.T) {
	r := &run{}
	r.logFile, _ = os.Create(filepath.Join(t.TempDir(), "x.log"))
	defer func() { _ = r.logFile.Close() }()
	r.add(gamelog.Line{Message: "Loading Minecraft 1.21.4 with Fabric Loader 0.16.10"})
	if r.loaded.Load() {
		t.Fatal("loaded too early")
	}
	r.add(gamelog.Line{Message: "Setting user: Steve"})
	r.add(gamelog.Line{Message: "Connecting to mc.hypixel.net, 25565"})
	if !r.loaded.Load() || r.server != "mc.hypixel.net" {
		t.Fatalf("loaded=%v server=%q", r.loaded.Load(), r.server)
	}
	r.add(gamelog.Line{Message: "Connecting to smp.kestrel.gg, 25570"})
	if r.server != "smp.kestrel.gg:25570" {
		t.Fatalf("server %q", r.server)
	}
}

func TestPruneRuns(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range maxRecords + 5 {
		id := newRunID(start.Add(time.Duration(i) * time.Minute))
		_ = os.WriteFile(filepath.Join(dir, id+".log"), []byte("x"), 0o600)
		if err := saveRecord(dir, Record{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	recs := listRecords(dir)
	if len(recs) != maxRecords {
		t.Fatalf("%d records kept", len(recs))
	}
	logs, _ := filepath.Glob(filepath.Join(dir, "*.log"))
	if len(logs) != maxEvidence {
		t.Fatalf("%d logs kept, want %d", len(logs), maxEvidence)
	}
	if _, err := EvidencePath(dir, "../../x", EvidenceLog); err == nil {
		t.Fatal("bad run id accepted")
	}
	if _, err := EvidencePath(dir, recs[0].ID, "passwd"); err == nil {
		t.Fatal("bad evidence kind accepted")
	}
}
