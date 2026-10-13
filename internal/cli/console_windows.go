package cli

import (
	"os"

	"golang.org/x/sys/windows"
)

// AttachConsole connects a GUI-subsystem exe to the console it was started
// from, so headless commands print there. Without one it does nothing.
func AttachConsole() {
	k := windows.NewLazySystemDLL("kernel32.dll")
	const attachParentProcess = ^uintptr(0) // ATTACH_PARENT_PROCESS = (DWORD)-1
	if r, _, _ := k.NewProc("AttachConsole").Call(attachParentProcess); r == 0 {
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = f, f
	}
}
