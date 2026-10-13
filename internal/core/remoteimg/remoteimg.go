// Package remoteimg proxies project pictures (icons, galleries, pictures in
// descriptions) from the Modrinth and CurseForge CDNs to the UI, so the page
// never loads anything from another origin. It is not an open proxy: the UI
// only ever holds tokens, and a token exists only for a URL the launcher read
// from a platform's answer and found on an allowed host. Files are cached on
// disk by token, sniffed for their real type, size-capped, and the cache is
// kept under a total size by evicting the least recently used.
package remoteimg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"quartz/internal/core/fsx"
	"quartz/internal/core/netx"
)

const (
	// MaxFile caps one picture.
	MaxFile = 10 << 20
	// maxTotal is the cache size; eviction brings it down to 3/4 of this.
	maxTotal = 256 << 20
	// touchEvery limits how often a cache hit rewrites the file's time.
	touchEvery = time.Hour
	maxURL     = 2048
)

// Hosts are the CDNs pictures may come from.
var Hosts = map[string]bool{
	"cdn.modrinth.com": true, "media.forgecdn.net": true, "edge.forgecdn.net": true, "mediafilez.forgecdn.net": true,
}

// ErrUnknown is returned for a token the launcher never handed out.
var ErrUnknown = errors.New("remoteimg: unknown picture")

var (
	tokenRe = regexp.MustCompile(`^[0-9a-f]{32}$`)
	exts    = []string{".png", ".jpg", ".gif", ".webp"}
)

// Cache is safe for concurrent use.
type Cache struct {
	dir  string
	http *netx.Client
	log  *slog.Logger
	// allowed is Allowed; tests point it at a local server.
	allowed func(string) bool

	mu    sync.Mutex
	urls  map[string]string // token → URL
	total int64             // bytes on disk, -1 until first counted

	flight singleflight.Group
}

// New uses dir as the cache folder.
func New(dir string, h *netx.Client, log *slog.Logger) *Cache {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Cache{dir: dir, http: h, log: log, allowed: Allowed, urls: map[string]string{}, total: -1}
}

// Allowed reports whether raw is an https URL on an allowed host.
func Allowed(raw string) bool {
	if len(raw) == 0 || len(raw) > maxURL || strings.ContainsAny(raw, " \r\n\\") {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && Hosts[strings.ToLower(u.Hostname())] && (u.Port() == "" || u.Port() == "443")
}

// Token registers raw and returns the name the UI asks for it by ("" when
// raw isn't allowed).
func (c *Cache) Token(raw string) string {
	if c == nil || !c.allowed(raw) {
		return ""
	}
	sum := sha256.Sum256([]byte(raw))
	t := hex.EncodeToString(sum[:16])
	c.mu.Lock()
	c.urls[t] = raw
	c.mu.Unlock()
	return t
}

// Src is the UI address of a picture ("" when raw isn't allowed).
func (c *Cache) Src(raw string) string {
	if t := c.Token(raw); t != "" {
		return "/tex/remote/" + t + ".png"
	}
	return ""
}

// Path returns the cached file for a token, fetching it first when needed.
func (c *Cache) Path(ctx context.Context, token string) (string, error) {
	if !tokenRe.MatchString(token) {
		return "", ErrUnknown
	}
	if p := c.cached(token); p != "" {
		return p, nil
	}
	c.mu.Lock()
	raw, ok := c.urls[token]
	c.mu.Unlock()
	if !ok {
		return "", ErrUnknown
	}
	p, err, _ := c.flight.Do(token, func() (any, error) { return c.fetch(ctx, token, raw) })
	if err != nil {
		c.log.Debug("picture not fetched", "err", err)
		return "", err
	}
	return p.(string), nil
}

// cached finds a token's file and marks it recently used.
func (c *Cache) cached(token string) string {
	for _, ext := range exts {
		p := filepath.Join(c.dir, token+ext)
		if fi, err := os.Stat(p); err == nil { //nolint:gosec // token matched tokenRe, so p stays in dir
			if now := time.Now(); now.Sub(fi.ModTime()) > touchEvery {
				_ = os.Chtimes(p, now, now)
			}
			return p
		}
	}
	return ""
}

func (c *Cache) fetch(ctx context.Context, token, raw string) (string, error) {
	resp, err := c.http.Get(ctx, raw, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxFile+1))
	if err != nil {
		return "", err
	}
	ext := Sniff(data)
	if len(data) > MaxFile || ext == "" {
		return "", errors.New("remoteimg: not a picture")
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(c.dir, token+ext)
	if err := fsx.AtomicWrite(p, data, fsx.FilePerm); err != nil {
		return "", err
	}
	c.grow(int64(len(data)))
	return p, nil
}

// Sniff names a picture's type by its first bytes ("" for anything else).
func Sniff(b []byte) string {
	switch {
	case len(b) > 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n":
		return ".png"
	case len(b) > 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return ".jpg"
	case len(b) > 6 && (string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a"):
		return ".gif"
	case len(b) > 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return ".webp"
	}
	return ""
}

// grow counts n new bytes and evicts the least recently used files once the
// cache is over its size.
func (c *Cache) grow(n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.total < 0 {
		c.total = 0
		for _, f := range c.files() {
			c.total += f.size
		}
	} else {
		c.total += n
	}
	if c.total <= maxTotal {
		return
	}
	files := c.files()
	sort.Slice(files, func(i, j int) bool { return files[i].used.Before(files[j].used) })
	for _, f := range files {
		if c.total <= maxTotal*3/4 {
			break
		}
		if os.Remove(f.path) == nil {
			c.total -= f.size
		}
	}
}

type entry struct {
	path string
	size int64
	used time.Time
}

func (c *Cache) files() []entry {
	var out []entry
	_ = filepath.WalkDir(c.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // an unreadable entry is skipped, not fatal
		}
		if fi, err := d.Info(); err == nil {
			out = append(out, entry{p, fi.Size(), fi.ModTime()})
		}
		return nil
	})
	return out
}
