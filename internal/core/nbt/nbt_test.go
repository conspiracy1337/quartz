package nbt

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// modernLevel builds a level.dat shaped like 1.21's: Data with a name, mode,
// version, seed and an unknown nested tag that must survive a round trip.
func modernLevel() *File {
	data := NewCompound()
	data.Comp.Set("LevelName", NewString("Northwatch ✦ 𝔑"))
	data.Comp.Set("GameType", NewInt(0))
	data.Comp.Set("hardcore", NewByte(1))
	data.Comp.Set("LastPlayed", NewLong(1_760_000_000_000))
	data.Comp.Set("DataVersion", NewInt(3955))
	ver := NewCompound()
	ver.Comp.Set("Name", NewString("1.21.1"))
	ver.Comp.Set("Id", NewInt(3955))
	data.Comp.Set("Version", ver)
	wg := NewCompound()
	wg.Comp.Set("seed", NewLong(-4172144997902289642))
	data.Comp.Set("WorldGenSettings", wg)
	odd := NewList(Compound)
	odd.List.Items = append(odd.List.Items, Tag{Kind: IntArray, Ints: []int32{1, -2}})
	odd.List.Elem = IntArray
	data.Comp.Set("x-mod", odd)
	data.Comp.Set("pi", Tag{Kind: Double, Num: int64(0x400921fb54442d18)})
	data.Comp.Set("nul", NewString("a\x00b"))
	data.Comp.Set("empty", NewList(End))
	root := NewCompound()
	root.Comp.Set("Data", data)
	return &File{Root: root, Gzip: true}
}

func TestRoundTripModern(t *testing.T) {
	f := modernLevel()
	raw := f.Bytes()
	g, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !g.Gzip {
		t.Fatal("gzip not detected")
	}
	d := g.Root.Get("Data")
	if got := d.Get("LevelName").Text(); got != "Northwatch ✦ 𝔑" {
		t.Fatalf("name %q", got)
	}
	if d.Get("hardcore").Int() != 1 || d.Path("WorldGenSettings", "seed").Int() != -4172144997902289642 {
		t.Fatal("numbers lost")
	}
	if d.Path("Version", "Name").Text() != "1.21.1" || d.Get("nul").Text() != "a\x00b" {
		t.Fatal("strings lost")
	}
	if v := d.Get("pi").Float(); v < 3.14159 || v > 3.1416 {
		t.Fatalf("double %v", v)
	}
	if !bytes.Equal(Encode(g.Name, g.Root), Encode(f.Name, f.Root)) {
		t.Fatal("payload changed in a round trip")
	}
}

func TestEditKeepsOtherTags(t *testing.T) {
	f := modernLevel()
	before := Encode(f.Name, f.Root)
	g, _ := Parse(f.Bytes())
	g.Root.Get("Data").Comp.Set("LevelName", NewString("Northwatch ✦ 𝔑"))
	if !bytes.Equal(Encode(g.Name, g.Root), before) {
		t.Fatal("setting the same name changed the payload")
	}
	g.Root.Get("Data").Comp.Set("LevelName", NewString("Renamed"))
	h, _ := Parse(g.Bytes())
	if h.Root.Path("Data", "LevelName").Text() != "Renamed" || h.Root.Path("Data", "x-mod").List.Items[0].Ints[1] != -2 {
		t.Fatal("edit lost data")
	}
}

func TestServersFixture(t *testing.T) {
	f, err := ReadFile(filepath.Join("testdata", "servers-xmcl.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Gzip {
		t.Fatal("servers.dat is not gzipped")
	}
	list := f.Root.Get("servers")
	if list == nil || list.Kind != List || len(list.List.Items) == 0 {
		t.Fatal("no servers list")
	}
	first := &list.List.Items[0]
	if first.Get("ip").Text() == "" {
		t.Fatal("server without ip")
	}
	raw, _ := os.ReadFile(filepath.Join("testdata", "servers-xmcl.dat"))
	if !bytes.Equal(f.Bytes(), raw) {
		t.Fatal("servers.dat changed in a round trip")
	}
}

func TestHostile(t *testing.T) {
	deep := []byte{byte(Compound), 0, 0}
	for range MaxDepth + 2 {
		deep = append(deep, byte(Compound), 0, 1, 'x')
	}
	cases := map[string]struct {
		b   []byte
		err error
	}{
		"empty":            {nil, ErrFormat},
		"not a compound":   {[]byte{byte(String), 0, 0, 0, 0}, ErrFormat},
		"truncated":        {[]byte{byte(Compound), 0, 0, byte(Int), 0, 1, 'a', 0, 0}, ErrFormat},
		"huge array":       {[]byte{byte(Compound), 0, 0, byte(LongArray), 0, 1, 'a', 0x7f, 0xff, 0xff, 0xff}, ErrFormat},
		"negative length":  {[]byte{byte(Compound), 0, 0, byte(ByteArray), 0, 1, 'a', 0xff, 0xff, 0xff, 0xff}, ErrFormat},
		"huge list":        {[]byte{byte(Compound), 0, 0, byte(List), 0, 1, 'a', byte(Compound), 0x10, 0, 0, 0}, ErrFormat},
		"bad kind":         {[]byte{byte(Compound), 0, 0, 0x7f, 0, 1, 'a'}, ErrFormat},
		"end list with n":  {[]byte{byte(Compound), 0, 0, byte(List), 0, 1, 'a', 0, 0, 0, 0, 3}, ErrFormat},
		"too deep":         {deep, ErrTooDeep},
		"bad gzip":         {[]byte{0x1f, 0x8b, 1, 2, 3}, ErrFormat},
	}
	for name, c := range cases {
		if _, err := Parse(c.b); !errors.Is(err, c.err) {
			t.Errorf("%s: got %v, want %v", name, err, c.err)
		}
	}
}

func TestModifiedUTF8(t *testing.T) {
	for _, s := range []string{"", "plain", "a\x00b", "é", "€", "𝔑 emoji 🎮", "mixed\x00𝔑"} {
		enc := EncodeString(s)
		if DecodeString(enc) != s {
			t.Errorf("%q did not round trip", s)
		}
	}
	if EncodeString("\x00") != "\xc0\x80" {
		t.Error("NUL must be C0 80")
	}
	if len(EncodeString("🎮")) != 6 {
		t.Error("supplementary characters are two surrogates")
	}
	if DecodeString("\xed\xa0\xbd") != "�" {
		t.Error("lone surrogate must decode to U+FFFD")
	}
}

func FuzzParse(f *testing.F) {
	f.Add(modernLevel().Bytes())
	f.Add(Encode("", modernLevel().Root))
	if raw, err := os.ReadFile(filepath.Join("testdata", "servers-xmcl.dat")); err == nil {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		g, err := Parse(b)
		if err != nil {
			return
		}
		// Whatever decodes must encode and decode to the same payload.
		p := Encode(g.Name, g.Root)
		h, err := Parse(p)
		if err != nil {
			t.Fatalf("re-parse: %v", err)
		}
		if !bytes.Equal(Encode(h.Name, h.Root), p) {
			t.Fatal("unstable encoding")
		}
	})
}
