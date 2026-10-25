package view

import (
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/power"
)

func ip(v int) *int { return &v }

func sampleBattery() power.Data {
	f := func(v float64) *float64 { return &v }
	return power.Data{
		SystemPowerW: 11.066, BatteryPowerW: -37.141, ACInputW: 48.207, ACAdapterWattage: 96,
		AdapterRatedVoltageMV: ip(20_000), AdapterRatedCurrentMA: ip(4_800),
		BatteryPercent: 85, IsOnAC: true, IsCharging: true, ConnectionPhase: power.PhaseOnAC,
		WallPowerW: f(53.068), AdapterEfficiencyLossW: f(4.9),
		SystemVoltageMV: ip(20_100), SystemCurrentMA: ip(2_400),
		BatteryVoltageMV: ip(12_570), BatteryAmperageMA: ip(2_896), InstantAmperageMA: ip(2_900),
		CycleCount: ip(74), DesignCycleCount: ip(1000), AdapterDescription: strp("pd charger"),
		BatteryHealthPercent: ip(99), DesignCapacityMAH: ip(5_600), RawMaxCapacityMAH: ip(5_560),
		NominalChargeCapacityMAH: ip(5_700), FullChargeCapacityMAH: ip(5_560), RemainingCapacityMAH: ip(4_700),
		ChargingVoltageMV: ip(12_600), ChargingCurrentMA: ip(2_900), NotChargingReason: ip(0), VacVoltageLimit: ip(4_400),
		CellVoltagesMV: []int{4_190, 4_189, 4_191}, QmaxMAH: []int{6_066, 6_085, 6_112},
		BatteryCellLayout:           &power.CellLayout{Kind: power.SeriesParallel, SeriesCount: 3, ParallelCount: 1, ParallelCountKnown: true},
		BatteryParallelCellCurrents: []power.ParallelCellCurrent{{BankID: 0, CellID: 0, CurrentMA: 1112}},
		DailyMinSoc:                 ip(75), DailyMaxSoc: ip(85),
		ChargeLimitPercent: 85, ChargeLimitSource: power.ChargeLimitMacOSSetting,
		TotalOperatingTimeMin: ip(12_000), LifetimeMaxTempC: ip(52), LifetimeMinTempC: ip(8), LifetimeAvgTempC: ip(305),
		LifetimeMaxPackVoltageMV: ip(13_050), LifetimeMinPackVoltageMV: ip(9_800),
		LifetimeMaxChargeCurrentMA: ip(5_000), LifetimeMaxDischargeCurrentMA: ip(0xFFFFFF),
		BatteryCellDisconnectCount: ip(0), PermanentFailureStatus: ip(0), AtCriticalLevel: boolp(false),
		Timestamp: time.Unix(0, 0),
	}
}

func strp(s string) *string { return &s }
func boolp(b bool) *bool    { return &b }

func TestBatteryPageRenders(t *testing.T) {
	d := sampleBattery()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().Children(func() { BatteryPage(c, d, 900) })
	}, 900, 2000)
	for _, want := range []string{
		"Battery Health", "99%", "5600 mAh · 100%", "85% (macOS setting)", "75% – 85%", "/ 1000",
		"Charger & Charging", "48.2 W", "/ 96 W", "2.40 A", "/ 4.8 A", "/ 20 V", "Charging", "pd charger",
		"Cells & Live Readings", "12.57 V", "2.90 A (Charging)", "Battery topology: 3 series groups",
		"2 mV spread — Normal", "Series Group 1", "4.190 V · 6066 mAh", "Series Group 1 · Cell 1",
		"Lifetime Statistics", "200 hours", "30.5 °C", "0 (normal)",
	} {
		if !tt.HasText(want) {
			t.Errorf("missing %q", want)
		}
	}
	if tt.HasText("16777215 mA") {
		t.Error("garbage lifetime discharge current must be hidden")
	}
	if path := os.Getenv("SNAPSHOT_BATTERY"); path != "" {
		f, _ := os.Create(path)
		defer f.Close()
		_ = png.Encode(f, tt.Image())
	}
}

func TestBatteryPageOnBatteryHidesCharger(t *testing.T) {
	d := sampleBattery()
	d.IsOnAC, d.ConnectionPhase = false, power.PhaseOnBattery
	tt := ui.NewTester(func(c *ui.Context) { BatteryPage(c, d, 600) }, 600, 2000)
	if tt.HasText("Charger & Charging") {
		t.Error("no charger card on battery")
	}
	if !tt.HasText("Battery Health") {
		t.Error("health card always shows")
	}
}

func TestBatteryPresentation(t *testing.T) {
	d := sampleBattery()
	if BatteryLevelTint(d) != Green || SignedBatteryPowerText(d) != "+37.1 W" {
		t.Errorf("charging: %v %q", BatteryLevelTint(d), SignedBatteryPowerText(d))
	}
	d.IsOnAC, d.ConnectionPhase, d.BatteryPercent, d.SystemPowerW = false, power.PhaseOnBattery, 15, 9.751
	if BatteryLevelTint(d) != Red || SignedBatteryPowerText(d) != "−9.8 W" {
		t.Errorf("discharging low: %v %q", BatteryLevelTint(d), SignedBatteryPowerText(d))
	}
	for p, want := range map[int]string{5: "battery", 30: "battery-low", 70: "battery-medium", 95: "battery-full"} {
		if BatteryLevelIcon(p) != want {
			t.Errorf("%d%% → %s", p, BatteryLevelIcon(p))
		}
	}
	if BatteryHealthTint(power.Data{}) != Gray {
		t.Error("unknown health is gray")
	}
}
