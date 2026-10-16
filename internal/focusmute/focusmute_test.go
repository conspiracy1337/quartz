package focusmute

import (
	"runtime"
	"slices"
	"testing"
)

func TestPlan(t *testing.T) {
	games := []Game{{Run: "a", PID: 10}, {Run: "b", PID: 20}}
	noTree := func() func(int) int { t.Fatal("tagged streams need no process tree"); return nil }
	streams := []Stream{{ID: 1, Run: "a"}, {ID: 2, Run: "b"}, {ID: 3, Run: "b", Mute: true}, {ID: 4, Run: "gone"}, {ID: 5}}
	// a has focus: b's streams get muted, the rest are left alone.
	if got := plan(streams, games, "a", false, noTree); !slices.Equal(got, []Change{{ID: 2, Mute: true}}) {
		t.Fatalf("a focused: %+v", got)
	}
	// Nothing of ours has focus: every running game is muted.
	if got := plan(streams, games, "", false, noTree); !slices.Equal(got, []Change{{ID: 1, Mute: true}, {ID: 2, Mute: true}}) {
		t.Fatalf("none focused: %+v", got)
	}
	// Turned off: everything tagged comes back.
	if got := plan(streams, games, "", true, noTree); !slices.Equal(got, []Change{{ID: 3, Mute: false}}) {
		t.Fatalf("off: %+v", got)
	}
}

// Windows and macOS streams carry the playing process, found up the tree.
func TestPlanByProcess(t *testing.T) {
	games := []Game{{Run: "a", PID: 10}, {Run: "b", PID: 20}}
	reads := 0
	parents := func() func(int) int {
		reads++
		tree := map[int]int{11: 10, 21: 20, 30: 1}
		return func(pid int) int { return tree[pid] }
	}
	streams := []Stream{{ID: 1, PID: 11}, {ID: 2, PID: 21}, {ID: 3, PID: 30}, {ID: 4, PID: 20, Mute: true}}
	if got := plan(streams, games, "b", false, parents); !slices.Equal(got, []Change{{ID: 1, Mute: true}, {ID: 4, Mute: false}}) {
		t.Fatalf("b focused: %+v", got)
	}
	if reads != 1 {
		t.Fatalf("read the process tree %d times", reads)
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
