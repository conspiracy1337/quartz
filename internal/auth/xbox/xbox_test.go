package xbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"quartz/internal/core/netx"
)

func fake(t *testing.T, xsts func(w http.ResponseWriter)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/user":
			props, _ := body["Properties"].(map[string]any)
			if r.Header.Get("X-Xbl-Contract-Version") != "1" || props["RpsTicket"] != "d=MSA" {
				t.Errorf("user request: %v %v", r.Header, body)
			}
			_, _ = io.WriteString(w, `{"Token":"XBL","DisplayClaims":{"xui":[{"uhs":"UHS1"}]}}`)
		case "/xsts":
			if body["RelyingParty"] != RelyingParty {
				t.Errorf("relying party: %v", body["RelyingParty"])
			}
			xsts(w)
		}
	}))
	t.Cleanup(srv.Close)
	h, err := netx.New(netx.Proxy{Mode: netx.ProxyNone}, netx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return &Client{HTTP: h, Endpoints: Endpoints{User: srv.URL + "/user", XSTS: srv.URL + "/xsts"}}
}

func TestAuthenticate(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter) {
		_, _ = io.WriteString(w, `{"Token":"XSTS","NotAfter":"2030-01-01T00:00:00Z","DisplayClaims":{"xui":[{"uhs":"UHS1"}]}}`)
	})
	tok, err := c.Authenticate(context.Background(), "MSA")
	if err != nil || tok.Token != "XSTS" || tok.UHS != "UHS1" || tok.NotAfter.Year() != 2030 {
		t.Fatalf("%+v %v", tok, err)
	}
}

func TestUHSMismatch(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter) {
		_, _ = io.WriteString(w, `{"Token":"XSTS","DisplayClaims":{"xui":[{"uhs":"OTHER"}]}}`)
	})
	if _, err := c.Authenticate(context.Background(), "MSA"); !errors.Is(err, ErrUHSMismatch) {
		t.Fatalf("err = %v", err)
	}
}

func TestXErrCodes(t *testing.T) {
	cases := map[string]struct {
		reason string
		fix    bool
	}{
		"2148916227": {"banned", true},
		"2148916229": {"parental-online", true},
		"2148916233": {"no-xbox-profile", true},
		"2148916234": {"xbox-terms", true},
		"2148916235": {"region", false},
		"2148916236": {"age-verification", true},
		"2148916237": {"age-verification", true},
		"2148916238": {"child-account", true},
		"2148916999": {"xbox", false},
	}
	for code, want := range cases {
		c := fake(t, func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"Identity":"0","XErr":`+code+`,"Message":"","Redirect":"https://start.ui.xboxlive.com/AddChildToFamily"}`)
		})
		_, err := c.Authenticate(context.Background(), "MSA")
		var xe *Error
		if !errors.As(err, &xe) || xe.Reason != want.reason || (xe.FixURL != "") != want.fix || xe.Message == "" {
			t.Fatalf("%s: %+v (%v)", code, xe, err)
		}
	}
	// A 401 without XErr, and a 5xx, stay plain network errors.
	c := fake(t, func(w http.ResponseWriter) { w.WriteHeader(http.StatusBadGateway) })
	_, err := c.Authenticate(context.Background(), "MSA")
	var xe *Error
	if errors.As(err, &xe) {
		t.Fatalf("5xx became an Xbox error: %v", err)
	}
	if cls, _ := netx.Classify(err); cls != netx.Retryable {
		t.Fatalf("5xx class %v", cls)
	}
}
