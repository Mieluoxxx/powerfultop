package view

import (
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/ELY-Open-Source/CrychicUI/button"
	"github.com/ELY-Open-Source/CrychicUI/choice"
	"github.com/ELY-Open-Source/CrychicUI/feedback"
	lucide "github.com/ELY-Open-Source/CrychicUI/icon"
	"powerfultop/internal/barmanager"
)

const managerControlCenterURL = "x-apple.systempreferences:com.apple.ControlCenter-Settings.extension"

// ManagerPageState is what the menu bar manager page keeps between frames.
type ManagerPageState struct {
	// icons caches decoded app logos by app path.
	icons map[string]*ui.Bitmap
}

// MenuBarManagerPage puts icons in the tray and arranges the order of the rest. macOS 27 tucks the
// leftmost icons behind its « once the bar runs out of room, so the order decides which icons stay
// in sight; the tray takes icons out of the bar altogether. Main reports the page's on-screen state
// to store.SetVisible, which scans the bar only while the page shows.
func MenuBarManagerPage(c *ui.Context, store *barmanager.ItemStore, tray *barmanager.TrayStore, state *ManagerPageState, width float32) {
	snap := store.Snapshot()
	var absent []barmanager.Item
	for _, it := range snap.Items {
		// The tray's apps are switched off too, but by the tray, not the user.
		if it.Section == barmanager.Absent && !tray.Contains(it.BundleID) {
			absent = append(absent, it)
		}
	}
	ui.Column(c).Gap(22).MaxWidth(880).Children(func() {
		managerTrayPanel(c, tray, snap, state)
		managerLayoutPanel(c, store, tray, snap, state)
		if len(absent) > 0 {
			managerAbsentPanel(c, store, state, absent)
		}
	})
}

// trayDrag is a tray icon being dragged, which the bar takes to put it back there.
type trayDrag struct{ id string }

func managerTrayPanel(c *ui.Context, tray *barmanager.TrayStore, snap barmanager.StoreSnapshot, state *ManagerPageState) {
	t := c.Theme()
	tsnap := tray.Snapshot()
	Surface(c, ui.Column(c).Padding(18).Gap(14), 18).Children(func() {
		PanelHeader(c, "archive", L("Tray", "托盘"), Indigo, func() {
			if tsnap.Enabled {
				managerAllowListAccess(c, tray, tsnap)
			}
			on := tsnap.Enabled
			if choice.Toggle(c, L("Use the Tray", "启用托盘"), &on).Changed() {
				tray.SetEnabled(on)
			}
		})
		ui.Text(c, L("Icons in the tray leave the menu bar and open from PowerfulTop's chevron, like the overflow of Windows' taskbar. macOS switches an app's icons together, so an app goes in whole. Quitting PowerfulTop puts them back.",
			"托盘里的图标会离开菜单栏，改从 PowerfulTop 的箭头图标里打开，就像 Windows 任务栏的溢出区。macOS 按 app 开关图标，所以同一个 app 的图标会一起收起。退出 PowerfulTop 时它们会回到菜单栏。")).
			FontSize(11).TextColor(t.TextMuted)
		if !tsnap.Enabled {
			return
		}
		zone := ui.Row(c).Wrap().Gap(4).AlignItems(ui.Center).Padding(8).MinHeight(54).Radius(12).
			Background(t.Text.Alpha(0.04)).Border(1, t.Text.Alpha(0.12)).BorderStyle(ui.BorderDashed)
		if id, over := ui.DragOver[string](zone); over {
			if it, ok := find(snap.Items, id); ok && tray.CanHold(it.BundleID) {
				zone.Border(2, Indigo.Color(c))
			}
		}
		if id, ok := ui.Drop[string](zone); ok {
			if it, ok := find(snap.Items, id); ok && tray.CanHold(it.BundleID) {
				tray.Add(it.BundleID)
			}
		}
		zone.Children(func() {
			if len(tsnap.Entries) == 0 {
				ui.Text(c, L("Drag icons here from the menu bar below, or right-click one there.", "从下方菜单栏把图标拖到这里，或在那里右键图标。")).
					FontSize(11).TextColor(t.TextMuted).TextAlign(ui.Center).Grow(1)
			}
			for _, e := range tsnap.Entries {
				managerTrayTile(c, tray, state, e)
			}
		})
		managerTrayStatus(c, tray, tsnap)
	})
}

