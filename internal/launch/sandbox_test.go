package launch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"quartz/internal/accounts"
	"quartz/internal/core/events"
	"quartz/internal/core/logging"
	"quartz/internal/install"
	"quartz/internal/instance"
	"quartz/internal/java"
	"quartz/internal/mojang"
	"quartz/internal/runview"
	"quartz/internal/sandbox"
	"quartz/internal/settings"
)

// A sandboxed run starts inside its jail, writes only its game folder, and
// its record names the sandbox.
func TestSandboxedRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the game")
	}
	dir := t.TempDir()
	sb := sandbox.New(sandbox.Config{DataRoot: dir})
	if st := sb.Status(); !st.Available {
		t.Skip(st.Reason)
	}
	data, err := os.ReadFile("../mojang/testdata/1.21.4.json")
	if err != nil {
		t.Fatal(err)
	}
	v, _ := mojang.Parse(data)
	insts, _ := instance.Open(filepath.Join(dir, "instances"), nil)
	inst, _ := insts.Create(instance.CreateRequest{Name: "T", Minecraft: "1.21.4", Sandbox: true}, instance.JavaSettings{})
	game := insts.GameDir(inst.ID)
	_ = os.MkdirAll(game, 0o700)
	secret := filepath.Join(dir, "accounts.json")
	_ = os.WriteFile(secret, []byte("x"), 0o600)
	// The "game" logs in, then records what it could reach.
	fake := filepath.Join(dir, "jdk", "bin", "java")
	script := "#!/bin/sh\ncat >/dev/null\necho 'Setting user: Steve'\n" +
		"if cat " + secret + " 2>/dev/null; then echo leaked > reach.txt; else echo hidden > reach.txt; fi\n"
	_ = os.MkdirAll(filepath.Dir(fake), 0o700)
	if err := os.WriteFile(fake, []byte(script), 0o700); err != nil { //nolint:gosec // an executable test script
		t.Fatal(err)
	}
	bus := events.NewBus()
	exited := make(chan State, 1)
	bus.Subscribe(events.GameState, func(p any) {
		if st := p.(State); st.State == "exited" {
			exited <- st
		}
	})
	st, _ := settings.Open(filepath.Join(dir, "settings.json"), bus, nil)
	l := New(Config{Instances: insts, Settings: st, Bus: bus, BinDir: filepath.Join(dir, "store", "bin"), Redactor: logging.NewRedactor(""),
		Views: runview.New(runview.Config{}), Sandbox: sb, StoreRead: []string{filepath.Join(dir, "store", "bin")}})
	res := &install.Resolved{Version: v, Classpath: []string{"/a.jar"}, NativesDir: "/n", AssetsRoot: "/a", GameAssets: "/a", LibrariesDir: "/l"}
	sp := spec{inst: inst, eff: inst.Resolve(st.Get()), acc: accounts.Account{ID: "acc"}, sess: accounts.Session{Name: "Steve", AccessToken: "t"},
		res: res, rt: java.Runtime{Path: fake}, mem: MemoryRange{Min: 256, Max: 512}, env: mojang.Env{OS: runtime.GOOS, Features: map[string]bool{}},
		gameDir: game, runID: "20260101-000000.001", sandboxed: true}
	if err := l.assemble(&sp); err != nil {
		t.Fatal(err)
	}
	sp.vars = instVars(inst, insts.Dir(inst.ID), sp.gameDir, fake)
	if sp.cmd, sp.script, sp.shown, err = l.command(&sp); err != nil {
		t.Fatal(err)
	}
	if sp.jail == nil || (!strings.Contains(sp.shown, "bwrap") && !strings.Contains(sp.shown, "sandbox-exec")) {
		t.Fatalf("not wrapped: %s", sp.shown)
	}
	if _, err := l.start(context.Background(), sp); err != nil {
		t.Fatal(err)
	}
	select {
	case st := <-exited:
		if st.Result != ResultOK {
			t.Fatalf("%+v", st)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the game never exited")
	}
	if b, _ := os.ReadFile(filepath.Join(game, "reach.txt")); strings.TrimSpace(string(b)) != "hidden" { //nolint:gosec // test temp dir
		t.Fatalf("reach.txt = %q", b)
	}
	if recs := listRecords(insts.RunsDir(inst.ID)); len(recs) != 1 || recs[0].Sandbox != sp.jail.Name {
		t.Fatalf("records %+v", recs)
	}
}
