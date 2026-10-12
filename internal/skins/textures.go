package skins

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"golang.org/x/sync/singleflight"

	"quartz/internal/core/fsx"
	"quartz/internal/core/netx"
	"quartz/internal/mcapi"
)

// Textures caches skin and cape images from textures.minecraft.net, named by
// their texture hash. The hash in the URL is the content's identity, so a
// cached file never needs revalidating.
type Textures struct {
	dir    string
	http   *netx.Client
	flight singleflight.Group
}

// NewTextures uses dir as the cache folder.
func NewTextures(dir string, h *netx.Client) *Textures { return &Textures{dir: dir, http: h} }

var textureHash = regexp.MustCompile(`^[0-9a-f]{16,128}$`)

// Path returns the cached file for a texture hash, downloading it first when
// needed. Only hashes are accepted, so the path cannot leave the cache.
func (t *Textures) Path(ctx context.Context, hash string) (string, error) {
	if !textureHash.MatchString(hash) {
		return "", mcapi.ErrTexture
	}
	path := filepath.Join(t.dir, hash+".png")
	if _, err := os.Stat(path); err == nil { //nolint:gosec // hash matched textureHash, so path stays in dir
		return path, nil
	}
	_, err, _ := t.flight.Do(hash, func() (any, error) { return nil, t.fetch(ctx, hash, path) })
	return path, err
}

// Read returns a texture's bytes (downloading it when needed).
func (t *Textures) Read(ctx context.Context, hash string) ([]byte, error) {
	path, err := t.Path(ctx, hash)
	if err != nil {
		return nil, err
	}
	return fsx.ReadCapped(path, MaxSize)
}

func (t *Textures) fetch(ctx context.Context, hash, path string) error {
	resp, err := t.http.Get(ctx, mcapi.TextureURL(hash), nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxSize+1))
	if err != nil {
		return err
	}
	if len(data) > MaxSize || !isPNG(data) {
		return errors.New("skins: texture is not a PNG")
	}
	if err := os.MkdirAll(t.dir, 0o700); err != nil {
		return err
	}
	return fsx.AtomicWrite(path, data, fsx.FilePerm)
}

func isPNG(b []byte) bool { return len(b) > 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n" }
