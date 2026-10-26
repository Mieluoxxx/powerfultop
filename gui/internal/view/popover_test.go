package view

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/display"
	"powerfultop/internal/power"
	"powerfultop/internal/system"
	"powerfultop/internal/thermal"
)

func intp(v int) *int { return &v }

func TestPopoverSections(t *testing.T) {
	mem := system.MemoryStats{TotalBytes: 128 << 30, AppBytes: 40 << 30, Pressure: system.PressureNormal}
	d := PopoverData{
		Available: true,
		Power: power.Data{
			SystemPowerW: 11.07, BatteryPercent: 85, IsOnAC: true, ACAdapterWattage: 65, ConnectionPhase: power.PhaseOnAC,
			BatteryHealthPercent: intp(99), CycleCount: intp(74), ChargeLimitPercent: 77,
		},
		Thermal: thermal.Snapshot{Temperatures: map[thermal.SensorGroup]thermal.TemperatureSummary{
			thermal.GroupCPU: {AverageC: 39, MaximumC: 45, SensorCount: 4},
		}},
		System:   system.Snapshot{Memory: &mem},
		Displays: []display.Brightness{{ID: 1, Name: "Built-in Retina Display", BuiltIn: true, Level: 0.6}},
		Version:  "1.4.0",
	}
	var opened []PageID
	a := PopoverActions{OpenDetails: func(p PageID) { opened = append(opened, p) }}
	tt := ui.NewTester(func(c *ui.Context) { Popover(c, d, a) }, PopoverWidth, 700)
	for _, want := range []string{"85%", "Charger", "65 W", "System", "11.1 W", "CPU", "39°C", "Memory", "Built-in Retina Display", "99%", "Health", "74", "Charge Limit", "Details"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	// The charge limit row spans the popover like the rows above it: its value sits at the right edge.
	if r, ok := tt.Find(d.Power.ChargeLimitText()); !ok || r.X+r.W < PopoverWidth-40 {
		t.Errorf("charge limit value %q at %+v, want it at the right edge", d.Power.ChargeLimitText(), r)
	}
	if err := tt.Click("Details"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Charge Limit"); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 2 || opened[1] != PageBattery {
		t.Errorf("opened %v, want the charge limit row to open the Battery page", opened)
	}
}

func TestPopoverOnBattery(t *testing.T) {
	d := PopoverData{Available: true, Power: power.Data{SystemPowerW: 9.75, BatteryPowerW: 9.75, BatteryPercent: 15}}
	tt := ui.NewTester(func(c *ui.Context) { Popover(c, d, PopoverActions{}) }, PopoverWidth, 600)
	if !tt.HasText("Battery") || tt.HasText("Charger") || tt.HasText("Charge Limit") {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestPopoverSetsChargeLimit(t *testing.T) {
	limit := power.ChargeLimiterWithState(power.ChargeLimitState{Loaded: true, Supported: true, Choices: []int{80, 85, 90, 95, 100}, Percent: 85})
	changed := make(chan struct{}, 4)
	limit.OnChange(func() { changed <- struct{}{} })
	d := PopoverData{Available: true, ChargeLimit: limit,
		Power: power.Data{BatteryPercent: 85, IsOnAC: true, ConnectionPhase: power.PhaseOnAC, BatteryHealthPercent: intp(99), ChargeLimitPercent: 85}}
	tt := ui.NewTester(func(c *ui.Context) { Popover(c, d, PopoverActions{}) }, PopoverWidth, 700)
	for _, want := range []string{"Charge Limit", "80%", "95%", "100%"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	if err := tt.Click("95%"); err != nil {
		t.Fatal(err)
	}
	for limit.State().Pending {
		<-changed
	}
	if got := limit.State().Percent; got != 95 {
		t.Errorf("limit = %d after choosing 95%% in the popover", got)
	}
}
