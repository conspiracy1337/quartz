package focusmute

import (
	"encoding/binary"
	"os"
	"testing"
)

func TestDisplaySocket(t *testing.T) {
	for in, want := range map[string]string{":0": "/tmp/.X11-unix/X0", ":1.0": "/tmp/.X11-unix/X1", "unix:2": "/tmp/.X11-unix/X2", "remote:0": "", "": "", ":x": ""} {
		if got, _, _ := displaySocket(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}

func TestParseXauth(t *testing.T) {
	host, _ := os.Hostname()
	entry := func(fam uint16, addr, num, name, data string) []byte {
		var b []byte
		b = binary.BigEndian.AppendUint16(b, fam)
		for _, f := range []string{addr, num, name, data} {
			b = binary.BigEndian.AppendUint16(b, uint16(len(f))) //nolint:gosec // short test strings
			b = append(b, f...)
		}
		return b
	}
	file := append(append(append(entry(familyLocal, host, "1", cookieAuth, "other-display"),
		entry(familyLocal, "elsewhere", "0", cookieAuth, "fallback")...),
		entry(familyLocal, host, "0", "XDM-AUTHORIZATION-1", "wrong-kind")...),
		entry(familyLocal, host, "0", cookieAuth, "this-one")...)
	if got := string(parseXauth(file, "0")); got != "this-one" {
		t.Fatalf("got %q", got)
	}
	if got := string(parseXauth(file[:len(file)-3], "0")); got != "fallback" {
		t.Fatalf("truncated file: %q", got)
	}
}

// TestFocusedWindow talks to this computer's X server (QUARTZ_IT_X=1): it
// must find the focused window's owner.
func TestFocusedWindow(t *testing.T) {
	if os.Getenv("QUARTZ_IT_X") != "1" {
		t.Skip("set QUARTZ_IT_X=1 to use the real X display")
	}
	x, err := dialX()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = x.Close() }()
	if err := x.watch(); err != nil {
		t.Fatal(err)
	}
	pid := x.focusedPID()
	t.Logf("X-Resource opcode %d, focused pid %d", x.res, pid)
	if x.res == 0 || pid <= 0 {
		t.Fatal("no focused window owner")
	}
}
