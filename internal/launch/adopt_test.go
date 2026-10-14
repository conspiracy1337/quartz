package launch

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"quartz/internal/accounts"
	"quartz/internal/core/events"
	"quartz/internal/core/fsx"
	"quartz/internal/core/logging"
	"quartz/internal/core/procx"
	"quartz/internal/install"
	"quartz/internal/instance"
	"quartz/internal/java"
	"quartz/internal/mojang"
	"quartz/internal/runview"
	"quartz/internal/settings"
	"quartz/internal/syncing"
)

type adoptEnv struct {
	dir   string
	insts *instance.Store
	st    *settings.Store
	sync  *syncing.Service
	bus   *events.Bus
	inst  instance.Instance
}

func newAdoptEnv(t *testing.T) *adoptEnv {
	t.Helper()
	dir := t.TempDir()
	e := &adoptEnv{dir: dir, bus: events.NewBus()}
	e.insts, _ = instance.Open(filepath.Join(dir, "instances"), nil)
	e.inst, _ = e.insts.Create(instance.CreateRequest{Name: "T", Minecraft: "1.21.4"})
	_ = os.MkdirAll(e.insts.GameDir(e.inst.ID), 0o700)
	e.st, _ = settings.Open(filepath.Join(dir, "settings.json"), e.bus, nil)
	e.sync = syncing.New(syncing.Config{Root: filepath.Join(dir, "shared"), Trash: filepath.Join(dir, "trash"), Settings: e.st, Instances: e.insts})
	return e
}

// launcher is a fresh launcher over the same data, as after a restart.
func (e *adoptEnv) launcher() *Launcher {
	return New(Config{Instances: e.insts, Settings: e.st, Bus: e.bus, Redactor: logging.NewRedactor(""), Syncing: e.sync, BinDir: filepath.Join(e.dir, "bin"),
		Views: runview.New(runview.Config{Backend: "clone"})})
}

func (e *adoptEnv) writePending(t *testing.T, p pending) {
	t.Helper()
	data, _ := json.Marshal(p)
	runs := e.insts.RunsDir(e.inst.ID)
	_ = os.MkdirAll(runs, 0o700)
	if err := fsx.AtomicWrite(pendingPath(runs, p.Record.ID), data, fsx.FilePerm); err != nil {
		t.Fatal(err)
	}
}

