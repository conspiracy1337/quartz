package remoteimg

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"quartz/internal/core/netx"
)

var webp = append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 32)...)

func newCache(t *testing.T, h http.HandlerFunc) (*Cache, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	nc, err := netx.New(netx.Proxy{Mode: netx.ProxyNone}, netx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	c := New(t.TempDir(), nc, nil)
	c.allowed = func(raw string) bool { return strings.HasPrefix(raw, srv.URL+"/") }
	return c, srv
}

func TestAllowed(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://cdn.modrinth.com/data/AANobbMI/icon.png":       true,
		"https://media.forgecdn.net/avatars/1/2/x.png":          true,
		"http://cdn.modrinth.com/x.png":                         false,
		"https://evil.example/x.png":                            false,
		"https://cdn.modrinth.com.evil.example/x.png":           false,
		"https://user@cdn.modrinth.com/x.png":                   false,
		"https://cdn.modrinth.com:8443/x.png":                   false,
		"https://cdn.modrinth.com/a b.png":                      false,
		"https://cdn.modrinth.com/" + strings.Repeat("a", 3000): false,
	} {
		if Allowed(raw) != want {
			t.Errorf("Allowed(%q) = %v", raw, !want)
		}
	}
	if (&Cache{allowed: Allowed, urls: map[string]string{}}).Src("https://evil.example/x.png") != "" {
		t.Fatal("token for a disallowed host")
	}
}

func TestFetchSniffAndCache(t *testing.T) {
	var hits atomic.Int32
	c, srv := newCache(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/icon":
			_, _ = w.Write(webp)
		case "/html":
			_, _ = w.Write([]byte("<html><script>alert(1)</script>"))
		case "/huge":
			_, _ = w.Write(append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, MaxFile)...))
		}
	})
	ctx := context.Background()
	if _, err := c.Path(ctx, strings.Repeat("a", 32)); !errors.Is(err, ErrUnknown) {
		t.Fatalf("unknown token: %v", err)
	}
	if _, err := c.Path(ctx, "../../etc/passwd"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("bad token: %v", err)
	}
	src := c.Src(srv.URL + "/icon")
	tok := strings.TrimSuffix(strings.TrimPrefix(src, "/tex/remote/"), ".png")
	p, err := c.Path(ctx, tok)
	if err != nil || filepath.Ext(p) != ".webp" {
		t.Fatalf("%q %v", p, err)
	}
	if _, err := c.Path(ctx, tok); err != nil || hits.Load() != 1 {
		t.Fatalf("cache missed: hits %d, %v", hits.Load(), err)
	}
	// A fresh process finds the file without knowing the URL.
	again := New(c.dir, c.http, nil)
	if _, err := again.Path(ctx, tok); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"/html", "/huge"} {
		if _, err := c.Path(ctx, c.Token(srv.URL+bad)); err == nil {
			t.Errorf("%s was served", bad)
		}
	}
}

func TestEvictsLeastRecentlyUsed(t *testing.T) {
	c, _ := newCache(t, func(http.ResponseWriter, *http.Request) {})
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(c.dir, strings.Repeat("1", 32)+".png")
	recent := filepath.Join(c.dir, strings.Repeat("2", 32)+".png")
	chunk := bytes.Repeat([]byte{1}, maxTotal/2+1)
	for i, p := range []string{old, recent} {
		if err := os.WriteFile(p, chunk, 0o600); err != nil {
			t.Fatal(err)
		}
		when := time.Now().Add(time.Duration(i-2) * time.Hour)
		_ = os.Chtimes(p, when, when)
	}
	c.lru.Added(0)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("least recently used file kept")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatal("recent file evicted")
	}
}
