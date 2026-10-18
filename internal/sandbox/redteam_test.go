//go:build linux

package sandbox

// Red-team harness: run a probe jar (standing in for a malicious mod) inside a
// real bubblewrap jail and record what it can reach, for the Standard and
// Strict presets. This is diagnostic tooling, not a unit test, so it is gated
// behind QUARTZ_REDTEAM=1 (like helper_test.go's QUARTZ_SANDBOX_HELPER) and
// skipped in normal `go test` and CI. It needs a desktop session (DISPLAY) and
// a JDK >= 16 (UnixDomainSocketAddress); point QUARTZ_JDK_BIN at a bin/ holding
// java+javac+jar, or it searches the usual asdf/.jdks/jvm locations.
//
// Invariants it FAILS on (these must always hold): the filesystem wall, no-new-
// privs, the seccomp denylist, PID-namespace isolation, and that Strict removes
// the network. Everything else - the X11 takeover, the shared-netns reach under
// Standard - is reported as a finding, since hardening is a separate round.

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"quartz/internal/core/bwrap"
	"quartz/internal/core/landlock"
	"quartz/internal/core/procx"
	"quartz/internal/sandbox/rules"
)

type redteamEnv struct {
	root, home, game, data, scratch string
	secret, homeFile                string
	jarDir, jarPath                 string
	javaBin                         string
	listen, abstract                string
	tools                           map[string]string // CANARY_* -> absolute path
}

func TestRedteamEscapes(t *testing.T) {
	if os.Getenv("QUARTZ_REDTEAM") == "" {
		t.Skip("set QUARTZ_REDTEAM=1 to run the sandbox red-team suite (needs a desktop session and a JDK)")
	}
	if st := New(Config{}).Status(); !st.Available {
		t.Skip("sandbox unavailable: " + st.Reason)
	}
	if os.Getenv("DISPLAY") == "" {
		t.Skip("no DISPLAY: the X11 vectors need a running X/XWayland server")
	}
	javaBin, javacBin, jarBin := findJDK(t)
	// A planted secret in the launcher's own environment: FilterEnv must drop it,
	// so the game never sees it (positive control for the env-leak vector).
	t.Setenv("AWS_SECRET_ACCESS_KEY", "CANARY-LEAKED-SECRET-must-not-reach-the-game")
	e := setupRedteam(t, javaBin)
	compileProbe(t, javacBin, jarBin, e)

	results := map[string]map[string]string{}
	for _, p := range []struct {
		name string
		set  rules.Settings
	}{
		{"Standard", rules.Default()},
		{"Strict", rules.Default().Apply(rules.Strict, runtime.GOOS)},
	} {
		results[p.name] = runProbe(t, e, p.name, p.set)
	}

	reportFindings(t, e, results)
	checkInvariants(t, results)
}

// --- invariants: these must hold under every preset. ---

