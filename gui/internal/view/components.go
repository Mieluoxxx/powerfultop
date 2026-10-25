// Package view holds the dashboard's building blocks, ported from the Swift app's
// DashboardComponents, and the pages built from them.
package view

import (
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"

	"powerfultop/internal/history"
	"powerfultop/internal/icons"
)

// Tint is one of macOS's system colors, which differ between light and dark appearances.
type Tint int

const (
	Blue Tint = iota
	Green
	Orange
	Red
	Teal
	Purple
	Indigo
	Yellow
	Pink
	Gray
	Cyan
	Mint
)

var tints = map[Tint][2]string{
	Blue:   {"#007AFF", "#0A84FF"},
	Green:  {"#34C759", "#30D158"},
	Orange: {"#FF9500", "#FF9F0A"},
	Red:    {"#FF3B30", "#FF453A"},
	Teal:   {"#30B0C7", "#40C8E0"},
	Purple: {"#AF52DE", "#BF5AF2"},
	Indigo: {"#5856D6", "#5E5CE6"},
	Yellow: {"#FFCC00", "#FFD60A"},
	Pink:   {"#FF2D55", "#FF375F"},
	Gray:   {"#8E8E93", "#98989D"},
	Cyan:   {"#32ADE6", "#64D2FF"},
	Mint:   {"#00C7BE", "#63E6E2"},
}

// Color resolves the tint for the theme's appearance.
func (t Tint) Color(c *ui.Context) ui.Color {
	pair := tints[t]
	if c.Theme().Dark {
		return ui.Hex(pair[1])
	}
	return ui.Hex(pair[0])
}

// CardSurface is the raised card face: white on the light window background, a faint lift in dark mode.
func CardSurface(c *ui.Context) ui.Color {
	if c.Theme().Dark {
		return ui.RGBA(255, 255, 255, 0.055)
	}
	return ui.RGBA(255, 255, 255, 0.85)
}

// CardBorder outlines cards.
func CardBorder(c *ui.Context) ui.Color {
	if c.Theme().Dark {
		return ui.RGBA(255, 255, 255, 0.07)
	}
	return ui.RGBA(0, 0, 0, 0.07)
}

// Surface gives an element the card face and outline.
func Surface(c *ui.Context, e ui.Element, radius float32) ui.Element {
	return e.Background(CardSurface(c)).Border(1, CardBorder(c)).Radius(radius)
}

// Card is a titled card: a tinted icon badge, the title, then content.
func Card(c *ui.Context, title, icon string, tint Tint, content func()) ui.Element {
	col := tint.Color(c)
	return Surface(c, ui.Column(c).Padding(18).Gap(10), 18).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).PaddingY(1).Children(func() {
			ui.Row(c).Width(24).Height(24).Center().Radius(7).Background(col.Alpha(0.16)).Children(func() {
				ui.Icon(c, icons.Get(icon)).Size(13, 13).TextColor(col)
			})
			ui.Text(c, title).FontSize(13).Bold()
		})
		content()
	})
}

// Hero is a card's lead figure, e.g. "48.2 W / 90 W" with a caption underneath.
func Hero(c *ui.Context, value, rated, caption string, size float32) {
	t := c.Theme()
	if size == 0 {
		size = 28
	}
	ui.Column(c).Gap(2).Children(func() {
		ui.Row(c).Gap(4).AlignItems(ui.End).Children(func() {
			ui.Text(c, value).FontSize(size).Bold().FontFeatures("tnum").NoWrap()
			if rated != "" {
				ui.Text(c, "/ "+rated).FontSize(size*0.5).TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
			}
		})
		if caption != "" {
			ui.Text(c, caption).FontSize(11).TextColor(t.TextMuted)
		}
	})
}

// Row is "Label    value / rated", rated being the limit the value is measured against.
func Row(c *ui.Context, label, value, rated string, emphasized bool) {
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).NoWrap().Grow(1).Shrink(0)
		ui.Row(c).Gap(3).Shrink(1).Children(func() {
			v := ui.Text(c, value).FontSize(12).FontFeatures("tnum").SingleLine()
			if emphasized {
				v.Bold()
			} else {
				v.FontWeight(500)
			}
			if rated != "" {
				ui.Text(c, "/ "+rated).FontSize(12).TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
			}
		})
	})
}

// BarRow is a Row with a level bar under it; a nil fraction keeps the bar's room but hides it.
func BarRow(c *ui.Context, label, value string, fraction *float64, tint Tint) {
	ui.Column(c).Gap(4).Children(func() {
		Row(c, label, value, "", false)
		f := 0.0
		if fraction != nil {
			f = *fraction
		}
		bar := LevelBar(c, f, tint, 5)
		if fraction == nil {
			bar.Opacity(0)
		}
	})
}

// Note is a small, muted explanation under a card's content.
func Note(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(10).TextColor(c.Theme().TextMuted.Alpha(0.75))
}

