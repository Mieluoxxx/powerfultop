package view

import (
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"powerfultop/internal/barmanager"
	"powerfultop/internal/icons"
)

const managerControlCenterURL = "x-apple.systempreferences:com.apple.ControlCenter-Settings.extension"

// ManagerPageState is what the menu bar manager page keeps between frames.
type ManagerPageState struct {
	// icons caches decoded app logos by app path.
	icons map[string]*ui.Bitmap
}

// MenuBarManagerPage puts the switches in a System Settings group, then the layout editor, which
// draws each section as a menu bar over a wallpaper the way Ice and Thaw do, then how it works.
// Main reports the page's on-screen state to store.SetVisible, which holds the bar open while it shows.
func MenuBarManagerPage(c *ui.Context, m *barmanager.Manager, store *barmanager.ItemStore, state *ManagerPageState, width float32) {
	snap := m.Snapshot()
	ui.Column(c).Gap(22).MaxWidth(880).Children(func() {
		managerSettings(c, m, store, snap)
		if snap.Supported {
			managerLayoutPanel(c, m, store, state)
		}
		managerHowItWorks(c, width)
	})
}

func managerSettings(c *ui.Context, m *barmanager.Manager, store *barmanager.ItemStore, snap barmanager.Snapshot) {
	t := c.Theme()
	rows := []func(){func() {
		label := L("Enable Menu Bar Manager", "启用菜单栏管理")
		ui.Column(c).Gap(2).Grow(1).Shrink(1).Children(func() {
			ui.Text(c, label).FontSize(13)
			ui.Text(c, L("Icons left of the │ separator tuck away behind a chevron.", "分隔符 │ 左侧的图标会收进箭头里。")).
				FontSize(11).TextColor(t.TextMuted)
		})
		enabled := snap.Enabled
		if ui.Switch(c.Key("enabled"), &enabled).Label(label).Disabled(!snap.Supported).Changed() {
			m.SetEnabled(enabled)
			store.Refresh()
		}
	}}
	if snap.Supported && snap.Enabled {
		rows = append(rows, func() {
			SectionLabel(c, L("Hidden Icons", "隐藏的图标"))
			if snap.Collapsed {
				Pill(c, "eye-off", L("Icons Hidden", "已隐藏"), Indigo)
			} else {
				Pill(c, "eye", L("Icons Shown", "已显示"), Gray)
			}
			label := L("Hide Items", "隐藏图标")
			if snap.Collapsed {
				label = L("Show Hidden Items", "显示隐藏的图标")
			}
			if ui.Button(c, label).Clicked() {
				m.Toggle()
			}
		}, func() {
			SectionLabel(c, L("Hide Again After", "自动重新隐藏"))
			labels := make([]string, len(barmanager.AutoHideChoices))
			chosen := ""
			for i, d := range barmanager.AutoHideChoices {
				labels[i] = managerAutoHideLabel(d)
				if d == snap.AutoHideDelay {
					chosen = labels[i]
				}
			}
			if ui.Select(c, &chosen, labels).Changed() {
				for i, label := range labels {
					if label == chosen {
						m.SetAutoHideDelay(barmanager.AutoHideChoices[i])
					}
				}
			}
		})
	}
	footer := ""
	if !snap.Supported {
		footer = L("Requires macOS 27 or later.", "需要 macOS 27 或更高版本。")
	}
	Section(c, L("Hiding", "图标隐藏"), footer, rows...)
}

// managerAutoHideLabel words a delay as the Swift app's DateComponentsFormatter did.
func managerAutoHideLabel(d time.Duration) string {
	switch {
	case d <= 0:
		return L("Never", "从不")
	case d < time.Minute:
		n := int(d / time.Second)
		if n == 1 {
			return L("1 second", "1秒钟")
		}
		return Lf("%d seconds", "%d秒钟", n)
	default:
		n := int(d / time.Minute)
		if n == 1 {
			return L("1 minute", "1分钟")
		}
		return Lf("%d minutes", "%d分钟", n)
	}
}

