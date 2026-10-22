//go:build windows

package sandbox

// Windows red-team harness: run a probe jar (standing in for a malicious mod)
// inside a real AppContainer and record what it can reach, for the Standard and
// Strict presets. Mirrors redteam_test.go (Linux) in shape and output protocol
// so the two matrices read alike. Diagnostic tooling, not a unit test, so it is
// gated behind QUARTZ_REDTEAM=1 and skipped in normal `go test` and CI. It needs
// a JDK (java+javac+jar); point QUARTZ_JDK_BIN at a bin/ holding them, or it
// searches the usual Windows locations.
//
// Invariants it FAILS on (these must always hold): the filesystem wall (launcher
// secret, home-like secret, other instance), the shared-store write protection
// (mods and read-only pack files), the env-allowlist, the positive controls
// (granted store read, game-dir write), and that Strict blocks outbound internet.
// Everything else - the ancestor-listing leak, the leftover detached process,
// loopback reach - is reported as a finding; hardening lands as a separate step.

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"quartz/internal/core/linkx"
	"quartz/internal/core/procx"
	"quartz/internal/sandbox/rules"
)

type winEnv struct {
	root, game, data, scratch, instances string
	store, mod, pack                     string
	secret, home, other, outside         string
	survivor, listen                     string
	jarDir, jarPath, javaBin             string
}

func TestRedteamEscapes(t *testing.T) {
	if os.Getenv("QUARTZ_REDTEAM") == "" {
		t.Skip("set QUARTZ_REDTEAM=1 to run the Windows sandbox red-team suite (needs a JDK)")
	}
	if st := New(Config{}).Status(); !st.Available {
		t.Skip("sandbox unavailable: " + st.Reason)
	}
	javaBin, javacBin, jarBin := findJDK(t)
	// A planted secret in the launcher's own environment: FilterEnv must drop it
	// so the game never inherits it (positive control for the env-leak vector).
	t.Setenv("AWS_SECRET_ACCESS_KEY", "CANARY-LEAKED-SECRET-must-not-reach-the-game")

	results := map[string]map[string]string{}
	for _, p := range []struct {
		name string
		set  rules.Settings
	}{
		{"Standard", rules.Default()},
		{"Strict", rules.Default().Apply(rules.Strict)},
	} {
		// A fresh fixture per preset: a successful write (a finding under the
		// current build) mutates the store, so presets must not share state.
		e := setupWin(t, javaBin)
		compileProbe(t, javacBin, jarBin, e)
		results[p.name] = runProbe(t, e, p.name, p.set)
	}

	reportFindings(t, results)
	checkInvariants(t, results)
}

// --- invariants: these must hold under every preset. ---

func checkInvariants(t *testing.T, results map[string]map[string]string) {
	for preset, got := range results {
		must := func(vector, want string) {
			if v := got[vector]; v != "" && result(v) != want {
				t.Errorf("[%s] INVARIANT BROKEN %s: got %q, want %s :: %s", preset, vector, result(v), want, detail(v))
			}
		}
		mustNot := func(vector, bad string) {
			if v := got[vector]; v != "" && result(v) == bad {
				t.Errorf("[%s] INVARIANT BROKEN %s: got %s :: %s", preset, vector, bad, detail(v))
			}
		}
		// The filesystem wall: nothing outside the grant set is readable.
		must("fs-launcher-secret", "BLOCKED")
		must("fs-home-secret", "BLOCKED")
		must("fs-other-instance", "BLOCKED")
		// Shared store blobs must not be changed through the game folder.
		mustNot("write-store-mod", "REACHABLE")
		mustNot("add-mod", "REACHABLE")
		mustNot("write-protected-file", "REACHABLE")
		// The env allowlist held.
		mustNot("env-leak", "LEAK")
		// Positive controls: the sandbox did not over-restrict.
		must("read-store", "OK")
		must("write-gamedir", "OK")
		// The real user profile must never be listable from inside the jail,
		// nor the ancestors of the game folder (no cross-instance discovery).
		must("list-home", "BLOCKED")
		must("list-ancestors", "BLOCKED")
		// A process the mod detaches must not outlive the game and jail.
		must("leftover-alive", "BLOCKED")
	}
	// Strict is LAN-only: LAN/VPN works but outbound internet must be blocked.
	if s := results["Strict"]; s != nil {
		if r := result(s["net-internet"]); r != "" && r != "BLOCKED" {
			t.Errorf("[Strict] INVARIANT BROKEN net-internet: got %q, want BLOCKED (Strict carries only the LAN capability, no internet)", r)
		}
	}
}

