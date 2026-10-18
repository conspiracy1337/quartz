//go:build linux

package bwrap

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func detect() Support {
	bin := "/usr/bin/bwrap"
	if _, err := os.Stat(bin); err != nil {
		p, err := exec.LookPath("bwrap")
		if err != nil || !filepath.IsAbs(p) {
			return Support{Reason: "bubblewrap isn't installed. Install the bubblewrap package to use the sandbox."}
		}
		bin = p
	}
	run := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, bin, args...).CombinedOutput() //nolint:gosec // fixed arguments to a fixed binary
	}
	s := Support{}
	if fi, err := os.Stat(bin); err == nil {
		s.Setuid = fi.Mode()&os.ModeSetuid != 0
	}
	if out, err := run("--version"); err == nil {
		s.Version = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "bubblewrap"))
	}
	if out, err := run("--dev-bind", "/", "/", "true"); err != nil {
		s.Reason = namespaceReason(out)
		return s
	}
	s.Bin = bin
	tmp := os.TempDir()
	_, err := run("--dev-bind", "/", "/", "--overlay-src", tmp, "--tmp-overlay", tmp, "true")
	s.Overlay = err == nil
	jail := []string{"--unshare-all", "--share-net", "--ro-bind", "/", "/", "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp"}
	_, err = run(append(jail, "--unshare-user", "--disable-userns", "true")...)
	s.DisableUserns = err == nil
	out, err := run(append(jail, "true")...)
	if s.Jail = err == nil; !s.Jail {
		s.Reason = namespaceReason(out)
	}
	return s
}

// namespaceReason explains a failed namespace probe in the player's terms.
func namespaceReason(out []byte) string {
	read := func(p string) string {
		b, _ := os.ReadFile(p) //nolint:gosec // fixed kernel settings files
		return strings.TrimSpace(string(b))
	}
	switch {
	case read("/proc/sys/kernel/apparmor_restrict_unprivileged_userns") == "1":
		return "AppArmor stops bubblewrap from creating user namespaces on this system."
	case read("/proc/sys/kernel/unprivileged_userns_clone") == "0", read("/proc/sys/user/max_user_namespaces") == "0":
		return "User namespaces are turned off on this system, so bubblewrap can't create a sandbox."
	}
	msg := string(bytes.TrimSpace(out))
	if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
		msg = msg[i+1:]
	}
	if msg == "" {
		return "bubblewrap can't create a sandbox on this system."
	}
	return "bubblewrap can't create a sandbox on this system: " + strings.TrimPrefix(msg, "bwrap: ")
}
