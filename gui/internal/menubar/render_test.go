package menubar

import (
	"bytes"
	"image/png"
	"testing"
)

func TestRenderLabel(t *testing.T) {
	cfg := DefaultConfiguration()
	cfg.Items = []Item{ItemBatteryPercent, ItemSystemPower}
	data, template, err := RenderLabel(cfg, sampleContext(), false)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dy() != heightPt*Scale || b.Dx() < 60*Scale || b.Dx() > 140*Scale {
		t.Errorf("a battery icon and two readings should be about 100 pt wide at 18 pt high, got %v", b)
	}
	if !template {
		t.Error("the simple style is monochrome, so macOS should tint it")
	}
	again, _, _ := RenderLabel(cfg, sampleContext(), true)
	if !bytes.Equal(data, again) {
		t.Error("an unchanged template label reuses the drawing in either appearance")
	}

	cfg.IconStyle = IconMacOSColor
	light, template, _ := RenderLabel(cfg, sampleContext(), false)
	dark, _, _ := RenderLabel(cfg, sampleContext(), true)
	if template || bytes.Equal(light, dark) {
		t.Error("the coloured battery is drawn per appearance, not as a template")
	}
}

// Figure spaces keep readings the same width as their values change.
func TestPaddedReadingsKeepWidth(t *testing.T) {
	cfg := DefaultConfiguration()
	cfg.IconStyle, cfg.Items = IconHidden, []Item{ItemSystemPower}
	ctx := sampleContext()
	ctx.SystemPowerW = 7
	narrow := RenderLabelImage(NewLabel(cfg, ctx), cfg.Spacing, false)
	ctx.SystemPowerW = 88
	wide := RenderLabelImage(NewLabel(cfg, ctx), cfg.Spacing, false)
	if narrow.Bounds().Dx() != wide.Bounds().Dx() {
		t.Errorf("7W is %d px, 88W %d px", narrow.Bounds().Dx(), wide.Bounds().Dx())
	}
}

func TestEveryIconDraws(t *testing.T) {
	icons := []Icon{*SymbolIcon("bolt.fill"), *SymbolIcon("bolt.circle"), *SymbolIcon("exclamationmark.triangle")}
	for _, name := range []string{"battery.0", "battery.25", "battery.50", "battery.75", "battery.100"} {
		icons = append(icons, *SymbolIcon(name))
	}
	for _, style := range []IconStyle{IconMacOS, IconMacOSColor, IconIOS, IconRing} {
		for _, charging := range []bool{false, true} {
			icons = append(icons, *BatteryIcon(style, 42, charging))
		}
	}
	for _, icon := range icons {
		img := IconImage(icon, false)
		if img == nil {
			t.Errorf("%+v drew nothing", icon)
			continue
		}
		opaque := 0
		for i := 3; i < len(img.Pix); i += 4 {
			if img.Pix[i] > 0 {
				opaque++
			}
		}
		if opaque == 0 {
			t.Errorf("%+v is blank", icon)
		}
	}
}
