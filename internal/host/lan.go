package host

import (
	"context"
	"net"
	"strconv"
	"time"
)

// Minecraft clients list LAN games they hear announced on this multicast
// group, as a singleplayer world opened to LAN does.
const (
	lanGroup = "224.0.2.60:4445"
	lanEvery = 1500 * time.Millisecond
)

// lanPayload is the announcement: the server's description and port.
func lanPayload(desc string, port int) []byte {
	return []byte("[MOTD]" + motd(desc) + "[/MOTD][AD]" + strconv.Itoa(port) + "[/AD]")
}

// announce sends the announcement until ctx ends. The socket keeps the
// system's multicast TTL of 1, so it never leaves the local network.
func announce(ctx context.Context, desc string, port int) {
	addr, err := net.ResolveUDPAddr("udp4", lanGroup)
	if err != nil {
		return
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	msg := lanPayload(desc, port)
	t := time.NewTicker(lanEvery)
	defer t.Stop()
	for {
		_, _ = conn.Write(msg)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// lanAddress is this computer's address on the local network: the one the
// system would route outside through (asking costs no packet), else the
// first private address of an interface that is up. "" when there is none.
func lanAddress() string {
	if c, err := net.Dial("udp4", "192.0.2.1:9"); err == nil { // TEST-NET-1: resolves the route only
		ip := c.LocalAddr().(*net.UDPAddr).IP
		_ = c.Close()
		if ip.IsPrivate() {
			return ip.String()
		}
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && n.IP.IsPrivate() {
				return n.IP.String()
			}
		}
	}
	return ""
}

// portFree reports whether the server can listen on host:port.
func portFree(host string, port int) bool {
	l, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}
