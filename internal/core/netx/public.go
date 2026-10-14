package netx

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"syscall"
)

// ErrNotPublic means a request marked PublicOnly would reach this computer
// or a private network.
var ErrNotPublic = errors.New("netx: refusing to fetch from this computer or a private network")

type publicKey struct{}

// PublicOnly lets requests made with the returned context reach only public
// internet addresses, for links that come from outside (a web page's
// quartz://import?url=…), so no page can make the launcher request a router
// or a local service. The host is checked before the request and each
// redirect, and a direct connection's address again as it is dialled, so a
// name that changes its answer (DNS rebinding) can't slip through.
func PublicOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, publicKey{}, true)
}

func publicOnly(ctx context.Context) bool {
	v, _ := ctx.Value(publicKey{}).(bool)
	return v
}

// Ranges that aren't the internet but netip doesn't flag: carrier-grade
// NAT (RFC 6598, private in practice) and "this network" (0.0.0.0/8).
var (
	cgnat       = netip.MustParsePrefix("100.64.0.0/10")
	thisNetwork = netip.MustParsePrefix("0.0.0.0/8")
)

// public reports whether ip is an internet address.
func public(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsValid() && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified() &&
		!ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsInterfaceLocalMulticast() && !ip.IsMulticast() &&
		!cgnat.Contains(ip) && !thisNetwork.Contains(ip)
}

// checkHost refuses a host that names or resolves to a non-public address.
func checkHost(ctx context.Context, host string) error {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return ErrNotPublic
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if !public(ip) {
			return ErrNotPublic
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if !public(ip) {
			return ErrNotPublic
		}
	}
	return nil
}

// publicControl refuses a connection to a non-public address; it sees the
// address actually dialled.
func publicControl(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil || !public(ap.Addr()) {
		return ErrNotPublic
	}
	return nil
}

// envProxies are the proxy addresses the environment names (system mode):
// connections to them aren't the request's target, so they aren't checked.
func envProxies() map[string]bool {
	out := map[string]bool{}
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		if a := proxyAddr(os.Getenv(k)); a != "" {
			out[a] = true
		}
	}
	return out
}

// proxyAddr is a proxy URL's host:port, with the scheme's default port.
func proxyAddr(raw string) string {
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"https": "443", "socks5": "1080"}[u.Scheme]
		if port == "" {
			port = "80"
		}
	}
	return net.JoinHostPort(u.Hostname(), port)
}

// guardDial wraps dial so marked requests connect only to public addresses,
// except for the proxies the request goes through.
func guardDial(base *net.Dialer, proxies map[string]bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	guarded := *base
	guarded.Control = publicControl
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if !publicOnly(ctx) || proxies[addr] {
			return base.DialContext(ctx, network, addr)
		}
		return guarded.DialContext(ctx, network, addr)
	}
}
