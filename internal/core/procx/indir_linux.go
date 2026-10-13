package procx

import (
	"os"
	"path/filepath"
	"strconv"
)

// InDir reports whether process pid runs with dir as its working folder, as
// a check that a recorded pid still is the process the launcher started (a
// reused pid belongs to something else and must never be stopped).
func InDir(pid int, dir string) bool {
	if pid <= 0 {
		return false
	}
	cwd, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "cwd"))
	return err == nil && filepath.Clean(cwd) == filepath.Clean(dir)
}
