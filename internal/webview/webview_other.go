//go:build !linux || !cgo

package webview

// Isolate does nothing here (see the package comment).
func Isolate(string, string) error { return nil }
