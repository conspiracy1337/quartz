//go:build windows

package sandbox

// Windows red-team harness, ROUND 3. Exercises the next layer of AppContainer
// escapes beyond redteam_windows_test.go: broker/service job-escape, sealed-
// folder delete/recreate, CREATE_BREAKAWAY_FROM_JOB, registry persistence,
// clipboard read, and a pre-planted reparse point stamped by the launcher's own
// ACL pass. Diagnostic tooling, gated behind QUARTZ_REDTEAM; it records a matrix
// and the confirmed escapes without asserting invariants (this is an audit pass,
// not a product change). Reuses the round-1 fixture helpers (setupWin, findJDK,
// result/detail/fileExists/cell, sidAllowed, runSpec) from the same package.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"quartz/internal/core/linkx"
	"quartz/internal/core/procx"
	"quartz/internal/sandbox/rules"
)

const regRunKeyHKCU = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
const regRunKeyHKLM = `HKLM\Software\Microsoft\Windows\CurrentVersion\Run`

func TestRound3Escapes(t *testing.T) {
	if os.Getenv("QUARTZ_REDTEAM") == "" {
		t.Skip("set QUARTZ_REDTEAM=1 to run the Windows sandbox round-3 suite (needs a JDK + Go)")
	}
	if st := New(Config{}).Status(); !st.Available {
		t.Skip("sandbox unavailable: " + st.Reason)
	}
	javaBin, javacBin, jarBin := findJDK(t)
	breakawayExe := buildBreakaway(t)

	results := map[string]map[string]string{}
	for _, p := range []struct {
		name string
		set  rules.Settings
	}{
		{"Standard", rules.Default()},
		{"Strict", rules.Default().Apply(rules.Strict, runtime.GOOS)},
	} {
		e := setupWin(t, javaBin)
		jar := compileProbe2(t, javacBin, jarBin, e)
		results[p.name] = runProbe3(t, e, p.name, p.set, jar, breakawayExe)
	}
	reportRound3(t, results)
}

// runProbe3 runs WinProbe2 inside a real AppContainer, then after the jail is
// closed measures whether any broker/breakaway survivor is still writing and
// whether the registry write reached the real user hive.
func runProbe3(t *testing.T, e *winEnv, preset string, set rules.Settings, jar, breakawayExe string) map[string]string {
	t.Helper()

	schtasksFile := filepath.Join(e.game, "broker-schtasks.txt")
	wmiFile := filepath.Join(e.game, "broker-wmi.txt")
	breakawayFile := filepath.Join(e.game, "breakaway.txt")

	// The container can only execute files in a granted folder; jarDir is granted
	// read+execute via the store capability, so stage the helper there (its own
	// TempDir is outside every grant and "Cannot run program" otherwise).
	stagedExe := filepath.Join(e.jarDir, "breakaway.exe")
	if data, err := os.ReadFile(breakawayExe); err == nil {
		if err := os.WriteFile(stagedExe, data, 0o700); err != nil {
			t.Fatalf("stage breakaway.exe: %v", err)
		}
		icacls(stagedExe, "/reset")
		breakawayExe = stagedExe
	}
	taskName := fmt.Sprintf("QuartzRedteam_%s_%d", preset, os.Getpid())
	regMarker := fmt.Sprintf("%s-%d", preset, time.Now().UnixNano())

	// Clean up anything this run creates outside the auto-removed test root.
	t.Cleanup(func() {
		reapByCmdline(schtasksFile, wmiFile, breakawayFile,
			e.game+`\sched-payload.ps1`, e.game+`\wmi-payload.cmd`)
		_, _ = sch(taskName, "/end")
		_, _ = sch(taskName, "/delete", "/f")
		regDelete(regRunKeyHKCU, "QuartzRedteam")
		regDelete(regRunKeyHKLM, "QuartzRedteam")
	})

	// Seed the clipboard with a secret the contained probe will try to read.
	clip := "CANARY-CLIPBOARD-" + regMarker
	setClipboard(clip)

	r := Run{
		Instance: "redteam-win-r3-" + preset,
		Rules:    set,
		GameDir:  e.game,
		Scratch:  e.scratch,
		Data:     e.data,
		Read:     []string{JavaRoot(e.javaBin), e.jarDir, filepath.Join(e.root, "store", "libraries")},
	}
	t.Cleanup(func() { _ = New(Config{DataRoot: e.root}).Forget(r.Instance) })

	env := append(FilterEnv(os.Environ()),
		"CANARY_GAME="+e.game,
		"CANARY_STORE="+e.store,
		"CANARY_SECRET="+e.secret,
		"CANARY_MOD="+e.mod,
		"CANARY_OUTSIDE="+e.outside,
		"CANARY_TASKNAME="+taskName,
		"CANARY_BROKER_SCHTASKS="+schtasksFile,
		"CANARY_BROKER_WMI="+wmiFile,
		"CANARY_BREAKAWAY="+breakawayFile,
		"CANARY_BREAKAWAY_EXE="+breakawayExe,
		"CANARY_REGMARKER="+regMarker,
		"CANARY_CLIP="+clip,
	)
	spec, j, err := New(Config{DataRoot: e.root}).Wrap(r, procx.Spec{Argv: []string{e.javaBin, "-jar", jar}, Dir: e.game, Env: env})
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
	case <-time.After(120 * time.Second):
		_ = procx.Kill(c.Process)
		j.Close()
		t.Fatalf("[%s] probe timed out", preset)
	}
	out, errOut := <-outc, <-errc
	// The game has exited; release the jail exactly as the launcher does.
	// Anything still writing a canary now outlived the sandbox.
	j.Close()

	got := map[string]string{}
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok {
			got[k] = v
		}
	}
	if len(got) == 0 {
		t.Fatalf("[%s] probe printed nothing; stderr:\n%s", preset, errOut)
	}

	// Measure survivors after jail close (same method as checkLeftover).
	got["broker-schtasks-alive"] = survived(schtasksFile)
	got["broker-wmi-alive"] = survived(wmiFile)
	got["breakaway-alive"] = survived(breakawayFile)
	// Did the HKCU Run write reach the REAL user hive (true persistence) or a
	// virtualised package view (contained)?
	got["reg-hkcu-persist"] = regPersisted(regRunKeyHKCU, "QuartzRedteam", regMarker)
	return got
}

