package runview

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"quartz/internal/core/bwrap"
	"slices"
	"testing"
	"time"
)

func put(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func body(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return string(b)
}

func gone(p string) bool { _, err := os.Lstat(p); return os.IsNotExist(err) }

type fixture struct {
	game, shared, runs string
	retitled           []string
	a                  *Assembler
}

func newFixture(t *testing.T, backend string) *fixture {
	dir := t.TempDir()
	f := &fixture{game: filepath.Join(dir, "game"), shared: filepath.Join(dir, "shared", "saves"), runs: filepath.Join(dir, "runs")}
	f.a = New(Config{Backend: backend, Retitle: func(d, suffix string) error {
		f.retitled = append(f.retitled, filepath.Base(d)+"|"+suffix)
		return nil
	}})
	put(t, filepath.Join(f.game, "options.txt"), "fov:0.5")
	put(t, filepath.Join(f.game, "old.txt"), "old")
	put(t, filepath.Join(f.game, "logs", "latest.log"), "first run")
	put(t, filepath.Join(f.game, "saves", "Home", "level.dat"), "home v1")
	put(t, filepath.Join(f.game, "saves", "Home", "region", "r.0.0.mca"), "region v1")
	put(t, filepath.Join(f.shared, "Shared", "level.dat"), "shared v1")
	return f
}

func (f *fixture) req(run string, shared bool) Request {
	r := Request{Game: f.game, Scratch: filepath.Join(f.runs, run+".view")}
	if shared {
		r.Folders = []Folder{{Rel: "saves", Shared: f.shared}}
		_ = os.RemoveAll(filepath.Join(f.game, "saves")) // syncing moved its worlds into the shared copy
	}
	return r
}

func (f *fixture) assemble(t *testing.T, run string, shared bool) *View {
	t.Helper()
	v, err := f.a.Assemble(f.req(run, shared))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func closeView(t *testing.T, v *View) []Copy {
	t.Helper()
	c, err := v.Close(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !gone(v.st.Scratch) {
		t.Fatal("scratch left behind")
	}
	return c
}

// A second copy of an instance works on a private clone; on exit its changes
// merge back without undoing the first copy's.
func TestCloneSecondCopyMergesBack(t *testing.T) {
	f := newFixture(t, "clone")
	first := f.assemble(t, "1", false)
	if first.GameDir() != f.game || first.Backend() != direct {
		t.Fatalf("the first copy runs in place: %s %s", first.GameDir(), first.Backend())
	}
	second := f.assemble(t, "2", false)
	g2 := second.GameDir()
	if g2 == f.game || body(t, filepath.Join(g2, "saves", "Home", "level.dat")) != "home v1" {
		t.Fatal("the second copy needs a private clone")
	}
	time.Sleep(5 * time.Millisecond)
	// The first copy plays Home and changes options; the second one too, and more.
	put(t, filepath.Join(f.game, "saves", "Home", "level.dat"), "home by first")
	put(t, filepath.Join(g2, "saves", "Home", "level.dat"), "home by second")
	put(t, filepath.Join(g2, "saves", "Home", "region", "r.1.0.mca"), "region by second")
	put(t, filepath.Join(g2, "saves", "New", "level.dat"), "new world")
	put(t, filepath.Join(g2, "options.txt"), "fov:0.9")
	put(t, filepath.Join(g2, "logs", "latest.log"), "second run")
	_ = os.Remove(filepath.Join(g2, "old.txt"))
	if body(t, filepath.Join(f.game, "options.txt")) != "fov:0.5" {
		t.Fatal("the second copy leaked into the instance")
	}
	copies := closeView(t, second)
	if !slices.Equal(copies, []Copy{{World: "Home", As: "Home (copy 2)"}}) || !slices.Equal(f.retitled, []string{"Home (copy 2)| (copy 2)"}) {
		t.Fatalf("copies %v retitled %v", copies, f.retitled)
	}
	for p, want := range map[string]string{
		"saves/Home/level.dat":                 "home by first",
		"saves/Home (copy 2)/level.dat":        "home by second",
		"saves/Home (copy 2)/region/r.0.0.mca": "region v1",
		"saves/Home (copy 2)/region/r.1.0.mca": "region by second",
		"saves/New/level.dat":                  "new world",
		"options.txt":                          "fov:0.9",
		"logs/latest.log":                      "first run",
	} {
		if got := body(t, filepath.Join(f.game, p)); got != want {
			t.Errorf("%s = %q, want %q", p, got, want)
		}
	}
	if !gone(filepath.Join(f.game, "old.txt")) || !gone(filepath.Join(f.game, "saves", "Home", "region", "r.1.0.mca")) {
		t.Fatal("deletions and clashing worlds")
	}
	closeView(t, first)
	if len(f.a.leases) != 0 {
		t.Fatalf("leases %v", f.a.leases)
	}
}

// Deletions only apply to files nobody changed since the run started.
func TestCloneDeletionKeepsNewerFile(t *testing.T) {
	f := newFixture(t, "clone")
	first := f.assemble(t, "1", false)
	second := f.assemble(t, "2", false)
	time.Sleep(5 * time.Millisecond)
	put(t, filepath.Join(f.game, "old.txt"), "changed by first")
	_ = os.Remove(filepath.Join(second.GameDir(), "old.txt"))
	closeView(t, second)
	if body(t, filepath.Join(f.game, "old.txt")) != "changed by first" {
		t.Fatal("deleted a file another copy changed")
	}
	closeView(t, first)
}

// Without a mount namespace, a shared folder is cloned into the game folder
// and merged back into the shared copy (the macOS path).
func TestCloneSharedFolderInPlace(t *testing.T) {
	f := newFixture(t, "clone")
	v := f.assemble(t, "1", true)
	saves := filepath.Join(f.game, "saves")
	if body(t, filepath.Join(saves, "Shared", "level.dat")) != "shared v1" {
		t.Fatal("not assembled")
	}
	put(t, filepath.Join(saves, "Shared", "level.dat"), "shared v2")
	closeView(t, v)
	if body(t, filepath.Join(f.shared, "Shared", "level.dat")) != "shared v2" {
		t.Fatal("not merged back")
	}
	if entries, _ := os.ReadDir(saves); len(entries) != 0 {
		t.Fatal("the game folder kept the clone")
	}
}

func TestRecover(t *testing.T) {
	f := newFixture(t, "clone")
	first := f.assemble(t, "1", false)
	second := f.assemble(t, "2", false)
	second.Started(1 << 30) // a process that doesn't exist
	put(t, filepath.Join(second.GameDir(), "options.txt"), "fov:1.0")
	// The launcher quits without closing; next start recovers.
	fresh := New(Config{Backend: "clone"})
	fresh.Recover([]string{second.st.Scratch, filepath.Join(f.runs, "missing.view")})
	if body(t, filepath.Join(f.game, "options.txt")) != "fov:1.0" || !gone(second.st.Scratch) {
		t.Fatal("not recovered")
	}
	closeView(t, first)
}

func bwrapOrSkip(t *testing.T) {
	t.Helper()
	if s := bwrap.Detect(); s.Bin == "" || !s.Overlay {
		t.Skip("bubblewrap with overlays isn't available here")
	}
}

func (f *fixture) run(t *testing.T, v *View, script string) {
	t.Helper()
	argv := v.Wrap([]string{"/bin/sh", "-c", script})
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // test
	cmd.Dir = v.GameDir()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

// The real thing on Linux: the first copy sees the shared worlds through a
// bind mount; a second copy gets overlays and merges back on exit.
func TestBwrapBindAndOverlay(t *testing.T) {
	bwrapOrSkip(t)
	f := newFixture(t, "bwrap")
	first := f.assemble(t, "1", true)
	if first.GameDir() != f.game || first.Backend() != "bwrap" {
		t.Fatal(first.Backend())
	}
	f.run(t, first, `echo -n "shared v2" > saves/Shared/level.dat && echo -n first > first.txt`)
	if body(t, filepath.Join(f.shared, "Shared", "level.dat")) != "shared v2" || body(t, filepath.Join(f.game, "first.txt")) != "first" {
		t.Fatal("the first copy must write the real folders")
	}
	if entries, _ := os.ReadDir(filepath.Join(f.game, "saves")); len(entries) != 0 {
		t.Fatal("the bind mount leaked")
	}

	second := f.assemble(t, "2", true)
	f.run(t, second, `echo -n "made by second" > saves/Made/level.dat 2>/dev/null || (mkdir -p saves/Made && echo -n "made by second" > saves/Made/level.dat)
		rm old.txt && echo -n "fov:0.8" > options.txt
		rm -rf logs && mkdir logs && echo -n x > logs/other.txt`)
	if body(t, filepath.Join(f.game, "options.txt")) != "fov:0.5" || gone(filepath.Join(f.game, "old.txt")) || !gone(filepath.Join(f.shared, "Made")) {
		t.Fatal("the second copy wrote the real folders")
	}
	closeView(t, second)
	if body(t, filepath.Join(f.shared, "Made", "level.dat")) != "made by second" || body(t, filepath.Join(f.game, "options.txt")) != "fov:0.8" ||
		!gone(filepath.Join(f.game, "old.txt")) || body(t, filepath.Join(f.game, "logs", "other.txt")) != "x" {
		t.Fatal("not merged back")
	}
	// logs/ was replaced (an opaque folder), but latest.log is each run's own.
	if body(t, filepath.Join(f.game, "logs", "latest.log")) != "first run" {
		t.Fatal("latest.log")
	}
	closeView(t, first)
}

// A sandbox's "Read only" shared folders: the run may write, nothing reaches
// the shared copy, and the game folder is empty afterwards.
func TestThrowawayFolders(t *testing.T) {
	for _, backend := range []string{"clone", "bwrap"} {
		t.Run(backend, func(t *testing.T) {
			if backend == "bwrap" {
				bwrapOrSkip(t)
			}
			f := newFixture(t, backend)
			_ = os.RemoveAll(filepath.Join(f.game, "saves"))
			other := filepath.Join(filepath.Dir(f.shared), "config")
			put(t, filepath.Join(other, "mod.toml"), "shared config")
			r := Request{Game: f.game, Scratch: filepath.Join(f.runs, "1.view"), Folders: []Folder{
				{Rel: "saves", Shared: f.shared, Mode: Private},
				{Rel: "config", Shared: other, Mode: Private},
			}}
			v, err := f.a.Assemble(r)
			if err != nil {
				t.Fatal(err)
			}
			script := `test "$(cat saves/Shared/level.dat)" = "shared v1" && echo -n changed > saves/Shared/level.dat && test "$(cat config/mod.toml)" = "shared config" && echo -n x > config/new.toml`
			if backend == "bwrap" {
				f.run(t, v, script)
			} else {
				cmd := exec.Command("/bin/sh", "-c", script)
				cmd.Dir = v.GameDir()
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %s", err, out)
				}
			}
			closeView(t, v)
			if body(t, filepath.Join(f.shared, "Shared", "level.dat")) != "shared v1" || body(t, filepath.Join(other, "mod.toml")) != "shared config" || !gone(filepath.Join(other, "new.toml")) {
				t.Fatal("a throwaway folder wrote the shared copy")
			}
			for _, rel := range []string{"saves", "config"} {
				if entries, _ := os.ReadDir(filepath.Join(f.game, rel)); len(entries) != 0 {
					t.Fatalf("%s kept the run's files", rel)
				}
			}
			// Throwaway folders don't hold the shared copy: a second run gets it directly.
			if only := f.a.lease([]string{f.shared}); !only[f.shared] {
				t.Fatal("a throwaway folder leased the shared copy")
			}
		})
	}
}

// Touched finds a world a run opened in its own copy (it would merge back
// over the real one), and leaves worlds it sees for real to session.lock.
func TestTouched(t *testing.T) {
	f := newFixture(t, "clone")
	first := f.assemble(t, "1", true) // the game folder direct, shared saves cloned in place
	shared := filepath.Join(f.shared, "Shared")
	if first.Touched(shared) {
		t.Fatal("untouched world")
	}
	put(t, filepath.Join(f.game, "saves", "Shared", "session.lock"), "☃")
	if !first.Touched(shared) || first.Touched(f.shared) {
		t.Fatal("a world opened in the clone")
	}

	home := filepath.Join(f.game, "saves", "Home")
	second := f.assemble(t, "2", false) // a clone of the game folder; saves inside it
	if second.Touched(home) || first.Touched(home) {
		t.Fatal("untouched world in the game folder")
	}
	put(t, filepath.Join(second.GameDir(), "saves", "Home", "session.lock"), "☃")
	if !second.Touched(home) {
		t.Fatal("a world opened in the game folder's clone")
	}
	closeView(t, second)

	third := f.assemble(t, "3", true) // the game clone skips saves: the shared mount holds them
	put(t, filepath.Join(third.GameDir(), "saves", "Home", "session.lock"), "☃")
	if third.Touched(home) {
		t.Fatal("a skipped folder belongs to another mount")
	}
	closeView(t, third)
	closeView(t, first)

	up := t.TempDir()
	ov := &View{a: f.a, st: state{Start: time.Now(), Mounts: []mount{{Kind: overlay, Path: f.game, Target: f.game, Upper: up}}}}
	put(t, filepath.Join(f.game, "saves", "Home", "session.lock"), "☃")
	if ov.Touched(home) {
		t.Fatal("an overlay sees the real lock through its lower folder")
	}
	put(t, filepath.Join(up, "saves", "Home", "session.lock"), "☃")
	if !ov.Touched(home) {
		t.Fatal("a world opened in an overlay")
	}
	direct := &View{a: f.a, st: state{Start: time.Now().Add(-time.Hour), Mounts: []mount{{Kind: direct, Path: f.game, Target: f.game}}}}
	if direct.Touched(home) {
		t.Fatal("a direct run has no copy")
	}
}

// With LiveSaves a second copy writes worlds straight into the real saves,
// whether they live in the game folder or in a shared copy, while the rest
// of the game folder stays its private view until it exits.
func TestBwrapLiveSaves(t *testing.T) {
	bwrapOrSkip(t)
	for _, shared := range []bool{false, true} {
		f := newFixture(t, "bwrap")
		saves := filepath.Join(f.game, "saves")
		if shared {
			saves = f.shared
		}
		live := func(run string) *View {
			r := f.req(run, shared)
			r.LiveSaves = true
			v, err := f.a.Assemble(r)
			if err != nil {
				t.Fatal(err)
			}
			return v
		}
		first := live("1")
		if !shared && first.Backend() != direct {
			t.Fatalf("a first copy runs in place, got %s", first.Backend())
		}
		second := live("2")
		f.run(t, second, `mkdir -p saves/Made && echo -n "made by second" > saves/Made/level.dat && echo -n "fov:0.8" > options.txt`)
		if body(t, filepath.Join(saves, "Made", "level.dat")) != "made by second" {
			t.Fatalf("shared=%v: the world didn't reach the real saves while running", shared)
		}
		if body(t, filepath.Join(f.game, "options.txt")) != "fov:0.5" {
			t.Fatalf("shared=%v: the rest must stay private while running", shared)
		}
		if kept := closeView(t, second); len(kept) != 0 {
			t.Fatalf("shared=%v: live worlds keep no copies: %+v", shared, kept)
		}
		if body(t, filepath.Join(f.game, "options.txt")) != "fov:0.8" || body(t, filepath.Join(saves, "Made", "level.dat")) != "made by second" {
			t.Fatalf("shared=%v: not merged back", shared)
		}
		closeView(t, first)
	}
}
