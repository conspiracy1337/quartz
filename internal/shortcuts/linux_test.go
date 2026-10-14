//go:build linux

package shortcuts

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quartz/internal/instance"
	"quartz/internal/intent"
)

func testService(t *testing.T, exe string) (*Service, *instance.Store, linux) {
	t.Helper()
	root := t.TempDir()
	store, err := instance.Open(filepath.Join(root, "instances"), nil)
	if err != nil {
		t.Fatal(err)
	}
	w := linux{desktopDir: filepath.Join(root, "Desktop"), menuDir: filepath.Join(root, "applications")}
	if err := os.MkdirAll(w.desktopDir, 0o700); err != nil {
		t.Fatal(err)
	}
	s := &Service{w: w, dir: filepath.Join(root, "data", "shortcuts"), store: store, exe: exe, log: slog.New(slog.DiscardHandler)}
	return s, store, w
}

func TestCreateHideRestore(t *testing.T) {
	s, store, w := testService(t, "/opt/quartz/quartz")
	in, err := store.Create(instance.CreateRequest{Name: "My Pack", Minecraft: "1.21.4"})
	if err != nil {
		t.Fatal(err)
	}
	into := instance.Into{Kind: "server", Target: "mc.example.org"}
	paths, err := s.Create(in.ID, []Where{Desktop, Menu}, "0123456789abcdef0123456789abcdef", into)
	if err != nil || len(paths) != 2 {
		t.Fatal(paths, err)
	}
	if paths[0] != filepath.Join(w.desktopDir, "My Pack.desktop") || filepath.Dir(paths[1]) != w.menuDir {
		t.Fatal(paths)
	}
	data, _ := os.ReadFile(paths[0])
	for _, want := range []string{
		`Exec="/opt/quartz/quartz" "--launch" "` + in.ID + `" "--server" "mc.example.org" "--account" "0123456789abcdef0123456789abcdef"`,
		"Name=My Pack", markerKey + "=" + in.ID,
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %q in\n%s", want, data)
		}
	}
	if info, _ := os.Stat(paths[0]); info.Mode()&0o100 == 0 {
		t.Error("a desktop launcher must be executable")
	}
	got, _ := store.Get(in.ID)
	if len(got.Shortcuts) != 2 || got.Shortcuts[0].Into != into {
		t.Fatalf("%+v", got.Shortcuts)
	}

	s.Hide(got)
	for _, p := range paths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s survived Hide", p)
		}
	}
	s.Restore(got)
	got, _ = store.Get(in.ID)
	for _, r := range got.Shortcuts {
		if _, err := os.Stat(r.Path); err != nil {
			t.Errorf("not restored: %v", err)
		}
	}
}

func TestRemoveLeavesOtherFiles(t *testing.T) {
	_, _, w := testService(t, "/x")
	foreign := filepath.Join(w.desktopDir, "Other.desktop")
	if err := os.WriteFile(foreign, []byte("[Desktop Entry]\nName=Other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := w.remove(foreign, "abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal("a file without our marker must stay")
	}
	outside := filepath.Join(t.TempDir(), "x.desktop")
	_ = os.WriteFile(outside, []byte("\n"+markerKey+"=abc\n"), 0o600)
	_ = w.remove(outside, "abc")
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("a file outside our folders must stay")
	}
}

func TestRefreshRewritesAfterAMove(t *testing.T) {
	s, store, _ := testService(t, "/old/quartz")
	in, _ := store.Create(instance.CreateRequest{Name: "Pack", Minecraft: "1.21.4"})
	paths, err := s.Create(in.ID, []Where{Desktop}, "", instance.Into{})
	if err != nil {
		t.Fatal(err)
	}
	s.Refresh() // records /old/quartz
	s.exe = "/new/quartz"
	s.Refresh()
	data, _ := os.ReadFile(paths[0])
	if !strings.Contains(string(data), `Exec="/new/quartz"`) {
		t.Fatalf("not rewritten:\n%s", data)
	}
}

// A launcher started with --dir or QUARTZ_DIR gives its shortcuts the data
// folder, or they would open another one.
func TestShortcutKeepsDataFolder(t *testing.T) {
	s, store, _ := testService(t, "/opt/quartz/quartz")
	s.lead = intent.DirArgs("/mnt/games/quartz")
	in, err := store.Create(instance.CreateRequest{Name: "Pack", Minecraft: "1.21.4"})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := s.Create(in.ID, []Where{Desktop}, "", instance.Into{})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(paths[0])
	want := `Exec="/opt/quartz/quartz" "--dir" "/mnt/games/quartz" "--launch" "` + in.ID + `"`
	if !strings.Contains(string(data), want) {
		t.Fatalf("missing %q in\n%s", want, data)
	}
}
