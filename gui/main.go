// PowerfulTop shows how much power a MacBook draws, in the menu bar and in a popover of native UI.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"powerfultop/internal/macos"
	"powerfultop/internal/power"
	"powerfultop/locales"
)

const refreshInterval = 2 * time.Second

// A press that closed the popover may still reach the status item as a click; reopening on it would flicker.
const reopenGuard = 400 * time.Millisecond

// app is read and changed on the main thread only: by tray callbacks, the view and Update.
type app struct {
	tray     *mygo.Tray
	panel    *mygo.Window
	clicks   *macos.ClickMonitor
	closedAt time.Time
	data     power.Snapshot
	err      error
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		if a.err != nil {
			ui.Text(c, locales.NoBattery()).TextColor(t.TextMuted)
			a.footer(c)
			return
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Textf(c, "%d%%", a.data.Percent).FontSize(34).Bold().FontFeatures("tnum")
			ui.Text(c, statusText(a.data)).TextColor(t.TextMuted)
		})
		ui.Divider(c)
		reading(c, locales.FlowSystem(), watts(a.data.SystemW), "")
		if a.data.OnAC {
			rating := ""
			if a.data.AdapterW > 0 {
				rating = locales.AdapterRating(a.data.AdapterW)
			}
			reading(c, locales.FlowCharger(), watts(a.data.ACInputW), rating)
		}
		reading(c, locales.FlowBattery(), batteryText(a.data), "")
		ui.Divider(c)
		a.footer(c)
	})
}

func (a *app) footer(c *ui.Context) {
	ui.Row(c).Children(func() {
		ui.Spacer(c)
		ui.Button(c, locales.Quit()).OnClick(mygo.App.Quit)
	})
}

// reading is one "label    value" line; detail goes under the value in a smaller, muted face.
func reading(c *ui.Context, label, value, detail string) {
	t := c.Theme()
	ui.Row(c).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).TextColor(t.TextMuted).Grow(1)
		ui.Column(c).AlignItems(ui.End).Children(func() {
			ui.Text(c, value).Bold().FontFeatures("tnum")
			if detail != "" {
				ui.Text(c, detail).FontSize(11).TextColor(t.TextMuted)
			}
		})
	})
}

func statusText(s power.Snapshot) string {
	switch {
	case !s.OnAC:
		return locales.StatusBatteryDischarging()
	case s.FullyCharged:
		return locales.StatusFullyCharged()
	case s.BatteryW < -power.BatteryFlowThresholdW:
		return locales.StatusACCharging()
	default:
		return locales.StatusACPowered()
	}
}

func batteryText(s power.Snapshot) string {
	switch {
	case s.BatteryW < -power.BatteryFlowThresholdW:
		return locales.BatteryCharging(watts(-s.BatteryW))
	case s.BatteryW > power.BatteryFlowThresholdW:
		return locales.BatteryDischarging(watts(s.BatteryW))
	default:
		return locales.BatteryIdle()
	}
}

func watts(w float64) string { return fmt.Sprintf("%.1f W", w) }

// trayTitle matches the Swift app's default label: whole watts of system power.
func trayTitle(s power.Snapshot, err error) string {
	if err != nil {
		return "—"
	}
	return fmt.Sprintf("%.0fW", s.SystemW)
}

// toggle opens the popover under the menu bar item, or closes it.
func (a *app) toggle() {
	if a.panel != nil {
		a.panel.Close()
		return
	}
	if time.Since(a.closedAt) < reopenGuard {
		return
	}
	a.panel = mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:      a.tray,
		Placement: mygo.PlacementBottom,
		Width:     300,
		Height:    250,
		Focusable: true,
		Popover:   true,
		Content:   ui.View(a.view),
	})
	a.clicks = macos.StartClickMonitor(a.dismissIfOutside)
	a.panel.OnClosed(func() {
		a.panel = nil
		// Closing often runs inside the monitor's own block, which must outlive this call.
		monitor := a.clicks
		a.clicks = nil
		go mygo.RunOnMain(monitor.Stop)
	})
}

func (a *app) dismissIfOutside() {
	if a.panel == nil || contains(a.panel.Bounds(), mygo.Screen.CursorScreenPoint()) {
		return
	}
	a.closedAt = time.Now()
	a.panel.Close()
}

func contains(r mygo.Rectangle, p mygo.Point) bool {
	return p.X >= r.X && p.X < r.X+r.Width && p.Y >= r.Y && p.Y < r.Y+r.Height
}

// refresh reads off the main thread, then hands the snapshot to the main thread.
func (a *app) refresh() {
	data, err := power.Read()
	mygo.RunOnMain(func() {
		a.data, a.err = data, err
		a.tray.SetTitle(trayTitle(data, err))
		if a.panel != nil {
			a.panel.Invalidate()
		}
	})
}

func (a *app) ready() {
	var err error
	a.tray, err = mygo.NewTray(mygo.TrayOptions{
		// Keeps the item where the user ⌘-drags it across launches.
		ID:      "PowerfulTop.status",
		Title:   "…",
		ToolTip: "PowerfulTop",
	})
	if err != nil {
		log.Fatal("tray: ", err)
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{{Role: mygo.RoleQuit}})
	a.tray.OnClick(a.toggle)
	a.tray.OnRightClick(func() {
		if a.panel != nil {
			a.panel.Close()
		}
		// Only for this popup, so primary clicks keep toggling the popover.
		a.tray.SetMenu(menu)
		defer a.tray.SetMenu(nil)
		a.tray.PopUpMenu()
	})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.tray.Destroy() })

	go func() {
		for {
			a.refresh()
			time.Sleep(refreshInterval)
		}
	}()
}

func main() {
	a := &app{}
	mygo.App.SetName("PowerfulTop")
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	// A menu bar app lives on after its popover, its only window, closes.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(a.ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
