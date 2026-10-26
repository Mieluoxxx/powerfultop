package view

import (
	"fmt"

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

// MenuBarManagerPage arranges the order of every app's menu bar icons. Hiding is left to macOS 27,
// which tucks the leftmost icons behind its « once the bar runs out of room, so the order is what
// decides which icons stay in sight. Main reports the page's on-screen state to store.SetVisible,
// which scans the bar only while the page shows.
func MenuBarManagerPage(c *ui.Context, store *barmanager.ItemStore, state *ManagerPageState, width float32) {
	snap := store.Snapshot()
	var absent []barmanager.Item
	for _, it := range snap.Items {
		if it.Section == barmanager.Absent {
			absent = append(absent, it)
		}
	}
	ui.Column(c).Gap(22).MaxWidth(880).Children(func() {
		managerLayoutPanel(c, store, snap, state)
		if len(absent) > 0 {
			managerAbsentPanel(c, store, state, absent)
		}
	})
}

func managerLayoutPanel(c *ui.Context, store *barmanager.ItemStore, snap barmanager.StoreSnapshot, state *ManagerPageState) {
	Surface(c, ui.Column(c).Padding(18).Gap(14), 18).Children(func() {
		PanelHeader(c, "layout-panel-top", L("Menu Bar Order", "菜单栏排序"), Purple, func() {
			if snap.Supported && snap.Trusted {
				managerLayoutAccess(c, store, snap)
			}
		})
		switch {
		case !snap.Supported:
			managerEmptyState(c, "triangle-alert", L("Requires macOS 27 or later.", "需要 macOS 27 或更高版本。"), "")
		case !snap.Trusted:
			managerEmptyState(c, "lock", L("Arranging icons here needs Accessibility access.", "在这里整理图标需要辅助功能权限。"),
				L("macOS has no API for other apps' menu bar icons, so PowerfulTop reads them through Accessibility and moves them with the same ⌘-drag you would do.",
					"macOS 没有提供管理其他 app 菜单栏图标的接口，所以 PowerfulTop 通过辅助功能读取图标，并模拟你手动的 ⌘ 拖动来移动它们。"))
			ui.Row(c).Justify(ui.Center).Children(func() {
				if ui.PrimaryButton(c, L("Grant Accessibility Access…", "授予辅助功能权限…")).Clicked() {
					store.RequestAccessibility()
				}
			})
		case len(snap.Items) == 0:
			ui.Row(c).Gap(8).AlignItems(ui.Center).Justify(ui.Center).PaddingY(24).Children(func() {
				ui.Spinner(c).Size(12, 12)
				ui.Text(c, L("Reading the menu bar…", "正在读取菜单栏…")).FontSize(12).TextColor(c.Theme().TextMuted)
			})
		default:
			managerBar(c, store, snap, state)
			managerStatus(c, snap)
			ui.Row(c).Gap(16).Wrap().Children(func() {
				managerTip(c, "move", L("Drop on an icon to place it to its left", "拖到图标上，放在它的左边"))
				managerTip(c, "arrow-right", L("Further right stays in sight longer", "越靠右越不容易被收起"))
				managerTip(c, "mouse-pointer-click", L("Right-click for more", "右键查看更多操作"))
			})
		}
	})
}

// managerLayoutAccess sits in the header: a badge once the layout file is allowed, else the button that asks for it.
func managerLayoutAccess(c *ui.Context, store *barmanager.ItemStore, snap barmanager.StoreSnapshot) {
	if snap.HasLayoutAccess {
		Pill(c, "badge-check", L("Layout file allowed", "布局文件已授权"), Green).
			Tooltip(L("Using the menu bar layout file: icons move without the pointer, even those behind «.",
				"已使用菜单栏布局文件：移动图标无需移动鼠标，收在 « 里的图标也能移动。"))
		return
	}
	if ui.Button(c, L("Allow Layout File…", "授权布局文件…")).
		Tooltip(L("Allow the menu bar layout file to move icons without the pointer, including those behind «.",
			"授权菜单栏布局文件后，移动图标不用移动鼠标，收在 « 里的图标也能移动。")).Clicked() {
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

func managerStatus(c *ui.Context, snap barmanager.StoreSnapshot) {
	switch {
	case snap.Moving:
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, L("Moving…", "正在移动…")).FontSize(11).TextColor(c.Theme().TextMuted)
		})
	case snap.Error != barmanager.NoError:
		Status(c, "triangle-alert", Orange, managerErrorText(snap.Error), "")
	}
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
		return L("The icon didn't move. Icons behind « can only be moved once the layout file is allowed.",
			"图标没有移动成功。收在 « 里的图标需要先授权布局文件才能移动。")
	}
}

// managerWallpaper stands in for the desktop behind the menu bar, so the strip reads as a menu bar.
func managerWallpaper(c *ui.Context) ui.LinearGradient {
	if c.Theme().Dark {
		return ui.LinearGradient{From: ui.RGB(38, 62, 128), To: ui.RGB(92, 52, 128), Angle: 90, Oklab: true}
	}
	return ui.LinearGradient{From: ui.RGB(96, 150, 240), To: ui.RGB(178, 128, 232), Angle: 90, Oklab: true}
}

