// Package clipimg puts a picture on the system clipboard (Wails' clipboard
// is text only). Each OS uses what it ships with, without cgo: user32 on
// Windows, osascript on macOS, and wl-copy or xclip on Linux. Paths are
// passed as arguments or file input, never through a shell.
package clipimg

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg" // screenshots re-saved as JPEG
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoTool is returned on Linux when neither wl-copy nor xclip is installed.
var ErrNoTool = errors.New("copying pictures needs wl-clipboard (Wayland) or xclip (X11) installed")

// CopyPNG puts the PNG file at path on the clipboard.
func CopyPNG(path string) error { return copyPNG(path) }

// maxSource caps a picture CopyImage decodes.
const maxSource = 64 << 20

// CopyImage puts the picture at path on the clipboard as a PNG: a PNG as it
// is, another format (a compressed JPEG screenshot) re-encoded first.
func CopyImage(path string) error {
	if strings.EqualFold(filepath.Ext(path), ".png") {
		return copyPNG(path)
	}
	f, err := os.Open(path) //nolint:gosec // a picture the caller checked
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxSource+1))
	_ = f.Close()
	if err != nil {
		return err
	}
	if len(data) > maxSource {
		return errors.New("clipimg: picture too large")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "quartz-clip-*.png")
	if err != nil {
		return err
	}
	// The clipboard tools have read it (or hold it open) by the time copyPNG
	// returns, so the file can go.
	defer func() { _ = os.Remove(tmp.Name()) }()
	err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(tmp, img)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return copyPNG(tmp.Name())
}
