package clipimg

import (
	"os"
	"os/exec"
)

func copyPNG(path string) error {
	f, err := os.Open(path) //nolint:gosec // a screenshot the caller checked
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var cmd *exec.Cmd
	if wl, err := exec.LookPath("wl-copy"); err == nil && os.Getenv("WAYLAND_DISPLAY") != "" {
		cmd = exec.Command(wl, "--type", "image/png") //nolint:gosec // fixed program and arguments
	} else if xc, err := exec.LookPath("xclip"); err == nil {
		cmd = exec.Command(xc, "-selection", "clipboard", "-t", "image/png", "-i") //nolint:gosec // fixed program and arguments
	} else {
		return ErrNoTool
	}
	cmd.Stdin = f
	// Both tools stay running to serve the clipboard; start and let them be.
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
