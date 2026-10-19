//go:build linux

package sandbox

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"quartz/internal/core/bwrap"
	"quartz/internal/core/landlock"
	"quartz/internal/core/procx"
	"quartz/internal/sandbox/rules"
)

func platformProbes() {
	var names []string
	_ = filepath.WalkDir(os.Getenv("HOME"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			names = append(names, strings.TrimPrefix(p, os.Getenv("HOME")))
		}
		return nil
	})
	say("home-files", fmt.Errorf("%s", strings.Join(names, ",")))
	_, err := os.Stat(filepath.Join("/proc", os.Getenv("PARENT")))
	say("see-parent", err)
	say("unshare", unix.Unshare(unix.CLONE_NEWUSER))
	say("ptrace", unix.PtraceAttach(1))
	c := byte('x')
	_, _, e := unix.Syscall(unix.SYS_IOCTL, 0, unix.TIOCSTI, uintptr(unsafe.Pointer(&c)))
	say("tiocsti", e)
	ifs, _ := net.Interfaces()
	say("interfaces", fmt.Errorf("%d", len(ifs)))
	say("abstract", func() error {
		c, err := net.DialTimeout("unix", os.Getenv("ABSTRACT"), 2*time.Second)
		if err == nil {
			_ = c.Close()
		}
		return err
	}())
	h, _ := os.Hostname()
	say("hostname", fmt.Errorf("%s", h))
	macs, _ := filepath.Glob("/sys/class/net/*/address")
	say("macs", fmt.Errorf("%d", len(macs)))
}

func checkPlatform(t *testing.T, got map[string]string) {
	t.Helper()
	for _, k := range []string{"unshare", "ptrace", "tiocsti"} {
		if got[k] != "operation not permitted" {
			t.Errorf("%s = %q, want operation not permitted", k, got[k])
		}
	}
	if !strings.Contains(got["see-parent"], "no such file") {
		t.Errorf("see-parent = %q: the jail sees the launcher's processes", got["see-parent"])
	}
	if !strings.Contains(got["write-store"], "read-only") {
		t.Errorf("write-store = %q", got["write-store"])
	}
	// Host fingerprints: the jail names itself and shows no network card.
	if got["hostname"] != hostname {
		t.Errorf("hostname = %q, want %q", got["hostname"], hostname)
	}
	if got["macs"] != "0" {
		t.Errorf("%s network card addresses readable", got["macs"])
	}
	// The host's abstract sockets (the X server's among them) share the
	// network namespace; Landlock's scope keeps the game off them.
	if landlock.Supported() && !bwrap.Detect().Setuid && got["abstract"] != "operation not permitted" {
		t.Errorf("abstract = %q: a host abstract socket is reachable", got["abstract"])
	}
	// Only what the desktop rules bind (the PulseAudio cookie, MangoHud's
	// settings) shows in the home folder.
	for _, p := range strings.Split(got["home-files"], ",") {
		if p != "" && p != "/.config/pulse/cookie" && !strings.HasPrefix(p, "/.config/MangoHud/") {
			t.Errorf("home shows %s", p)
		}
	}
}

// The Blocked internet level unshares the net namespace, leaving only
// loopback. (Strict keeps the host network now — a no-network server is
// useless and outbound abuse isn't the threat.)
func TestJailBlockedLoopbackOnly(t *testing.T) {
	f := newJailFixture(t)
	set, _ := rules.Default().SetLevel(rules.Internet, rules.Blocked, "linux")
	if got := f.probe(t, set); got["interfaces"] != "1" {
		t.Errorf("interfaces = %s", got["interfaces"])
	}
}

// A process the game leaves running (a mod's daemon) ends with the game,
// so it can't keep the run, or anything else, alive.
func TestJailLeftoversEndWithGame(t *testing.T) {
	f := newJailFixture(t)
	spec, j := f.runArgs(t, rules.Default(), "probe")
	defer j.Close()
	spec.Argv = append(spec.Argv[:len(spec.Argv)-1], "/bin/sh", "-c", "setsid sleep 30 & echo started")
	c, err := procx.Start(spec)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	go func() { _, _ = io.Copy(io.Discard, c.Stderr) }()
	if out, _ := io.ReadAll(c.Stdout); !strings.Contains(string(out), "started") {
		t.Fatalf("output %q", out)
	}
	_ = c.Wait()
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("the run ended only after %v", d)
	}
}

func TestNamesHost(t *testing.T) {
	for line, want := range map[string]bool{
		"127.0.1.1\talice-laptop":                    true,
		"127.0.1.1 alice-laptop.lan alias":           true,
		"127.0.0.1 localhost":                        false,
		"192.168.1.5 mc.lan # alice-laptop's server": false,
		"10.0.0.2 alice-laptopish":                   false,
		"# 127.0.1.1 alice-laptop":                   false,
		"alice-laptop":                               false, // an address alone names nothing
		"":                                           false,
	} {
		if got := namesHost(line, "alice-laptop"); got != want {
			t.Errorf("namesHost(%q) = %v, want %v", line, got, want)
		}
	}
}
