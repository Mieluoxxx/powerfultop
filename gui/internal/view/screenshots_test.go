package view

import (
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"powerfultop/internal/barmanager"
	"powerfultop/internal/display"
	"powerfultop/internal/history"
	"powerfultop/internal/macos"
	"powerfultop/internal/menubar"
	"powerfultop/internal/power"
	"powerfultop/internal/system"
	"powerfultop/internal/thermal"
)

// TestReadmeScreenshots renders the README's screenshots from the app's own views and one Mac's
// readings, so they can be regenerated without a window server or a plugged-in charger:
//
//	README_SCREENSHOTS=../../screenshot go test ./internal/view -run TestReadmeScreenshots
//
// App logos for the Menu Bar Manager come from a directory of PNGs named after the app, since
// reading them out of an .app bundle needs the app's run loop: README_ICONS=<dir>.
func TestReadmeScreenshots(t *testing.T) {
	dir := os.Getenv("README_SCREENSHOTS")
	if dir == "" {
		t.Skip("README_SCREENSHOTS names the directory to write the screenshots to")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := renderDemo(os.Getenv("README_LOCALE"), dir, os.Getenv("README_ICONS")); err != nil {
		t.Fatal(err)
	}
}

// demo is the one Mac the screenshots show: its battery, charger, sensors and window.
type demo struct {
	power    power.Data
	thermal  thermal.Snapshot
	fans     *thermal.Monitor
	system   system.Snapshot
	history  *history.History
	limit    *power.ChargeLimiter
	menu     *menubar.Settings
	menuCtx  menubar.Context
	store    *barmanager.ItemStore
	tray     *barmanager.TrayStore
	cooling  CoolingState
	barState MenuBarPageState
	mgrState ManagerPageState

	now time.Time
}

// demoTime is when the screenshots were taken.
func demoTime() time.Time { return time.Date(2026, 10, 28, 14, 30, 0, 0, time.Local) }

// renderDemo writes every screenshot; locale is "" for the system's language.
func renderDemo(locale, dir, iconDir string) error {
	if locale != "" {
		// view.L reads the language through i18n, as the app's registered catalogs set it.
		i18n.SetLocaleFunc(func() string { return locale })
	}
	now := demoTime()
	prefs := macos.DomainPreferences("io.github.mieluoxxx.PowerfulTop.readme")
	defer func() { _ = prefs.Set("menuBarConfiguration", nil) }()
	menu := menubar.NewSettings(prefs)
	menu.Set(menubar.Configuration{
		IconStyle:      menubar.IconMacOSColor,
		Items:          []menubar.Item{menubar.ItemSystemPower, menubar.ItemBatteryPercent, menubar.ItemCPUTemperature, menubar.ItemFanSpeed},
		Spacing:        6,
		UpdateInterval: 2,
	})

	store, tray := demoBarManager(iconDir)
	d := &demo{
		power:   demoPower(stateACPowered, now),
		thermal: demoThermal(),
		system:  demoSystem(now),
		history: demoHistory(now),
		limit:   demoLimit(),
		menu:    menu,
		store:   store,
		tray:    tray,
		now:     now,
	}
	d.fans = thermal.MonitorWithSnapshot(d.thermal)
	d.menuCtx = demoMenuBarContext(d.power, d.thermal, d.system)

	popovers := []struct {
		name string
		kind int
	}{
		{"popover-ac-powered", stateACPowered},
		{"popover-ac-charging", stateCharging},
		{"popover-battery-discharging", stateDischarging},
		{"popover-ac-supplement", stateSupplement},
	}
	for _, p := range popovers {
		before := d.power
		d.power = demoPower(p.kind, now)
		d.menuCtx = demoMenuBarContext(d.power, d.thermal, d.system)
		data := PopoverData{
			Power: d.power, Available: true, Thermal: d.thermal, Fans: d.fans, System: d.system,
			Displays:      demoDisplays(),
			ChargeLimit:   d.limit,
			LaunchAtLogin: true,
		}
		if err := writeShot(filepath.Join(dir, p.name+".png"), PopoverWidth, 900, func(c *ui.Context) {
			Popover(c, data, PopoverActions{})
		}); err != nil {
			return err
		}
		d.power = before
	}
	d.menuCtx = demoMenuBarContext(d.power, d.thermal, d.system)

	// The details window on each page, with the title bar a window would draw.
	for _, page := range d.pages() {
		selected := page.ID
		pages := d.pages()
		for i := range pages {
			if pages[i].ID == selected {
				// Freeze the page the shot is of, as clicking its sidebar row would.
				pages[i].Body = page.Body
			}
		}
		state := &DetailsState{Selected: selected, Available: true, UpdatedAt: now}
		name := "detail-" + string(selected) + ".png"
		if err := writeShot(filepath.Join(dir, name), DetailsWidth, 820, func(c *ui.Context) {
			detailsWindow(c, pages, state)
		}); err != nil {
			return err
		}
	}
	return nil
}

// detailsWindow draws the details window's content under the title bar AppKit would add.
func detailsWindow(c *ui.Context, pages []WindowPage, state *DetailsState) {
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Height(36).PaddingX(14).AlignItems(ui.Center).Background(t.Text.Alpha(0.03)).Children(func() {
			ui.Row(c).Width(56).Gap(8).Children(func() {
				for _, color := range []ui.Color{ui.Hex("#ff5f57"), ui.Hex("#febc2e"), ui.Hex("#28c840")} {
					ui.Box(c).Width(12).Height(12).Radius(6).Background(color)
				}
			})
			ui.Text(c, L("PowerfulTop Details", "PowerfulTop 详细参数")).FontSize(13).FontWeight(600).
				TextAlign(ui.Center).Grow(1).TextColor(t.Text.Alpha(0.8))
			ui.Box(c).Width(56)
		})
		ui.Row(c).Grow(1).Children(func() { Details(c, pages, state) })
	})
}

