//go:build !windows

package procx

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// NoConsole is a no-op outside Windows.
func NoConsole(*exec.Cmd) {}

// Detach starts the game in its own process group, so a signal aimed at the
// launcher (Ctrl+C in a terminal) does not reach it, and Stop can signal the
// whole group.
func Detach(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// startContained refuses: app containers exist only on Windows, and a
// child asked to be contained must never run uncontained.
func startContained(Spec) (*Child, error) {
	return nil, errors.New("procx: app containers are Windows-only")
}

// Terminate asks the game to quit (SIGTERM to its process group).
func Terminate(p *os.Process) error { return syscall.Kill(-p.Pid, syscall.SIGTERM) }

// Kill ends the game's process group at once.
func Kill(p *os.Process) error { return syscall.Kill(-p.Pid, syscall.SIGKILL) }

// Alive reports whether a process with this id exists (it may belong to
// someone else after the id was reused; callers treat true as "maybe").
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