func managerTrayTile(c *ui.Context, tray *barmanager.TrayStore, state *ManagerPageState, e barmanager.TrayEntry) {
	t := c.Theme()
	name := trayEntryName(e)
	help := name
	if e.SwitchedOn {
		help += "\n" + L("Switched back on in System Settings, so it shows in the menu bar.", "已在系统设置中重新打开，所以显示在菜单栏上。")
	}
	tile := ui.Box(c).Key("tray:"+e.ID).Size(36, 34).Center().Radius(8).Tooltip(help).Label(name).Drag(trayDrag{e.ID})
	if tile.Hovered() {
		tile.Background(t.Text.Alpha(0.08))
	}
	if !e.Running {
		tile.Opacity(0.5)
	}
	tile.ContextMenu(func(m *ui.Menu) {
		m.Item(name).Disabled(true)
		if e.SwitchedOn && m.Item(L("Hide Again", "重新收起")).Chosen() {
			tray.Rehide(e.ID)
		}
		if m.Item(L("Put Back in Menu Bar", "放回菜单栏")).Chosen() {
			tray.Remove(e.ID, "")
		}
	})
	tile.Children(func() {
		trayEntryLogo(c, tray, &state.icons, e, 22)
		if e.SwitchedOn {
			// A dot in the corner, like a notification badge.
			ui.Box(c).Size(8, 8).Radius(4).Background(Orange.Color(c)).Absolute().Top(2).Right(2)
		}
	})
}

// managerAllowListAccess sits in the tray's header: a badge once the list is allowed, else the button that asks for it.
func managerAllowListAccess(c *ui.Context, tray *barmanager.TrayStore, snap barmanager.TraySnapshot) {
	if snap.HasAllowListAccess {
		Pill(c, "badge-check", L("Switch file allowed", "开关文件已授权"), Green).
			Tooltip(L("Using the Allow in the Menu Bar list of System Settings to switch icons on and off.",
				"正在使用系统设置中「允许在菜单栏中显示」的列表来开关图标。"))
		return
	}
	if button.Outline(c, L("Allow Switch File…", "授权开关文件…")).
		Tooltip(L("Allow the Allow in the Menu Bar list, which the tray switches apps' icons with. Apple's own icons need no file.",
			"授权「允许在菜单栏中显示」的列表，托盘用它开关 app 的图标。系统自带的图标不需要这个文件。")).Clicked() {
		tray.RequestAllowListAccess()
	}
}

func managerTrayStatus(c *ui.Context, tray *barmanager.TrayStore, snap barmanager.TraySnapshot) {
	for _, e := range snap.Entries {
		if e.SwitchedOn {
			Status(c, "triangle-alert", Orange, L("Some icons were switched back on in System Settings. Right-click one to hide it again or put it back.",
				"有图标在系统设置中被重新打开了。右键它，可以重新收起或放回菜单栏。"), "")
			break
		}
	}
	switch snap.Error {
	case barmanager.ErrTrayNoAccess:
		Status(c, "lock", Orange, L("PowerfulTop can't read the Allow in the Menu Bar list yet. Allow the switch file above.",
			"PowerfulTop 还无法读取「允许在菜单栏中显示」的列表，请先授权上方的开关文件。"), "")
	case barmanager.ErrTrayUntracked:
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).Shrink(1).Children(func() {
				Status(c, "triangle-alert", Orange, L("macOS hasn't listed this app under Allow in the Menu Bar yet.", "macOS 还没有把这个 app 列进「允许在菜单栏中显示」。"), "")
			})
			if button.Outline(c, L("Refresh Menu Bar", "刷新菜单栏")).
				Tooltip(L("Restarts the menu bar so macOS lists the app; the menu bar blinks once.", "重启菜单栏让 macOS 列出这个 app，菜单栏会闪一下。")).Clicked() {
				tray.RefreshMenuBar()
			}
		})
	case barmanager.ErrTrayCannotOpen:
		Status(c, "triangle-alert", Orange, L("That icon couldn't be opened.", "这个图标没能打开。"),
			L("It didn't come back to the menu bar; there may not be room for it.", "它没能回到菜单栏，可能是菜单栏放不下了。"))
	}
}

