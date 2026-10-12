package network

import (
	"testing"

	"quartz/internal/core/netx"
	"quartz/internal/settings"
)

func TestProxy(t *testing.T) {
	d := settings.Defaults().Downloads
	if p := Proxy(d, ""); p.Mode != netx.ProxySystem {
		t.Fatalf("default = %+v", p)
	}
	d.Proxy = "none"
	if p := Proxy(d, ""); p.Mode != netx.ProxyNone {
		t.Fatalf("none = %+v", p)
	}
	d.Proxy = "custom"
	if p := Proxy(d, "pw"); p.Mode != netx.ProxySystem {
		t.Fatalf("incomplete custom proxy must fall back to system, got %+v", p)
	}
	d.ProxyCustom = settings.ProxyCustom{Type: "socks5", Host: "proxy.lan", Port: 1080, User: "me"}
	if p := Proxy(d, "pw"); p.Mode != netx.ProxyCustom || p.Password != "pw" || p.Type != "socks5" {
		t.Fatalf("custom = %+v", p)
	}
}
