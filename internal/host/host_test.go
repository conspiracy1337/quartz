package host

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"quartz/internal/core/events"
	"quartz/internal/core/nbt"
	"quartz/internal/gamelog"
)

func TestWriteProps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.properties")
	orig := "#Minecraft server properties\nmotd=My own words\nserver-port=1\nspawn-protection=0\ndifficulty = hard\n"
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeServerFiles(filepath.Dir(path), Options{Port: 25570, MaxPlayers: 8, LAN: false, Offline: true}, "Wörld\\1", "Ignored"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	text := string(got)
	for _, want := range []string{
		"#Minecraft server properties\n", "motd=My own words\n", "server-port=25570\n", "spawn-protection=0\n", "difficulty = hard\n",
		"max-players=8\n", "server-ip=127.0.0.1\n", "online-mode=false\n", "enforce-secure-profile=false\n",
		"enable-rcon=false\n", "enable-query=false\n", "level-name=W\\u00f6rld\\\\1\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	if eula, _ := os.ReadFile(filepath.Join(filepath.Dir(path), "eula.txt")); !strings.Contains(string(eula), "eula=true") {
		t.Fatal("the EULA is accepted")
	}
	// Shown on the network: every interface, and the motd only fills a gap.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := writeServerFiles(filepath.Dir(path), Options{Port: 25565, MaxPlayers: 10, LAN: true}, "w", "Survival §cplus\nmore"); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	if text := string(got); !strings.Contains(text, "server-ip=\n") || !strings.Contains(text, "online-mode=true\n") ||
		!strings.Contains(text, "motd=Survival cplusmore\n") || strings.Contains(text, "enforce-secure-profile") {
		t.Fatalf("lan properties:\n%s", text)
	}
}