// Status is one short line with an icon; long explanations go into the tooltip.
func Status(c *ui.Context, icon string, tint Tint, text, detail string) {
	if detail == "" {
		detail = text
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Tooltip(detail).Children(func() {
		ui.Icon(c, icons.Get(icon)).Size(12, 12).TextColor(tint.Color(c)).Shrink(0)
		ui.Text(c, text).FontSize(11).TextColor(c.Theme().TextMuted)
	})
}

// LevelBar is a capsule filled to fraction (0...1); the fill never shrinks below a circle.
func LevelBar(c *ui.Context, fraction float64, tint Tint, height float32) ui.Element {
	if height == 0 {
		height = 6
	}
	f := float32(math.Min(math.Max(fraction, 0), 1))
	track := ui.RGBA(128, 128, 128, 0.18)
	fill := tint.Color(c)
	return ui.Box(c).Height(height).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, track, height/2)
		w := max(height, r.W*f)
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: w, H: r.H}, fill, height/2)
	})
}

// StripItem is an icon, a label and a value in a horizontal strip; compact moves the label into the tooltip.
func StripItem(c *ui.Context, icon string, tint Tint, label, value string, compact bool, tooltip string) {
	t := c.Theme()
	help := tooltip
	if compact {
		help = label
		if tooltip != "" {
			help += "\n" + tooltip
		}
	}
	e := ui.Row(c).Gap(6).AlignItems(ui.Center).PaddingX(10)
	if help != "" {
		e.Tooltip(help)
	}
	e.Children(func() {
		ui.Icon(c, icons.Get(icon)).Size(12, 12).TextColor(tint.Color(c))
		if !compact {
			ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).NoWrap()
		}
		ui.Text(c, value).FontSize(12).Bold().FontFeatures("tnum").NoWrap()
	})
}

// StripDivider separates strip items.
func StripDivider(c *ui.Context) {
	ui.Box(c).Width(1).Height(14).Background(CardBorder(c))
}

// Point is one sample of a chart.
type Point = history.Point

// ChartPanel is a live value over the recent past: title, value, icon and a gradient area chart.
func ChartPanel(c *ui.Context, title, value, detail, icon string, tint Tint, points []Point, domain *[2]float64, collecting string) {
	t := c.Theme()
	Surface(c, ui.Column(c).Padding(16).Gap(10), 18).Children(func() {
		ui.Row(c).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Gap(2).Grow(1).Children(func() {
				ui.Text(c, title).FontSize(11).TextColor(t.TextMuted)
				ui.Text(c, value).FontSize(22).Bold().FontFeatures("tnum").NoWrap()
			})
			ui.Column(c).Gap(4).AlignItems(ui.End).Children(func() {
				ui.Icon(c, icons.Get(icon)).Size(14, 14).TextColor(tint.Color(c))
				if detail != "" {
					ui.Text(c, detail).FontSize(10).TextColor(t.TextMuted).FontFeatures("tnum").NoWrap()
				}
			})
		})
		if len(points) >= 2 {
			AreaChart(c, points, tint, domain, 110)
		} else {
			ui.Row(c).Height(110).Center().Children(func() {
				ui.Text(c, collecting).FontSize(11).TextColor(t.TextMuted)
			})
		}
	})
}

// AreaChart draws points as a gradient area under a smooth line, with a light grid on the right.
func AreaChart(c *ui.Context, points []Point, tint Tint, domain *[2]float64, height float32) {
	t := c.Theme()
	lo, hi := autoDomain(points)
	if domain != nil {
		lo, hi = domain[0], domain[1]
	}
	y := chart.NewLinear(lo, hi)
	x := chart.NewTime(points[0].Time, points[len(points)-1].Time)
	xs := make([]float32, len(points))
	ys := make([]float32, len(points))
	for i, p := range points {
		xs[i] = x.At(p.Time)
		ys[i] = y.At(math.Min(math.Max(p.Value, lo), hi))
	}
	ticks := y.Ticks(3)
	label := func(_ chart.Side, tick chart.Tick) {
		ui.Text(c, tick.Label).NoWrap().FontSize(9).TextColor(t.TextMuted)
	}
	col := tint.Color(c)
	chart.Plot(c, chart.Axes{Right: ticks, Label: label}, func(area ui.Element) {
		area.Draw(func(p *ui.Painter, r ui.Rect) {
			for _, tick := range ticks {
				p.Line(r.X, chart.Y(r, tick.At), r.X+r.W, chart.Y(r, tick.At), 1, t.Text.Alpha(0.06))
			}
			p.FillPathGradient(chart.Area(r, xs, nil, ys, chart.CurveMonotone),
				ui.LinearGradient{From: col.Alpha(0.35), To: col.Alpha(0.02), Angle: 180})
			p.StrokePath(chart.Line(r, xs, ys, chart.CurveMonotone), 1.6, col)
		})
	}).Height(height).GapX(6)
}

// autoDomain pads flat series so a steady 11.1 W doesn't render as a noisy full-height line.
func autoDomain(points []Point) (float64, float64) {
	low, high := math.Inf(1), math.Inf(-1)
	for _, p := range points {
		low, high = math.Min(low, p.Value), math.Max(high, p.Value)
	}
	pad := math.Max((high-low)*0.25, math.Max(math.Abs(high)*0.1, 1))
	return math.Max(0, low-pad), high + pad
}