// pages is the details window's sidebar, as the app builds it, filled with this Mac's readings.
func (d *demo) pages() []WindowPage {
	cpuTemp := func() (float64, bool) {
		s, ok := d.thermal.Temperatures[thermal.GroupCPU]
		return s.AverageC, ok
	}
	return []WindowPage{
		{ID: PageOverview, Title: L("Overview", "概览"), Icon: "layout-grid", Body: func(c *ui.Context, width float32) {
			OverviewPage(c, OverviewData{Power: d.power, Thermal: d.thermal, System: d.system, History: d.history}, width)
		}},
		{ID: PageSystem, Title: L("System", "系统"), Icon: "cpu", Body: func(c *ui.Context, width float32) {
			SystemPage(c, d.system, cpuTemp, d.history, width)
		}},
		{ID: PageCooling, Title: L("Cooling", "散热"), Icon: "fan", Body: func(c *ui.Context, width float32) {
			CoolingPage(c, d.fans, d.history, &d.cooling, width)
		}},
		{ID: PageBattery, Title: L("Battery", "电池"), Icon: "battery-full", Body: func(c *ui.Context, width float32) {
			BatteryPage(c, d.power, d.limit, width)
		}},
		{ID: PageMenuBar, Title: L("Menu Bar", "菜单栏"), Icon: "panel-top", Body: func(c *ui.Context, width float32) {
			MenuBarPage(c, d.menu, d.menuCtx, &d.barState, width)
		}},
		{ID: PageMenuBarManager, Title: L("Menu Bar Manager", "菜单栏管理"), Icon: "app-window", Body: func(c *ui.Context, width float32) {
			MenuBarManagerPage(c, d.store, d.tray, &d.mgrState, width)
		}},
	}
}

// The power states the popover shows, as the four situations a MacBook goes through.
const (
	stateACPowered = iota
	stateCharging
	stateDischarging
	stateSupplement
)

