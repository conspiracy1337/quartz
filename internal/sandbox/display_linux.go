//go:build linux

package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"quartz/internal/core/fsx"
)

// dpyMode is how a run reaches the desktop display. The display is the sandbox's
// weakest seam: X11 has no inter-client isolation, so a trusted X client can
// screenshot, keylog and inject input across the whole session. We therefore
// prefer Wayland (which isolates clients), fall back to an untrusted X cookie
// (the X SECURITY extension confines the client), and only expose the session's
// X server untrusted as a last resort.
type dpyMode int

const (
	dpyNone       dpyMode = iota // headless, or no display at all
	dpyWayland                   // Wayland only; the X socket is not bound
	dpyXUntrusted                // X with a freshly minted untrusted cookie
	dpyXTrusted                  // X with the real cookie (no isolation available)
)

// displayMode decides how this run reaches the display, and mints the untrusted
// X cookie when it can. QUARTZ_X11=1 forces the X server through even on a
// Wayland session (an escape hatch for a game whose GLFW is X11-only).
func displayMode(r Run, env []string) (dpyMode, string) {
	if r.Headless {
		return dpyNone, ""
	}
	display := getEnv(env, "DISPLAY")
	wayland := getEnv(env, "WAYLAND_DISPLAY")
	forceX := os.Getenv("QUARTZ_X11") == "1"
	if display == "" {
		if wayland != "" {
			return dpyWayland, ""
		}
		return dpyNone, ""
	}
	if wayland != "" && !forceX {
		return dpyWayland, "" // Wayland isolates clients: drop the X server
	}
	if cookie, ok := mintUntrusted(display, getEnv(env, "XAUTHORITY"), r.Scratch); ok {
		return dpyXUntrusted, cookie
	}
	return dpyXTrusted, ""
}

// mintUntrusted asks the X server (over the real cookie) for a fresh untrusted
// authorization via the SECURITY extension and writes it alone to a new cookie
// file. It returns false when xauth is missing or the server has no SECURITY
// extension (XWayland), leaving the caller to fall back.
func mintUntrusted(display, xauthority, scratch string) (string, bool) {
	if scratch == "" {
		return "", false
	}
	bin, err := exec.LookPath("xauth")
	if err != nil {
		return "", false
	}
	out := filepath.Join(scratch, "xauth-untrusted")
	_ = os.Remove(out)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-f", out, "generate", display, ".", "untrusted", "timeout", "0")
	env := []string{"DISPLAY=" + display, "PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	if xauthority != "" {
		env = append(env, "XAUTHORITY="+xauthority)
	}
	cmd.Env = env
	if cmd.Run() != nil {
		_ = os.Remove(out)
		return "", false
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		return "", false
	}
	return out, true
}

// applyDisplayEnv points the child's environment at the chosen display: Wayland
// drops DISPLAY/XAUTHORITY so the toolkit selects the (bound) Wayland socket; an
// untrusted X run uses the minted cookie; trusted X and headless are unchanged.
func applyDisplayEnv(env []string, dm dpyMode, cookie string) []string {
	switch dm {
	case dpyWayland:
		return delEnv(env, "DISPLAY", "XAUTHORITY")
	case dpyXUntrusted:
		return setEnv(env, "XAUTHORITY="+cookie)
	default:
		return env
	}
}

// delEnv returns env without the named variables.
func delEnv(env []string, names ...string) []string {
	out := env[:0:0]
	for _, kv := range env {
		if n, _, _ := strings.Cut(kv, "="); !slices.Contains(names, n) {
			out = append(out, kv)
		}
	}
	return out
}

// machineID writes a fresh random machine-id for this run into dir (which is not
// bound into the jail) and returns its path. A per-run random id is bound over
// /etc/machine-id so a mod reads an unlinkable value, never the host's stable id.
func machineID(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("sandbox: no run folder")
	}
	id, err := randHex(16)
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, "machine-id")
	return p, fsx.AtomicWrite(p, []byte(id+"\n"), fsx.FilePerm)
}

func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
