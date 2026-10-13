package manual

import (
	"os"
	"path/filepath"
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
