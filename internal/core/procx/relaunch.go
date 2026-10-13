package procx

import (
	"os"
	"os/exec"
	"time"
)

// Relaunch starts exe with args as a detached process, for the launcher
// restarting itself (after an update or a data folder change). The new
// process waits for this one to exit (see WaitExit) before it opens a window.
func Relaunch(exe string, args []string) error {
	cmd := exec.Command(exe, args...) //nolint:gosec // our own executable with fixed arguments
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	Detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// WaitExit waits until process pid has exited or timeout passes, and
// reports whether it exited. It checks every 50 ms: it only runs for the
// moment a restarting launcher waits for the old one to close.
func WaitExit(pid int, timeout time.Duration) bool {
	if pid == os.Getpid() {
		return false
	}
	deadline := time.Now().Add(timeout)
	for Alive(pid) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
	return true
}
