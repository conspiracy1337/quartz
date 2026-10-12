package msa

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"html"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"quartz/internal/brand"
)

// BrowserTimeout bounds how long the loopback server waits for the redirect.
const BrowserTimeout = 10 * time.Minute

// BrowserFlow is one browser sign-in: a loopback server waiting for the
// redirect, and the authorize URL to open. Close it when done.
type BrowserFlow struct {
	c        *Client
	url      string
	redirect string
	state    string
	verifier string
	srv      *http.Server
	result   chan callback
	once     sync.Once
}

type callback struct {
	code string
	err  error
}

// StartBrowser starts the loopback server and builds the authorize URL. The
// redirect is http://localhost:<port>/ (register "http://localhost" as a
// "Mobile and desktop" redirect in Azure; Microsoft ignores the port).
func (c *Client) StartBrowser() (*BrowserFlow, error) {
	if c.ClientID == "" {
		return nil, ErrNotConfigured
	}
	verifier, state := random(32), random(32)
	f := &BrowserFlow{c: c, state: state, verifier: verifier, result: make(chan callback, 1)}

	// localhost may resolve to either loopback address, so listen on both,
	// on the same port. IPv6 is optional.
	l4, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := l4.Addr().(*net.TCPAddr).Port
	f.redirect = "http://localhost:" + strconv.Itoa(port) + "/"
	f.srv = &http.Server{Handler: http.HandlerFunc(f.serve), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = f.srv.Serve(l4) }()
	if l6, err := net.Listen("tcp6", "[::1]:"+strconv.Itoa(port)); err == nil {
		go func() { _ = f.srv.Serve(l6) }()
	}

	sum := sha256.Sum256([]byte(verifier))
	q := url.Values{
		"client_id":             {c.ClientID},
		"response_type":         {"code"},
		"redirect_uri":          {f.redirect},
		"response_mode":         {"query"},
		"scope":                 {Scope},
		"state":                 {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
		"prompt":                {"select_account"},
	}
	f.url = c.Endpoints.Authorize + "?" + q.Encode()
	return f, nil
}

// URL is the Microsoft sign-in page to open in the system browser.
func (f *BrowserFlow) URL() string { return f.url }

// Wait blocks until the browser comes back, then exchanges the code. It gives
// up after BrowserTimeout or when ctx ends.
func (f *BrowserFlow) Wait(ctx context.Context) (Token, error) {
	defer f.Close()
	timer := time.NewTimer(BrowserTimeout)
	defer timer.Stop()
	var cb callback
	select {
	case <-ctx.Done():
		return Token{}, ctx.Err()
	case <-timer.C:
		return Token{}, ErrTimeout
	case cb = <-f.result:
	}
	if cb.err != nil {
		return Token{}, cb.err
	}
	return f.c.token(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {cb.code},
		"redirect_uri":  {f.redirect},
		"code_verifier": {f.verifier},
	})
}

// Close stops the loopback server. It is safe to call more than once.
func (f *BrowserFlow) Close() {
	f.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = f.srv.Shutdown(ctx)
	})
}

// serve accepts exactly one GET / that carries our state. Everything else is
// a 404, so other local software can't probe or finish the flow.
func (f *BrowserFlow) serve(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if r.Method != http.MethodGet || r.URL.Path != "/" || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(f.state)) != 1 {
		http.NotFound(w, r)
		return
	}
	var cb callback
	switch {
	case q.Get("code") != "":
		cb.code = q.Get("code")
	case q.Get("error") == "access_denied":
		cb.err = ErrDenied
	default:
		cb.err = &OAuthError{Code: q.Get("error"), Description: q.Get("error_description")}
	}
	select {
	case f.result <- cb:
	default: // already answered; the first redirect wins
	}
	page(w, cb.err == nil)
}

// page is the static, script-free reply shown in the browser tab.
func page(w http.ResponseWriter, ok bool) {
	msg := "Signed in. You can close this tab and go back to " + brand.Name + "."
	if !ok {
		msg = "Sign-in didn't finish. You can close this tab and try again in " + brand.Name + "."
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>` + html.EscapeString(brand.Name) +
		`</title><body style="font:15px system-ui,sans-serif;background:#0b0b0d;color:#e8e8ea;display:grid;place-items:center;height:100vh;margin:0"><p>` +
		html.EscapeString(msg) + `</p>`))
}

// random returns n random bytes, base64url encoded (43 characters for 32).
func random(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // never fails since Go 1.24
	return base64.RawURLEncoding.EncodeToString(b)
}
