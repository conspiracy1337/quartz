package procx

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

const createNoWindow = 0x08000000

// NoConsole keeps a console program (java.exe) from opening a window.
func NoConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}

const createNewProcessGroup = 0x00000200

// Detach starts the game in its own process group.
func Detach(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNewProcessGroup
}

var procPostMessageW = windows.NewLazySystemDLL("user32.dll").NewProc("PostMessageW")

const wmClose = 0x0010

// Terminate asks the game to quit by closing its windows (WM_CLOSE), as
// Pandora does; without a window it falls back to ending the process.
func Terminate(p *os.Process) error {
	pid := uint32(p.Pid) //nolint:gosec // Windows process ids are 32-bit
	closed := false
	cb := windows.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		var owner uint32
		if _, err := windows.GetWindowThreadProcessId(hwnd, &owner); err == nil && owner == pid && windows.IsWindowVisible(hwnd) {
			_, _, _ = procPostMessageW.Call(uintptr(hwnd), wmClose, 0, 0)
			closed = true
		}
		return 1
	})
	_ = windows.EnumWindows(cb, nil)
	if !closed {
		return p.Kill()
	}
	return nil
}

// Kill ends the game at once.
func Kill(p *os.Process) error { return p.Kill() }
