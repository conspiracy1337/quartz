//go:build !linux

package landlock

// Supported is false: Landlock is Linux-only.
func Supported() bool { return false }

// Main does nothing off Linux.
func Main() {}
