//go:build !windows

package linkx

import "os"

// SetReadOnly drops or restores the owner's write bit. Every hardlink of the
// file shares it.
func SetReadOnly(path string, ro bool) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()
	if ro {
		mode &^= 0o222
	} else {
		mode |= 0o200
	}
	if mode == info.Mode().Perm() {
		return nil
	}
	return os.Chmod(path, mode)
}