func find(items []barmanager.Item, id string) (barmanager.Item, bool) {
	for _, it := range items {
		if it.ID == id {
			return it, true
		}
	}
	return barmanager.Item{}, false
}

func managerLayoutPanel(c *ui.Context, store *barmanager.ItemStore, tray *barmanager.TrayStore, snap barmanager.StoreSnapshot, state *ManagerPageState) {
	Surface(c, ui.Column(c).Padding(18).Gap(14), 18).Children(func() {
		PanelHeader(c, "layout-panel-top", L("Menu Bar Order", "菜单栏排序"), Purple, func() {
			if snap.Trusted {
				managerLayoutAccess(c, store, snap)
			}
		})
		switch {
		case !snap.Trusted:
			managerEmptyState(c, "lock", L("Arranging icons here needs Accessibility access.", "在这里整理图标需要辅助功能权限。"),
				L("macOS has no API for other apps' menu bar icons, so PowerfulTop reads them through Accessibility and moves them with the same ⌘-drag you would do.",
					"macOS 没有提供管理其他 app 菜单栏图标的接口，所以 PowerfulTop 通过辅助功能读取图标，并模拟你手动的 ⌘ 拖动来移动它们。"))
			ui.Row(c).Justify(ui.Center).Children(func() {
				if button.Solid(c, L("Grant Accessibility Access…", "授予辅助功能权限…")).Clicked() {
					store.RequestAccessibility()
				}
			})
		case len(snap.Items) == 0:
			ui.Row(c).Gap(8).AlignItems(ui.Center).Justify(ui.Center).PaddingY(24).Children(func() {
				feedback.Spin(c, L("Reading the menu bar…", "正在读取菜单栏…"), feedback.SpinOptions{Size: feedback.SpinSmall})
			})
		default:
			managerDisplays(c, store, snap)
			managerBar(c, store, tray, snap, state)
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
	if button.Outline(c, L("Allow Layout File…", "授权布局文件…")).
		Tooltip(L("Allow the menu bar layout file to move icons without the pointer, including those behind «.",
			"授权菜单栏布局文件后，移动图标不用移动鼠标，收在 « 里的图标也能移动。")).Clicked() {
		store.RequestLayoutAccess()
	}
}

func managerEmptyState(c *ui.Context, icon, title, detail string) {
	empty, _ := feedback.EmptyState(c, icon, title, detail, "")
	empty.MaxWidth(560)
}

func managerStatus(c *ui.Context, snap barmanager.StoreSnapshot) {
	switch {
	case snap.Moving:
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			feedback.Spin(c, L("Moving…", "正在移动…"), feedback.SpinOptions{Size: feedback.SpinSmall})
		})
	case snap.Error != barmanager.NoError:
		Status(c, "triangle-alert", Orange, managerErrorText(snap.Error), "")
	}
}

// managerDisplays picks the display whose menu bar the editor shows: each has its own, and only
// the one with a notch is likely to need «.
func managerDisplays(c *ui.Context, store *barmanager.ItemStore, snap barmanager.StoreSnapshot) {
	if len(snap.Displays) < 2 {
		return
	}
	selected := 0
	segments := make([]segment, len(snap.Displays))
	for i, d := range snap.Displays {
		icon := "monitor"
		if d.Internal {
			icon = "laptop"
		}
		segments[i] = segment{icon, d.Name}
		if d.Name == snap.Display {
			selected = i
		}
	}
	if segmented(c, L("Display", "显示器"), &selected, segments, false, false).Changed() {
		store.SelectDisplay(snap.Displays[selected].Name)
	}
}

func managerTip(c *ui.Context, icon, text string) {
	t := c.Theme()
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, lucide.Must(icon)).Size(11, 11).TextColor(t.TextMuted.Alpha(0.8))
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
func managerBar(c *ui.Context, store *barmanager.ItemStore, tray *barmanager.TrayStore, snap barmanager.StoreSnapshot, state *ManagerPageState) {
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
	_, over := ui.DragOver[string](frame)
	_, overFromTray := ui.DragOver[trayDrag](frame)
	if over || overFromTray {
		frame.Border(2, white(0.9))
	}
	// Dropping anywhere but on an icon sends it to the right end, where it stays in sight longest.
	if id, ok := ui.Drop[string](frame); ok {
		store.Move(id, "")
	}
	// A tray icon dropped on the bar but not on an icon goes back where it was.
	if d, ok := ui.Drop[trayDrag](frame); ok {
		tray.Remove(d.id, "")
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
							managerItemTile(c, store, tray, state, item, leftmost)
						}
					})
			}
			for _, item := range shown {
				managerItemTile(c, store, tray, state, item, leftmost)
			}
			ui.Text(c, "9:41").FontSize(12).FontWeight(600).TextColor(white(0.7)).PaddingStart(10).PaddingEnd(2)
		})
		ui.Box(c).Height(18)
	})
}

