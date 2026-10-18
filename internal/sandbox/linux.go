//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"quartz/internal/core/bwrap"
	"quartz/internal/core/fsx"
	"quartz/internal/core/procx"
	discordipc "quartz/internal/discord"
	"quartz/internal/sandbox/rules"
	"quartz/internal/sandbox/seccomp"
)

type linuxBackend struct {
	once sync.Once
	st   Status
}

func newBackend() backend { return &linuxBackend{} }

func (b *linuxBackend) status() Status {
	b.once.Do(func() {
		sup := bwrap.Detect()
		b.st = Status{Name: "bubblewrap", Version: sup.Version, Available: sup.Bin != "" && sup.Jail, Reason: sup.Reason}
		if _, err := seccomp.Program(); err != nil && b.st.Available {
			b.st.Available, b.st.Reason = false, "There's no system-call filter for this processor yet."
		}
	})
	return b.st
}

// systemDirs are bound read-only, or recreated as the links they are
// (/bin → usr/bin on merged-/usr systems). NixOS and Guix keep the system,
// its Java and its GPU drivers in their stores and link to them from /run.
var systemDirs = []string{"/usr", "/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32",
	"/nix/store", "/gnu/store", "/run/current-system", "/run/opengl-driver", "/run/opengl-driver-32"}

// etcFiles are the parts of /etc a game needs: name lookup, TLS roots,
// fonts, time, the dynamic linker, drivers and system Java configuration.
var etcFiles = []string{
	"resolv.conf", "hosts", "host.conf", "nsswitch.conf", "gai.conf", "ld.so.cache", "ld.so.conf", "ld.so.conf.d", "localtime",
	"timezone", "locale.conf", "fonts", "ssl", "pki", "ca-certificates", "crypto-policies", "alternatives",
	"os-release", "lsb-release", "drirc", "vulkan", "glvnd", "OpenCL", "nvidia", "asound.conf", "alsa", "pulse", "pipewire",
}

// sysDirs are the sysfs parts graphics drivers read (Flatpak's set).
var sysDirs = []string{"/sys/block", "/sys/bus", "/sys/class", "/sys/dev", "/sys/devices", "/sys/module"}

