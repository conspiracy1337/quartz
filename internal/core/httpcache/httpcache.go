// Package httpcache is the launcher's polite metadata cache (Prism's
// HttpMetaCache with its bugs fixed). It honours Cache-Control and Expires,
// revalidates with ETag / Last-Modified, deduplicates concurrent fetches,
// skips the network entirely for content pinned by hash, and serves the last
// good copy when the network is down.
package httpcache

import (
	"context"
	"crypto/sha1" //nolint:gosec // cache file names only
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"quartz/internal/core/flightx"
	"quartz/internal/core/fsx"
	"quartz/internal/core/hashx"
	"quartz/internal/core/jsonfile"
)

const defaultMaxBody = 64 << 20

// Doer sends HTTP requests; *netx.Client implements it.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Policy controls one lookup.
type Policy struct {
	// TTL is how long a response without server caching headers stays fresh.
	// Zero means "always revalidate".
	TTL time.Duration
	// ForceRevalidate asks the server even when the entry is fresh.
	ForceRevalidate bool
	// SHA1, when set, pins the content: a cached file with this hash is used
	// without any request, and a downloaded body must match it.
	SHA1 string
	// MaxSize caps the body; default 64 MB.
	MaxSize int64
}

// entry is one cached response in the index.
type entry struct {
	File         string `json:"file"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	SHA1         string `json:"sha1"`
	Size         int64  `json:"size"`
	ModTime      int64  `json:"mtime"`    // unix nanoseconds of the body file
	StoredAt     int64  `json:"storedAt"` // unix seconds, already adjusted by Age
	Lifetime     int64  `json:"lifetime"` // seconds, when the server gave one
	HasLifetime  bool   `json:"hasLifetime,omitempty"`
	NoCache      bool   `json:"noCache,omitempty"`
}

type index struct {
	Version int               `json:"version"`
	Entries map[string]*entry `json:"entries"`
}

// Cache stores bodies in dir/http and the index in dir/index.json.
type Cache struct {
	dir   string
	idx   *jsonfile.Store[index]
	doer  Doer
	log   *slog.Logger
	group flightx.Group[[]byte]
	now   func() time.Time
}

// Open loads or creates the cache in dir.
func Open(dir string, doer Doer, log *slog.Logger) (*Cache, error) {
	if err := os.MkdirAll(filepath.Join(dir, "http"), 0o700); err != nil {
		return nil, err
	}
	idx, err := jsonfile.Open(jsonfile.Options[index]{
		Path:    filepath.Join(dir, "index.json"),
		Default: func() index { return index{Version: 1, Entries: map[string]*entry{}} },
		Normalize: func(i index) index {
			if i.Entries == nil {
				i.Entries = map[string]*entry{}
			}
			return i
		},
		MaxSize: 16 << 20, Compact: true, Delay: 2 * time.Second, Log: log,
	})
	if err != nil {
		return nil, err
	}
	return &Cache{dir: dir, idx: idx, doer: doer, log: log, now: time.Now}, nil
}

// Flush writes the index; call on shutdown.
func (c *Cache) Flush() error { return c.idx.Flush() }

// Get returns the body for url according to p. Concurrent calls for the same
// URL share one fetch.
func (c *Cache) Get(ctx context.Context, url string, p Policy) ([]byte, error) {
	return c.group.Do(ctx, url+"\x00"+p.SHA1, func(ctx context.Context) ([]byte, error) { return c.get(ctx, url, p) })
}

// GetJSON fetches url through the cache and decodes it into T.
func GetJSON[T any](ctx context.Context, c *Cache, url string, p Policy) (T, error) {
	var v T
	data, err := c.Get(ctx, url, p)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return v, fmt.Errorf("decoding %s: %w", url, err)
	}
	return v, nil
}

func (c *Cache) get(ctx context.Context, url string, p Policy) ([]byte, error) {
	if p.MaxSize == 0 {
		p.MaxSize = defaultMaxBody
	}
	e := c.valid(url)
	if e != nil {
		pinned := p.SHA1 != "" && e.SHA1 == strings.ToLower(p.SHA1)
		if pinned || (p.SHA1 == "" && !p.ForceRevalidate && c.fresh(e, p)) {
			return c.read(e)
		}
	}
	data, err := c.fetch(ctx, url, p, e)
	if err != nil && e != nil && p.SHA1 == "" && ctx.Err() == nil {
		c.log.Warn("using cached copy, fetch failed", "url", url, "err", err)
		return c.read(e)
	}
	return data, err
}

// valid returns the index entry if its body file is intact. Size and mtime are
// checked first; the file is re-hashed only when they changed.
func (c *Cache) valid(url string) *entry {
	cur, ok := c.idx.Get().Entries[url]
	if !ok {
		return nil
	}
	e := *cur
	path := filepath.Join(c.dir, "http", e.File)
	info, err := os.Stat(path)
	if err == nil && info.Size() == e.Size && info.ModTime().UnixNano() == e.ModTime {
		return &e
	}
	if err == nil && info.Size() == e.Size {
		if sum, herr := hashx.File(path, hashx.SHA1); herr == nil && sum == e.SHA1 {
			e.ModTime = info.ModTime().UnixNano()
			c.put(url, &e)
			return &e
		}
	}
	c.drop(url)
	return nil
}

func (c *Cache) fresh(e *entry, p Policy) bool {
	if e.NoCache {
		return false
	}
	life := p.TTL
	if e.HasLifetime {
		life = time.Duration(e.Lifetime) * time.Second
	}
	return c.now().Before(time.Unix(e.StoredAt, 0).Add(life))
}

func (c *Cache) read(e *entry) ([]byte, error) {
	return fsx.ReadCapped(filepath.Join(c.dir, "http", e.File), defaultMaxBody*4)
}

func (c *Cache) fetch(ctx context.Context, url string, p Policy, old *entry) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if old != nil {
		if old.ETag != "" {
			req.Header.Set("If-None-Match", old.ETag)
		}
		if old.LastModified != "" {
			req.Header.Set("If-Modified-Since", old.LastModified)
		}
	}
	resp, err := c.doer.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified && old != nil {
		next := *old
		c.applyHeaders(&next, resp.Header)
		c.put(url, &next)
		return c.read(&next)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("httpcache: unexpected status %d for %s", resp.StatusCode, url)
	}
	data, sum, err := readBody(resp.Body, p.MaxSize)
	if err != nil {
		return nil, err
	}
	if p.SHA1 != "" {
		if err := hashx.Of(hashx.SHA1, p.SHA1).Check(sum); err != nil {
			return nil, err
		}
	}
	cc := parseCacheControl(resp.Header.Get("Cache-Control"))
	if cc.noStore {
		c.drop(url)
		return data, nil
	}
	if err := c.store(url, data, sum, old, resp.Header); err != nil {
		c.log.Warn("could not cache response", "url", url, "err", err)
	}
	return data, nil
}

func readBody(r io.Reader, limit int64) ([]byte, string, error) {
	h := hashx.New(hashx.SHA1)
	data, err := io.ReadAll(io.TeeReader(io.LimitReader(r, limit+1), h))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > limit {
		return nil, "", fmt.Errorf("httpcache: response larger than %d bytes", limit)
	}
	return data, hashx.Hex(h), nil
}

func (c *Cache) store(url string, data []byte, sum string, old *entry, h http.Header) error {
	file := fileName(url)
	path := filepath.Join(c.dir, "http", file)
	if err := fsx.AtomicWrite(path, data, fsx.FilePerm); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	e := &entry{File: file, SHA1: sum, Size: info.Size(), ModTime: info.ModTime().UnixNano()}
	if old != nil {
		e.ETag, e.LastModified = old.ETag, old.LastModified
	}
	c.applyHeaders(e, h)
	c.put(url, e)
	return nil
}

// applyHeaders records validators and freshness from a 200 or 304 response.
// An empty ETag never overwrites a stored one.
func (c *Cache) applyHeaders(e *entry, h http.Header) {
	if v := h.Get("ETag"); v != "" {
		e.ETag = v
	}
	if v := h.Get("Last-Modified"); v != "" {
		e.LastModified = v
	}
	now := c.now()
	age, _ := strconv.ParseInt(h.Get("Age"), 10, 64)
	e.StoredAt = now.Unix() - max(age, 0)
	cc := parseCacheControl(h.Get("Cache-Control"))
	e.NoCache = cc.noCache
	switch {
	case cc.hasMaxAge:
		e.Lifetime, e.HasLifetime = cc.maxAge, true
	case h.Get("Expires") != "":
		base := now
		if d, err := http.ParseTime(h.Get("Date")); err == nil {
			base = d
		}
		var life int64 // an unparsable Expires means "already expired" (RFC 9111 §5.3)
		if exp, err := http.ParseTime(h.Get("Expires")); err == nil {
			life = max(int64(exp.Sub(base).Seconds()), 0)
		}
		e.Lifetime, e.HasLifetime = life, true
	default:
		e.Lifetime, e.HasLifetime = 0, false
	}
}

func (c *Cache) put(url string, e *entry) {
	_, _ = c.idx.Update(func(i *index) error { i.Entries[url] = e; return nil })
}

func (c *Cache) drop(url string) {
	if _, ok := c.idx.Get().Entries[url]; !ok {
		return
	}
	var file string
	_, _ = c.idx.Update(func(i *index) error {
		if e := i.Entries[url]; e != nil {
			file = e.File
		}
		delete(i.Entries, url)
		return nil
	})
	if file != "" {
		_ = os.Remove(filepath.Join(c.dir, "http", file))
	}
}

func fileName(url string) string {
	sum := sha1.Sum([]byte(url)) //nolint:gosec // naming only
	return hex.EncodeToString(sum[:])
}

type cacheControl struct {
	noStore, noCache, hasMaxAge bool
	maxAge                      int64
}

func parseCacheControl(v string) cacheControl {
	var cc cacheControl
	for _, part := range strings.Split(v, ",") {
		k, val, _ := strings.Cut(strings.TrimSpace(strings.ToLower(part)), "=")
		switch k {
		case "no-store":
			cc.noStore = true
		case "no-cache":
			cc.noCache = true
		case "max-age":
			if n, err := strconv.ParseInt(strings.Trim(val, `"`), 10, 64); err == nil && n >= 0 {
				cc.maxAge, cc.hasMaxAge = n, true
			}
		}
	}
	return cc
}

// ErrNotCached is returned by Peek when nothing usable is cached.
var ErrNotCached = errors.New("httpcache: not cached")

// Peek returns the cached body for url without any network access.
func (c *Cache) Peek(url string) ([]byte, error) {
	if e := c.valid(url); e != nil {
		return c.read(e)
	}
	return nil, ErrNotCached
}
