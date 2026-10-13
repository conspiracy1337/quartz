//go:build linux

package desktop

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testLinux(t *testing.T) (*Registrar, linux, *[]string) {
	t.Helper()
	root := t.TempDir()
	var calls []string
	l := linux{dataHome: filepath.Join(root, "data"), configHome: filepath.Join(root, "config"),
		run: func(name string, args ...string) error {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil
		}}
	return &Registrar{path: filepath.Join(root, "quartz", stateFile), log: slog.New(slog.DiscardHandler), sys: l}, l, &calls
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLinuxRegisterAndUnregister(t *testing.T) {
	r, l, calls := testLinux(t)
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		t.Fatal(err)
	}
	want := Want{Exe: "/opt/q uartz/quartz", Links: true, Autostart: true, Version: "1.0.0"}
	if err := r.Sync(want); err != nil {
		t.Fatal(err)
	}
	entry := read(t, l.entryPath())
	for _, s := range []string{`Exec="/opt/q uartz/quartz" %U`, "MimeType=application/x-modrinth-modpack+zip;x-scheme-handler/modrinth;x-scheme-handler/curseforge;x-scheme-handler/quartz;", "Icon=" + desktopID[:len(desktopID)-8]} {
		if !strings.Contains(entry, s) {
			t.Errorf("entry lacks %q:\n%s", s, entry)
		}
	}
	if !strings.Contains(read(t, l.mimePath()), `<glob pattern="*.mrpack"/>`) {
		t.Error("mime type missing")
	}
	if _, err := os.Stat(l.iconPath()); err != nil {
		t.Error(err)
	}
	if !strings.Contains(read(t, l.autostartPath()), `Exec="/opt/q uartz/quartz" "--minimized"`) {
		t.Error("autostart missing")
	}
	if !strings.Contains(strings.Join(*calls, "\n"), "xdg-mime default "+desktopID+" application/x-modrinth-modpack+zip") {
		t.Errorf("xdg-mime not run: %q", *calls)
	}

	n := len(*calls)
	if err := r.Sync(want); err != nil || len(*calls) != n {
		t.Fatal("an unchanged sync must do nothing")
	}

	// The user's other defaults stay; only ours go.
	mimeapps := filepath.Join(l.configHome, "mimeapps.list")
	if err := os.WriteFile(mimeapps, []byte("[Default Applications]\nx-scheme-handler/modrinth="+desktopID+"\ntext/plain=gedit.desktop;\nx-scheme-handler/curseforge=other.desktop;"+desktopID+";\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	want.Links, want.Autostart = false, false
	if err := r.Sync(want); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read(t, l.entryPath()), "MimeType") {
		t.Error("links off keeps no MimeType")
	}
	if _, err := os.Stat(l.mimePath()); !os.IsNotExist(err) {
		t.Error("mime type kept")
	}
	if _, err := os.Stat(l.autostartPath()); !os.IsNotExist(err) {
		t.Error("autostart kept")
	}
	if got := read(t, mimeapps); got != "[Default Applications]\ntext/plain=gedit.desktop;\nx-scheme-handler/curseforge=other.desktop;\n" {
		t.Errorf("mimeapps.list:\n%s", got)
	}
}

// TestLinuxRealTools registers into scratch XDG folders with the real
// xdg-utils, then asks xdg-mime who handles each type.
func TestLinuxRealTools(t *testing.T) {
	if _, err := exec.LookPath("xdg-mime"); err != nil {
		t.Skip("xdg-mime isn't installed")
	}
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	l := newSystem().(linux)
	if err := l.apply(Want{}, Want{Exe: "/opt/quartz/quartz", Links: true}); err != nil {
		t.Fatal(err)
	}
	for _, typ := range associations() {
		out, err := exec.Command("xdg-mime", "query", "default", typ).Output()
		if err != nil || strings.TrimSpace(string(out)) != desktopID {
			t.Errorf("%s: handled by %q (%v)", typ, out, err)
		}
	}
	if err := l.apply(Want{Exe: "/opt/quartz/quartz", Links: true}, Want{Exe: "/opt/quartz/quartz"}); err != nil {
		t.Fatal(err)
	}
	out, _ := exec.Command("xdg-mime", "query", "default", "x-scheme-handler/modrinth").Output()
	if strings.TrimSpace(string(out)) == desktopID {
		t.Error("still the modrinth handler after links were turned off")
	}
}