// --- findings: print the full matrix and the confirmed escapes. ---

func reportFindings(t *testing.T, results map[string]map[string]string) {
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
	fmt.Fprintf(&b, "\n=== WINDOWS SANDBOX RED-TEAM MATRIX (runtime proof) ===\n%-22s  %-28s  %-28s\n", "vector", "Standard", "Strict")
	for _, v := range vectors {
		fmt.Fprintf(&b, "%-22s  %-28s  %-28s\n", v, cell(results["Standard"][v]), cell(results["Strict"][v]))
	}
	fmt.Fprintf(&b, "\n--- escapes / leaks / surfaces under Standard (the default preset) ---\n")
	for _, v := range vectors {
		switch result(results["Standard"][v]) {
		case "REACHABLE", "LEAK", "OPEN", "PRESENT":
			fmt.Fprintf(&b, "  [%s] %s :: %s\n", result(results["Standard"][v]), v, detail(results["Standard"][v]))
		}
	}
	t.Log(b.String())
}

func cell(line string) string {
	r, d := result(line), detail(line)
	if d == "" {
		return r
	}
	if len(d) > 22 {
		d = d[:22]
	}
	return r + " " + d
}

// --- running one probe inside the jail ---

func runProbe(t *testing.T, e *winEnv, preset string, set rules.Settings) map[string]string {
	t.Helper()
	// Clear any artifacts a prior preset left so each run starts clean.
	_ = os.Remove(e.survivor)
	_ = linkx.RemoveJunction(filepath.Join(e.game, "breakout"))

	r := Run{
		Instance: "redteam-win-" + preset,
		Rules:    set,
		GameDir:  e.game,
		Scratch:  e.scratch,
		Data:     e.data,
		Java:     JavaRoot(e.javaBin),
		Read:     []string{e.jarDir, filepath.Join(e.root, "store", "libraries")},
	}
	t.Cleanup(func() { _ = New(Config{DataRoot: e.root}).Forget(r.Instance) })

	env := append(FilterEnv(os.Environ()),
		"CANARY_GAME="+e.game,
		"CANARY_SECRET="+e.secret,
		"CANARY_HOME="+e.home,
		"CANARY_OTHER="+e.other,
		"CANARY_STORE="+e.store,
		"CANARY_MOD="+e.mod,
		"CANARY_PACK="+e.pack,
		"CANARY_INSTANCES="+e.instances,
		"CANARY_REALHOME="+os.Getenv("USERPROFILE"),
		"CANARY_LISTEN="+e.listen,
		"CANARY_OUTSIDE="+e.outside,
		"CANARY_SURVIVOR="+e.survivor,
	)
	argv := []string{e.javaBin, "-jar", e.jarPath}
	spec, j, err := New(Config{DataRoot: e.root}).Wrap(r, procx.Spec{Argv: argv, Dir: e.game, Env: env})
	if err != nil {
		t.Fatalf("[%s] Wrap: %v", preset, err)
	}
	c, err := procx.Start(spec)
	if err != nil {
		j.Close()
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
		j.Close()
		t.Fatalf("[%s] probe timed out", preset)
	}
	out, errOut := <-outc, <-errc
	// The game process has exited; release the jail, exactly as the launcher
	// does. Anything the probe left running now is a true leftover.
	j.Close()

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
	got["leftover-alive"] = checkLeftover(e, got["leftover-survivor"])
	return got
}

