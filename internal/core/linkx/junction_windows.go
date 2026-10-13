//go:build windows

package linkx

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

// Directory junctions need no privileges (unlike symlinks), and Minecraft
// only refuses links inside a world, not a junction to the saves folder.

const (
	ioReparseTagMountPoint = 0xA0000003
	fsctlSetReparsePoint   = 0x000900A4
)

// Junction makes link (which must not exist) a directory junction to target.
func Junction(link, target string) error {
	target, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if err := os.Mkdir(link, 0o700); err != nil {
		return err
	}
	if err := setMountPoint(link, target); err != nil {
		_ = os.Remove(link)
		return err
	}
	return nil
}

func setMountPoint(link, target string) error {
	p, err := windows.UTF16PtrFromString(link)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(h) }()
	buf := mountPointData(`\??\`+target, target)
	var n uint32
	return windows.DeviceIoControl(h, fsctlSetReparsePoint, &buf[0], uint32(len(buf)), nil, 0, &n, nil)
}

// mountPointData is a REPARSE_DATA_BUFFER for a mount point: the substitute
// name, then the print name, each NUL-terminated UTF-16.
func mountPointData(sub, print string) []byte {
	s := utf16.Encode([]rune(sub))
	pr := utf16.Encode([]rune(print))
	names := make([]byte, 0, (len(s)+len(pr)+2)*2)
	for _, c := range append(append(append(s, 0), pr...), 0) {
		names = binary.LittleEndian.AppendUint16(names, c)
	}
	data := make([]byte, 0, 8+8+len(names))
	data = binary.LittleEndian.AppendUint32(data, ioReparseTagMountPoint)
	data = binary.LittleEndian.AppendUint16(data, uint16(8+len(names))) //nolint:gosec // paths are far below 64 KiB
	data = binary.LittleEndian.AppendUint16(data, 0)
	data = binary.LittleEndian.AppendUint16(data, 0)                    // substitute name offset
	data = binary.LittleEndian.AppendUint16(data, uint16(len(s)*2))     //nolint:gosec // as above
	data = binary.LittleEndian.AppendUint16(data, uint16((len(s)+1)*2)) //nolint:gosec // print name offset
	data = binary.LittleEndian.AppendUint16(data, uint16(len(pr)*2))    //nolint:gosec // as above
	return append(data, names...)
}

// JunctionTarget returns where a junction points, and false for anything
// that isn't a junction (or symlink).
func JunctionTarget(path string) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&(os.ModeIrregular|os.ModeSymlink) == 0 {
		return "", false
	}
	t, err := os.Readlink(path)
	if err != nil {
		return "", false
	}
	return strings.TrimPrefix(t, `\??\`), true
}

// RemoveJunction removes the junction itself, never what it points to.
func RemoveJunction(path string) error {
	if _, ok := JunctionTarget(path); !ok {
		return errors.New("linkx: not a junction: " + path)
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.RemoveDirectory(p)
}