// survived reports REACHABLE if the canary keeps growing after the jail closed.
func survived(file string) string {
	size := func() int64 {
		fi, err := os.Stat(file)
		if err != nil {
			return -1
		}
		return fi.Size()
	}
	s1 := size()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(600 * time.Millisecond)
		s2 := size()
		if s1 >= 0 && s2 > s1 {
			return fmt.Sprintf("REACHABLE :: survivor still writing after jail close (%d->%d bytes)", s1, s2)
		}
		if s1 < 0 && s2 >= 0 {
			s1 = s2
		}
	}
	if s1 < 0 {
		return "BLOCKED :: survivor file never appeared"
	}
	return "BLOCKED :: survivor stopped with the game"
}

// regPersisted reports REACHABLE if the real user's Run key now carries our
// value (persistence that survives), BLOCKED if it does not (virtualised away).
func regPersisted(key, name, marker string) string {
	out, _ := regQuery(key, name)
	if strings.Contains(out, "payload-"+marker) {
		return "REACHABLE :: value landed in the real HKCU Run key"
	}
	return "BLOCKED :: not present in the real HKCU (virtualised or denied)"
}

func reportRound3(t *testing.T, results map[string]map[string]string) {
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
	fmt.Fprintf(&b, "\n=== WINDOWS SANDBOX RED-TEAM MATRIX — ROUND 3 (runtime proof) ===\n%-24s  %-30s  %-30s\n", "vector", "Standard", "Strict")
	for _, v := range vectors {
		fmt.Fprintf(&b, "%-24s  %-30s  %-30s\n", v, cell(results["Standard"][v]), cell(results["Strict"][v]))
	}
	fmt.Fprintf(&b, "\n--- escapes / leaks / surfaces under Standard (the default preset) ---\n")
	for _, v := range vectors {
		switch result(results["Standard"][v]) {
		case "REACHABLE", "LEAK", "OPEN", "PRESENT":
			fmt.Fprintf(&b, "  [%s] %s :: %s\n", result(results["Standard"][v]), v, detail(results["Standard"][v]))
		}
	}
	fmt.Fprintf(&b, "\n--- full detail (untruncated), Standard ---\n")
	for _, v := range vectors {
		fmt.Fprintf(&b, "  %-24s %s\n", v, results["Standard"][v])
	}
	t.Log(b.String())
}

// TestRound3PreplantedJunction models a junction that already exists in the game
// folder (planted by a modpack import or a previous unsandboxed run), then asks
// whether Wrap's inheritable ACL grant follows it and stamps the container's
// access onto a target outside every grant. The round-1 TestJunctionEscape only
// covered a mod planting the junction from inside (blocked); here the junction
// pre-exists, which is the realistic case.
func TestRound3PreplantedJunction(t *testing.T) {
	if os.Getenv("QUARTZ_REDTEAM") == "" {
		t.Skip("set QUARTZ_REDTEAM=1 to run the Windows sandbox round-3 suite")
	}
	if st := New(Config{}).Status(); !st.Available {
		t.Skip("sandbox unavailable: " + st.Reason)
	}
	javaBin, _, _ := findJDK(t)
	e := setupWin(t, javaBin)
	instance := "redteam-win-r3-preplant"
	t.Cleanup(func() { _ = New(Config{DataRoot: e.root}).Forget(instance) })

	link := filepath.Join(e.game, "preplanted")
	if err := linkx.Junction(link, e.outside); err != nil {
		t.Skipf("could not pre-plant a junction as the user: %v", err)
	}
	defer func() { _ = linkx.RemoveJunction(link) }()
	if tgt, ok := linkx.JunctionTarget(link); !ok || !strings.EqualFold(filepath.Clean(tgt), filepath.Clean(e.outside)) {
		t.Fatalf("pre-planted junction points at %q (ok=%v), want %q", tgt, ok, e.outside)
	}
	t.Logf("pre-planted junction (as the user): %s -> %s", link, e.outside)

	rr, rspec := runSpec(e, rules.Default(), instance, nil)
	if _, j, err := New(Config{DataRoot: e.root}).Wrap(rr, rspec); err != nil {
		t.Fatalf("wrap over a pre-planted junction: %v", err)
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
		t.Errorf("PRE-PLANTED JUNCTION ESCAPE CONFIRMED: Wrap's inheritable ACL stamping followed %s and granted the container access to %s (outside every grant)", link, e.outside)
	} else {
		t.Logf("HELD: Wrap did not stamp the container onto the junction target")
	}
}

