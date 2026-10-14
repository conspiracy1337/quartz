//go:build integration

package integration

import (
	"archive/zip"
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"quartz/internal/accounts"
	"quartz/internal/content"
	"quartz/internal/content/store"
	"quartz/internal/core/events"
	"quartz/internal/core/localimg"
	"quartz/internal/core/logging"
	"quartz/internal/core/nbt"
	"quartz/internal/core/slp"
	"quartz/internal/host"
	"quartz/internal/icons"
	"quartz/internal/instance"
	"quartz/internal/loader"
	"quartz/internal/sandbox"
	"quartz/internal/sandbox/rules"
	"quartz/internal/settings"
	"quartz/internal/worlds"
)

// TestHost runs real dedicated servers (no window: they run with nogui):
// QUARTZ_IT_HOST=1 go test -tags integration -run TestHost ./internal/integration/
// QUARTZ_IT_HOST_ONLY=fabric picks one case.
func TestHost(t *testing.T) {
	if os.Getenv("QUARTZ_IT_HOST") != "1" {
		t.Skip("set QUARTZ_IT_HOST=1 to download and run real servers")
	}
	cases := []struct{ name, mc, loader string }{
		{"vanilla", "1.21.4", ""},
		{"fabric", "1.21.4", "fabric"},
		{"forge", "1.20.1", "forge"},
		{"neoforge", "1.21.1", "neoforge"},
		{"forge-legacy", "1.12.2", "forge"},
	}
	// QUARTZ_IT_HOST_LATEST=<version> adds a vanilla case on that release.
	if v := os.Getenv("QUARTZ_IT_HOST_LATEST"); v != "" {
		cases = append(cases, struct{ name, mc, loader string }{"vanilla-latest", v, ""})
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
	blobs, err := store.Open(filepath.Join(h.root, "store", "content"), nil, h.log)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := content.New(content.Deps{Instances: insts, Store: blobs, Icons: icons.OpenSized(filepath.Join(h.root, "icons"), 64), Bus: bus,
		Running: func(string) bool { return false }, ClientJar: func(string) string { return "" }, Log: h.log}, filepath.Join(h.root, "store", "content"))
	if err != nil {
		t.Fatal(err)
	}
	ws := worlds.New(worlds.Deps{Instances: insts, Images: localimg.New(filepath.Join(h.root, "thumbs")), Busy: func(string) bool { return false }, Bus: bus, Log: h.log})
	// QUARTZ_IT_SANDBOX=standard|strict hosts each server in its sandbox.
	preset := os.Getenv("QUARTZ_IT_SANDBOX")
	svc := host.New(host.Deps{Instances: insts, Installer: h.installer(), Java: h.java, Tasks: h.tasks, Content: cs, Store: blobs, Worlds: ws,
		Accounts: accs, Settings: st, Bus: bus, Redactor: logging.NewRedactor(""), Log: h.log,
		Sandbox: sandbox.New(sandbox.Config{DataRoot: h.root, Log: h.log}), StoreRead: h.storeRead()})

	var mu sync.Mutex
	var lines []string
	states := make(chan host.State, 64)
	bus.Subscribe(events.HostState, func(p any) { states <- p.(host.State) })
	bus.Subscribe(events.HostLog, func(p any) {
		mu.Lock()
		for _, l := range p.(host.LogBatch).Lines {
			lines = append(lines, l.Message)
		}
		mu.Unlock()
	})
	logged := func(sub string) bool {
		mu.Lock()
		defer mu.Unlock()
		for _, l := range lines {
			if strings.Contains(l, sub) {
				return true
			}
		}
		return false
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if only := os.Getenv("QUARTZ_IT_HOST_ONLY"); only != "" && only != c.name {
				t.Skip()
			}
			mu.Lock()
			lines = nil
			mu.Unlock()
			req := instance.CreateRequest{Name: "Host test " + c.name, Minecraft: c.mc}
			if c.loader != "" {
				// QUARTZ_IT_LOADER_VERSION pins a build, for when an upstream's version list is down.
				build := os.Getenv("QUARTZ_IT_LOADER_VERSION")
				if build == "" {
					v, err := h.loaders().Latest(h.ctx(), loader.Kind(c.loader), c.mc)
					if err != nil {
						t.Fatal(err)
					}
					build = v.ID
				}
				req.Loader, req.LoaderVersion = c.loader, build
			}
			req.Sandbox = preset != ""
			in, err := insts.Create(req)
			if err != nil {
				t.Fatal(err)
			}
			if preset != "" {
				if in, err = insts.EditSandbox(in.ID, func(s *rules.Settings) error { *s = s.Apply(rules.Preset(preset), runtime.GOOS); return nil }); err != nil {
					t.Fatal(err)
				}
			}
			if c.loader == "fabric" {
				mods := filepath.Join(insts.GameDir(in.ID), "mods")
				fabricMod(t, filepath.Join(mods, "qtest-client.jar"), "qtest_client", "client")
				fabricMod(t, filepath.Join(mods, "qtest-server.jar"), "qtest_server", "*")
			}
			port := freePort(t)
			if err := svc.Start(in.ID, host.Options{Port: port, MaxPlayers: 10, MemoryMB: 2048}); err != nil {
				t.Fatal(err)
			}
			// A failing case still stops its server (it runs in its own process group).
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
				defer cancel()
				svc.Close(ctx)
			})
			wait(t, states, "running", 15*time.Minute)
			if preset != "" && runtime.GOOS == "linux" {
				// The recorded server process is the sandbox around java.
				pid, _ := os.ReadFile(filepath.Join(svc.Dir(in.ID), ".quartz", "pid"))
				comm, _ := os.ReadFile(filepath.Join("/proc", strings.TrimSpace(string(pid)), "comm"))
				if strings.TrimSpace(string(comm)) != "bwrap" {
					t.Fatalf("the server runs as %q, not in its sandbox", comm)
				}
			}

			// Old servers fill in players on their first status refresh after Done.
			var ping *slp.Status
			for range 20 {
				if ping, err = slp.Ping(h.ctx(), slp.Address{Host: "127.0.0.1", Port: port}, -1); err == nil && ping.Max > 0 {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
			if err != nil || ping.Online != 0 || ping.Max != 10 {
				t.Fatalf("ping %+v %v", ping, err)
			}
			if c.loader == "fabric" {
				entries, _ := os.ReadDir(filepath.Join(svc.Dir(in.ID), "mods"))
				if len(entries) != 1 || entries[0].Name() != "qtest-server.jar" || !logged("qtest_server") {
					t.Fatalf("server mods %v", entries)
				}
			}
			if err := svc.Command(in.ID, "/list"); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(30 * time.Second)
			for !logged("players online") && time.Now().Before(deadline) {
				time.Sleep(200 * time.Millisecond)
			}
			if !logged("players online") {
				t.Fatal("the list command got no answer")
			}
			if err := svc.Stop(in.ID); err != nil {
				t.Fatal(err)
			}
			end := wait(t, states, "stopped", time.Minute)
			if end.Result != "ok" {
				t.Fatalf("stopped with %+v", end)
			}
			world := filepath.Join(insts.Path(in.ID, "saves"), "New world")
			if _, err := os.Stat(filepath.Join(world, "level.dat")); err != nil {
				t.Fatalf("the new world was saved in the instance: %v", err)
			}
			hostAgainAsHard(t, svc, ws, states, in.ID, world, port)
		})
	}
}