func managerLayoutPanel(c *ui.Context, m *barmanager.Manager, store *barmanager.ItemStore, state *ManagerPageState) {
	snap := store.Snapshot()
	enabled := m.Snapshot().Enabled
	Surface(c, ui.Column(c).Padding(18).Gap(16), 18).Children(func() {
		PanelHeader(c, "layout-panel-top", L("Menu Bar Layout", "菜单栏布局"), Purple, func() {
			if enabled && snap.Trusted {
				managerLayoutAccess(c, store, snap)
			}
		})
		switch {
		case !enabled:
			managerEmptyState(c, "eye-off", L("Turn on the menu bar manager to arrange icons here.", "启用菜单栏管理后，即可在这里整理图标。"), "")
		case !snap.Trusted:
			managerEmptyState(c, "lock", L("Arranging icons here needs Accessibility access.", "在这里整理图标需要辅助功能权限。"),
				L("macOS has no API for other apps' menu bar icons, so PowerfulTop reads them through Accessibility and moves them with the same ⌘-drag you would do.",
					"macOS 没有提供管理其他 app 菜单栏图标的接口，所以 PowerfulTop 通过辅助功能读取图标，并模拟你手动的 ⌘ 拖动来移动它们。"))
			ui.Row(c).Justify(ui.Center).Children(func() {
				if ui.PrimaryButton(c, L("Grant Accessibility Access…", "授予辅助功能权限…")).Clicked() {
					store.RequestAccessibility()
				}
			})
		case snap.Crowded:
			managerEmptyState(c, "fold-vertical", L("The active menu bar is too crowded to show the separator.", "当前活动的菜单栏太挤，连分隔符都放不下。"),
				L("Click an empty spot in the menu bar of a wider display, or hide a few icons with ⌘-drag first.",
					"请在更宽的显示器的菜单栏空白处点一下，或先用 ⌘ 拖动隐藏几个图标。"))
		default:
			managerLayoutSections(c, store, snap, state)
		}
	})
}

// managerLayoutAccess sits in the header: a badge once the layout file is allowed, else the button that asks for it.
func managerLayoutAccess(c *ui.Context, store *barmanager.ItemStore, snap barmanager.StoreSnapshot) {
	if snap.HasLayoutAccess {
		Pill(c, "badge-check", L("Layout file allowed", "布局文件已授权"), Green).
			Tooltip(L("Using the menu bar layout file: icons move without the pointer, even behind the notch.",
				"已使用菜单栏布局文件：移动图标无需移动鼠标，被刘海遮住的图标也能移动。"))
		return
	}
	if ui.Button(c, L("Allow Layout File…", "授权布局文件…")).
		Tooltip(L("Allow the menu bar layout file for reliable sorting and pointer-free moves.",
			"授权菜单栏布局文件后，分区判断更准确，移动图标也不用移动鼠标。")).Clicked() {
		store.RequestLayoutAccess()
	}
}

func managerEmptyState(c *ui.Context, icon, title, detail string) {
	t := c.Theme()
	ui.Column(c).Gap(8).AlignItems(ui.Center).PaddingY(18).PaddingX(24).Radius(12).Background(t.Text.Alpha(0.03)).Children(func() {
		ui.Icon(c, icons.Get(icon)).Size(22, 22).TextColor(t.TextMuted)
		ui.Text(c, title).FontSize(13).FontWeight(600).TextAlign(ui.Center)
		if detail != "" {
			ui.Text(c, detail).FontSize(11).TextColor(t.TextMuted).TextAlign(ui.Center).MaxWidth(520)
		}
	})
}

func managerLayoutSections(c *ui.Context, store *barmanager.ItemStore, snap barmanager.StoreSnapshot, state *ManagerPageState) {
	t := c.Theme()
	var shown, hidden, absent []barmanager.Item
	for _, it := range snap.Items {
		switch it.Section {
		case barmanager.Shown:
			shown = append(shown, it)
		case barmanager.Hidden:
			hidden = append(hidden, it)
		default:
			absent = append(absent, it)
		}
	}
	managerSection(c, store, state, barmanager.Shown, "eye", L("Shown Icons", "显示区"), L("Always in the menu bar", "始终显示在菜单栏"), shown)
	managerSection(c, store, state, barmanager.Hidden, "eye-off", L("Hidden Icons", "隐藏区"), L("Revealed with the chevron", "点击箭头时显示"), hidden)
	if len(absent) > 0 {
		managerSection(c, store, state, barmanager.Absent, "circle-slash", L("Not in Menu Bar", "未在菜单栏显示"), L("Usually switched off in System Settings", "通常是在系统设置中被关闭了"), absent)
	}
	switch {
	case snap.Moving:
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, L("Moving…", "正在移动…")).FontSize(11).TextColor(t.TextMuted)
		})
	case snap.Error != barmanager.NoError:
		Status(c, "triangle-alert", Orange, managerErrorText(snap.Error), "")
	}
	ui.Row(c).Gap(16).Wrap().Children(func() {
		managerTip(c, "move", L("Drop on an icon to place it to its left", "拖到图标上，放在它的左边"))
		managerTip(c, "mouse-pointer-click", L("Right-click for more", "右键查看更多操作"))
		managerTip(c, "panel-top", L("The menu bar stays expanded while this page is open", "此页面打开时菜单栏保持展开"))
	})
}

