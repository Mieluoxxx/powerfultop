// Package power reads the battery and power telemetry macOS keeps on AppleSmartBattery.
package power

import (
	"errors"
	"math"
	"time"
)

// BatteryFlowThresholdW is the battery flow below which a reading is noise, not charge or discharge.
const BatteryFlowThresholdW = 0.3

// Snapshot is one reading of the built-in battery and what flows through it.
type Snapshot struct {
	Percent      int
	OnAC         bool
	Charging     bool
	FullyCharged bool
	// SystemW is what the Mac draws right now, from the charger, the battery or both.
	SystemW float64
	// BatteryW follows PowerData: positive while discharging, negative while charging.
	BatteryW float64
	ACInputW float64
	// AdapterW is the charger's rating, 0 when none is connected or it isn't reported.
	AdapterW int
	Time     time.Time
}

// ErrNoBattery means the Mac has no built-in battery to read.
var ErrNoBattery = errors.New("no built-in battery")

// Flow is the system and battery power resolved from one snapshot.
type Flow struct {
	SystemW  float64
	BatteryW float64
}

// ResolveFlow turns IOKit's readings into system and battery power.
//
// IOKit reports Amperage and BatteryPower as negative while discharging (observed on battery:
// Amperage -791 mA, BatteryPower -9751 mW, SystemLoad 9751 mW), so they flip here. Within a
// snapshot SystemLoad == SystemPowerIn - BatteryPower, which makes SystemLoad the direct system
// consumption in every power state.
func ResolveFlow(onAC bool, acInputW float64, systemLoadW, ioBatteryW, ioAmperageW, systemInputW *float64) Flow {
	ioBattery := 0.0
	if ioBatteryW != nil && math.Abs(*ioBatteryW) > BatteryFlowThresholdW {
		ioBattery = *ioBatteryW
	} else if ioAmperageW != nil && math.Abs(*ioAmperageW) > BatteryFlowThresholdW {
		ioBattery = *ioAmperageW
	}

	// Without a charger the pack can only discharge; a positive reading there is a stale snapshot.
	batteryW := math.Abs(ioBattery)
	if onAC {
		batteryW = -ioBattery
	}

	var systemW float64
	switch {
	case systemLoadW != nil && *systemLoadW > 0:
		systemW = *systemLoadW
	case onAC:
		systemW = max(0, acInputW+batteryW)
	default:
		systemW = batteryW
	}
	if systemW == 0 && onAC && systemInputW != nil && *systemInputW > 0 {
		systemW = *systemInputW
	}
	return Flow{SystemW: systemW, BatteryW: batteryW}
}

// Read takes one compact snapshot of the built-in battery; the Monitor gives the full Data.
func Read() (Snapshot, error) {
	d, ok := ReadData()
	if !ok {
		return Snapshot{}, ErrNoBattery
	}
	return SnapshotOf(d), nil
}

// SnapshotOf condenses Data; charger fields stay zero while unplugged, where they go stale.
func SnapshotOf(d Data) Snapshot {
	s := Snapshot{
		Percent: d.BatteryPercent, OnAC: d.IsOnAC, Charging: d.IsCharging, FullyCharged: d.FullyCharged,
		SystemW: d.SystemPowerW, BatteryW: d.BatteryPowerW, ACInputW: d.ACInputW, Time: d.Timestamp,
	}
	if d.IsOnAC {
		s.AdapterW = d.ACAdapterWattage
	}
	return s
}

func ptr(v float64) *float64 { return &v }
