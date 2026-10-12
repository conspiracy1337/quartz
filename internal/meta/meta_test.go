package meta

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"quartz/internal/core/httpcache"
)

// failingDoer counts requests and always fails.
type failingDoer struct{ n int }

func (d *failingDoer) Do(*http.Request) (*http.Response, error) {
	d.n++
	return nil, errors.New("network down")
}

func TestBackoffLadder(t *testing.T) {
	d := &failingDoer{}
	c, err := httpcache.Open(t.TempDir(), d, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	s := New(c)
	now := time.Now()
	s.now = func() time.Time { return now }
	ctx := context.Background()
	if _, err := s.Manifest(ctx); err == nil {
		t.Fatal("expected failure")
	}
	if _, err := s.Manifest(ctx); err == nil || d.n != 1 {
		t.Fatalf("second call inside the 1 s window hit the network (%d requests)", d.n)
	}
	now = now.Add(1100 * time.Millisecond)
	_, _ = s.Manifest(ctx)
	if d.n != 2 {
		t.Fatalf("call after the window did not retry (%d requests)", d.n)
	}
	now = now.Add(2 * time.Second)
	_, _ = s.Manifest(ctx)
	if d.n != 2 {
		t.Fatal("second failure must back off for 3 s")
	}
}

// bodyDoer answers every request with a fixed body.
type bodyDoer struct {
	body string
	n    int
}

func (d *bodyDoer) Do(r *http.Request) (*http.Response, error) {
	d.n++
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(d.body)), Request: r}, nil
}

func TestSidecarSHA1(t *testing.T) {
	for body, want := range map[string]string{
		"0123456789abcdef0123456789abcdef01234567":                        "0123456789abcdef0123456789abcdef01234567",
		"0123456789ABCDEF0123456789ABCDEF01234567  forge-installer.jar\n": "0123456789abcdef0123456789abcdef01234567",
		"<html>not found</html>":                                          "",
	} {
		d := &bodyDoer{body: body}
		c, err := httpcache.Open(t.TempDir(), d, slog.New(slog.DiscardHandler))
		if err != nil {
			t.Fatal(err)
		}
		got, err := New(c).SidecarSHA1(context.Background(), "https://maven.example/a.jar")
		if got != want || (want == "") != (err != nil) {
			t.Errorf("SidecarSHA1(%q) = %q, %v; want %q", body, got, err, want)
		}
	}
}