func managerTip(c *ui.Context, icon, text string) {
	t := c.Theme()
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icons.Get(icon)).Size(11, 11).TextColor(t.TextMuted.Alpha(0.8))
		ui.Text(c, text).FontSize(11).TextColor(t.TextMuted.Alpha(0.8)).NoWrap()
	})
}

func managerErrorText(e barmanager.StoreError) string {
	switch e {
	case barmanager.ErrNotInMenuBar:
		return L("This icon isn't in the menu bar right now, so it can't be moved.", "这个图标目前不在菜单栏上，无法移动。")
	case barmanager.ErrNoLayoutAccess:
		return L("PowerfulTop still can't read the menu bar layout file. Pick com.apple.MenuBar.plist in the folder that opens.",
			"PowerfulTop 仍然无法读取菜单栏布局文件，请在打开的文件夹中选择 com.apple.MenuBar.plist。")
	default:
		return L("The icon didn't move. It may be covered by the notch or the « overflow; try again on a wider display.",
			"图标没有移动成功，可能被刘海或 « 溢出区遮住了，请在更宽的显示器上再试一次。")
	}
}

// managerWallpaper stands in for the desktop behind the menu bar, so the strips read as menu bars.
func managerWallpaper(c *ui.Context) ui.LinearGradient {
	if c.Theme().Dark {
		return ui.LinearGradient{From: ui.RGB(38, 62, 128), To: ui.RGB(92, 52, 128), Angle: 90, Oklab: true}
	}
	return ui.LinearGradient{From: ui.RGB(96, 150, 240), To: ui.RGB(178, 128, 232), Angle: 90, Oklab: true}
}

// managerSection is one section: its title and count, then its icons on a menu bar in their real
// left-to-right order, right-aligned like the status area itself.
func managerSection(c *ui.Context, store *barmanager.ItemStore, state *ManagerPageState, section barmanager.Section, icon, title, subtitle string, items []barmanager.Item) {
	t := c.Theme()
	droppable := section != barmanager.Absent
	ui.Column(c.Key(int(section))).Gap(8).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Get(icon)).Size(13, 13).TextColor(t.TextMuted)
			ui.Text(c, title).FontSize(13).FontWeight(600)
			ui.Text(c, fmt.Sprint(len(items))).FontSize(10).FontWeight(600).TextColor(t.TextMuted).FontFeatures("tnum").
				Padding(1, 6).Radius(7).Background(t.Text.Alpha(0.07))
			ui.Spacer(c)
			ui.Text(c, subtitle).FontSize(11).TextColor(t.TextMuted)
		})
		frame := ui.Column(c).Radius(12).Clip()
		if droppable {
			frame.LinearGradient(managerWallpaper(c))
		} else {
			frame.Background(t.Text.Alpha(0.04)).Border(1, t.Text.Alpha(0.12)).BorderStyle(ui.BorderDashed)
		}
		if droppable {
			if _, over := ui.DragOver[string](frame); over {
				frame.Border(2, ui.RGBA(255, 255, 255, 0.9))
			}
			if id, ok := ui.Drop[string](frame); ok {
				store.Move(id, section, "")
			}
		}
		frame.Children(func() {
			// A mini desktop: the bar along the top edge and a strip of wallpaper under it.
			bar := ui.Row(c).Wrap().Gap(2).AlignItems(ui.Center).Justify(ui.End).PaddingX(10).PaddingY(3).MinHeight(36)
			if droppable {
				bar.Background(ui.RGBA(0, 0, 0, 0.3))
			}
			bar.Children(func() {
				for _, item := range items {
					managerItemTile(c, store, state, item)
				}
				if len(items) == 0 {
					color := t.TextMuted
					if droppable {
						color = ui.RGBA(255, 255, 255, 0.6)
					}
					ui.Text(c, L("Drop icons here", "拖到这里")).FontSize(11).TextColor(color).Grow(1).TextAlign(ui.Center)
				}
			})
			if droppable {
				ui.Box(c).Height(18)
			}
		})
	})
}

