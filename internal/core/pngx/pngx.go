// Package pngx turns untrusted images (skins, instance icons) into canonical
// PNGs: the header is checked before anything is decoded, so a huge declared
// size never allocates, and the result is re-encoded from pixels alone, so no
// stray chunk or trailing data survives. Pure Go.
package pngx

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	_ "image/jpeg" // icons may be photos
	"image/png"
)

// ErrNotImage is returned for anything that isn't an acceptable image.
var ErrNotImage = errors.New("pngx: not a usable image")

// Limits bound what Decode accepts.
type Limits struct {
	MaxBytes int // the file
	MaxSide  int // either dimension, in pixels
	// PNGOnly refuses other formats (skins must be PNG).
	PNGOnly bool
}

// Decode checks data against l and decodes it.
func Decode(data []byte, l Limits) (image.Image, error) {
	if len(data) == 0 || len(data) > l.MaxBytes {
		return nil, ErrNotImage
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > l.MaxSide || cfg.Height > l.MaxSide || (l.PNGOnly && format != "png") {
		return nil, ErrNotImage
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrNotImage
	}
	return img, nil
}

// Encode writes img as a PNG and names it by the SHA-256 of that encoding.
func Encode(img image.Image) ([]byte, string, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:]), nil
}

// Square crops img to its centred square and scales it down to at most side
// pixels, averaging the source pixels each output pixel covers (a box
// filter: sharp for pixel art at whole ratios, smooth for photos).
func Square(img image.Image, side int) *image.NRGBA {
	b := img.Bounds()
	n := min(b.Dx(), b.Dy())
	x0, y0 := b.Min.X+(b.Dx()-n)/2, b.Min.Y+(b.Dy()-n)/2
	out := min(n, side)
	dst := image.NewNRGBA(image.Rect(0, 0, out, out))
	for y := range out {
		sy0, sy1 := y0+y*n/out, y0+(y+1)*n/out
		for x := range out {
			sx0, sx1 := x0+x*n/out, x0+(x+1)*n/out
			var r, g, bl, a, count uint64
			for sy := sy0; sy < max(sy1, sy0+1); sy++ {
				for sx := sx0; sx < max(sx1, sx0+1); sx++ {
					c := color.NRGBA64Model.Convert(img.At(sx, sy)).(color.NRGBA64)
					// Weight colour by alpha so transparent pixels don't darken edges.
					r += uint64(c.R) * uint64(c.A)
					g += uint64(c.G) * uint64(c.A)
					bl += uint64(c.B) * uint64(c.A)
					a += uint64(c.A)
					count++
				}
			}
			if a == 0 {
				continue
			}
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r / a >> 8), G: uint8(g / a >> 8), B: uint8(bl / a >> 8), //nolint:gosec // 16-bit averages shifted to 8 bits
				A: uint8(a / count >> 8), //nolint:gosec // as above
			})
		}
	}
	return dst
}
