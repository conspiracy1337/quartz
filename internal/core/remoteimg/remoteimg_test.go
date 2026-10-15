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
		"https://i.imgur.com/abc.png":                           true,
		"https://github.com/user-attachments/assets/1-2-3":      true,
		"https://github.com/someone/repo/raw/main/x.png":        false,
		"https://github.com/user-attachments/../login":          false,
		// Object storage: buckets on each service, never look-alikes or the
		// provider's other (arbitrary) servers.
		"https://my-bucket.s3.amazonaws.com/a.png":                        true,
		"https://s3.amazonaws.com/my-bucket/a.png":                        true,
		"https://my.bucket.s3.eu-west-1.amazonaws.com/a.png":              true,
		"https://my-bucket.s3-eu-west-1.amazonaws.com/a.png":              true,
		"https://s3.dualstack.us-east-1.amazonaws.com/b/a.png":            true,
		"https://pub-0123abcd.r2.dev/a.png":                               true,
		"https://acct.r2.cloudflarestorage.com/b/a.png?X-Amz-Signature=1": true,
		"https://storage.googleapis.com/b/a.png":                          true,
		"https://b.storage.googleapis.com/a.png":                          true,
		"https://firebasestorage.googleapis.com/v0/b/x/o/a.png":           true,
		"https://acct.blob.core.windows.net/c/a.png":                      true,
		"https://f003.backblazeb2.com/file/b/a.png":                       true,
		"https://b.nyc3.digitaloceanspaces.com/a.png":                     true,
		"https://s3.eu-central-1.wasabisys.com/b/a.png":                   true,
		"https://b.us-east-1.linodeobjects.com/a.png":                     true,
		"https://b.fsn1.your-objectstorage.com/a.png":                     true,
		"https://ewr1.vultrobjects.com/b/a.png":                           true,
		"https://b.s3.fr-par.scw.cloud/a.png":                             true,
		"https://s3.gra.io.cloud.ovh.net/b/a.png":                         true,
		"https://objectstorage.eu-frankfurt-1.oraclecloud.com/n/a.png":    true,
		"https://abc123.execute-api.us-east-1.amazonaws.com/a.png":        false,
		"https://ec2-1-2-3-4.compute-1.amazonaws.com/a.png":               false,
		"https://x.functions.fnc.fr-par.scw.cloud/a.png":                  false,
		"https://evilr2.dev/a.png":                                        false,
		"https://pub-1.r2.dev.evil.example/a.png":                         false,
		"https://b.s3.amazonaws.com.evil.example/a.png":                   false,
		"http://pub-1.r2.dev/a.png":                                       false,
		"https://pub-1.r2.dev:8443/a.png":                                 false,
		"https://user@pub-1.r2.dev/a.png":                                 false,
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

func TestRedirectMustStayAllowed(t *testing.T) {
	var reached atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Store(true)
		_, _ = w.Write(webp)
	}))
	t.Cleanup(other.Close)
	c, srv := newCache(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/x.webp", http.StatusFound)
	})
	tok := c.Token(srv.URL + "/pic")
	if _, err := c.Path(context.Background(), tok); err == nil || reached.Load() {
		t.Fatalf("followed a redirect to a host that isn't allowed (err %v)", err)
	}
}

// A picture that failed isn't fetched again on every render, only once it
// may have changed.
func TestFailuresWait(t *testing.T) {
	var hits atomic.Int32
	c, srv := newCache(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/gone" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("<html>"))
	})
	now := time.Now()
	c.now = func() time.Time { return now }
	ctx := context.Background()
	for _, path := range []string{"/gone", "/html"} {
		tok := c.Token(srv.URL + path)
		for range 3 {
			if _, err := c.Path(ctx, tok); err == nil {
				t.Fatalf("%s was served", path)
			}
		}
	}
	if hits.Load() != 2 {
		t.Fatalf("hits = %d, want one per picture", hits.Load())
	}
	now = now.Add(retryPermanent + time.Second)
	_, _ = c.Path(ctx, c.Token(srv.URL+"/gone"))
	if hits.Load() != 3 {
		t.Fatalf("hits = %d: a failure kept counting after its wait", hits.Load())
	}
}
