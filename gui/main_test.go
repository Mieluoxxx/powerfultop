package main

import (
	"errors"
	"testing"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/power"
)

func TestPopoverWhileCharging(t *testing.T) {
	a := &app{data: power.Snapshot{
		Percent: 85, OnAC: true, Charging: true,
		SystemW: 11.066, BatteryW: -37.141, ACInputW: 48.207, AdapterW: 96,
	}}
	tt := ui.NewTester(a.view, 300, 250)
	for _, want := range []string{"85%", "AC Charging", "11.1 W", "48.2 W", "96 W adapter", "Charging 37.1 W"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
}

func TestPopoverOnBattery(t *testing.T) {
	a := &app{data: power.Snapshot{Percent: 40, SystemW: 9.751, BatteryW: 9.751}}
	tt := ui.NewTester(a.view, 300, 250)
	if !tt.HasText("Battery Discharging") || !tt.HasText("Discharging 9.8 W") {
		t.Errorf("texts %q", tt.Texts())
	}
	if tt.HasText("Charger") {
		t.Error("no charger row without a charger")
	}
}

func TestPopoverWithoutBattery(t *testing.T) {
	a := &app{err: power.ErrNoBattery}
	tt := ui.NewTester(a.view, 300, 250)
	if !tt.HasText("Built-in battery not detected. PowerfulTop requires a MacBook.") {
		t.Errorf("texts %q", tt.Texts())
	}
	if trayTitle(power.Snapshot{}, errors.New("x")) != "—" {
		t.Error("tray title should show a dash without data")
	}
	if got := trayTitle(power.Snapshot{SystemW: 23.4}, nil); got != "23W" {
		t.Errorf("tray title %q", got)
	}
}
