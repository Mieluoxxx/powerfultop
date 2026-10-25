//go:build darwin

package power

import (
	"errors"
	"testing"
)

// Reads this Mac's battery through purego, so a broken binding fails here rather than in the app.
func TestReadLive(t *testing.T) {
	s, err := Read()
	if errors.Is(err, ErrNoBattery) {
		t.Skip("no built-in battery")
	}
	if err != nil {
		t.Fatal(err)
	}
	if s.Percent <= 0 || s.Percent > 100 || s.SystemW <= 0 {
		t.Errorf("implausible reading: %+v", s)
	}
	t.Logf("%+v", s)
}

func TestReadDataLive(t *testing.T) {
	d, ok := ReadData()
	if !ok {
		t.Skip("no built-in battery")
	}
	if d.DesignCapacityMAH == nil || d.CycleCount == nil || d.BatteryVoltageMV == nil {
		t.Errorf("missing core fields: %+v", d)
	}
	if d.HasCellData() && d.CellTopologyDescription() == "" {
		t.Error("cells without a topology")
	}
	t.Logf("state=%s health=%v limit=%s cells=%q date=%v est=%q",
		d.PowerSourceDescription(), deref(ptrIntToFloat(d.BatteryHealthPercent)), d.ChargeLimitText(),
		d.CellTopologyDescription(), d.BatteryManufactureDate, d.EstimatedTimeRemainingText())

	m := NewMonitor()
	m.Start()
	defer m.Stop()
	if !m.Available() || m.Snapshot().BatteryPercent != d.BatteryPercent {
		t.Errorf("monitor snapshot %+v", m.Snapshot())
	}
}

func ptrIntToFloat(v *int) *float64 {
	if v == nil {
		return nil
	}
	f := float64(*v)
	return &f
}

// Reads the native charge limit through PowerUIAgent, the path the Battery page writes through.
func TestPowerUILive(t *testing.T) {
	s, err := (&powerUI{}).read()
	if errors.Is(err, errPowerUIUnavailable) {
		t.Skip("PowerUI charge limit unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	if s.supported && (len(s.limits) == 0 || s.limits[len(s.limits)-1] != 100) {
		t.Errorf("implausible limits: %+v", s)
	}
	t.Logf("%+v", s)
}
