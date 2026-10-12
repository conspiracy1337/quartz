// Package meta fetches metadata through the HTTP cache: Mojang's version
// manifest (short TTL), version JSONs, asset indexes and runtime manifests
// (pinned by sha1, so cached forever and verified), the runtime index, and,
// through Fetch and Bytes, any other upstream such as loader version lists.
// Repeated failures back off (1 s, 3 s, 9 s, 27 s) instead of hammering a
// host that is down; the cache still serves the last good copy meanwhile.
package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"quartz/internal/core/httpcache"
	"quartz/internal/mojang"
)

const (
	manifestTTL     = 10 * time.Minute
	runtimeIndexTTL = time.Hour
)

var ladder = [...]time.Duration{time.Second, 3 * time.Second, 9 * time.Second, 27 * time.Second}

// ErrUnknownVersion is returned for ids missing from the manifest.
var ErrUnknownVersion = errors.New("meta: unknown Minecraft version")

// Service is safe for concurrent use.
type Service struct {
	cache *httpcache.Cache
	now   func() time.Time

	mu       sync.Mutex
	failures map[string]failure
}

type failure struct {
	n     int
	until time.Time
	err   error
}

// New returns a metadata service backed by cache.
func New(cache *httpcache.Cache) *Service {
	return &Service{cache: cache, now: time.Now, failures: map[string]failure{}}
}

// Fetch decodes the JSON at url through the cache, backing off after failures.
func Fetch[T any](ctx context.Context, s *Service, url string, p httpcache.Policy) (T, error) {
	return guarded(ctx, s, url, func() (T, error) { return httpcache.GetJSON[T](ctx, s.cache, url, p) })
}

// Bytes returns the body at url through the cache, backing off after failures.
func (s *Service) Bytes(ctx context.Context, url string, p httpcache.Policy) ([]byte, error) {
	return guarded(ctx, s, url, func() ([]byte, error) { return s.cache.Get(ctx, url, p) })
}

// guarded runs get unless url failed recently, and records the outcome.
func guarded[T any](ctx context.Context, s *Service, url string, get func() (T, error)) (T, error) {
	var zero T
	s.mu.Lock()
	f, failing := s.failures[url]
	s.mu.Unlock()
	if failing && s.now().Before(f.until) {
		return zero, f.err
	}
	v, err := get()
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case err == nil:
		delete(s.failures, url)
	case ctx.Err() == nil:
		f.n++
		f.err = err
		f.until = s.now().Add(ladder[min(f.n, len(ladder))-1])
		s.failures[url] = f
	}
	return v, err
}

// sidecarTTL keeps a maven checksum for a long time: a published artifact never changes.
const sidecarTTL = 365 * 24 * time.Hour

var sha1Hex = regexp.MustCompile(`^[0-9a-f]{40}`)

// SidecarSHA1 returns the sha1 a maven repository publishes next to an
// artifact (url + ".sha1"). Some repositories append the file name after the
// hash; only the leading 40 hex digits count.
func (s *Service) SidecarSHA1(ctx context.Context, url string) (string, error) {
	data, err := s.Bytes(ctx, url+".sha1", httpcache.Policy{TTL: sidecarTTL, MaxSize: 1 << 10})
	if err != nil {
		return "", err
	}
	sum := sha1Hex.FindString(strings.ToLower(strings.TrimSpace(string(data))))
	if sum == "" {
		return "", fmt.Errorf("meta: %s.sha1 is not a checksum", url)
	}
	return sum, nil
}

// Manifest returns the version manifest.
func (s *Service) Manifest(ctx context.Context) (mojang.Manifest, error) {
	return Fetch[mojang.Manifest](ctx, s, mojang.ManifestURL, httpcache.Policy{TTL: manifestTTL})
}

// Entry returns the manifest entry for id.
func (s *Service) Entry(ctx context.Context, id string) (mojang.ManifestEntry, error) {
	m, err := s.Manifest(ctx)
	if err != nil {
		return mojang.ManifestEntry{}, err
	}
	e, ok := m.Find(id)
	if !ok || !e.Supported() {
		return mojang.ManifestEntry{}, fmt.Errorf("%w: %q", ErrUnknownVersion, id)
	}
	return e, nil
}

// Version returns the version JSON for id.
func (s *Service) Version(ctx context.Context, id string) (*mojang.Version, error) {
	e, err := s.Entry(ctx, id)
	if err != nil {
		return nil, err
	}
	v, err := Fetch[mojang.Version](ctx, s, e.URL, httpcache.Policy{SHA1: e.SHA1})
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// cachedVersion returns a version JSON only if it is already cached.
func (s *Service) cachedVersion(e mojang.ManifestEntry) (*mojang.Version, bool) {
	data, err := s.cache.Peek(e.URL)
	if err != nil {
		return nil, false
	}
	v, err := mojang.Parse(data)
	return v, err == nil
}

// Families groups the manifest for the version picker. The Java column uses
// each version's real requirement when its JSON is cached, else an estimate.
func (s *Service) Families(ctx context.Context) ([]mojang.Family, error) {
	m, err := s.Manifest(ctx)
	if err != nil {
		return nil, err
	}
	return mojang.Families(m, func(e mojang.ManifestEntry) int {
		if v, ok := s.cachedVersion(e); ok {
			return v.JavaMajor()
		}
		return e.EstimatedJava()
	}), nil
}

// AssetIndex returns the asset index a version uses, parsed and as raw bytes
// (the game reads the file itself from assets/indexes).
func (s *Service) AssetIndex(ctx context.Context, v *mojang.Version) (mojang.AssetIndex, []byte, error) {
	if v.AssetIndex == nil {
		return mojang.AssetIndex{}, nil, fmt.Errorf("meta: %s names no asset index", v.ID)
	}
	data, err := s.cache.Get(ctx, v.AssetIndex.URL, httpcache.Policy{SHA1: v.AssetIndex.SHA1, MaxSize: 32 << 20})
	if err != nil {
		return mojang.AssetIndex{}, nil, err
	}
	var ix mojang.AssetIndex
	if err := json.Unmarshal(data, &ix); err != nil {
		return ix, nil, fmt.Errorf("meta: asset index %s: %w", v.AssetIndex.ID, err)
	}
	return ix, data, nil
}

// RuntimeIndex returns Mojang's Java runtime index.
func (s *Service) RuntimeIndex(ctx context.Context) (mojang.RuntimeIndex, error) {
	return Fetch[mojang.RuntimeIndex](ctx, s, mojang.RuntimeIndexURL, httpcache.Policy{TTL: runtimeIndexTTL})
}

// RuntimeManifest returns the file list of one runtime build.
func (s *Service) RuntimeManifest(ctx context.Context, b mojang.RuntimeBuild) (mojang.RuntimeManifest, error) {
	return Fetch[mojang.RuntimeManifest](ctx, s, b.Manifest.URL, httpcache.Policy{SHA1: b.Manifest.SHA1})
}
