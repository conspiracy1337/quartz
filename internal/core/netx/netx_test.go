package netx

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newClient(t *testing.T, o Options) *Client {
	t.Helper()
	c, err := New(Proxy{Mode: ProxyNone}, o)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func tlsOptions(srv *httptest.Server) Options {
	return Options{TLS: &tls.Config{RootCAs: srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs, MinVersion: tls.VersionTLS12}}
}

func TestRefusesHTTPSDowngrade(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "plain") }))
	defer plain.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	defer secure.Close()
	c := newClient(t, tlsOptions(secure))
	err := getErr(c, secure.URL)
	if !errors.Is(err, ErrInsecureRedirect) {
		t.Fatalf("err = %v", err)
	}
	if cls, _ := Classify(err); cls != Permanent {
		t.Fatalf("class = %v", cls)
	}
}

func TestRedirectLimit(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/again", http.StatusFound)
	}))
	defer srv.Close()
	err := getErr(newClient(t, Options{}), srv.URL)
	if !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("err = %v", err)
	}
}

func TestRateLimitGateAndClassify(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()
	c := newClient(t, Options{})
	err := getErr(c, srv.URL)
	cls, after := Classify(err)
	if cls != RateLimited || after != time.Second {
		t.Fatalf("class=%v after=%v err=%v", cls, after, err)
	}
	start := time.Now()
	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if waited := time.Since(start); waited < 900*time.Millisecond {
		t.Fatalf("host gate not honoured, waited %v", waited)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Duration{
		"":                              0,
		"30":                            30 * time.Second,
		"-5":                            0,
		"99999":                         maxRateWait,
		"Mon, 12 Oct 2026 12:00:45 GMT": 45 * time.Second,
		"garbage":                       0,
	}
	for in, want := range cases {
		if got := parseRetryAfter(in, now); got != want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestClassifyStatuses(t *testing.T) {
	cases := map[int]Class{404: Permanent, 403: Permanent, 410: Permanent, 408: Retryable, 425: Retryable, 500: Retryable, 502: Retryable, 503: Retryable, 429: RateLimited}
	for code, want := range cases {
		if got, _ := Classify(&StatusError{Code: code}); got != want {
			t.Errorf("Classify(%d) = %v, want %v", code, got, want)
		}
	}
	if got, _ := Classify(&StatusError{Code: 503, RetryAfter: time.Second}); got != RateLimited {
		t.Errorf("503 with Retry-After = %v", got)
	}
	if got, _ := Classify(&PermanentError{Err: errors.New("hash")}); got != Permanent {
		t.Errorf("PermanentError = %v", got)
	}
	if got, _ := Classify(context.Canceled); got != Permanent {
		t.Errorf("cancel = %v", got)
	}
}

func TestIdleTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "partial")
		w.(http.Flusher).Flush()
		<-release
	}))
	defer srv.Close()
	defer close(release)
	resp, err := newClient(t, Options{IdleTimeout: 50 * time.Millisecond}).Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, err = io.ReadAll(resp.Body)
	if !errors.Is(err, ErrIdleTimeout) {
		t.Fatalf("err = %v", err)
	}
	if cls, _ := Classify(err); cls != Retryable {
		t.Fatalf("class = %v", cls)
	}
}

type keyInjector struct{ host string }

func (k keyInjector) Headers(host string) http.Header {
	if host == k.host {
		return http.Header{"X-Api-Key": {"secret"}}
	}
	return nil
}

func TestHeadersOnlyForTheirHost(t *testing.T) {
	var gotOther, gotUA string
	other := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotOther = r.Header.Get("X-Api-Key")
		gotUA = r.Header.Get("User-Agent")
	}))
	defer other.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "secret" {
			t.Error("key missing on its own host")
		}
		http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1), http.StatusFound)
	}))
	defer api.Close()
	c := newClient(t, Options{Injectors: []HeaderInjector{keyInjector{host: "127.0.0.1"}}})
	resp, err := c.Get(context.Background(), api.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if gotOther != "" {
		t.Fatal("API key leaked to another host on redirect")
	}
	if gotUA != UserAgent {
		t.Fatalf("user agent = %q", gotUA)
	}
}

func TestCustomHTTPProxy(t *testing.T) {
	var auth, target string
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, target = r.Header.Get("Proxy-Authorization"), r.URL.String()
		_, _ = io.WriteString(w, "via proxy")
	}))
	defer proxySrv.Close()
	host, port := splitHostPort(t, proxySrv.URL)
	c, err := New(Proxy{Mode: ProxyCustom, Type: "http", Host: host, Port: port, User: "u", Password: "p"}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(context.Background(), "http://example.invalid/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "via proxy" || target != "http://example.invalid/x" || auth == "" {
		t.Fatalf("body=%q target=%q auth=%q", body, target, auth)
	}
}

func TestUnknownProxyType(t *testing.T) {
	if _, err := New(Proxy{Mode: ProxyCustom, Type: "ftp", Host: "h", Port: 1}, Options{}); err == nil {
		t.Fatal("unknown proxy type accepted")
	}
}

func splitHostPort(t *testing.T, raw string) (string, int) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return u.Hostname(), p
}

// getErr performs a GET that is expected to fail and returns its error.
func getErr(c *Client, url string) error {
	resp, err := c.Get(context.Background(), url, nil)
	if err == nil {
		_ = resp.Body.Close()
	}
	return err
}
