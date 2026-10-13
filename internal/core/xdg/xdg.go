// Package xdg reads the freedesktop.org user folders (user-dirs.dirs), such
// as the desktop and downloads folders, which are localised and movable.
package xdg

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// UserDir reads XDG_<name>_DIR (name is DESKTOP, DOWNLOAD, …) from
// user-dirs.dirs in config (default ~/.config). It returns "" when unset.
func UserDir(home, config, name string) string {
	if config == "" {
		config = filepath.Join(home, ".config")
	}
	f, err := os.Open(filepath.Join(config, "user-dirs.dirs")) //nolint:gosec // the user's own config file
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	key := "XDG_" + name + "_DIR"
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || k != key {
			continue
		}
		v = strings.Trim(v, `"`)
		v = strings.Replace(v, "$HOME", home, 1)
		if !filepath.IsAbs(v) || filepath.Clean(v) == filepath.Clean(home) {
			return "" // the spec's way of saying "not set"
		}
		return filepath.Clean(v)
	}
	return ""
}