func managerItemTile(c *ui.Context, store *barmanager.ItemStore, tray *barmanager.TrayStore, state *ManagerPageState, item barmanager.Item, leftmost string) {
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
	holdable := tray.Snapshot().Enabled && tray.CanHold(item.BundleID)
	if movable || holdable {
		// The tray takes icons that can't be moved too: it switches them off rather than dragging them.
		tile.Drag(item.ID)
	}
	if movable {
		// Shows where the dragged icon will land: just left of this one.
		if id, over := ui.DragOver[string](tile); over && id != item.ID {
			tile.Border(2, ui.RGBA(255, 255, 255, 0.9))
		}
		if id, ok := ui.Drop[string](tile); ok && id != item.ID {
			store.Move(id, item.ID)
		}
	}
	if _, over := ui.DragOver[trayDrag](tile); over {
		tile.Border(2, ui.RGBA(255, 255, 255, 0.9))
	}
	if d, ok := ui.Drop[trayDrag](tile); ok {
		tray.Remove(d.id, item.LayoutKey)
	}
	if movable || holdable {
		tile.ContextMenu(func(menu *ui.Menu) {
			menu.Item(title).Disabled(true)
			if holdable && menu.Item(L("Move to Tray", "收进托盘")).Chosen() {
				tray.Add(item.BundleID)
			}
			if movable && menu.Item(L("Move to Right End (Stays in Sight)", "移到最右（优先显示）")).Chosen() {
				store.Move(item.ID, "")
			}
			if movable && leftmost != "" && leftmost != item.ID && menu.Item(L("Move to Left End (Tucked Away First)", "移到最左（优先收起）")).Chosen() {
				store.Move(item.ID, leftmost)
			}
		})
	}
	tile.Children(func() { managerLogo(c, store, state, item, ui.RGBA(255, 255, 255, 0.85)) })
}

func managerLogo(c *ui.Context, store *barmanager.ItemStore, state *ManagerPageState, item barmanager.Item, fallback ui.Color) {
	if item.Icon != "" {
		ui.Icon(c, lucide.Must(item.Icon)).Size(17, 17).TextColor(fallback)
		return
	}
	if bitmap := managerAppLogo(store, state, item); bitmap != nil {
		ui.Image(c, bitmap).Size(20, 20)
	} else {
		ui.Icon(c, lucide.Must("app-window")).Size(18, 18).TextColor(fallback)
	}
}

func managerAppLogo(store *barmanager.ItemStore, state *ManagerPageState, item barmanager.Item) *ui.Bitmap {
	return cachedLogo(&state.icons, item.AppPath, func() []byte { return store.Icon(item) })
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
			if button.Outline(c, L("Open System Settings…", "打开系统设置…")).Clicked() {
				_ = mygo.Shell.OpenExternal(managerControlCenterURL)
			}
		})
	})
}
