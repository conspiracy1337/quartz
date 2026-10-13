package procx

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// Reveal shows a file selected in the system file manager, falling back to
// opening its folder. The path is passed as a single argument, never through a shell.
func Reveal(path string) error {
	switch runtime.GOOS {
	case "windows":
		// explorer takes "/select," and the path as one argument.
		return start(exec.Command("explorer", "/select,"+path)) //nolint:gosec // fixed program
	case "darwin":
		return start(exec.Command("open", "-R", path)) //nolint:gosec // fixed program, path as one argument
	}
	if showItems(path) == nil {
		return nil
	}
	return OpenFolder(filepath.Dir(path))
}

func start(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
