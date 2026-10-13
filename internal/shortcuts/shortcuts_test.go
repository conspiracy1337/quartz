package shortcuts

import (
	"bytes"
	"image/png"
	"testing"

	"quartz/internal/instance"
)

func TestPictureDrawsTheTile(t *testing.T) {
	data, err := Picture(instance.Instance{Art: instance.Art{Background: "#3ecf8e", Initial: "F"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != iconSide || b.Dy() != iconSide {
		t.Fatal(b)
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Fatal("the rounded corner must be transparent")
	}
	if r, g, b, _ := img.At(20, iconSide/2).RGBA(); r>>8 != 0x3e || g>>8 != 0xcf || b>>8 != 0x8e {
		t.Fatal("the tile must be the art colour")
	}
	dark := 0
	for x := range iconSide {
		if r, _, _, a := img.At(x, iconSide/2).RGBA(); a > 0 && r>>8 < 0x30 {
			dark++
		}
	}
	if dark == 0 {
		t.Fatal("the letter must be drawn across the middle")
	}
}
