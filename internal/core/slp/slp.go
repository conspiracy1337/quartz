// Package slp asks a Minecraft Java server for its status (the server list ping,
// 1.7 and later): version, players, a favicon and the round trip time. Servers
// are untrusted: every read has a deadline, packet sizes are capped, and the
// favicon is decoded and re-encoded as a small PNG before anyone sees it.
package slp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"quartz/internal/core/flightx"
	"quartz/internal/core/pngx"
)

// DefaultPort is Minecraft's port when an address names none.
const DefaultPort = 25565

// Limits.
const (
	maxPacket  = 1 << 20 // the status JSON is at most 32767 UTF-16 units; Forge adds mod lists
	maxFavicon = 64 << 10
	Timeout    = 3 * time.Second
)

// ErrBadAddress is returned for an address that isn't host or host:port.
var ErrBadAddress = errors.New("slp: not a server address")

// Status is what a server reports about itself.
type Status struct {
	Version  string // e.g. "1.21.4" or "Paper 1.21.4"; may hold formatting codes stripped
	Protocol int
	Online   int
	Max      int
	MOTD     string // plain text
	Latency  time.Duration
	Favicon  []byte // canonical PNG, at most 64×64; nil without one
}

// Address is a parsed server address.
type Address struct {
	Host string
	Port int // 0 when the address named none
}

// String is the address as the game writes it (the port only when given).
func (a Address) String() string {
	if a.Port == 0 {
		return a.Host
	}
	return net.JoinHostPort(a.Host, strconv.Itoa(a.Port))
}

// Key is the address for merging duplicates: lower-case, no trailing dot, default port dropped.
func (a Address) Key() string {
	h := strings.TrimSuffix(strings.ToLower(a.Host), ".")
	if a.Port == 0 || a.Port == DefaultPort {
		return h
	}
	return net.JoinHostPort(h, strconv.Itoa(a.Port))
}

// Parse reads "host", "host:port", "[v6]:port" or a bare IPv6 address.
func Parse(s string) (Address, error) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 255 || strings.ContainsAny(s, " /\\@\x00") {
		return Address{}, ErrBadAddress
	}
	if ip := net.ParseIP(strings.Trim(s, "[]")); ip != nil && strings.Count(s, ":") > 1 && !strings.HasPrefix(s, "[") {
		return Address{Host: ip.String()}, nil
	}
	host, port := s, 0
	if h, p, err := net.SplitHostPort(s); err == nil {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return Address{}, ErrBadAddress
		}
		host, port = h, n
	}
	if host == "" || (strings.ContainsAny(host, ":[]") && net.ParseIP(host) == nil) {
		return Address{}, ErrBadAddress
	}
	return Address{Host: host, Port: port}, nil
}

// Ping asks the server at addr for its status. protocol is the version the
// handshake claims (-1 asks which one the server wants).
func Ping(ctx context.Context, addr Address, protocol int) (*Status, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	host, port := resolve(ctx, addr)
	var d net.Dialer
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	connected := time.Since(start)

	var hs bytes.Buffer
	putVarInt(&hs, 0x00)
	putVarInt(&hs, int32(protocol))
	putString(&hs, addr.Host)
	_ = binary.Write(&hs, binary.BigEndian, uint16(port))
	putVarInt(&hs, 1) // next state: status
	if err := writePacket(conn, hs.Bytes()); err != nil {
		return nil, err
	}
	if err := writePacket(conn, []byte{0x00}); err != nil {
		return nil, err
	}
	r := bufio.NewReader(conn)
	body, err := readPacket(r)
	if err != nil {
		return nil, err
	}
	br := bytes.NewReader(body)
	if id, err := readVarInt(br); err != nil || id != 0x00 {
		return nil, fmt.Errorf("slp: unexpected packet")
	}
	n, err := readVarInt(br)
	if err != nil || n < 0 || int(n) > br.Len() {
		return nil, fmt.Errorf("slp: bad status")
	}
	js := make([]byte, n)
	_, _ = io.ReadFull(br, js)
	st, err := decode(js)
	if err != nil {
		return nil, err
	}

	// Ping for the round trip; servers that skip it get the connect time instead.
	st.Latency = connected
	var pp bytes.Buffer
	putVarInt(&pp, 0x01)
	_ = binary.Write(&pp, binary.BigEndian, time.Now().UnixMilli())
	sent := time.Now()
	if writePacket(conn, pp.Bytes()) == nil {
		if body, err := readPacket(r); err == nil && len(body) == 9 && body[0] == 0x01 {
			st.Latency = time.Since(sent)
		}
	}
	return st, nil
}

// resolve follows the _minecraft._tcp SRV record when the address names no port.
func resolve(ctx context.Context, a Address) (string, int) {
	if a.Port != 0 {
		return a.Host, a.Port
	}
	if net.ParseIP(a.Host) == nil {
		if _, srvs, err := net.DefaultResolver.LookupSRV(ctx, "minecraft", "tcp", a.Host); err == nil && len(srvs) > 0 {
			return strings.TrimSuffix(srvs[0].Target, "."), int(srvs[0].Port)
		}
	}
	return a.Host, DefaultPort
}

type wire struct {
	Version struct {
		Name     string `json:"name"`
		Protocol int    `json:"protocol"`
	} `json:"version"`
	Players struct {
		Max    int `json:"max"`
		Online int `json:"online"`
	} `json:"players"`
	Description json.RawMessage `json:"description"`
	Favicon     string          `json:"favicon"`
}