func checkInvariants(t *testing.T, results map[string]map[string]string) {
	landlockScoped := landlock.Supported() && !bwrap.Detect().Setuid
	for preset, got := range results {
		must := func(vector, want string) {
			if v := got[vector]; v != "" && result(v) != want {
				t.Errorf("[%s] INVARIANT BROKEN %s: got %q, want %s", preset, vector, result(v), want)
			}
		}
		// mustNot fails only on a specific escape value, so SKIP (FFI absent) and
		// BLOCKED/OPEN both pass - the right shape for the FFI-gated vectors.
		mustNot := func(vector, bad string) {
			if v := got[vector]; v != "" && result(v) == bad {
				t.Errorf("[%s] INVARIANT BROKEN %s: got %s :: %s", preset, vector, bad, detail(v))
			}
		}
		// The abstract-socket scope (round 2's headline fix): the host X server
		// and the canary abstract socket must be unreachable where Landlock runs.
		if landlockScoped {
			mustNot("abstract-x11", "REACHABLE")
			mustNot("abstract-canary", "REACHABLE")
		}
		// seccomp must hold regardless of preset or Landlock.
		mustNot("sock-family-alg", "REACHABLE")
		mustNot("sock-family-packet", "REACHABLE")
		mustNot("seccomp-raw", "REACHABLE")
		// Round-2 fingerprint scrubbing and the env filter.
		mustNot("hw-ids", "LEAK")
		mustNot("env-leak", "LEAK")
		must("fs-launcher-secret", "BLOCKED")
		must("fs-home-secret", "BLOCKED")
		must("no-new-privs", "INFO") // NoNewPrivs=1
		must("proc-pids", "INFO")    // PID namespace isolates host
		for _, v := range []string{"seccomp-unshare", "seccomp-mount", "seccomp-ptrace"} {
			if r := result(got[v]); r != "" && r != "BLOCKED" && r != "SKIP" {
				t.Errorf("[%s] INVARIANT BROKEN %s: got %q, want BLOCKED", preset, v, r)
			}
		}
		// Fingerprint masking: DMI hidden, machine-id synthesized not the host's.
		if r := result(got["leak-dmi"]); r != "" && r != "BLOCKED" {
			t.Errorf("[%s] INVARIANT BROKEN leak-dmi: got %q, want BLOCKED (DMI must be masked)", preset, r)
		}
		if d := detail(got["leak-machine-id"]); d != "" {
			reported := d[strings.LastIndex(d, " ")+1:]
			if host := hostMachineID(); host != "" && reported == host {
				t.Errorf("[%s] INVARIANT BROKEN leak-machine-id: probe read the host's real machine-id %q", preset, host)
			}
		}
		// Display isolation: on a Wayland session (no escape hatch) the X server
		// must not be exposed, so the takeover vectors cannot connect. SKIP (the
		// game got no DISPLAY) and BLOCKED (socket unbound) are both contained;
		// only REACHABLE means the takeover is back.
		if os.Getenv("WAYLAND_DISPLAY") != "" && os.Getenv("QUARTZ_X11") != "1" {
			for _, v := range []string{"x11-handshake", "x11-capture", "x11-inject"} {
				if result(got[v]) == "REACHABLE" {
					t.Errorf("[%s] INVARIANT BROKEN %s: got REACHABLE (Wayland-first must not expose the X server)", preset, v)
				}
			}
		}
	}
	// Strict must remove the host network, so loopback, netlink and the host
	// connection table are all gone once the net namespace is unshared.
	if s := results["Strict"]; s != nil {
		for _, v := range []string{"net-loopback", "net-netlink", "proc-net"} {
			if r := result(s[v]); r != "" && r != "BLOCKED" {
				t.Errorf("[Strict] INVARIANT BROKEN %s: got %q, want BLOCKED (Strict unshares the net namespace)", v, r)
			}
		}
	}
}

// --- findings: print the full matrix and the confirmed escapes. ---

func reportFindings(t *testing.T, e *redteamEnv, results map[string]map[string]string) {
	keys := map[string]bool{}
	for _, got := range results {
		for k := range got {
			keys[k] = true
		}
	}
	var vectors []string
	for k := range keys {
		vectors = append(vectors, k)
	}
	sort.Strings(vectors)

	var b strings.Builder
	fmt.Fprintf(&b, "\n=== SANDBOX RED-TEAM MATRIX (runtime proof) ===\n%-22s  %-10s  %-10s\n", "vector", "Standard", "Strict")
	for _, v := range vectors {
		fmt.Fprintf(&b, "%-22s  %-10s  %-10s\n", v, result(results["Standard"][v]), result(results["Strict"][v]))
	}
	fmt.Fprintf(&b, "\n--- escapes / leaks confirmed under Standard (the default preset) ---\n")
	for _, v := range vectors {
		switch result(results["Standard"][v]) {
		case "REACHABLE", "LEAK", "OPEN", "PRESENT":
			fmt.Fprintf(&b, "  [%s] %s :: %s\n", result(results["Standard"][v]), v, detail(results["Standard"][v]))
		}
	}
	if png := filepath.Join(e.game, "redteam-canary-capture.png"); fileExists(png) {
		fi, _ := os.Stat(png)
		fmt.Fprintf(&b, "\nscreen-capture artifact written inside the jail: %s (%d bytes)\n", png, fi.Size())
	}
	t.Log(b.String())
}

