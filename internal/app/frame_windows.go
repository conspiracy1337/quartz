package app

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// wsOverlappedWindow is WS_OVERLAPPEDWINDOW, the style Wails gives the window.
const wsOverlappedWindow = 0x00CF0000

var procAdjustWindowRectExForDpi = windows.NewLazySystemDLL("user32.dll").NewProc("AdjustWindowRectExForDpi")

// windowFrame is how much wider and taller the window is than the page it
// shows. Wails sizes a Windows window (and its minimum) by the outer frame,
// resize borders and title bar included, in 96-DPI pixels it scales itself;
// the layout is designed for the page area, as Linux and macOS size it.
//
//nolint:gosec // a Win32 call on Go-owned memory
func windowFrame() (w, h int) {
	var r windows.Rect
	if ok, _, _ := procAdjustWindowRectExForDpi.Call(uintptr(unsafe.Pointer(&r)), wsOverlappedWindow, 0, 0, 96); ok == 0 {
		return 16, 39 // Windows 10 and 11 at 96 DPI
	}
	return int(r.Right - r.Left), int(r.Bottom - r.Top)
}
