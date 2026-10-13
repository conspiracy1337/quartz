//go:build !windows

package paths

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"quartz/internal/brand"
)

func realEnv(t *testing.T) (Env, string) {
	t.Helper()
	home := t.TempDir()
	env := SystemEnv("linux")
	env.Getenv = func(k string) string {
		if k == "XDG_DATA_HOME" {
			return filepath.Join(home, "share")
		}
		return ""
	}
	env.ExePath = filepath.Join(home, "bin", "quartz")
	return env, home
}

func TestMoveToAndApply(t *testing.T) {
	env, home := realEnv(t)
	p, err := Resolve("", env)
	if err != nil || p.Chosen != ByDefault || p.Default != filepath.Join(home, "share", brand.DirName) {
		t.Fatalf("%+v %v", p, err)
	}
	if err := p.Ensure(); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(p.Settings(), []byte("{}"), 0o600)
	_ = os.WriteFile(filepath.Join(p.Logs(), "a.log"), []byte("log"), 0o600)

	// A folder with other things in it gets a Quartz folder inside.
	games := filepath.Join(home, "Games")
	_ = os.MkdirAll(filepath.Join(games, "other"), 0o700)
	target := Target(games)
	if target != filepath.Join(games, brand.DirName) {
		t.Fatal(target)
	}
	if err := p.MoveTo(target); err != nil {
		t.Fatal(err)
	}
	// The next start resolves to the new folder and moves the data first.
	next, _ := Resolve("", env)
	if next.Root != target || next.Chosen != ByPointer {
		t.Fatalf("%+v", next)
	}
	if err := ApplyMove(next); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(target, "logs", "a.log")); err != nil || string(b) != "log" {
		t.Fatal("logs not moved")
	}
	if _, err := os.Stat(p.Settings()); !os.IsNotExist(err) {
		t.Fatal("the old folder still has settings")
	}
	// Applying again does nothing.
	if err := ApplyMove(next); err != nil {
		t.Fatal(err)
	}

	// Moving back to the OS location removes the pointer.
	if err := next.MoveTo(p.Default); err != nil {
		t.Fatal(err)
	}
	back, _ := Resolve("", env)
	if err := ApplyMove(back); err != nil {
		t.Fatal(err)
	}
	if back.Root != p.Default || !IsDataFolder(p.Default) {
		t.Fatalf("%+v", back)
	}
	if again, _ := Resolve("", env); again.Chosen != ByDefault {
		t.Fatal("the pointer must be gone")
	}
}

func TestMoveToRefuses(t *testing.T) {
	env, home := realEnv(t)
	p, _ := Resolve("", env)
	for _, bad := range []string{"/", "/etc", p.Root, filepath.Join(p.Root, "inside"), filepath.Dir(p.Root), "relative"} {
		if err := p.MoveTo(bad); err == nil {
			t.Errorf("%s: allowed", bad)
		}
	}
	fixed, _ := Resolve(filepath.Join(home, "x"), env)
	if err := fixed.MoveTo(filepath.Join(home, "y")); !errors.Is(err, ErrFixedRoot) {
		t.Fatal(err)
	}
}
