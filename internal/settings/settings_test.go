package settings

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"appearance":{"accent":"#60a5fa"},"unknown":{"x":1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path, events.NewBus(), discard)
	if err != nil {
		t.Fatal(err)
	}
	got := s.Get()
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
	if !reflect.DeepEqual(s.Get(), Defaults()) {
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

func TestProxyCustomValidation(t *testing.T) {
	cases := []struct {
		in   ProxyCustom
		want ProxyCustom
	}{
		{ProxyCustom{Type: "socks5", Host: "proxy.lan", Port: 1080, User: "me"}, ProxyCustom{Type: "socks5", Host: "proxy.lan", Port: 1080, User: "me"}},
		{ProxyCustom{Type: "ftp", Host: "http://evil/path", Port: 70000, User: "a\nb"}, ProxyCustom{Type: "http", Host: "", Port: 65535, User: ""}},
		{ProxyCustom{Type: "https", Host: "[::1]", Port: 0}, ProxyCustom{Type: "https", Host: "[::1]", Port: 8080}},
		{ProxyCustom{Host: "has space"}, ProxyCustom{Type: "http", Port: 8080}},
	}
	for _, c := range cases {
		s := Defaults()
		s.Downloads.ProxyCustom = c.in
		if got := s.Normalize().Downloads.ProxyCustom; got != c.want {
			t.Errorf("normalize(%+v) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestKeysAreCleaned(t *testing.T) {
	s := Defaults()
	s.General.Keys = map[string]string{"palette": "Ctrl+J", "zoom": "Ctrl++", "bad id": "Ctrl+K", "bare": "K", "long": "Ctrl+" + strings.Repeat("x", 20)}
	got := s.Normalize().General.Keys
	if len(got) != 2 || got["palette"] != "Ctrl+J" || got["zoom"] != "Ctrl++" {
		t.Fatal(got)
	}
}

func TestServerOrderIsCleaned(t *testing.T) {
	s := Defaults()
	s.General.ServerOrder = []string{"mc.hypixel.net", "", strings.Repeat("x", 301), "smp.kestrel.gg"}
	if got := s.Normalize().General.ServerOrder; len(got) != 2 || got[1] != "smp.kestrel.gg" {
		t.Fatal(got)
	}
	s.General.ServerOrder = make([]string, maxServers+5)
	for i := range s.General.ServerOrder {
		s.General.ServerOrder[i] = "a.gg"
	}
	if got := s.Normalize().General.ServerOrder; len(got) != maxServers {
		t.Fatal(len(got))
	}
}
