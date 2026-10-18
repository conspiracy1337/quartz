// Package bwrap finds bubblewrap and builds its command lines. Run views
// (package runview) and the game sandbox (package sandbox) share one binary,
// one probe and one argument builder, so a run is always a single bwrap.
package bwrap

import "sync"

// Support is what bubblewrap can do on this system.
type Support struct {
	Bin     string // absolute path; "" when it can't make a mount namespace here
	Version string // "0.11.1"
	// Overlay: --overlay and --tmp-overlay work (bubblewrap 0.9 and a kernel
	// that lets unprivileged users mount overlays).
	Overlay bool
	// Jail: a fully unshared namespace with its own /proc and /dev works,
	// which is what the sandbox needs.
	Jail bool
	// DisableUserns: --disable-userns is understood (bubblewrap 0.8).
	DisableUserns bool
	// Setuid: the binary is setuid root (older distributions), so it must
	// not be started with no_new_privs set.
	Setuid bool
	// Reason says, for the player, why Bin or Jail is missing.
	Reason string
}

// Detect probes once and remembers the answer.
var Detect = sync.OnceValue(detect)

// Args builds a bwrap argument list; every path is its own argument.
type Args struct{ a []string }

// Add appends raw arguments.
func (b *Args) Add(s ...string) *Args { b.a = append(b.a, s...); return b }

// Bind mounts src read-write at dst.
func (b *Args) Bind(src, dst string) *Args { return b.Add("--bind", src, dst) }

// RoBind mounts src read-only at dst.
func (b *Args) RoBind(src, dst string) *Args { return b.Add("--ro-bind", src, dst) }

// DevBind mounts src at dst with device access.
func (b *Args) DevBind(src, dst string) *Args { return b.Add("--dev-bind", src, dst) }

// Tmpfs mounts an empty, writable, throwaway folder at dst.
func (b *Args) Tmpfs(dst string) *Args { return b.Add("--tmpfs", dst) }

// Dir makes an empty folder at dst.
func (b *Args) Dir(dst string) *Args { return b.Add("--dir", dst) }

// Symlink makes dst a link to target inside the namespace.
func (b *Args) Symlink(target, dst string) *Args { return b.Add("--symlink", target, dst) }

// Overlay mounts lower with upper and work folders on the host at dst.
func (b *Args) Overlay(lower, upper, work, dst string) *Args {
	return b.Add("--overlay-src", lower, "--overlay", upper, work, dst)
}

// TmpOverlay mounts lower at dst with writes kept in memory and dropped.
func (b *Args) TmpOverlay(lower, dst string) *Args {
	return b.Add("--overlay-src", lower, "--tmp-overlay", dst)
}

// List is the arguments so far.
func (b *Args) List() []string { return b.a }
