package menubar

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Context is everything the menu bar can show, captured from the monitors by the app so labels
// stay pure and testable. A nil reading doesn't apply right now and its segment is hidden.
type Context struct {
	DataAvailable       bool
	OnAC                bool
	BatteryPercent      int
	TimeRemaining       *string
	BatteryTemperatureC *float64
	BatteryCurrentMA    *int
	BatteryVoltageMV    *int
	// BatteryPowerW is signed: positive into the pack.
	BatteryPowerW      *float64
	SystemPowerW       float64
	SystemPowerWarning bool
	AdapterCurrentA    *float64
	AdapterVoltageV    *float64
	AdapterPowerW      *float64
	Health             *int
	CycleCount         *int
	// CPUUsage and MemoryUsage are fractions, 0...1.
	CPUUsage        *float64
	MemoryUsage     *float64
	CPUTemperatureC *float64
	FanRPM          *float64
}

// Segment is one reading: an optional glyph (SF Symbol name) and its text.
type Segment struct {
	Symbol string
	Text   string
}

// IconKind tells a symbol icon from a battery drawn to the exact level.
type IconKind int

const (
	IconSymbol IconKind = iota + 1
	// IconDrawnBattery is drawn per refresh because it reflects the exact level rather than five symbol steps.
	IconDrawnBattery
)

// Icon is the label's leading icon; comparable.
type Icon struct {
	Kind     IconKind
	Symbol   string
	Style    IconStyle
	Percent  int
	Charging bool
}

// SymbolIcon and BatteryIcon build icons as the Swift enum cases did.
func SymbolIcon(name string) *Icon { return &Icon{Kind: IconSymbol, Symbol: name} }

func BatteryIcon(style IconStyle, percent int, charging bool) *Icon {
	return &Icon{Kind: IconDrawnBattery, Style: style, Percent: percent, Charging: charging}
}

// Label is the content of the status item.
type Label struct {
	Icon     *Icon
	Segments []Segment
}

// Equal compares labels by value; an unchanged label needs no redraw.
func (l Label) Equal(o Label) bool {
	if (l.Icon == nil) != (o.Icon == nil) || (l.Icon != nil && *l.Icon != *o.Icon) || len(l.Segments) != len(o.Segments) {
		return false
	}
	for i := range l.Segments {
		if l.Segments[i] != o.Segments[i] {
			return false
		}
	}
	return true
}

// Key is a string that differs exactly when labels differ, for caches.
func (l Label) Key() string {
	var b strings.Builder
	if l.Icon != nil {
		fmt.Fprintf(&b, "%d|%s|%s|%d|%t", l.Icon.Kind, l.Icon.Symbol, l.Icon.Style, l.Icon.Percent, l.Icon.Charging)
	}
	for _, s := range l.Segments {
		fmt.Fprintf(&b, "\x00%s\x01%s", s.Symbol, s.Text)
	}
	return b.String()
}

// NewLabel builds the label for a configuration and the current readings.
func NewLabel(cfg Configuration, ctx Context) Label {
	if !ctx.DataAvailable {
		return Label{Icon: SymbolIcon("exclamationmark.triangle")}
	}
	var segments []Segment
	for _, item := range cfg.Items {
		if s, ok := SegmentFor(item, ctx); ok {
			segments = append(segments, s)
		}
	}
	percent := min(100, max(0, ctx.BatteryPercent))
	label := Label{Segments: segments}
	switch cfg.IconStyle {
	case IconHidden:
		// An empty status item cannot be clicked, which would lock users out of the settings.
		if len(segments) == 0 {
			label.Icon = SymbolIcon("bolt.circle")
		}
	case IconBattery:
		if ctx.OnAC {
			label.Icon = SymbolIcon("bolt.fill")
		} else {
			label.Icon = SymbolIcon(BatteryIconName(percent))
		}
	case IconAppIcon:
		label.Icon = SymbolIcon("bolt.circle")
	default:
		label.Icon = BatteryIcon(cfg.IconStyle, percent, ctx.OnAC)
	}
	return label
}

