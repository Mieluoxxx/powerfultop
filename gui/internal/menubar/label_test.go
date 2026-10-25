package menubar

import (
	"slices"
	"testing"
)

// Mirrors testMenuBarLabelStyles in the Swift regression tests.
func TestLabelStyles(t *testing.T) {
	if got := FromLegacy(LegacyBatteryPercent).Items; !slices.Equal(got, []Item{ItemBatteryPercent}) {
		t.Errorf("percent style migrates: %v", got)
	}
	if got := FromLegacy(LegacyBatteryPercentPower).Items; !slices.Equal(got, []Item{ItemBatteryPercent, ItemSystemPower}) {
		t.Errorf("percent + power migrates: %v", got)
	}
	powerOnly := FromLegacy(LegacyPowerOnly)
	if powerOnly.IconStyle != IconHidden || !slices.Equal(powerOnly.Items, []Item{ItemSystemPower}) {
		t.Errorf("power-only drops the icon and keeps wattage: %+v", powerOnly)
	}

	ctx := sampleContext()
	cfg := DefaultConfiguration()
	cfg.Items = []Item{ItemBatteryPercent, ItemSystemPower, ItemBatteryPower, ItemCPUUsage, ItemMemoryUsage, ItemCPUTemperature, ItemFanSpeed, ItemAdapterPower}
	label := NewLabel(cfg, ctx)
	if *label.Icon != *SymbolIcon("battery.50") {
		t.Errorf("battery icon follows level: %+v", *label.Icon)
	}
	var texts []string
	for _, s := range label.Segments {
		texts = append(texts, s.Text)
	}
	want := []string{"50%", " 7W", "+37W", " 5%", "43%", "43°", "   0"}
	if !slices.Equal(texts, want) {
		t.Errorf("values are padded to a fixed width; adapter power is hidden on battery:\n got %q\nwant %q", texts, want)
	}
	if label.Segments[1].Symbol != "laptopcomputer" {
		t.Errorf("system load carries its glyph: %q", label.Segments[1].Symbol)
	}

	empty := DefaultConfiguration()
	empty.IconStyle, empty.Items = IconHidden, nil
	if got := NewLabel(empty, ctx).Icon; got == nil || *got != *SymbolIcon("bolt.circle") {
		t.Errorf("an empty menu bar item keeps a clickable icon: %+v", got)
	}

	drawn := cfg
	drawn.IconStyle = IconIOS
	if got := NewLabel(drawn, ctx).Icon; *got != *BatteryIcon(IconIOS, 50, false) {
		t.Errorf("drawn styles carry the exact level: %+v", *got)
	}

	ctx.DataAvailable = false
	if got := NewLabel(cfg, ctx); *got.Icon != *SymbolIcon("exclamationmark.triangle") || len(got.Segments) != 0 {
		t.Errorf("warn when data is unavailable: %+v", got)
	}
}

func TestSystemPowerSegment(t *testing.T) {
	ctx := Context{DataAvailable: true, SystemPowerW: 18}
	if s, _ := SegmentFor(ItemSystemPower, ctx); s.Text != "18W" {
		t.Errorf("system power: %q", s.Text)
	}
	ctx.SystemPowerW, ctx.SystemPowerWarning = 123, true
	if s, _ := SegmentFor(ItemSystemPower, ctx); s.Text != "⚠ 99W" {
		t.Errorf("clamped with warning: %q", s.Text)
	}
	ctx.BatteryPowerW = ptr(-9.6)
	if s, _ := SegmentFor(ItemBatteryPower, ctx); s.Text != "−10W" {
		t.Errorf("discharge sign: %q", s.Text)
	}
	ctx.BatteryPowerW = ptr(0.02)
	if s, _ := SegmentFor(ItemBatteryPower, ctx); s.Text != " 0W" {
		t.Errorf("idle has no sign: %q", s.Text)
	}
}

func TestOnACShowsBolt(t *testing.T) {
	ctx := sampleContext()
	ctx.OnAC = true
	if got := NewLabel(DefaultConfiguration(), ctx).Icon; *got != *SymbolIcon("bolt.fill") {
		t.Errorf("simple style shows a bolt on AC: %+v", *got)
	}
	for percent, want := range map[int]string{0: "battery.0", 10: "battery.0", 11: "battery.25", 60: "battery.50", 85: "battery.75", 86: "battery.100"} {
		if got := BatteryIconName(percent); got != want {
			t.Errorf("%d%%: %s, want %s", percent, got, want)
		}
	}
}

func TestLabelEquality(t *testing.T) {
	cfg := DefaultConfiguration()
	a, b := NewLabel(cfg, sampleContext()), NewLabel(cfg, sampleContext())
	if !a.Equal(b) || a.Key() != b.Key() {
		t.Error("same inputs make equal labels")
	}
	ctx := sampleContext()
	ctx.BatteryPercent = 51
	c := NewLabel(cfg, ctx)
	if a.Equal(c) || a.Key() == c.Key() {
		t.Error("a new level makes a new label")
	}
}