func wait(t *testing.T, states <-chan host.State, want string, d time.Duration) host.State {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	for {
		select {
		case s := <-states:
			if s.State == want {
				return s
			}
			if s.State == "stopped" {
				t.Fatalf("server stopped while waiting for %s: %+v", want, s)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s", want)
		}
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// fabricMod writes a mod with no code: Fabric loads it and lists it.
func fabricMod(t *testing.T, path, id, env string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("fabric.mod.json")
	_, _ = w.Write([]byte(`{"schemaVersion":1,"id":"` + id + `","version":"1.0.0","environment":"` + env + `"}`))
	_ = zw.Close()
	_ = f.Close()
}

// hostAgainAsHard hosts the world again after making it a Hard world with
// cheats in singleplayer: the server must keep its difficulty (it forces
// server.properties' onto the world) and make the host an operator.
func hostAgainAsHard(t *testing.T, svc *host.Service, ws *worlds.Service, states <-chan host.State, id, world string, port int) {
	t.Helper()
	lvl := filepath.Join(world, "level.dat")
	f, err := nbt.ReadFile(lvl)
	if err != nil {
		t.Fatal(err)
	}
	data := f.Root.Get("Data")
	t.Logf("level.dat Data keys: %v", data.Comp.Names)
	if ds := data.Get("difficulty_settings"); ds != nil {
		ds.Comp.Set("difficulty", nbt.NewString("hard"))
	} else {
		data.Comp.Set("Difficulty", nbt.NewByte(3))
	}
	data.Comp.Set("allowCommands", nbt.NewByte(1))
	if err := nbt.WriteFile(lvl, f); err != nil {
		t.Fatal(err)
	}
	if seed, err := ws.Seed(id, "New world"); err != nil || seed == "0" {
		t.Errorf("seed %q %v", seed, err)
	}
	if err := svc.Start(id, host.Options{World: "New world", Port: port, MaxPlayers: 10, MemoryMB: 2048}); err != nil {
		t.Fatal(err)
	}
	wait(t, states, "running", 5*time.Minute)
	props, _ := os.ReadFile(filepath.Join(svc.Dir(id), "server.properties"))
	if !strings.Contains(string(props), "difficulty=hard") && !strings.Contains(string(props), "difficulty=3") {
		t.Errorf("server.properties:\n%s", props)
	}
	ops, _ := os.ReadFile(filepath.Join(svc.Dir(id), "ops.json"))
	opsTxt, _ := os.ReadFile(filepath.Join(svc.Dir(id), "ops.txt"))
	if !strings.Contains(string(ops), accounts.OfflineUUID("Quartz_Test")) && !strings.Contains(string(opsTxt), "Quartz_Test") {
		t.Errorf("the host isn't an operator: %s %s", ops, opsTxt)
	}
	if err := svc.Stop(id); err != nil {
		t.Fatal(err)
	}
	if end := wait(t, states, "stopped", time.Minute); end.Result != "ok" {
		t.Fatalf("stopped with %+v", end)
	}
	after := worlds.LevelData(world)
	d := after.Get("Difficulty").Int()
	if ds := after.Get("difficulty_settings"); ds != nil {
		if ds.Get("difficulty").Text() == "hard" {
			d = 3
		}
	}
	if d != 3 {
		t.Errorf("the world isn't Hard any more: %v", after.Comp.Names)
	}
	for _, p := range []string{"playerdata", "players"} {
		if entries, err := os.ReadDir(filepath.Join(world, p)); err == nil {
			t.Logf("%s/: %d entries", p, len(entries))
		}
	}
	if _, err := os.Stat(filepath.Join(world, "data", "minecraft", "world_gen_settings.dat")); err == nil {
		t.Log("seed lives in data/minecraft/world_gen_settings.dat")
	}
}
