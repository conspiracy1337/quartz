package ttf

import "golang.org/x/image/math/fixed"

// Outlines from the glyf table, as sfnt's truetype.go reads them.

type segOp uint8

const (
	opMoveTo segOp = iota
	opLineTo
	opQuadTo
)

type segment struct {
	op   segOp
	args [2]fixed.Point26_6
}

// A component of a compound glyph.
type component struct {
	glyph              int
	dx, dy             int16
	hasTransform       bool
	txx, txy, tyx, tyy int16 // 2.14 fixed point
}

const (
	flagOnCurve      = 1 << 0
	flagXShortVector = 1 << 1
	flagYShortVector = 1 << 2
	flagRepeat       = 1 << 3
	// These two bits mean one thing with the short-vector bit and another
	// without it.
	flagPositiveXShortVector = 1 << 4
	flagThisXIsSame          = 1 << 4
	flagPositiveYShortVector = 1 << 5
	flagThisYIsSame          = 1 << 5
)

const (
	flagArg1And2AreWords   = 1 << 0
	flagArgsAreXYValues    = 1 << 1
	flagWeHaveAScale       = 1 << 3
	flagMoreComponents     = 1 << 5
	flagWeHaveAnXAndYScale = 1 << 6
	flagWeHaveATwoByTwo    = 1 << 7
)

const glyfHeaderLen = 10

func (fc *Face) glyphData(x int) ([]byte, error) {
	if x >= len(fc.f.locations)-1 {
		return nil, errNotFound
	}
	i, j := fc.f.locations[x], fc.f.locations[x+1]
	if j < i || j > uint32(len(fc.f.data)) {
		return nil, errInvalid
	}
	if j-i > maxGlyphDataLength {
		return nil, errUnsupported
	}
	return fc.f.data[i:j], nil
}

// loadGlyf appends glyph x's segments, in font units, to fc.segs.
func (fc *Face) loadGlyf(x int, stackBottom, depth int) error {
	data, err := fc.glyphData(x)
	if err != nil || len(data) == 0 {
		return err
	}
	if len(data) < glyfHeaderLen {
		return errInvalid
	}
	index := glyfHeaderLen
	numContours, numPoints := int16(u16(data)), 0
	switch {
	case numContours == -1:
		return fc.loadCompound(data[glyfHeaderLen:], stackBottom, depth)
	case numContours == 0:
		return nil
	case numContours > 0:
		index += 2 * int(numContours)
		if index > len(data) {
			return errInvalid
		}
		// The file stores the last point's index.
		numPoints = 1 + int(u16(data[index-2:]))
	default:
		return errInvalid
	}

	// Skip the hinting instructions.
	index += 2
	if index > len(data) {
		return errInvalid
	}
	index += int(u16(data[index-2:]))
	if index > len(data) {
		return errInvalid
	}

	// Points are stored in columns (flags, then x, then y); find where the
	// columns start, then read the points in order.
	xIndex, yIndex, ok := findXYIndexes(data, index, numPoints)
	if !ok {
		return errInvalid
	}
	g := glyfIter{
		data:        data,
		flagIndex:   int32(index),
		xIndex:      xIndex,
		yIndex:      yIndex,
		endIndex:    glyfHeaderLen,
		prevEnd:     -1,
		finalEnd:    int32(numPoints - 1),
		numContours: int32(numContours),
	}
	for g.nextContour() {
		for g.nextSegment() {
			fc.segs = append(fc.segs, g.seg)
		}
	}
	return g.err
}

func findXYIndexes(data []byte, index, numPoints int) (xIndex, yIndex int32, ok bool) {
	xDataLen, yDataLen := 0, 0
	for i := 0; ; {
		if i > numPoints {
			return 0, 0, false
		}
		if i == numPoints {
			break
		}
		repeatCount := 1
		if index >= len(data) {
			return 0, 0, false
		}
		flag := data[index]
		index++
		if flag&flagRepeat != 0 {
			if index >= len(data) {
				return 0, 0, false
			}
			repeatCount += int(data[index])
			index++
		}
		xSize := 0
		if flag&flagXShortVector != 0 {
			xSize = 1
		} else if flag&flagThisXIsSame == 0 {
			xSize = 2
		}
		xDataLen += xSize * repeatCount
		ySize := 0
		if flag&flagYShortVector != 0 {
			ySize = 1
		} else if flag&flagThisYIsSame == 0 {
			ySize = 2
		}
		yDataLen += ySize * repeatCount
		i += repeatCount
	}
	if index+xDataLen+yDataLen > len(data) {
		return 0, 0, false
	}
	return int32(index), int32(index + xDataLen), true
}