// checkLeftover measures whether the detached survivor is still writing into
// the granted folder after the game exited and the jail was released, then
// reaps it. REACHABLE means the orphan outlived the sandbox.
func checkLeftover(e *winEnv, survivorLine string) string {
	defer reapSurvivor(survivorLine)
	size := func() int64 {
		fi, err := os.Stat(e.survivor)
		if err != nil {
			return -1
		}
		return fi.Size()
	}
	// Wait long enough for the orphan (a cold-starting powershell) to write at
	// least twice after the jail was released.
	s1 := size()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(600 * time.Millisecond)
		if s2 := size(); s2 > s1 && s1 >= 0 {
			return fmt.Sprintf("REACHABLE :: orphan still writing after jail close (%d->%d bytes)", s1, s2)
		} else if s1 < 0 && s2 >= 0 {
			s1 = s2 // file just appeared; need a second growth sample
		}
	}
	if s1 < 0 {
		why, _ := os.ReadFile(filepath.Join(e.game, "survivor-console.txt"))
		w := strings.ReplaceAll(strings.TrimSpace(string(why)), "\n", " ")
		if len(w) > 60 {
			w = w[:60]
		}
		return "BLOCKED :: survivor file never appeared (console: " + w + ")"
	}
	return "BLOCKED :: survivor stopped with the game"
}

func reapSurvivor(survivorLine string) {
	for _, tok := range strings.Fields(detail(survivorLine)) {
		if pid, ok := strings.CutPrefix(tok, "pid="); ok {
			_ = exec.Command(filepath.Join(os.Getenv("SYSTEMROOT"), "System32", "taskkill.exe"), "/PID", pid, "/T", "/F").Run()
		}
	}
}

// --- the junction / ACL-stamping escape (its own test) ---

func TestJunctionEscape(t *testing.T) {
	if os.Getenv("QUARTZ_REDTEAM") == "" {
		t.Skip("set QUARTZ_REDTEAM=1 to run the Windows sandbox red-team suite (needs a JDK)")
	}
	if st := New(Config{}).Status(); !st.Available {
		t.Skip("sandbox unavailable: " + st.Reason)
	}
	javaBin, javacBin, jarBin := findJDK(t)
	e := setupWin(t, javaBin)
	compileProbe(t, javacBin, jarBin, e)
	instance := "redteam-win-junction"
	t.Cleanup(func() { _ = New(Config{DataRoot: e.root}).Forget(instance) })

	// Run 1: the mod plants a junction game/breakout -> outside (ungranted).
	got := runProbeOnce(t, e, instance, rules.Default())
	if result(got["junction-plant"]) != "PRESENT" {
		t.Skipf("the mod could not plant a junction (%s); the ACL-follow escape needs one", got["junction-plant"])
	}
	link := filepath.Join(e.game, "breakout")
	defer func() { _ = linkx.RemoveJunction(link) }()
	if tgt, ok := linkx.JunctionTarget(link); !ok || !strings.EqualFold(filepath.Clean(tgt), filepath.Clean(e.outside)) {
		t.Fatalf("planted junction points at %q (ok=%v), want %q", tgt, ok, e.outside)
	}
	t.Logf("junction planted: %s -> %s", link, e.outside)

	// Model the next launch: Wrap re-applies the inheritable grant on the game
	// folder via SetNamedSecurityInfo. The question is whether it follows the
	// junction and stamps the container's access onto the outside target.
	rr, rspec := runSpec(e, rules.Default(), instance, nil)
	if _, j, err := New(Config{DataRoot: e.root}).Wrap(rr, rspec); err != nil {
		t.Fatalf("re-wrap: %v", err)
	} else {
		j.Close()
	}

	sid, err := containerSID(instance)
	if err != nil {
		t.Fatal(err)
	}
	outHit, outWhy := sidAllowed(e.outside, sid)
	fileHit, fileWhy := sidAllowed(filepath.Join(e.outside, "secret.txt"), sid)
	t.Logf("outside dir: container-allowed=%v (%s)", outHit, outWhy)
	t.Logf("outside/secret.txt: container-allowed=%v (%s)", fileHit, fileWhy)
	if outHit || fileHit {
		t.Errorf("JUNCTION ESCAPE CONFIRMED: inheritable ACL stamping followed game/breakout and granted the container access to %s (outside every grant)", e.outside)
	}
}

