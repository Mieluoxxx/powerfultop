// Package view holds the dashboard's building blocks, built from CrychicUI's components, and the
// pages put together from them.
package view

import (
	"math"
	"time"

	"github.com/ELY-Open-Source/CrychicUI/chart"
	"github.com/ELY-Open-Source/CrychicUI/display"
	"github.com/ELY-Open-Source/CrychicUI/feedback"
	lucide "github.com/ELY-Open-Source/CrychicUI/icon"
	"github.com/ELY-Open-Source/CrychicUI/kit"
	"github.com/ELY-Open-Source/CrychicUI/layout"
	"github.com/egoist/mygo/ui"

	"powerfultop/internal/history"
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

// tone maps a tint onto the library's semantic message tones, which color a message's icon.
func (t Tint) tone() feedback.Tone {
	switch t {
	case Red:
		return feedback.ToneDanger
	case Orange, Yellow:
		return feedback.ToneWarning
	case Green, Mint:
		return feedback.ToneSuccess
	}
	return feedback.ToneInfo
}

// CardSurface is the raised face the segmented controls lift their chosen segment onto.
func CardSurface(c *ui.Context) ui.Color {
	if c.Theme().Dark {
		return ui.RGBA(255, 255, 255, 0.055)
	}
	return ui.RGBA(255, 255, 255, 0.85)
}

// CardBorder outlines panels and separates rows.
func CardBorder(c *ui.Context) ui.Color { return c.Theme().Border }

// Surface gives an element the card face and outline, at the caller's radius.
func Surface(c *ui.Context, e ui.Element, radius float32) ui.Element {
	return e.Background(c.Theme().Background).Border(1, c.Theme().Border).Radius(radius).
		Shadow(0, 1, 3, 0, kit.Shade(0.06))
}

// Card is a titled card: a tinted icon badge, the title, then content.
func Card(c *ui.Context, title, icon string, tint Tint, content func()) ui.Element {
	t := c.Theme()
	return layout.Card(c, func() {
		PanelHeader(c, icon, title, tint, nil)
		ui.Column(c).Gap(t.Space(2)).Children(content)
	})
}

// Hero is a card's lead figure, e.g. "48.2 W / 90 W" with a caption underneath.
func Hero(c *ui.Context, value, rated, caption string, size float32) {
	t := c.Theme()
	if size == 0 {
		size = t.Rem(1.8)
	}
	ui.Column(c).Gap(t.Space(0.5)).Children(func() {
		ui.Row(c).Gap(t.Space(1)).AlignItems(ui.End).Children(func() {
			kit.Numeric(c, value).FontSize(size).Bold().NoWrap()
			if rated != "" {
				kit.Muted(c, "/ "+rated).FontSize(size * 0.5).NoWrap()
			}
		})
		if caption != "" {
			kit.Muted(c, caption).FontSize(t.Rem(0.8))
		}
	})
}

// Row is "Label    value / rated", rated being the limit the value is measured against.
func Row(c *ui.Context, label, value, rated string, emphasized bool) {
	t := c.Theme()
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		kit.Muted(c, label).FontSize(t.Rem(0.8)).Grow(1).Shrink(1)
		ui.Row(c).Gap(t.Space(0.75)).Shrink(1).Children(func() {
			v := kit.Numeric(c, value).FontSize(t.Rem(0.85)).ContentTransition(ui.ContentTransition{Numeric: true})
			if emphasized {
				v.Bold()
			} else {
				v.FontWeight(500)
			}
			if rated != "" {
				kit.Muted(c, "/ "+rated).FontSize(t.Rem(0.85)).NoWrap()
			}
		})
	})
}

// BarRow is a Row with a level bar under it; a nil fraction keeps the bar's room but hides it.
func BarRow(c *ui.Context, label, value string, fraction *float64, tint Tint) {
	t := c.Theme()
	ui.Column(c).Gap(t.Space(1)).Children(func() {
		Row(c, label, value, "", false)
		f := 0.0
		if fraction != nil {
			f = *fraction
		}
		bar := LevelBar(c, f, tint, t.Space(1.5))
		if fraction == nil {
			bar.Opacity(0)
		}
	})
}

// Note is a small, muted explanation under a card's content.
func Note(c *ui.Context, text string) {
	if text == "" {
		return
	}
	feedback.Hint(c, text)
}

// Status is one short line with an icon; long explanations go into the tooltip.
func Status(c *ui.Context, icon string, tint Tint, text, detail string) {
	if text == "" {
		return
	}
	message := feedback.InlineMessage(c, tint.tone(), text)
	if detail == "" || detail == text {
		message.Tooltip(text)
		return
	}
	message.Tooltip(detail)
}

// LevelBar is a capsule filled to fraction (0...1); the fill never shrinks below a circle. The
// library's bars are accent-only, and these encode severity, so the fill keeps its own color.
func LevelBar(c *ui.Context, fraction float64, tint Tint, height float32) ui.Element {
	if height == 0 {
		height = c.Theme().Space(1.5)
	}
	f := float32(math.Min(math.Max(fraction, 0), 1))
	track := c.Theme().Text.Alpha(0.08)
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
	e := ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).PaddingX(t.Space(2.5))
	if help != "" {
		e.Tooltip(help)
	}
	e.Children(func() {
		ui.Icon(c, lucide.Must(icon)).Size(12, 12).TextColor(tint.Color(c))
		if !compact {
			kit.Muted(c, label).FontSize(t.Rem(0.85)).NoWrap()
		}
		kit.Numeric(c, value).FontSize(t.Rem(0.85)).Bold().NoWrap()
	})
}

