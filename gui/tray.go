package main

import (
	"bytes"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"powerfultop/internal/macos"
	"powerfultop/internal/view"
)

// trayID is the status item's autosave name.
const trayID = "PowerfulTop.status"

// A press that closed the popover may still reach the status item as a click; reopening on it would flicker.
const reopenGuard = 400 * time.Millisecond

func (a *app) setupTray() {
	var err error
	a.tray, err = mygo.NewTray(mygo.TrayOptions{
		// Keeps the item where the user ⌘-drags it across launches.
		ID:      trayID,
		Title:   "…",
		ToolTip: "PowerfulTop",
	})
	if err != nil {
		log.Fatal("tray: ", err)
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: view.L("Details", "详细参数"), Click: func(*mygo.MenuItem, *mygo.Window) { a.openDetails("") }},
		mygo.Separator(),
		{Role: mygo.RoleQuit},
	})
	a.tray.OnClick(a.togglePopover)
	a.tray.OnRightClick(func() {
		if p := a.panel.Load(); p != nil {
			p.Close()
		}
		// Only for this popup, so primary clicks keep toggling the popover.
		a.tray.SetMenu(menu)
		defer a.tray.SetMenu(nil)
		a.tray.PopUpMenu()
	})
	// Coloured label styles are drawn for the bar's appearance.
	a.tray.OnAppearanceChange(a.updateTray)
	a.updateTray()
	a.scheduleTray()
}

// scheduleTray redraws the label at the user's chosen interval; values come from several monitors,
// so polling is simpler than observing every field and also caps how often the menu bar reflows.
func (a *app) scheduleTray() {
	interval := time.Duration(a.menuSettings.Configuration().UpdateInterval * float64(time.Second))
	if interval < time.Second {
		interval = 2 * time.Second
	}
	if a.trayTicker != nil {
		a.trayTicker.Reset(interval)
		return
	}
	a.trayTicker = time.NewTicker(interval)
	go func() {
		for range a.trayTicker.C {
			mygo.RunOnMain(a.updateTray)
		}
	}()
}

// updateTray renders the whole label to an image, since trays can't show attributed titles.
func (a *app) updateTray() {
	ctx := menuBarContext(a.power.Snapshot(), a.power.Available(), a.thermal.Snapshot(), a.system.Snapshot())
	png, template, err := renderLabel(a, ctx)

	if err != nil || len(png) == 0 {
		a.tray.SetTitle("PowerfulTop")
		return
	}
	if bytes.Equal(png, a.trayIcon) {
		return
	}
	a.trayIcon = png
	if err := a.tray.SetIcon(png, template); err != nil {
		a.tray.SetTitle("PowerfulTop")
		return
	}
	a.tray.SetTitle("")
}

func (a *app) togglePopover() {
	if p := a.panel.Load(); p != nil {
		p.Close()
		return
	}
	if time.Since(a.closedAt) < reopenGuard {
		return
	}
	a.panelHeight = 0
	a.chargeLimit.Refresh()
	panel := mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:      a.tray,
		Placement: mygo.PlacementBottom,
		Width:     view.PopoverWidth,
		Height:    560,
		Focusable: true,
		Popover:   true,
		Content:   ui.View(a.popoverView),
	})
	a.panel.Store(panel)
	a.setDisplaysShown(true)
	a.clicks = macos.StartClickMonitor(a.dismissIfOutside)
	panel.OnClosed(func() {
		a.panel.Store(nil)
		a.setDisplaysShown(false)
		// Closing often runs inside the monitor's own block, which must outlive this call.
		monitor := a.clicks
		a.clicks = nil
		go mygo.RunOnMain(monitor.Stop)
	})
}

// setDisplaysShown polls native brightness only while its sliders are on screen.
func (a *app) setDisplaysShown(shown bool) {
	if shown != a.displaysShown {
		a.displaysShown = shown
		a.displays.SetActive(shown)
	}
}

func (a *app) dismissIfOutside() {
	p := a.panel.Load()
	if p == nil || contains(p.Bounds(), mygo.Screen.CursorScreenPoint()) {
		return
	}
	a.closedAt = time.Now()
	p.Close()
}

func contains(r mygo.Rectangle, p mygo.Point) bool {
	return p.X >= r.X && p.X < r.X+r.Width && p.Y >= r.Y && p.Y < r.Y+r.Height
}

func (a *app) popoverView(c *ui.Context) {
	d := view.PopoverData{
		Power:              a.power.Snapshot(),
		Available:          a.power.Available(),
		Thermal:            a.thermal.Snapshot(),
		Fans:               a.thermal,
		System:             a.system.Snapshot(),
		Displays:           a.displays.Snapshot(),
		LaunchAtLogin:      mygo.App.OpenAtLogin(),
		LaunchAtLoginError: a.loginError,
		Version:            mygo.App.Version(),
	}
	root := view.Popover(c, d, view.PopoverActions{
		OpenDetails: func(page view.PageID) {
			if p := a.panel.Load(); p != nil {
				p.Close()
			}
			a.openDetails(page)
		},
		SetBrightness:    a.displays.SetLevel,
		SetLaunchAtLogin: a.setLaunchAtLogin,
		Quit:             mygo.App.Quit,
	})
	// Flyouts have a fixed size; follow the content's height as NSPopover's fitting size does.
	if h := int(root.Bounds().H + 0.5); h > 0 && h != a.panelHeight {
		a.panelHeight = h
		if p := a.panel.Load(); p != nil {
			go mygo.RunOnMain(func() { p.SetContentSize(view.PopoverWidth, h) })
		}
	}
}
