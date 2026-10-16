package focusmute

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A minimal X11 client: just enough to follow the focused window and find
// the process that owns it. X speaks little-endian here (we say so at setup).

var le = binary.LittleEndian

const (
	opChangeWindowAttributes = 2
	opInternAtom             = 16
	opGetProperty            = 20
	opQueryExtension         = 98

	cwEventMask             = 0x800
	propertyChangeMask      = 0x400000
	evPropertyNotify        = 28
	evGeneric               = 35
	resQueryVersion         = 0
	resQueryClientIDs       = 4
	resClientPIDMask        = 2
	cookieAuth              = "MIT-MAGIC-COOKIE-1"
	maxReplyBytes           = 1 << 20
	setupTimeout            = 5 * time.Second
	familyLocal, familyWild = 256, 65535 // Xauthority families: this host, any host
)

type xconn struct {
	c       net.Conn
	r       *bufio.Reader
	root    uint32
	seq     uint16
	pending [][]byte // events read while waiting for a reply
	res     byte     // X-Resource's major opcode (0 without it)
	active  uint32   // _NET_ACTIVE_WINDOW
	wmPID   uint32   // _NET_WM_PID
}

// displaySocket is the local socket of a DISPLAY like ":0", ":1.0" or
// "unix:0"; ok is false for a display on another host.
func displaySocket(display string) (sock, num string, ok bool) {
	host, rest, found := strings.Cut(display, ":")
	if !found || (host != "" && host != "unix") {
		return "", "", false
	}
	num, _, _ = strings.Cut(rest, ".")
	if num == "" || strings.Trim(num, "0123456789") != "" {
		return "", "", false
	}
	return "/tmp/.X11-unix/X" + num, num, true
}

// dialX connects to the X server of DISPLAY.
func dialX() (*xconn, error) {
	sock, num, ok := displaySocket(os.Getenv("DISPLAY"))
	if !ok {
		return nil, errors.New("focusmute: no local X display")
	}
	c, err := net.DialTimeout("unix", sock, setupTimeout) //nolint:gosec // a local socket: /tmp/.X11-unix/X plus digits (displaySocket)
	if err != nil {
		if c, err = net.DialTimeout("unix", "@"+sock, setupTimeout); err != nil { //nolint:gosec // the same socket, abstract
			return nil, err
		}
	}
	x := &xconn{c: c, r: bufio.NewReader(c)}
	_ = c.SetDeadline(time.Now().Add(setupTimeout))
	if err := x.setup(xauthCookie(num)); err != nil {
		_ = c.Close()
		return nil, err
	}
	_ = c.SetDeadline(time.Time{})
	return x, nil
}

func pad4(n int) int { return (4 - n%4) % 4 }

func (x *xconn) setup(cookie []byte) error {
	name := ""
	if cookie != nil {
		name = cookieAuth
	}
	b := make([]byte, 12+len(name)+pad4(len(name))+len(cookie)+pad4(len(cookie)))
	b[0] = 'l'
	le.PutUint16(b[2:], 11)
	le.PutUint16(b[6:], uint16(len(name)))   //nolint:gosec // a fixed short name
	le.PutUint16(b[8:], uint16(len(cookie))) //nolint:gosec // a 16-byte cookie
	copy(b[12:], name)
	copy(b[12+len(name)+pad4(len(name)):], cookie)
	if _, err := x.c.Write(b); err != nil {
		return err
	}
	head := make([]byte, 8)
	if _, err := io.ReadFull(x.r, head); err != nil {
		return err
	}
	body := make([]byte, int(le.Uint16(head[6:]))*4)
	if _, err := io.ReadFull(x.r, body); err != nil {
		return err
	}
	if head[0] != 1 {
		return fmt.Errorf("focusmute: the X server refused the connection: %s", strings.TrimRight(string(body), "\x00"))
	}
	if len(body) < 32 {
		return errors.New("focusmute: short X setup")
	}
	vendor := int(le.Uint16(body[16:]))
	off := 32 + vendor + pad4(vendor) + 8*int(body[21])
	if len(body) < off+4 {
		return errors.New("focusmute: short X setup")
	}
	x.root = le.Uint32(body[off:])
	return nil
}

// xauthCookie finds the display's MIT cookie in the Xauthority file (nil
// without one: the server may still let this user in).
func xauthCookie(num string) []byte {
	path := os.Getenv("XAUTHORITY")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".Xauthority")
	}
	data, err := os.ReadFile(path) //nolint:gosec // the user's own X auth file
	if err != nil {
		return nil
	}
	return parseXauth(data, num)
}

func parseXauth(b []byte, num string) []byte {
	host, _ := os.Hostname()
	field := func() ([]byte, bool) {
		if len(b) < 2 {
			return nil, false
		}
		n := int(binary.BigEndian.Uint16(b))
		if len(b) < 2+n {
			return nil, false
		}
		v := b[2 : 2+n]
		b = b[2+n:]
		return v, true
	}
	var fallback []byte
	for len(b) >= 2 {
		fam := binary.BigEndian.Uint16(b)
		b = b[2:]
		addr, ok1 := field()
		n, ok2 := field()
		name, ok3 := field()
		data, ok4 := field()
		if !ok1 || !ok2 || !ok3 || !ok4 {
			break
		}
		if string(name) != cookieAuth || (len(n) > 0 && string(n) != num) {
			continue
		}
		if fam == familyWild || (fam == familyLocal && string(addr) == host) {
			return data
		}
		if fallback == nil {
			fallback = data
		}
	}
	return fallback
}