// --- running one probe inside the jail ---

func runProbe(t *testing.T, e *redteamEnv, preset string, set rules.Settings) map[string]string {
	t.Helper()
	r := Run{
		Instance: "redteam-" + preset,
		Rules:    set,
		GameDir:  e.game,
		Scratch:  e.scratch,
		Data:     e.data,
		Read:     []string{JavaRoot(e.javaBin), e.jarDir},
	}
	env := append(FilterEnv(os.Environ()),
		"CANARY_SECRET="+e.secret,
		"CANARY_HOME="+e.homeFile,
		"CANARY_GAME="+e.game,
		"CANARY_LISTEN="+e.listen,
		"CANARY_ABSTRACT="+e.abstract,
	)
	for k, v := range e.tools {
		env = append(env, k+"="+v)
	}
	// --enable-native-access silences the FFI warning (JDK 17+); the probe uses
	// the Foreign Function API to issue the raw syscalls a real mod would.
	argv := []string{e.javaBin, "--enable-native-access=ALL-UNNAMED", "-jar", e.jarPath}
	spec, j, err := New(Config{DataRoot: e.root, Shim: landlock.Self}).Wrap(r, procx.Spec{Argv: argv, Dir: e.game, Env: env})
	if err != nil {
		t.Fatalf("[%s] Wrap: %v", preset, err)
	}
	defer j.Close()
	c, err := procx.Start(spec)
	if err != nil {
		t.Fatalf("[%s] Start: %v", preset, err)
	}
	j.Started(c.Process.Pid)
	_ = c.Stdin.Close()

	outc := make(chan []byte, 1)
	go func() { b, _ := io.ReadAll(c.Stdout); outc <- b }()
	errc := make(chan []byte, 1)
	go func() { b, _ := io.ReadAll(c.Stderr); errc <- b }()

	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		_ = procx.Kill(c.Process)
		t.Fatalf("[%s] probe timed out", preset)
	}
	out, errOut := <-outc, <-errc

	got := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "="); ok {
			got[k] = v
		}
	}
	if len(got) == 0 {
		t.Fatalf("[%s] probe printed nothing; stderr:\n%s", preset, errOut)
	}
	return got
}

// --- fixture & tooling ---

