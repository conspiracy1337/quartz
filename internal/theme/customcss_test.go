package theme

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quartz/internal/core/events"
)

func TestCustomCSSReloadsOnSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.css")
	if err := os.WriteFile(path, []byte(":root{--accent:#fff}"), 0o600); err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus()
	got := make(chan CSSState, 8)
	bus.Subscribe(events.CustomCSS, func(p any) { got <- p.(CSSState) })
	c := NewCustomCSS(bus, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer c.Close()

	c.Configure(true, path)
	if s := <-got; s.CSS != ":root{--accent:#fff}" {
		t.Fatalf("initial load: %+v", s)
	}
	// Save the way editors do: write elsewhere, rename over.
	tmp := filepath.Join(dir, "theme.css.tmp")
	if err := os.WriteFile(tmp, []byte(":root{--accent:#000}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-got:
		if !strings.Contains(s.CSS, "#000") {
			t.Fatalf("reload: %+v", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no reload after save")
	}
}

func TestCustomCSSRejects(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.css")
	if err := os.WriteFile(big, make([]byte, MaxCSSSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"relative.css", filepath.Join(dir, "x.txt"), big} {
		if s := load(p); s.Error == "" || s.CSS != "" {
			t.Errorf("load(%q) accepted: %+v", p, s)
		}
	}
}
