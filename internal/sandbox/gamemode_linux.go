//go:build linux

package sandbox

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// gameMode registers a sandboxed game with GameMode from the launcher: the
// sandbox has no session bus, so gamemoderun can't do it from inside (Prism
// registers by pid too). bubblewrap writes the sandbox's first process to
// the returned file (--info-fd); the game is that process's child.
func gameMode(j *Jail) (*os.File, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	var game int
	j.started = func(int) {
		_ = w.Close() // the child holds its own copy
		go func() {
			defer close(done)
			defer func() { _ = r.Close() }()
			var info struct {
				ChildPid int `json:"child-pid"`
			}
			if json.NewDecoder(io.LimitReader(r, 4096)).Decode(&info) != nil || info.ChildPid <= 0 {
				return
			}
			if game = firstChild(info.ChildPid); game > 0 {
				callGameMode("RegisterGameByPID", game)
			}
		}()
	}
	j.closers = append(j.closers, func() {
		_ = w.Close()
		select {
		case <-done:
			if game > 0 {
				callGameMode("UnregisterGameByPID", game)
			}
		case <-time.After(time.Second):
		}
	})
	return w, nil
}

// firstChild waits briefly for pid's first child (the game under the
// sandbox's init) and returns it, or 0.
func firstChild(pid int) int {
	p := strconv.Itoa(pid)
	for range 50 {
		b, err := os.ReadFile(filepath.Join("/proc", p, "task", p, "children")) //nolint:gosec // a /proc path built from a pid
		if err != nil {
			return 0
		}
		if f := strings.Fields(string(b)); len(f) > 0 {
			n, _ := strconv.Atoi(f[0])
			return n
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0
}

// callGameMode asks gamemoded to (un)register game; a missing daemon is fine.
func callGameMode(method string, game int) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return
	}
	obj := conn.Object("com.feralinteractive.GameMode", "/com/feralinteractive/GameMode")
	var res int32
	_ = obj.Call("com.feralinteractive.GameMode."+method, 0, int32(os.Getpid()), int32(game)).Store(&res) //nolint:gosec // pids fit
}
