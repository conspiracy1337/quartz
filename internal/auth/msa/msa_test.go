package msa

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"quartz/internal/core/netx"
)

const testClient = "11111111-2222-3333-4444-555555555555"

// fakeMS is a tiny Microsoft identity endpoint. token decides each /token reply.
type fakeMS struct {
	*httptest.Server
	mu    sync.Mutex
	forms []url.Values
	token func(form url.Values) (int, string)
}

func newFakeMS(t *testing.T) *fakeMS {
	t.Helper()
	f := &fakeMS{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.forms = append(f.forms, r.PostForm)
		f.mu.Unlock()
		if r.PostForm.Get("client_id") != testClient || r.PostForm.Get("scope") != Scope {
			t.Errorf("%s: client or scope missing: %v", r.URL.Path, r.PostForm)
		}
		switch r.URL.Path {
		case "/devicecode":
			_, _ = io.WriteString(w, `{"device_code":"DEV","user_code":"K7QX2M9P","verification_uri":"https://www.microsoft.com/link","expires_in":900,"interval":5}`)
		case "/token":
			code, body := f.token(r.PostForm)
			w.WriteHeader(code)
			_, _ = io.WriteString(w, body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeMS) client(t *testing.T) *Client {
	t.Helper()
	h, err := netx.New(netx.Proxy{Mode: netx.ProxyNone}, netx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	c := New(h, testClient)
	c.Endpoints = Endpoints{Authorize: f.URL + "/authorize", Token: f.URL + "/token", DeviceCode: f.URL + "/devicecode"}
	return c
}

const okToken = `{"access_token":"ACCESS","refresh_token":"REFRESH","expires_in":3600}`

func oauthErr(code string) (int, string) { return 400, `{"error":"` + code + `","error_description":"AADSTS"}` }

func TestBrowserFlow(t *testing.T) {
	f := newFakeMS(t)
	f.token = func(form url.Values) (int, string) {
		if form.Get("grant_type") != "authorization_code" || form.Get("code") != "THECODE" {
			return oauthErr("invalid_request")
		}
		return 200, okToken
	}
	c := f.client(t)
	flow, err := c.StartBrowser()
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(flow.URL())
	q := u.Query()
	redirect, state := q.Get("redirect_uri"), q.Get("state")
	if !strings.HasPrefix(redirect, "http://localhost:") || q.Get("code_challenge_method") != "S256" || len(state) < 43 || q.Get("prompt") != "select_account" {
		t.Fatalf("authorize url: %v", q)
	}
	base := strings.Replace(redirect, "localhost", "127.0.0.1", 1)

	// Wrong state, wrong path and wrong method are all 404 and don't end the flow.
	for _, probe := range []struct{ method, url string }{
		{"GET", base + "?state=nope&code=EVIL"},
		{"GET", base + "favicon.ico?state=" + state + "&code=EVIL"},
		{"POST", base + "?state=" + state + "&code=EVIL"},
	} {
		req, _ := http.NewRequest(probe.method, probe.url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Fatalf("%s %s: %d", probe.method, probe.url, resp.StatusCode)
		}
	}

	resp, err := http.Get(base + "?state=" + url.QueryEscape(state) + "&code=THECODE")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || strings.Contains(string(body), "<script") || resp.Header.Get("Content-Security-Policy") == "" {
		t.Fatalf("callback page: %d %s", resp.StatusCode, body)
	}
	tok, err := flow.Wait(context.Background())
	if err != nil || tok.Access != "ACCESS" || tok.Refresh != "REFRESH" {
		t.Fatalf("token %+v, %v", tok, err)
	}
	// The verifier sent with the code matches the challenge in the authorize URL.
	form := f.forms[len(f.forms)-1]
	sum := sha256.Sum256([]byte(form.Get("code_verifier")))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != q.Get("code_challenge") || form.Get("redirect_uri") != redirect {
		t.Fatalf("pkce mismatch: %v", form)
	}
	// The server is gone after the flow.
	if resp, err := http.Get(base); err == nil {
		_ = resp.Body.Close()
		t.Fatal("loopback server still listening")
	}
}

func TestBrowserFlowDeniedAndCancelled(t *testing.T) {
	f := newFakeMS(t)
	c := f.client(t)
	flow, _ := c.StartBrowser()
	q, _ := url.ParseQuery(strings.SplitN(flow.URL(), "?", 2)[1])
	base := strings.Replace(q.Get("redirect_uri"), "localhost", "127.0.0.1", 1)
	resp, err := http.Get(base + "?state=" + url.QueryEscape(q.Get("state")) + "&error=access_denied")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if _, err := flow.Wait(context.Background()); !errors.Is(err, ErrDenied) {
		t.Fatalf("denied: %v", err)
	}

	flow, _ = c.StartBrowser()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := flow.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestDeviceFlow(t *testing.T) {
	f := newFakeMS(t)
	n := 0
	f.token = func(form url.Values) (int, string) {
		if form.Get("device_code") != "DEV" {
			return oauthErr("invalid_request")
		}
		defer func() { n++ }()
		if n == 0 {
			return oauthErr("authorization_pending")
		}
		if n == 1 {
			return oauthErr("slow_down")
		}
		return 200, okToken
	}
	c := f.client(t)
	var waits []time.Duration
	c.wait = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	dc, err := c.StartDevice(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if dc.UserCode != "K7QX2M9P" || dc.QRURL != "https://www.microsoft.com/link?otc=K7QX2M9P" {
		t.Fatalf("device code: %+v", dc)
	}
	tok, err := c.PollDevice(context.Background(), dc)
	if err != nil || tok.Access != "ACCESS" {
		t.Fatalf("poll: %+v %v", tok, err)
	}
	if len(waits) != 3 || waits[0] != 5*time.Second || waits[2] != 10*time.Second {
		t.Fatalf("intervals %v", waits)
	}
}

func TestDeviceFlowEnds(t *testing.T) {
	for code, want := range map[string]error{"authorization_declined": ErrDenied, "expired_token": ErrExpired} {
		f := newFakeMS(t)
		f.token = func(url.Values) (int, string) { return oauthErr(code) }
		c := f.client(t)
		c.wait = func(context.Context, time.Duration) error { return nil }
		dc, _ := c.StartDevice(context.Background())
		if _, err := c.PollDevice(context.Background(), dc); !errors.Is(err, want) {
			t.Fatalf("%s: %v", code, err)
		}
	}
	// Running past expires_in stops polling without another request.
	f := newFakeMS(t)
	f.token = func(url.Values) (int, string) { return oauthErr("authorization_pending") }
	c := f.client(t)
	now := time.Now()
	c.now = func() time.Time { return now }
	c.wait = func(context.Context, time.Duration) error { now = now.Add(time.Minute); return nil }
	dc, _ := c.StartDevice(context.Background())
	if _, err := c.PollDevice(context.Background(), dc); !errors.Is(err, ErrExpired) {
		t.Fatalf("expiry: %v", err)
	}
}

func TestRefreshErrors(t *testing.T) {
	f := newFakeMS(t)
	c := f.client(t)
	f.token = func(form url.Values) (int, string) {
		if form.Get("refresh_token") != "OLD" {
			return oauthErr("invalid_grant")
		}
		return 200, okToken
	}
	if tok, err := c.Refresh(context.Background(), "OLD"); err != nil || tok.Refresh != "REFRESH" {
		t.Fatalf("refresh: %+v %v", tok, err)
	}
	if _, err := c.Refresh(context.Background(), "REVOKED"); !errors.Is(err, ErrReauth) {
		t.Fatalf("invalid_grant: %v", err)
	}
	f.token = func(url.Values) (int, string) { return 503, `{"error":"temporarily_unavailable"}` }
	_, err := c.Refresh(context.Background(), "OLD")
	if errors.Is(err, ErrReauth) {
		t.Fatal("a server error must not ask for a new sign-in")
	}
	if cls, _ := netx.Classify(err); cls == netx.Permanent {
		t.Fatalf("503 should be transient: %v", err)
	}
	if _, err := New(nil, "").Refresh(context.Background(), "x"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured: %v", err)
	}
	var oe *OAuthError
	f.token = func(url.Values) (int, string) { return oauthErr("invalid_client") }
	if _, err := c.Refresh(context.Background(), "OLD"); !errors.As(err, &oe) || oe.Code != "invalid_client" {
		t.Fatalf("typed oauth error: %v", err)
	}
}