// runProbeOnce runs the probe a single time without the leftover bookkeeping.
func runProbeOnce(t *testing.T, e *winEnv, instance string, set rules.Settings) map[string]string {
	t.Helper()
	env := append(FilterEnv(os.Environ()),
		"CANARY_GAME="+e.game, "CANARY_OUTSIDE="+e.outside, "CANARY_SECRET="+e.secret,
		"CANARY_STORE="+e.store, "CANARY_INSTANCES="+e.instances, "CANARY_REALHOME="+os.Getenv("USERPROFILE"),
		"CANARY_LISTEN="+e.listen, "CANARY_SURVIVOR="+e.survivor,
	)
	r, inSpec := runSpec(e, set, instance, env)
	spec, j, err := New(Config{DataRoot: e.root}).Wrap(r, inSpec)
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	c, err := procx.Start(spec)
	if err != nil {
		j.Close()
		t.Fatalf("Start: %v", err)
	}
	j.Started(c.Process.Pid)
	_ = c.Stdin.Close()
	out, _ := io.ReadAll(c.Stdout)
	go func() { _, _ = io.Copy(io.Discard, c.Stderr) }()
	_ = c.Wait()
	j.Close()
	reapSurvivor("")
	got := map[string]string{}
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok {
			got[k] = v
		}
	}
	return got
}

// sidAllowed reports whether path's DACL carries any allow ACE (inherited or
// explicit) for sid - i.e. whether the container can reach it.
//
//nolint:gosec // Win32 calls on Go-owned memory
func sidAllowed(path string, sid *windows.SID) (bool, string) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false, "no security info: " + err.Error()
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return false, "no dacl"
	}
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(dacl, i, &ace) != nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		if (*windows.SID)(unsafe.Pointer(&ace.SidStart)).Equals(sid) {
			return true, fmt.Sprintf("ace flags=%#x mask=%#x", ace.Header.AceFlags, uint32(ace.Mask))
		}
	}
	return false, "no container ace"
}

// --- fixture & tooling ---

