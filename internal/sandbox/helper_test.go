package sandbox

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"quartz/internal/core/landlock"
	"quartz/internal/core/procx"
	"quartz/internal/sandbox/rules"
)

// The jail tests start this test binary again inside a real sandbox; the
// helper then probes what it can reach and prints one "name=result" line
// per probe ("ok" or the error).
func TestMain(m *testing.M) {
	landlock.Main() // the fixture's runs go through this binary as the shim
	Main()          // and the Windows administrator helper (allowParentListing)
	if os.Getenv("QUARTZ_SANDBOX_HELPER") != "" {
		helper()
		return
	}
	os.Exit(m.Run())
}

func say(name string, err error) {
	r := "ok"
	if err != nil {
		r = err.Error()
		var en syscall.Errno
		if errors.As(err, &en) {
			r = en.Error()
		}
	}
	fmt.Printf("%s=%s\n", name, r)
}

func helper() {
	if os.Getenv("QUARTZ_SANDBOX_HELPER") == "sleep" {
		time.Sleep(time.Minute)
		return
	}
	say("write-game", os.WriteFile(filepath.Join(os.Getenv("GAME"), "made.txt"), []byte("x"), 0o600))
	_, err := os.ReadDir(os.Getenv("OTHER"))
	say("see-other", err)
	_, err = os.ReadFile(os.Getenv("SECRET"))
	say("see-secret", err)
	say("write-ro", os.WriteFile(filepath.Join(os.Getenv("RO"), "x"), []byte("x"), 0o600))
	_, err = os.ReadDir(os.Getenv("RO"))
	say("read-ro", err)
	say("write-rw", os.WriteFile(filepath.Join(os.Getenv("RW"), "x"), []byte("x"), 0o600))
	say("read-store", func() error { _, err := os.ReadFile(os.Getenv("STORE")); return err }())
	say("write-store", os.WriteFile(os.Getenv("STORE"), []byte("y"), 0o600))
	mod := filepath.Join(os.Getenv("GAME"), "mods", "a.jar")
	say("write-mod", func() error {
		if err := os.Chmod(mod, 0o600); err != nil {
			return err
		}
		return os.WriteFile(mod, []byte("infected"), 0o600)
	}())
	pack := filepath.Join(os.Getenv("GAME"), "resourcepacks", "pack.zip")
	say("write-pack", func() error {
		if err := os.Chmod(pack, 0o600); err != nil {
			return err
		}
		return os.WriteFile(pack, []byte("infected"), 0o600)
	}())
	say("add-pack-file", os.WriteFile(pack+".txt", []byte("x"), 0o600))
	say("write-skin", os.WriteFile(filepath.Join(os.Getenv("SKINS"), "ab.png"), []byte("x"), 0o600))
	say("add-mod", os.WriteFile(filepath.Join(os.Getenv("GAME"), "mods", "new.jar"), []byte("x"), 0o600))
	say("dial", func() error {
		c, err := net.DialTimeout("tcp", os.Getenv("LISTEN"), 2*time.Second)
		if err == nil {
			_ = c.Close()
		}
		return err
	}())
	platformProbes()
}

type jailFixture struct {
	s                         *Service
	root, game, other, secret string
	store, pack, ro, rw       string
	scratch, data, listen     string
	abstract                  string // a host abstract unix socket (Linux)
}

