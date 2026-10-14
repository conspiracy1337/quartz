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
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/sync/singleflight"

	"quartz/internal/core/disklru"
	"quartz/internal/core/fsx"
	"quartz/internal/core/netx"
)

const (
	// MaxFile caps one picture.
	MaxFile = 10 << 20
	// maxTotal is the cache size (see disklru).
	maxTotal = 256 << 20
	maxURL   = 2048
)

// Hosts are the CDNs pictures may come from.
var Hosts = map[string]bool{
	"cdn.modrinth.com": true, "media.forgecdn.net": true, "edge.forgecdn.net": true, "mediafilez.forgecdn.net": true,
	// The official launcher's news pictures (Home's news strip).
	"launchercontent.mojang.com": true,
	// Where project descriptions commonly keep their pictures.
	"i.imgur.com": true, "raw.githubusercontent.com": true, "user-images.githubusercontent.com": true,
	"private-user-images.githubusercontent.com": true, "media.githubusercontent.com": true,
}

// pathHosts are hosts that serve pictures only under one path; GitHub's
// uploads redirect from there to private-user-images.githubusercontent.com.
var pathHosts = map[string]string{"github.com": "/user-attachments/"}

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

	mu   sync.Mutex
	urls map[string]string // token → URL
	lru  *disklru.LRU

	flight singleflight.Group
}

// New uses dir as the cache folder.
func New(dir string, h *netx.Client, log *slog.Logger) *Cache {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Cache{dir: dir, http: h, log: log, allowed: Allowed, urls: map[string]string{}, lru: disklru.New(dir, maxTotal)}
}

// Allowed reports whether raw is an https URL on an allowed host.
func Allowed(raw string) bool {
	if len(raw) == 0 || len(raw) > maxURL || strings.ContainsAny(raw, " \r\n\\") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if prefix, ok := pathHosts[host]; ok {
		return strings.HasPrefix(u.EscapedPath(), prefix) && !strings.Contains(u.Path, "..")
	}
	return Hosts[host]
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
		if p := c.lru.Hit(filepath.Join(c.dir, token+ext)); p != "" { // token matched tokenRe, so p stays in dir
			return p
		}
	}
	return ""
}

func (c *Cache) fetch(ctx context.Context, token, raw string) (string, error) {
	// A redirect may only lead to another allowed address, so no picture
	// can point the launcher at a host it wouldn't fetch directly.
	ctx = netx.WithRedirects(ctx, func(u *url.URL) bool { return c.allowed(u.String()) })
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
	c.lru.Added(int64(len(data)))
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
