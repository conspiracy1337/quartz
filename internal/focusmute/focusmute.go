// Package focusmute mutes a game while its window isn't the focused one.
// Minecraft has no such option, so the launcher does it from outside: it
// follows the focused window and mutes the audio of every game that doesn't
// own it. Each system has its own backend: on Linux X11 (games run under
// XWayland or X) and pactl (PipeWire or PulseAudio), on Windows a WinEvent
// hook and WASAPI's per-session volume, on macOS NSWorkspace and CoreAudio
// process taps. The Watcher here is the same everywhere.
package focusmute

import (
	"context"
	"log/slog"
	"runtime"
	"strconv"
	"sync"
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
// stream; PULSE_PROP does the same through PulseAudio. A sandboxed game's
// own process ids are hidden there, so the tag is what finds its streams.
func Env(run string) []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	return []string{"PIPEWIRE_PROPS={ " + runProp + " = " + strconv.Quote(run) + " }", "PULSE_PROP=" + runProp + "=" + run}
}

// Stream is one program's audio as a backend sees it: a PulseAudio sink
// input, a Windows audio session, a macOS audio process. Its game is Run
// when the stream names it (Linux), else the game PID belongs to.
type Stream struct {
	ID   uint64
	Run  string
	PID  int
	Mute bool
}

// Change mutes or unmutes one stream.
type Change struct {
	ID   uint64
	Mute bool
}

// backend is one system's way to follow the focus and mute a stream.
type backend interface {
	// watch follows the focus and new audio streams until ctx ends, calling
	// focus with the newly focused window's process (0 for none) and
	// changed when a stream appears, from its own goroutines (they may
	// block). It returns the process focused now.
	watch(ctx context.Context, focus func(pid int), changed func()) (int, error)
	streams() ([]Stream, error)
	setMute(id uint64, mute bool) error
	// parents returns a process's parent (0 at the top or when gone), read
	// as of this call where the system makes per-lookup reads costly.
	parents() func(pid int) int
}

// Available reports whether games can be muted on focus loss here.
func Available() bool { return available() }

// Watcher follows the focus while the setting is on and a game runs.
type Watcher struct {
	c Config

	mu      sync.Mutex
	b       backend            // the open backend while watching
	cancel  context.CancelFunc // ends the running watch, nil while idle
	focused string             // the run whose window has focus ("" for none)
}

// New returns a Watcher; call Update when the setting or the games change.
func New(c Config) *Watcher {
	if c.Log == nil {
		c.Log = slog.New(slog.DiscardHandler)
	}
	return &Watcher{c: c}
}

// Update starts or stops watching to match the setting and the running
// games, and applies the mute state for games that started or exited.
func (w *Watcher) Update() {
	w.mu.Lock()
	defer w.mu.Unlock()
	on := w.c.Enabled() && len(w.c.Games()) > 0
	switch {
	case on && w.cancel == nil:
		w.start()
	case !on && w.cancel != nil:
		w.apply(true) // the setting went off: unmute what we muted
		w.cancel()
		w.b, w.cancel = nil, nil
	case on:
		w.apply(false)
	}
}

// start opens the backend and its watches. w.mu is held.
func (w *Watcher) start() {
	b, err := newBackend()
	if err != nil {
		w.c.Log.Warn("can't mute games on focus loss", "err", err)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	// Events arrive from the backend's goroutines; one that waited for the
	// lock while the watch stopped is dropped (stopping unmutes).
	locked := func(fn func()) {
		w.mu.Lock()
		defer w.mu.Unlock()
		if ctx.Err() == nil {
			fn()
			w.apply(false)
		}
	}
	pid, err := b.watch(ctx, func(pid int) { locked(func() { w.focus(pid) }) }, func() { locked(func() {}) })
	if err != nil {
		cancel()
		w.c.Log.Warn("can't follow the focused window", "err", err)
		return
	}
	w.b, w.cancel = b, cancel
	w.focus(pid)
	w.apply(false)
}

// focus records which game owns the focused process. w.mu is held.
func (w *Watcher) focus(pid int) {
	w.focused = ""
	if pid > 0 {
		w.focused = gameOf(pid, w.c.Games(), w.b.parents())
	}
}

// apply mutes every game's streams but the focused one's (off unmutes all
// of them). w.mu is held.
func (w *Watcher) apply(off bool) {
	if w.b == nil {
		return
	}
	list, err := w.b.streams()
	if err != nil {
		w.c.Log.Warn("listing audio streams failed", "err", err)
		return
	}
	for _, c := range plan(list, w.c.Games(), w.focused, off, w.b.parents) {
		if err := w.b.setMute(c.ID, c.Mute); err != nil {
			w.c.Log.Warn("muting a game failed", "stream", c.ID, "err", err)
		}
	}
}

// plan lists the streams to change so that every game's streams are muted
// unless it is the focused run; with off, every stream of ours is unmuted.
// A stream without a run tag belongs to the game its process descends from
// (parents is called once, and only then).
func plan(streams []Stream, games []Game, focused string, off bool, parents func() func(int) int) []Change {
	running := make(map[string]bool, len(games))
	for _, g := range games {
		running[g.Run] = true
	}
	var parent func(int) int
	var out []Change
	for _, s := range streams {
		run := s.Run
		if run == "" && s.PID > 0 {
			if parent == nil {
				parent = parents()
			}
			run = gameOf(s.PID, games, parent)
		}
		if run == "" || (!off && !running[run]) {
			continue
		}
		if want := !off && run != focused; want != s.Mute {
			out = append(out, Change{ID: s.ID, Mute: want})
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
