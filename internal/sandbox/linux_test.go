//go:build linux

package sandbox

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"quartz/internal/core/procx"
	"quartz/internal/sandbox/rules"
)

// The helper runs inside the jail (the test binary started again) and
// prints one "name=result" line per probe.
func TestMain(m *testing.M) {
	if os.Getenv("QUARTZ_SANDBOX_HELPER") != "" {
		helper()
		return
	}
	os.Exit(m.Run())
}

func helper() {
	if os.Getenv("QUARTZ_SANDBOX_HELPER") == "sleep" {
		time.Sleep(time.Minute)
		return
	}
	say := func(name string, err error) {
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
	say("write-game", os.WriteFile(filepath.Join(os.Getenv("GAME"), "made.txt"), []byte("x"), 0o600))
	_, err := os.Stat(os.Getenv("OTHER"))
	say("see-other", err)
	_, err = os.Stat(os.Getenv("SECRET"))
	say("see-secret", err)
	var names []string
	_ = filepath.WalkDir(os.Getenv("HOME"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			names = append(names, strings.TrimPrefix(p, os.Getenv("HOME")))
		}
		return nil
	})
	say("home-files", fmt.Errorf("%s", strings.Join(names, ",")))
	_, err = os.Stat(filepath.Join("/proc", os.Getenv("PARENT")))
	say("see-parent", err)
	say("unshare", unix.Unshare(unix.CLONE_NEWUSER))
	say("ptrace", unix.PtraceAttach(1))
	c := byte('x')
	_, _, e := unix.Syscall(unix.SYS_IOCTL, 0, unix.TIOCSTI, uintptr(unsafe.Pointer(&c)))
	say("tiocsti", e)
	ifs, _ := net.Interfaces()
	say("interfaces", fmt.Errorf("%d", len(ifs)))
	say("write-ro", os.WriteFile(filepath.Join(os.Getenv("RO"), "x"), []byte("x"), 0o600))
	say("write-rw", os.WriteFile(filepath.Join(os.Getenv("RW"), "x"), []byte("x"), 0o600))
	say("read-store", func() error { _, err := os.ReadFile(os.Getenv("STORE")); return err }())
	say("write-store", os.WriteFile(os.Getenv("STORE"), []byte("y"), 0o600))
}

type jailFixture struct {
	s                            *Service
	root, game, other, secret    string
	store, ro, rw, scratch, data string
}

func newJailFixture(t *testing.T) *jailFixture {
	t.Helper()
	st := New(Config{}).Status()
	if !st.Available {
		t.Skip(st.Reason)
	}
	root := t.TempDir()
	f := &jailFixture{root: root, game: filepath.Join(root, "instances", "a", "game"), other: filepath.Join(root, "instances", "b", "game"),
		secret: filepath.Join(root, "accounts.json"), store: filepath.Join(root, "store", "libraries", "lib.jar"),
		scratch: filepath.Join(root, "instances", "a", "runs", "1.view"), data: filepath.Join(root, "instances", "a", "sandbox")}
	outside := t.TempDir()
	f.ro, f.rw = filepath.Join(outside, "maps"), filepath.Join(outside, "notes")
	for _, d := range []string{f.game, f.other, f.scratch, f.ro, f.rw, filepath.Dir(f.store)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{f.secret, f.store} {
		if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f.s = New(Config{DataRoot: root})
	return f
}

func (f *jailFixture) run(t *testing.T, set rules.Settings, mode string) (map[string]string, *procx.Child, *Jail) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	set.Folders = []rules.Folder{{ID: "a1", Path: f.ro, Level: rules.RO}, {ID: "b2", Path: f.rw, Level: rules.RW}}
	r := Run{Instance: "a", Rules: set, GameDir: f.game, Scratch: f.scratch, Data: f.data,
		Read: []string{filepath.Dir(exe), filepath.Join(f.root, "store", "libraries")}}
	env := append(FilterEnv(os.Environ()), "QUARTZ_SANDBOX_HELPER="+mode, "GAME="+f.game, "OTHER="+f.other, "SECRET="+f.secret,
		"PARENT="+fmt.Sprint(os.Getpid()), "RO="+f.ro, "RW="+f.rw, "STORE="+f.store)
	spec, j, err := f.s.Wrap(r, procx.Spec{Argv: []string{exe}, Dir: f.game, Env: env})
	if err != nil {
		t.Fatal(err)
	}
	c, err := procx.Start(spec)
	if err != nil {
		j.Close()
		t.Fatal(err)
	}
	j.Started(c.Process.Pid)
	if mode == "sleep" {
		return nil, c, j
	}
	defer j.Close()
	out, _ := io.ReadAll(c.Stdout)
	errOut, _ := io.ReadAll(c.Stderr)
	_ = c.Wait()
	got := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			got[k] = v
		}
	}
	if len(got) == 0 {
		t.Fatalf("the helper printed nothing: %s", errOut)
	}
	return got, c, j
}