func managerItemTile(c *ui.Context, store *barmanager.ItemStore, state *ManagerPageState, item barmanager.Item) {
	title := item.Name
	if item.Detail != "" {
		title += " · " + item.Detail
	}
	movable := item.CanMove()
	help := title
	if !movable && item.Section != barmanager.Absent {
		help += "\n" + L("Behind the « button right now, so it can't be moved.", "目前被收在 « 按钮后面，暂时无法移动。")
	}
	tile := ui.Box(c).Key(item.ID).Size(32, 30).Center().Radius(6).Tooltip(help)
	if tile.Hovered() {
		tile.Background(ui.RGBA(255, 255, 255, 0.18))
	}
	if !movable {
		tile.Opacity(0.45)
	}
	if movable {
		tile.Drag(item.ID)
		// Shows where the dragged icon will land: just left of this one.
		if id, over := ui.DragOver[string](tile); over && id != item.ID {
			tile.Border(2, ui.RGBA(255, 255, 255, 0.9))
		}
		if id, ok := ui.Drop[string](tile); ok && id != item.ID {
			store.Move(id, item.Section, item.ID)
		}
		tile.ContextMenu(func(menu *ui.Menu) {
			menu.Item(title).Disabled(true)
			target, label := barmanager.Hidden, L("Move to Hidden", "移到隐藏区")
			if item.Section == barmanager.Hidden {
				target, label = barmanager.Shown, L("Move to Shown", "移到显示区")
			}
			if menu.Item(label).Chosen() {
				store.Move(item.ID, target, "")
			}
		})
	}
	tile.Children(func() {
		if bitmap := managerAppLogo(store, state, item); bitmap != nil {
			ui.Image(c, bitmap).Size(20, 20)
		} else {
			color := ui.RGBA(255, 255, 255, 0.85)
			if item.Section == barmanager.Absent {
				color = c.Theme().TextMuted
			}
			ui.Icon(c, icons.Get("app-window")).Size(18, 18).TextColor(color)
		}
	})
}

func managerAppLogo(store *barmanager.ItemStore, state *ManagerPageState, item barmanager.Item) *ui.Bitmap {
	if item.AppPath == "" {
		return nil
	}
	if bitmap, ok := state.icons[item.AppPath]; ok {
		return bitmap
	}
	png := store.Icon(item)
	if png == nil {
		return nil
	}
	bitmap, err := ui.DecodeBitmap(png)
	if err != nil {
		bitmap = nil
	}
	if state.icons == nil {
		state.icons = map[string]*ui.Bitmap{}
	}
	state.icons[item.AppPath] = bitmap
	return bitmap
}

// managerHowItWorks lays the three steps side by side, each a short card, with the note and the
// System Settings link as the section's footer.
func managerHowItWorks(c *ui.Context, width float32) {
	t := c.Theme()
	columns := 3
	if width < 640 {
		columns = 1
	}
	steps := []struct{ icon, title, text string }{
		{"command", L("⌘-drag to hide", "⌘ 拖动来隐藏"), L("Hold ⌘ and drag menu bar icons to the left of the │ separator.", "按住 ⌘ 把菜单栏图标拖到 │ 分隔符左侧。")},
		{"chevron-left", L("Click the chevron", "点击箭头"), L("It hides or shows them. The macOS « button can't reveal them.", "隐藏或显示它们。系统的 « 按钮无法显示这些图标。")},
		{"mouse-pointer-click", L("Right-click for more", "右键查看更多"), L("Right-click the chevron for more options.", "右键点击箭头查看更多选项。")},
	}
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, L("How It Works", "使用方法")).FontSize(13).FontWeight(600).PaddingX(10)
		ui.Grid(c).Columns(columns).Gap(12).Children(func() {
			for i, s := range steps {
				Surface(c, ui.Column(c.Key(i)).Padding(14).Gap(6), 12).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Row(c).Size(22, 22).Center().Radius(11).Background(Blue.Color(c).Alpha(0.15)).Children(func() {
							ui.Icon(c, icons.Get(s.icon)).Size(12, 12).TextColor(Blue.Color(c))
						})
						ui.Text(c, s.title).FontSize(12).FontWeight(600)
					})
					ui.Text(c, s.text).FontSize(11).TextColor(t.TextMuted)
				})
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).PaddingX(10).PaddingY(2).Children(func() {
			ui.Text(c, L("Hiding uses the macOS 27 menu bar itself, so it needs no Screen Recording or Accessibility access. Apple's own icons are switched on and off in System Settings.",
				"隐藏功能直接利用 macOS 27 菜单栏自身的机制，无需屏幕录制或辅助功能权限。Apple 自带的图标请在系统设置中开关。")).
				FontSize(11).TextColor(t.TextMuted).Grow(1).Shrink(1)
			if ui.Button(c, L("Open System Settings…", "打开系统设置…")).Clicked() {
				_ = mygo.Shell.OpenExternal(managerControlCenterURL)
			}
		})
	})
}