func (b *linuxBackend) wrap(s *Service, r Run, spec procx.Spec) (procx.Spec, *Jail, error) {
	sup := bwrap.Detect()
	var a bwrap.Args
	// --as-pid-1: the game is the sandbox's first process, so whatever it
	// leaves running (a mod's daemon, a bus it autolaunched) ends with it
	// instead of keeping the run alive. The JVM handles SIGTERM, so Stop
	// still lets it save and quit.
	a.Add(sup.Bin, "--unshare-user", "--unshare-pid", "--as-pid-1", "--unshare-uts", "--unshare-cgroup-try")
	if sup.DisableUserns {
		a.Add("--disable-userns")
	}
	dm, cookie := displayMode(r, spec.Env)
	if dm != dpyXTrusted {
		// Only a trusted X client needs the shared IPC namespace (MIT-SHM for
		// the X server); Wayland, an untrusted X cookie and headless do not.
		a.Add("--unshare-ipc")
	}
	switch dm {
	case dpyWayland:
		s.c.Log.Info("sandbox: Wayland display; X server withheld (set QUARTZ_X11=1 if the game can't open a window)")
	case dpyXTrusted:
		s.c.Log.Warn("sandbox: X display isolation unavailable (no Wayland and no SECURITY extension); the game can see other X clients", "display", getEnv(spec.Env, "DISPLAY"))
	}
	if !r.Headless && r.Level(rules.Internet) == rules.Blocked {
		a.Add("--unshare-net")
	}
	a.Add("--proc", "/proc", "--dev", "/dev").Tmpfs("/dev/shm").Tmpfs("/tmp")

	for _, d := range systemDirs {
		bindSystem(&a, d)
	}
	for _, f := range etcFiles {
		bindResolved(&a, filepath.Join("/etc", f))
	}
	if m, _ := filepath.Glob("/etc/java*"); len(m) > 0 {
		for _, p := range m {
			bindResolved(&a, p)
		}
	}
	passwd, group, err := identity(r.Scratch, s.home)
	if err != nil {
		return procx.Spec{}, nil, err
	}
	a.RoBind(passwd, "/etc/passwd").RoBind(group, "/etc/group")
	// A fresh random machine-id, so a mod can't read the host's stable one.
	if mid, err := machineID(r.Scratch); err == nil {
		a.RoBind(mid, "/etc/machine-id").RoBind(mid, "/var/lib/dbus/machine-id")
	}

	// Home: an empty throwaway folder, or the real one read-only. The
	// player's own folders come next, then the launcher's data is hidden
	// again before its allowed parts are put back.
	if s.home != "" {
		if r.Level(rules.Home) == rules.RO {
			a.RoBind(s.home, s.home)
		} else {
			a.Tmpfs(s.home)
		}
	}
	for _, f := range r.Rules.Folders {
		if f.Level == rules.RW {
			a.Bind(f.Path, f.Path)
		} else {
			a.RoBind(f.Path, f.Path)
		}
	}
	if s.c.DataRoot != "" {
		a.Tmpfs(s.c.DataRoot)
	}
	bound := slices.Clone(systemDirs)
	for _, p := range r.Read {
		bindRead(&a, p, &bound)
	}
	// Libraries the player's environment points the game at (outside the
	// home folder and the launcher's data, which stay hidden).
	for _, p := range filepath.SplitList(getEnv(spec.Env, "LD_LIBRARY_PATH")) {
		if filepath.IsAbs(p) && (s.home == "" || !fsx.Within(p, s.home)) && (s.c.DataRoot == "" || !fsx.Within(p, s.c.DataRoot)) {
			bindRead(&a, p, &bound)
		}
	}
	for _, d := range r.Redirects {
		a.Bind(d.From, d.To)
	}
	if r.Data != "" {
		a.Bind(r.Data, r.Data)
	}
	for _, p := range r.Write {
		a.Bind(p, p)
	}
	a.Bind(r.GameDir, r.GameDir)
	a.Add(r.Mounts...)
	// Protected folders: writes land in a throwaway layer (a mod's own cache
	// still works), or fail where overlays don't.
	for _, p := range r.Protected() {
		if sup.Overlay {
			a.TmpOverlay(p, p)
		} else {
			a.RoBind(p, p)
		}
	}
	for _, f := range r.ProtectedFiles() {
		a.RoBind(f.Real, f.Seen)
	}

	env := applyDisplayEnv(spec.Env, dm, cookie)
	if !r.Headless {
		env = desktop(&a, r, s.home, env)
	}
	if r.Data != "" {
		env = setEnv(env, "XDG_CACHE_HOME="+filepath.Join(r.Data, "cache"), "XDG_CONFIG_HOME="+filepath.Join(r.Data, "config"),
			"XDG_DATA_HOME="+filepath.Join(r.Data, "data"), "__GL_SHADER_DISK_CACHE_PATH="+filepath.Join(r.Data, "cache"))
	}
	if s.home != "" {
		env = setEnv(env, "HOME="+s.home)
	}
	// There is no session bus inside; naming one that doesn't exist stops
	// libdbus from autolaunching its own (dbus-launch) in the sandbox.
	env = setEnv(env, "DBUS_SESSION_BUS_ADDRESS=unix:path=/nonexistent/quartz-sandbox-bus")

	filter, err := seccomp.File()
	if err != nil {
		return procx.Spec{}, nil, err
	}
	j := &Jail{Name: "bubblewrap"}
	j.closers = append(j.closers, func() { _ = filter.Close() })
	files := append(slices.Clone(spec.ExtraFiles), filter)
	a.Add("--seccomp", strconv.Itoa(2+len(files)))
	if r.GameMode {
		info, err := gameMode(j)
		if err != nil {
			j.Close()
			return procx.Spec{}, nil, err
		}
		files = append(files, info)
		a.Add("--info-fd", strconv.Itoa(2+len(files)))
	}
	a.Add("--chdir", r.GameDir, "--")
	spec.Argv = append(a.List(), spec.Argv...)
	spec.Env, spec.ExtraFiles = env, files
	return spec, j, nil
}

