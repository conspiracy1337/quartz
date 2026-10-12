package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"quartz/internal/auth/msa"
	"quartz/internal/auth/xbox"
	"quartz/internal/core/netx"
	"quartz/internal/mcapi"
)

// fakeServices answers every step of a sign-in on one server.
func fakeServices(t *testing.T, xstsFail string) *Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_, _ = io.WriteString(w, `{"access_token":"MSA","refresh_token":"R2","expires_in":3600}`)
		case "/user":
			_, _ = io.WriteString(w, `{"Token":"XBL","DisplayClaims":{"xui":[{"uhs":"U"}]}}`)
		case "/xsts":
			if xstsFail != "" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"XErr":`+xstsFail+`}`)
				return
			}
			_, _ = io.WriteString(w, `{"Token":"XSTS","DisplayClaims":{"xui":[{"uhs":"U"}]}}`)
		case "/authentication/login_with_xbox":
			_, _ = io.WriteString(w, `{"access_token":"MC","expires_in":86400}`)
		case "/minecraft/profile":
			_ = json.NewEncoder(w).Encode(mcapi.Profile{ID: "069a79f444e94726a5befca90e38aaf5", Name: "Notch"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	h, err := netx.New(netx.Proxy{Mode: netx.ProxyNone}, netx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := New(h, "11111111-2222-3333-4444-555555555555")
	s.MSA.Endpoints = msa.Endpoints{Token: srv.URL + "/token"}
	s.Xbox.Endpoints = xbox.Endpoints{User: srv.URL + "/user", XSTS: srv.URL + "/xsts"}
	s.MC.Endpoints = mcapi.Endpoints{Services: srv.URL}
	return s
}

func TestRefreshRunsEveryStep(t *testing.T) {
	s := fakeServices(t, "")
	var steps []string
	m, err := s.MSA.Refresh(context.Background(), "R1")
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Complete(context.Background(), m, func(st string) { steps = append(steps, st) })
	if err != nil || res.MSA.Refresh != "R2" || res.MC.Access != "MC" || res.Profile.Name != "Notch" {
		t.Fatalf("%+v %v", res, err)
	}
	if strings.Join(steps, ",") != StepXbox+","+StepMinecraft+","+StepProfile {
		t.Fatalf("steps %v", steps)
	}
}

func TestXboxRefusalNeedsReauth(t *testing.T) {
	_, err := fakeServices(t, "2148916238").Refresh(context.Background(), "R1")
	if !NeedsReauth(err) || Transient(err) {
		t.Fatalf("child account: %v", err)
	}
	if p := Describe(err); p.Reason != "child-account" || p.FixURL == "" {
		t.Fatalf("describe: %+v", p)
	}
}

func TestDescribeAndClassify(t *testing.T) {
	cases := []struct {
		err       error
		reason    string
		reauth    bool
		transient bool
	}{
		{msa.ErrNotConfigured, "not-configured", false, false},
		{fmt.Errorf("x: %w", msa.ErrReauth), "reauth", true, false},
		{msa.ErrDenied, "cancelled", false, false},
		{context.Canceled, "cancelled", false, false},
		{msa.ErrExpired, "expired", false, false},
		{mcapi.ErrNotOwned, "not-owned", true, false},
		{mcapi.ErrNoProfile, "no-profile", true, false},
		{&netx.StatusError{Code: 429}, "rate-limited", false, true},
		{&netx.StatusError{Code: 503}, "network", false, true},
		{errors.New("connection reset"), "network", false, true},
		{&netx.StatusError{Code: 400}, "failed", false, false},
	}
	for _, c := range cases {
		if p := Describe(c.err); p.Reason != c.reason || p.Message == "" {
			t.Errorf("%v: %+v", c.err, p)
		}
		if NeedsReauth(c.err) != c.reauth || Transient(c.err) != c.transient {
			t.Errorf("%v: reauth=%v transient=%v", c.err, NeedsReauth(c.err), Transient(c.err))
		}
	}
}
