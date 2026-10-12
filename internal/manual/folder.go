package manual

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// DownloadsFolder is the folder a browser saves into: the OS's own setting
// (XDG user-dirs on Linux, the known folder on Windows), else ~/Downloads.
func DownloadsFolder() string {
	if p := osDownloads(); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Downloads")
}

// xdgDownloads reads XDG_DOWNLOAD_DIR from user-dirs.dirs.
func xdgDownloads(home, config string) string {
	if config == "" {
		config = filepath.Join(home, ".config")
	}
	f, err := os.Open(filepath.Join(config, "user-dirs.dirs")) //nolint:gosec // the user's own config file
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || k != "XDG_DOWNLOAD_DIR" {
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
