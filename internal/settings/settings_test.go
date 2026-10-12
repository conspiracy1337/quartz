package settings

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"quartz/internal/core/events"
)

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestNormalizeFixesFieldByField(t *testing.T) {
	s := Defaults()
	s.Appearance.Accent = "red;}body{x"
	s.Appearance.Theme = "neon"
	s.Appearance.Density = "compact"
	s.Downloads.Parallel = 99
	s.Java.MemoryMinMB, s.Java.MemoryMaxMB = 4096, 1024
	s.General.Language = "../../etc"
	got := s.Normalize()
	d := Defaults()
	if got.Appearance.Accent != d.Appearance.Accent || got.Appearance.Theme != d.Appearance.Theme {
		t.Errorf("invalid appearance kept: %+v", got.Appearance)
	}
	if got.Appearance.Density != "compact" {
		t.Errorf("valid field reset: %q", got.Appearance.Density)
	}
	if got.Downloads.Parallel != 16 {
		t.Errorf("parallel = %d, want clamp to 16", got.Downloads.Parallel)
	}
	if got.Java.MemoryMaxMB < got.Java.MemoryMinMB {
		t.Errorf("max < min: %+v", got.Java)
	}
	if got.General.Language != "system" {
		t.Errorf("language = %q", got.General.Language)
	}
}

func TestDecodeTolerant(t *testing.T) {
	got, err := decode([]byte(`{"appearance":{"accent":"#60a5fa"},"unknown":{"x":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.Appearance.Accent != "#60a5fa" || got.Appearance.Theme != "monolith" || got.Downloads.Parallel != 8 {
		t.Fatalf("got %+v", got)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	bus := events.NewBus()
	var changed int
	bus.Subscribe(events.SettingsChanged, func(any) { changed++ })
	s, err := Open(path, bus, discard)
	if err != nil {
		t.Fatal(err)
	}
	next := s.Get()
	next.Appearance.Theme = "oled"
	s.Replace(next)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(path, bus, discard)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Get().Appearance.Theme != "oled" || changed != 1 {
		t.Fatalf("theme=%q changed=%d", s2.Get().Appearance.Theme, changed)
	}
}

func TestStoreCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path, events.NewBus(), discard)
	if err != nil {
		t.Fatal(err)
	}
	if s.Get() != Defaults() {
		t.Fatal("corrupt file did not fall back to defaults")
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatal("corrupt file not kept aside")
	}
}

func TestResolve(t *testing.T) {
	v := 2
	if Resolve(&v, 1) != 2 || Resolve[int](nil, 1) != 1 {
		t.Fatal("Resolve wrong")
	}
}
