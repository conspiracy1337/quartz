// Package localimg serves pictures from game folders (screenshots, world
// icons) to the UI without the UI ever naming a path: the launcher registers
// each file it lists and hands out an opaque token, derived from the path,
// size and modification time. A token whose file changed or moved resolves to
// nothing, so the image routes can cache forever. Thumbnails are made on
// first request and kept in a size-capped cache.
package localimg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/jpeg"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"quartz/internal/core/disklru"
	"quartz/internal/core/flightx"
	"quartz/internal/core/fsx"
	"quartz/internal/core/pngx"
)

// Limits.
const (
	ThumbWidth = 480
	maxSource  = 64 << 20
	maxSide    = 16384
	maxThumbs  = 256 << 20
	maxEntries = 50_000
)

// ErrUnknown is returned for a token never handed out, or whose file changed.
var ErrUnknown = errors.New("localimg: unknown picture")

var tokenRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

type file struct {
	path string
	size int64
	mod  time.Time
}

// Registry is safe for concurrent use.
type Registry struct {
	thumbs *disklru.LRU

	mu    sync.Mutex
	files map[string]file
	order []string // registration order, for the entry cap

	flight flightx.Group[string]
}

// New keeps thumbnails in thumbDir.
func New(thumbDir string) *Registry {
	return &Registry{thumbs: disklru.New(thumbDir, maxThumbs), files: map[string]file{}}
}

// Register returns the token for a file the launcher just listed.
func (r *Registry) Register(path string, fi fs.FileInfo) string {
	sum := sha256.Sum256([]byte(path + "\x00" + strconv.FormatInt(fi.Size(), 10) + "\x00" + strconv.FormatInt(fi.ModTime().UnixNano(), 10)))
	t := hex.EncodeToString(sum[:16])
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.files[t]; !ok {
		r.files[t] = file{path: path, size: fi.Size(), mod: fi.ModTime()}
		r.order = append(r.order, t)
		if len(r.order) > maxEntries {
			drop := r.order[:len(r.order)-maxEntries]
			for _, old := range drop {
				delete(r.files, old)
			}
			r.order = append([]string(nil), r.order[len(drop):]...)
		}
	}
	return t
}

// URL is the UI address of a token on an image route ("shot", "thumb", "world").
func URL(route, token string) string { return "/tex/" + route + "/" + token + ".png" }

// Path resolves a token to its file, if the file is unchanged.
func (r *Registry) Path(_ context.Context, token string) (string, error) {
	f, err := r.lookup(token)
	if err != nil {
		return "", err
	}
	return f.path, nil
}

func (r *Registry) lookup(token string) (file, error) {
	if !tokenRe.MatchString(token) {
		return file{}, ErrUnknown
	}
	r.mu.Lock()
	f, ok := r.files[token]
	r.mu.Unlock()
	if !ok {
		return file{}, ErrUnknown
	}
	fi, err := os.Lstat(f.path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() != f.size || !fi.ModTime().Equal(f.mod) {
		return file{}, ErrUnknown
	}
	return f, nil
}

// Thumb resolves a token to a thumbnail at most ThumbWidth wide, made on
// first request. Pictures already that small are served as they are.
func (r *Registry) Thumb(ctx context.Context, token string) (string, error) {
	f, err := r.lookup(token)
	if err != nil {
		return "", err
	}
	p := filepath.Join(r.thumbs.Dir(), token+".jpg")
	if hit := r.thumbs.Hit(p); hit != "" {
		return hit, nil
	}
	return r.flight.Do(ctx, token, func(ctx context.Context) (string, error) { return r.makeThumb(ctx, f, p) })
}

func (r *Registry) makeThumb(_ context.Context, f file, dest string) (string, error) {
	data, err := fsx.ReadCapped(f.path, maxSource)
	if err != nil {
		return "", err
	}
	img, err := pngx.Decode(data, pngx.Limits{MaxBytes: maxSource, MaxSide: maxSide})
	if err != nil {
		return "", err
	}
	b := img.Bounds()
	if b.Dx() <= ThumbWidth {
		return f.path, nil
	}
	h := max(1, b.Dy()*ThumbWidth/b.Dx())
	dst := pngx.Scale(img, ThumbWidth, h)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 82}); err != nil {
		return "", err
	}
	if err := os.MkdirAll(r.thumbs.Dir(), 0o700); err != nil {
		return "", err
	}
	if err := fsx.AtomicWrite(dest, buf.Bytes(), fsx.FilePerm); err != nil {
		return "", err
	}
	r.thumbs.Added(int64(buf.Len()))
	return dest, nil
}

// Dims reads a PNG's width and height from its header without decoding it.
func Dims(path string) (int, int, error) {
	f, err := os.Open(path) //nolint:gosec // a file the caller listed inside a game folder
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = f.Close() }()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}