// bindSystem binds a top-level system folder, or recreates it as a link.
func bindSystem(a *bwrap.Args, p string) {
	fi, err := os.Lstat(p)
	switch {
	case err != nil:
	case fi.Mode()&os.ModeSymlink != 0:
		if t, err := os.Readlink(p); err == nil {
			a.Symlink(t, p)
		}
	case fi.IsDir():
		a.RoBind(p, p)
	}
}

// bindResolved binds what p points to at p, read-only (/etc/resolv.conf is
// often a link into /run, which the sandbox doesn't have).
func bindResolved(a *bwrap.Args, p string) {
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return
	}
	a.RoBind(real, p)
}

// bindRead binds a read-only path the game needs (the store's folders, the
// Java runtime, a wrapper program) unless a bound folder already holds it.
// A Java root's linked parts (Arch keeps lib/ elsewhere) come along.
func bindRead(a *bwrap.Args, p string, bound *[]string) {
	covered := func(q string) bool {
		return slices.ContainsFunc(*bound, func(b string) bool { return fsx.Within(q, b) })
	}
	add := func(q string) {
		if !covered(q) {
			a.RoBind(q, q)
			*bound = append(*bound, q)
		}
	}
	if _, err := os.Stat(p); err != nil || fsx.IsSystemRoot(filepath.Clean(p)) {
		return // the system is bound already, never as a whole
	}
	add(p)
	if real, err := filepath.EvalSymlinks(p); err == nil && real != p {
		add(real)
	}
	entries, err := os.ReadDir(p)
	if err != nil || len(entries) > 64 {
		return
	}
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			if real, err := filepath.EvalSymlinks(filepath.Join(p, e.Name())); err == nil {
				add(real)
			}
		}
	}
}

// identity writes passwd and group files naming only the player, so Java
// finds its user and home folder without seeing the system's accounts.
func identity(dir, home string) (string, string, error) {
	if dir == "" {
		return "", "", fmt.Errorf("sandbox: no run folder")
	}
	uid, gid := os.Getuid(), os.Getgid()
	name, gname := "player", "player"
	if u, err := user.Current(); err == nil && validName(u.Username) {
		name = u.Username
	}
	if g, err := user.LookupGroupId(strconv.Itoa(gid)); err == nil && validName(g.Name) {
		gname = g.Name
	}
	if home == "" {
		home = "/"
	}
	passwd, group := filepath.Join(dir, "passwd"), filepath.Join(dir, "group")
	if err := fsx.AtomicWrite(passwd, fmt.Appendf(nil, "%s:x:%d:%d::%s:/bin/sh\n", name, uid, gid, home), fsx.FilePerm); err != nil {
		return "", "", err
	}
	return passwd, group, fsx.AtomicWrite(group, fmt.Appendf(nil, "%s:x:%d:\n", gname, gid), fsx.FilePerm)
}

func validName(s string) bool {
	return s != "" && len(s) <= 64 && !strings.ContainsAny(s, ":\n\r\x00/")
}

