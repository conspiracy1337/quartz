//go:build !windows

package app

// windowFrame is nothing here: Linux and macOS size the window by its page.
func windowFrame() (w, h int) { return 0, 0 }
