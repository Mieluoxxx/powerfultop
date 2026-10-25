// Package power reads the battery and power telemetry macOS keeps on AppleSmartBattery.
package power

import (
	"errors"
	"math"
	"time"

	"powerfultop/internal/macos"
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

// Read takes one snapshot of the built-in battery.
func Read() (Snapshot, error) {
	services, err := macos.ServiceProperties("AppleSmartBattery")
	if err != nil {
		return Snapshot{}, err
	}
	if len(services) == 0 {
		return Snapshot{}, ErrNoBattery
	}
	return snapshot(services[0], time.Now()), nil
}

func snapshot(props map[string]any, now time.Time) Snapshot {
	onAC, _ := macos.Bool(props, "ExternalConnected")
	charging, _ := macos.Bool(props, "IsCharging")
	full, _ := macos.Bool(props, "FullyCharged")
	current, _ := macos.Int(props, "CurrentCapacity")
	maxCapacity, ok := macos.Int(props, "MaxCapacity")
	if !ok {
		maxCapacity = 100
	}
	percent := 0
	if maxCapacity > 0 {
		percent = int(current * 100 / maxCapacity)
	}

	s := Snapshot{Percent: percent, OnAC: onAC, Charging: charging, FullyCharged: full, Time: now}
	if adapter, ok := macos.Dict(props, "AdapterDetails"); ok && onAC {
		watts, _ := macos.Int(adapter, "Watts")
		s.AdapterW = int(watts)
	}

	var amperageW *float64
	amperage, hasAmperage := macos.Int(props, "Amperage")
	voltage, hasVoltage := macos.Int(props, "Voltage")
	if hasAmperage && hasVoltage && amperage != 0 && voltage > 0 {
		amperageW = ptr(float64(amperage) * float64(voltage) / 1e6)
	}

	telemetry, ok := macos.Dict(props, "PowerTelemetryData")
	if !ok {
		flow := ResolveFlow(onAC, 0, nil, nil, amperageW, nil)
		s.SystemW, s.BatteryW = flow.SystemW, flow.BatteryW
		return s
	}
	systemLoad, _ := macos.Int(telemetry, "SystemLoad")
	systemPowerIn, _ := macos.Int(telemetry, "SystemPowerIn")
	batteryPower, _ := macos.Int(telemetry, "BatteryPower")
	// SystemPowerIn goes stale after unplugging; ExternalConnected is authoritative.
	if onAC {
		s.ACInputW = float64(systemPowerIn) / 1000
	}
	var systemInputW *float64
	sysVoltage, okV := macos.Int(telemetry, "SystemVoltageIn")
	sysCurrent, okC := macos.Int(telemetry, "SystemCurrentIn")
	if okV && okC && sysVoltage > 0 && sysCurrent > 0 {
		systemInputW = ptr(float64(sysVoltage) * float64(sysCurrent) / 1e6)
	}
	flow := ResolveFlow(onAC, s.ACInputW, ptr(float64(systemLoad)/1000), ptr(float64(batteryPower)/1000), amperageW, systemInputW)
	s.SystemW, s.BatteryW = flow.SystemW, flow.BatteryW
	return s
}

func ptr(v float64) *float64 { return &v }
