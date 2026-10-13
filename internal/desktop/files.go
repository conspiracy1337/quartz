//go:build !windows

package desktop

import (
	"errors"
	"os"
	"path/filepath"

	"quartz/internal/core/fsx"
)

// writeFile writes a user-visible system file (an entry, an icon, a mime
// type): readable by the desktop, created with its folders.
func writeFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return fsx.AtomicWrite(path, data, perm)
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
