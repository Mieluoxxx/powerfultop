package view

import (
	"fmt"

	"github.com/ELY-Open-Source/CrychicUI/choice"
	"github.com/egoist/mygo/ui"

	lucide "github.com/ELY-Open-Source/CrychicUI/icon"
	"powerfultop/internal/display"
)

// BrightnessSection is one BrightnessRow per display, as the popover shows them. builtInPowerW is the
// built-in panel's draw, the only one measurable (external monitors have their own supply).
func BrightnessSection(c *ui.Context, displays []display.Brightness, builtInPowerW *float64, setLevel func(id uint32, level float64)) {
	ui.Column(c).Gap(10).Children(func() {
		for _, d := range displays {
			var power *float64
			if d.BuiltIn {
				power = builtInPowerW
			}
			ui.Column(c.Key(d.ID)).Children(func() {
				BrightnessRow(c, d, power, func(level float64) { setLevel(d.ID, level) })
			})
		}
	})
}

// BrightnessRow is a display's name, how its brightness is controlled, and a slider.
func BrightnessRow(c *ui.Context, d display.Brightness, powerW *float64, onChange func(float64)) {
	t := c.Theme()
	name := d.Name
	if name == "" {
		name = L("Display", "显示器")
	}
	icon := "monitor"
	if d.BuiltIn {
		icon = "laptop"
	}
	ui.Column(c).Gap(4).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, lucide.Must(icon)).Size(13, 13).TextColor(Yellow.Color(c)).Shrink(0)
			ui.Text(c, name).FontSize(12).SingleLine().Shrink(1)
			badge := ui.Row(c).PaddingX(5).PaddingY(1).Radius(8).Background(t.Text.Alpha(0.08)).Tooltip(brightnessMethodHelp(d.Method))
			badge.Children(func() {
				label := ui.Text(c, brightnessMethodLabel(d.Method)).FontSize(9).FontWeight(500).NoWrap()
				if d.Method == display.Software {
					label.TextColor(Orange.Color(c))
				} else {
					label.TextColor(t.TextMuted)
				}
			})
			ui.Spacer(c)
			if powerW != nil {
				ui.Text(c, fmt.Sprintf("%.1f W", *powerW)).FontSize(12).TextColor(t.TextMuted).FontFeatures("tnum").NoWrap().
					Tooltip(L("Built-in display, included in system power", "内建屏幕功耗，已包含在系统功耗中"))
			}
			ui.Text(c, fmt.Sprintf("%.0f%%", d.Level*100)).FontSize(12).Bold().FontFeatures("tnum").NoWrap()
		})
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, lucide.Must("sun-dim")).Size(11, 11).TextColor(t.TextMuted).Shrink(0)
			percent := d.Level * 100
			if choice.ValueSlider(c, name, &percent, 0, 100, choice.ValueSliderOptions{Step: 1, Format: "%.0f%%"}).Grow(1).Changed() {
				onChange(percent / 100)
			}
			ui.Icon(c, lucide.Must("sun")).Size(12, 12).TextColor(t.TextMuted).Shrink(0)
		})
	})
}

func brightnessMethodLabel(m display.Method) string {
	switch m {
	case display.DDC:
		return L("DDC", "DDC")
	case display.Software:
		return L("Software", "软件")
	}
	return L("Native", "原生")
}

func brightnessMethodHelp(m display.Method) string {
	switch m {
	case display.DDC:
		return L("Backlight controlled over DDC/CI", "通过 DDC/CI 调节背光")
	case display.Software:
		return L("This monitor doesn't answer DDC/CI, so the picture is darkened instead of the backlight. Turn on DDC/CI in the monitor's menu or connect over USB-C or DisplayPort.",
			"这台显示器没有响应 DDC/CI，只能调暗画面，背光亮度不变。请在显示器菜单中开启 DDC/CI，或改用 USB-C / DisplayPort 连接。")
	}
	return L("Backlight controlled by macOS", "由 macOS 调节背光")
}