// next reads one packet: a 32-byte event or error, or a reply with its tail.
func (x *xconn) next() ([]byte, error) {
	p := make([]byte, 32)
	if _, err := io.ReadFull(x.r, p); err != nil {
		return nil, err
	}
	if p[0] == 1 || p[0]&0x7f == evGeneric {
		n := int(le.Uint32(p[4:])) * 4
		if n > maxReplyBytes {
			return nil, errors.New("focusmute: oversized X reply")
		}
		tail := make([]byte, n)
		if _, err := io.ReadFull(x.r, tail); err != nil {
			return nil, err
		}
		p = append(p, tail...)
	}
	return p, nil
}

// call sends a request and waits for its reply; events that arrive first
// are kept for event.
func (x *xconn) call(req []byte) ([]byte, error) {
	x.seq++
	seq := x.seq
	if _, err := x.c.Write(req); err != nil {
		return nil, err
	}
	for {
		p, err := x.next()
		if err != nil {
			return nil, err
		}
		switch {
		case p[0] == 0 && le.Uint16(p[2:]) == seq:
			return nil, fmt.Errorf("focusmute: X error %d", p[1])
		case p[0] == 1 && le.Uint16(p[2:]) == seq:
			return p, nil
		case p[0] > 1:
			x.pending = append(x.pending, p)
		}
	}
}

// send sends a request that has no reply.
func (x *xconn) send(req []byte) error {
	x.seq++
	_, err := x.c.Write(req)
	return err
}

func named(op byte, name string) []byte {
	b := make([]byte, 8+len(name)+pad4(len(name)))
	b[0] = op
	le.PutUint16(b[2:], uint16(len(b)/4))  //nolint:gosec // short fixed names
	le.PutUint16(b[4:], uint16(len(name))) //nolint:gosec // short fixed names
	copy(b[8:], name)
	return b
}

func (x *xconn) atom(name string) (uint32, error) {
	r, err := x.call(named(opInternAtom, name))
	if err != nil {
		return 0, err
	}
	return le.Uint32(r[8:]), nil
}

// watch asks for the root window's property changes and looks up what the
// focus lookups need.
func (x *xconn) watch() error {
	var err error
	if x.active, err = x.atom("_NET_ACTIVE_WINDOW"); err != nil {
		return err
	}
	if x.wmPID, err = x.atom("_NET_WM_PID"); err != nil {
		return err
	}
	if r, err := x.call(named(opQueryExtension, "X-Resource")); err == nil && r[8] == 1 {
		x.res = r[9]
		ver := []byte{x.res, resQueryVersion, 2, 0, 1, 2, 0, 0}
		if _, err := x.call(ver); err != nil {
			x.res = 0
		}
	}
	b := make([]byte, 16)
	b[0] = opChangeWindowAttributes
	le.PutUint16(b[2:], 4)
	le.PutUint32(b[4:], x.root)
	le.PutUint32(b[8:], cwEventMask)
	le.PutUint32(b[12:], propertyChangeMask)
	return x.send(b)
}

// cardinal reads a window's first 32-bit property value (0 when unset).
func (x *xconn) cardinal(win, prop uint32) (uint32, error) {
	b := make([]byte, 24)
	b[0] = opGetProperty
	le.PutUint16(b[2:], 6)
	le.PutUint32(b[4:], win)
	le.PutUint32(b[8:], prop)
	le.PutUint32(b[20:], 1)
	r, err := x.call(b)
	if err != nil {
		return 0, err
	}
	if r[1] != 32 || le.Uint32(r[16:]) < 1 || len(r) < 36 {
		return 0, nil
	}
	return le.Uint32(r[32:]), nil
}

// focusedPID is the process owning the focused window, as the system sees
// it (the X server asks the socket, so a sandboxed game's real id), else as
// the window says (_NET_WM_PID). 0 when nothing (or a Wayland window) has focus.
func (x *xconn) focusedPID() int {
	win, err := x.cardinal(x.root, x.active)
	if err != nil || win == 0 {
		return 0
	}
	if x.res != 0 {
		b := make([]byte, 16)
		b[0], b[1] = x.res, resQueryClientIDs
		le.PutUint16(b[2:], 4)
		le.PutUint32(b[4:], 1)
		le.PutUint32(b[8:], win)
		le.PutUint32(b[12:], resClientPIDMask)
		if r, err := x.call(b); err == nil {
			for ids, off := le.Uint32(r[8:]), 32; ids > 0 && off+12 <= len(r); ids-- {
				mask, n := le.Uint32(r[off+4:]), int(le.Uint32(r[off+8:]))
				if mask&resClientPIDMask != 0 && n >= 4 && off+16 <= len(r) {
					return int(le.Uint32(r[off+12:]))
				}
				off += 12 + n
			}
		}
	}
	pid, _ := x.cardinal(win, x.wmPID)
	return int(pid)
}

// focusChanged waits for the next change of the focused window.
func (x *xconn) focusChanged() error {
	for {
		var p []byte
		if len(x.pending) > 0 {
			p, x.pending = x.pending[0], x.pending[1:]
		} else {
			var err error
			if p, err = x.next(); err != nil {
				return err
			}
		}
		if p[0]&0x7f == evPropertyNotify && le.Uint32(p[4:]) == x.root && le.Uint32(p[8:]) == x.active {
			return nil
		}
	}
}

func (x *xconn) Close() error { return x.c.Close() }
