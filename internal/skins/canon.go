package skins

import (
	"errors"
	"image"
	"image/color"

	"quartz/internal/core/pngx"
)

// MaxSize caps a skin file before it is decoded. Real skins are a few KB.
const MaxSize = 256 << 10

// ErrNotSkin is returned for anything that isn't a 64×64 or 64×32 PNG.
var ErrNotSkin = errors.New("skins: not a Minecraft skin (a 64×64 or 64×32 PNG)")

// Canonical is a validated skin: re-encoded so only its pixels survive, named
// by the SHA-256 of that encoding.
type Canonical struct {
	PNG  []byte
	Hash string
	Slim bool
}

// Canonicalize validates data and re-encodes it. The header is checked before
// the image is decoded, so a huge declared size never allocates. Legacy 64×32
// skins are converted to the 64×64 layout, and fully transparent pixels are
// zeroed, so the same picture always gets the same hash.
func Canonicalize(data []byte) (Canonical, error) {
	src, err := pngx.Decode(data, pngx.Limits{MaxBytes: MaxSize, MaxSide: 64, PNGOnly: true})
	if err != nil || src.Bounds().Dx() != 64 || (src.Bounds().Dy() != 64 && src.Bounds().Dy() != 32) {
		return Canonical{}, ErrNotSkin
	}
	height := src.Bounds().Dy()
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := range height {
		for x := range 64 {
			c := color.NRGBAModel.Convert(src.At(src.Bounds().Min.X+x, src.Bounds().Min.Y+y)).(color.NRGBA)
			if c.A == 0 {
				c = color.NRGBA{}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	if height == 32 {
		upgradeLegacy(img)
	}
	data, hash, err := pngx.Encode(img)
	if err != nil {
		return Canonical{}, err
	}
	return Canonical{PNG: data, Hash: hash, Slim: isSlim(img)}, nil
}

// isSlim checks the column only classic arms use: with slim (3 px) arms the
// right arm's back face ends at x=53, so (54,20) stays transparent.
func isSlim(img *image.NRGBA) bool { return img.NRGBAAt(54, 20).A == 0 }

// upgradeLegacy fills the left arm and leg of a 64×32 skin by mirroring the
// right ones, as Minecraft does since 1.8.
func upgradeLegacy(img *image.NRGBA) {
	parts := [][6]int{ // source x, y, width, height, destination x, y
		{4, 16, 4, 4, 20, 48}, {8, 16, 4, 4, 24, 48}, // leg top, bottom
		{0, 20, 4, 12, 24, 52}, {4, 20, 4, 12, 20, 52}, {8, 20, 4, 12, 16, 52}, {12, 20, 4, 12, 28, 52}, // leg sides
		{44, 16, 4, 4, 36, 48}, {48, 16, 4, 4, 40, 48}, // arm top, bottom
		{40, 20, 4, 12, 40, 52}, {44, 20, 4, 12, 36, 52}, {48, 20, 4, 12, 32, 52}, {52, 20, 4, 12, 44, 52}, // arm sides
	}
	for _, p := range parts {
		sx, sy, w, h, dx, dy := p[0], p[1], p[2], p[3], p[4], p[5]
		for y := range h {
			for x := range w {
				img.SetNRGBA(dx+w-1-x, dy+y, img.NRGBAAt(sx+x, sy+y))
			}
		}
	}
}