func TestEscapeProp(t *testing.T) {
	for in, want := range map[string]string{"plain": "plain", " lead": `\ lead`, `a\b`: `a\\b`, "\u00e9": `\u00e9`, "\U0001F600": `\ud83d\ude00`, "x\ny": `x\ny`} {
		if got := escapeProp(in); got != want {
			t.Errorf("escapeProp(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCarryInventory(t *testing.T) {
	world := t.TempDir()
	uuid := "069a79f4-44e9-4726-a5be-fca90e38aaf5"
	root := nbt.NewCompound()
	data := nbt.NewCompound()
	data.Comp.Set("DataVersion", nbt.NewInt(4189))
	player := nbt.NewCompound()
	player.Comp.Set("XpLevel", nbt.NewInt(30))
	data.Comp.Set("Player", player)
	root.Comp.Set("Data", data)
	lvl := filepath.Join(world, "level.dat")
	if err := nbt.WriteFile(lvl, &nbt.File{Root: root, Gzip: true}); err != nil {
		t.Fatal(err)
	}
	if err := carryIn(world, uuid); err != nil {
		t.Fatal(err)
	}
	pf := filepath.Join(world, "playerdata", uuid+".dat")
	p, err := nbt.ReadFile(pf)
	if err != nil || p.Root.Get("XpLevel").Int() != 30 {
		t.Fatalf("the singleplayer player is the server's: %v", err)
	}

	// The server saves the player later: back in level.dat it goes.
	start := time.Now()
	p.Root.Comp.Set("XpLevel", nbt.NewInt(31))
	if err := nbt.WriteFile(pf, p); err != nil {
		t.Fatal(err)
	}
	later := start.Add(time.Minute)
	if err := os.Chtimes(pf, later, later); err != nil {
		t.Fatal(err)
	}
	if err := carryOut(world, uuid, start); err != nil {
		t.Fatal(err)
	}
	f, _ := nbt.ReadFile(lvl)
	if got := f.Root.Path("Data", "Player", "XpLevel").Int(); got != 31 {
		t.Fatalf("level.dat player XpLevel = %d", got)
	}
	// A server file newer than level.dat is not overwritten by carryIn.
	if err := carryIn(world, uuid); err != nil {
		t.Fatal(err)
	}
	if err := carryIn(world, "../../evil"); err == nil {
		t.Fatal("a bad player id must be refused")
	}
}

func TestBackupPrune(t *testing.T) {
	world := filepath.Join(t.TempDir(), "Northwatch")
	if err := os.MkdirAll(world, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(world, "level.dat"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "backups")
	base := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	for i := range 7 {
		if err := backupWorld(world, dir, base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != keepBackups || entries[0].Name() != "2026-10-13_12-02-00.zip" {
		t.Fatalf("kept %v", entries)
	}
}

func TestChecks(t *testing.T) {
	ok := Options{Port: 25565, MaxPlayers: 10, MemoryMB: 1024}
	if err := Check(ok); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		o   Options
		err error
	}{
		{Options{Port: 80, MaxPlayers: 10, MemoryMB: 1024}, ErrBadPort},
		{Options{Port: 70000, MaxPlayers: 10, MemoryMB: 1024}, ErrBadPort},
		{Options{Port: 25565, MaxPlayers: 0, MemoryMB: 1024}, ErrBadPlayers},
		{Options{Port: 25565, MaxPlayers: 10, MemoryMB: 100}, ErrBadMemory},
		{Options{Port: 25565, MaxPlayers: 10, MemoryMB: 1024, World: "../x"}, ErrNoWorld},
	} {
		if err := Check(c.o); !errors.Is(err, c.err) {
			t.Errorf("%+v: %v, want %v", c.o, err, c.err)
		}
	}
	for in, want := range map[string]string{"/op Kestrel_Vane": "op Kestrel_Vane", " list ": "list"} {
		if got, err := CleanCommand(in); err != nil || got != want {
			t.Errorf("CleanCommand(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "/", "say a\nop me", "say \x1b[2J", strings.Repeat("x", 257)} {
		if _, err := CleanCommand(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if got := string(lanPayload("Survival [beta]", 25570)); got != "[MOTD]Survival beta[/MOTD][AD]25570[/AD]" {
		t.Fatalf("payload %q", got)
	}
}

type noRedact struct{}

func (noRedact) Redact(s string) string { return s }

// fakeServer is a shell script that behaves like a server's console.
const fakeServer = `#!/bin/sh
echo '[12:00:00] [Server thread/INFO]: Starting minecraft server version 1.21.4'
echo "[12:00:00] [Server thread/INFO]: args: $*"
echo '[12:00:01] [Server thread/INFO]: Done (1.042s)! For help, type "help"'
while read line; do
  echo "[12:00:02] [Server thread/INFO]: got $line"
  if [ "$line" = "stop" ]; then [ -n "$STUBBORN" ] && continue; echo '[12:00:03] [Server thread/INFO]: Stopping the server'; exit 0; fi
done
`

type recorder struct {
	mu     sync.Mutex
	states []State
	lines  []gamelog.Line
}

func (r *recorder) has(state string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.states {
		if s.State == state {
			return true
		}
	}
	return false
}

func (r *recorder) last() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.states[len(r.states)-1]
}

func (r *recorder) logged(sub string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, l := range r.lines {
		if strings.Contains(l.Message, sub) {
			return true
		}
	}
	return false
}

func until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func fakeService(t *testing.T, script string) (*Service, *server, *recorder) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake server is a shell script")
	}
	bus := events.NewBus()
	rec := &recorder{}
	bus.Subscribe(events.HostState, func(p any) { rec.mu.Lock(); rec.states = append(rec.states, p.(State)); rec.mu.Unlock() })
	bus.Subscribe(events.HostLog, func(p any) { rec.mu.Lock(); rec.lines = append(rec.lines, p.(LogBatch).Lines...); rec.mu.Unlock() })
	s := New(Deps{Bus: bus, Redactor: noRedact{}, Log: slog.New(slog.DiscardHandler)})
	dir := t.TempDir()
	path := filepath.Join(dir, "server.sh")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil { //nolint:gosec // a test script must run
		t.Fatal(err)
	}
	_, cancel := context.WithCancel(context.Background())
	srv := &server{id: "inst", runID: "run", name: "Survival", opts: Options{Port: 1}, dir: dir, saves: filepath.Join(dir, "saves"), world: "Northwatch",
		cancel: cancel, done: make(chan struct{}), ping: make(chan struct{}, 1)}
	srv.st = State{InstanceID: "inst", RunID: "run", State: "starting"}
	s.cur = srv
	if err := s.spawn(context.Background(), srv, "/bin/sh", []string{path, "nogui"}); err != nil {
		t.Fatal(err)
	}
	return s, srv, rec
}

func TestServerLifecycle(t *testing.T) {
	s, srv, rec := fakeService(t, fakeServer)
	until(t, "running", func() bool { return rec.has("running") })
	if !s.Running("inst") || s.HostedWorld("inst") != "Northwatch" || len(s.Status()) != 1 {
		t.Fatal("the server is listed while it runs")
	}
	if err := s.Command("inst", "/op Kestrel_Vane"); err != nil {
		t.Fatal(err)
	}
	until(t, "the command's echo", func() bool { return rec.logged("got op Kestrel_Vane") })
	if !rec.logged("args: nogui") {
		t.Fatal("arguments reach the server")
	}
	if err := s.Stop("inst"); err != nil {
		t.Fatal(err)
	}
	<-srv.done
	if st := rec.last(); st.State != "stopped" || st.Result != "ok" {
		t.Fatalf("final state %+v", st)
	}
	if s.Running("inst") || len(s.Status()) != 0 {
		t.Fatal("the slot frees up")
	}
	if lines := s.Live("inst"); len(lines) == 0 || !strings.Contains(lines[len(lines)-1].Message, "Stopping") {
		t.Fatal("the last console stays readable")
	}
	if _, err := os.Stat(pidFile(srv.dir)); !os.IsNotExist(err) {
		t.Fatal("the pid file goes with the server")
	}
	if err := s.Stop("inst"); !errors.Is(err, ErrNotRunning) {
		t.Fatal(err)
	}
}

func TestSecondStopKills(t *testing.T) {
	t.Setenv("STUBBORN", "1")
	s, srv, rec := fakeService(t, fakeServer)
	until(t, "running", func() bool { return rec.has("running") })
	if err := s.Stop("inst"); err != nil {
		t.Fatal(err)
	}
	until(t, "stop reaching the console", func() bool { return rec.logged("got stop") })
	if err := s.Stop("inst"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-srv.done:
	case <-time.After(5 * time.Second):
		t.Fatal("a second Stop kills the server")
	}
}

func TestFailureBeforeReady(t *testing.T) {
	_, srv, rec := fakeService(t, "#!/bin/sh\necho 'Error: Unable to access jarfile server.jar' >&2\nexit 1\n")
	<-srv.done
	if st := rec.last(); st.Result != "failed" || st.Message != "Error: Unable to access jarfile server.jar" {
		t.Fatalf("final state %+v", st)
	}
}
