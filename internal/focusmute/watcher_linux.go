package focusmute

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// pactlTimeout bounds one pactl call.
const pactlTimeout = 5 * time.Second

// Available reports whether games can be muted on focus loss here: a local
// X display (XWayland counts) to follow the focus and pactl to mute with.
func Available() bool {
	_, _, ok := displaySocket(os.Getenv("DISPLAY"))
	_, err := exec.LookPath("pactl")
	return ok && err == nil
}

// Watcher follows the focus while the setting is on and a game runs.
type Watcher struct {
	c Config

	mu      sync.Mutex
	cancel  context.CancelFunc // the running watch, nil while idle
	pactl   string
	focused string // the run whose window has focus ("" for none)
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
		w.cancel()
		w.cancel = nil
		w.apply(true) // the setting went off: unmute what we muted
	case on:
		w.apply(false)
	}
}

// start connects to X and runs the two watches: focus changes and new
// audio streams (a game opens its stream after its window). w.mu is held.
func (w *Watcher) start() {
	p, err := exec.LookPath("pactl")
	if err != nil {
		return
	}
	x, err := dialX()
	if err == nil {
		err = x.watch()
	}
	if err != nil {
		w.c.Log.Warn("can't follow the focused window", "err", err)
		if x != nil {
			_ = x.Close()
		}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel, w.pactl = cancel, p
	go func() {
		<-ctx.Done()
		_ = x.Close()
	}()
	w.focus(x)
	w.apply(false)
	go func() {
		for x.focusChanged() == nil {
			w.mu.Lock()
			if ctx.Err() == nil { // not stopped meanwhile (stopping unmutes)
				w.focus(x)
				w.apply(false)
			}
			w.mu.Unlock()
		}
	}()
	go w.streams(ctx)
}

// focus records which game owns the focused window. w.mu is held.
func (w *Watcher) focus(x *xconn) {
	w.focused = ""
	if pid := x.focusedPID(); pid > 0 {
		w.focused = gameOf(pid, w.c.Games(), parentPID)
	}
}

// streams re-applies the mute state whenever an audio stream appears.
func (w *Watcher) streams(ctx context.Context) {
	cmd := exec.CommandContext(ctx, w.pactl, "subscribe") //nolint:gosec // pactl from PATH, fixed arguments
	out, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		return
	}
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		if strings.Contains(sc.Text(), "'new' on sink-input") {
			w.mu.Lock()
			if ctx.Err() == nil {
				w.apply(false)
			}
			w.mu.Unlock()
		}
	}
	_ = cmd.Wait()
}

// apply mutes every game's streams but the focused one's (off unmutes all
// of them). w.mu is held.
func (w *Watcher) apply(off bool) {
	if w.pactl == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), pactlTimeout)
	defer cancel()
	raw, err := exec.CommandContext(ctx, w.pactl, "-f", "json", "list", "sink-inputs").Output() //nolint:gosec // fixed arguments
	if err != nil {
		return
	}
	var list []Stream
	if json.Unmarshal(raw, &list) != nil {
		return
	}
	for _, c := range plan(list, w.c.Games(), w.focused, off) {
		mute := "0"
		if c.Mute {
			mute = "1"
		}
		if err := exec.CommandContext(ctx, w.pactl, "set-sink-input-mute", strconv.Itoa(c.Index), mute).Run(); err != nil { //nolint:gosec // a number and 0/1
			w.c.Log.Warn("muting a game failed", "stream", c.Index, "err", err)
		}
	}
}

// parentPID is a process's parent from /proc (0 when it is gone).
func parentPID(pid int) int {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	// "pid (comm) state ppid …"; comm may hold spaces and parentheses.
	if i := bytes.LastIndexByte(b, ')'); i >= 0 {
		if f := strings.Fields(string(b[i+1:])); len(f) > 1 {
			ppid, _ := strconv.Atoi(f[1])
			return ppid
		}
	}
	return 0
}
