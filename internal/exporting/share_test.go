package exporting

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"quartz/internal/content"
	"quartz/internal/core/netx"
)

func shareEnv(t *testing.T, h http.HandlerFunc) *env {
	t.Helper()
	e := newEnv(t)
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	hc, err := netx.New(netx.Proxy{Mode: netx.ProxyNone}, netx.Options{TLS: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}})
	if err != nil {
		t.Fatal(err)
	}
	e.svc.d.HTTP, e.svc.d.shareURL = hc, srv.URL+"/api/modlists"
	e.items = manifest{{Kind: content.Mods, Rel: "mods/a.jar", Enabled: true, Name: "Sodium", Modrinth: mr("AANobbMI")}}
	return e
}

func TestShare(t *testing.T) {
	var got ModList
	e := shareEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/modlists" || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"code":"K7QXM","expiresAt":"`+time.Now().Add(ShareTTL).UTC().Format(time.RFC3339)+`"}`)
	})
	c, err := e.svc.Share(context.Background(), e.in.ID)
	if err != nil || c.Code != "K7QXM" || c.ExpiresAt <= time.Now().UnixMilli() {
		t.Fatalf("share = %+v, %v", c, err)
	}
	if len(got.Mods) != 1 || got.Mods[0].Project != "AANobbMI" || got.Name != "Survival" {
		t.Fatalf("sent %+v", got)
	}
}

func TestShareRefusesOddAnswers(t *testing.T) {
	soon := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	for name, body := range map[string]string{
		"lower case":   `{"code":"k7qxm","expiresAt":"` + soon + `"}`,
		"too long":     `{"code":"K7QXMA","expiresAt":"` + soon + `"}`,
		"expired":      `{"code":"K7QXM","expiresAt":"2020-01-01T00:00:00Z"}`,
		"a year":       `{"code":"K7QXM","expiresAt":"` + time.Now().AddDate(1, 0, 0).UTC().Format(time.RFC3339) + `"}`,
		"huge":         `{"code":"K7QXM","pad":"` + strings.Repeat("x", 8<<10) + `"}`,
		"server error": "",
	} {
		e := shareEnv(t, func(w http.ResponseWriter, _ *http.Request) {
			if body == "" {
				http.Error(w, "down", http.StatusInternalServerError)
				return
			}
			_, _ = io.WriteString(w, body)
		})
		if _, err := e.svc.Share(context.Background(), e.in.ID); !errors.Is(err, ErrShare) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
