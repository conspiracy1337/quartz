package launch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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
	"quartz/internal/settings"
)

// Two copies of one instance run at once: the second works on a private
// view, and when both exit the instance has what each of them wrote.
func TestTwoCopiesRunAndMergeBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the game")
	}
	dir := t.TempDir()
	data, err := os.ReadFile("../mojang/testdata/1.21.4.json")
	if err != nil {
		t.Fatal(err)
	}
	v, _ := mojang.Parse(data)
	insts, _ := instance.Open(filepath.Join(dir, "instances"), nil)
	inst, _ := insts.Create(instance.CreateRequest{Name: "T", Minecraft: "1.21.4"})
	game := insts.GameDir(inst.ID)
	_ = os.MkdirAll(game, 0o700)
	// The "game": takes its arguments, logs in, writes one file of its own, waits.
	fake := filepath.Join(dir, "java")
	script := "#!/bin/sh\ncat >/dev/null\necho 'Setting user: Steve'\necho $$ > \"copy-$$.txt\"\nsleep 0.3\n"
	if err := os.WriteFile(fake, []byte(script), 0o700); err != nil { //nolint:gosec // an executable test script
		t.Fatal(err)
	}
	bus := events.NewBus()
	exited := make(chan State, 2)
	bus.Subscribe(events.GameState, func(p any) {
		if st := p.(State); st.State == "exited" {
			exited <- st
		}
	})
	st, _ := settings.Open(filepath.Join(dir, "settings.json"), bus, nil)
	l := New(Config{Instances: insts, Settings: st, Bus: bus, BinDir: filepath.Join(dir, "bin"), Redactor: logging.NewRedactor(""),
		Views: runview.New(runview.Config{Backend: "clone"})})
	res := &install.Resolved{Version: v, Classpath: []string{"/a.jar"}, NativesDir: "/n", AssetsRoot: "/a", GameAssets: "/a", LibrariesDir: "/l"}
	start := func(runID string) {
		t.Helper()
		sp := spec{inst: inst, eff: inst.Resolve(st.Get()), acc: accounts.Account{ID: "acc"}, sess: accounts.Session{Name: "Steve", AccessToken: "t"},
			res: res, rt: java.Runtime{Path: fake}, mem: MemoryRange{Min: 256, Max: 512}, env: mojang.Env{OS: "linux", Features: map[string]bool{}},
			gameDir: game, runID: runID}
		if err := l.assemble(&sp); err != nil {
			t.Fatal(err)
		}
		sp.vars = instVars(inst, insts.Dir(inst.ID), sp.gameDir, fake)
		var err error
		if sp.cmd, sp.script, sp.shown, err = l.command(&sp); err != nil {
			t.Fatal(err)
		}
		if _, err := l.start(context.Background(), sp); err != nil {
			t.Fatal(err)
		}
	}
	start("20260101-000000.001")
	start("20260101-000000.002")
	if !l.Running(inst.ID) || len(l.running[inst.ID]) != 2 || !l.AccountInUse("acc") {
		t.Fatal("two copies should run")
	}
	if c := l.Copies(inst.ID); len(c) != 2 || c[0].RunID != "20260101-000000.001" || c[1].RunID != "20260101-000000.002" || c[1].AccountID != "acc" {
		t.Fatalf("the live console lists each copy, oldest first: %+v", c)
	}
	if left, _ := filepath.Glob(filepath.Join(insts.RunsDir(inst.ID), "*"+pendingExt)); len(left) != 2 {
		t.Fatalf("each running copy is recorded for a later launcher: %v", left)
	}
	for range 2 {
		select {
		case st := <-exited:
			if st.Result != ResultOK {
				t.Fatalf("%+v", st)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the copies never exited")
		}
	}
	if l.Running(inst.ID) {
		t.Fatal("still running")
	}
	written, _ := filepath.Glob(filepath.Join(game, "copy-*.txt"))
	if len(written) != 2 {
		t.Fatalf("the instance has %v; the second copy's file must be merged back", written)
	}
	if views, _ := filepath.Glob(filepath.Join(insts.RunsDir(inst.ID), "*.view")); len(views) != 0 {
		t.Fatalf("views left behind: %v", views)
	}
	if left, _ := filepath.Glob(filepath.Join(insts.RunsDir(inst.ID), "*"+pendingExt)); len(left) != 0 {
		t.Fatalf("finished runs left pending: %v", left)
	}
}