func (fc *Face) loadCompound(data []byte, stackBottom, depth int) error {
	if depth++; depth == maxCompoundRecursionDepth {
		return errUnsupported
	}
	// Read every component first: loading them reuses the stack above.
	stackTop := stackBottom
	for {
		if stackTop >= maxCompoundStackSize {
			return errUnsupported
		}
		c := &fc.stack[stackTop]
		stackTop++
		if len(data) < 4 {
			return errInvalid
		}
		flags := u16(data)
		c.glyph = int(u16(data[2:]))
		if flags&flagArg1And2AreWords == 0 {
			if len(data) < 6 {
				return errInvalid
			}
			c.dx, c.dy = int16(int8(data[4])), int16(int8(data[5]))
			data = data[6:]
		} else {
			if len(data) < 8 {
				return errInvalid
			}
			c.dx, c.dy = int16(u16(data[4:])), int16(u16(data[6:]))
			data = data[8:]
		}
		if flags&flagArgsAreXYValues == 0 {
			return errUnsupported
		}
		c.hasTransform = flags&(flagWeHaveAScale|flagWeHaveAnXAndYScale|flagWeHaveATwoByTwo) != 0
		switch {
		case flags&flagWeHaveAScale != 0:
			if len(data) < 2 {
				return errInvalid
			}
			c.txx, c.txy, c.tyx, c.tyy = int16(u16(data)), 0, 0, int16(u16(data))
			data = data[2:]
		case flags&flagWeHaveAnXAndYScale != 0:
			if len(data) < 4 {
				return errInvalid
			}
			c.txx, c.txy, c.tyx, c.tyy = int16(u16(data)), 0, 0, int16(u16(data[2:]))
			data = data[4:]
		case flags&flagWeHaveATwoByTwo != 0:
			if len(data) < 8 {
				return errInvalid
			}
			c.txx, c.txy, c.tyx, c.tyy = int16(u16(data)), int16(u16(data[2:])), int16(u16(data[4:])), int16(u16(data[6:]))
			data = data[8:]
		}
		if flags&flagMoreComponents == 0 {
			break
		}
	}

	for i := stackBottom; i < stackTop; i++ {
		c := &fc.stack[i]
		base := len(fc.segs)
		if err := fc.loadGlyf(c.glyph, stackTop, depth); err != nil {
			return err
		}
		dx, dy := fixed.Int26_6(c.dx), fixed.Int26_6(c.dy)
		for j := range fc.segs[base:] {
			a := &fc.segs[base+j].args
			for k := range a {
				if c.hasTransform {
					a[k] = tform(c.txx, c.txy, c.tyx, c.tyy, dx, dy, a[k])
				} else {
					a[k].X += dx
					a[k].Y += dy
				}
			}
		}
	}
	return nil
}

func tform(txx, txy, tyx, tyy int16, dx, dy fixed.Int26_6, p fixed.Point26_6) fixed.Point26_6 {
	const half = 1 << 13
	return fixed.Point26_6{
		X: dx +
			fixed.Int26_6((int64(p.X)*int64(txx)+half)>>14) +
			fixed.Int26_6((int64(p.Y)*int64(tyx)+half)>>14),
		Y: dy +
			fixed.Int26_6((int64(p.X)*int64(txy)+half)>>14) +
			fixed.Int26_6((int64(p.Y)*int64(tyy)+half)>>14),
	}
}

// bounds is the box around every segment point; empty for no segments.
func bounds(segs []segment) (b fixed.Rectangle26_6) {
	if len(segs) == 0 {
		return b
	}
	b.Min.X, b.Min.Y = fixed.Int26_6(1<<31-1), fixed.Int26_6(1<<31-1)
	b.Max.X, b.Max.Y = fixed.Int26_6(-1<<31), fixed.Int26_6(-1<<31)
	for _, s := range segs {
		n := 1
		if s.op == opQuadTo {
			n = 2
		}
		for _, p := range s.args[:n] {
			b.Max.X, b.Min.X = max(b.Max.X, p.X), min(b.Min.X, p.X)
			b.Max.Y, b.Min.Y = max(b.Max.Y, p.Y), min(b.Min.Y, p.Y)
		}
	}
	return b
}

// glyfIter turns a simple glyph's points into segments. Two on-curve points
// make a line; off-curve points make quadratic curves, with an implied
// on-curve point midway between two consecutive off-curve ones. A contour
// may start off-curve and wraps around to its first point.
type glyfIter struct {
	data []byte
	err  error

	flagIndex, xIndex, yIndex int32

	// endIndex is where the current contour's (inclusive) end point index is
	// stored; prevEnd is the previous contour's end, finalEnd the last one.
	endIndex, prevEnd, finalEnd int32

	c, numContours int32
	p, nPoints     int32

	x, y    int16
	on      bool
	flag    uint8
	repeats uint8

	closing, closed                                          bool
	firstOnCurveValid, firstOffCurveValid, lastOffCurveValid bool
	firstOnCurve, firstOffCurve, lastOffCurve                fixed.Point26_6
	seg                                                      segment
}

