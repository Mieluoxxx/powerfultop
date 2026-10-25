package view

import (
	"fmt"

	"powerfultop/internal/power"
)

// Display rules shared by the popover and the details window so the two never disagree.

// BatteryLevelTint: green while charging, red at 20% and below, orange up to 50%.
func BatteryLevelTint(d power.Data) Tint {
	switch p := d.DisplayBatteryPercent(); {
	case d.IsBatteryCharging():
		return Green
	case p <= 20:
		return Red
	case p <= 50:
		return Orange
	}
	return Green
}

// BatteryHealthTint grades health: green from 80%, orange from 60%, red below, gray unknown.
func BatteryHealthTint(d power.Data) Tint {
	switch h := d.BatteryHealthPercent; {
	case h == nil:
		return Gray
	case *h >= 80:
		return Green
	case *h >= 60:
		return Orange
	}
	return Red
}

// BatteryLevelIcon is the Lucide stand-in for the SF Symbol battery.0…battery.100 ladder.
func BatteryLevelIcon(percent int) string {
	switch {
	case percent <= 10:
		return "battery"
	case percent <= 35:
		return "battery-low"
	case percent <= 85:
		return "battery-medium"
	}
	return "battery-full"
}

// SignedBatteryPowerText is the battery flow for display: "+" into the pack, "−" out of it.
func SignedBatteryPowerText(d power.Data) string {
	switch s := d.PowerState(); s.Kind {
	case power.StateCharging:
		return "+" + fmt.Sprintf("%.1f W", s.RateW)
	case power.StateSupplementing:
		return "−" + fmt.Sprintf("%.1f W", s.RateW)
	case power.StateOnBattery:
		return "−" + fmt.Sprintf("%.1f W", d.SystemPowerW)
	}
	return fmt.Sprintf("%.1f W", 0.0)
}
