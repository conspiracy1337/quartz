//go:build windows

package linkx

import "golang.org/x/sys/windows"

// SetReadOnly sets or clears FILE_ATTRIBUTE_READONLY. Every hardlink of the
// file shares it.
func SetReadOnly(path string, ro bool) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return err
	}
	next := attrs &^ windows.FILE_ATTRIBUTE_READONLY
	if ro {
		next |= windows.FILE_ATTRIBUTE_READONLY
	}
	if next == attrs {
		return nil
	}
	return windows.SetFileAttributes(p, next)
}
