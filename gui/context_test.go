package main

import (
	"testing"
	"time"

	"powerfultop/internal/history"
	"powerfultop/internal/macos"
	"powerfultop/internal/power"
	"powerfultop/internal/system"
	"powerfultop/internal/thermal"
)

func ip(v int) *int { return &v }

func TestMenuBarContextCharging(t *testing.T) {
	p := power.Data{SystemPowerW: 11.066, BatteryPowerW: -37.141, ACInputW: 48.207, BatteryPercent: 85, IsOnAC: true,
		ConnectionPhase: power.PhaseOnAC, SystemCurrentMA: ip(2400), SystemVoltageMV: ip(20000), BatteryHealthPercent: ip(99)}
	th := thermal.Snapshot{Fans: []thermal.FanReading{{ActualRPM: 1500}, {ActualRPM: 1800}}}
	ctx := menuBarContext(p, true, th, system.Snapshot{})
	if !ctx.OnAC || ctx.BatteryPercent != 85 || ctx.SystemPowerW != 11.066 {
		t.Errorf("basics: %+v", ctx)
	}
	if ctx.BatteryPowerW == nil || *ctx.BatteryPowerW <= 0 {
		t.Errorf("charging power flows into the pack: %v", ctx.BatteryPowerW)
	}
	if ctx.AdapterVoltageV == nil || *ctx.AdapterVoltageV != 20 || ctx.FanRPM == nil || *ctx.FanRPM != 1800 {
		t.Errorf("adapter %v fan %v", ctx.AdapterVoltageV, ctx.FanRPM)
	}
	if ctx.CPUUsage != nil || ctx.MemoryUsage != nil {
		t.Error("no CPU or memory readings yet")
	}
}

func TestHistorySampleSkipsMissingReadings(t *testing.T) {
	s := historySample(time.Unix(0, 0), power.Data{SystemPowerW: 9, BatteryPercent: 50}, thermal.Snapshot{}, system.Snapshot{})
	if s.Values[history.SystemPower] != 9 || s.Values[history.BatteryPercent] != 50 {
		t.Errorf("values %v", s.Values)
	}
	if _, ok := s.Values[history.FanRPM]; ok {
		t.Error("no fan reading without fans")
	}
}

func TestMigrateLegacyDefaults(t *testing.T) {
	current := macos.DomainPreferences("io.github.mieluoxxx.PowerfulTop.gotest.current")
	legacy := macos.DomainPreferences("io.github.mieluoxxx.PowerfulTop.gotest.legacy")
	t.Cleanup(func() {
		for _, k := range []string{"a", "b", legacyMigratedKey, "NSStatusItem VisibleCC Item-0"} {
			_ = current.Set(k, nil)
			_ = legacy.Set(k, nil)
		}
	})
	_ = legacy.Set("NSStatusItem VisibleCC Item-0", false)
	_ = legacy.Set("a", "old")
	_ = legacy.Set("b", "old")
	_ = current.Set("b", "new")
	migrateLegacyDefaults(current, legacy)
	if v, _ := current.String("a"); v != "old" {
		t.Errorf("a = %q, should come from the legacy domain", v)
	}
	if _, ok := current.Value("NSStatusItem VisibleCC Item-0"); ok {
		t.Error("AppKit's status item state must not migrate")
	}
	if v, _ := current.String("b"); v != "new" {
		t.Errorf("b = %q, values set since the rename win", v)
	}
	_ = legacy.Set("a", "changed")
	_ = current.Set("a", nil)
	migrateLegacyDefaults(current, legacy)
	if _, ok := current.Value("a"); ok {
		t.Error("migration runs once")
	}
}

func TestClearStaleStatusItemVisibility(t *testing.T) {
	p := macos.DomainPreferences("io.github.mieluoxxx.PowerfulTop.gotest.visibility")
	keys := []string{"NSStatusItem VisibleCC Item-0", "NSStatusItem Visible " + trayID, "NSStatusItem VisibleCC Other"}
	t.Cleanup(func() {
		for _, k := range keys {
			_ = p.Set(k, nil)
		}
	})
	for _, k := range keys {
		_ = p.Set(k, false)
	}
	clearStaleStatusItemVisibility(p)
	for i, k := range keys {
		_, ok := p.Value(k)
		if want := i == 2; ok != want {
			t.Errorf("%s kept = %v, want %v", k, ok, want)
		}
	}
}