// A run the launcher didn't see end (it closed when the game started) is
// finished by the next launcher: its option changes reach the shared copy
// instead of being overwritten, and its record and playtime are kept.
func TestRecoverFinishesARunThatEndedUnseen(t *testing.T) {
	e := newAdoptEnv(t)
	ctx := context.Background()
	game := e.insts.GameDir(e.inst.ID)
	opts := filepath.Join(game, "options.txt")
	_ = os.WriteFile(opts, []byte("version:4189\nfov:0.0\n"), 0o600)
	if err := e.sync.SetEnabled(ctx, syncing.OptionsID, true); err != nil {
		t.Fatal(err)
	}
	if err := e.sync.Join(ctx, e.inst.ID, syncing.OptionsID); err != nil {
		t.Fatal(err)
	}
	se, err := e.sync.Prepare(e.inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The "game" ran and exited: a process that is gone now.
	gone := exec.Command(os.Args[0], "-test.run=^$") //nolint:gosec // the test binary itself
	if err := gone.Run(); err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-10 * time.Minute)
	st := se.State()
	id := newRunID(started)
	e.writePending(t, pending{Record: Record{ID: id, StartedAt: started.UnixMilli(), AccountID: "acc"}, Started: started, PID: gone.Process.Pid, Sync: &st})
	_ = os.WriteFile(opts, []byte("version:4189\nfov:0.5\n"), 0o600) // changed in-game

	l := e.launcher()
	l.Recover()
	if l.Busy(e.inst.ID) {
		t.Fatal("a game that is gone counts as running")
	}
	shared, _ := os.ReadFile(filepath.Join(e.dir, "shared", "options.txt"))
	if !strings.Contains(string(shared), "fov:0.5") {
		t.Fatalf("the run's option change was lost:\n%s", shared)
	}
	if _, err := e.sync.Prepare(e.inst.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(opts); !strings.Contains(string(got), "fov:0.5") {
		t.Fatalf("the next launch overwrote the change:\n%s", got)
	}
	if recs := l.Runs(e.inst.ID); len(recs) != 1 || recs[0].ID != id || recs[0].Result != ResultOK {
		t.Fatalf("records %+v", recs)
	}
	if in, _ := e.insts.Get(e.inst.ID); in.PlaytimeMin < 9 || in.LastPlayed == nil {
		t.Fatalf("playtime %d", in.PlaytimeMin)
	}
	if _, err := os.Stat(pendingPath(e.insts.RunsDir(e.inst.ID), id)); !os.IsNotExist(err) {
		t.Fatal("the pending run is still there")
	}
}

// A game still running when the launcher starts is adopted: it counts as
// running, its folders stay leased, and its run finishes once it exits.
func TestRecoverAdoptsARunningGame(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the game")
	}
	e := newAdoptEnv(t)
	data, err := os.ReadFile("../mojang/testdata/1.21.4.json")
	if err != nil {
		t.Fatal(err)
	}
	v, _ := mojang.Parse(data)
	game := e.insts.GameDir(e.inst.ID)
	fake := filepath.Join(e.dir, "java")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\ncat >/dev/null\necho 'Setting user: Steve'\nexec sleep 30\n"), 0o700); err != nil { //nolint:gosec // an executable test script
		t.Fatal(err)
	}
	// The first launcher starts the game, then quits without seeing it end.
	first := e.launcher()
	res := &install.Resolved{Version: v, Classpath: []string{"/a.jar"}, NativesDir: "/n", AssetsRoot: "/a", GameAssets: "/a", LibrariesDir: "/l"}
	sp := spec{inst: e.inst, eff: e.inst.Resolve(e.st.Get()), acc: accounts.Account{ID: "acc"}, sess: accounts.Session{Name: "Steve", AccessToken: "t"},
		res: res, rt: java.Runtime{Path: fake}, mem: MemoryRange{Min: 256, Max: 512}, env: mojang.Env{OS: "linux", Features: map[string]bool{}},
		gameDir: game, runID: newRunID(time.Now())}
	if err := first.assemble(&sp); err != nil {
		t.Fatal(err)
	}
	sp.vars = instVars(e.inst, e.insts.Dir(e.inst.ID), sp.gameDir, fake)
	if sp.cmd, sp.script, sp.shown, err = first.command(&sp); err != nil {
		t.Fatal(err)
	}
	// Started as start does, but with nobody waiting on it here (init reaps
	// a game whose launcher quit).
	child, err := procx.Start(sp.cmd)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = child.Stdin.Write(sp.script)
	_ = child.Stdin.Close()
	go func() { _ = child.Wait() }()
	pid := child.Process.Pid
	defer func() { _ = procx.Kill(child.Process) }()
	sp.view.Started(pid)
	r := &run{id: sp.runID, instanceID: e.inst.ID, started: time.Now(), record: Record{ID: sp.runID, AccountID: "acc"}, pid: pid, sp: sp}
	r.ident, _ = procx.Ident(pid)
	first.savePending(r)

	exited := make(chan State, 4)
	e.bus.Subscribe(events.GameState, func(p any) {
		if st := p.(State); st.State == "exited" && st.RunID == sp.runID {
			exited <- st
		}
	})
	l := e.launcher()
	l.Recover()
	if !l.Busy(e.inst.ID) || !l.Running(e.inst.ID) || !l.AccountInUse("acc") {
		t.Fatal("the running game wasn't adopted")
	}
	// The adopted view still holds the game folder: another run gets a copy.
	other, err := l.cfg.Views.Assemble(runview.Request{Game: game, Scratch: filepath.Join(e.dir, "other.view")})
	if err != nil {
		t.Fatal(err)
	}
	if other.GameDir() == game {
		t.Fatal("a second run got the adopted game's folder directly")
	}
	_, _ = other.Close(context.Background())

	if err := l.Stop(e.inst.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case st := <-exited:
		if st.Result != ResultOK {
			t.Fatalf("result %q", st.Result)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the adopted run never finished")
	}
	if l.Busy(e.inst.ID) {
		t.Fatal("still running after the game exited")
	}
	if recs := l.Runs(e.inst.ID); len(recs) != 1 || recs[0].ID != sp.runID {
		t.Fatalf("records %+v", recs)
	}
}
