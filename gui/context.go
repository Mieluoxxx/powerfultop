package main

import (
	"strings"
	"time"

	"powerfultop/internal/history"
	"powerfultop/internal/macos"
	"powerfultop/internal/menubar"
	"powerfultop/internal/power"
	"powerfultop/internal/system"
	"powerfultop/internal/thermal"
	"powerfultop/internal/view"
)

// menuBarContext gathers every value the menu bar label can show, as the Swift makeMenuBarContext.
func menuBarContext(p power.Data, available bool, th thermal.Snapshot, s system.Snapshot) menubar.Context {
	ctx := menubar.Context{
		DataAvailable:      available,
		OnAC:               p.EffectiveIsOnAC(),
		BatteryPercent:     p.DisplayBatteryPercent(),
		BatteryCurrentMA:   p.BatteryAmperageMA,
		BatteryVoltageMV:   p.BatteryVoltageMV,
		SystemPowerW:       p.SystemPowerW,
		SystemPowerWarning: p.MenuBarPowerShowsWarning(),
		Health:             p.BatteryHealthPercent,
		CycleCount:         p.CycleCount,
	}
	if t := p.EstimatedTimeRemainingText(); t != "" {
		ctx.TimeRemaining = &t
	}
	if t, ok := view.BatteryTemperatureC(p, th); ok {
		ctx.BatteryTemperatureC = &t
	}
	var batteryW float64
	switch state := p.PowerState(); state.Kind {
	case power.StateCharging:
		batteryW = state.RateW
	case power.StateSupplementing:
		batteryW = -state.RateW
	case power.StateOnBattery:
		batteryW = -p.SystemPowerW
	}
	ctx.BatteryPowerW = &batteryW
	if p.HasSettledAC() {
		if sc := p.SystemCurrentMA; sc != nil {
			a := float64(*sc) / 1000
			ctx.AdapterCurrentA = &a
		}
		if sv := p.SystemVoltageMV; sv != nil {
			v := float64(*sv) / 1000
			ctx.AdapterVoltageV = &v
		}
		w := p.EffectiveACOutputW()
		ctx.AdapterPowerW = &w
	}
	if len(s.CPU.PerCore) > 0 {
		u := s.CPU.Usage()
		ctx.CPUUsage = &u
	}
	if m := s.Memory; m != nil {
		u := m.UsedFraction()
		ctx.MemoryUsage = &u
	}
	if cpu, ok := th.Temperatures[thermal.GroupCPU]; ok {
		ctx.CPUTemperatureC = &cpu.AverageC
	}
	if th.HasFans() {
		rpm := th.MaxActualRPM()
		ctx.FanRPM = &rpm
	}
	return ctx
}

// historySample is one row of the dashboard charts, as the Swift HistoryRecorder records it.
func historySample(now time.Time, p power.Data, th thermal.Snapshot, s system.Snapshot) history.Sample {
	values := map[history.Metric]float64{
		history.BatteryPercent: float64(p.DisplayBatteryPercent()),
		history.SystemPower:    p.SystemPowerW,
	}
	if cpu, ok := th.Temperatures[thermal.GroupCPU]; ok {
		values[history.CPUTemperature] = cpu.AverageC
	}
	if th.HasFans() {
		values[history.FanRPM] = th.MaxActualRPM()
	}
	if t, ok := view.BatteryTemperatureC(p, th); ok {
		values[history.BatteryTemp] = t
	}
	if len(s.CPU.PerCore) > 0 {
		values[history.CPUUsage] = s.CPU.Usage() * 100
	}
	if m := s.Memory; m != nil {
		values[history.MemoryUsage] = m.UsedFraction() * 100
	}
	return history.Sample{Time: now, Values: values}
}

const (
	// Builds before the rename stored their preferences under the upstream bundle ID.
	legacyBundleID    = "com.kdolphin.PowerTop"
	legacyMigratedKey = "migratedFromLegacyBundleID"
)

// migrateLegacyDefaults copies values the current domain doesn't have yet; anything set since the rename wins.
func migrateLegacyDefaults(current, legacy macos.Preferences) {
	if done, _ := current.Bool(legacyMigratedKey); done {
		return
	}
	have := map[string]bool{}
	for _, k := range current.Keys() {
		have[k] = true
	}
	for _, k := range legacy.Keys() {
		// AppKit's state for the old app's own status items, not a setting: carried over it hid ours.
		if have[k] || strings.HasPrefix(k, statusItemKeyPrefix) {
			continue
		}
		if v, ok := legacy.Value(k); ok {
			_ = current.Set(k, v)
		}
	}
	_ = current.Set(legacyMigratedKey, true)
}

const statusItemKeyPrefix = "NSStatusItem "

// clearStaleStatusItemVisibility removes "hidden" marks for the tray's status item. MyGo creates it
// under AppKit's default autosave name before renaming it, so a stale "Item-0" mark (earlier
// builds migrated one from the upstream app) hides it for good; the item doesn't allow removal,
// so no such mark is ever the user's choice.
func clearStaleStatusItemVisibility(p macos.Preferences) {
	for _, k := range p.Keys() {
		if !strings.HasPrefix(k, statusItemKeyPrefix) {
			continue
		}
		name := k[strings.LastIndex(k, " ")+1:]
		if name != "Item-0" && name != trayID {
			continue
		}
		if visible, ok := p.Bool(k); ok && !visible {
			_ = p.Set(k, nil)
		}
	}
}