// demoPower is this Mac's battery in one power state. The readings are one MacBook Pro's: 90 W
// adapter, 3S1P pack, 98% health, 75 cycles, 85% charge limit.
func demoPower(state int, now time.Time) power.Data {
	layout := power.CellLayout{Kind: power.SeriesParallel, SeriesCount: 3, ParallelCount: 1, ParallelCountKnown: true}
	d := power.Data{
		ACAdapterWattage:         90,
		AdapterRatedVoltageMV:    intp(20000),
		AdapterRatedCurrentMA:    intp(4500),
		SystemVoltageMV:          intp(20210),
		SystemCurrentMA:          intp(761),
		BatteryVoltageMV:         intp(12580),
		BatteryAmperageMA:        intp(0),
		CycleCount:               intp(75),
		BatteryHealthPercent:     intp(98),
		DesignCapacityMAH:        intp(5600),
		NominalChargeCapacityMAH: intp(5695),
		DesignCycleCount:         intp(1000),
		FullChargeCapacityMAH:    intp(5543),
		CellVoltagesMV:           []int{4193, 4192, 4193},
		QmaxMAH:                  []int{6066, 6085, 6112},
		BatteryCellLayout:        &layout,
		BatteryParallelCellCurrents: []power.ParallelCellCurrent{
			{BankID: 0, CellID: 0, CurrentMA: 1078},
			{BankID: 1, CellID: 0, CurrentMA: 1295},
			{BankID: 2, CellID: 0, CurrentMA: 1495},
		},
		BatterySerial:          strp("F8YHAC00EHL0000FWK"),
		DeviceName:             strp("bq40z651"),
		BatteryManufactureDate: strp("2024-10"),
		AverageSystemPowerW:    fptr(8.42),
		ChargeLimitPercent:     85,
		ChargeLimitSource:      power.ChargeLimitMacOSSetting,
		DataSource:             power.SourceTelemetry,
		Timestamp:              now,
	}
	switch state {
	case stateCharging:
		d.ConnectionPhase = power.PhaseOnAC
		d.IsOnAC, d.IsCharging = true, true
		d.BatteryPercent = 62
		d.SystemPowerW, d.ACInputW, d.BatteryPowerW = 15.4, 33.9, -18.5
		d.RemainingCapacityMAH, d.SmoothedChargePowerW = intp(3410), fptr(18.5)
		d.BatteryAmperageMA = intp(-1470)
	case stateDischarging:
		d.ConnectionPhase = power.PhaseOnBattery
		d.BatteryPercent = 47
		d.SystemPowerW, d.BatteryPowerW = 11.2, 11.2
		d.BatteryVoltageMV = intp(12470)
		d.RemainingCapacityMAH, d.SmoothedDischargePowerW = intp(2580), fptr(11.2)
		d.BatteryAmperageMA, d.InstantAmperageMA = intp(898), intp(898)
	case stateSupplement:
		d.ConnectionPhase = power.PhaseOnAC
		d.IsOnAC = true
		d.BatteryPercent = 38
		d.ACAdapterWattage = 45
		d.SystemPowerW, d.ACInputW, d.BatteryPowerW = 41.8, 20.5, 21.3
		d.BatteryVoltageMV = intp(12310)
		d.RemainingCapacityMAH, d.SmoothedDischargePowerW = intp(2080), fptr(21.3)
		d.BatteryAmperageMA, d.InstantAmperageMA = intp(1730), intp(1730)
	default:
		d.ConnectionPhase = power.PhaseOnAC
		d.IsOnAC = true
		d.BatteryPercent = 85
		d.SystemPowerW, d.ACInputW, d.BatteryPowerW = 15.4, 15.4, 0
		d.RemainingCapacityMAH = intp(4657)
		d.NotChargingReason = intp(0x01000000)
	}
	return d
}

// demoThermal is this Mac's sensors and fans while it idles on the desktop.
func demoThermal() thermal.Snapshot {
	return thermal.Snapshot{
		Fans: []thermal.FanReading{
			{Index: 0, ActualRPM: 2344, MinRPM: 2317, MaxRPM: 7826, TargetRPM: 2317},
			{Index: 1, ActualRPM: 2503, MinRPM: 2317, MaxRPM: 7826, TargetRPM: 2502},
		},
		Temperatures: map[thermal.SensorGroup]thermal.TemperatureSummary{
			thermal.GroupCPU:     {AverageC: 54.5, MaximumC: 80.9, SensorCount: 84},
			thermal.GroupGPU:     {AverageC: 52.5, MaximumC: 56.4, SensorCount: 44},
			thermal.GroupBattery: {AverageC: 34.8, MaximumC: 34.9, SensorCount: 3},
		},
		DisplayPowerW:  fptr(1.93),
		Mode:           thermal.Automatic,
		LastCustomMode: thermal.Constant(3000),
		HelperStatus:   thermal.HelperReady,
		Available:      true,
	}
}

