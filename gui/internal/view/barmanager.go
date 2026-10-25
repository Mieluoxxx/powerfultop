package view

import (
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

// MenuBarManagerPage shows the hiding settings, the Ice/Thaw-style layout editor and how it works.
// Main reports the page's on-screen state to store.SetVisible, which holds the bar open while it shows.
func MenuBarManagerPage(c *ui.Context, m *barmanager.Manager, store *barmanager.ItemStore, state *ManagerPageState, width float32) {
	snap := m.Snapshot()
	ui.Column(c).Gap(16).MaxWidth(560).Children(func() {
		Card(c, L("Hidden Items", "图标隐藏"), "eye-off", Indigo, func() {
			enabled := snap.Enabled
			ui.Checkbox(c, &enabled, L("Enable Menu Bar Manager", "启用菜单栏管理")).FontSize(12).Disabled(!snap.Supported)
			if enabled != snap.Enabled {
				m.SetEnabled(enabled)
				store.Refresh()
			}
			switch {
			case !snap.Supported:
				Status(c, "triangle-alert", Orange, L("Requires macOS 27 or later.", "需要 macOS 27 或更高版本。"), "")
			case snap.Enabled:
				status := L("Icons Shown", "已显示")
				if snap.Collapsed {
					status = L("Icons Hidden", "已隐藏")
				}
				Row(c, L("Status", "状态"), status, "", true)
				managerAutoHideRow(c, m, snap)
			}
		})
		if snap.Supported {
			managerLayoutCard(c, m, store, state)
		}
		managerHowItWorks(c)
	})
}

func managerAutoHideRow(c *ui.Context, m *barmanager.Manager, snap barmanager.Snapshot) {
	labels := make([]string, len(barmanager.AutoHideChoices))
	selected := ""
	for i, d := range barmanager.AutoHideChoices {
		labels[i] = managerAutoHideLabel(d)
		if d == snap.AutoHideDelay {
			selected = labels[i]
		}
	}
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, L("Hide Again After", "自动重新隐藏")).FontSize(12).TextColor(c.Theme().TextMuted).NoWrap()
		chosen := selected
		if ui.Select(c, &chosen, labels).Changed() {
			for i, label := range labels {
				if label == chosen {
					m.SetAutoHideDelay(barmanager.AutoHideChoices[i])
				}
			}
		}
		ui.Spacer(c)
		label := L("Hide Items", "隐藏图标")
		if snap.Collapsed {
			label = L("Show Hidden Items", "显示隐藏的图标")
		}
		if ui.Button(c, label).Clicked() {
			m.Toggle()
		}
	})
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

func managerLayoutCard(c *ui.Context, m *barmanager.Manager, store *barmanager.ItemStore, state *ManagerPageState) {
	Card(c, L("Menu Bar Layout", "菜单栏布局"), "layout-panel-top", Purple, func() {
		snap := store.Snapshot()
		switch {
		case !m.Snapshot().Enabled:
			Note(c, L("Turn on the menu bar manager to arrange icons here.", "启用菜单栏管理后，即可在这里整理图标。"))
		case !snap.Trusted:
			Status(c, "lock", Orange, L("Arranging icons here needs Accessibility access.", "在这里整理图标需要辅助功能权限。"), "")
			ui.Row(c).Children(func() {
				if ui.Button(c, L("Grant Accessibility Access…", "授予辅助功能权限…")).Clicked() {
					store.RequestAccessibility()
				}
			})
			Note(c, L("macOS has no API for other apps' menu bar icons, so PowerfulTop reads them through Accessibility and moves them with the same ⌘-drag you would do.",
				"macOS 没有提供管理其他 app 菜单栏图标的接口，所以 PowerfulTop 通过辅助功能读取图标，并模拟你手动的 ⌘ 拖动来移动它们。"))
		default:
			managerLayoutAccessRow(c, store, snap)
			if snap.Crowded {
				Status(c, "fold-vertical", Orange, L("The active menu bar is too crowded to show the separator.", "当前活动的菜单栏太挤，连分隔符都放不下。"), "")
				Note(c, L("Click an empty spot in the menu bar of a wider display, or hide a few icons with ⌘-drag first.",
					"请在更宽的显示器的菜单栏空白处点一下，或先用 ⌘ 拖动隐藏几个图标。"))
				return
			}
			managerLayoutRows(c, store, snap, state)
		}
	})
}

func managerLayoutAccessRow(c *ui.Context, store *barmanager.ItemStore, snap barmanager.StoreSnapshot) {
	if snap.HasLayoutAccess {
		Status(c, "badge-check", Green, L("Using the menu bar layout file: icons move without the pointer, even behind the notch.",
			"已使用菜单栏布局文件：移动图标无需移动鼠标，被刘海遮住的图标也能移动。"), "")
		return
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Grow(1).Shrink(1).Children(func() {
			Status(c, "file-cog", Blue, L("Allow the menu bar layout file for reliable sorting and pointer-free moves.",
				"授权菜单栏布局文件后，分区判断更准确，移动图标也不用移动鼠标。"), "")
		})
		if ui.Button(c, L("Allow…", "授权…")).Clicked() {
			store.RequestLayoutAccess()
		}
	})
}

