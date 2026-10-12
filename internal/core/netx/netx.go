// Package netx owns the launcher's single HTTP client: timeouts, proxy, safe
// redirects, per-host headers, per-host rate-limit gates and error
// classification. Every network call in the launcher goes through it.
package netx

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/proxy"

	"quartz/internal/brand"
)

// UserAgent identifies the launcher to every host, as Modrinth and Mojang ask.
var UserAgent = brand.Slug + "-launcher/" + brand.Version

// Errors with fixed meaning for Classify.
var (
	ErrInsecureRedirect = errors.New("netx: refusing redirect from https to http")
	ErrTooManyRedirects = errors.New("netx: too many redirects")
	ErrIdleTimeout      = errors.New("netx: connection stalled")
)

const (
	maxRedirects       = 10
	defaultIdleTimeout = 60 * time.Second
	defaultRateWait    = 10 * time.Second
	maxRateWait        = 10 * time.Minute
)

// ProxyMode mirrors settings.Downloads.Proxy.
type ProxyMode string

const (
	ProxySystem ProxyMode = "system"
	ProxyNone   ProxyMode = "none"
	ProxyCustom ProxyMode = "custom"
)

// Proxy is a fully resolved proxy configuration. Password comes from the
// keychain and is never stored with the settings.
type Proxy struct {
	Mode     ProxyMode
	Type     string // http | https | socks5
	Host     string
	Port     int
	User     string
	Password string
}

// HeaderInjector adds headers for one host, e.g. an API key. Headers are added
// per request hop, so a redirect to another host never carries them.
type HeaderInjector interface {
	Headers(host string) http.Header
}

// Options tune the client; the zero value is production-ready.
type Options struct {
	IdleTimeout time.Duration // a body read stalling this long fails; default 60 s
	TLS         *tls.Config   // tests inject their CA here
	Injectors   []HeaderInjector
}

// Client is safe for concurrent use. Configure swaps the transport live.
type Client struct {
	o     Options
	hc    atomic.Pointer[http.Client]
	gates gates
}

// New builds a client with the given proxy.
func New(p Proxy, o Options) (*Client, error) {
	if o.IdleTimeout == 0 {
		o.IdleTimeout = defaultIdleTimeout
	}
	c := &Client{o: o, gates: gates{until: map[string]time.Time{}}}
	if err := c.Configure(p); err != nil {
		return nil, err
	}
	return c, nil
}

// Configure rebuilds the transport for a new proxy configuration.
func (c *Client) Configure(p Proxy) error {
	tr, err := c.transport(p)
	if err != nil {
		return err
	}
	next := &http.Client{
		Transport:     &headerTransport{next: tr, injectors: c.o.Injectors},
		CheckRedirect: checkRedirect,
	}
	if old := c.hc.Swap(next); old != nil {
		old.CloseIdleConnections()
	}
	return nil
}

func (c *Client) transport(p Proxy) (*http.Transport, error) {
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	tr := &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       c.o.TLS,
	}
	switch p.Mode {
	case ProxyNone:
	case ProxyCustom:
		addr := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
		switch p.Type {
		case "http", "https":
			u := &url.URL{Scheme: p.Type, Host: addr}
			if p.User != "" {
				u.User = url.UserPassword(p.User, p.Password)
			}
			tr.Proxy = http.ProxyURL(u)
		case "socks5":
			var auth *proxy.Auth
			if p.User != "" {
				auth = &proxy.Auth{User: p.User, Password: p.Password}
			}
			d, err := proxy.SOCKS5("tcp", addr, auth, dialer)
			if err != nil {
				return nil, err
			}
			cd, ok := d.(proxy.ContextDialer)
			if !ok {
				return nil, errors.New("netx: socks5 dialer lacks context support")
			}
			tr.DialContext = cd.DialContext
		default:
			return nil, fmt.Errorf("netx: unknown proxy type %q", p.Type)
		}
	default:
		tr.Proxy = http.ProxyFromEnvironment
	}
	return tr, nil
}

// checkRedirect follows at most 10 hops and never downgrades https to http.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return ErrTooManyRedirects
	}
	if req.URL.Scheme != "https" && via[0].URL.Scheme == "https" {
		return ErrInsecureRedirect
	}
	return nil
}

