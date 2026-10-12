package pngx

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

func pngOf(t *testing.T, w, h int, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDecodeLimits(t *testing.T) {
	ok := pngOf(t, 10, 10, color.NRGBA{R: 255, A: 255})
	if _, err := Decode(ok, Limits{MaxBytes: 1 << 20, MaxSide: 16}); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(ok, Limits{MaxBytes: 1 << 20, MaxSide: 8}); err == nil {
		t.Fatal("oversized dimensions accepted")
	}
	if _, err := Decode(ok, Limits{MaxBytes: 10, MaxSide: 16}); err == nil {
		t.Fatal("oversized file accepted")
	}
	if _, err := Decode([]byte("GIF89a not really"), Limits{MaxBytes: 1 << 20, MaxSide: 16}); err == nil {
		t.Fatal("non-image accepted")
	}
}

func TestEncodeDropsTrailingData(t *testing.T) {
	data := pngOf(t, 4, 4, color.NRGBA{G: 200, A: 255})
	img, err := Decode(append(data, []byte("<script>trailing</script>")...), Limits{MaxBytes: 1 << 20, MaxSide: 16})
	if err != nil {
		t.Fatal(err)
	}
	out, hash, err := Encode(img)
	if err != nil || bytes.Contains(out, []byte("script")) || len(hash) != 64 {
		t.Fatalf("re-encode kept extra data or no hash: %v", err)
	}
	_, again, _ := Encode(img)
	if hash != again {
		t.Fatal("the same picture must get the same name")
	}
}

func TestSquare(t *testing.T) {
	img, _ := Decode(pngOf(t, 300, 200, color.NRGBA{B: 255, A: 255}), Limits{MaxBytes: 1 << 20, MaxSide: 400})
	sq := Square(img, 128)
	if sq.Bounds().Dx() != 128 || sq.Bounds().Dy() != 128 {
		t.Fatalf("size %v", sq.Bounds())
	}
	if c := sq.NRGBAAt(64, 64); c.B != 255 || c.A != 255 {
		t.Fatalf("colour %v", c)
	}
	small, _ := Decode(pngOf(t, 20, 30, color.NRGBA{A: 255}), Limits{MaxBytes: 1 << 20, MaxSide: 400})
	if s := Square(small, 128); s.Bounds().Dx() != 20 {
		t.Fatal("small pictures are cropped, never enlarged")
	}
}

func TestDecodesWebP(t *testing.T) {
	data, err := os.ReadFile("testdata/icon.webp")
	if err != nil {
		t.Fatal(err)
	}
	img, err := Decode(data, Limits{MaxBytes: 1 << 20, MaxSide: 64})
	if err != nil || img.Bounds().Dx() != 20 || img.Bounds().Dy() != 10 {
		t.Fatalf("webp: %v", err)
	}
	if _, err := Decode(data, Limits{MaxBytes: 1 << 20, MaxSide: 64, PNGOnly: true}); err == nil {
		t.Fatal("webp accepted as a skin")
	}
}
