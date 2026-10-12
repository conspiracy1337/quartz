package manual

import "os"

func osDownloads() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return xdgDownloads(home, os.Getenv("XDG_CONFIG_HOME"))
}