// desktop binds what a game window needs: graphics devices, the display,
// audio and (when allowed) Discord.
func desktop(a *bwrap.Args, r Run, home string, env []string) []string {
	for _, d := range []string{"/dev/dri", "/dev/snd"} {
		if _, err := os.Stat(d); err == nil {
			a.DevBind(d, d)
		}
	}
	if m, _ := filepath.Glob("/dev/nvidia*"); len(m) > 0 {
		for _, d := range m {
			a.DevBind(d, d)
		}
	}
	for _, d := range sysDirs {
		if _, err := os.Stat(d); err == nil {
			a.RoBind(d, d)
		}
	}
	// Hide the board/BIOS fingerprint (DMI) that rides in with sysfs; the GPU
	// PCI nodes the drivers read stay bound.
	if exists("/sys/devices/virtual/dmi") {
		a.Tmpfs("/sys/devices/virtual/dmi")
	}
	if d := getEnv(env, "DISPLAY"); d != "" {
		if n, ok := displayNumber(d); ok {
			sock := "/tmp/.X11-unix/X" + n
			if _, err := os.Stat(sock); err == nil {
				a.RoBind(sock, sock)
			}
		}
		if x := getEnv(env, "XAUTHORITY"); x != "" && filepath.IsAbs(x) {
			if _, err := os.Stat(x); err == nil {
				a.RoBind(x, x)
			}
		} else if home != "" {
			if p := filepath.Join(home, ".Xauthority"); exists(p) {
				a.RoBind(p, p)
			}
		}
	}
	run := getEnv(env, "XDG_RUNTIME_DIR")
	if run != "" && filepath.IsAbs(run) {
		a.Add("--perms", "0700").Dir(run)
		if w := getEnv(env, "WAYLAND_DISPLAY"); w != "" {
			sock := w
			if !filepath.IsAbs(sock) {
				sock = filepath.Join(run, w)
			}
			if exists(sock) {
				a.RoBind(sock, sock)
			}
		}
		for _, s := range []string{"pipewire-0", "pulse/native"} {
			if p := filepath.Join(run, s); exists(p) {
				a.RoBind(p, p)
			}
		}
		if r.Level(rules.Discord) == rules.Allowed {
			discord(a, run)
		}
	}
	if ps := getEnv(env, "PULSE_SERVER"); strings.HasPrefix(ps, "unix:") {
		if p := strings.TrimPrefix(ps, "unix:"); filepath.IsAbs(p) && exists(p) {
			a.RoBind(p, p)
		}
	}
	if home != "" {
		for _, rel := range []string{".config/pulse/cookie", ".config/MangoHud"} {
			if p := filepath.Join(home, rel); exists(p) {
				a.RoBind(p, p)
			}
		}
	}
	return env
}

// discord binds the Discord client's IPC sockets (native, Flatpak, Snap and
// Vesktop installs) where Rich Presence libraries look: $XDG_RUNTIME_DIR.
func discord(a *bwrap.Args, run string) {
	for n, src := range discordipc.Sockets(run, "/tmp") {
		if n == 10 {
			break
		}
		a.RoBind(src, filepath.Join(run, "discord-ipc-"+strconv.Itoa(n)))
	}
}

// displayNumber reads N from ":N", ":N.S" or "unix:N".
func displayNumber(d string) (string, bool) {
	_, after, ok := strings.Cut(d, ":")
	if !ok {
		return "", false
	}
	n, _, _ := strings.Cut(after, ".")
	if _, err := strconv.Atoi(n); err != nil {
		return "", false
	}
	return n, true
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil } //nolint:gosec // only stats system and session paths

// getEnv reads a variable from env.
func getEnv(env []string, name string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if n, v, _ := strings.Cut(env[i], "="); n == name {
			return v
		}
	}
	return ""
}

// forget is a no-op: nothing outlives a run here.
func forget(string) error { return nil }

// Desktop names what the Graphics and audio rule lets through here.
func Desktop() string {
	var parts []string
	if exists("/dev/dri") || exists("/dev/nvidiactl") {
		parts = append(parts, "GPU")
	}
	run := os.Getenv("XDG_RUNTIME_DIR")
	switch {
	case run != "" && exists(filepath.Join(run, "pipewire-0")):
		parts = append(parts, "PipeWire")
	case run != "" && exists(filepath.Join(run, "pulse", "native")):
		parts = append(parts, "PulseAudio")
	}
	switch {
	case os.Getenv("WAYLAND_DISPLAY") != "":
		parts = append(parts, "Wayland")
	case os.Getenv("DISPLAY") != "":
		parts = append(parts, "X11")
	}
	return strings.Join(parts, ", ")
}
