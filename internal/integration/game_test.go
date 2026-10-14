//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"runtime"
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
	"quartz/internal/loader"
	"quartz/internal/mojang"
	"quartz/internal/runview"
	"quartz/internal/sandbox"
	"quartz/internal/sandbox/rules"
	"quartz/internal/settings"
	"quartz/internal/syncing"
)

func (h *harness) loaders() *loader.Registry {
	s := filepath.Join(h.root, "store")
	return loader.New(loader.Deps{Meta: h.meta, Downloader: h.dl, Verified: h.ver, Log: h.log,
		Dirs: loader.Dirs{Libraries: filepath.Join(s, "libraries"), Versions: filepath.Join(s, "versions")}})
}

// storeRead are the store folders a sandboxed game or server reads.
func (h *harness) storeRead() []string {
	s := filepath.Join(h.root, "store")
	var out []string
	for _, d := range []string{"libraries", "versions", "assets", "natives", "logconfigs", "bin"} {
		out = append(out, filepath.Join(s, d))
	}
	return out
}

func (h *harness) installer() *install.Installer {
	s := filepath.Join(h.root, "store")
	return install.New(h.meta, h.dl, h.java, h.loaders(), install.Dirs{
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
			res, err := in.Resolve(h.ctx(), install.Target{Minecraft: id}, gameDir, env)
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
			res, err = in.Resolve(h.ctx(), install.Target{Minecraft: id}, gameDir, env)
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
	req := instance.CreateRequest{Name: "Launch test " + version, Minecraft: version}
	if k := os.Getenv("QUARTZ_IT_LOADER"); k != "" {
		v, err := h.loaders().Latest(h.ctx(), loader.Kind(k), version)
		if err != nil {
			t.Fatal(err)
		}
		req.Loader, req.LoaderVersion, req.Name = k, v.ID, req.Name+" "+k
	}
	var (
		mu     sync.Mutex
		lines  []string
		states = make(chan launch.State, 4)
		failed = make(chan launch.Failure, 1)
	)
	// QUARTZ_IT_SANDBOX=standard|strict runs the game in its sandbox.
	preset := os.Getenv("QUARTZ_IT_SANDBOX")
	req.Sandbox = preset != ""
	inst, err := insts.Create(req)
	if err != nil {
		t.Fatal(err)
	}
	if preset != "" {
		if inst, err = insts.EditSandbox(inst.ID, func(s *rules.Settings) error { *s = s.Apply(rules.Preset(preset), runtime.GOOS); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	red := logging.NewRedactor("")
	mgr := accounts.NewManager(accounts.Config{Store: accs, Bus: bus, Redactor: red, Settings: func() settings.Accounts { return st.Get().Accounts }})
	l := launch.New(launch.Config{
		Instances: insts, Accounts: mgr, Installer: h.installer(), Loaders: h.loaders(), Java: h.java, Tasks: h.tasks,
		Settings: st, Bus: bus, Redactor: red, BinDir: filepath.Join(h.root, "store", "bin"), Log: h.log,
		Views: runview.New(runview.Config{Log: h.log}), Sandbox: sandbox.New(sandbox.Config{DataRoot: h.root, Log: h.log}), StoreRead: h.storeRead(),
	})
	progress := map[int]string{}
	bus.Subscribe(events.LaunchProgress, func(p any) {
		pr := p.(launch.Progress)
		mu.Lock()
		for i, d := range pr.Details {
			if d != "" {
				progress[i] = d
			}
		}
		mu.Unlock()
	})

	bus.Subscribe(events.GameLog, func(p any) {
		mu.Lock()
		for _, ln := range p.(launch.LogBatch).Lines {
			lines = append(lines, string(ln.Level)+" ["+ln.Thread+"] "+ln.Message)
		}
		mu.Unlock()
	})
	bus.Subscribe(events.GameState, func(p any) { states <- p.(launch.State) })
	bus.Subscribe(events.LaunchFailed, func(p any) { failed <- p.(launch.Failure) })

	if err := l.Play(inst.ID, "", instance.Into{}); err != nil {
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
	if len(runs) != 1 || runs[0].Account != "Quartz_Test" || (preset != "") != (runs[0].Sandbox != "") {
		t.Fatalf("runs = %+v", runs)
	}
	mu.Lock()
	t.Logf("sandbox step: %q", progress[launch.StageSandbox])
	t.Logf("last lines:\n%s", strings.Join(lines[max(0, len(lines)-12):], "\n"))
	t.Logf("first lines:\n%s", strings.Join(lines[:min(len(lines), 15)], "\n"))
	var problems []string
	for _, ln := range lines {
		if strings.HasPrefix(ln, "WARN") || strings.HasPrefix(ln, "ERROR") {
			problems = append(problems, ln)
		}
	}
	t.Logf("warnings and errors (%d):\n%s", len(problems), strings.Join(problems[:min(len(problems), 40)], "\n"))
	mu.Unlock()
	if got, _ := insts.Get(inst.ID); got.LastPlayed == nil {
		t.Fatal("last played not recorded")
	}
}

// TestTwoCopies starts two real game windows of one instance with the same
// offline account, sharing worlds: the first sees the shared folder in
// place, the second a private view that merges back when it stops. It needs
// a display and QUARTZ_IT_LAUNCH=1.
func TestTwoCopies(t *testing.T) {
	if os.Getenv("QUARTZ_IT_LAUNCH") != "1" || (os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "") {
		t.Skip("set QUARTZ_IT_LAUNCH=1 with a display to open two game windows")
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
	inst, err := insts.Create(instance.CreateRequest{Name: "Two copies", Minecraft: "1.21.4"})
	if err != nil {
		t.Fatal(err)
	}
	shared := syncing.New(syncing.Config{Root: filepath.Join(h.root, "shared"), Trash: filepath.Join(h.root, "trash"), Settings: st, Instances: insts, Bus: bus, Log: h.log})
	if err := shared.JoinAll(h.ctx(), inst.ID, []string{"saves", "options"}); err != nil {
		t.Fatal(err)
	}
	red := logging.NewRedactor("")
	mgr := accounts.NewManager(accounts.Config{Store: accs, Bus: bus, Redactor: red, Settings: func() settings.Accounts { return st.Get().Accounts }})
	l := launch.New(launch.Config{
		Instances: insts, Accounts: mgr, Installer: h.installer(), Loaders: h.loaders(), Java: h.java, Tasks: h.tasks,
		Settings: st, Bus: bus, Redactor: red, BinDir: filepath.Join(h.root, "store", "bin"), Log: h.log,
		Syncing: shared, Views: runview.New(runview.Config{Log: h.log}),
	})
	states := make(chan launch.State, 8)
	failed := make(chan launch.Failure, 2)
	bus.Subscribe(events.GameState, func(p any) { states <- p.(launch.State) })
	bus.Subscribe(events.LaunchFailed, func(p any) { failed <- p.(launch.Failure) })
	wait := func(want string) launch.State {
		t.Helper()
		select {
		case s := <-states:
			if s.State != want {
				t.Fatalf("state %+v, want %s", s, want)
			}
			return s
		case f := <-failed:
			t.Fatalf("launch failed at stage %d: %s", f.Stage, f.Message)
		case <-time.After(10 * time.Minute):
			t.Fatalf("no %s state", want)
		}
		return launch.State{}
	}
	for copies := 1; copies <= 2; copies++ {
		if err := l.Play(inst.ID, "", instance.Into{}); err != nil {
			t.Fatal(err)
		}
		if s := wait("running"); s.Copies != copies {
			t.Fatalf("copies = %d, want %d", s.Copies, copies)
		}
		time.Sleep(20 * time.Second) // let the game write its options
	}
	if err := l.Stop(inst.ID); err != nil {
		t.Fatal(err)
	}
	wait("exited")
	wait("exited")
	if l.Running(inst.ID) || len(l.Runs(inst.ID)) != 2 {
		t.Fatalf("running %v, runs %d", l.Running(inst.ID), len(l.Runs(inst.ID)))
	}
	if views, _ := filepath.Glob(filepath.Join(insts.RunsDir(inst.ID), "*.view")); len(views) != 0 {
		t.Fatalf("views left: %v", views)
	}
	if _, err := os.Stat(filepath.Join(insts.GameDir(inst.ID), "options.txt")); err != nil {
		t.Fatal("the game's options never reached the instance")
	}
	if _, err := os.Stat(filepath.Join(h.root, "shared", "options.txt")); err != nil {
		t.Fatal("the options never reached the shared copy")
	}
}
