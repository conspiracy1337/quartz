package focusmute

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// pactlTimeout bounds one pactl call.
const pactlTimeout = 5 * time.Second

// available: a local X display (XWayland counts) to follow the focus and
// pactl to mute with.
func available() bool {
	_, _, ok := displaySocket(os.Getenv("DISPLAY"))
	_, err := exec.LookPath("pactl")
	return ok && err == nil
}

// pulse follows the focus over X11 and mutes sink inputs through pactl.
type pulse struct{ pactl string }

func newBackend() (backend, error) {
	p, err := exec.LookPath("pactl")
	if err != nil {
		return nil, err
	}
	return &pulse{pactl: p}, nil
}

func (p *pulse) watch(ctx context.Context, focus func(int), changed func()) (int, error) {
	x, err := dialX()
	if err == nil {
		err = x.watch()
	}
	if err != nil {
		if x != nil {
			_ = x.Close()
		}
		return 0, err
	}
	go func() {
		<-ctx.Done()
		_ = x.Close()
	}()
	// The connection is this goroutine's alone from here on.
	pid := x.focusedPID()
	go func() {
		for x.focusChanged() == nil {
			focus(x.focusedPID())
		}
	}()
	go p.subscribe(ctx, changed)
	return pid, nil
}

// subscribe calls changed whenever an audio stream appears (a game opens
// its stream after its window).
func (p *pulse) subscribe(ctx context.Context, changed func()) {
	cmd := exec.CommandContext(ctx, p.pactl, "subscribe") //nolint:gosec // pactl from PATH, fixed arguments
	out, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		return
	}
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		if strings.Contains(sc.Text(), "'new' on sink-input") {
			changed()
		}
	}
	_ = cmd.Wait()
}

// sinkInput is one playing stream as pactl lists it.
type sinkInput struct {
	Index      uint64            `json:"index"`
	Mute       bool              `json:"mute"`
	Properties map[string]string `json:"properties"`
}

func (p *pulse) streams() ([]Stream, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pactlTimeout)
	defer cancel()
	raw, err := exec.CommandContext(ctx, p.pactl, "-f", "json", "list", "sink-inputs").Output() //nolint:gosec // fixed arguments
	if err != nil {
		return nil, err
	}
	var list []sinkInput
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	out := make([]Stream, len(list))
	for i, s := range list {
		// Only the tag counts: the process ids a sandboxed game reports are its own.
		out[i] = Stream{ID: s.Index, Run: s.Properties[runProp], Mute: s.Mute}
	}
	return out, nil
}

func (p *pulse) setMute(id uint64, mute bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), pactlTimeout)
	defer cancel()
	v := "0"
	if mute {
		v = "1"
	}
	return exec.CommandContext(ctx, p.pactl, "set-sink-input-mute", strconv.FormatUint(id, 10), v).Run() //nolint:gosec // a number and 0/1
}

func (*pulse) parents() func(int) int { return parentPID }

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
