//go:build windows

package linkx

import (
	"fmt"
	"io/fs"

	"golang.org/x/sys/windows"
)

// identity names the file at path when it has other hardlinks, so a copy
// can link them again ("" and false for a file with one name).
func identity(path string, _ fs.FileInfo) (string, bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", false
	}
	h, err := windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return "", false
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var d windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &d) != nil || d.NumberOfLinks < 2 {
		return "", false
	}
	return fmt.Sprint(d.VolumeSerialNumber, ":", d.FileIndexHigh, ":", d.FileIndexLow), true
}