// SegmentFor returns the item's segment; false when the reading doesn't apply right now (e.g.
// adapter values on battery).
func SegmentFor(item Item, ctx Context) (Segment, bool) {
	text, ok := segmentText(item, ctx)
	if !ok {
		return Segment{}, false
	}
	return Segment{Symbol: item.Symbol(), Text: text}, true
}

func segmentText(item Item, ctx Context) (string, bool) {
	round := func(v float64) int { return int(math.Round(v)) }
	switch item {
	case ItemBatteryPercent:
		return strconv.Itoa(min(100, max(0, ctx.BatteryPercent))) + "%", true
	case ItemTimeRemaining:
		if ctx.TimeRemaining == nil {
			return "", false
		}
		return *ctx.TimeRemaining, true
	case ItemBatteryTemperature:
		if ctx.BatteryTemperatureC == nil {
			return "", false
		}
		return Padded(round(*ctx.BatteryTemperatureC), 2) + "°", true
	case ItemBatteryCurrent:
		if ctx.BatteryCurrentMA == nil {
			return "", false
		}
		return fmt.Sprintf("%.1fA", float64(*ctx.BatteryCurrentMA)/1000), true
	case ItemBatteryVoltage:
		if ctx.BatteryVoltageMV == nil {
			return "", false
		}
		return fmt.Sprintf("%.1fV", float64(*ctx.BatteryVoltageMV)/1000), true
	case ItemBatteryPower:
		if ctx.BatteryPowerW == nil {
			return "", false
		}
		w := *ctx.BatteryPowerW
		sign := ""
		if w > 0.05 {
			sign = "+"
		} else if w < -0.05 {
			sign = "−"
		}
		return sign + Padded(round(math.Abs(w)), 2) + "W", true
	case ItemSystemPower:
		text := Padded(min(99, max(0, round(ctx.SystemPowerW))), 2) + "W"
		if ctx.SystemPowerWarning {
			text = "⚠ " + text
		}
		return text, true
	case ItemAdapterCurrent:
		if ctx.AdapterCurrentA == nil {
			return "", false
		}
		return fmt.Sprintf("%.1fA", *ctx.AdapterCurrentA), true
	case ItemAdapterVoltage:
		if ctx.AdapterVoltageV == nil {
			return "", false
		}
		return fmt.Sprintf("%.0fV", *ctx.AdapterVoltageV), true
	case ItemAdapterPower:
		if ctx.AdapterPowerW == nil {
			return "", false
		}
		return Padded(round(*ctx.AdapterPowerW), 2) + "W", true
	case ItemHealth:
		if ctx.Health == nil {
			return "", false
		}
		return strconv.Itoa(*ctx.Health) + "%", true
	case ItemCycleCount:
		if ctx.CycleCount == nil {
			return "", false
		}
		return strconv.Itoa(*ctx.CycleCount), true
	case ItemCPUUsage:
		if ctx.CPUUsage == nil {
			return "", false
		}
		return Padded(round(*ctx.CPUUsage*100), 2) + "%", true
	case ItemMemoryUsage:
		if ctx.MemoryUsage == nil {
			return "", false
		}
		return Padded(round(*ctx.MemoryUsage*100), 2) + "%", true
	case ItemCPUTemperature:
		if ctx.CPUTemperatureC == nil {
			return "", false
		}
		return Padded(round(*ctx.CPUTemperatureC), 2) + "°", true
	case ItemFanSpeed:
		if ctx.FanRPM == nil {
			return "", false
		}
		return Padded(round(*ctx.FanRPM), 4), true
	}
	return "", false
}

// FigureSpace is exactly one digit wide in a tabular-digit font, so padded values keep their
// width as they change and neighbouring menu bar items don't shift.
const FigureSpace = "\u2007"

// Padded left-pads value with figure spaces to digits characters.
func Padded(value, digits int) string {
	text := strconv.Itoa(value)
	return strings.Repeat(FigureSpace, max(0, digits-len(text))) + text
}

// BatteryIconName is the five-step battery symbol for a level.
func BatteryIconName(percent int) string {
	switch {
	case percent <= 10:
		return "battery.0"
	case percent <= 35:
		return "battery.25"
	case percent <= 60:
		return "battery.50"
	case percent <= 85:
		return "battery.75"
	default:
		return "battery.100"
	}
}
