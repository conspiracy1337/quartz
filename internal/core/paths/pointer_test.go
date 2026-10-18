//go:build !windows

package paths

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"quartz/internal/brand"
)

func realEnv(t *testing.T) (Env, string) {
	t.Helper()
	// Tests run in the temporary folder, which Problem refuses.
	old := tempDir
	tempDir = func() string { return "/nonexistent-temp" }
	t.Cleanup(func() { tempDir = old })
	home := t.TempDir()
	env := SystemEnv("linux")
	env.Getenv = func(k string) string {
		switch k {
		case "XDG_DATA_HOME":
			return filepath.Join(home, "share")
		case "XDG_CONFIG_HOME":
			return filepath.Join(home, "config")
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

	// A folder with other things in it gets a quartz folder inside.
	games := filepath.Join(home, "Games")
	_ = os.MkdirAll(filepath.Join(games, "other"), 0o700)
	target := Target(games)
	if target != filepath.Join(games, brand.DirName) {
		t.Fatal(target)
	}
	if err := p.MoveTo(target); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, "config", brand.DirName)
	if _, err := os.Stat(filepath.Join(config, pointerFile)); err != nil {
		t.Fatal("no pointer in the config folder:", err)
	}
	// The next start resolves to the new folder and moves the data first.
	next, _ := Resolve("", env)
	if next.Root != target || next.Chosen != ByPointer {
		t.Fatalf("%+v", next)
	}
	if moved, err := ApplyMove(next); err != nil || moved.Root != target {
		t.Fatal(moved, err)
	}
	if b, err := os.ReadFile(filepath.Join(target, "logs", "a.log")); err != nil || string(b) != "log" {
		t.Fatal("logs not moved")
	}
	if _, err := os.Stat(p.Root); !os.IsNotExist(err) {
		t.Fatal("the old folder stayed behind")
	}
	// Applying again does nothing.
	if _, err := ApplyMove(next); err != nil {
		t.Fatal(err)
	}

	// Moving back to the OS location removes the pointer.
	if err := next.MoveTo(p.Default); err != nil {
		t.Fatal(err)
	}
	back, _ := Resolve("", env)
	if _, err := ApplyMove(back); err != nil {
		t.Fatal(err)
	}
	if back.Root != p.Default || !IsDataFolder(p.Default) {
		t.Fatalf("%+v", back)
	}
	if again, _ := Resolve("", env); again.Chosen != ByDefault {
		t.Fatal("the pointer must be gone")
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatal("the empty config folder stayed behind")
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

// A move that fails half way is cancelled: the old folder stays the data
// folder.
func TestApplyMoveCancelsOnFailure(t *testing.T) {
	env, home := realEnv(t)
	p, _ := Resolve("", env)
	if err := p.Ensure(); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(p.Settings(), []byte("{}"), 0o600)
	target := filepath.Join(home, "elsewhere")
	if err := p.MoveTo(target); err != nil {
		t.Fatal(err)
	}
	old := moveData
	moveData = func(string, string) error { return errors.New("disk full") }
	t.Cleanup(func() { moveData = old })

	next, _ := Resolve("", env)
	got, err := ApplyMove(next)
	if err == nil || got.Root != p.Root || got.Chosen != ByDefault {
		t.Fatalf("%+v %v", got, err)
	}
	if again, _ := Resolve("", env); again.Root != p.Root || again.Chosen != ByDefault {
		t.Fatalf("the move wasn't cancelled: %+v", again)
	}
}

// move is all or nothing: a copy that fails leaves both folders as they were.
func TestMoveUndoes(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads every file")
	}
	from, to := filepath.Join(t.TempDir(), "from"), filepath.Join(t.TempDir(), "to")
	_ = os.MkdirAll(filepath.Join(from, "store"), 0o700)
	_ = os.WriteFile(filepath.Join(from, "settings.json"), []byte("{}"), 0o600)
	locked := filepath.Join(from, "store", "locked")
	_ = os.WriteFile(locked, []byte("x"), 0)
	// Renames out of a read-only folder fail, so it copies, and can't read "locked".
	_ = os.Chmod(from, 0o500)
	t.Cleanup(func() { _ = os.Chmod(from, 0o700) })
	if err := move(from, to); err == nil {
		t.Fatal("moved an unreadable file")
	}
	if !IsDataFolder(from) {
		t.Fatal("the old folder lost its data")
	}
	if entries, _ := os.ReadDir(to); len(entries) != 0 {
		t.Fatalf("the new folder kept %v", entries)
	}
}

func TestProblem(t *testing.T) {
	old := tempDir
	tempDir = func() string { return "/tmp" }
	t.Cleanup(func() { tempDir = old })
	for root, want := range map[string]string{
		"/home/a/.local/share/quartz": "",
		"/home/a!b/quartz":            ProblemBang,
		"/tmp/quartz":                 ProblemTemp,
		"/tmpfoo/quartz":              "",
		`C:\Users\a\AppData\Local\Temp\Rar$EXa1.2\data`: ProblemTemp,
		"/x/TempState/quartz":                           ProblemTemp,
	} {
		if got := Problem(root); got != want {
			t.Errorf("%s: %q, want %q", root, got, want)
		}
	}
	env, home := realEnv(t)
	p, _ := Resolve("", env)
	if err := p.MoveTo(filepath.Join(home, "a!b")); !errors.Is(err, ErrBadRoot) {
		t.Fatal(err)
	}
}

// Across drives, a move needs the room first, and keeps store files shared
// and times as they were (nothing is verified again afterwards).
func TestCopyStaged(t *testing.T) {
	from, to := filepath.Join(t.TempDir(), "from"), filepath.Join(t.TempDir(), "to")
	blob := filepath.Join(from, "store", "blob")
	mod := filepath.Join(from, "instances", "a", "mods", "x.jar")
	for _, d := range []string{filepath.Dir(blob), filepath.Dir(mod), to} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(blob, []byte("jar"), 0o400)
	if err := os.Link(blob, mod); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	_ = os.Chtimes(blob, old, old)

	free := diskFree
	t.Cleanup(func() { diskFree = free })
	diskFree = func(string) (uint64, error) { return 1 << 20, nil }
	if err := copyStaged(from, to, []string{"store", "instances"}); !errors.Is(err, ErrNoRoom) {
		t.Fatalf("err = %v, want ErrNoRoom", err)
	}
	if entries, _ := os.ReadDir(to); len(entries) != 0 {
		t.Fatalf("a refused move left %v", entries)
	}

	diskFree = func(string) (uint64, error) { return 1 << 40, nil }
	if err := copyStaged(from, to, []string{"store", "instances"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(filepath.Join(to, "store", "blob"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := os.Stat(filepath.Join(to, "instances", "a", "mods", "x.jar"))
	if err != nil || !os.SameFile(b, m) {
		t.Fatalf("the mod is no longer the store file: %v", err)
	}
	if !b.ModTime().Equal(old) || b.Mode().Perm()&0o200 != 0 {
		t.Fatalf("store file: %v %v", b.ModTime(), b.Mode())
	}
	if _, err := os.Stat(blob); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the original stayed behind")
	}
}

func TestMoveTakesTheWebviewLast(t *testing.T) {
	from, to := t.TempDir(), filepath.Join(t.TempDir(), "new")
	for _, f := range []string{"settings.json", "webview/EBWebView/lockfile", "webview/EBWebView/Default/Preferences"} {
		p := filepath.Join(from, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := move(from, to); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"settings.json", "webview/EBWebView/Default/Preferences"} {
		if _, err := os.Stat(filepath.Join(to, f)); err != nil {
			t.Errorf("%s didn't move: %v", f, err)
		}
	}
	if entries, _ := os.ReadDir(from); len(entries) != 0 {
		t.Errorf("left behind: %v", entries)
	}
}
