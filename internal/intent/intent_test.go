package intent

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"quartz/internal/instance"
)

func TestParse(t *testing.T) {
	wd := filepath.FromSlash("/home/p")
	abs := filepath.Join(wd, "pack.mrpack")
	cases := []struct {
		args []string
		want Args
	}{
		{nil, Args{}},
		{[]string{"--dir", "/d"}, Args{Dir: "/d"}},
		{[]string{"list", "--json"}, Args{Command: List, JSON: true}},
		{[]string{"accounts"}, Args{Command: Accounts}},
		{[]string{"--version"}, Args{Command: Version}},
		{[]string{"-V", "--json"}, Args{Command: Version, JSON: true}},
		{[]string{"-h"}, Args{Command: Help}},
		{[]string{"--launch", "abc"}, Args{Launch: &Launch{Instance: "abc"}}},
		{[]string{"-l", "My Pack", "-s", "play.example.org:25566", "-a", "Steve"},
			Args{Launch: &Launch{Instance: "My Pack", Account: "Steve", Into: instance.Into{Kind: "server", Target: "play.example.org:25566"}}}},
		{[]string{"--launch", "abc", "--world", "New World"}, Args{Launch: &Launch{Instance: "abc", Into: instance.Into{Kind: "world", Target: "New World"}}}},
		{[]string{"modrinth://modpack/abc"}, Args{Open: "modrinth://modpack/abc"}},
		{[]string{"--import", "https://modrinth.com/modpack/x"}, Args{Open: "https://modrinth.com/modpack/x"}},
		{[]string{"pack.mrpack"}, Args{Open: abs}},
		{[]string{"pack.mrpack", "--dir", "/d"}, Args{Open: abs, Dir: "/d"}},
		{[]string{"--wait-pid", "42"}, Args{WaitPID: 42}},
		{[]string{"--minimized"}, Args{Minimized: true}},
	}
	for _, c := range cases {
		got, err := Parse(c.args, wd)
		if err != nil {
			t.Errorf("%q: %v", c.args, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %+v want %+v", c.args, got, c.want)
		}
	}
}

func TestParseRejects(t *testing.T) {
	long := string(make([]byte, 300))
	for _, args := range [][]string{
		{"--bogus"},
		{"list", "extra"},
		{"list", "--launch", "x"},
		{"--json"},
		{"--server", "x"},
		{"--launch", "x", "--server", "a", "--world", "b"},
		{"--launch", "x", "--world", "../evil"},
		{"--launch", "x", "--world", ".hidden"},
		{"--launch", "x", "--server", "bad host:99999"},
		{"--launch", ""},
		{"--launch", "x", "--import", "y"},
		{"a", "b"},
		{"--import", "a", "b"},
		{"--launch", "x" + long},
		{"--wait-pid", "-3"},
		{"--import", "bad\x00name"},
	} {
		if _, err := Parse(args, "/wd"); !errors.Is(err, ErrUsage) {
			t.Errorf("%q: want a usage error, got %v", args, err)
		}
	}
	if _, err := Parse([]string{"pack.mrpack"}, ""); !errors.Is(err, ErrUsage) {
		t.Error("a relative file without a working folder must fail")
	}
}

func TestIsLink(t *testing.T) {
	for v, want := range map[string]bool{
		"modrinth://modpack/x": true, "https://a.b/c": true, "quartz://open/x": true,
		`C:\packs\x.mrpack`: false, "/home/p/x.mrpack": false, "x.mrpack": false, "a://": false, "1x://y": false,
	} {
		if IsLink(v) != want {
			t.Errorf("%q: want %v", v, want)
		}
	}
}

func TestArgvRoundTrips(t *testing.T) {
	for _, l := range []Launch{
		{Instance: "abc"},
		{Instance: "abc", Account: "0123456789abcdef0123456789abcdef", Into: instance.Into{Kind: "server", Target: "mc.example.org"}},
		{Instance: "abc", Into: instance.Into{Kind: "world", Target: "My World"}},
	} {
		got, err := Parse(l.Argv(), "")
		if err != nil || got.Launch == nil || !reflect.DeepEqual(*got.Launch, l) {
			t.Errorf("%+v: got %+v %v", l, got.Launch, err)
		}
	}
}

func TestFind(t *testing.T) {
	type it struct{ id, name string }
	items := []it{{"a1", "Survival"}, {"b2", "Creative"}, {"c3", "creative"}}
	id := func(i it) string { return i.id }
	name := func(i it) string { return i.name }
	if got, err := Find(items, "b2", "instance", id, name); err != nil || got.id != "b2" {
		t.Fatal(got, err)
	}
	if got, err := Find(items, "SURVIVAL", "instance", id, name); err != nil || got.id != "a1" {
		t.Fatal(got, err)
	}
	if _, err := Find(items, "Creative", "instance", id, name); !errors.Is(err, ErrAmbiguous) {
		t.Fatal(err)
	}
	if _, err := Find(items, "x", "instance", id, name); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestForward(t *testing.T) {
	a, _ := Parse([]string{"--dir", "/d", "--wait-pid", "9", "pack.mrpack"}, "/wd")
	got, err := Parse(a.Forward(), "")
	if err != nil || got.Open != a.Open || got.Dir != "" || got.WaitPID != 0 {
		t.Fatalf("%+v %v", got, err)
	}
	if (Args{}).Forward() != nil {
		t.Fatal("nothing to forward")
	}
}
