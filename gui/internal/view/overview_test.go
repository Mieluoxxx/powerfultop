package view

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/history"
	"powerfultop/internal/power"
	"powerfultop/internal/system"
	"powerfultop/internal/thermal"
)

func TestOverviewPage(t *testing.T) {
	var h history.History
	start := time.Unix(1_700_000_000, 0)
	for i := range 5 {
		h.Append(history.Sample{Time: start.Add(time.Duration(i) * history.SampleInterval), Values: map[history.Metric]float64{
			history.SystemPower: 10 + float64(i), history.BatteryPercent: 85,
		}})
	}
	mem := system.MemoryStats{TotalBytes: 128 << 30, AppBytes: 40 << 30, Pressure: system.PressureNormal}
	d := OverviewData{
		Power: power.Data{SystemPowerW: 11.07, BatteryPercent: 85, IsOnAC: true, ConnectionPhase: power.PhaseOnAC,
			ACAdapterWattage: 65, DesignCapacityMAH: intp(8579), BatteryHealthPercent: intp(99), CycleCount: intp(74)},
		Thermal: thermal.Snapshot{Fans: []thermal.FanReading{{ActualRPM: 1200, MaxRPM: 6000}}, Temperatures: map[thermal.SensorGroup]thermal.TemperatureSummary{
			thermal.GroupCPU: {AverageC: 39.5, MaximumC: 45},
		}},
		System:  system.Snapshot{Memory: &mem, LoadAverage: []float64{2.5}},
		History: &h,
	}
	tt := ui.NewTester(func(c *ui.Context) { OverviewPage(c, d, 950) }, 950, 1400)
	for _, want := range []string{"85%", "Charger", "Battery Health", "8579 mAh", "Power Adapter", "65 W", "Battery Level", "System Power", "CPU Usage", "Load 2.50", "Memory", "CPU Average", "39.5°C", "Fan Speed", "1200 RPM", "Controlled by macOS"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
}
