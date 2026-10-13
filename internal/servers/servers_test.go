package servers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quartz/internal/core/slp"
	"quartz/internal/instance"
	"quartz/internal/launch"
)

type fakePinger struct{ status map[string]*slp.Status }

func (f fakePinger) Get(_ context.Context, a slp.Address, _ int, _ bool) slp.Result {
	if st, ok := f.status[a.Key()]; ok {
		return slp.Result{Status: st}
	}
	return slp.Result{Err: errors.New("down")}
}

type env struct {
	svc   *Service
	insts *instance.Store
	a, b  instance.Instance
	busy  bool
	runs  map[string][]launch.Record
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	insts, err := instance.Open(filepath.Join(dir, "instances"), nil)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{insts: insts, runs: map[string][]launch.Record{}}
	e.a, _ = insts.Create(instance.CreateRequest{Name: "Vanilla", Minecraft: "1.21.4"}, instance.JavaSettings{})
	e.b, _ = insts.Create(instance.CreateRequest{Name: "Modded", Minecraft: "1.20.1"}, instance.JavaSettings{})
	ping := fakePinger{status: map[string]*slp.Status{
		"smp.kestrel.gg": {Version: "Paper 1.21.4", Protocol: 769, Online: 6, Max: 20, Latency: 21 * time.Millisecond},
		"mc.hypixel.net": {Version: "Requires MC 1.8 / 1.21", Protocol: 769, Online: 41208, Max: 200000, Latency: 38 * time.Millisecond},
	}}
	e.svc = New(Deps{
		Instances: insts, Busy: func(string) bool { return e.busy }, Pinger: ping,
		Protocol: func(mc string) int { return map[string]int{"1.21.4": 769, "1.20.1": 763}[mc] },
		Runs:     func(id string) []launch.Record { return e.runs[id] },
		Favicons: NewFavicons(filepath.Join(dir, "favicons")),
	})
	return e
}

func TestFixtureRoundTrip(t *testing.T) {
	e := newEnv(t)
	raw, _ := os.ReadFile("testdata/servers-xmcl.dat")
	p := filepath.Join(e.insts.GameDir(e.a.ID), "servers.dat")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, raw, 0o644)
	list, err := e.svc.entries(e.a.ID)
	if err != nil || len(list) == 0 {
		t.Fatalf("%v %v", list, err)
	}
	// Moving a server down and back up leaves the file as it was.
	first := list[0]
	if len(list) > 1 {
		if err := e.svc.Move(e.a.ID, first.Index, first.Address, 1); err != nil {
			t.Fatal(err)
		}
		moved, _ := e.svc.entries(e.a.ID)
		if moved[1].Address != first.Address {
			t.Fatalf("move: %+v", moved[:2])
		}
		if err := e.svc.Move(e.a.ID, moved[1].Index, first.Address, -1); err != nil {
			t.Fatal(err)
		}
		back, _ := os.ReadFile(p)
		if string(back) != string(raw) {
			t.Fatal("servers.dat changed after moving down and up")
		}
	}
}

func TestEditCycle(t *testing.T) {
	e := newEnv(t)
	id := e.a.ID
	if err := e.svc.Add(id, Entry{Name: "Friends SMP", Address: "smp.kestrel.gg"}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Add(id, Entry{Name: "Hypixel", Address: "mc.hypixel.net", Packs: PacksDisabled}); err != nil {
		t.Fatal(err)
	}
	list, err := e.svc.List(context.Background(), id, false)
	if err != nil || len(list) != 2 {
		t.Fatalf("%v %v", list, err)
	}
	if list[0].Packs != PacksPrompt || list[1].Packs != PacksDisabled || !list[0].Online || list[0].PingMs != 21 || list[0].Version != "1.21.4" {
		t.Fatalf("%+v", list)
	}
	if err := e.svc.Edit(id, 0, "smp.kestrel.gg", Entry{Name: "SMP", Address: "smp.kestrel.gg:25570", Packs: PacksEnabled}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Edit(id, 0, "smp.kestrel.gg", Entry{Name: "x", Address: "y.z"}); !errors.Is(err, ErrChanged) {
		t.Fatal("stale edit accepted")
	}
	token, err := e.svc.Remove(id, 1, "mc.hypixel.net")
	if err != nil || e.svc.Count(id) != 1 {
		t.Fatalf("remove %v", err)
	}
	if err := e.svc.Undo(id, token); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Undo(id, token); !errors.Is(err, ErrExpired) {
		t.Fatal("double undo")
	}
	list, _ = e.svc.List(context.Background(), id, false)
	if len(list) != 2 || list[0].Name != "SMP" || list[0].Packs != PacksEnabled || list[1].Address != "mc.hypixel.net" {
		t.Fatalf("%+v", list)
	}
	e.busy = true
	if err := e.svc.Add(id, Entry{Name: "x", Address: "x.y"}); !errors.Is(err, ErrBusy) {
		t.Fatal("edit while running")
	}
	e.busy = false
	for _, bad := range []Entry{{Name: "", Address: "a.b"}, {Name: "n", Address: "a b"}, {Name: "n", Address: "a.b", Packs: "always"}, {Name: strings.Repeat("x", 65), Address: "a.b"}} {
		if err := e.svc.Add(id, bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}

func TestNeedsAndVersionLabel(t *testing.T) {
	e := newEnv(t)
	_ = e.svc.Add(e.b.ID, Entry{Name: "SMP", Address: "smp.kestrel.gg"})
	list, _ := e.svc.List(context.Background(), e.b.ID, false)
	if list[0].Needs != "1.21.4" {
		t.Fatalf("needs %q", list[0].Needs)
	}
	for in, want := range map[string]string{"Paper 1.21.4": "1.21.4", "Requires MC 1.8 / 1.21": "1.8 to 1.21", "BungeeCord": "BungeeCord", "1.20.1": "1.20.1"} {
		if got := VersionLabel(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}

func TestHomeMerges(t *testing.T) {
	e := newEnv(t)
	_ = e.svc.Add(e.a.ID, Entry{Name: "Friends SMP", Address: "smp.kestrel.gg"})
	_ = e.svc.Add(e.a.ID, Entry{Name: "Old", Address: "old.example.net"})
	_ = e.svc.Add(e.b.ID, Entry{Name: "friends", Address: "SMP.Kestrel.gg.:25565"})
	_ = e.svc.Add(e.b.ID, Entry{Name: "Hypixel", Address: "mc.hypixel.net"})
	e.runs[e.b.ID] = []launch.Record{{StartedAt: time.Now().UnixMilli(), Server: "smp.kestrel.gg"}}
	home, err := e.svc.Home(context.Background(), false)
	if err != nil || len(home) != 3 {
		t.Fatalf("%+v %v", home, err)
	}
	var smp Merged
	for _, m := range home {
		if m.ID == "smp.kestrel.gg" {
			smp = m
		}
	}
	if len(smp.Instances) != 2 || smp.Instances[0] != e.b.ID || smp.Name != "friends" {
		t.Fatalf("smp %+v", smp)
	}
	if home[len(home)-1].Online {
		t.Fatal("offline servers go last")
	}
	// A later run on the other instance moves it first.
	e.svc.FinishRun(&launch.Finished{InstanceID: e.a.ID, Started: time.Now().Add(time.Minute), Record: &launch.Record{Server: "smp.kestrel.gg"}})
	home, _ = e.svc.Home(context.Background(), false)
	for _, m := range home {
		if m.ID == "smp.kestrel.gg" && m.Instances[0] != e.a.ID {
			t.Fatalf("last used not updated: %+v", m)
		}
	}
}