func decode(js []byte) (*Status, error) {
	var w wire
	if err := json.Unmarshal(js, &w); err != nil {
		return nil, fmt.Errorf("slp: bad status json")
	}
	st := &Status{
		Version:  clip(StripCodes(w.Version.Name), 64),
		Protocol: w.Version.Protocol,
		Online:   max(0, w.Players.Online),
		Max:      max(0, w.Players.Max),
		MOTD:     clip(StripCodes(chatText(w.Description)), 256),
	}
	st.Favicon = favicon(w.Favicon)
	return st, nil
}

// favicon accepts only a base64 PNG data URL and returns it re-encoded.
func favicon(s string) []byte {
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(s, prefix) || len(s) > maxFavicon*2 {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(s[len(prefix):], "\n", ""))
	if err != nil {
		return nil
	}
	img, err := pngx.Decode(raw, pngx.Limits{MaxBytes: maxFavicon, MaxSide: 64, PNGOnly: true})
	if err != nil {
		return nil
	}
	b, _, err := pngx.Encode(img)
	if err != nil {
		return nil
	}
	return b
}

// chatText flattens a chat component (a string, or an object with text and extra).
func chatText(raw json.RawMessage) string {
	var b strings.Builder
	var walk func(v any, depth int)
	walk = func(v any, depth int) {
		if depth > 16 || b.Len() > 1024 {
			return
		}
		switch t := v.(type) {
		case string:
			b.WriteString(t)
		case []any:
			for _, x := range t {
				walk(x, depth+1)
			}
		case map[string]any:
			walk(t["text"], depth+1)
			walk(t["extra"], depth+1)
		}
	}
	var v any
	if json.Unmarshal(raw, &v) == nil {
		walk(v, 0)
	}
	return b.String()
}

// StripCodes removes § formatting codes.
func StripCodes(s string) string {
	if !strings.Contains(s, "§") {
		return strings.TrimSpace(s)
	}
	var b strings.Builder
	skip := false
	for _, r := range s {
		switch {
		case skip:
			skip = false
		case r == '§':
			skip = true
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !isRuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func isRuneStart(b byte) bool { return b&0xc0 != 0x80 }

// ---- framing ----

func putVarInt(b *bytes.Buffer, v int32) {
	u := uint32(v)
	for {
		if u&^0x7f == 0 {
			b.WriteByte(byte(u))
			return
		}
		b.WriteByte(byte(u&0x7f | 0x80))
		u >>= 7
	}
}

func readVarInt(r io.ByteReader) (int32, error) {
	var v uint32
	for i := range 5 {
		c, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		v |= uint32(c&0x7f) << (7 * i)
		if c&0x80 == 0 {
			return int32(v), nil
		}
	}
	return 0, fmt.Errorf("slp: varint too long")
}

func putString(b *bytes.Buffer, s string) {
	putVarInt(b, int32(len(s)))
	b.WriteString(s)
}

func writePacket(w io.Writer, body []byte) error {
	var b bytes.Buffer
	putVarInt(&b, int32(len(body)))
	b.Write(body)
	_, err := w.Write(b.Bytes())
	return err
}

func readPacket(r *bufio.Reader) ([]byte, error) {
	n, err := readVarInt(r)
	if err != nil {
		return nil, err
	}
	if n <= 0 || n > maxPacket {
		return nil, fmt.Errorf("slp: packet size %d", n)
	}
	body := make([]byte, n)
	_, err = io.ReadFull(r, body)
	return body, err
}

// ---- cached pinger ----

// Result is a cached ping: a status, or the error that stopped it.
type Result struct {
	Status *Status
	Err    error
	At     time.Time
}

// Pinger pings with a short cache, merges concurrent pings of one address and
// limits how many run at once.
type Pinger struct {
	TTL  time.Duration
	ping func(context.Context, Address, int) (*Status, error)

	sem   chan struct{}
	group flightx.Group[Result]
	mu    sync.Mutex
	cache map[string]Result
}

// NewPinger returns a Pinger with a 60 s cache and 8 pings at a time.
func NewPinger() *Pinger {
	return &Pinger{TTL: time.Minute, ping: Ping, sem: make(chan struct{}, 8), cache: map[string]Result{}}
}

// Get pings addr unless a fresh result is cached; force skips the cache.
func (p *Pinger) Get(ctx context.Context, addr Address, protocol int, force bool) Result {
	key := addr.Key() + "#" + strconv.Itoa(protocol)
	if !force {
		p.mu.Lock()
		r, ok := p.cache[key]
		p.mu.Unlock()
		if ok && time.Since(r.At) < p.TTL {
			return r
		}
	}
	r, err := p.group.Do(ctx, key, func(ctx context.Context) (Result, error) {
		select {
		case p.sem <- struct{}{}:
		case <-ctx.Done():
			return Result{Err: ctx.Err(), At: time.Now()}, nil
		}
		defer func() { <-p.sem }()
		st, err := p.ping(ctx, addr, protocol)
		r := Result{Status: st, Err: err, At: time.Now()}
		if !errors.Is(err, context.Canceled) {
			p.mu.Lock()
			p.cache[key] = r
			if len(p.cache) > 512 {
				for k, old := range p.cache {
					if time.Since(old.At) >= p.TTL {
						delete(p.cache, k)
					}
				}
			}
			p.mu.Unlock()
		}
		return r, nil
	})
	if err != nil {
		return Result{Err: err, At: time.Now()} // this caller gave up; others may still wait
	}
	return r
}
