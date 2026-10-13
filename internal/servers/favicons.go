package servers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"quartz/internal/core/fsx"
	"quartz/internal/core/pngx"
)

// Favicons keeps server pictures (from pings and from servers.dat) as small
// canonical PNGs named by their SHA-256, for the "server" image route.
type Favicons struct {
	dir   string
	mu    sync.Mutex
	have  map[string]bool
	saved map[[32]byte]string // servers.dat icon text → its address, decoded once
}

// NewFavicons keeps pictures in dir.
func NewFavicons(dir string) *Favicons {
	return &Favicons{dir: dir, have: map[string]bool{}, saved: map[[32]byte]string{}}
}

var hashRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Put stores a PNG that slp already re-encoded and returns its UI address.
func (f *Favicons) Put(png []byte) string {
	sum := sha256.Sum256(png)
	h := hex.EncodeToString(sum[:])
	f.mu.Lock()
	known := f.have[h]
	f.mu.Unlock()
	if !known {
		if err := os.MkdirAll(f.dir, 0o700); err != nil {
			return ""
		}
		p := filepath.Join(f.dir, h+".png")
		if _, err := os.Stat(p); err != nil {
			if fsx.AtomicWrite(p, png, fsx.FilePerm) != nil {
				return ""
			}
		}
		f.mu.Lock()
		f.have[h] = true
		f.mu.Unlock()
	}
	return "/tex/server/" + h + ".png"
}

// PutBase64 stores servers.dat's icon (a bare base64 PNG) after checking and
// re-encoding it.
func (f *Favicons) PutBase64(b64 string) string {
	if b64 == "" || len(b64) > 128<<10 {
		return ""
	}
	key := sha256.Sum256([]byte(b64))
	f.mu.Lock()
	url, ok := f.saved[key]
	f.mu.Unlock()
	if ok {
		return url
	}
	url = f.decode(b64)
	f.mu.Lock()
	f.saved[key] = url
	f.mu.Unlock()
	return url
}

func (f *Favicons) decode(b64 string) string {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return ""
	}
	img, err := pngx.Decode(raw, pngx.Limits{MaxBytes: 64 << 10, MaxSide: 64, PNGOnly: true})
	if err != nil {
		return ""
	}
	png, _, err := pngx.Encode(img)
	if err != nil {
		return ""
	}
	return f.Put(png)
}

// Path resolves a picture's hash for the image route.
func (f *Favicons) Path(_ context.Context, h string) (string, error) {
	if !hashRe.MatchString(h) {
		return "", errors.New("servers: unknown picture")
	}
	return filepath.Join(f.dir, h+".png"), nil
}
