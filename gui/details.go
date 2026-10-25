package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"powerfultop/internal/menubar"
	"powerfultop/internal/power"
	"powerfultop/internal/thermal"
	"powerfultop/internal/view"
)

// dashboardPageKey is the Swift app's key for the selected page, so the choice carries over.
const dashboardPageKey = "dashboardPage"

func renderLabel(a *app, ctx menubar.Context) ([]byte, bool, error) {
	return menubar.RenderLabel(a.menuSettings.Configuration(), ctx, a.tray.Dark())
}

func (a *app) persistPage(id view.PageID) {
	_ = a.prefs.Set(dashboardPageKey, string(id))
}

// openDetails shows the details window, on page when one is given.
func (a *app) openDetails(page view.PageID) {
	if page != "" {
		a.detailsState.Selected = page
		a.persistPage(page)
	}
	w := a.details.Load()
	if w == nil {
		w = mygo.NewWindow(mygo.WindowOptions{
			Title:     view.L("PowerfulTop Details", "PowerfulTop 详细参数"),
			Width:     view.DetailsWidth,
			Height:    view.DetailsHeight,
			MinWidth:  view.DetailsMinWidth,
			MinHeight: view.DetailsMinHeight,
			StateKey:  "details",
			Hidden:    true,
			Content:   ui.View(a.detailsView),
		})
		// Closing only hides it, which keeps each page's scroll and editing state.
		w.OnClose(func(e *mygo.CloseEvent) {
			if !a.quitting {
				e.PreventDefault()
				w.Hide()
			}
		})
		w.OnShow(func() { a.setDetailsOpen(true) })
		w.OnHide(func() { a.setDetailsOpen(false) })
		a.details.Store(w)
	}
	w.Show()
	w.Focus()
	a.setDetailsOpen(true)
	w.Invalidate()
}

func (a *app) setDetailsOpen(open bool) {
	a.detailsOpen = open
	a.applyPageEffects()
}

// applyPageEffects starts what only the page on screen needs: process sampling for the system
// page, and the manager's editing hold, which keeps the menu bar expanded for the layout editor.
func (a *app) applyPageEffects() {
	selected := a.detailsState.Selected
	if sampling := a.detailsOpen && selected == view.PageSystem; sampling != a.sampling {
		a.sampling = sampling
		a.system.SetProcessSamplingActive(sampling)
	}
	// The limit can change in System Settings or AlDente, so it is reread whenever the page comes up.
	if shown := a.detailsOpen && selected == view.PageBattery; shown != a.batteryShown {
		a.batteryShown = shown
		if shown {
			a.chargeLimit.Refresh()
		}
	}
	if visible := a.detailsOpen && selected == view.PageMenuBarManager; visible != a.storeVisible {
		a.storeVisible = visible
		a.store.SetVisible(visible)
	}
}

func (a *app) detailsView(c *ui.Context) {
	a.applyPageEffects()
	p := a.power.Snapshot()
	th := a.thermal.Snapshot()
	sys := a.system.Snapshot()
	a.detailsState.Available = a.power.Available()
	a.detailsState.UpdatedAt = p.Timestamp

	helperHint := view.L("The fan helper needs to be authorized again.", "风扇驱动需要重新授权。")
	pages := []view.WindowPage{
		{ID: view.PageOverview, Title: view.L("Overview", "概览"), Icon: "layout-grid", Body: func(c *ui.Context, width float32) {
			view.OverviewPage(c, view.OverviewData{Power: p, Thermal: th, System: sys, History: &a.history}, width)
		}},
		{ID: view.PageSystem, Title: view.L("System", "系统"), Icon: "cpu", Body: func(c *ui.Context, width float32) {
			cpuTemp := func() (float64, bool) {
				s, ok := th.Temperatures[thermal.GroupCPU]
				return s.AverageC, ok
			}
			view.SystemPage(c, sys, cpuTemp, &a.history, width)
		}},
		{ID: view.PageCooling, Title: view.L("Cooling", "散热"), Icon: "fan",
			// Cooling hosts the fan controls and the helper card, so a stale helper is flagged there.
			Attention: th.HelperStatus == thermal.HelperNeedsUpdate, Hint: helperHint,
			Body: func(c *ui.Context, width float32) {
				view.CoolingPage(c, a.thermal, &a.history, &a.coolingState, width)
			}},
		{ID: view.PageBattery, Title: view.L("Battery", "电池"), Icon: "battery-full", Body: func(c *ui.Context, width float32) {
			view.BatteryPage(c, p, a.chargeLimit, width)
		}},
		{ID: view.PageMenuBar, Title: view.L("Menu Bar", "菜单栏"), Icon: "panel-top", Body: func(c *ui.Context, width float32) {
			view.MenuBarPage(c, a.menuSettings, menuBarContext(p, a.power.Available(), th, sys), &a.menuBarState, width)
		}},
		{ID: view.PageMenuBarManager, Title: view.L("Menu Bar Manager", "菜单栏管理"), Icon: "app-window", Body: func(c *ui.Context, width float32) {
			view.MenuBarManagerPage(c, a.bar, a.store, &a.managerState, width)
		}},
		{ID: view.PageSettings, Title: view.L("Settings", "设置"), Icon: "settings", Body: func(c *ui.Context, _ float32) {
			view.SettingsPage(c, a.settingsData(p), a.settingsActions())
		}},
	}
	view.Details(c, pages, &a.detailsState)
}

func (a *app) settingsData(p power.Data) view.SettingsData {
	d := view.SettingsData{
		LaunchAtLogin:      mygo.App.OpenAtLogin(),
		LaunchAtLoginError: a.loginError,
		Version:            mygo.App.Version(),
	}
	if p.DeviceName != nil {
		d.ChipName = *p.DeviceName
	}
	if p.BatteryManufactureDate != nil {
		d.ManufactureDate = *p.BatteryManufactureDate
	}
	if p.BatterySerial != nil {
		d.Serial = *p.BatterySerial
	}
	return d
}

func (a *app) settingsActions() view.SettingsActions {
	return view.SettingsActions{
		SetLaunchAtLogin: a.setLaunchAtLogin,
		CopySerial:       mygo.Clipboard.WriteText,
	}
}