// --- tooling -------------------------------------------------------------

// compileProbe2 builds WinProbe2.java into its own jar, resetting the jar's DACL
// so the contained JVM can read it (same reason as compileProbe).
func compileProbe2(t *testing.T, javacBin, jarBin string, e *winEnv) string {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("testdata", "redteam", "WinProbe2.java"))
	if err != nil || !fileExists(src) {
		t.Fatalf("probe2 source not found: %v", err)
	}
	classes := t.TempDir()
	if out, err := exec.Command(javacBin, "-d", classes, src).CombinedOutput(); err != nil {
		t.Fatalf("javac WinProbe2: %v\n%s", err, out)
	}
	jar := filepath.Join(e.jarDir, "winprobe2.jar")
	if out, err := exec.Command(jarBin, "--create", "--file", jar, "--main-class", "WinProbe2", "-C", classes, ".").CombinedOutput(); err != nil {
		t.Fatalf("jar WinProbe2: %v\n%s", err, out)
	}
	icacls(jar, "/reset")
	return jar
}

// buildBreakaway compiles testdata/redteam/breakaway.go (an explicit file build,
// so the testdata exclusion doesn't hide it) into a throwaway exe.
func buildBreakaway(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("testdata", "redteam", "breakaway.go"))
	if err != nil || !fileExists(src) {
		t.Fatalf("breakaway source not found: %v", err)
	}
	exe := filepath.Join(t.TempDir(), "breakaway.exe")
	cmd := exec.Command(goBinary(t), "build", "-o", exe, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build breakaway helper: %v\n%s", err, out)
	}
	icacls(exe, "/reset")
	return exe
}

// goBinary locates the Go toolchain used to build the native helper.
func goBinary(t *testing.T) string {
	t.Helper()
	cands := []string{}
	if b := os.Getenv("QUARTZ_GO_BIN"); b != "" {
		cands = append(cands, b)
	}
	if gr := runtime.GOROOT(); gr != "" {
		cands = append(cands, filepath.Join(gr, "bin", "go.exe"))
	}
	cands = append(cands, `C:\quartz-rt\go\bin\go.exe`)
	for _, c := range cands {
		if fileExists(c) {
			return c
		}
	}
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	t.Skip("no Go toolchain found to build the breakaway helper (set QUARTZ_GO_BIN)")
	return ""
}

// --- small Windows CLI wrappers (run as the user, outside the sandbox) ---

func sysExe(name string) string {
	return filepath.Join(os.Getenv("SYSTEMROOT"), "System32", name)
}

func icacls(path string, args ...string) {
	_, _ = exec.Command(sysExe("icacls.exe"), append([]string{path}, args...)...).CombinedOutput()
}

func sch(name string, args ...string) (string, error) {
	out, err := exec.Command(sysExe("schtasks.exe"), append([]string{"/tn", name}, args...)...).CombinedOutput()
	return string(out), err
}

func regQuery(key, name string) (string, error) {
	out, err := exec.Command(sysExe("reg.exe"), "query", key, "/v", name).CombinedOutput()
	return string(out), err
}

func regDelete(key, name string) {
	_, _ = exec.Command(sysExe("reg.exe"), "delete", key, "/v", name, "/f").CombinedOutput()
}

func setClipboard(s string) {
	ps := filepath.Join(os.Getenv("SYSTEMROOT"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	_ = exec.Command(ps, "-NoProfile", "-NonInteractive", "-Command", "Set-Clipboard -Value '"+s+"'").Run()
}

// reapByCmdline kills any lingering process whose command line mentions one of
// the given tokens (the survivors' canary files / payload script).
func reapByCmdline(tokens ...string) {
	var ors []string
	for _, tk := range tokens {
		ors = append(ors, "$_.CommandLine -like '*"+strings.ReplaceAll(tk, `\`, `\\`)+"*'")
	}
	script := "Get-CimInstance Win32_Process -Filter \"Name='powershell.exe' OR Name='cmd.exe' OR Name='ping.exe'\" | Where-Object { " +
		strings.Join(ors, " -or ") + " } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }"
	ps := filepath.Join(os.Getenv("SYSTEMROOT"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	_ = exec.Command(ps, "-NoProfile", "-NonInteractive", "-Command", script).Run()
}
