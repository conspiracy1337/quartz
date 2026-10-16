package pngx

import (
	"bytes"
	"image"
	"image/color"
	"image/color/palette"
	"math/rand/v2"
	"testing"

	"golang.org/x/image/draw"
)

// TestScaleMatchesXImageDraw checks Scale against the x/image/draw code it
// was taken from, pixel for pixel, for every source type the decoders return.
func TestScaleMatchesXImageDraw(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	fill := func(p []byte) {
		for i := range p {
			p[i] = byte(rng.UintN(256))
		}
	}
	r := image.Rect(3, 5, 3+257, 5+139) // an offset origin catches indexing slips
	sources := map[string]image.Image{}
	{
		m := image.NewRGBA(r)
		fill(m.Pix)
		for i := 0; i < len(m.Pix); i += 4 { // keep it valid premultiplied
			a := m.Pix[i+3]
			m.Pix[i], m.Pix[i+1], m.Pix[i+2] = min(m.Pix[i], a), min(m.Pix[i+1], a), min(m.Pix[i+2], a)
		}
		sources["RGBA"] = m
	}
	{
		m := image.NewNRGBA(r)
		fill(m.Pix)
		sources["NRGBA"] = m
	}
	{
		m := image.NewGray(r)
		fill(m.Pix)
		sources["Gray"] = m
	}
	{
		m := image.NewNRGBA64(r)
		fill(m.Pix)
		sources["NRGBA64"] = m
	}
	{
		m := image.NewPaletted(r, palette.Plan9)
		fill(m.Pix)
		sources["Paletted"] = m
	}
	{
		m := image.NewCMYK(r)
		fill(m.Pix)
		sources["CMYK"] = m
	}
	for _, ratio := range []image.YCbCrSubsampleRatio{image.YCbCrSubsampleRatio444, image.YCbCrSubsampleRatio422, image.YCbCrSubsampleRatio420, image.YCbCrSubsampleRatio440, image.YCbCrSubsampleRatio411, image.YCbCrSubsampleRatio410} {
		m := image.NewYCbCr(r, ratio)
		fill(m.Y)
		fill(m.Cb)
		fill(m.Cr)
		sources["YCbCr"+ratio.String()] = m
	}
	sources["plain Image"] = &onlyImage{sources["NRGBA"]} // neither RGBA64Image nor a known type

	for name, src := range sources {
		for _, size := range [][2]int{{128, 69}, {256, 138}, {64, 200}, {1, 1}, {257, 139}, {300, 150}} {
			got := Scale(src, size[0], size[1])
			want := image.NewRGBA(image.Rect(0, 0, size[0], size[1]))
			draw.ApproxBiLinear.Scale(want, want.Bounds(), src, src.Bounds(), draw.Src, nil)
			if !bytes.Equal(got.Pix, want.Pix) {
				t.Errorf("%s to %dx%d: pixels differ from x/image/draw", name, size[0], size[1])
			}
		}
	}
}

type onlyImage struct{ m image.Image }

func (o *onlyImage) ColorModel() color.Model { return o.m.ColorModel() }
func (o *onlyImage) Bounds() image.Rectangle { return o.m.Bounds() }
func (o *onlyImage) At(x, y int) color.Color { return o.m.At(x, y) }
