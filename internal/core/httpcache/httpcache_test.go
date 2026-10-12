package httpcache

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quartz/internal/core/hashx"
	"quartz/internal/core/netx"
)

type server struct {
	*httptest.Server
	hits    atomic.Int32
	mu      sync.Mutex
	headers http.Header
	body    string
	status  int
}

func newServer(t *testing.T) *server {
	s := &server{headers: http.Header{}, body: "v1", status: 200}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.mu.Lock()
		defer s.mu.Unlock()
		for k, v := range s.headers {
			w.Header()[k] = v
		}
		if etag := s.headers.Get("ETag"); etag != "" && r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(s.status)
		_, _ = io.WriteString(w, s.body)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *server) set(fn func(s *server)) {
	s.mu.Lock()
	fn(s)
	s.mu.Unlock()
}

func open(t *testing.T) (*Cache, *time.Time) {
	t.Helper()
	nc, err := netx.New(netx.Proxy{Mode: netx.ProxyNone}, netx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	c, err := Open(t.TempDir(), nc, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	c.now = func() time.Time { return now }
	return c, &now
}

func get(t *testing.T, c *Cache, url string, p Policy) string {
	t.Helper()
	b, err := c.Get(context.Background(), url, p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMaxAgeThenRevalidate304(t *testing.T) {
	s := newServer(t)
	s.set(func(s *server) { s.headers.Set("Cache-Control", "max-age=60"); s.headers.Set("ETag", `"a"`) })
	c, now := open(t)
	for range 2 {
		if get(t, c, s.URL, Policy{}) != "v1" {
			t.Fatal("wrong body")
		}
	}
	if s.hits.Load() != 1 {
		t.Fatalf("fresh entry refetched: %d hits", s.hits.Load())
	}
	*now = now.Add(61 * time.Second)
	if get(t, c, s.URL, Policy{}) != "v1" || s.hits.Load() != 2 {
		t.Fatalf("expired entry not revalidated: %d hits", s.hits.Load())
	}
	if get(t, c, s.URL, Policy{}) != "v1" || s.hits.Load() != 2 {
		t.Fatal("304 did not refresh the lifetime")
	}
}

func TestExpiresAndAge(t *testing.T) {
	s := newServer(t)
	date := time.Now().UTC().Truncate(time.Second)
	s.set(func(s *server) {
		s.headers.Set("Date", date.Format(http.TimeFormat))
		s.headers.Set("Expires", date.Add(30*time.Second).Format(http.TimeFormat))
	})
	c, now := open(t)
	get(t, c, s.URL, Policy{})
	*now = now.Add(20 * time.Second)
	get(t, c, s.URL, Policy{})
	if s.hits.Load() != 1 {
		t.Fatal("Expires not honoured")
	}
	*now = now.Add(20 * time.Second)
	get(t, c, s.URL, Policy{})
	if s.hits.Load() != 2 {
		t.Fatal("expired Expires not refetched")
	}
	s.set(func(s *server) { s.headers = http.Header{"Cache-Control": {"max-age=60"}, "Age": {"50"}} })
	get(t, c, s.URL+"/aged", Policy{})
	*now = now.Add(15 * time.Second)
	get(t, c, s.URL+"/aged", Policy{})
	if s.hits.Load() != 4 {
		t.Fatalf("Age not subtracted: %d hits", s.hits.Load())
	}
}

func TestNoStoreNoCacheAndTTL(t *testing.T) {
	s := newServer(t)
	c, now := open(t)
	s.set(func(s *server) { s.headers.Set("Cache-Control", "no-store") })
	get(t, c, s.URL, Policy{TTL: time.Hour})
	if _, err := c.Peek(s.URL); !errors.Is(err, ErrNotCached) {
		t.Fatal("no-store response cached")
	}
	s.set(func(s *server) { s.headers.Set("Cache-Control", "no-cache") })
	get(t, c, s.URL+"/nc", Policy{TTL: time.Hour})
	get(t, c, s.URL+"/nc", Policy{TTL: time.Hour})
	if s.hits.Load() != 3 {
		t.Fatal("no-cache not revalidated")
	}
	s.set(func(s *server) { s.headers = http.Header{} })
	get(t, c, s.URL+"/ttl", Policy{TTL: time.Minute})
	get(t, c, s.URL+"/ttl", Policy{TTL: time.Minute})
	*now = now.Add(2 * time.Minute)
	get(t, c, s.URL+"/ttl", Policy{TTL: time.Minute})
	if s.hits.Load() != 5 {
		t.Fatalf("policy TTL wrong: %d hits", s.hits.Load())
	}
}

func TestEmptyETagKeepsStored(t *testing.T) {
	s := newServer(t)
	s.set(func(s *server) { s.headers.Set("ETag", `"keep"`) })
	c, _ := open(t)
	get(t, c, s.URL, Policy{})
	s.set(func(s *server) { s.headers = http.Header{}; s.body = "v2" })
	get(t, c, s.URL, Policy{})
	if e := c.idx.Get().Entries[s.URL]; e.ETag != `"keep"` {
		t.Fatalf("etag = %q", e.ETag)
	}
}

func TestPinnedHashSkipsNetwork(t *testing.T) {
	s := newServer(t)
	c, _ := open(t)
	sum := hashSHA1("v1")
	get(t, c, s.URL, Policy{SHA1: sum})
	get(t, c, s.URL, Policy{SHA1: sum, ForceRevalidate: true})
	if s.hits.Load() != 1 {
		t.Fatal("pinned content refetched")
	}
	if _, err := c.Get(context.Background(), s.URL+"/bad", Policy{SHA1: "0000000000000000000000000000000000000000"}); !errors.Is(err, hashx.ErrMismatch) {
		t.Fatalf("hash mismatch not reported: %v", err)
	}
}

func TestStaleIfError(t *testing.T) {
	s := newServer(t)
	c, _ := open(t)
	get(t, c, s.URL, Policy{})
	s.set(func(s *server) { s.status = 500 })
	if get(t, c, s.URL, Policy{}) != "v1" {
		t.Fatal("stale copy not served on error")
	}
	if _, err := c.Get(context.Background(), s.URL+"/never", Policy{}); err == nil {
		t.Fatal("uncached URL with failing server must error")
	}
}

func TestTamperedFileRefetched(t *testing.T) {
	s := newServer(t)
	s.set(func(s *server) { s.headers.Set("Cache-Control", "max-age=600") })
	c, _ := open(t)
	get(t, c, s.URL, Policy{})
	path := filepath.Join(c.dir, "http", fileName(s.URL))
	if err := os.WriteFile(path, []byte("v9"), 0o600); err != nil {
		t.Fatal(err)
	}
	if get(t, c, s.URL, Policy{}) != "v1" || s.hits.Load() != 2 {
		t.Fatal("tampered cache file served")
	}
}

func TestConcurrentFetchesShareOneRequest(t *testing.T) {
	release := make(chan struct{})
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		<-release
		_, _ = io.WriteString(w, "shared")
	}))
	defer srv.Close()
	c, _ := open(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if b, err := c.Get(context.Background(), srv.URL, Policy{}); err != nil || string(b) != "shared" {
				t.Errorf("got %q, %v", b, err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if hits.Load() != 1 {
		t.Fatalf("%d requests for one URL", hits.Load())
	}
}

func TestParseCacheControl(t *testing.T) {
	cc := parseCacheControl(`public, Max-Age="120", no-cache`)
	if !cc.hasMaxAge || cc.maxAge != 120 || !cc.noCache || cc.noStore {
		t.Fatalf("got %+v", cc)
	}
	if parseCacheControl("max-age=-1").hasMaxAge {
		t.Fatal("negative max-age accepted")
	}
}

func hashSHA1(s string) string {
	h := hashx.New(hashx.SHA1)
	_, _ = io.WriteString(h, s)
	return hashx.Hex(h)
}
