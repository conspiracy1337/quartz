package manual

import (
	"os"
	"quartz/internal/core/xdg"
)

func osDownloads() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return xdg.UserDir(home, os.Getenv("XDG_CONFIG_HOME"), "DOWNLOAD")
}
