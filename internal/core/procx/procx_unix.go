//go:build !windows

package procx

import "os/exec"

// NoConsole is a no-op outside Windows.
func NoConsole(*exec.Cmd) {}
