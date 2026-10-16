package shortcuts

import (
	"bytes"
	"image"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"quartz/internal/instance"
)

// opentypeLetter is how the tile letter was drawn with x/image/font before
// internal/core/ttf replaced it; the tile must come out pixel for pixel the same.
func opentypeLetter(t *testing.T, img *image.NRGBA, s string, size float64) {
	f, err := opentype.Parse(gomedium.TTF)
	if err != nil {
		t.Fatal(err)
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		t.Fatal(err)
	}
	d := font.Drawer{Dst: img, Src: image.NewUniform(ink), Face: face}
	b, adv := d.BoundString(s)
	side := fixed.I(img.Bounds().Dx())
	d.Dot = fixed.Point26_6{X: (side - adv) / 2, Y: (side-(b.Max.Y-b.Min.Y))/2 - b.Min.Y}
	d.DrawString(s)
}

func TestTileLetterMatchesOpentype(t *testing.T) {
	for _, letter := range []string{"A", "g", "W", "Q", "Ж", "Ω", "é", "Å", "ß", "7", "@", "AB", "fi", "漢", "😀", "?", "ǅ"} {
		for _, side := range []int{iconSide, 64, 48} {
			got := drawTile(instance.Art{Background: "#3ecf8e", Initial: letter}, side)
			want := drawTile(instance.Art{Background: "#3ecf8e"}, side)
			opentypeLetter(t, want, letter, letterRatio*float64(side))
			if !bytes.Equal(got.Pix, want.Pix) {
				t.Errorf("%q at %d px: tile differs from the opentype rendering", letter, side)
			}
		}
	}
}
