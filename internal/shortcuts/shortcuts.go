// Package shortcuts puts an instance on the desktop or in the app menu: a
// .desktop file on Linux, a .lnk on Windows, a small .app on macOS. Each one
// runs the launcher with --launch <id> (plus a fixed account and a world or
// server when chosen), so it starts the game through the open launcher or a
// new one. Shortcuts are recorded on their instance: trashing the instance
// removes the files, restoring it puts them back, and a launcher that moved
// rewrites them.
package shortcuts

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// Where is a place for a shortcut.
type Where string

const (
	Desktop Where = "desktop"
	Menu    Where = "menu" // the app menu (Linux), Start menu (Windows), ~/Applications (macOS)
)

// Valid reports whether w is a known place.
func (w Where) Valid() bool { return w == Desktop || w == Menu }

// Spec is one shortcut to write.
type Spec struct {
	ID    string // the instance id, also the marker that makes the file ours
	Name  string // what the shortcut is called (the instance's name)
	Exe   string // the launcher
	Args  []string
	Icon  []byte // a square PNG
	Where Where
}

// writer is one OS's shortcut files.
type writer interface {
	// create writes s (iconDir holds icon files it refers to) and returns its path.
	create(s Spec, iconDir string) (string, error)
	// remove deletes a shortcut it wrote for instance id; anything else at
	// path is left alone.
	remove(path, id string) error
}

var (
	// ErrNoFolder means the desktop or menu folder can't be found.
	ErrNoFolder = errors.New("shortcuts: that folder doesn't exist on this computer")
	// ErrUnsupported means shortcuts don't exist on this OS.
	ErrUnsupported = errors.New("shortcuts: not supported on this system")
)

// fileName turns an instance name into a safe file name: no separators,
// reserved or control characters, no leading dots, at most 64 characters.
func fileName(name, fallback string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return -1
		}
		return r
	}, name)
	name = strings.Trim(name, ". ")
	if r := []rune(name); len(r) > 64 {
		name = strings.TrimSpace(string(r[:64]))
	}
	switch strings.ToUpper(name) {
	case "", "CON", "PRN", "AUX", "NUL", "COM1", "LPT1":
		return fallback
	}
	return name
}

// free returns dir/base+ext, or base (2)+ext and so on when taken.
func free(dir, base, ext string) (string, error) {
	for n := 1; n <= 50; n++ {
		name := base
		if n > 1 {
			name += " (" + strconv.Itoa(n) + ")"
		}
		p := filepath.Join(dir, name+ext)
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			return p, nil
		}
	}
	return "", os.ErrExist
}
