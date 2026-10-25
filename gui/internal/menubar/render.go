package menubar

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"math"
	"strconv"
	"sync"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/icons"
)

// MyGo's tray takes a PNG and scales it to 18 pt high, and can't show attributed titles or SF
// Symbols, so the whole label is drawn here: at 2x for Retina menu bars.
const (
	Scale    = 2
	heightPt = 18
	// iconGapPt is the gap NSStatusBarButton leaves between an image and a leading-image title.
	iconGapPt = 4
	// spaceWidthPt is a space at the 12 pt status font; segments were joined by a kerned space.
	spaceWidthPt = 3.4
	// thinSpacePt is the U+2009 the Swift app put between a glyph and its value.
	thinSpacePt = 2
)

var (
	black = ui.RGB(0, 0, 0)
	white = ui.RGB(255, 255, 255)
)

// labelColor is NSColor.labelColor in the given appearance.
func labelColor(dark bool) ui.Color {
	if dark {
		return white.Alpha(0.85)
	}
	return black.Alpha(0.85)
}

func systemGreen(dark bool) ui.Color {
	if dark {
		return ui.Hex("#30D158")
	}
	return ui.Hex("#34C759")
}

func systemRed(dark bool) ui.Color {
	if dark {
		return ui.Hex("#FF453A")
	}
	return ui.Hex("#FF3B30")
}

func windowBackground(dark bool) ui.Color {
	if dark {
		return ui.Hex("#323232")
	}
	return ui.Hex("#ECECEC")
}

// IsTemplate reports whether macOS should tint the label: every part is monochrome except the
// coloured battery.
func (l Label) IsTemplate() bool {
	return l.Icon == nil || l.Icon.Kind != IconDrawnBattery || l.Icon.Style != IconMacOSColor
}

// RenderLabel draws the label for cfg and ctx into a PNG for the tray. template tells to show it
// as a template image, which macOS tints for the menu bar; otherwise it is drawn for dark or light.
// Call it on the main thread; an unchanged label returns the previous PNG without drawing.
func RenderLabel(cfg Configuration, ctx Context, dark bool) (pngData []byte, template bool, err error) {
	return defaultRenderer.render(NewLabel(cfg, ctx), cfg.Spacing, dark)
}

// RenderLabelImage draws a label at Scale, for previews; nil when the label is empty.
func RenderLabelImage(l Label, spacing float64, dark bool) *image.RGBA {
	defaultRenderer.mu.Lock()
	defer defaultRenderer.mu.Unlock()
	return defaultRenderer.compose(l, spacing, dark)
}

// IconImage draws one icon at Scale, for previews such as the editor's style chips.
func IconImage(icon Icon, dark bool) *image.RGBA {
	defaultRenderer.mu.Lock()
	defer defaultRenderer.mu.Unlock()
	return defaultRenderer.icon(icon, dark)
}

// Tint recolors a template image with c, keeping its coverage, so it reads on any background.
func Tint(img *image.RGBA, c ui.Color) *image.RGBA {
	out := image.NewRGBA(img.Bounds())
	for i := 0; i < len(img.Pix); i += 4 {
		a := float64(img.Pix[i+3]) / 255 * float64(c.A) / 255
		out.Pix[i] = uint8(float64(c.R) * a)
		out.Pix[i+1] = uint8(float64(c.G) * a)
		out.Pix[i+2] = uint8(float64(c.B) * a)
		out.Pix[i+3] = uint8(255 * a)
	}
	return out
}

var defaultRenderer = &renderer{}

type renderer struct {
	mu       sync.Mutex
	layer    *layerTester
	text     *textTester
	lastKey  string
	lastPNG  []byte
	lastTmpl bool
}

func (r *renderer) render(l Label, spacing float64, dark bool) ([]byte, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	template := l.IsTemplate()
	key := l.Key() + "|" + strconv.FormatFloat(spacing, 'g', -1, 64)
	// Template images look the same in both appearances; only coloured ones depend on it.
	if !template {
		key += "|" + strconv.FormatBool(dark)
	}
	if key == r.lastKey && r.lastPNG != nil {
		return r.lastPNG, r.lastTmpl, nil
	}
	img := r.compose(l, spacing, dark)
	if img == nil {
		img = image.NewRGBA(image.Rect(0, 0, 1, heightPt*Scale))
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, false, err
	}
	r.lastKey, r.lastPNG, r.lastTmpl = key, b.Bytes(), template
	return r.lastPNG, template, nil
}

