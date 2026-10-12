package hashx

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// refMurmur2 is the textbook MurmurHash2 (seed 1) over the filtered bytes,
// written independently of the incremental version under test.
func refMurmur2(data []byte) uint32 {
	var f []byte
	for _, c := range data {
		if c != 9 && c != 10 && c != 13 && c != 32 {
			f = append(f, c)
		}
	}
	const m, r = 0x5bd1e995, 24
	h := uint32(1) ^ uint32(len(f)) //nolint:gosec // test data is small
	for len(f) >= 4 {
		k := binary.LittleEndian.Uint32(f)
		k *= m
		k ^= k >> r
		k *= m
		h = h*m ^ k
		f = f[4:]
	}
	switch len(f) {
	case 3:
		h ^= uint32(f[2]) << 16
		fallthrough
	case 2:
		h ^= uint32(f[1]) << 8
		fallthrough
	case 1:
		h ^= uint32(f[0])
		h *= m
	}
	h ^= h >> 13
	h *= m
	h ^= h >> 15
	return h
}

func TestMurmur2(t *testing.T) {
	random := make([]byte, 3001)
	_, _ = rand.Read(random)
	cases := map[string][]byte{
		"empty":           nil,
		"one byte":        []byte("a"),
		"three bytes":     []byte("abc"),
		"only whitespace": []byte(" \t\r\n"),
		"whitespace mid":  []byte("ab cd\nef\tg\rh"),
		"random":          random,
	}
	for name, data := range cases {
		if got, want := Murmur2(data), refMurmur2(data); got != want {
			t.Errorf("%s: got %d, want %d", name, got, want)
		}
	}
	if Murmur2([]byte("ab cd")) != Murmur2([]byte("abcd")) {
		t.Error("whitespace must not change the fingerprint")
	}
}

func TestMultiMatchesSeparateHashes(t *testing.T) {
	for name, size := range map[string]int{"empty": 0, "small": 1000, "exactly one buffer": chunk, "several buffers": 3*chunk + 17} {
		t.Run(name, func(t *testing.T) {
			data := make([]byte, size)
			_, _ = rand.Read(data)
			// Plenty of skipped bytes so the two murmur passes are exercised.
			for i := 0; i < len(data); i += 7 {
				data[i] = ' '
			}
			p := filepath.Join(t.TempDir(), "f")
			if err := os.WriteFile(p, data, 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := MultiFile(p)
			if err != nil {
				t.Fatal(err)
			}
			s1, _ := File(p, SHA1)
			s512, _ := File(p, SHA512)
			want := Multi{SHA1: s1, SHA512: s512, Murmur2: refMurmur2(data), Size: int64(size)}
			if got != want {
				t.Fatalf("got %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestMultiOfReader(t *testing.T) {
	got, err := MultiOf(bytes.NewReader([]byte("hello")))
	if err != nil || got.Size != 5 || got.SHA1 != "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d" {
		t.Fatalf("%+v %v", got, err)
	}
}
