package clipimg

import "os/exec"

func copyPNG(path string) error {
	// The path goes in as argv, so no quoting can break out of the script.
	return exec.Command("osascript", //nolint:gosec // fixed script; the path is an argument
		"-e", "on run argv",
		"-e", "set the clipboard to (read (POSIX file (item 1 of argv)) as «class PNGf»)",
		"-e", "end run", path).Run()
}