// StripDivider separates strip items.
func StripDivider(c *ui.Context) {
	ui.Box(c).Width(1).Height(c.Theme().Space(3.5)).Background(CardBorder(c))
}

// Point is one sample of a chart.
type Point = history.Point

// chartBuckets is how many samples a panel's chart shows: the library's charts label every sample,
// so 30 minutes of five-second readings are averaged down to a dozen readable points.
const chartBuckets = 6

// ChartPanel is a live value over the recent past: title, value, icon and an area chart.
func ChartPanel(c *ui.Context, title, value, detail, icon string, tint Tint, points []Point, collecting string) {
	t := c.Theme()
	card := kit.Card(c).FillWidth().Padding(t.Space(4)).Gap(t.Space(3))
	resource := kit.Arrive(card, ui.Pose{Y: t.Space(1), Fade: 1}, 0)
	resource.Children(func() {
		ui.Row(c).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Gap(t.Space(0.5)).Grow(1).Children(func() {
				kit.Muted(c, title).FontSize(t.Rem(0.8))
				kit.Numeric(c, value).FontSize(t.Rem(1.5)).Bold().NoWrap()
			})
			ui.Column(c).Gap(t.Space(1)).AlignItems(ui.End).Children(func() {
				ui.Icon(c, lucide.Must(icon)).Size(14, 14).TextColor(tint.Color(c))
				if detail != "" {
					kit.Muted(c, detail).FontSize(t.Rem(0.7)).NoWrap()
				}
			})
		})
		if len(points) < 2 {
			ui.Row(c).Height(t.Space(24)).Center().Children(func() {
				kit.Muted(c, collecting).FontSize(t.Rem(0.8))
			})
			return
		}
		labels, values := chartSamples(points)
		// A chart without a hover choice: each frame starts over, so a click leaves no marker behind.
		at := -1
		chart.Area(c, title, labels, []chart.Series{{Name: title, Values: values}}, &at,
			chart.AreaOptions{Height: t.Space(24), Format: "%.1f"})
	})
}

// chartLabels writes each sample's time; the library's charts need distinct labels, so samples
// closer together than a minute keep their seconds.
func chartLabels(times []time.Time) []string {
	labels := make([]string, len(times))
	layout := "15:04"
	seen := map[string]bool{}
	for _, t := range times {
		label := t.Format(layout)
		if seen[label] {
			layout = "15:04:05"
			break
		}
		seen[label] = true
	}
	for i, t := range times {
		labels[i] = t.Format(layout)
	}
	return labels
}

// chartSamples averages points into chartBuckets samples with time labels.
func chartSamples(points []Point) ([]string, []float64) {
	if len(points) <= chartBuckets {
		times := make([]time.Time, len(points))
		values := make([]float64, len(points))
		for i, p := range points {
			times[i], values[i] = p.Time, p.Value
		}
		return chartLabels(times), values
	}
	times := make([]time.Time, chartBuckets)
	values := make([]float64, chartBuckets)
	size := float64(len(points)) / chartBuckets
	for i := range chartBuckets {
		lo, hi := int(math.Floor(float64(i)*size)), int(math.Floor(float64(i+1)*size))
		hi = min(max(hi, lo+1), len(points))
		sum := 0.0
		for _, p := range points[lo:hi] {
			sum += p.Value
		}
		times[i] = points[lo].Time
		values[i] = sum / float64(hi-lo)
	}
	return chartLabels(times), values
}

// AreaChart draws points as an area chart, for the pages that chart a series of their own.
func AreaChart(c *ui.Context, label string, points []Point, tint Tint, height float32) {
	labels, values := chartSamples(points)
	at := -1
	chart.Area(c, label, labels, []chart.Series{{Name: label, Values: values}}, &at,
		chart.AreaOptions{Height: height, Format: "%.1f"})
}

// PanelHeader is a panel's tinted icon badge and title, with trailing content such as a Pill at the far end.
func PanelHeader(c *ui.Context, icon, title string, tint Tint, trailing func()) {
	t := c.Theme()
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		display.IconBox(c, icon, display.IconBoxOptions{Color: tint.Color(c)})
		ui.Text(c, title).FontSize(t.Rem(1)).Bold().Grow(1)
		if trailing != nil {
			trailing()
		}
	})
}

// Pill is a small tinted status capsule; Gray reads as neutral.
func Pill(c *ui.Context, icon, text string, tint Tint) ui.Element {
	col := tint.Color(c)
	if tint == Gray {
		col = c.Theme().TextMuted
	}
	t := c.Theme()
	return ui.Row(c).AlignSelf(ui.Start).Gap(t.Space(1)).AlignItems(ui.Center).
		Padding(t.Space(0.75), t.Space(2)).Radius(t.Radius).Background(col.Alpha(0.12)).Children(func() {
		ui.Icon(c, lucide.Must(icon)).Size(12, 12).TextColor(col).Shrink(0)
		kit.Numeric(c, text).FontSize(t.Rem(0.75)).Bold().TextColor(col).NoWrap()
	})
}
