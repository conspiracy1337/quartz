package procx

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Ident is a running process's start time, in clock ticks after boot: it
// tells a process apart from a later one given the same pid.
func Ident(pid int) (uint64, bool) {
	if pid <= 0 {
		return 0, false
	}
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat")) //nolint:gosec // a /proc path
	if err != nil {
		return 0, false
	}
	// pid (comm) state ppid …: comm may hold spaces and parentheses, so the
	// fields are counted from the last ')'; starttime is field 22.
	i := strings.LastIndexByte(string(data), ')')
	if i < 0 {
		return 0, false
	}
	f := strings.Fields(string(data[i+1:]))
	if len(f) < 20 {
		return 0, false
	}
	v, err := strconv.ParseUint(f[19], 10, 64)
	return v, err == nil && v > 0
}