// compose lays the icon and the segments side by side, both centered in the 18 pt height.
func (r *renderer) compose(l Label, spacing float64, dark bool) *image.RGBA {
	color := black
	if !l.IsTemplate() {
		color = labelColor(dark)
	}
	var iconImg, textImg *image.RGBA
	if l.Icon != nil {
		iconImg = r.icon(*l.Icon, dark)
	}
	if len(l.Segments) > 0 {
		if r.text == nil {
			r.text = newTextTester()
		}
		textImg = r.text.render(l.Segments, spacing, color)
	}
	width := 0
	if iconImg != nil {
		width += iconImg.Bounds().Dx()
	}
	if textImg != nil {
		width += textImg.Bounds().Dx()
	}
	if iconImg != nil && textImg != nil {
		width += iconGapPt * Scale
	}
	if width == 0 {
		return nil
	}
	h := heightPt * Scale
	out := image.NewRGBA(image.Rect(0, 0, width, h))
	x := 0
	for _, part := range []*image.RGBA{iconImg, textImg} {
		if part == nil {
			continue
		}
		b := part.Bounds()
		y := (h - b.Dy()) / 2
		draw.Draw(out, image.Rect(x, y, x+b.Dx(), y+b.Dy()), part, b.Min, draw.Over)
		x += b.Dx() + iconGapPt*Scale
	}
	return out
}

// MARK: - Icons

// Geometry of MenuBarIconRenderer, in points with the y axis pointing down.
var (
	batteryCanvas = [2]float32{22 + 1.6 + 1, 13}
	batteryBody   = ui.Rect{X: 0.5, Y: 1, W: 22, H: 11}
	batteryCap    = ui.Rect{X: 23.3, Y: 4.5, W: 1.6, H: 4}
	boltInBody    = ui.Rect{X: 8, Y: 2, W: 7, H: 9}
)

func inset(r ui.Rect, d float32) ui.Rect {
	return ui.Rect{X: r.X + d, Y: r.Y + d, W: r.W - 2*d, H: r.H - 2*d}
}

func offset(r ui.Rect, by ui.Rect) ui.Rect {
	return ui.Rect{X: r.X + by.X, Y: r.Y + by.Y, W: r.W, H: r.H}
}

// boltPath is the Swift app's bolt in rect.
func boltPath(r ui.Rect) *ui.Path {
	return new(ui.Path).
		MoveTo(r.X+r.W*0.62, r.Y).
		LineTo(r.X, r.Y+r.H*0.58).
		LineTo(r.X+r.W*0.48, r.Y+r.H*0.58).
		LineTo(r.X+r.W*0.38, r.Y+r.H).
		LineTo(r.X+r.W, r.Y+r.H*0.42).
		LineTo(r.X+r.W*0.52, r.Y+r.H*0.42).
		Close()
}

func (r *renderer) icon(icon Icon, dark bool) *image.RGBA {
	if r.layer == nil {
		r.layer = newLayerTester()
	}
	l := r.layer
	switch icon.Kind {
	case IconDrawnBattery:
		switch icon.Style {
		case IconMacOS:
			return r.outlinedBattery(icon.Percent, icon.Charging, false, dark, 0.5)
		case IconMacOSColor:
			return r.outlinedBattery(icon.Percent, icon.Charging, true, dark, 0.55)
		case IconIOS:
			return r.iOSBattery(icon.Percent, icon.Charging)
		case IconRing:
			return l.render(14, 14, func(p *ui.Painter, b ui.Rect) { drawRing(p, b, icon.Percent, icon.Charging) }, nil)
		}
	case IconSymbol:
		switch icon.Symbol {
		case "battery.0", "battery.25", "battery.50", "battery.75", "battery.100":
			var level int
			switch icon.Symbol {
			case "battery.25":
				level = 25
			case "battery.50":
				level = 50
			case "battery.75":
				level = 75
			case "battery.100":
				level = 100
			}
			return l.render(batteryCanvas[0], batteryCanvas[1], func(p *ui.Painter, b ui.Rect) {
				drawOutline(p, b, black)
				if level > 0 {
					inner := offset(inset(batteryBody, 2), b)
					inner.W *= float32(level) / 100
					p.Fill(inner, black, 1.5)
				}
			}, nil)
		case "bolt.fill":
			return l.render(9, 13, func(p *ui.Painter, b ui.Rect) {
				p.FillPath(boltPath(offset(ui.Rect{X: 0.5, Y: 0.5, W: 8, H: 12}, b)), black)
			}, nil)
		case "bolt.circle":
			return l.render(15, 15, func(p *ui.Painter, b ui.Rect) {
				p.StrokePath(new(ui.Path).Circle(b.X+7.5, b.Y+7.5, 6.5), 1.3, black)
				p.FillPath(boltPath(offset(ui.Rect{X: 5, Y: 3.5, W: 5, H: 8}, b)), black)
			}, nil)
		default:
			name := LucideName(icon.Symbol)
			return l.render(15, 15, nil, func(c *ui.Context) {
				ui.Icon(c, icons.Get(name)).Size(15, 15).TextColor(black)
			})
		}
	}
	return nil
}