func setupWin(t *testing.T, javaBin string) *winEnv {
	t.Helper()
	// A drive-root test root, deliberately NOT under %TEMP%/%USERPROFILE%, so
	// the parent-traverse grants the sandbox applies never touch the real home.
	base := `C:\quartz-rt`
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(base, fmt.Sprintf("run-%d-", os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = linkx.RemoveJunction(filepath.Join(root, "instances", "a", "game", "breakout"))
		_ = os.RemoveAll(root)
	})

	e := &winEnv{
		root:      root,
		game:      filepath.Join(root, "instances", "a", "game"),
		data:      filepath.Join(root, "instances", "a", "sandbox"),
		scratch:   filepath.Join(root, "instances", "a", "runs", "1.view"),
		instances: filepath.Join(root, "instances"),
		other:     filepath.Join(root, "instances", "b", "game", "accounts.json"),
		store:     filepath.Join(root, "store", "libraries", "lib.jar"),
		pack:      filepath.Join(root, "instances", "a", "game", "resourcepacks", "pack.zip"),
		secret:    filepath.Join(root, "accounts.json"),
		home:      filepath.Join(root, "fakehome", ".ssh", "id_ed25519"),
		outside:   filepath.Join(root, "outside"),
		survivor:  filepath.Join(root, "instances", "a", "game", "survivor-log.txt"),
		jarDir:    filepath.Join(root, "probe"),
		javaBin:   javaBin,
	}
	e.mod = filepath.Join(e.game, "mods", "a.jar")
	e.jarPath = filepath.Join(e.jarDir, "winprobe.jar")

	for _, d := range []string{
		e.game, e.scratch, e.jarDir, filepath.Dir(e.other), filepath.Dir(e.store),
		filepath.Dir(e.home), e.outside, filepath.Join(e.game, "mods"), filepath.Join(e.game, "resourcepacks"),
	} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{e.secret, e.home, e.other, e.store} {
		if err := os.WriteFile(p, []byte(`{"token":"CANARY-SECRET"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(e.outside, "secret.txt"), []byte("CANARY-OUTSIDE"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The instance's mod is the store blob itself (a hard link), sealed
	// read-only; so is a resource pack in an otherwise-writable content folder.
	seal := func(storePath, linkPath string) {
		if err := os.Chmod(storePath, 0o400); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(storePath, linkPath); err != nil {
			t.Fatal(err)
		}
	}
	seal(e.store, e.mod)
	packStore := filepath.Join(root, "store", "content", "pack.zip")
	if err := os.MkdirAll(filepath.Dir(packStore), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packStore, []byte("PACKDATA"), 0o600); err != nil {
		t.Fatal(err)
	}
	seal(packStore, e.pack)

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
	return e
}

func compileProbe(t *testing.T, javacBin, jarBin string, e *winEnv) {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("testdata", "redteam", "WinProbe.java"))
	if err != nil || !fileExists(src) {
		t.Fatalf("probe source not found: %v", err)
	}
	classes := t.TempDir()
	if out, err := exec.Command(javacBin, "-d", classes, src).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.Command(jarBin, "--create", "--file", e.jarPath, "--main-class", "WinProbe", "-C", classes, ".").CombinedOutput(); err != nil {
		t.Fatalf("jar: %v\n%s", err, out)
	}
	// jar.exe writes the archive with a protected DACL (inheritance disabled),
	// so the sandbox's inheritable store-capability grant on the jar folder
	// would never reach it and the contained JVM couldn't open its own jar.
	// Reset it to inherit from the folder, as a normally-created file would.
	// (A harness detail: the launcher's own store files are placed by ordinary
	// Go I/O, which inherits, so this does not reflect a product behaviour.)
	if out, err := exec.Command(filepath.Join(os.Getenv("SYSTEMROOT"), "System32", "icacls.exe"), e.jarPath, "/reset").CombinedOutput(); err != nil {
		t.Fatalf("icacls reset: %v\n%s", err, out)
	}
}

// findJDK returns java, javac and jar from one JDK. QUARTZ_JDK_BIN wins;
// otherwise it searches the usual Windows install locations.
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
		`C:\quartz-rt\jdk\*\bin`,
		`C:\Program Files\Eclipse Adoptium\*\bin`,
		`C:\Program Files\Java\*\bin`,
		`C:\Program Files\Microsoft\*\bin`,
		filepath.Join(home, ".jdks", "*", "bin"),
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
	jv, jc, jr = filepath.Join(bin, "java.exe"), filepath.Join(bin, "javac.exe"), filepath.Join(bin, "jar.exe")
	return jv, jc, jr, fileExists(jv) && fileExists(jc) && fileExists(jr)
}

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
	ver = strings.TrimPrefix(ver, "1.")
	n, _ := strconv.Atoi(strings.SplitN(ver, ".", 2)[0])
	return n
}

func result(line string) string { r, _, _ := strings.Cut(line, " :: "); return r }
func detail(line string) string { _, d, _ := strings.Cut(line, " :: "); return d }
func fileExists(p string) bool  { _, err := os.Stat(p); return err == nil }

// runSpec builds the Run and Spec for a probe from the fixture, so the main
// suite and the junction test wrap identically.
func runSpec(e *winEnv, s rules.Settings, instance string, env []string) (Run, procx.Spec) {
	r := Run{
		Instance: instance, Rules: s, GameDir: e.game, Scratch: e.scratch, Data: e.data,
		Java: JavaRoot(e.javaBin), Read: []string{e.jarDir, filepath.Join(e.root, "store", "libraries")},
	}
	return r, procx.Spec{Argv: []string{e.javaBin, "-jar", e.jarPath}, Dir: e.game, Env: env}
}

// TestServerConnectivitySpike launches a headless listener inside a real
// AppContainer and measures whether the host can reach it on loopback and on
// the machine's LAN IP, to decide how far a sandboxed Windows server is
// joinable without an (admin-only) loopback exemption. Diagnostic, not an
// assertion: it logs the reachability matrix.
func TestServerConnectivitySpike(t *testing.T) {
	if os.Getenv("QUARTZ_REDTEAM") == "" {
		t.Skip("set QUARTZ_REDTEAM=1 to run the Windows sandbox spikes (needs a JDK)")
	}
	if st := New(Config{}).Status(); !st.Available {
		t.Skip("sandbox unavailable: " + st.Reason)
	}
	javaBin, javacBin, jarBin := findJDK(t)
	e := setupWin(t, javaBin)
	instance := "redteam-win-serverspike"
	t.Cleanup(func() { _ = New(Config{DataRoot: e.root}).Forget(instance) })

	// A tiny headless server: bind 0.0.0.0:0, announce the port, echo "OK".
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "ServerSpike.java")
	if err := os.WriteFile(src, []byte(`import java.io.*; import java.net.*;
public class ServerSpike { public static void main(String[] a) throws Exception {
  ServerSocket ss = new ServerSocket();
  ss.bind(new InetSocketAddress("0.0.0.0", 0));
  System.out.println("PORT=" + ss.getLocalPort()); System.out.flush();
  while (true) { try (Socket s = ss.accept()) { s.getOutputStream().write("OK\n".getBytes()); s.getOutputStream().flush(); } catch (Exception e) {} }
}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	classes := t.TempDir()
	if out, err := exec.Command(javacBin, "-d", classes, src).CombinedOutput(); err != nil {
		t.Fatalf("javac: %s", out)
	}
	jar := filepath.Join(e.jarDir, "serverspike.jar")
	if out, err := exec.Command(jarBin, "--create", "--file", jar, "--main-class", "ServerSpike", "-C", classes, ".").CombinedOutput(); err != nil {
		t.Fatalf("jar: %s", out)
	}
	_, _ = exec.Command(filepath.Join(os.Getenv("SYSTEMROOT"), "System32", "icacls.exe"), jar, "/reset").CombinedOutput()

	// Headless under Strict (Internet=LAN) — the server keeps the LAN network.
	r := Run{Instance: instance, Rules: rules.Default().Apply(rules.Strict), Headless: true,
		GameDir: e.game, Scratch: e.scratch, Data: e.data,
		Java: JavaRoot(javaBin), Read: []string{e.jarDir, filepath.Join(e.root, "store", "libraries")}}
	spec, j, err := New(Config{DataRoot: e.root}).Wrap(r, procx.Spec{Argv: []string{javaBin, "-jar", jar}, Dir: e.game, Env: FilterEnv(os.Environ())})
	if err != nil {
		t.Fatalf("Wrap(headless): %v", err)
	}
	c, err := procx.Start(spec)
	if err != nil {
		j.Close()
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = procx.Kill(c.Process); j.Close() }()
	_ = c.Stdin.Close()

	portc := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(c.Stdout)
		for sc.Scan() {
			if p, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "PORT="); ok {
				portc <- p
				return
			}
		}
		portc <- ""
	}()
	var port string
	select {
	case port = <-portc:
	case <-time.After(30 * time.Second):
		t.Fatal("server never announced a port (bind failed inside the container?)")
	}
	if port == "" {
		t.Fatal("server bound no port — headless network capability missing")
	}
	t.Logf("sandboxed headless server bound 0.0.0.0:%s", port)

	dial := func(host string) string {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 3*time.Second)
		if err != nil {
			return "UNREACHABLE: " + err.Error()
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		b, _ := io.ReadAll(conn)
		return "REACHABLE reply=" + strings.TrimSpace(string(b))
	}
	t.Logf("host -> 127.0.0.1:%s  => %s", port, dial("127.0.0.1"))
	if lan := firstLANIP(); lan != "" {
		t.Logf("host -> %s:%s (LAN) => %s", lan, port, dial(lan))
	} else {
		t.Logf("no non-loopback IPv4 found; skipped LAN dial")
	}
}

func firstLANIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLoopback() {
			return ipn.IP.String()
		}
	}
	return ""
}
