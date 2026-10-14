//go:build !windows

package linkx

import (
	"fmt"
	"io/fs"
	"syscall"
)

// identity names the file behind info when it has other hardlinks, so a
// copy can link them again ("" and false for a file with one name).
func identity(_ string, info fs.FileInfo) (string, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Nlink < 2 {
		return "", false
	}
	return fmt.Sprint(st.Dev, ":", st.Ino), true
}
