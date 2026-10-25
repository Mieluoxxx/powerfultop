// PowerfulTop shows how much power a MacBook draws, in the menu bar and in windows of native UI.
package main

import (
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"powerfultop/internal/barmanager"
	"powerfultop/internal/display"
	"powerfultop/internal/history"
	"powerfultop/internal/macos"
	"powerfultop/internal/menubar"
	"powerfultop/internal/power"
	"powerfultop/internal/system"
	"powerfultop/internal/thermal"
	"powerfultop/internal/view"
	// Registers the app's languages with MyGo, so view.L and MyGo's own controls follow the system.
	_ "powerfultop/locales"
)

// app's fields are read and changed on the main thread, except the atomics, which monitor
// callbacks read from their goroutines to ask the windows for a frame.
type app struct {
	prefs        macos.Preferences
	power        *power.Monitor
	chargeLimit  *power.ChargeLimiter
	thermal      *thermal.Monitor
	system       *system.Monitor
	displays     *display.Controller
	menuSettings *menubar.Settings
	bar          *barmanager.Manager
	store        *barmanager.ItemStore
	history      history.History

	tray     *mygo.Tray
	panel    atomic.Pointer[mygo.Window]
	details  atomic.Pointer[mygo.Window]
	clicks   *macos.ClickMonitor
	closedAt time.Time

	trayIcon      []byte
	trayTicker    *time.Ticker
	panelHeight   int
	loginError    string
	quitting      bool
	detailsOpen   bool
	detailsState  view.DetailsState
	coolingState  view.CoolingState
	menuBarState  view.MenuBarPageState
	managerState  view.ManagerPageState
	sampling      bool
	storeVisible  bool
	batteryShown  bool
	displaysShown bool
}

func newApp() *app {
	a := &app{prefs: macos.AppPreferences()}
	migrateLegacyDefaults(a.prefs, macos.DomainPreferences(legacyBundleID))
	clearStaleStatusItemVisibility(a.prefs)
	a.power = power.NewMonitor()
	a.chargeLimit = power.NewChargeLimiter()
	resources, _ := mygo.App.Path(mygo.PathResources)
	a.thermal = thermal.NewMonitor(filepath.Join(resources, thermal.BundledHelperName))
	a.system = system.NewMonitor()
	a.displays = display.NewController()
	a.menuSettings = menubar.NewSettings(a.prefs)
	a.menuSettings.Load()
	a.bar = barmanager.NewManager()
	a.store = barmanager.NewItemStore(a.bar)
	a.detailsState = view.DetailsState{Selected: view.PageOverview, Available: true, OnSelect: a.persistPage}
	if page, ok := a.prefs.String(dashboardPageKey); ok {
		a.detailsState.Selected = view.PageID(page)
	}
	return a
}

func (a *app) ready() {
	a.power.Start()
	a.chargeLimit.Refresh()
	a.thermal.Start()
	a.system.Start()
	a.displays.Start()
	a.setupTray()

	for _, onChange := range []func(func()){a.power.OnChange, a.chargeLimit.OnChange, a.thermal.OnChange, a.system.OnChange, a.displays.OnChange, a.bar.OnChange, a.store.OnChange} {
		onChange(a.invalidate)
	}
	a.menuSettings.OnChange(func() {
		mygo.RunOnMain(func() {
			a.updateTray()
			a.scheduleTray()
		})
	})
	a.bar.OpenSettings = func() { a.openDetails(view.PageMenuBarManager) }
	a.bar.Start()

	mygo.Power.OnSuspend(a.thermal.HandleSleep)
	// App.Quit must still close the details window, which otherwise only hides.
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.shutdown() })

	go a.recordHistory()
	if smokeLaunch() {
		a.checkSmokeLaunch()
	}
}

// invalidate redraws whatever shows; monitors call it from their goroutines.
func (a *app) invalidate() {
	if w := a.panel.Load(); w != nil {
		w.Invalidate()
	}
	if w := a.details.Load(); w != nil {
		w.Invalidate()
	}
}

func (a *app) recordHistory() {
	for {
		if a.power.Available() {
			a.history.Append(historySample(time.Now(), a.power.Snapshot(), a.thermal.Snapshot(), a.system.Snapshot()))
		}
		time.Sleep(history.SampleInterval)
	}
}

// shutdown runs synchronously at quit: a deferred hand-back could leave the fans in manual mode.
func (a *app) shutdown() {
	a.thermal.Shutdown()
	a.displays.Shutdown()
	a.bar.Shutdown()
	a.system.Stop()
	a.power.Stop()
	if a.trayTicker != nil {
		a.trayTicker.Stop()
	}
	a.tray.Destroy()
}

func (a *app) setLaunchAtLogin(on bool) {
	a.loginError = ""
	if err := mygo.App.SetOpenAtLogin(on); err != nil {
		a.loginError = err.Error()
	}
	a.invalidate()
}

func smokeLaunch() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--smoke-launch" {
			return true
		}
	}
	return false
}

// checkSmokeLaunch fails the automated launch check if a window appeared on its own.
func (a *app) checkSmokeLaunch() {
	time.AfterFunc(700*time.Millisecond, func() {
		mygo.RunOnMain(func() {
			if a.panel.Load() != nil || a.details.Load() != nil {
				log.Print("smoke-launch FAIL: a window opened at launch")
				os.Exit(1)
			}
			log.Print("smoke-launch OK: no content window at launch")
			os.Exit(0)
		})
	})
}

func main() {
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	a := newApp()
	mygo.App.SetName("PowerfulTop")
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	// A menu bar app lives on after its windows close.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(a.ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
