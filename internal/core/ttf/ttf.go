// Package ttf draws text with a TrueType outline font. It is the part of
// golang.org/x/image/font/sfnt, font/opentype and font.Drawer the launcher
// uses (unhinted glyf outlines, a format 4 Unicode cmap, no kerning: the Go
// fonts), with the same arithmetic, so it draws the same pixels without
// pulling in sfnt's CFF, name and charmap support.
//
// Copyright 2016 The Go Authors. All rights reserved. Use of this source code
// is governed by a BSD-style license that can be found at
// https://go.googlesource.com/image/+/refs/heads/master/LICENSE.
package ttf

import (
	"errors"
	"image"
	"image/draw"

	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

var (
	errInvalid     = errors.New("ttf: invalid font")
	errUnsupported = errors.New("ttf: unsupported font")
	errNotFound    = errors.New("ttf: glyph not found")
)

// Limits from sfnt.
const (
	maxCmapSegments           = 20000
	maxCompoundRecursionDepth = 8
	maxCompoundStackSize      = 64
	maxGlyphDataLength        = 64 * 1024
)

// Font is a parsed font. It reads glyph data from the bytes it was parsed
// from, which must not change.
type Font struct {
	data        []byte
	unitsPerEm  fixed.Int26_6
	numHMetrics int
	hmtx        []byte
	locations   []uint32 // glyph data offsets into data; one more than glyphs
	cmap        []cmapEntry
	cmapIndexes []byte // the glyph index array after a format 4 cmap's segments
}

type cmapEntry struct{ end, start, delta, offset uint16 }

// Parse reads a TrueType font.
func Parse(data []byte) (*Font, error) {
	if len(data) < 12 || u32(data) != 0x00010000 {
		return nil, errUnsupported
	}
	tables, offsets := map[string][]byte{}, map[string]uint32{}
	n := int(u16(data[4:]))
	if len(data) < 12+16*n {
		return nil, errInvalid
	}
	for i := range n {
		rec := data[12+16*i:]
		off, size := u32(rec[8:]), u32(rec[12:])
		if uint64(off)+uint64(size) > uint64(len(data)) {
			return nil, errInvalid
		}
		tables[string(rec[:4])], offsets[string(rec[:4])] = data[off:off+size], off
	}
	head, hhea, maxp, loca, glyf := tables["head"], tables["hhea"], tables["maxp"], tables["loca"], tables["glyf"]
	if len(head) < 54 || len(hhea) < 36 || len(maxp) < 6 || glyf == nil {
		return nil, errInvalid
	}
	f := &Font{data: data, unitsPerEm: fixed.Int26_6(u16(head[18:])), numHMetrics: int(u16(hhea[34:])), hmtx: tables["hmtx"]}
	numGlyphs := int(u16(maxp[4:]))
	if f.unitsPerEm == 0 || f.numHMetrics == 0 || f.numHMetrics > numGlyphs || len(f.hmtx) < 4*f.numHMetrics {
		return nil, errInvalid
	}
	glyfOffset := offsets["glyf"]
	f.locations = make([]uint32, numGlyphs+1)
	if u16(head[50:]) != 0 {
		if len(loca) != 4*(numGlyphs+1) {
			return nil, errInvalid
		}
		for i := range f.locations {
			f.locations[i] = u32(loca[4*i:]) + glyfOffset
		}
	} else {
		if len(loca) != 2*(numGlyphs+1) {
			return nil, errInvalid
		}
		for i := range f.locations {
			f.locations[i] = 2*uint32(u16(loca[2*i:])) + glyfOffset
		}
	}
	if err := f.parseCmap(tables["cmap"]); err != nil {
		return nil, err
	}
	return f, nil
}

// parseCmap picks the first Unicode BMP subtable, as sfnt does for a font
// without full-repertoire subtables, and requires it to be format 4.
func (f *Font) parseCmap(cmap []byte) error {
	if len(cmap) < 4 {
		return errInvalid
	}
	n := int(u16(cmap[2:]))
	if len(cmap) < 4+8*n {
		return errInvalid
	}
	for i := range n {
		rec := cmap[4+8*i:]
		pid, psid, off := u16(rec), u16(rec[2:]), u32(rec[4:])
		unicodeBMP := pid == 0 && psid == 3 || pid == 3 && (psid == 0 || psid == 1)
		if !unicodeBMP {
			continue
		}
		if int64(off)+14 > int64(len(cmap)) {
			return errInvalid
		}
		if u16(cmap[off:]) != 4 {
			return errUnsupported
		}
		segCount := int(u16(cmap[off+6:]))
		if segCount&1 != 0 {
			return errInvalid
		}
		segCount /= 2
		if segCount > maxCmapSegments {
			return errUnsupported
		}
		segs := int(off) + 14
		indexes := segs + 8*segCount + 2
		if indexes > len(cmap) {
			return errInvalid
		}
		s := cmap[segs:]
		f.cmap = make([]cmapEntry, segCount)
		for j := range f.cmap {
			f.cmap[j] = cmapEntry{
				end:    u16(s[2*j:]),
				start:  u16(s[2*segCount+2+2*j:]),
				delta:  u16(s[4*segCount+2+2*j:]),
				offset: u16(s[6*segCount+2+2*j:]),
			}
		}
		f.cmapIndexes = cmap[indexes:]
		return nil
	}
	return errUnsupported
}

// index is the glyph for r; 0 (.notdef) when the font has none.
func (f *Font) index(r rune) (int, error) {
	if uint32(r) > 0xffff {
		return 0, nil
	}
	c := uint16(r)
	for i, j := 0, len(f.cmap); i < j; {
		h := i + (j-i)/2
		e := &f.cmap[h]
		switch {
		case c < e.start:
			j = h
		case e.end < c:
			i = h + 1
		case e.offset == 0:
			return int(c + e.delta), nil
		default:
			off := uint32(e.offset) + 2*uint32(h-len(f.cmap)+int(c-e.start))
			if off+2 > uint32(len(f.cmapIndexes)) {
				return 0, errInvalid
			}
			return int(u16(f.cmapIndexes[off:])), nil
		}
	}
	return 0, nil
}

// Face is a font at one size. It is not safe for concurrent use.
type Face struct {
	f     *Font
	scale fixed.Int26_6 // pixels per em
	segs  []segment
	stack [maxCompoundStackSize]component
	rast  vector.Rasterizer
	mask  image.Alpha
}

// NewFace returns f at size points and 72 DPI, as opentype.NewFace does.
func NewFace(f *Font, size float64) *Face {
	const dpi = 72
	return &Face{f: f, scale: fixed.Int26_6(0.5 + (size * dpi * 64 / 72))}
}

// BoundString is font.BoundString: the ink box of s drawn at the origin, and
// its advance.
func (fc *Face) BoundString(s string) (bounds fixed.Rectangle26_6, advance fixed.Int26_6) {
	for _, c := range s {
		x, _ := fc.f.index(c)
		b, a, err := fc.glyphBounds(x)
		if err != nil {
			b, a = fixed.Rectangle26_6{}, 0
		}
		if !b.Empty() {
			b.Min.X += advance
			b.Max.X += advance
			bounds = bounds.Union(b)
		}
		advance += a
	}
	return bounds, advance
}

// DrawString is font.Drawer.DrawString: it draws s with src through each
// glyph's coverage, starting at dot.
func (fc *Face) DrawString(dst draw.Image, src image.Image, dot fixed.Point26_6, s string) {
	for _, c := range s {
		dr, advance := fc.glyph(dot, c)
		if !dr.Empty() {
			draw.DrawMask(dst, dr, src, image.Point{}, &fc.mask, fc.mask.Rect.Min, draw.Over)
		}
		dot.X += advance
	}
}

func (fc *Face) glyphBounds(x int) (fixed.Rectangle26_6, fixed.Int26_6, error) {
	adv, err := fc.advance(x)
	if err != nil {
		return fixed.Rectangle26_6{}, 0, err
	}
	if err := fc.load(x); err != nil {
		return fixed.Rectangle26_6{}, 0, err
	}
	return bounds(fc.segs), adv, nil
}

// glyph rasterises c at dot into fc.mask, as opentype.Face.Glyph does, and
// returns where it goes and the advance.
func (fc *Face) glyph(dot fixed.Point26_6, c rune) (dr image.Rectangle, advance fixed.Int26_6) {
	x, err := fc.f.index(c)
	if err != nil {
		return image.Rectangle{}, 0
	}
	if advance, err = fc.advance(x); err != nil {
		return image.Rectangle{}, 0
	}
	if err := fc.load(x); err != nil {
		return image.Rectangle{}, 0
	}
	b := bounds(fc.segs).Add(dot)
	dr = image.Rect(b.Min.X.Floor(), b.Min.Y.Floor(), b.Max.X.Ceil(), b.Max.Y.Ceil())
	w, h := dr.Dx(), dr.Dy()
	if w < 0 || h < 0 {
		return image.Rectangle{}, 0
	}
	biasX := dot.X - fixed.Int26_6(dr.Min.X<<6)
	biasY := dot.Y - fixed.Int26_6(dr.Min.Y<<6)
	fc.mask = image.Alpha{Pix: make([]uint8, w*h), Stride: w, Rect: image.Rect(0, 0, w, h)}
	fc.rast.Reset(w, h)
	fc.rast.DrawOp = draw.Src
	px := func(p fixed.Point26_6) (float32, float32) {
		return float32(p.X+biasX) / 64, float32(p.Y+biasY) / 64
	}
	for _, s := range fc.segs {
		x0, y0 := px(s.args[0])
		switch s.op {
		case opMoveTo:
			fc.rast.MoveTo(x0, y0)
		case opLineTo:
			fc.rast.LineTo(x0, y0)
		case opQuadTo:
			x1, y1 := px(s.args[1])
			fc.rast.QuadTo(x0, y0, x1, y1)
		}
	}
	fc.rast.Draw(&fc.mask, fc.mask.Bounds(), image.Opaque, image.Point{})
	return dr, advance
}

// advance is glyph x's scaled advance width.
func (fc *Face) advance(x int) (fixed.Int26_6, error) {
	if x >= len(fc.f.locations)-1 {
		return 0, errNotFound
	}
	// Glyphs past the last hmtx record share its advance.
	x = min(x, fc.f.numHMetrics-1)
	return scale(fixed.Int26_6(u16(fc.f.hmtx[4*x:]))*fc.scale, fc.f.unitsPerEm), nil
}

// load sets fc.segs to glyph x's outline, scaled, with Y pointing down.
func (fc *Face) load(x int) error {
	fc.segs = fc.segs[:0]
	if err := fc.loadGlyf(x, 0, 0); err != nil {
		return err
	}
	for i := range fc.segs {
		a := &fc.segs[i].args
		for j := range a {
			a[j].X = +scale(a[j].X*fc.scale, fc.f.unitsPerEm)
			a[j].Y = -scale(a[j].Y*fc.scale, fc.f.unitsPerEm)
		}
	}
	return nil
}

// scale returns x divided by unitsPerEm, rounded to the nearest 1/64.
func scale(x, unitsPerEm fixed.Int26_6) fixed.Int26_6 {
	if x >= 0 {
		x += unitsPerEm / 2
	} else {
		x -= unitsPerEm / 2
	}
	return x / unitsPerEm
}

func u16(b []byte) uint16 { return uint16(b[0])<<8 | uint16(b[1]) }

func u32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
