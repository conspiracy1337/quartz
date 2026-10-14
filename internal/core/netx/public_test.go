package netx

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"testing"
)

func TestPublic(t *testing.T) {
	for addr, want := range map[string]bool{
		"1.1.1.1": true, "2606:4700::1111": true,
		"127.0.0.1": false, "10.0.0.1": false, "172.16.5.4": false, "192.168.1.1": false, "169.254.169.254": false,
		"100.64.0.1": false, "0.0.0.0": false, "0.1.2.3": false, "224.0.0.1": false,
		"::1": false, "fe80::1": false, "fd00::1": false, "::": false, "::ffff:127.0.0.1": false, "::ffff:8.8.8.8": true,
	} {
		if got := public(netip.MustParseAddr(addr)); got != want {
			t.Errorf("public(%s) = %v, want %v", addr, got, want)
		}
	}
}

func TestPublicOnlyRefusesLocalHosts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "local") }))
	defer srv.Close()
	c := newClient(t, Options{})
	// Unmarked requests reach it as before.
	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	u, _ := url.Parse(srv.URL)
	for _, raw := range []string{srv.URL, "http://localhost:" + u.Port(), "http://10.1.2.3/x", "http://[::1]:" + u.Port()} {
		err := getErrCtx(PublicOnly(context.Background()), c, raw)
		if !errors.Is(err, ErrNotPublic) {
			t.Errorf("%s: err = %v, want ErrNotPublic", raw, err)
		}
		if cls, _ := Classify(err); cls != Permanent {
			t.Errorf("%s: class = %v", raw, cls)
		}
	}
}

func TestPublicOnlyRedirect(t *testing.T) {
	req, err := http.NewRequestWithContext(PublicOnly(context.Background()), http.MethodGet, "https://192.168.0.1/admin", nil)
	if err != nil {
		t.Fatal(err)
	}
	prev, _ := http.NewRequest(http.MethodGet, "https://example.org/pack.mrpack", nil)
	if err := checkRedirect(req, []*http.Request{prev}); !errors.Is(err, ErrNotPublic) {
		t.Fatalf("redirect to a private address: %v", err)
	}
}

// A name that passed the check but connects to a private address (DNS
// rebinding) is caught as it is dialled; the proxy itself may be local.
func TestGuardDial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	addr := ln.Addr().String()
	ctx := PublicOnly(context.Background())
	if _, err := guardDial(&net.Dialer{}, nil)(ctx, "tcp", addr); !errors.Is(err, ErrNotPublic) {
		t.Fatalf("dial %s: %v", addr, err)
	}
	c, err := guardDial(&net.Dialer{}, map[string]bool{addr: true})(ctx, "tcp", addr)
	if err != nil {
		t.Fatalf("the proxy was refused: %v", err)
	}
	_ = c.Close()
}

func TestPublicOnlyThroughProxy(t *testing.T) {
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "via proxy") }))
	defer proxySrv.Close()
	host, port := splitHostPort(t, proxySrv.URL)
	c, err := New(Proxy{Mode: ProxyCustom, Type: "http", Host: host, Port: port}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := PublicOnly(context.Background())
	resp, err := c.Get(ctx, "http://93.184.215.14/pack.zip", nil)
	if err != nil {
		t.Fatalf("a public target through a local proxy: %v", err)
	}
	_ = resp.Body.Close()
	if err := getErrCtx(ctx, c, "http://10.1.2.3/pack.zip"); !errors.Is(err, ErrNotPublic) {
		t.Fatalf("a private target through the proxy: %v", err)
	}
}

func TestProxyAddr(t *testing.T) {
	for raw, want := range map[string]string{
		"": "", "http://proxy:3128": "proxy:3128", "proxy.lan": "proxy.lan:80", "https://p": "p:443", "socks5://127.0.0.1": "127.0.0.1:1080",
	} {
		if got := proxyAddr(raw); got != want {
			t.Errorf("proxyAddr(%q) = %q, want %q", raw, got, want)
		}
	}
}

// getErrCtx performs a GET with ctx that is expected to fail.
func getErrCtx(ctx context.Context, c *Client, url string) error {
	resp, err := c.Get(ctx, url, nil)
	if err == nil {
		_ = resp.Body.Close()
	}
	return err
}