// demoSystem is this Mac's load and memory, with the busiest processes it was running.
func demoSystem(now time.Time) system.Snapshot {
	perCore := []float64{0.43, 0.37, 0.30, 0.26, 0.02, 0.01, 0.01, 0.02,
		0.01, 0.00, 0.19, 0.15, 0.11, 0.08, 0.04, 0.03}
	kinds := make([]system.CoreKind, len(perCore))
	for i := range perCore {
		if i < 4 {
			kinds[i] = 1
		} else {
			kinds[i] = 2
		}
	}
	gb := uint64(1) << 30
	return system.Snapshot{
		CPU: system.CPULoad{
			User: 0.084, System: 0.039, Idle: 0.877, PerCore: perCore,
			Efficiency: 0.34, Performance: 0.052, HasEfficiency: true, HasPerformance: true,
		},
		Memory: &system.MemoryStats{
			TotalBytes: 128 * gb, AppBytes: 56 * gb, WiredBytes: 6 * gb, CompressedBytes: 1 * gb,
			Pressure: system.PressureNormal,
		},
		LoadAverage: []float64{3.06, 3.59, 3.77},
		BootTime:    now.Add(-118 * time.Hour),
		TopCPU: []system.ProcessUsage{
			{PID: 402, Name: "WindowServer", Value: 40.9},
			{PID: 66743, Name: "tern", Value: 23.9},
			{PID: 79857, Name: "ego lite", Value: 20.2},
			{PID: 893, Name: "WeType", Value: 16.3},
			{PID: 940, Name: "OrbStack Helper", Value: 14.9},
		},
		TopMemory: []system.ProcessUsage{
			{PID: 940, Name: "OrbStack Helper", Value: 1.4 * float64(gb)},
			{PID: 56104, Name: "mediaanalysisd", Value: 1.2 * float64(gb)},
			{PID: 3913, Name: "QQ", Value: 1.1 * float64(gb)},
			{PID: 66743, Name: "tern", Value: 1.0 * float64(gb)},
			{PID: 3923, Name: "QQ Helper (Renderer)", Value: 0.9 * float64(gb)},
		},
		CoreKinds: kinds,
	}
}

// demoHistory is the last 30 minutes of charts, ending at now.
func demoHistory(now time.Time) *history.History {
	h := &history.History{}
	const samples = 360
	for i := range samples {
		at := now.Add(time.Duration(i-samples+1) * history.SampleInterval)
		w := math.Sin(float64(i)/26)*3 + math.Sin(float64(i)/7)*0.9
		load := math.Sin(float64(i)/31)*0.08 + math.Sin(float64(i)/5)*0.02
		h.Append(history.Sample{Time: at, Values: map[history.Metric]float64{
			history.BatteryPercent: 62 + float64(i)*0.004,
			history.SystemPower:    15.4 + w,
			history.CPUTemperature: 54 + w*3.4,
			history.FanRPM:         2344 + w*180,
			history.BatteryTemp:    34.8 + w*0.4,
			history.CPUUsage:       18 + load*100,
			history.MemoryUsage:    46 + load*25,
		}})
	}
	return h
}

// demoLimit is the charge limit control, holding 85% as this Mac does.
func demoLimit() *power.ChargeLimiter {
	return power.ChargeLimiterWithState(power.ChargeLimitState{
		Loaded: true, Supported: true, Choices: []int{80, 85, 90, 95, 100}, Percent: 85,
	})
}

// demoDisplays is the built-in display and one external monitor on a DDC slider.
func demoDisplays() []display.Brightness {
	return []display.Brightness{
		{ID: 1, Name: "Built-in Retina Display", BuiltIn: true, Method: display.Native, Level: 0.62},
		{ID: 4, Name: "P275MV MAX", Method: display.DDC, Level: 0.8},
	}
}

// demoMenuBarContext is what the menu bar label reads, as the app gathers it.
func demoMenuBarContext(p power.Data, th thermal.Snapshot, s system.Snapshot) menubar.Context {
	ctx := menubar.Context{
		DataAvailable: true, OnAC: p.EffectiveIsOnAC(), BatteryPercent: p.DisplayBatteryPercent(),
		BatteryCurrentMA: p.BatteryAmperageMA, BatteryVoltageMV: p.BatteryVoltageMV,
		SystemPowerW: p.SystemPowerW, Health: p.BatteryHealthPercent, CycleCount: p.CycleCount,
	}
	if t := p.EstimatedTimeRemainingText(); t != "" {
		ctx.TimeRemaining = &t
	}
	if temp, ok := BatteryTemperatureC(p, th); ok {
		ctx.BatteryTemperatureC = &temp
	}
	if cpu, ok := th.Temperatures[thermal.GroupCPU]; ok {
		ctx.CPUTemperatureC = &cpu.AverageC
	}
	if th.HasFans() {
		rpm := th.MaxActualRPM()
		ctx.FanRPM = &rpm
	}
	usage := s.CPU.Usage()
	ctx.CPUUsage = &usage
	return ctx
}