func (g *glyfIter) nextContour() bool {
	if g.c == g.numContours {
		if g.prevEnd != g.finalEnd {
			g.err = errInvalid
		}
		return false
	}
	g.c++
	end := int32(u16(g.data[g.endIndex:]))
	g.endIndex += 2
	if end <= g.prevEnd || g.finalEnd < end {
		g.err = errInvalid
		return false
	}
	g.nPoints = end - g.prevEnd
	g.p = 0
	g.prevEnd = end
	g.closing, g.closed = false, false
	g.firstOnCurveValid, g.firstOffCurveValid, g.lastOffCurveValid = false, false, false
	return true
}

func (g *glyfIter) close() {
	switch {
	case !g.firstOffCurveValid && !g.lastOffCurveValid:
		g.closed = true
		g.seg = segment{op: opLineTo, args: [2]fixed.Point26_6{g.firstOnCurve}}
	case !g.firstOffCurveValid && g.lastOffCurveValid:
		g.closed = true
		g.seg = segment{op: opQuadTo, args: [2]fixed.Point26_6{g.lastOffCurve, g.firstOnCurve}}
	case g.firstOffCurveValid && !g.lastOffCurveValid:
		g.closed = true
		g.seg = segment{op: opQuadTo, args: [2]fixed.Point26_6{g.firstOffCurve, g.firstOnCurve}}
	default:
		g.lastOffCurveValid = false
		g.seg = segment{op: opQuadTo, args: [2]fixed.Point26_6{g.lastOffCurve, midPoint(g.lastOffCurve, g.firstOffCurve)}}
	}
}

func (g *glyfIter) nextSegment() bool {
	for !g.closed {
		if g.closing || !g.nextPoint() {
			g.closing = true
			g.close()
			return true
		}
		// Font units for now; load scales them.
		p := fixed.Point26_6{X: fixed.Int26_6(g.x), Y: fixed.Int26_6(g.y)}
		switch {
		case !g.firstOnCurveValid:
			switch {
			case g.on:
				g.firstOnCurve, g.firstOnCurveValid = p, true
				g.seg = segment{op: opMoveTo, args: [2]fixed.Point26_6{p}}
				return true
			case !g.firstOffCurveValid:
				g.firstOffCurve, g.firstOffCurveValid = p, true
				continue
			default:
				g.firstOnCurve, g.firstOnCurveValid = midPoint(g.firstOffCurve, p), true
				g.lastOffCurve, g.lastOffCurveValid = p, true
				g.seg = segment{op: opMoveTo, args: [2]fixed.Point26_6{g.firstOnCurve}}
				return true
			}
		case !g.lastOffCurveValid:
			if !g.on {
				g.lastOffCurve, g.lastOffCurveValid = p, true
				continue
			}
			g.seg = segment{op: opLineTo, args: [2]fixed.Point26_6{p}}
			return true
		default:
			if !g.on {
				g.seg = segment{op: opQuadTo, args: [2]fixed.Point26_6{g.lastOffCurve, midPoint(g.lastOffCurve, p)}}
				g.lastOffCurve, g.lastOffCurveValid = p, true
				return true
			}
			g.seg = segment{op: opQuadTo, args: [2]fixed.Point26_6{g.lastOffCurve, p}}
			g.lastOffCurveValid = false
			return true
		}
	}
	return false
}

func (g *glyfIter) nextPoint() bool {
	if g.p == g.nPoints {
		return false
	}
	g.p++
	if g.repeats > 0 {
		g.repeats--
	} else {
		g.flag = g.data[g.flagIndex]
		g.flagIndex++
		if g.flag&flagRepeat != 0 {
			g.repeats = g.data[g.flagIndex]
			g.flagIndex++
		}
	}
	if g.flag&flagXShortVector != 0 {
		if g.flag&flagPositiveXShortVector != 0 {
			g.x += int16(g.data[g.xIndex])
		} else {
			g.x -= int16(g.data[g.xIndex])
		}
		g.xIndex++
	} else if g.flag&flagThisXIsSame == 0 {
		g.x += int16(u16(g.data[g.xIndex:]))
		g.xIndex += 2
	}
	if g.flag&flagYShortVector != 0 {
		if g.flag&flagPositiveYShortVector != 0 {
			g.y += int16(g.data[g.yIndex])
		} else {
			g.y -= int16(g.data[g.yIndex])
		}
		g.yIndex++
	} else if g.flag&flagThisYIsSame == 0 {
		g.y += int16(u16(g.data[g.yIndex:]))
		g.yIndex += 2
	}
	g.on = g.flag&flagOnCurve != 0
	return true
}

func midPoint(p, q fixed.Point26_6) fixed.Point26_6 {
	return fixed.Point26_6{X: (p.X + q.X) / 2, Y: (p.Y + q.Y) / 2}
}
