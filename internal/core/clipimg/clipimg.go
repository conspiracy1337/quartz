// Package clipimg puts a picture on the system clipboard (Wails' clipboard
// is text only). Each OS uses what it ships with, without cgo: user32 on
// Windows, osascript on macOS, and wl-copy or xclip on Linux. Paths are
// passed as arguments or file input, never through a shell.
package clipimg

import "errors"

// ErrNoTool is returned on Linux when neither wl-copy nor xclip is installed.
var ErrNoTool = errors.New("copying pictures needs wl-clipboard (Wayland) or xclip (X11) installed")

// CopyPNG puts the PNG file at path on the clipboard.
func CopyPNG(path string) error { return copyPNG(path) }
