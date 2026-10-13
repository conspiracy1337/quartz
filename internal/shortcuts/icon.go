package shortcuts

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"quartz/internal/core/fsx"
	"quartz/internal/core/pngx"
	"quartz/internal/icons"
	"quartz/internal/instance"
)

// iconSide is the drawn tile's size. A chosen picture is stored at
// icons.Side and used as it is.
const iconSide = 256

// Tile proportions from the design's .ico-xl (64 px, 12 px corners, 24 px letter).
const (
	cornerRatio = 12.0 / 64
	letterRatio = 24.0 / 64
)

var ink = color.NRGBA{R: 0x0a, G: 0x0a, B: 0x0b, A: 0xff} // the tile letter's colour

// Picture returns the instance's shortcut icon as a PNG: its chosen picture,
// or else its coloured tile with its letter, as the Library draws it.
func Picture(in instance.Instance, pictures *icons.Library) ([]byte, error) {
	if icons.ValidHash(in.Icon) && pictures != nil {
		if p, err := pictures.Path(in.Icon); err == nil {
			if data, err := fsx.ReadCapped(p, icons.MaxFile); err == nil {
				if img, err := pngx.Decode(data, pngx.Limits{MaxBytes: icons.MaxFile, MaxSide: 1024}); err == nil {
					out, _, err := pngx.Encode(pngx.Square(img, icons.Side))
					return out, err
				}
			}
		}
	}
	out, _, err := pngx.Encode(drawTile(in.Art, iconSide))
	return out, err
}

// drawTile draws a rounded square in art's colour with its letter.
func drawTile(art instance.Art, side int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	bg := parseHex(art.Background)
	r := cornerRatio * float64(side)
	const ss = 4 // 4×4 samples per pixel for smooth corners
	for y := range side {
		for x := range side {
			in := 0
			for sy := range ss {
				for sx := range ss {
					if insideRounded(float64(x)+(float64(sx)+0.5)/ss, float64(y)+(float64(sy)+0.5)/ss, float64(side), r) {
						in++
					}
				}
			}
			if in > 0 {
				c := bg
				c.A = uint8(int(bg.A) * in / (ss * ss)) //nolint:gosec // at most 255
				img.SetNRGBA(x, y, c)
			}
		}
	}
	if letter := strings.TrimSpace(art.Initial); letter != "" {
		drawLetter(img, letter, letterRatio*float64(side))
	}
	return img
}

func insideRounded(x, y, side, r float64) bool {
	cx := min(max(x, r), side-r)
	cy := min(max(y, r), side-r)
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= r*r
}

var (
	fontOnce sync.Once
	fontData *opentype.Font
)

func drawLetter(img *image.NRGBA, s string, size float64) {
	fontOnce.Do(func() { fontData, _ = opentype.Parse(gomedium.TTF) })
	if fontData == nil {
		return
	}
	face, err := opentype.NewFace(fontData, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return
	}
	defer func() { _ = face.Close() }()
	d := font.Drawer{Dst: img, Src: image.NewUniform(ink), Face: face}
	b, adv := d.BoundString(s)
	side := fixed.I(img.Bounds().Dx())
	// Centre the advance horizontally and the ink box vertically.
	d.Dot = fixed.Point26_6{X: (side - adv) / 2, Y: (side-(b.Max.Y-b.Min.Y))/2 - b.Min.Y}
	d.DrawString(s)
}

// parseHex reads #rrggbb; anything else is the default tile blue.
func parseHex(h string) color.NRGBA {
	if len(h) == 7 && h[0] == '#' {
		if v, err := strconv.ParseUint(h[1:], 16, 32); err == nil {
			return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff} //nolint:gosec // byte fields of a 24-bit value
		}
	}
	return color.NRGBA{R: 0x6b, G: 0x8a, B: 0xfd, A: 0xff}
}
