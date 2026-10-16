package pngx

// Scale is golang.org/x/image/draw's ApproxBiLinear.Scale for an *image.RGBA
// destination, the whole source and the Src operator, which is all the
// launcher needs. That package compiles a copy of the loop per destination,
// source and operator; here one loop reads samples through a small reader per
// source type, with the same arithmetic, so the pixels are identical.
//
// Copyright 2015 The Go Authors. All rights reserved. Use of this source code
// is governed by a BSD-style license that can be found at
// https://go.googlesource.com/image/+/refs/heads/master/LICENSE.

import (
	"image"
	"image/draw"
)

// sampler returns the premultiplied 16-bit colour of the source pixel (x, y).
type sampler func(x, y int) (r, g, b, a uint32)

// Scale returns src resized to w×h with approximate bilinear interpolation.
func Scale(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sr := src.Bounds()
	if dst.Rect.Size() == sr.Size() {
		draw.Draw(dst, dst.Rect, src, sr.Min, draw.Src)
		return dst
	}
	if dst.Rect.Empty() || sr.Empty() {
		return dst
	}
	scaleABL(dst, sr, samplerOf(src))
	return dst
}

func samplerOf(src image.Image) sampler {
	switch s := src.(type) {
	case *image.NRGBA:
		return func(x, y int) (r, g, b, a uint32) {
			i := s.PixOffset(x, y)
			a = uint32(s.Pix[i+3]) * 0x101
			return uint32(s.Pix[i+0]) * a / 0xff, uint32(s.Pix[i+1]) * a / 0xff, uint32(s.Pix[i+2]) * a / 0xff, a
		}
	case *image.RGBA:
		return func(x, y int) (r, g, b, a uint32) {
			i := s.PixOffset(x, y)
			return uint32(s.Pix[i+0]) * 0x101, uint32(s.Pix[i+1]) * 0x101, uint32(s.Pix[i+2]) * 0x101, uint32(s.Pix[i+3]) * 0x101
		}
	case *image.YCbCr:
		// x/image/draw has exact copies of YCbCr.RGBA64At for these ratios and
		// falls back to At for the others.
		switch s.SubsampleRatio {
		case image.YCbCrSubsampleRatio444, image.YCbCrSubsampleRatio422, image.YCbCrSubsampleRatio420, image.YCbCrSubsampleRatio440:
			return rgba64Sampler(s)
		}
		return atSampler(s)
	case image.RGBA64Image:
		return rgba64Sampler(s)
	}
	return atSampler(src)
}

func rgba64Sampler(s image.RGBA64Image) sampler {
	return func(x, y int) (r, g, b, a uint32) {
		c := s.RGBA64At(x, y)
		return uint32(c.R), uint32(c.G), uint32(c.B), uint32(c.A)
	}
}

func atSampler(s image.Image) sampler {
	return func(x, y int) (r, g, b, a uint32) { return s.At(x, y).RGBA() }
}

// scaleABL keeps x/image/draw's float64 conversions on every product, which
// also stops the compiler from fusing them into FMA instructions.
func scaleABL(dst *image.RGBA, sr image.Rectangle, at sampler) {
	dr := dst.Rect
	sw := int32(sr.Dx())
	sh := int32(sr.Dy())
	yscale := float64(sh) / float64(dr.Dy())
	xscale := float64(sw) / float64(dr.Dx())
	swMinus1, shMinus1 := sw-1, sh-1

	for dy := int32(0); dy < int32(dr.Dy()); dy++ {
		sy := float64((float64(dy)+0.5)*yscale) - 0.5
		// If sy < 0, sy0 is clamped to 0 anyway, so int32(sy) can stand in
		// for int32(math.Floor(sy)). Likewise for sx below.
		sy0 := int32(sy)
		yFrac0 := sy - float64(sy0)
		yFrac1 := 1 - yFrac0
		sy1 := sy0 + 1
		if sy < 0 {
			sy0, sy1 = 0, 0
			yFrac0, yFrac1 = 0, 1
		} else if sy1 > shMinus1 {
			sy0, sy1 = shMinus1, shMinus1
			yFrac0, yFrac1 = 1, 0
		}
		y0, y1 := sr.Min.Y+int(sy0), sr.Min.Y+int(sy1)
		d := int(dy) * dst.Stride

		for dx := int32(0); dx < int32(dr.Dx()); dx, d = dx+1, d+4 {
			sx := float64((float64(dx)+0.5)*xscale) - 0.5
			sx0 := int32(sx)
			xFrac0 := sx - float64(sx0)
			xFrac1 := 1 - xFrac0
			sx1 := sx0 + 1
			if sx < 0 {
				sx0, sx1 = 0, 0
				xFrac0, xFrac1 = 0, 1
			} else if sx1 > swMinus1 {
				sx0, sx1 = swMinus1, swMinus1
				xFrac0, xFrac1 = 1, 0
			}
			x0, x1 := sr.Min.X+int(sx0), sr.Min.X+int(sx1)

			s00ru, s00gu, s00bu, s00au := at(x0, y0)
			s10ru, s10gu, s10bu, s10au := at(x1, y0)
			s01ru, s01gu, s01bu, s01au := at(x0, y1)
			s11ru, s11gu, s11bu, s11au := at(x1, y1)
			s10r := float64(xFrac1*float64(s00ru)) + float64(xFrac0*float64(s10ru))
			s10g := float64(xFrac1*float64(s00gu)) + float64(xFrac0*float64(s10gu))
			s10b := float64(xFrac1*float64(s00bu)) + float64(xFrac0*float64(s10bu))
			s10a := float64(xFrac1*float64(s00au)) + float64(xFrac0*float64(s10au))
			s11r := float64(xFrac1*float64(s01ru)) + float64(xFrac0*float64(s11ru))
			s11g := float64(xFrac1*float64(s01gu)) + float64(xFrac0*float64(s11gu))
			s11b := float64(xFrac1*float64(s01bu)) + float64(xFrac0*float64(s11bu))
			s11a := float64(xFrac1*float64(s01au)) + float64(xFrac0*float64(s11au))
			s11r = float64(yFrac1*s10r) + float64(yFrac0*s11r)
			s11g = float64(yFrac1*s10g) + float64(yFrac0*s11g)
			s11b = float64(yFrac1*s10b) + float64(yFrac0*s11b)
			s11a = float64(yFrac1*s10a) + float64(yFrac0*s11a)
			dst.Pix[d+0] = uint8(uint32(s11r) >> 8)
			dst.Pix[d+1] = uint8(uint32(s11g) >> 8)
			dst.Pix[d+2] = uint8(uint32(s11b) >> 8)
			dst.Pix[d+3] = uint8(uint32(s11a) >> 8)
		}
	}
}