// headerTransport sets the User-Agent and the per-host injected headers on
// every hop, including redirects.
type headerTransport struct {
	next      http.RoundTripper
	injectors []HeaderInjector
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header.Set("User-Agent", UserAgent)
	for _, inj := range t.injectors {
		for k, vs := range inj.Headers(r.URL.Hostname()) {
			r.Header[k] = vs
		}
	}
	return t.next.RoundTrip(r)
}

// StatusError is returned for any response that is not 2xx or 304.
type StatusError struct {
	Code       int
	URL        string
	RetryAfter time.Duration // parsed Retry-After, 0 if absent
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s returned %d %s", e.URL, e.Code, http.StatusText(e.Code))
}

// Do sends req after waiting out any rate-limit gate on its host. Non-success
// statuses come back as *StatusError with the body closed. The returned body
// fails with ErrIdleTimeout when it stalls for longer than the idle timeout.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	if err := c.gates.wait(req.Context(), host); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancelCause(req.Context())
	resp, err := c.hc.Load().Do(req.WithContext(ctx))
	if err != nil {
		cancel(nil)
		return nil, err
	}
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotModified {
		se := &StatusError{Code: resp.StatusCode, URL: redactURL(req.URL), RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		cancel(nil)
		if se.Code == http.StatusTooManyRequests || (se.Code == http.StatusServiceUnavailable && se.RetryAfter > 0) {
			c.gates.close(host, se.RetryAfter)
		}
		return nil, se
	}
	resp.Body = newIdleBody(resp.Body, ctx, cancel, c.o.IdleTimeout)
	return resp, nil
}

// Get is a convenience GET with optional extra headers.
func (c *Client) Get(ctx context.Context, rawURL string, h http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, vs := range h {
		req.Header[k] = vs
	}
	return c.Do(req)
}

// redactURL drops the query string and user info, which may carry tokens.
func redactURL(u *url.URL) string {
	cp := *u
	cp.RawQuery, cp.User, cp.Fragment = "", nil, ""
	return cp.String()
}

// parseRetryAfter accepts delta-seconds or an HTTP date, capped at 10 minutes.
func parseRetryAfter(v string, now time.Time) time.Duration {
	if v == "" {
		return 0
	}
	var d time.Duration
	if secs, err := strconv.Atoi(v); err == nil {
		d = time.Duration(secs) * time.Second
	} else if t, err := http.ParseTime(v); err == nil {
		d = t.Sub(now)
	}
	return min(max(d, 0), maxRateWait)
}

// gates hold rate-limited hosts closed until their Retry-After passes.
type gates struct {
	mu    sync.Mutex
	until map[string]time.Time
}

func (g *gates) close(host string, d time.Duration) {
	if d <= 0 {
		d = defaultRateWait
	}
	g.mu.Lock()
	if t := time.Now().Add(d); t.After(g.until[host]) {
		g.until[host] = t
	}
	g.mu.Unlock()
}

func (g *gates) wait(ctx context.Context, host string) error {
	g.mu.Lock()
	d := time.Until(g.until[host])
	g.mu.Unlock()
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// idleBody cancels the request when no byte arrives for the idle timeout.
type idleBody struct {
	rc     io.ReadCloser
	ctx    context.Context
	cancel context.CancelCauseFunc
	timer  *time.Timer
	idle   time.Duration
}

func newIdleBody(rc io.ReadCloser, ctx context.Context, cancel context.CancelCauseFunc, idle time.Duration) *idleBody {
	b := &idleBody{rc: rc, ctx: ctx, cancel: cancel, idle: idle}
	b.timer = time.AfterFunc(idle, func() { cancel(ErrIdleTimeout) })
	return b
}

func (b *idleBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if n > 0 {
		b.timer.Reset(b.idle)
	}
	if err != nil && !errors.Is(err, io.EOF) && errors.Is(context.Cause(b.ctx), ErrIdleTimeout) {
		err = ErrIdleTimeout
	}
	return n, err
}

func (b *idleBody) Close() error {
	b.timer.Stop()
	err := b.rc.Close()
	b.cancel(nil)
	return err
}
