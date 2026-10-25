package view

import (
	"fmt"
	"image"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/icons"
	"powerfultop/internal/menubar"
)

// MenuBarPageState keeps the editor's previews between frames.
type MenuBarPageState struct {
	bitmaps map[string]*ui.Bitmap
}

// bitmap converts a drawn icon once per look; frames otherwise rebuild it every refresh.
func (s *MenuBarPageState) bitmap(key string, draw func() *image.RGBA) *ui.Bitmap {
	if s.bitmaps == nil || len(s.bitmaps) > 64 {
		s.bitmaps = map[string]*ui.Bitmap{}
	}
	if b, ok := s.bitmaps[key]; ok {
		return b
	}
	img := draw()
	if img == nil {
		return nil
	}
	b := ui.NewBitmap(img)
	s.bitmaps[key] = b
	return b
}

// MenuBarStyleTitle names an icon style.
func MenuBarStyleTitle(s menubar.IconStyle) string {
	switch s {
	case menubar.IconHidden:
		return L("Hidden", "不显示")
	case menubar.IconBattery:
		return L("Simple", "简约")
	case menubar.IconMacOS:
		return L("macOS Native", "macOS 原生")
	case menubar.IconMacOSColor:
		return L("macOS Color", "macOS 彩色")
	case menubar.IconIOS:
		return L("iOS Native", "iOS 原生")
	case menubar.IconRing:
		return L("Ring", "圆环")
	default:
		return L("PowerfulTop Icon", "PowerfulTop 图标")
	}
}

// MenuBarGroupTitle names a catalog group.
func MenuBarGroupTitle(g menubar.Group) string {
	switch g {
	case menubar.GroupAdapter:
		return L("Power Adapter", "电源适配器")
	case menubar.GroupHealth:
		return L("Battery Health", "电池健康")
	case menubar.GroupSystem:
		return L("System", "系统")
	default:
		return L("Battery", "电池")
	}
}

// MenuBarItemTitle names a catalog item.
func MenuBarItemTitle(i menubar.Item) string {
	switch i {
	case menubar.ItemBatteryPercent:
		return L("Percentage", "百分比")
	case menubar.ItemTimeRemaining:
		return L("Time Remaining", "满载/剩余时间")
	case menubar.ItemBatteryTemperature:
		return L("Temperature", "温度")
	case menubar.ItemBatteryCurrent, menubar.ItemAdapterCurrent:
		return L("Current", "电流")
	case menubar.ItemBatteryVoltage, menubar.ItemAdapterVoltage:
		return L("Voltage", "电压")
	case menubar.ItemBatteryPower, menubar.ItemAdapterPower:
		return L("Power", "功率")
	case menubar.ItemSystemPower:
		return L("System Load", "系统负载")
	case menubar.ItemHealth:
		return L("Health", "健康度")
	case menubar.ItemCycleCount:
		return L("Cycle Count", "循环次数")
	case menubar.ItemCPUUsage:
		return L("CPU Usage", "CPU 占用")
	case menubar.ItemMemoryUsage:
		return L("Memory", "内存")
	case menubar.ItemCPUTemperature:
		return L("CPU Temperature", "CPU 温度")
	default:
		return L("Fan Speed", "风扇转速")
	}
}

// MenuBarPage is the AlDente-style editor: selected items preview live values and reorder by
// dragging; the catalog toggles items on and off.
func MenuBarPage(c *ui.Context, settings *menubar.Settings, ctx menubar.Context, state *MenuBarPageState, width float32) {
	cfg := settings.Configuration()
	ui.Column(c).Gap(16).MaxWidth(760).Children(func() {
		menuBarSelected(c, settings, cfg, ctx, state)
		menuBarCatalog(c, settings, cfg, ctx, state)
		menuBarTuning(c, settings, cfg)
	})
}