func drawOutline(p *ui.Painter, b ui.Rect, c ui.Color) {
	p.Stroke(offset(inset(batteryBody, 0.5), b), c, 3, 1)
	p.Fill(offset(batteryCap, b), c, 0.8)
}

// outlinedBattery is the macOS-style outline filled to the level. Template icons punch the bolt
// out of the fill so it reads on any menu bar; coloured icons draw it over a halo.
func (r *renderer) outlinedBattery(percent int, charging, colored, dark bool, outlineAlpha float32) *image.RGBA {
	l := r.layer
	outline := black.Alpha(outlineAlpha)
	fill := black
	if colored {
		outline = labelColor(dark).Alpha(outlineAlpha)
		fill = systemGreen(dark)
		if !charging && percent <= 20 {
			fill = systemRed(dark)
		}
	}
	w, h := batteryCanvas[0], batteryCanvas[1]
	base := l.render(w, h, func(p *ui.Painter, b ui.Rect) {
		drawOutline(p, b, outline)
		inner := offset(inset(batteryBody, 2), b)
		inner.W = max(1.5, inner.W*float32(percent)/100)
		p.Fill(inner, fill, 1.5)
		if charging && colored {
			bolt := boltPath(offset(boltInBody, b))
			p.StrokePath(bolt, 1.2, windowBackground(dark))
			p.FillPath(bolt, labelColor(dark))
		}
	}, nil)
	if !charging || colored {
		return base
	}
	halo := l.render(w, h, func(p *ui.Painter, b ui.Rect) {
		bolt := boltPath(offset(boltInBody, b))
		p.StrokePath(bolt, 1.3, black)
		p.FillPath(bolt, black)
	}, nil)
	bolt := l.render(w, h, func(p *ui.Painter, b ui.Rect) {
		p.FillPath(boltPath(offset(boltInBody, b)), black)
	}, nil)
	knockout(base, halo)
	draw.Draw(base, base.Bounds(), bolt, image.Point{}, draw.Over)
	return base
}

// iOSBattery is a solid body with the percentage knocked out of the filled part, like iOS.
func (r *renderer) iOSBattery(percent int, charging bool) *image.RGBA {
	l := r.layer
	w, h := batteryCanvas[0], batteryCanvas[1]
	base := l.render(w, h, func(p *ui.Painter, b ui.Rect) {
		body := offset(batteryBody, b)
		p.Fill(body, black.Alpha(0.35), 3.5)
		p.Fill(offset(batteryCap, b), black.Alpha(0.35), 0.8)
		p.Clip(body, 3.5, func() {
			p.Fill(ui.Rect{X: body.X, Y: body.Y, W: body.W * float32(percent) / 100, H: body.H}, black, 0)
		})
	}, nil)
	size := float32(9)
	if percent == 100 {
		size = 8
	}
	text := l.render(w, h, nil, func(c *ui.Context) {
		ui.Row(c).Width(batteryBody.W).Height(batteryBody.H).Center().Gap(0.5).Children(func() {
			ui.Textf(c, "%d", percent).FontSize(size).Bold().TextColor(black).NoWrap()
			if charging {
				ui.Box(c).Size(5, batteryBody.H-3).Draw(func(p *ui.Painter, b ui.Rect) {
					p.FillPath(boltPath(b), black)
				})
			}
		})
	})
	knockout(base, text)
	return base
}

