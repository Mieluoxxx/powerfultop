package barmanager

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// The glyphs are drawn at 2× in black on transparent; AppKit tints template images for the bar.
const imageScale = 2

type segment struct{ x0, y0, x1, y1 float64 }

// strokeImage draws round-capped strokes of width (in points) on a canvas of w×h points, at alpha.
func strokeImage(w, h float64, width, alpha float64, segments ...segment) []byte {
	pw, ph := int(w*imageScale), int(h*imageScale)
	img := image.NewNRGBA(image.Rect(0, 0, pw, ph))
	r := width * imageScale / 2
	for y := 0; y < ph; y++ {
		for x := 0; x < pw; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			d := math.Inf(1)
			for _, s := range segments {
				d = math.Min(d, distanceToSegment(px, py, s.x0*imageScale, s.y0*imageScale, s.x1*imageScale, s.y1*imageScale))
			}
			// One pixel of falloff antialiases the edge.
			if coverage := math.Min(math.Max(r+0.5-d, 0), 1); coverage > 0 {
				img.SetNRGBA(x, y, color.NRGBA{A: uint8(math.Round(coverage * alpha * 255))})
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func distanceToSegment(px, py, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	t := 0.0
	if l := dx*dx + dy*dy; l > 0 {
		t = math.Min(math.Max(((px-x0)*dx+(py-y0)*dy)/l, 0), 1)
	}
	return math.Hypot(px-(x0+t*dx), py-(y0+t*dy))
}

// Point sizes of the glyphs, matching the Swift app's NSImage sizes.
var (
	separatorSize = nsSize{8, 16}
	chevronSize   = nsSize{10, 16}
)

// separatorPNG is a half-opaque vertical bar.
func separatorPNG() []byte {
	return strokeImage(separatorSize.W, separatorSize.H, 2, 0.5, segment{4, 2, 4, 14})
}

// chevronPNG points left while collapsed (more icons that way) and right while expanded.
func chevronPNG(left bool) []byte {
	if left {
		return strokeImage(chevronSize.W, chevronSize.H, 1.8, 1, segment{6.5, 4.5, 3, 8}, segment{3, 8, 6.5, 11.5})
	}
	return strokeImage(chevronSize.W, chevronSize.H, 1.8, 1, segment{3.5, 4.5, 7, 8}, segment{7, 8, 3.5, 11.5})
}