func menuBarSelected(c *ui.Context, settings *menubar.Settings, cfg menubar.Configuration, ctx menubar.Context, state *MenuBarPageState) {
	t := c.Theme()
	label := menubar.NewLabel(cfg, ctx)
	Card(c, L("Selected Items", "已选菜单项"), "panel-top", Blue, func() {
		flow := ui.Row(c).Wrap().Gap(8)
		if _, over := ui.DragOver[menubar.Item](flow); over {
			flow.Background(t.Accent.Alpha(0.06)).Radius(12)
		}
		// Dropping past the last chip moves the item to the end.
		if item, ok := ui.Drop[menubar.Item](flow); ok {
			settings.Update(func(c *menubar.Configuration) { c.Move(item, "") })
		}
		flow.Children(func() {
			if cfg.IconStyle != menubar.IconHidden && label.Icon != nil {
				icon := *label.Icon
				menuBarChip(c, state, chipSpec{key: "icon", icon: &icon, filled: true})
			}
			for _, item := range cfg.Items {
				text := MenuBarItemTitle(item)
				if s, ok := menubar.SegmentFor(item, ctx); ok {
					text = s.Text
				}
				chip := menuBarChip(c, state, chipSpec{
					key: string(item), symbol: item.Symbol(), text: text, filled: true,
					onRemove: func() { settings.Update(func(c *menubar.Configuration) { c.Toggle(item) }) },
				}).Drag(item).Tooltip(MenuBarItemTitle(item))
				if chip.Dragging() {
					chip.Opacity(0.35)
				}
				if dragged, over := ui.DragOver[menubar.Item](chip); over && dragged != item {
					chip.Border(2, t.Accent.Alpha(0.6))
				}
				if dragged, ok := ui.Drop[menubar.Item](chip); ok {
					settings.Update(func(c *menubar.Configuration) { c.Move(dragged, item) })
				}
			}
		})
		if len(cfg.Items) == 0 && cfg.IconStyle == menubar.IconHidden {
			Note(c, L("Nothing selected; a small icon stays in the menu bar so PowerfulTop can still be opened.",
				"未选择任何项；菜单栏会保留一个小图标，以便仍能打开 PowerfulTop。"))
		} else {
			Note(c, L("Drag items to reorder. Readings that don't apply right now, such as adapter values on battery, are hidden.",
				"拖动可调整顺序。当前不适用的读数（例如使用电池时的适配器数值）会自动隐藏。"))
		}
	})
}

func menuBarCatalog(c *ui.Context, settings *menubar.Settings, cfg menubar.Configuration, ctx menubar.Context, state *MenuBarPageState) {
	Card(c, L("Item Catalog", "菜单项目录"), "library", Purple, func() {
		catalogGroup(c, L("Main Icon", "主图标样式"), L("(choose one)", "（选择一个）"), func() {
			for _, style := range menubar.IconStyles {
				spec := chipSpec{key: "style-" + string(style), text: MenuBarStyleTitle(style), filled: cfg.IconStyle == style}
				if style == menubar.IconHidden {
					spec.symbol = style.Symbol()
				} else {
					// Each style chip shows how that style would draw the current level.
					sample := cfg
					sample.IconStyle = style
					if icon := menubar.NewLabel(sample, ctx).Icon; icon != nil {
						spec.icon = icon
					}
				}
				if menuBarChip(c, state, spec).Clicked() {
					settings.Update(func(c *menubar.Configuration) { c.IconStyle = style })
				}
			}
		})
		for _, group := range menubar.Groups {
			catalogGroup(c, MenuBarGroupTitle(group), "", func() {
				for _, item := range menubar.Items {
					if item.Group() != group {
						continue
					}
					symbol := item.Symbol()
					if symbol == "" {
						symbol = "percent"
					}
					spec := chipSpec{key: "item-" + string(item), symbol: symbol, text: MenuBarItemTitle(item), filled: cfg.Contains(item)}
					if menuBarChip(c, state, spec).Clicked() {
						settings.Update(func(c *menubar.Configuration) { c.Toggle(item) })
					}
				}
			})
		}
		ui.Divider(c)
		ui.Row(c).Gap(8).Children(func() {
			ui.Spacer(c)
			if ui.Button(c, L("Reset", "重置")).Clicked() {
				settings.Set(menubar.DefaultConfiguration())
			}
			if ui.Button(c, L("Clear All", "全部清除")).Clicked() {
				settings.Update(func(c *menubar.Configuration) { c.Items = nil })
			}
		})
	})
}

