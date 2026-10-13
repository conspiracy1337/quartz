// Package browse is the Browse page and the project page: one search over
// Modrinth and CurseForge whose results merge into one row per project, and
// project pages that read both platforms. Platforms are only ever asked by
// the launcher; the UI gets plain data, same-origin pictures and opaque
// references, never URLs it could send back.
package browse

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"quartz/internal/content"
	"quartz/internal/core/remoteimg"
	"quartz/internal/modplatform"
)

// Target is the instance Browse adds to.
type Target struct {
	Name      string
	Minecraft string
	Loader    string // "" for vanilla
	Installed []content.Installed
}

// Deps wires the service.
type Deps struct {
	Platforms []modplatform.Platform
	Pictures  *remoteimg.Cache
	// Target describes an instance ("" never reaches it).
	Target func(id string) (Target, error)
	// Channel is how unstable a version may be (the update channel setting).
	Channel func() modplatform.Channel
	Log     *slog.Logger
	Now     func() time.Time
}

// Service is safe for concurrent use.
type Service struct {
	d Deps

	mu       sync.Mutex
	sessions map[string]*session

	cats     *ttl[map[string]string]
	details  *ttl[modplatform.Detail]
	versions *ttl[[]modplatform.Version]
	twins    *ttl[string]

	// noDisclose is set once Modrinth refuses disclosure filters.
	noDisclose atomic.Bool
}

// ErrUnknownSearch is returned for an expired or unknown search.
var ErrUnknownSearch = errors.New("browse: this search has expired")

// New returns a service.
func New(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Channel == nil {
		d.Channel = func() modplatform.Channel { return modplatform.Release }
	}
	return &Service{
		d: d, sessions: map[string]*session{},
		cats: newTTL[map[string]string](32), details: newTTL[modplatform.Detail](128),
		versions: newTTL[[]modplatform.Version](64), twins: newTTL[string](256),
	}
}

// platform returns a usable platform, or nil.
func (s *Service) platform(src modplatform.Source) modplatform.Platform {
	for _, p := range s.d.Platforms {
		if p.Source() == src && p.Ready() {
			return p
		}
	}
	return nil
}

func (s *Service) target(id string) (*Target, error) {
	if id == "" || s.d.Target == nil {
		return nil, nil
	}
	t, err := s.d.Target(id)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// ttl is a small in-memory cache with per-entry expiry and a size cap.
type ttl[V any] struct {
	mu    sync.Mutex
	max   int
	items map[string]ttlEntry[V]
}

type ttlEntry[V any] struct {
	v   V
	exp time.Time
}

func newTTL[V any](max int) *ttl[V] { return &ttl[V]{max: max, items: map[string]ttlEntry[V]{}} }

func (c *ttl[V]) get(k string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[k]
	if !ok || time.Now().After(e.exp) {
		var zero V
		return zero, false
	}
	return e.v, true
}

func (c *ttl[V]) put(k string, v V, life time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.max {
		now := time.Now()
		for key, e := range c.items {
			if now.After(e.exp) {
				delete(c.items, key)
			}
		}
		for key := range c.items {
			if len(c.items) < c.max {
				break
			}
			delete(c.items, key)
		}
	}
	c.items[k] = ttlEntry[V]{v, time.Now().Add(life)}
}

// cached runs fetch once per key and life.
func cached[V any](c *ttl[V], key string, life time.Duration, fetch func() (V, error)) (V, error) {
	if v, ok := c.get(key); ok {
		return v, nil
	}
	v, err := fetch()
	if err == nil {
		c.put(key, v, life)
	}
	return v, err
}

// detail reads a project from one platform, cached for five minutes.
func (s *Service) detail(ctx context.Context, src modplatform.Source, id string) (modplatform.Detail, error) {
	p := s.platform(src)
	if p == nil {
		return modplatform.Detail{}, errors.New("browse: " + string(src) + " is not available")
	}
	return cached(s.details, modplatform.Key(src, id), 5*time.Minute, func() (modplatform.Detail, error) { return p.Project(ctx, id) })
}

// allVersions lists a project's versions on one platform, cached for five minutes.
func (s *Service) allVersions(ctx context.Context, src modplatform.Source, id string) ([]modplatform.Version, error) {
	p := s.platform(src)
	if p == nil {
		return nil, errors.New("browse: " + string(src) + " is not available")
	}
	return cached(s.versions, modplatform.Key(src, id), 5*time.Minute, func() ([]modplatform.Version, error) {
		return p.Versions(ctx, id, modplatform.Query{})
	})
}
