package discord

import (
	"io"
	"os"
)

func dial(path string) (io.ReadWriteCloser, error) {
	return os.OpenFile(path, os.O_RDWR, 0) //nolint:gosec // Discord's fixed pipe names
}
