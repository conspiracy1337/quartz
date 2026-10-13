package procx

import (
	"os/exec"
	"runtime"
)

// OpenFolder shows a folder in the system file manager. The path is passed
// as a single argument, never through a shell.
func OpenFolder(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path) //nolint:gosec // fixed program, path as one argument
	case "darwin":
		cmd = exec.Command("open", path) //nolint:gosec // fixed program, path as one argument
	default:
		cmd = exec.Command("xdg-open", path) //nolint:gosec // fixed program, path as one argument
	}
	return start(cmd)
}