func catalogGroup(c *ui.Context, title, subtitle string, chips func()) {
	ui.Column(c).Gap(8).PaddingY(2).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(12).Bold()
			if subtitle != "" {
				ui.Text(c, subtitle).FontSize(11).TextColor(c.Theme().TextMuted)
			}
		})
		ui.Row(c).Wrap().Gap(8).Children(chips)
	})
}

func menuBarTuning(c *ui.Context, settings *menubar.Settings, cfg menubar.Configuration) {
	Card(c, L("Layout", "布局"), "sliders-horizontal", Gray, func() {
		spacing := cfg.Spacing
		sliderRow(c, L("Item Spacing", "菜单项间距"), &spacing, menubar.SpacingRange, Lf("%.0f pt", "%.0f pt", spacing))
		if spacing != cfg.Spacing {
			settings.Update(func(c *menubar.Configuration) { c.Spacing = spacing })
		}
		interval := cfg.UpdateInterval
		sliderRow(c, L("Update Interval", "菜单更新间隔"), &interval, menubar.UpdateIntervalRange, Lf("%.0f s", "%.0f 秒", interval))
		if interval != cfg.UpdateInterval {
			settings.Update(func(c *menubar.Configuration) { c.UpdateInterval = interval })
		}
	})
}

func sliderRow(c *ui.Context, title string, value *float64, bounds [2]float64, formatted string) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(12).TextColor(c.Theme().TextMuted).Width(120).NoWrap()
		ui.StepSlider(c, value, bounds[0], bounds[1], 1).Grow(1).Label(title)
		ui.Text(c, formatted).FontSize(12).FontWeight(500).FontFeatures("tnum").Width(48).TextAlign(ui.End).NoWrap()
	})
}

type chipSpec struct {
	key string
	// icon is drawn as the menu bar would draw it; symbol is an SF Symbol name shown as its Lucide icon.
	icon     *menubar.Icon
	symbol   string
	text     string
	filled   bool
	onRemove func()
}

// menuBarChip is a capsule: filled with the accent while selected, outlined otherwise.
func menuBarChip(c *ui.Context, state *MenuBarPageState, spec chipSpec) ui.Element {
	t := c.Theme()
	fg := t.Accent
	if spec.filled {
		fg = t.AccentText
	}
	chip := ui.ButtonBase(c).Key(spec.key).PaddingX(12).PaddingY(6).Radius(999)
	if spec.filled {
		chip.Background(t.Accent)
	} else {
		chip.Border(1.5, t.Accent)
	}
	chip.Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			if spec.icon != nil {
				icon, dark := *spec.icon, t.Dark
				key := fmt.Sprint(menubar.Label{Icon: &icon}.Key(), fg)
				if b := state.bitmap(key, func() *image.RGBA {
					img := menubar.IconImage(icon, dark)
					if img == nil {
						return nil
					}
					if (menubar.Label{Icon: &icon}).IsTemplate() {
						img = menubar.Tint(img, fg)
					}
					return img
				}); b != nil {
					w, h := b.Size()
					ui.Image(c, b).Size(float32(w)/menubar.Scale, float32(h)/menubar.Scale)
				}
			} else if spec.symbol != "" {
				ui.Icon(c, icons.Get(menubar.LucideName(spec.symbol))).Size(12, 12).TextColor(fg)
			}
			if spec.text != "" {
				ui.Text(c, spec.text).FontSize(12).FontWeight(500).FontFeatures("tnum").TextColor(fg).NoWrap()
			}
			if spec.onRemove != nil {
				remove := ui.ButtonBase(c).Padding(2).Label(L("Remove", "移除")).Children(func() {
					ui.Icon(c, icons.Get("x")).Size(9, 9).TextColor(fg.Alpha(0.7))
				})
				if remove.Clicked() {
					spec.onRemove()
				}
			}
		})
	})
	return chip
}
