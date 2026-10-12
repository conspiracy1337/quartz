//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"quartz/internal/accounts"
	"quartz/internal/core/events"
	"quartz/internal/core/logging"
	"quartz/internal/core/sysinfo"
	"quartz/internal/core/tasks"
	"quartz/internal/install"
	"quartz/internal/instance"
	"quartz/internal/java"
	"quartz/internal/launch"
	"quartz/internal/mojang"
	"quartz/internal/settings"
)

func (h *harness) installer() *install.Installer {
	s := filepath.Join(h.root, "store")
	return install.New(h.meta, h.dl, h.java, install.Dirs{
		Libraries: filepath.Join(s, "libraries"), Versions: filepath.Join(s, "versions"), Assets: filepath.Join(s, "assets"),
		LogConfigs: filepath.Join(s, "logconfigs"), Natives: filepath.Join(s, "natives"),
	})
}

// TestInstallVanilla installs two versions completely, then proves a second
// pass plans nothing and needs no network: a launch of an installed game only
// stats files.
func TestInstallVanilla(t *testing.T) {
	h := newHarness(t)
	in := h.installer()
	env := mojang.CurrentEnv(sysinfo.OSVersion())
	for _, id := range []string{"1.21.4", "1.12.2"} {
		t.Run(id, func(t *testing.T) {
			gameDir := filepath.Join(h.root, "instances", id, "game")
			res, err := in.Resolve(h.ctx(), id, gameDir, env)
			if err != nil {
				t.Fatal(err)
			}
			_, plan, err := h.java.Resolve(h.ctx(), java.Request{Major: res.Version.JavaMajor(), Component: res.Version.JavaComponent()})
			if err != nil {
				t.Fatal(err)
			}
			spec, err := in.Plan(install.Request{Resolved: res, Java: plan, Meta: tasks.Meta{Title: id}})
			if err != nil {
				t.Fatal(err)
			}
			if spec != nil {
				start := time.Now()
				h.run(spec)
				t.Logf("%s installed in %s", id, time.Since(start).Round(time.Millisecond))
			}
			start := time.Now()
			res, err = in.Resolve(h.ctx(), id, gameDir, env)
			if err != nil {
				t.Fatal(err)
			}
			again, err := in.Plan(install.Request{Resolved: res, Meta: tasks.Meta{Title: id}})
			took := time.Since(start)
			if err != nil || again != nil {
				t.Fatalf("second plan not empty: %v", err)
			}
			t.Logf("%s: installed-game check took %s", id, took.Round(time.Microsecond))
			if took > 500*time.Millisecond {
				t.Errorf("installed-game check too slow: %s", took)
			}
		})
	}
}

// TestLaunchOffline starts a real game window with an offline account. It
// needs a display and QUARTZ_IT_LAUNCH=1.
func TestLaunchOffline(t *testing.T) {
	if os.Getenv("QUARTZ_IT_LAUNCH") != "1" || (os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "") {
		t.Skip("set QUARTZ_IT_LAUNCH=1 with a display to open a game window")
	}
	version := os.Getenv("QUARTZ_IT_VERSION")
	if version == "" {
		version = "1.21.4"
	}
	h := newHarness(t)
	bus := events.NewBus()
	accs, err := accounts.Open(filepath.Join(h.root, "accounts.json"), h.log)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = accs.AddOffline("Quartz_Test")
	insts, err := instance.Open(filepath.Join(h.root, "instances"), h.log)
	if err != nil {
		t.Fatal(err)
	}
	st, err := settings.Open(filepath.Join(h.root, "settings.json"), bus, h.log)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := insts.Create(instance.CreateRequest{Name: "Launch test " + version, Minecraft: version}, instance.JavaSettings{})
	if err != nil {
		t.Fatal(err)
	}
	red := logging.NewRedactor("")
	l := launch.New(launch.Config{
		Instances: insts, Accounts: accs, Installer: h.installer(), Java: h.java, Tasks: h.tasks,
		Settings: st, Bus: bus, Redactor: red, BinDir: filepath.Join(h.root, "store", "bin"), Log: h.log,
	})

	var (
		mu     sync.Mutex
		lines  []string
		states = make(chan launch.State, 4)
		failed = make(chan launch.Failure, 1)
	)
	bus.Subscribe(events.GameLog, func(p any) {
		mu.Lock()
		for _, ln := range p.(launch.LogBatch).Lines {
			lines = append(lines, string(ln.Level)+" ["+ln.Thread+"] "+ln.Message)
		}
		mu.Unlock()
	})
	bus.Subscribe(events.GameState, func(p any) { states <- p.(launch.State) })
	bus.Subscribe(events.LaunchFailed, func(p any) { failed <- p.(launch.Failure) })

	if err := l.Play(inst.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-states:
		if s.State != "running" {
			t.Fatalf("first state %+v", s)
		}
	case f := <-failed:
		t.Fatalf("launch failed at stage %d: %s", f.Stage, f.Message)
	case <-time.After(10 * time.Minute):
		t.Fatal("game never started")
	}
	deadline := time.Now().Add(90 * time.Second)
	for {
		mu.Lock()
		joined := strings.Join(lines, "\n")
		mu.Unlock()
		if strings.Contains(joined, "Setting user: Quartz_Test") || strings.Contains(joined, "LWJGL") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no game log seen:\n%s", joined)
		}
		time.Sleep(200 * time.Millisecond)
	}
	time.Sleep(5 * time.Second) // let the window appear
	if err := l.Stop(inst.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-states:
		if s.State != "exited" || (s.Result != launch.ResultOK && s.Result != launch.ResultKilled) {
			t.Fatalf("exit state %+v", s)
		}
		t.Logf("exited: %+v", s)
	case <-time.After(30 * time.Second):
		t.Fatal("game did not stop")
	}
	runs := l.Runs(inst.ID)
	if len(runs) != 1 || runs[0].Account != "Quartz_Test" {
		t.Fatalf("runs = %+v", runs)
	}
	mu.Lock()
	t.Logf("first lines:\n%s", strings.Join(lines[:min(len(lines), 15)], "\n"))
	mu.Unlock()
	if got, _ := insts.Get(inst.ID); got.LastPlayed == nil {
		t.Fatal("last played not recorded")
	}
}
