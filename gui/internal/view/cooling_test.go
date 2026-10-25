package view

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/history"
	"powerfultop/internal/thermal"
)

func coolingSnapshot(mode thermal.FanControlMode, status thermal.HelperStatus) thermal.Snapshot {
	return thermal.Snapshot{
		Available: true,
		Fans: []thermal.FanReading{
			{Index: 0, ActualRPM: 2400, MinRPM: 2317, MaxRPM: 7826, Mode: 3},
			{Index: 1, ActualRPM: 2500, MinRPM: 2317, MaxRPM: 7826, Mode: 3},
		},
		Temperatures: map[thermal.SensorGroup]thermal.TemperatureSummary{
			thermal.GroupCPU:     {AverageC: 52.3, MaximumC: 78, SensorCount: 16},
			thermal.GroupBattery: {AverageC: 31, MaximumC: 32, SensorCount: 2},
		},
		Mode:           mode,
		LastCustomMode: thermal.DefaultConstant,
		HelperStatus:   status,
	}
}

func renderCooling(t *testing.T, s thermal.Snapshot) *ui.Tester {
	t.Helper()
	m := thermal.MonitorWithSnapshot(s)
	var h history.History
	now := time.Unix(1_700_000_000, 0)
	for i := range 3 {
		h.Append(history.Sample{Time: now.Add(time.Duration(i) * 10 * time.Second), Values: map[history.Metric]float64{
			history.CPUTemperature: 50 + float64(i), history.FanRPM: 2400,
		}})
	}
	state := &CoolingState{}
	return ui.NewTester(func(c *ui.Context) { CoolingPage(c, m, &h, state, 760) }, 760, 1200)
}

func TestCoolingPageAutomatic(t *testing.T) {
	tt := renderCooling(t, coolingSnapshot(thermal.Automatic, thermal.HelperReady))
	for _, want := range []string{"Temperatures", "52.3°C", "CPU Average", "52.3°C avg · 78.0°C max", "Fans", "2500 RPM",
		"Controlled by macOS", "Fan 1", "2400 / 7826 RPM", "Auto", "Full Blast", "Custom", "Fan Speed"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	if tt.HasText("Target Speed") {
		t.Error("custom controls only show in Custom mode")
	}
}

func TestCoolingPageCustomNeedsHelper(t *testing.T) {
	tt := renderCooling(t, coolingSnapshot(thermal.Constant(3000), thermal.HelperNeedsUpdate))
	for _, want := range []string{"Constant RPM", "Follow Temperature", "Target Speed", "3000 RPM", "Update Fan Helper",
		"PowerfulTop was updated, so the fan helper must be authorized again."} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
}

func TestCoolingPageSensorRamp(t *testing.T) {
	s := coolingSnapshot(thermal.Sensor(thermal.GroupCPU, 55, 85), thermal.HelperReady)
	s.ConflictDetected = true
	tt := renderCooling(t, s)
	for _, want := range []string{"Sensor", "Start ramping at 55°C", "Full speed at 85°C", "Another fan tool is in control"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
}

func TestFanSectionAndHelperCard(t *testing.T) {
	s := coolingSnapshot(thermal.Sensor(thermal.GroupGPU, 50, 90), thermal.HelperReady)
	m := thermal.MonitorWithSnapshot(s)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() {
			FanSection(c, m)
			FanHelperCard(c, m)
		})
	}, 320, 600)
	for _, want := range []string{"2500 RPM", "GPU ramp 50–90°C", "Fan Helper", "Installed", "Uninstall"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
}

func TestHelperErrorText(t *testing.T) {
	if HelperErrorText(thermal.ErrCancelled) != "Authorization was cancelled." {
		t.Error("sentinel errors read as the Swift app's messages")
	}
}
