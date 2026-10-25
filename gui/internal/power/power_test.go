package power

import (
	"testing"
	"time"
)

func f(v float64) *float64 { return &v }

// fromRegistry parses without touching pmset, AlDente or the battery pack service.
func fromRegistry(props map[string]any) Data {
	return dataFromRegistry(props, nil, nil, time.Unix(0, 0),
		func(_, _ *int) ResolvedChargeLimit { return FullCharge },
		func(map[string]any, time.Time) *string { return nil })
}

// Cases mirror testPowerFlowUsesIOKitDischargeSign in the Swift regression tests.
func TestResolveFlowUsesIOKitDischargeSign(t *testing.T) {
	cases := []struct {
		name string
		got  Flow
		want Flow
	}{
		{"battery discharge", ResolveFlow(false, 0, f(9.751), f(-9.751), f(-9.751), nil), Flow{9.751, 9.751}},
		{"AC idle", ResolveFlow(true, 11.501, f(11.501), f(0), nil, f(11.498)), Flow{11.501, 0}},
		{"AC + battery supplement", ResolveFlow(true, 20, f(25), f(-5), f(-5), f(20)), Flow{25, 5}},
		{"AC charging", ResolveFlow(true, 40, f(30), f(10), f(10), f(40)), Flow{30, -10}},
		{"live charging sample", ResolveFlow(true, 48.207, f(11.066), f(37.141), f(37.141), f(48.207)), Flow{11.066, -37.141}},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, c.got, c.want)
		}
	}
}

func TestSnapshotFromRegistry(t *testing.T) {
	props := map[string]any{
		"ExternalConnected": true,
		"IsCharging":        true,
		"CurrentCapacity":   int64(85),
		"MaxCapacity":       int64(100),
		"Voltage":           int64(12500),
		"Amperage":          int64(800),
		"AdapterDetails":    map[string]any{"Watts": int64(96)},
		"PowerTelemetryData": map[string]any{
			"SystemLoad":    int64(11066),
			"SystemPowerIn": int64(48207),
			"BatteryPower":  int64(37141),
		},
	}
	s := SnapshotOf(fromRegistry(props))
	if s.Percent != 85 || !s.OnAC || !s.Charging || s.AdapterW != 96 {
		t.Errorf("fields: %+v", s)
	}
	if s.SystemW != 11.066 || s.BatteryW != -37.141 || s.ACInputW != 48.207 {
		t.Errorf("power: system %v battery %v input %v", s.SystemW, s.BatteryW, s.ACInputW)
	}

	props["ExternalConnected"] = false
	if s := SnapshotOf(fromRegistry(props)); s.ACInputW != 0 || s.AdapterW != 0 {
		t.Errorf("stale charger fields must be ignored on battery: %+v", s)
	}
}
