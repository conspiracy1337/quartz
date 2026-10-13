//go:build !linux

package procx

// InDir can't tell a process's working folder here, so it never confirms one.
func InDir(int, string) bool { return false }
