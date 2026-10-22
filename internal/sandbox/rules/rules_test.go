package rules

import (
	"runtime"
	"testing"
)

func TestPresets(t *testing.T) {
	std := Default()
	strict := std.Apply(Strict)
	cases := []struct {
		s    Settings
		id   ID
		goos string
		want Level
	}{
		{std, Synced, "linux", RW},
		{std, Internet, "linux", Allowed},
		{std, Discord, "linux", Allowed},
		{std, Discord, "windows", Blocked}, // fixed there
		{std, Home, "linux", Blocked},
		{strict, Synced, "linux", RO},
		{strict, Internet, "linux", LAN},
		{strict, Discord, "linux", Blocked},
		{strict, Graphics, "linux", Allowed},
		{strict, Instance, "darwin", RW},
		{strict, Mods, "windows", RO},
	}
	for _, c := range cases {
		if got := c.s.Level(c.id, c.goos); got != c.want {
			t.Errorf("%s %s on %s = %s, want %s", c.s.Preset, c.id, c.goos, got, c.want)
		}
	}
	if Default().Apply(Strict).Apply(Standard).Level(Internet, "linux") != Allowed {
		t.Fatal("standard did not reset")
	}
}

func TestNormalize(t *testing.T) {
	// Absolute paths differ by OS (filepath.IsAbs wants a drive on Windows).
	abs := "/srv/"
	if runtime.GOOS == "windows" {
		abs = `C:\srv\`
	}
	s := Settings{Preset: "custom", Folders: []Folder{
		{ID: "a1", Path: abs + "maps", Level: RO},
		{ID: "a1", Path: abs + "other", Level: RO}, // duplicate id
		{ID: "b2", Path: "relative", Level: RO},
		{ID: "c3", Path: abs + "x", Level: Allowed},
		{ID: "BAD", Path: abs + "y", Level: RW},
	}}.Normalize()
	if s.Preset != Standard || len(s.Folders) != 1 || s.Folders[0].Path != abs+"maps" {
		t.Fatalf("got %+v", s)
	}
}
