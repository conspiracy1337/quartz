// Package icons keeps the pictures people choose for their instances: each is
// re-encoded to a square PNG of at most 128 pixels and stored once, named by
// its SHA-256, so the same picture on two instances is one file.
package icons

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"

	"quartz/internal/core/fsx"
	"quartz/internal/core/pngx"
)

// Side is the stored icon size; the largest tile drawn is 56 px (112 at 2x).
const Side = 128

// MaxFile caps a picked file before it is decoded.
const MaxFile = 8 << 20

// ErrNotIcon is returned for anything that can't be an icon.
var ErrNotIcon = errors.New("icons: pick a PNG or JPEG picture")

var hashRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidHash reports whether h names an icon.
func ValidHash(h string) bool { return hashRe.MatchString(h) }

// Library is an icon folder.
type Library struct {
	dir  string
	side int
}

// Open returns the instance icon library in dir.
func Open(dir string) *Library { return OpenSized(dir, Side) }

// OpenSized returns a library whose pictures are stored at most side pixels
// square (mod and pack icons are drawn smaller than instance icons).
func OpenSized(dir string, side int) *Library { return &Library{dir: dir, side: side} }

// Import reads a picture the user chose in a native dialog and returns its name.
func (l *Library) Import(path string) (string, error) {
	data, err := fsx.ReadCapped(path, MaxFile)
	if errors.Is(err, fsx.ErrTooLarge) {
		return "", ErrNotIcon
	}
	if err != nil {
		return "", err
	}
	return l.Add(data)
}

// Add validates, normalises and stores an image.
func (l *Library) Add(data []byte) (string, error) {
	img, err := pngx.Decode(data, pngx.Limits{MaxBytes: MaxFile, MaxSide: 8192})
	if err != nil {
		return "", ErrNotIcon
	}
	out, hash, err := pngx.Encode(pngx.Square(img, l.side))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		return "", err
	}
	dst := filepath.Join(l.dir, hash+".png")
	if _, err := os.Stat(dst); err == nil {
		return hash, nil
	}
	return hash, fsx.AtomicWrite(dst, out, fsx.FilePerm)
}

// Path returns the file of a stored icon.
func (l *Library) Path(hash string) (string, error) {
	if !ValidHash(hash) {
		return "", os.ErrNotExist
	}
	p := filepath.Join(l.dir, hash+".png")
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	return p, nil
}
