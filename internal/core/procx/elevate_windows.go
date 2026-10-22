package procx

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	cbSize        uint32
	fMask         uint32
	hwnd          windows.Handle
	verb          *uint16
	file          *uint16
	parameters    *uint16
	directory     *uint16
	show          int32
	instApp       windows.Handle
	idList        uintptr
	class         *uint16
	keyClass      windows.Handle
	hotKey        uint32
	iconOrMonitor windows.Handle
	process       windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	seeMaskFlagNoUI       = 0x00000400
)

var procShellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// RunElevated runs exe with args as administrator, which Windows confirms
// with its UAC prompt, waits for it and returns its exit code. ErrDeclined
// means the prompt was dismissed (or the account can't approve it).
//
//nolint:gosec // a Win32 call on Go-owned memory
func RunElevated(exe string, args []string) (int, error) {
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return 0, err
	}
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, err
	}
	params, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(args))
	if err != nil {
		return 0, err
	}
	info := shellExecuteInfo{fMask: seeMaskNoCloseProcess | seeMaskNoAsync | seeMaskFlagNoUI, verb: verb, file: file, parameters: params, show: windows.SW_HIDE}
	info.cbSize = uint32(unsafe.Sizeof(info))
	if r, _, e := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		if errors.Is(e, windows.ERROR_CANCELLED) {
			return 0, ErrDeclined
		}
		return 0, fmt.Errorf("procx: running as administrator: %w", e)
	}
	if info.process == 0 {
		return 0, errors.New("procx: running as administrator gave no process")
	}
	defer func() { _ = windows.CloseHandle(info.process) }()
	if _, err := windows.WaitForSingleObject(info.process, windows.INFINITE); err != nil {
		return 0, err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.process, &code); err != nil {
		return 0, err
	}
	return int(code), nil
}
