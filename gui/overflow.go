package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"powerfultop/internal/barmanager"
	"powerfultop/internal/view"
)

// overflowID is the tray chevron's autosave name.
const overflowID = "PowerfulTop.overflow"

func (a *app) setupOverflow() {
	// Not waiting for the main thread: the store may call this while the main thread waits for it.
	a.trayStore.OnChange(func() { go mygo.RunOnMain(a.syncOverflow) })
	// The editor shows what went into or came out of the tray now, not at its next poll; it scans
	// only while its page shows.
	a.trayStore.OnChange(a.store.Refresh)
	a.trayStore.Start()
	a.syncOverflow()
}

// syncOverflow shows the chevron while the tray is on, and redraws what shows the tray.
func (a *app) syncOverflow() {
	snap := a.trayStore.Snapshot()
	want := snap.Enabled
	switch {
	case want && a.overflow == nil:
		a.trayStore.PlaceOverflow(trayID)
		tray, err := mygo.NewTray(mygo.TrayOptions{
			ID:             overflowID,
			Icon:           barmanager.OverflowIcon(false),
			IconIsTemplate: true,
			ToolTip:        view.L("Tray", "托盘"),
		})
		if err != nil {
			log.Print("overflow tray: ", err)
			return
		}
		a.overflow = tray
		tray.OnClick(a.toggleOverflow)
		tray.OnRightClick(a.overflowMenu)
	case !want && a.overflow != nil:
		if p := a.overflowPanel.Load(); p != nil {
			p.Close()
		}
		a.overflow.Destroy()
		a.overflow = nil
	}
	if p := a.overflowPanel.Load(); p != nil {
		p.Invalidate()
	}
}

func (a *app) overflowMenu() {
	if p := a.overflowPanel.Load(); p != nil {
		p.Close()
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: view.L("Menu Bar Manager…", "菜单栏管理…"), Click: func(*mygo.MenuItem, *mygo.Window) { a.openDetails(view.PageMenuBarManager) }},
		mygo.Separator(),
		{Label: view.L("Put All Back in Menu Bar", "全部放回菜单栏"), Click: func(*mygo.MenuItem, *mygo.Window) { a.trayStore.SetEnabled(false) }},
	})
	a.overflow.SetMenu(menu)
	defer a.overflow.SetMenu(nil)
	a.overflow.PopUpMenu()
}

func (a *app) toggleOverflow() {
	if p := a.overflowPanel.Load(); p != nil {
		p.Close()
		return
	}
	a.trayStore.Reconcile()
	a.overflowHeight = 0
	panel := mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:      a.overflow,
		Placement: mygo.PlacementBottom,
		Width:     view.TrayFlyoutWidth,
		Height:    120,
		Focusable: true,
		Popover:   true,
		Content:   ui.View(a.overflowView),
	})
	a.overflowPanel.Store(panel)
	_ = a.overflow.SetIcon(barmanager.OverflowIcon(true), true)
	panel.OnClosed(func() {
		a.overflowPanel.Store(nil)
		if a.overflow != nil {
			_ = a.overflow.SetIcon(barmanager.OverflowIcon(false), true)
		}
	})
}

func (a *app) closeOverflow() {
	if p := a.overflowPanel.Load(); p != nil {
		p.Close()
	}
}

func (a *app) overflowView(c *ui.Context) {
	root := view.TrayFlyout(c, a.trayStore, &a.overflowState, view.TrayFlyoutActions{
		Open:   a.openTrayItem,
		Remove: func(id string) { a.trayStore.Remove(id, "") },
		OpenManager: func() {
			a.closeOverflow()
			a.openDetails(view.PageMenuBarManager)
		},
	})
	// Flyouts have a fixed size; follow the content's height as NSPopover's fitting size does.
	if h := int(root.Bounds().H + 0.5); h > 0 && h != a.overflowHeight {
		a.overflowHeight = h
		if p := a.overflowPanel.Load(); p != nil {
			go mygo.RunOnMain(func() { p.SetContentSize(view.TrayFlyoutWidth, h) })
		}
	}
}

// openTrayItem reads the item's menu off the main thread, since Accessibility waits for the app,
// then hangs a copy of it under the icon.
func (a *app) openTrayItem(id string, tile ui.Rect) {
	go func() {
		entries, _ := a.trayStore.Open(id, func() { mygo.RunOnMain(a.closeOverflow) })
		if len(entries) == 0 {
			return
		}
		mygo.RunOnMain(func() {
			p := a.overflowPanel.Load()
			if p == nil {
				return
			}
			mygo.NewMenu(a.trayMenuItems(entries)).PopupAt(p, int(tile.X), int(tile.Y+tile.H+4))
		})
	}()
}

func (a *app) trayMenuItems(entries []barmanager.MenuEntry) []*mygo.MenuItem {
	items := make([]*mygo.MenuItem, 0, len(entries))
	for _, e := range entries {
		if e.IsSeparator() {
			items = append(items, mygo.Separator())
			continue
		}
		item := &mygo.MenuItem{Label: e.Title, Disabled: !e.Enabled}
		if e.Submenu != nil {
			item.Submenu = a.trayMenuItems(e.Submenu)
		} else {
			if e.Mark != "" {
				item.Type, item.Checked = mygo.MenuItemCheckbox, true
			}
			item.Click = func(*mygo.MenuItem, *mygo.Window) {
				a.closeOverflow()
				a.trayStore.Press(e)
			}
		}
		items = append(items, item)
	}
	return items
}
