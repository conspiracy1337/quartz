package meta

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
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
