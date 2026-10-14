package fsx

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestLockHelper holds a lock the way the game does until its stdin closes;
// it only runs as TestLocked's helper process.
func TestLockHelper(t *testing.T) {
	p := os.Getenv("FSX_LOCK_HELPER")
	if p == "" {
		t.Skip("helper process only")
	}
	unlock, err := holdLock(p)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	_, _ = os.Stdout.WriteString("locked\n")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func TestLocked(t *testing.T) {
	p := filepath.Join(t.TempDir(), "session.lock")
	if err := os.WriteFile(p, []byte("☃"), 0o600); err != nil {
		t.Fatal(err)
	}
	if Locked(p) {
		t.Fatal("an unlocked file reads as locked")
	}
	if Locked(filepath.Join(t.TempDir(), "missing")) {
		t.Fatal("a missing file reads as locked")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockHelper$") //nolint:gosec // the test binary itself
	cmd.Env = append(os.Environ(), "FSX_LOCK_HELPER="+p)
	stdin, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close(); _ = cmd.Wait() }()
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "locked\n" {
		t.Fatalf("helper: %q %v", line, err)
	}
	if !Locked(p) {
		t.Fatal("a file another process locked reads as unlocked")
	}
}