// managerBar draws the bar as macOS lays it out: what « holds, then the icons in sight, then the
// clock standing in for Apple's own items, along the top of a strip of wallpaper.
func managerBar(c *ui.Context, store *barmanager.ItemStore, snap barmanager.StoreSnapshot, state *ManagerPageState) {
	var overflow, shown []barmanager.Item
	for _, it := range snap.Items {
		switch it.Section {
		case barmanager.Overflow:
			overflow = append(overflow, it)
		case barmanager.Shown:
			shown = append(shown, it)
		}
	}
	leftmost := ""
	if onBar := append(append([]barmanager.Item{}, overflow...), shown...); len(onBar) > 0 {
		leftmost = onBar[0].ID
	}
	white := func(a float32) ui.Color { return ui.RGBA(255, 255, 255, a) }
	frame := ui.Column(c).Radius(12).Clip().LinearGradient(managerWallpaper(c))
	if _, over := ui.DragOver[string](frame); over {
		frame.Border(2, white(0.9))
	}
	// Dropping anywhere but on an icon sends it to the right end, where it stays in sight longest.
	if id, ok := ui.Drop[string](frame); ok {
		store.Move(id, "")
	}
	frame.Children(func() {
		ui.Row(c).Wrap().Gap(2).AlignItems(ui.Center).Justify(ui.End).PaddingX(10).PaddingY(3).MinHeight(36).
			Background(ui.RGBA(0, 0, 0, 0.3)).Children(func() {
			if len(overflow) > 0 {
				ui.Row(c).Gap(2).AlignItems(ui.Center).PaddingStart(8).PaddingEnd(2).MarginEnd(6).Radius(8).Background(white(0.12)).
					Tooltip(L("macOS tucks these behind « when the menu bar runs out of room.", "菜单栏放不下时，macOS 把这些图标收进 «。")).
					Children(func() {
						ui.Text(c, "«").FontSize(14).Bold().TextColor(white(0.85)).PaddingEnd(4)
						for _, item := range overflow {
							managerItemTile(c, store, state, item, leftmost)
						}
					})
			}
			for _, item := range shown {
				managerItemTile(c, store, state, item, leftmost)
			}
			ui.Text(c, "9:41").FontSize(12).FontWeight(600).TextColor(white(0.7)).PaddingStart(10).PaddingEnd(2)
		})
		ui.Box(c).Height(18)
	})
}

func managerItemTile(c *ui.Context, store *barmanager.ItemStore, state *ManagerPageState, item barmanager.Item, leftmost string) {
	title := item.Name
	if item.Detail != "" {
		title += " · " + item.Detail
	}
	movable := item.CanMove()
	help := title
	if !movable && item.Section == barmanager.Overflow {
		help += "\n" + L("Behind « right now; allow the layout file to move it.", "目前收在 « 里，授权布局文件后才能移动。")
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
			store.Move(id, item.ID)
		}
		tile.ContextMenu(func(menu *ui.Menu) {
			menu.Item(title).Disabled(true)
			if menu.Item(L("Move to Right End (Stays in Sight)", "移到最右（优先显示）")).Chosen() {
				store.Move(item.ID, "")
			}
			if leftmost != "" && leftmost != item.ID && menu.Item(L("Move to Left End (Tucked Away First)", "移到最左（优先收起）")).Chosen() {
				store.Move(item.ID, leftmost)
			}
		})
	}
	tile.Children(func() { managerLogo(c, store, state, item, ui.RGBA(255, 255, 255, 0.85)) })
}

func managerLogo(c *ui.Context, store *barmanager.ItemStore, state *ManagerPageState, item barmanager.Item, fallback ui.Color) {
	if bitmap := managerAppLogo(store, state, item); bitmap != nil {
		ui.Image(c, bitmap).Size(20, 20)
	} else {
		ui.Icon(c, icons.Get("app-window")).Size(18, 18).TextColor(fallback)
	}
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

// managerAbsentPanel lists icons the bar doesn't draw at all; macOS switches them on and off, not the order.
func managerAbsentPanel(c *ui.Context, store *barmanager.ItemStore, state *ManagerPageState, items []barmanager.Item) {
	t := c.Theme()
	Surface(c, ui.Column(c).Padding(18).Gap(12), 18).Children(func() {
		PanelHeader(c, "circle-slash", L("Not in Menu Bar", "未在菜单栏显示"), Gray, func() {
			ui.Text(c, fmt.Sprint(len(items))).FontSize(11).FontWeight(600).TextColor(t.TextMuted).FontFeatures("tnum").
				Padding(1, 7).Radius(8).Background(t.Text.Alpha(0.07))
		})
		ui.Row(c).Wrap().Gap(2).AlignItems(ui.Center).PaddingX(8).PaddingY(4).MinHeight(38).Radius(10).
			Background(t.Text.Alpha(0.04)).Border(1, t.Text.Alpha(0.12)).BorderStyle(ui.BorderDashed).
			Children(func() {
				for _, item := range items {
					title := item.Name
					if item.Detail != "" {
						title += " · " + item.Detail
					}
					ui.Box(c).Key(item.ID).Size(32, 30).Center().Tooltip(title).Children(func() {
						managerLogo(c, store, state, item, t.TextMuted)
					})
				}
			})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, L("These apps' icons are switched off in System Settings › Menu Bar.", "这些 app 的图标在系统设置 › 菜单栏中被关闭了。")).
				FontSize(11).TextColor(t.TextMuted).Grow(1).Shrink(1)
			if ui.Button(c, L("Open System Settings…", "打开系统设置…")).Clicked() {
				_ = mygo.Shell.OpenExternal(managerControlCenterURL)
			}
		})
	})
}
