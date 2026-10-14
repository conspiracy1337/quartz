//go:build !linux && !darwin && !windows

package procx

// Ident can't read a process's start here; Same then trusts the pid.
func Ident(int) (uint64, bool) { return 0, false }