func newJailFixture(t *testing.T) *jailFixture {
	t.Helper()
	st := New(Config{}).Status()
	if !st.Available {
		t.Skip(st.Reason)
	}
	root := jailRoot(t, "jail-")
	f := &jailFixture{root: root, game: filepath.Join(root, "instances", "a", "game"), other: filepath.Join(root, "instances", "b", "game"),
		secret: filepath.Join(root, "accounts.json"), store: filepath.Join(root, "store", "libraries", "lib.jar"),
		pack:    filepath.Join(root, "store", "content", "pack.zip"),
		scratch: filepath.Join(root, "instances", "a", "runs", "1.view"), data: filepath.Join(root, "instances", "a", "sandbox")}
	outside := t.TempDir()
	f.ro, f.rw = filepath.Join(outside, "maps"), filepath.Join(outside, "notes")
	for _, d := range []string{f.game, f.other, f.scratch, f.ro, f.rw, filepath.Dir(f.store), filepath.Dir(f.pack), filepath.Join(f.game, "resourcepacks")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{f.secret, f.store, f.pack} {
		if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The instance's mod is the store's file itself, as a hardlink is,
	// sealed read-only.
	if err := os.MkdirAll(filepath.Join(f.game, "mods"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(f.store, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(f.store, filepath.Join(f.game, "mods", "a.jar")); err != nil {
		t.Fatal(err)
	}
	// So is a resource pack, in a folder the game may otherwise write.
	if err := os.Chmod(f.pack, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(f.pack, filepath.Join(f.game, "resourcepacks", "pack.zip")); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	f.listen = ln.Addr().String()
	if runtime.GOOS == "linux" {
		f.abstract = fmt.Sprintf("@quartz-sandbox-test-%d", os.Getpid())
		al, err := net.Listen("unix", f.abstract)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = al.Close() })
	}
	f.s = New(Config{DataRoot: root, Shim: landlock.Self})
	t.Cleanup(func() { _ = f.s.Forget("sandbox-test") })
	return f
}

// skins is the skin cache inside the shared (read-only) assets.
func (f *jailFixture) skins() string { return filepath.Join(f.root, "store", "assets", "skins") }

func (f *jailFixture) runArgs(t *testing.T, set rules.Settings, mode string) (procx.Spec, *Jail) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	set.Folders = []rules.Folder{{ID: "a1", Path: f.ro, Level: rules.RO}, {ID: "b2", Path: f.rw, Level: rules.RW}}
	r := Run{Instance: "sandbox-test", Rules: set, GameDir: f.game, Scratch: f.scratch, Data: f.data,
		Read:      []string{filepath.Dir(exe), filepath.Join(f.root, "store", "libraries"), filepath.Join(f.root, "store", "assets")},
		Redirects: []Redirect{{From: filepath.Join(f.data, "skins"), To: f.skins()}}}
	env := append(FilterEnv(os.Environ()), "QUARTZ_SANDBOX_HELPER="+mode, "GAME="+f.game, "OTHER="+f.other, "SECRET="+f.secret,
		"PARENT="+fmt.Sprint(os.Getpid()), "RO="+f.ro, "RW="+f.rw, "STORE="+f.store, "LISTEN="+f.listen, "SKINS="+f.skins(), "ABSTRACT="+f.abstract)
	spec, j, err := f.s.Wrap(r, procx.Spec{Argv: []string{exe}, Dir: f.game, Env: env})
	if err != nil {
		t.Fatal(err)
	}
	return spec, j
}

func (f *jailFixture) probe(t *testing.T, set rules.Settings) map[string]string {
	t.Helper()
	spec, j := f.runArgs(t, set, "probe")
	defer j.Close()
	c, err := procx.Start(spec)
	if err != nil {
		t.Fatal(err)
	}
	j.Started(c.Process.Pid)
	_ = c.Stdin.Close()
	errc := make(chan []byte, 1)
	go func() { b, _ := io.ReadAll(c.Stderr); errc <- b }()
	out, _ := io.ReadAll(c.Stdout)
	errOut := <-errc
	_ = c.Wait()
	got := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			got[k] = v
		}
	}
	if len(got) == 0 {
		t.Fatalf("the helper printed nothing: %s", errOut)
	}
	t.Logf("%v", got)
	return got
}

func TestJailStandard(t *testing.T) {
	f := newJailFixture(t)
	got := f.probe(t, rules.Default())
	for _, k := range []string{"write-game", "write-rw", "read-store", "read-ro"} {
		if got[k] != "ok" {
			t.Errorf("%s = %q, want ok", k, got[k])
		}
	}
	for _, k := range []string{"see-other", "see-secret", "write-ro", "write-store"} {
		if got[k] == "ok" {
			t.Errorf("%s: the jail reached it", k)
		}
	}
	if runtime.GOOS != "windows" && got["dial"] != "ok" { // app containers never reach loopback
		t.Errorf("dial = %q: Standard lost the network", got["dial"])
	}
	if _, err := os.Stat(filepath.Join(f.game, "made.txt")); err != nil {
		t.Error("the game's write didn't reach its folder")
	}
	// The mods folder is protected: whatever the game did there (refused,
	// or kept in a throwaway layer) never reached the store's file.
	if b, err := os.ReadFile(f.store); err != nil || string(b) != "data" {
		t.Errorf("the game changed a store file through its mods folder: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(f.game, "mods", "new.jar")); err == nil {
		t.Error("the game added a file to its mods folder")
	}
	// Store files in other content folders are protected one by one: the
	// game's own files beside them still work.
	if b, err := os.ReadFile(f.pack); err != nil || string(b) != "data" {
		t.Errorf("the game changed a store file through its resourcepacks folder: %q %v", b, err)
	}
	if got["add-pack-file"] != "ok" {
		t.Errorf("add-pack-file = %q: the game lost its own files beside a resource pack", got["add-pack-file"])
	}
	// The skin cache is the instance's own: the write lands in its data.
	if got["write-skin"] != "ok" {
		t.Errorf("write-skin = %q", got["write-skin"])
	}
	if runtime.GOOS == "linux" {
		if _, err := os.Stat(filepath.Join(f.data, "skins", "ab.png")); err != nil {
			t.Error("the skin didn't land in the instance's own cache")
		}
	}
	checkPlatform(t, got)
}

// Stop signals the game's process group (or posts WM_CLOSE, then kills),
// which reaches the jailed game.
func TestJailStopReachesGame(t *testing.T) {
	f := newJailFixture(t)
	spec, j := f.runArgs(t, rules.Default(), "sleep")
	defer j.Close()
	c, err := procx.Start(spec)
	if err != nil {
		t.Fatal(err)
	}
	j.Started(c.Process.Pid)
	go func() { _, _ = io.Copy(io.Discard, c.Stdout) }()
	go func() { _, _ = io.Copy(io.Discard, c.Stderr) }()
	time.Sleep(300 * time.Millisecond)
	if err := procx.Terminate(c.Process); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = procx.Kill(c.Process)
		t.Fatal("the jailed game outlived Stop")
	}
}

func TestFilterEnv(t *testing.T) {
	got := FilterEnv([]string{"PATH=/usr/bin", "SSH_AUTH_SOCK=/run/agent", "DBUS_SESSION_BUS_ADDRESS=unix:x", "LC_ALL=C",
		"GITHUB_TOKEN=secret", "DISPLAY=:0", "__GL_SYNC_TO_VBLANK=0", "broken", "XCURSOR_SIZE=48", "GBM_BACKEND=nvidia-drm", "__EGL_VENDOR_LIBRARY_FILENAMES=/x.json"})
	if strings.Join(got, " ") != "PATH=/usr/bin LC_ALL=C DISPLAY=:0 __GL_SYNC_TO_VBLANK=0 XCURSOR_SIZE=48 GBM_BACKEND=nvidia-drm __EGL_VENDOR_LIBRARY_FILENAMES=/x.json" {
		t.Fatal(got)
	}
	if e := setEnv([]string{"A=1", "B=2"}, "A=3"); strings.Join(e, " ") != "B=2 A=3" {
		t.Fatal(e)
	}
}
