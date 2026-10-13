//go:build linux

package sandbox

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"

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
	// Only what the desktop rules bind (the PulseAudio cookie, MangoHud's
	// settings) shows in the home folder.
	for _, p := range strings.Split(got["home-files"], ",") {
		if p != "" && p != "/.config/pulse/cookie" && !strings.HasPrefix(p, "/.config/MangoHud/") {
			t.Errorf("home shows %s", p)
		}
	}
}

func TestJailStrictLoopbackOnly(t *testing.T) {
	f := newJailFixture(t)
	if got := f.probe(t, rules.Default().Apply(rules.Strict, "linux")); got["interfaces"] != "1" {
		t.Errorf("interfaces = %s", got["interfaces"])
	}
}
