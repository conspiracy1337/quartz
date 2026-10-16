package paths

import (
	"errors"
	"path/filepath"
	"testing"

	"quartz/internal/brand"
)

func testEnv(goos string, vars map[string]string, files ...string) Env {
	set := map[string]bool{}
	for _, f := range files {
		set[f] = true
	}
	return Env{
		GOOS:       goos,
		Getenv:     func(k string) string { return vars[k] },
		HomeDir:    "/home/u",
		ExePath:    "/opt/quartz/quartz",
		FileExists: func(p string) bool { return set[p] },
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		env      Env
		want     string
		portable bool
	}{
		{"flag wins", "/data/q", testEnv("linux", map[string]string{brand.EnvDir: "/env"}), "/data/q", false},
		{"env var", "", testEnv("linux", map[string]string{brand.EnvDir: "/env/q"}), "/env/q", false},
		{"portable marker", "", testEnv("linux", nil, "/opt/quartz/"+brand.PortableMarker), "/opt/quartz/data", true},
		{"linux xdg", "", testEnv("linux", map[string]string{"XDG_DATA_HOME": "/xdg"}), "/xdg/" + brand.DirName, false},
		{"linux relative xdg ignored", "", testEnv("linux", map[string]string{"XDG_DATA_HOME": "rel"}), "/home/u/.local/share/" + brand.DirName, false},
		{"linux default", "", testEnv("linux", nil), "/home/u/.local/share/" + brand.DirName, false},
		{"darwin", "", testEnv("darwin", nil), "/home/u/Library/Application Support/" + brand.DirName, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.flag, tt.env)
			if err != nil {
				t.Fatal(err)
			}
			if got.Root != filepath.Clean(tt.want) || got.Portable != tt.portable {
				t.Fatalf("got %+v, want %s portable=%v", got, tt.want, tt.portable)
			}
		})
	}
}

// The pointer to a chosen folder lives in the config directory, never in
// the data directory a moved folder leaves.
func TestResolvePointer(t *testing.T) {
	for goos, want := range map[string]string{
		"linux":   "/home/u/.config/" + brand.DirName + "/" + pointerFile,
		"darwin":  "/home/u/Library/Preferences/" + brand.DirName + "/" + pointerFile,
		"windows": filepath.Join(`C:\Roaming`, brand.DirName, pointerFile),
	} {
		env := testEnv(goos, map[string]string{"APPDATA": `C:\Roaming`, "LOCALAPPDATA": `C:\Local`})
		if got, err := Resolve("", env); err != nil || got.pointer != filepath.FromSlash(want) {
			t.Errorf("%s: %q %v, want %q", goos, got.pointer, err, want)
		}
	}
	xdg := testEnv("linux", map[string]string{"XDG_CONFIG_HOME": "/cfg"})
	if got, _ := Resolve("", xdg); got.pointer != "/cfg/"+brand.DirName+"/"+pointerFile {
		t.Errorf("xdg: %q", got.pointer)
	}
}

func TestResolveNoDir(t *testing.T) {
	env := testEnv("windows", nil)
	if _, err := Resolve("", env); !errors.Is(err, ErrNoDataDir) {
		t.Fatalf("want ErrNoDataDir, got %v", err)
	}
}

func TestEnsure(t *testing.T) {
	p := Paths{Root: filepath.Join(t.TempDir(), "root")}
	if err := p.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{p.Logs(), p.Instances(), p.Store(), p.Cache(), p.Themes()} {
		if !(SystemEnv("linux").FileExists(d)) {
			t.Fatalf("%s not created", d)
		}
	}
}
