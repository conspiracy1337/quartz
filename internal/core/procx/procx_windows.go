package procx

import (
	"os/exec"
	"syscall"
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