// demoBarManager shows the menu bar manager with the menu bar apps a MacBook runs; iconDir holds
// each app's logo as <name>.png, and apps without one fall back to a lettered tile.
func demoBarManager(iconDir string) (*barmanager.ItemStore, *barmanager.TrayStore) {
	item := func(name, detail, section, icon string) barmanager.Item {
		sections := map[string]barmanager.Section{
			"shown": barmanager.Shown, "overflow": barmanager.Overflow, "absent": barmanager.Absent,
		}
		return barmanager.Item{
			ID: name + detail, Name: name, Detail: detail, AppPath: "/Applications/" + name + ".app",
			BundleID: "com.example." + name, Section: sections[section], LayoutKey: name + detail,
			Reachable: sections[section] == barmanager.Shown, Icon: icon,
		}
	}
	items := []barmanager.Item{
		item("PowerfulTop", "", "shown", ""),
		item("Stats", "CPU", "shown", ""),
		item("FineTune", "", "shown", ""),
		item("AlDente", "", "shown", ""),
		item("OrbStack", "", "shown", ""),
		item("WeChat", "", "overflow", ""),
		item("QQ", "", "overflow", ""),
		item("Safari", "", "absent", ""),
		item("ChatGPT", "", "absent", ""),
	}
	store := barmanager.NewPreview(barmanager.StoreSnapshot{
		Supported: true, Trusted: true, HasLayoutAccess: true, Items: items,
		Displays: []barmanager.Display{{Name: "Built-in Retina Display", Frame: barmanager.Rect{X: 0, Y: 0, W: 1512, H: 32}}},
	})
	entries := []barmanager.TrayEntry{
		{ID: "com.example.WeChat", Name: "WeChat", AppPath: "/Applications/WeChat.app", Running: true},
		{ID: "com.example.QQ", Name: "QQ", AppPath: "/Applications/QQ.app", Running: true},
		{ID: "com.apple.menuextra.wifi", Name: "Wi-Fi", NameZh: "Wi-Fi", Icon: "wifi", Running: true},
		{ID: "com.apple.menuextra.sound", Name: "Sound", NameZh: "声音", Icon: "volume-2", Running: true},
	}
	tray := barmanager.NewTrayPreview(barmanager.TraySnapshot{
		Supported: true, Enabled: true, HasAllowListAccess: true, Entries: entries,
	})
	for _, it := range items {
		if png, err := os.ReadFile(filepath.Join(iconDir, it.Name+".png")); err == nil {
			store.SetPreviewIcon(it.AppPath, png)
		}
	}
	for _, e := range entries {
		if png, err := os.ReadFile(filepath.Join(iconDir, e.Name+".png")); err == nil {
			tray.SetPreviewIcon(e.AppPath, png)
		}
	}
	return store, tray
}

// writeShot renders one view in a window of width×height and saves it, cropped to its content.
func writeShot(path string, width, height int, view func(*ui.Context)) error {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Background(c.Theme().Background).Children(func() { view(c) })
	}, width, height)
	// Let the library's arrivals and chart growth finish before the shot.
	tt.Advance(2 * time.Second)
	tt.Frame()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, cropBottom(tt.Image()))
}

// cropBottom drops the blank rows a canvas taller than the content leaves below it.
func cropBottom(img *image.RGBA) *image.RGBA {
	bg := img.RGBAAt(0, 0)
	bottom := img.Rect.Dy()
	for ; bottom > 1; bottom-- {
		blank := true
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			if img.RGBAAt(x, bottom-1) != bg {
				blank = false
				break
			}
		}
		if !blank {
			break
		}
	}
	return img.SubImage(image.Rect(0, 0, img.Rect.Dx(), bottom)).(*image.RGBA)
}

func fptr(v float64) *float64 { return &v }