func TestJailStandard(t *testing.T) {
	f := newJailFixture(t)
	got, _, _ := f.run(t, rules.Default(), "probe")
	want := map[string]string{
		"write-game": "ok", "unshare": "operation not permitted", "ptrace": "operation not permitted",
		"tiocsti": "operation not permitted", "write-rw": "ok", "read-store": "ok", "write-store": "read-only file system",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	for _, k := range []string{"see-other", "see-secret", "see-parent"} {
		if !strings.Contains(got[k], "no such file") {
			t.Errorf("%s = %q: the jail can see it", k, got[k])
		}
	}
	if !strings.Contains(got["write-ro"], "read-only") {
		t.Errorf("write-ro = %q", got["write-ro"])
	}
	if got["interfaces"] == "1" {
		t.Error("Standard lost the network")
	}
	// Only what the desktop rules bind (the PulseAudio cookie, MangoHud's
	// settings) shows in the home folder.
	for _, p := range strings.Split(got["home-files"], ",") {
		if p != "" && p != "/.config/pulse/cookie" && !strings.HasPrefix(p, "/.config/MangoHud/") {
			t.Errorf("home shows %s", p)
		}
	}
	if _, err := os.Stat(filepath.Join(f.game, "made.txt")); err != nil {
		t.Error("the game's write didn't reach its folder")
	}
}

func TestJailStrictHasNoNetwork(t *testing.T) {
	f := newJailFixture(t)
	got, _, _ := f.run(t, rules.Default().Apply(rules.Strict, "linux"), "probe")
	if got["interfaces"] != "1" { // loopback only
		t.Errorf("interfaces = %s", got["interfaces"])
	}
}

// Stop signals the launcher's process group, which reaches the jailed game.
func TestJailStopReachesGame(t *testing.T) {
	f := newJailFixture(t)
	_, c, j := f.run(t, rules.Default(), "sleep")
	defer j.Close()
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

func TestCheckFolder(t *testing.T) {
	root := t.TempDir()
	s := New(Config{DataRoot: filepath.Join(root, "Quartz")})
	_ = os.MkdirAll(filepath.Join(root, "Quartz", "instances"), 0o700)
	_ = os.MkdirAll(filepath.Join(root, "maps"), 0o700)
	_ = os.Symlink(filepath.Join(root, "Quartz"), filepath.Join(root, "sneaky"))
	_ = os.WriteFile(filepath.Join(root, "file"), nil, 0o600)
	home, _ := os.UserHomeDir()
	for _, c := range []struct {
		p  string
		ok bool
	}{
		{filepath.Join(root, "maps"), true},
		{root, false},                                       // holds the data folder
		{filepath.Join(root, "Quartz", "instances"), false}, // inside it
		{filepath.Join(root, "sneaky"), false},              // a link to it
		{filepath.Join(root, "file"), false},
		{filepath.Join(root, "missing"), false},
		{"/", false}, {"/etc", false}, {"relative", false},
		{filepath.Dir(home), false}, // holds the home folder
	} {
		if err := s.CheckFolder(c.p); (err == nil) != c.ok {
			t.Errorf("%s: %v", c.p, err)
		}
	}
}

func TestFilterEnv(t *testing.T) {
	got := FilterEnv([]string{"PATH=/usr/bin", "SSH_AUTH_SOCK=/run/agent", "DBUS_SESSION_BUS_ADDRESS=unix:x", "LC_ALL=C",
		"GITHUB_TOKEN=secret", "DISPLAY=:0", "__GL_SYNC_TO_VBLANK=0", "broken"})
	if strings.Join(got, " ") != "PATH=/usr/bin LC_ALL=C DISPLAY=:0 __GL_SYNC_TO_VBLANK=0" {
		t.Fatal(got)
	}
	if e := setEnv([]string{"A=1", "B=2"}, "A=3"); strings.Join(e, " ") != "B=2 A=3" {
		t.Fatal(e)
	}
}