func setupRedteam(t *testing.T, javaBin string) *redteamEnv {
	t.Helper()
	root := t.TempDir()
	e := &redteamEnv{
		root:    root,
		home:    filepath.Join(root, "home"),
		game:    filepath.Join(root, "instances", "a", "game"),
		data:    filepath.Join(root, "instances", "a", "sandbox"),
		scratch: filepath.Join(root, "instances", "a", "runs", "1.view"),
		secret:  filepath.Join(root, "accounts.json"),
		jarDir:  t.TempDir(),
		javaBin: javaBin,
		tools:   map[string]string{},
	}
	e.homeFile = filepath.Join(e.home, ".ssh", "id_ed25519")
	for _, d := range []string{e.game, e.scratch, filepath.Dir(e.homeFile)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(e.secret, []byte(`{"token":"CANARY-LAUNCHER-SECRET"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.homeFile, []byte("CANARY-PRIVATE-KEY"), 0o600); err != nil {
		t.Fatal(err)
	}

	// A host-local service the game should only reach when it shares the net namespace.
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
	e.listen = ln.Addr().String()

	// A host abstract unix socket: a known-present target for the abstract-socket
	// scope. Abstract names live in the net namespace, so the mount wall can't
	// hide it; the Landlock scope must. "@" is Go's spelling of a leading NUL.
	e.abstract = fmt.Sprintf("@quartz-redteam-%d", os.Getpid())
	al, err := net.Listen("unix", e.abstract)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = al.Close() })
	go func() {
		for {
			c, err := al.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	// Host binaries the probe execs to show seccomp denials and netlink reach;
	// absolute paths, valid inside the jail because /usr is bound read-only.
	for env, name := range map[string]string{"CANARY_IP": "ip", "CANARY_UNSHARE": "unshare", "CANARY_MOUNT": "mount", "CANARY_STRACE": "strace"} {
		if p, err := exec.LookPath(name); err == nil {
			e.tools[env] = p
		}
	}
	return e
}

func compileProbe(t *testing.T, javacBin, jarBin string, e *redteamEnv) {
	t.Helper()
	src, err := filepath.Abs("testdata/redteam/Probe.java")
	if err != nil || !fileExists(src) {
		t.Fatalf("probe source not found: %v", err)
	}
	classes := t.TempDir()
	if out, err := exec.Command(javacBin, "-d", classes, src).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	e.jarPath = filepath.Join(e.jarDir, "redteam-probe.jar")
	if out, err := exec.Command(jarBin, "--create", "--file", e.jarPath, "--main-class", "Probe", "-C", classes, ".").CombinedOutput(); err != nil {
		t.Fatalf("jar: %v\n%s", err, out)
	}
}

// findJDK returns java, javac and jar from one JDK >= 16. QUARTZ_JDK_BIN wins
// outright; otherwise it prefers the highest feature version present, so the FFI
// vectors (stable in JDK 22) run by default instead of being skipped.
func findJDK(t *testing.T) (javaBin, javacBin, jarBin string) {
	t.Helper()
	if b := os.Getenv("QUARTZ_JDK_BIN"); b != "" {
		if jv, jc, jr, ok := jdkTriple(b); ok {
			return jv, jc, jr
		}
		t.Skipf("QUARTZ_JDK_BIN=%s has no java+javac+jar", b)
	}
	var bins []string
	home, _ := os.UserHomeDir()
	for _, pat := range []string{
		filepath.Join(home, ".asdf/installs/java/*/bin"),
		filepath.Join(home, ".jdks/*/bin"),
		"/usr/lib/jvm/*/bin",
	} {
		m, _ := filepath.Glob(pat)
		bins = append(bins, m...)
	}
	if jc, err := exec.LookPath("javac"); err == nil {
		bins = append(bins, filepath.Dir(jc))
	}
	best := -1
	for _, b := range bins {
		jv, jc, jr, ok := jdkTriple(b)
		if !ok {
			continue
		}
		if m := javaMajor(jv); m > best {
			best, javaBin, javacBin, jarBin = m, jv, jc, jr
		}
	}
	if best < 0 {
		t.Skip("no JDK (java+javac+jar) found; set QUARTZ_JDK_BIN")
	}
	return javaBin, javacBin, jarBin
}

func jdkTriple(bin string) (jv, jc, jr string, ok bool) {
	jv, jc, jr = filepath.Join(bin, "java"), filepath.Join(bin, "javac"), filepath.Join(bin, "jar")
	return jv, jc, jr, fileExists(jv) && fileExists(jc) && fileExists(jr)
}

// javaMajor is the feature version from `java -version` (e.g. "26.0.2" -> 26),
// or 0 if it can't be read.
func javaMajor(javaBin string) int {
	out, err := exec.Command(javaBin, "-version").CombinedOutput()
	if err != nil {
		return 0
	}
	i := strings.IndexByte(string(out), '"')
	if i < 0 {
		return 0
	}
	ver := string(out)[i+1:]
	ver = ver[:strings.IndexByte(ver, '"')]
	ver = strings.TrimPrefix(ver, "1.") // 1.8 -> 8
	n, _ := strconv.Atoi(strings.SplitN(ver, ".", 2)[0])
	return n
}

func hostMachineID() string {
	b, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func result(line string) string { r, _, _ := strings.Cut(line, " :: "); return r }
func detail(line string) string { _, d, _ := strings.Cut(line, " :: "); return d }
func fileExists(p string) bool  { _, err := os.Stat(p); return err == nil }