func managerLayoutRows(c *ui.Context, store *barmanager.ItemStore, snap barmanager.StoreSnapshot, state *ManagerPageState) {
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
	managerLayoutBar(c, store, state, L("Shown Icons", "显示区"), L("Always in the menu bar", "始终显示在菜单栏"), shown, barmanager.Shown)
	managerLayoutBar(c, store, state, L("Hidden Icons", "隐藏区"), L("Revealed with the chevron", "点击箭头时显示"), hidden, barmanager.Hidden)
	if len(absent) > 0 {
		managerLayoutBar(c, store, state, L("Not in Menu Bar", "未在菜单栏显示"), L("Usually switched off in System Settings", "通常是在系统设置中被关闭了"), absent, barmanager.Absent)
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		switch {
		case snap.Moving:
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, L("Moving…", "正在移动…")).FontSize(11).TextColor(c.Theme().TextMuted)
		case snap.Error != barmanager.NoError:
			Status(c, "triangle-alert", Orange, managerErrorText(snap.Error), "")
		}
	})
	Note(c, L("Drag an icon onto another to put it just left of that one, or onto an empty spot to move it to that row. Right-click an icon for more. The pointer jumps for a moment while an icon moves, and the menu bar stays expanded while this page is open.",
		"把图标拖到另一个图标上，就会放到它的左边；拖到空白处则移到该行。右键图标可查看更多操作。移动时鼠标指针会短暂跳动；此页面打开期间菜单栏会保持展开。"))
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

// managerLayoutBar draws one section like a strip of the menu bar, logos in their real left-to-right
// order, after Ice and Thaw.
func managerLayoutBar(c *ui.Context, store *barmanager.ItemStore, state *ManagerPageState, title, subtitle string, items []barmanager.Item, section barmanager.Section) {
	t := c.Theme()
	ui.Column(c).Key(int(section)).Gap(6).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(12).Bold()
			ui.Text(c, subtitle).FontSize(11).TextColor(t.TextMuted)
		})
		strip := ui.Row(c).Wrap().Gap(4).AlignItems(ui.Center).PaddingX(8).PaddingY(3).MinHeight(34).Radius(9)
		if section == barmanager.Absent {
			strip.Background(ui.RGBA(0, 0, 0, 0.35))
		} else {
			strip.Background(ui.RGBA(0, 0, 0, 0.72))
		}
		strip.Border(1, ui.RGBA(255, 255, 255, 0.08))
		if section != barmanager.Absent {
			if _, over := ui.DragOver[string](strip); over {
				strip.Border(2, t.Accent)
			}
			if id, ok := ui.Drop[string](strip); ok {
				store.Move(id, section, "")
			}
		}
		strip.Children(func() {
			for _, item := range items {
				managerItemTile(c, store, state, item)
			}
			if len(items) == 0 && section != barmanager.Absent {
				ui.Text(c, L("Drop icons here", "拖到这里")).FontSize(11).TextColor(ui.RGBA(255, 255, 255, 0.4)).Grow(1).TextAlign(ui.Center).AlignSelf(ui.Center)
			}
		})
	})
}

func managerItemTile(c *ui.Context, store *barmanager.ItemStore, state *ManagerPageState, item barmanager.Item) {
	t := c.Theme()
	title := item.Name
	if item.Detail != "" {
		title += " · " + item.Detail
	}
	movable := item.CanMove()
	help := title
	if !movable && item.Section != barmanager.Absent {
		help += "\n" + L("Behind the « button right now, so it can't be moved.", "目前被收在 « 按钮后面，暂时无法移动。")
	}
	tile := ui.Box(c).Key(item.ID).Size(28, 28).Center().Radius(6).Tooltip(help)
	if tile.Hovered() {
		tile.Background(ui.RGBA(255, 255, 255, 0.16))
	}
	if !movable {
		tile.Opacity(0.45)
	}
	if movable {
		tile.Drag(item.ID)
		// Shows where the dragged icon will land: just left of this one.
		if id, over := ui.DragOver[string](tile); over && id != item.ID {
			tile.Border(2, t.Accent)
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
			ui.Icon(c, icons.Get("app-window")).Size(18, 18).TextColor(ui.RGBA(255, 255, 255, 0.85))
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

func managerHowItWorks(c *ui.Context) {
	Card(c, L("How It Works", "使用方法"), "hand", Blue, func() {
		managerStep(c, 1, L("Hold ⌘ and drag menu bar icons to the left of the │ separator to choose which ones hide.",
			"按住 ⌘ 把菜单栏图标拖到 │ 分隔符左侧，就会被归入隐藏区域。"))
		managerStep(c, 2, L("Click the chevron to hide or show them. The macOS « button can't reveal them, because it shows the blank space that pushes them out first.",
			"点击箭头即可隐藏或显示这些图标。系统的 « 按钮展开后只会显示用来挤出图标的空白区域，所以请用箭头来显示它们。"))
		managerStep(c, 3, L("Right-click the chevron for more options.", "右键点击箭头查看更多选项。"))
		Note(c, L("Uses the macOS 27 menu bar itself, so no Screen Recording or Accessibility access is needed. Apple's own icons are switched on and off in System Settings.",
			"直接利用 macOS 27 菜单栏自身的机制，无需屏幕录制或辅助功能权限。Apple 自带的图标请在系统设置中开关。"))
		ui.Row(c).Children(func() {
			if ui.Button(c, L("Open System Settings…", "打开系统设置…")).Clicked() {
				_ = mygo.Shell.OpenExternal(managerControlCenterURL)
			}
		})
	})
}

func managerStep(c *ui.Context, number int, text string) {
	ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
		ui.Box(c).Size(16, 16).Center().Radius(8).Background(Blue.Color(c)).Shrink(0).Children(func() {
			ui.Textf(c, "%d", number).FontSize(10).Bold().TextColor(ui.RGB(255, 255, 255))
		})
		ui.Text(c, text).FontSize(12).Grow(1).Shrink(1)
	})
}
