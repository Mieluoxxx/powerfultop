package barmanager

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// The glyph is drawn at 2× in black on transparent; AppKit tints template images for the bar.
const imageScale = 2

type segment struct{ x0, y0, x1, y1 float64 }

// strokeImage draws round-capped strokes of width (in points) on a canvas of w×h points.
func strokeImage(w, h, width float64, segments ...segment) []byte {
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
				img.SetNRGBA(x, y, color.NRGBA{A: uint8(math.Round(coverage * 255))})
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

// OverflowIcon is the tray's chevron as a template PNG: pointing down to the flyout it opens,
// and up while the flyout shows.
func OverflowIcon(open bool) []byte {
	const w, h = 14.0, 16.0
	if open {
		return strokeImage(w, h, 1.8, segment{3.5, 9.75, 7, 6.25}, segment{7, 6.25, 10.5, 9.75})
	}
	return strokeImage(w, h, 1.8, segment{3.5, 6.25, 7, 9.75}, segment{7, 9.75, 10.5, 6.25})
}
