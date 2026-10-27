package view

import (
	"github.com/egoist/mygo/ui"

	"powerfultop/internal/barmanager"
	"powerfultop/internal/icons"
)

const (
	trayColumns = 5
	trayTile    = 40
	trayGap     = 4
	trayPadding = 10
	// TrayFlyoutWidth fits five icons a row, as the overflow of Windows' taskbar does.
	TrayFlyoutWidth = trayColumns*trayTile + (trayColumns-1)*trayGap + 2*trayPadding
)

// TrayFlyoutState is what the flyout keeps between frames.
type TrayFlyoutState struct {
	icons map[string]*ui.Bitmap
}

// TrayFlyoutActions are what the flyout's icons do.
type TrayFlyoutActions struct {
	// Open is given the icon's bounds in the flyout, for its menu to hang under it.
	Open func(id string, tile ui.Rect)
	// Remove puts the item back in the menu bar for good.
	Remove      func(id string)
	OpenManager func()
}

// TrayFlyout is the grid of the tray's icons that hangs under its chevron.
func TrayFlyout(c *ui.Context, tray *barmanager.TrayStore, state *TrayFlyoutState, a TrayFlyoutActions) ui.Element {
	t := c.Theme()
	snap := tray.Snapshot()
	return ui.Column(c).Width(TrayFlyoutWidth).Padding(trayPadding).Gap(8).Children(func() {
		if len(snap.Entries) == 0 {
			ui.Column(c).Gap(6).AlignItems(ui.Center).PaddingY(10).Children(func() {
				ui.Icon(c, icons.Get("archive")).Size(20, 20).TextColor(t.TextMuted)
				ui.Text(c, L("No icons in the tray", "托盘里还没有图标")).FontSize(12).FontWeight(600)
				ui.Text(c, L("Add them in Menu Bar Manager.", "在「菜单栏管理」中添加。")).FontSize(11).TextColor(t.TextMuted).TextAlign(ui.Center)
			})
		} else {
			ui.Row(c).Wrap().Gap(trayGap).Children(func() {
				for _, e := range snap.Entries {
					trayFlyoutTile(c, tray, state, e, a)
				}
			})
		}
		switch snap.Error {
		case barmanager.ErrTrayCannotOpen:
			Status(c, "triangle-alert", Orange, L("That icon couldn't be opened.", "这个图标没能打开。"),
				L("It didn't come back to the menu bar; there may not be room for it.", "它没能回到菜单栏，可能是菜单栏放不下了。"))
		case barmanager.ErrTrayNoAccess:
			Status(c, "lock", Orange, L("Allow the switch file in Menu Bar Manager.", "请在「菜单栏管理」中授权开关文件。"), "")
		}
		ui.Row(c).Justify(ui.End).Children(func() {
			manage := ui.ButtonBase(c).Padding(2, 6).Radius(5).Children(func() {
				ui.Text(c, L("Menu Bar Manager…", "菜单栏管理…")).FontSize(11).TextColor(t.TextMuted)
			})
			if manage.Hovered() {
				manage.Background(t.Text.Alpha(0.06))
			}
			if manage.Clicked() && a.OpenManager != nil {
				a.OpenManager()
			}
		})
	})
}

func trayFlyoutTile(c *ui.Context, tray *barmanager.TrayStore, state *TrayFlyoutState, e barmanager.TrayEntry, a TrayFlyoutActions) {
	t := c.Theme()
	name := trayEntryName(e)
	help := name
	if !e.Running {
		help += "\n" + L("Not running; click to open it.", "未在运行，点击打开。")
	}
	tile := ui.Box(c).Key(e.ID).Size(trayTile, trayTile).Center().Radius(8).Tooltip(help).Label(name)
	if tile.Hovered() {
		tile.Background(t.Text.Alpha(0.08))
	}
	if tile.Pressed() {
		tile.Background(t.Text.Alpha(0.14))
	}
	if !e.Running {
		tile.Opacity(0.5)
	}
	if tile.Clicked() && a.Open != nil {
		a.Open(e.ID, tile.Bounds())
	}
	tile.ContextMenu(func(m *ui.Menu) {
		m.Item(name).Disabled(true)
		if m.Item(L("Put Back in Menu Bar", "放回菜单栏")).Chosen() && a.Remove != nil {
			a.Remove(e.ID)
		}
	})
	tile.Children(func() { trayEntryLogo(c, tray, &state.icons, e, 22) })
}

func trayEntryName(e barmanager.TrayEntry) string {
	if e.NameZh != "" {
		return L(e.Name, e.NameZh)
	}
	return e.Name
}

// trayEntryLogo draws the app's logo, or the lucide icon standing in for one of Apple's items.
func trayEntryLogo(c *ui.Context, tray *barmanager.TrayStore, cache *map[string]*ui.Bitmap, e barmanager.TrayEntry, size float32) {
	if e.Icon != "" {
		ui.Icon(c, icons.Get(e.Icon)).Size(size-4, size-4).TextColor(c.Theme().Text)
		return
	}
	if bitmap := cachedLogo(cache, e.AppPath, func() []byte { return tray.Icon(e) }); bitmap != nil {
		ui.Image(c, bitmap).Size(size, size)
		return
	}
	ui.Icon(c, icons.Get("app-window")).Size(size-4, size-4).TextColor(c.Theme().TextMuted)
}

// cachedLogo decodes an app's logo once; a logo that hasn't arrived yet is asked for again.
func cachedLogo(cache *map[string]*ui.Bitmap, path string, fetch func() []byte) *ui.Bitmap {
	if path == "" {
		return nil
	}
	if bitmap, ok := (*cache)[path]; ok {
		return bitmap
	}
	png := fetch()
	if png == nil {
		return nil
	}
	bitmap, err := ui.DecodeBitmap(png)
	if err != nil {
		bitmap = nil
	}
	if *cache == nil {
		*cache = map[string]*ui.Bitmap{}
	}
	(*cache)[path] = bitmap
	return bitmap
}
