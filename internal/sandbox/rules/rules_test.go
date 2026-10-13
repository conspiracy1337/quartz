package rules

import (
	"encoding/json"
	"testing"
)

func TestPresets(t *testing.T) {
	std := Default()
	strict := std.Apply(Strict, "linux")
	cases := []struct {
		s    Settings
		id   ID
		goos string
		want Level
	}{
		{std, Synced, "linux", RW},
		{std, Internet, "linux", Allowed},
		{std, Discord, "linux", Allowed},
		{std, Discord, "windows", Blocked}, // the only choice there
		{std, Home, "linux", Blocked},
		{strict, Synced, "linux", RO},
		{strict, Internet, "linux", Blocked},
		{strict, Discord, "linux", Blocked},
		{strict, Graphics, "linux", Allowed},
		{strict, Instance, "darwin", RW},
	}
	for _, c := range cases {
		if got := c.s.Level(c.id, c.goos); got != c.want {
			t.Errorf("%s %s on %s = %s, want %s", c.s.Preset, c.id, c.goos, got, c.want)
		}
	}
}

func TestSetLevelGoesCustom(t *testing.T) {
	s, ok := Default().SetLevel(Internet, Blocked, "linux")
	if !ok || s.Preset != Custom || s.Level(Internet, "linux") != Blocked || s.Level(Synced, "linux") != RW {
		t.Fatalf("got %+v", s)
	}
	if _, ok := Default().SetLevel(Instance, RO, "linux"); ok {
		t.Fatal("the instance row is fixed")
	}
	if _, ok := Default().SetLevel(Home, RO, "windows"); ok {
		t.Fatal("home is Blocked only on Windows")
	}
	// The same level keeps the preset.
	if s, _ := Default().SetLevel(Synced, RW, "linux"); s.Preset != Standard {
		t.Fatalf("preset %s", s.Preset)
	}
	// Custom keeps what shows; Standard resets.
	c := Default().Apply(Strict, "linux").Apply(Custom, "linux")
	if c.Level(Internet, "linux") != Blocked {
		t.Fatal("custom lost the strict levels")
	}
	if c.Apply(Standard, "linux").Level(Internet, "linux") != Allowed {
		t.Fatal("standard did not reset")
	}
}

func TestDecodeLegacyBool(t *testing.T) {
	for _, c := range []struct {
		in   string
		want Settings
	}{
		{`true`, Settings{On: true, Preset: Standard}},
		{`false`, Settings{Preset: Standard}},
		{`{"on":true,"preset":"strict"}`, Settings{On: true, Preset: Strict}},
	} {
		var s Settings
		if err := json.Unmarshal([]byte(c.in), &s); err != nil || s.On != c.want.On || s.Preset != c.want.Preset {
			t.Errorf("%s: %+v %v", c.in, s, err)
		}
	}
}

func TestNormalize(t *testing.T) {
	s := Settings{Preset: "bogus", Levels: map[ID]Level{Internet: "maybe"}, Folders: []Folder{
		{ID: "a1", Path: "/srv/maps", Level: RO},
		{ID: "a1", Path: "/srv/other", Level: RO}, // duplicate id
		{ID: "b2", Path: "relative", Level: RO},
		{ID: "c3", Path: "/srv/x", Level: Allowed},
		{ID: "BAD", Path: "/srv/y", Level: RW},
	}}.Normalize()
	if s.Preset != Standard || s.Levels != nil || len(s.Folders) != 1 || s.Folders[0].Path != "/srv/maps" {
		t.Fatalf("got %+v", s)
	}
}
