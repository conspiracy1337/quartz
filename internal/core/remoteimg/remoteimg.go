// Package remoteimg proxies project pictures (icons, galleries, pictures in
// descriptions) from the Modrinth and CurseForge CDNs, the image and object
// storage hosts descriptions link to, and Mojang's news to the UI, so the page
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
	"time"

	"quartz/internal/core/disklru"
	"quartz/internal/core/flightx"
	"quartz/internal/core/fsx"
	"quartz/internal/core/netx"
)

const (
	// MaxFile caps one picture.
	MaxFile = 10 << 20
	// maxTotal is the cache size (see disklru).
	maxTotal = 256 << 20
	maxURL   = 2048
	// maxTokens caps the URLs remembered for the UI. Past it they are all
	// forgotten: cached pictures still load by token, and the UI asks
	// for a fresh token whenever it shows a picture again.
	maxTokens = 20000
	// A picture that failed isn't asked for again for a while: longer when
	// asking again can't help (gone, not a picture, too large).
	retryPermanent = 10 * time.Minute
	retryOther     = time.Minute
)

// Hosts are the CDNs pictures may come from.
var Hosts = map[string]bool{
	"cdn.modrinth.com": true, "media.forgecdn.net": true, "edge.forgecdn.net": true, "mediafilez.forgecdn.net": true,
	// The official launcher's and the launcher's own news pictures (Home's news strip).
	"launchercontent.mojang.com": true, "quartz.conspiracy.moe": true,
	// Where project descriptions commonly keep their pictures.
	"i.imgur.com": true, "raw.githubusercontent.com": true, "user-images.githubusercontent.com": true,
	"private-user-images.githubusercontent.com": true, "media.githubusercontent.com": true,
	"firebasestorage.googleapis.com": true,
	// NoRiskClient's built-in mods show NoRisk's own pictures (modplatform/norisk).
	"norisk.gg": true,
}

// storageHosts are object storage services. They give every bucket its own
// host name, so they are matched by pattern, at a label boundary. Only a
// service's storage endpoints count: amazonaws.com and scw.cloud also name
// servers anyone can run.
var storageHosts = regexp.MustCompile(`^(?:[a-z0-9-]+\.)*(?:` + strings.Join([]string{
	`s3(?:-[a-z0-9-]+)?(?:\.dualstack)?(?:\.[a-z]{2}(?:-[a-z]+)+-[0-9]+)?\.amazonaws\.com`, // Amazon S3
	`r2\.dev`, `r2\.cloudflarestorage\.com`, // Cloudflare R2
	`storage\.googleapis\.com`,                     // Google Cloud Storage
	`blob\.core\.windows\.net`,                     // Azure Blob Storage
	`backblazeb2\.com`,                             // Backblaze B2
	`digitaloceanspaces\.com`,                      // DigitalOcean Spaces
	`wasabisys\.com`,                               // Wasabi
	`linodeobjects\.com`,                           // Akamai (Linode) Object Storage
	`your-objectstorage\.com`,                      // Hetzner
	`vultrobjects\.com`,                            // Vultr
	`s3\.[a-z]{2}-[a-z]{3}\.scw\.cloud`,            // Scaleway
	`s3\.[a-z0-9-]+\.(?:io|perf)\.cloud\.ovh\.net`, // OVHcloud
	`objectstorage\.[a-z0-9-]+\.oraclecloud\.com`,  // Oracle Cloud
}, "|") + `)$`)

// pathHosts are hosts that serve pictures only under one path; GitHub's
// uploads redirect from there to private-user-images.githubusercontent.com.
var pathHosts = map[string]string{"github.com": "/user-attachments/"}

// ErrUnknown is returned for a token the launcher never handed out.
var ErrUnknown = errors.New("remoteimg: unknown picture")

// errNotPicture is returned for a download that isn't a picture the UI may
// show; asking again won't change that.
var errNotPicture = &netx.PermanentError{Err: errors.New("remoteimg: not a picture")}

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

	mu     sync.Mutex
	urls   map[string]string  // token → URL
	failed map[string]failure // token → its last failure, while it counts
	lru    *disklru.LRU
	now    func() time.Time

	flight flightx.Group[string]
}

// New uses dir as the cache folder.
func New(dir string, h *netx.Client, log *slog.Logger) *Cache {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Cache{dir: dir, http: h, log: log, allowed: Allowed, urls: map[string]string{}, failed: map[string]failure{},
		lru: disklru.New(dir, maxTotal), now: time.Now}
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
	return Hosts[host] || storageHosts.MatchString(host)
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
	if _, ok := c.urls[t]; !ok && len(c.urls) >= maxTokens {
		c.urls, c.failed = map[string]string{}, map[string]failure{}
	}
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
	f, failed := c.failed[token]
	c.mu.Unlock()
	if !ok {
		return "", ErrUnknown
	}
	if failed && c.now().Before(f.until) {
		return "", f.err
	}
	p, err := c.flight.Do(ctx, token, func(ctx context.Context) (string, error) { return c.fetch(ctx, token, raw) })
	if err != nil && ctx.Err() == nil { // a page that stopped waiting isn't a failure
		c.log.Debug("picture not fetched", "err", err)
		wait := retryOther
		if cls, _ := netx.Classify(err); cls == netx.Permanent {
			wait = retryPermanent
		}
		c.mu.Lock()
		c.failed[token] = failure{err: err, until: c.now().Add(wait)}
		c.mu.Unlock()
	}
	return p, err
}

// failure is a fetch that failed and when it may be tried again.
type failure struct {
	err   error
	until time.Time
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
		return "", errNotPicture
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
