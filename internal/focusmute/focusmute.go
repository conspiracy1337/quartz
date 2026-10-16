// Package focusmute mutes a game while its window isn't the focused one.
// Minecraft has no such option, so the launcher does it from outside: on
// Linux it follows the focused window over X11 (games run under XWayland or
// X) and mutes the audio streams of every game that doesn't own it, through
// pactl (PipeWire or PulseAudio). Each game's streams carry its run id
// (Env), so even a sandboxed game, whose own process ids are hidden, is found.
package focusmute

import (
	"log/slog"
	"runtime"
	"strconv"
)

// Game is one running game: its run id and the process the launcher
// started (the sandbox, a wrapper or java itself).
type Game struct {
	Run string
	PID int
}

// Config wires a Watcher.
type Config struct {
	Games   func() []Game
	Enabled func() bool // the "mute when unfocused" setting
	Log     *slog.Logger
}

// runProp names a stream's run in its PipeWire and PulseAudio properties.
const runProp = "application.quartz.run"

// Env tags a game's audio streams with its run (Linux; nothing elsewhere).
// OpenAL Soft talks to PipeWire directly and copies PIPEWIRE_PROPS into its
// stream; PULSE_PROP does the same through PulseAudio.
func Env(run string) []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	return []string{"PIPEWIRE_PROPS={ " + runProp + " = " + strconv.Quote(run) + " }", "PULSE_PROP=" + runProp + "=" + run}
}

// Stream is one playing audio stream as pactl lists it.
type Stream struct {
	Index      int               `json:"index"`
	Mute       bool              `json:"mute"`
	Properties map[string]string `json:"properties"`
}

// Change mutes or unmutes one stream.
type Change struct {
	Index int
	Mute  bool
}

// plan lists the streams to change so that every game's streams are muted
// unless it is the focused run; with off, every tagged stream is unmuted.
func plan(streams []Stream, games []Game, focused string, off bool) []Change {
	running := make(map[string]bool, len(games))
	for _, g := range games {
		running[g.Run] = true
	}
	var out []Change
	for _, s := range streams {
		run := s.Properties[runProp]
		if run == "" || (!off && !running[run]) {
			continue
		}
		if want := !off && run != focused; want != s.Mute {
			out = append(out, Change{Index: s.Index, Mute: want})
		}
	}
	return out
}

// gameOf is the run whose process pid is or descends from (parent walks up
// the process tree; it returns 0 at the top).
func gameOf(pid int, games []Game, parent func(int) int) string {
	for depth := 0; pid > 1 && depth < 64; depth++ {
		for _, g := range games {
			if g.PID == pid {
				return g.Run
			}
		}
		pid = parent(pid)
	}
	return ""
}
