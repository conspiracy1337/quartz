package focusmute

import (
	"runtime"
	"slices"
	"testing"
)

func TestPlan(t *testing.T) {
	games := []Game{{Run: "a", PID: 10}, {Run: "b", PID: 20}}
	tagged := func(i int, run string, mute bool) Stream {
		return Stream{Index: i, Mute: mute, Properties: map[string]string{runProp: run}}
	}
	streams := []Stream{tagged(1, "a", false), tagged(2, "b", false), tagged(3, "b", true), tagged(4, "gone", false), {Index: 5, Properties: map[string]string{}}}
	// a has focus: b's streams get muted, the rest are left alone.
	if got := plan(streams, games, "a", false); !slices.Equal(got, []Change{{Index: 2, Mute: true}}) {
		t.Fatalf("a focused: %+v", got)
	}
	// Nothing of ours has focus: every running game is muted.
	if got := plan(streams, games, "", false); !slices.Equal(got, []Change{{Index: 1, Mute: true}, {Index: 2, Mute: true}}) {
		t.Fatalf("none focused: %+v", got)
	}
	// Turned off: everything tagged comes back.
	if got := plan(streams, games, "", true); !slices.Equal(got, []Change{{Index: 3, Mute: false}}) {
		t.Fatalf("off: %+v", got)
	}
}

func TestGameOf(t *testing.T) {
	tree := map[int]int{300: 200, 200: 20, 20: 1, 50: 1}
	parent := func(pid int) int { return tree[pid] }
	games := []Game{{Run: "a", PID: 10}, {Run: "b", PID: 20}}
	if got := gameOf(300, games, parent); got != "b" {
		t.Fatalf("a game's child: %q", got)
	}
	if got := gameOf(50, games, parent); got != "" {
		t.Fatalf("another program: %q", got)
	}
}

func TestEnvTagsStreams(t *testing.T) {
	env := Env("20261016-140211.000")
	if runtime.GOOS != "linux" {
		if env != nil {
			t.Fatal("Linux only")
		}
		return
	}
	want := []string{`PIPEWIRE_PROPS={ application.quartz.run = "20261016-140211.000" }`, "PULSE_PROP=application.quartz.run=20261016-140211.000"}
	if !slices.Equal(env, want) {
		t.Fatalf("%q", env)
	}
}