func drawRing(p *ui.Painter, b ui.Rect, percent int, charging bool) {
	cx, cy, radius := b.X+7, b.Y+7, float32(5.5)
	p.StrokePath(new(ui.Path).Circle(cx, cy, radius), 2.2, black.Alpha(0.25))
	if percent > 0 {
		arc := new(ui.Path)
		const steps = 72
		sweep := 2 * math.Pi * float64(percent) / 100
		for i := 0; i <= steps; i++ {
			// From the top, clockwise on screen, as the Swift ring.
			a := -math.Pi/2 + sweep*float64(i)/steps
			x, y := cx+radius*float32(math.Cos(a)), cy+radius*float32(math.Sin(a))
			if i == 0 {
				arc.MoveTo(x, y)
			} else {
				arc.LineTo(x, y)
			}
		}
		p.StrokePath(arc, 2.2, black)
	}
	if charging {
		p.FillPath(boltPath(ui.Rect{X: cx - 2, Y: cy - 3, W: 4, H: 6}), black)
	}
}

// knockout clears base where mask covers it, as NSCompositingOperation.destinationOut did.
func knockout(base, mask *image.RGBA) {
	for i := 3; i < len(base.Pix) && i < len(mask.Pix); i += 4 {
		keep := 255 - uint32(mask.Pix[i])
		for j := i - 3; j <= i; j++ {
			base.Pix[j] = uint8(uint32(base.Pix[j]) * keep / 255)
		}
	}
}

// layerTester draws one layer of an icon off screen, with a transparent background.
type layerTester struct {
	t     *ui.Tester
	w, h  float32
	draw  func(p *ui.Painter, b ui.Rect)
	build func(c *ui.Context)
}

const layerCanvasPt = 32

func newLayerTester() *layerTester {
	l := &layerTester{}
	l.t = ui.NewTester(func(c *ui.Context) {
		transparent(c)
		box := ui.Box(c).Size(l.w, l.h)
		if l.draw != nil {
			box.Draw(l.draw)
		}
		if l.build != nil {
			box.Children(func() { l.build(c) })
		}
	}, layerCanvasPt, layerCanvasPt)
	l.t.SetScale(Scale)
	return l
}

// render returns a copy of the layer, w×h points at Scale.
func (l *layerTester) render(w, h float32, drawFn func(p *ui.Painter, b ui.Rect), build func(c *ui.Context)) *image.RGBA {
	l.w, l.h, l.draw, l.build = w, h, drawFn, build
	l.t.Frame()
	return crop(l.t.Image(), image.Rect(0, 0, int(math.Ceil(float64(w*Scale))), int(math.Ceil(float64(h*Scale)))))
}

// textTester lays the segments out on one line.
type textTester struct {
	t        *ui.Tester
	segments []Segment
	gap      float32
	color    ui.Color
	width    float32
}

const textCanvasPt = 640

func newTextTester() *textTester {
	tt := &textTester{}
	tt.t = ui.NewTester(func(c *ui.Context) {
		transparent(c)
		ui.Row(c).AlignSelf(ui.Start).Height(heightPt).AlignItems(ui.Center).Gap(tt.gap).
			Draw(func(_ *ui.Painter, b ui.Rect) { tt.width = b.W }).
			Children(func() {
				for i, s := range tt.segments {
					ui.Row(c).Key(i).AlignItems(ui.Center).Gap(thinSpacePt).Children(func() {
						if s.Symbol != "" {
							ui.Icon(c, icons.Get(LucideName(s.Symbol))).Size(10.5, 10.5).TextColor(tt.color)
						}
						ui.Text(c, s.Text).FontSize(12).FontWeight(500).FontFeatures("tnum").TextColor(tt.color).NoWrap()
					})
				}
			})
	}, textCanvasPt, heightPt)
	tt.t.SetScale(Scale)
	return tt
}

func (tt *textTester) render(segments []Segment, spacing float64, color ui.Color) *image.RGBA {
	tt.segments, tt.gap, tt.color = segments, float32(spacing)+spaceWidthPt, color
	tt.t.Frame()
	img := tt.t.Image()
	w := min(int(math.Ceil(float64(tt.width*Scale))), img.Bounds().Dx())
	return crop(img, image.Rect(0, 0, max(1, w), heightPt*Scale))
}

func crop(img *image.RGBA, r image.Rectangle) *image.RGBA {
	r = r.Intersect(img.Bounds())
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), img, r.Min, draw.Src)
	return out
}

func transparent(c *ui.Context) {
	theme := *c.Theme()
	theme.Background = ui.Color{}
	c.SetTheme(&theme)
}
