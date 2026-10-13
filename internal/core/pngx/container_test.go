package pngx

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"testing"
)

func square(t *testing.T, side int) []byte {
	t.Helper()
	return pngOf(t, side, side, color.NRGBA{A: 255})
}

func TestICO(t *testing.T) {
	p32, p256 := square(t, 32), square(t, 256)
	ico, err := ICO(p32, p256)
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint16(ico[2:]) != 1 || binary.LittleEndian.Uint16(ico[4:]) != 2 {
		t.Fatal("bad header")
	}
	// Second entry: 256 is written as 0, and its offset points at its PNG.
	e := ico[6+16:]
	if e[0] != 0 || e[1] != 0 {
		t.Fatal("256 px must be stored as 0")
	}
	off, size := binary.LittleEndian.Uint32(e[12:]), binary.LittleEndian.Uint32(e[8:])
	if !bytes.Equal(ico[off:off+size], p256) {
		t.Fatal("entry offset or size is wrong")
	}
	if _, err := ICO(square(t, 512)); err == nil {
		t.Fatal("ICO holds at most 256 px")
	}
}

func TestICNS(t *testing.T) {
	p := square(t, 256)
	icns, err := ICNS(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(icns[:4]) != "icns" || int(binary.BigEndian.Uint32(icns[4:])) != len(icns) || string(icns[8:12]) != "ic08" ||
		!bytes.Equal(icns[16:], p) {
		t.Fatal("bad icns")
	}
	if _, err := ICNS(square(t, 100)); err == nil {
		t.Fatal("100 px isn't an icns size")
	}
	if _, err := ICNS([]byte("nope")); err == nil {
		t.Fatal("not a PNG")
	}
}
