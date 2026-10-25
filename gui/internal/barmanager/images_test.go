package barmanager

import (
	"bytes"
	"image/png"
	"testing"
)

func TestGlyphs(t *testing.T) {
	for name, data := range map[string][]byte{"separator": separatorPNG(), "left": chevronPNG(true), "right": chevronPNG(false)} {
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b := img.Bounds()
		opaque := 0
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
					opaque++
				}
			}
		}
		if opaque == 0 || opaque == b.Dx()*b.Dy() {
			t.Errorf("%s draws nothing or everything: %d of %d pixels", name, opaque, b.Dx()*b.Dy())
		}
	}
	// The chevron's tip is on the side it points to.
	left, _ := png.Decode(bytes.NewReader(chevronPNG(true)))
	if _, _, _, a := left.At(6, 16).RGBA(); a == 0 {
		t.Error("left chevron should have its tip near x=3pt")
	}
}
