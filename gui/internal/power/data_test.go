package power

import (
	"math"
	"testing"
	"time"
)

func ip(v int) *int { return &v }

// makeData mirrors makeData in the Swift regression tests: 50% of a 5000 mAh, 10 V pack.
func makeData(system, battery, input float64, onAC, charging, full bool, phase ConnectionPhase, at time.Time) Data {
	return Data{
		SystemPowerW: system, BatteryPowerW: battery, ACInputW: input, ACAdapterWattage: 140,
		BatteryPercent: 50, IsOnAC: onAC, IsCharging: charging, FullyCharged: full,
		BatteryVoltageMV: ip(10_000), Timestamp: at, ConnectionPhase: phase,
		StateOfCharge: ip(50), RemainingCapacityMAH: ip(5_000), FullChargeCapacityMAH: ip(5_000),
	}.normalized()
}

func data(system, battery, input float64, onAC bool, phase ConnectionPhase) Data {
	return makeData(system, battery, input, onAC, false, false, phase, time.Unix(0, 0))
}

func approx(t *testing.T, got *float64, want float64, msg string) {
	t.Helper()
	if got == nil || math.Abs(*got-want) > 1e-6 {
		t.Errorf("%s: got %v, want %v", msg, deref(got), want)
	}
}

func deref(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func TestSupplementalEstimateUsesBatteryPower(t *testing.T) {
	d := data(25, 5, 20, true, PhaseOnAC)
	if !d.IsSupplementalDischarge() {
		t.Fatal("detect supplemental discharge")
	}
	if d.BatterySupplementalW() != 5 {
		t.Errorf("battery contribution %v", d.BatterySupplementalW())
	}
	if m := d.EstimatedTimeRemainingMinutes(); m == nil || *m != 600 {
		t.Errorf("estimate 50 Wh at 5 W as 10 hours, got %v", m)
	}
}

func TestDischargeEstimatorResetsAcrossPowerSources(t *testing.T) {
	var e Estimator
	discharge, _ := e.SmoothedPower(data(25, 5, 20, true, PhaseOnAC))
	approx(t, discharge, 5, "smooth battery contribution during supplemental discharge")
	discharge, _ = e.SmoothedPower(data(25, 25, 0, false, PhaseOnBattery))
	approx(t, discharge, 25, "reset EMA when switching to battery-only discharge")
}

func TestMenuBarAlwaysUsesSystemPower(t *testing.T) {
	cases := []struct {
		name string
		d    Data
		want float64
	}{
		{"on battery", data(18, 18, 0, false, PhaseOnBattery), 18},
		{"AC powered", data(15, 0, 15, true, PhaseOnAC), 15},
		{"charging", makeData(30, -10, 40, true, true, false, PhaseOnAC, time.Unix(0, 0)), 30},
		{"supplemental", data(25, 5, 20, true, PhaseOnAC), 25},
	}
	for _, c := range cases {
		if c.d.MenuBarPowerW() != c.want {
			t.Errorf("%s: %v", c.name, c.d.MenuBarPowerW())
		}
	}
	if !cases[2].d.IsBatteryCharging() {
		t.Error("detect active charging")
	}
	if makeData(80, -40, 120, true, true, false, PhaseOnAC, time.Unix(0, 0)).MenuBarPowerExceedsCap() {
		t.Error("do not warn for charger input above 99 W")
	}
	above := makeData(120, -20, 140, true, true, false, PhaseOnAC, time.Unix(0, 0))
	if !above.MenuBarPowerExceedsCap() || above.MenuBarPowerRoundedW() != 99 {
		t.Error("warn and cap system power above 99 W")
	}
	if got := above.MenuBarPowerText(); got != "⚠ 99W" {
		t.Errorf("menu bar text %q", got)
	}
}

func TestGapAloneIsNotSupplementalDischarge(t *testing.T) {
	d := data(25, 0, 20, true, PhaseOnAC)
	if d.IsSupplementalDischarge() || d.MenuBarPowerShowsWarning() {
		t.Error("do not infer supplement from a load/input gap")
	}
}

func TestEstimatorIgnoresRepeatedSnapshots(t *testing.T) {
	var e Estimator
	sample := func(w float64, seconds int64) *float64 {
		d, _ := e.SmoothedPower(makeData(w, w, 0, false, false, false, PhaseOnBattery, time.Unix(seconds, 0)))
		return d
	}
	approx(t, sample(10, 0), 10, "seed EMA with first snapshot")
	for range 30 {
		sample(20, 0)
	}
	approx(t, sample(20, 0), 10, "repeated polls of one snapshot do not move EMA")
	approx(t, sample(20, 60), 10+10*(1-math.Exp(-60.0/180.0)), "weight by snapshot age")
}

func TestTelemetryEnergyUnits(t *testing.T) {
	for _, c := range []struct {
		raw  int
		want float64
	}{{3194, 11.501}, {3321, 11.956}, {13390, 48.207}, {14741, 53.068}} {
		if math.Abs(TelemetryEnergyToWatts(c.raw)-c.want) >= 0.01 {
			t.Errorf("%d → %v, want %v", c.raw, TelemetryEnergyToWatts(c.raw), c.want)
		}
	}
}

func TestPowerStateClassification(t *testing.T) {
	state := func(system, battery, input float64, onAC, charging bool, phase ConnectionPhase) State {
		return makeData(system, battery, input, onAC, charging, false, phase, time.Unix(0, 0)).PowerState()
	}
	cases := []struct {
		name string
		got  State
		want State
	}{
		{"battery discharge", state(9.751, 9.751, 0, false, false, PhaseOnBattery), State{Kind: StateOnBattery}},
		{"AC idle at charge limit", state(11.501, 0, 11.501, true, false, PhaseOnAC), State{Kind: StateACIdle}},
		{"live charging sample", state(11.066, -37.141, 48.207, true, true, PhaseOnAC), State{StateCharging, 37.141}},
		{"battery supplements AC", state(25, 5, 20, true, false, PhaseOnAC), State{StateSupplementing, 5}},
		{"stale discharge while connecting", state(5.818, 5.818, 0, true, false, PhaseConnectingAC), State{Kind: StateConnectingAC}},
		{"discharge without AC input", state(5.818, 5.818, 0, true, false, PhaseOnAC), State{Kind: StateACIdle}},
		{"trickle charge", state(12, -0.1, 12.1, true, true, PhaseOnAC), State{StateCharging, 0.1}},
		{"ExternalConnected=false wins", state(12, 0, 12, false, false, PhaseOnAC), State{Kind: StateOnBattery}},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, c.got, c.want)
		}
	}
	charging := makeData(30, -10, 0, true, true, false, PhaseOnAC, time.Unix(0, 0))
	if charging.EffectiveACOutputW() != 40 {
		t.Errorf("charger output without SystemPowerIn: %v", charging.EffectiveACOutputW())
	}
}

func TestDetailDerivations(t *testing.T) {
	for _, c := range []struct {
		ma   int
		want string
	}{{2_896, "2.90 A (Charging)"}, {-791, "0.79 A (Discharging)"}, {0, "0.00 A (Idle)"}} {
		if got := BatteryCurrentText(c.ma); got != c.want {
			t.Errorf("%d mA: %q", c.ma, got)
		}
	}
	d := Data{
		SystemPowerW: 10, ACInputW: 10, BatteryPercent: 85, IsOnAC: true,
		DesignCapacityMAH:           ip(5_600),
		CellVoltagesMV:              []int{4_190, 4_189, 4_250},
		QmaxMAH:                     []int{6_066, 6_085, 6_112},
		BatteryCellLayout:           &CellLayout{Kind: SeriesParallel, SeriesCount: 3, ParallelCount: 1, ParallelCountKnown: true},
		BatteryParallelCellCurrents: []ParallelCellCurrent{{0, 0, 1_000}, {0, 1, 1_200}},
		ChargeLimitPercent:          85, ChargeLimitSource: ChargeLimitMacOSSetting,
	}.normalized()
	approx(t, d.CapacityFractionOfDesign(ip(5_560)), 5_560.0/5_600.0, "capacity relative to design")
	if d.CapacityFractionOfDesign(nil) != nil {
		t.Error("no fraction without a value")
	}
	checks := map[string]string{
		SeriesVoltageBalanceSummary([]int{4_190, 4_189, 4_191}):      "2 mV spread — Normal",
		SeriesVoltageBalanceSummary([]int{4_190, 4_189, 4_250}):      "61 mV spread — Fair",
		ParallelCurrentBalanceSummary(d.BatteryParallelCellCurrents): "200 mA max within-group spread — Worth Checking",
		d.ChargeLimitText():         "85% (macOS setting)",
		d.CellTopologyDescription(): "Battery topology: 3 series groups",
	}
	for got, want := range checks {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	unlimited := Data{BatteryPercent: 85, IsOnAC: true}.normalized()
	if unlimited.ChargeLimitText() != "Off" {
		t.Errorf("no limit reads as off: %q", unlimited.ChargeLimitText())
	}
	if !d.HasCellData() || d.HasLifetimeData() {
		t.Error("cell data present, lifetime absent")
	}
}

func TestCellTelemetryParsing(t *testing.T) {
	var banks []map[string]any
	for _, b := range [][3]int64{{0, 4190, 6066}, {1, 4189, 6085}, {2, 4191, 6112}} {
		banks = append(banks, map[string]any{"BankID": b[0], "BatteryData": map[string]any{"CellVoltage": b[1], "Qmax": b[2], "StateOfCharge": int64(85)}})
	}
	var cells []map[string]any
	for _, c := range [][2]int64{{2, 1417}, {0, 1112}, {1, 1271}} {
		cells = append(cells, map[string]any{"BankID": c[0], "CellID": int64(0), "CellData": map[string]any{"CellCurrent": c[1]}})
	}
	telemetry := ReadCellTelemetry(nil, banks, cells)
	if telemetry == nil {
		t.Fatal("no telemetry")
	}
	if !equalInts(telemetry.VoltagesMV, []int{4190, 4189, 4191}) || !equalInts(telemetry.QmaxMAH, []int{6066, 6085, 6112}) {
		t.Errorf("bank order: %v %v", telemetry.VoltagesMV, telemetry.QmaxMAH)
	}
	var currents []int
	for _, c := range telemetry.ParallelCellCurrents {
		currents = append(currents, c.CurrentMA)
	}
	if !equalInts(currents, []int{1112, 1271, 1417}) {
		t.Errorf("cell currents sorted by bank: %v", currents)
	}
	if telemetry.Layout != (CellLayout{SeriesParallel, 3, 1, true}) {
		t.Errorf("3S1P topology: %+v", telemetry.Layout)
	}
	if soc := ReadCapacitySnapshot(map[string]any{}, banks).StateOfCharge; soc == nil || *soc != 85 {
		t.Error("fall back to bank SoC when the pack omits it")
	}
	pack := ReadCellTelemetry(map[string]any{"CellVoltage": []any{int64(3900), int64(3910)}, "Qmax": []any{int64(5000), int64(5010)}}, nil, nil)
	if pack == nil || pack.Layout.Kind != PerCellArrays {
		t.Error("per-cell arrays without bank nodes")
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func packed(digits string) int64 {
	var v int64
	for _, b := range []byte(digits) {
		v = v<<8 | int64(b)
	}
	return v
}

func TestManufactureDateDecoding(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	str := func(s *string) string {
		if s == nil {
			return "<nil>"
		}
		return *s
	}
	cases := []struct {
		name    string
		encoded int64
		gauge   string
		want    string
	}{
		{"M5 issue sample", 52_987_853_550_643, "bq40z651", "2026-02"},
		{"live sample, case-insensitive", 56_286_354_946_354, "BQ40Z651", "2021-10"},
		{"current month", packed("108043"), "A1964", "2026-08"},
		{"other gauge encoding", 52_987_853_550_643, "SN7038", "<nil>"},
		{"impossible date", packed("032033"), "bq40z651", "<nil>"},
		{"future month", packed("109043"), "bq40z651", "<nil>"},
		{"non-digit bytes", packed("21A043"), "bq40z651", "<nil>"},
	}
	for _, c := range cases {
		if got := str(FormatManufactureDate(c.encoded, c.gauge, now)); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
	nested := map[string]any{"BatteryData": map[string]any{"ManufactureDate": int64(52_987_853_550_643)}}
	if got := str(ParseManufactureDate(nested, "bq40z651", now)); got != "2026-02" {
		t.Errorf("read ManufactureDate from BatteryData: %s", got)
	}
	if ParseManufactureDate(map[string]any{"ManufacturerData": []byte{4, 0x31, 0x39, 0x31, 0x34}}, "bq40z651", now) != nil {
		t.Error("never infer a date from ManufacturerData")
	}
}

func TestPowerdChargeLimitParsing(t *testing.T) {
	const active = `Battery level limits:
(
        {
        Terminated = 0;
        chargeSocLimitDrain = 1;
        chargeSocLimitOwner = 473;
        chargeSocLimitReason = manualChargeLimit;
        chargeSocLimitSoc = 85;
    },
        {
        chargeSocLimitReason = manualChargeLimit;
        chargeSocLimitSoc = 90;
    }
)`
	if v := ParsePowerdChargeLimit(active); v == nil || *v != 85 {
		t.Errorf("strictest powerd limit applies: %v", v)
	}
	if ParsePowerdChargeLimit("No battery level limits set") != nil {
		t.Error("no limit when powerd reports none")
	}
	if ParsePowerdChargeLimit("chargeSocLimitSoc = 100;") != nil {
		t.Error("100% means no limit")
	}
}

func TestResolveChargeLimit(t *testing.T) {
	if got := resolveChargeLimit(ip(85), ip(80), nil, nil); got != (ResolvedChargeLimit{80, ChargeLimitAlDente}) {
		t.Errorf("strictest of macOS and AlDente: %+v", got)
	}
	if got := resolveChargeLimit(nil, nil, ip(80), ip(0x0100_0000)); got != (ResolvedChargeLimit{80, ChargeLimitOptimizedCharging}) {
		t.Errorf("optimized charging hold: %+v", got)
	}
	if got := resolveChargeLimit(nil, nil, ip(80), ip(0)); got != FullCharge {
		t.Errorf("daily max alone is no limit: %+v", got)
	}
}

func TestChargeLimitEstimates(t *testing.T) {
	// Charging at 10 W from 50% to an 80% limit: 30% of 50 Wh = 15 Wh → 90 minutes.
	d := makeData(20, -10, 30, true, true, false, PhaseOnAC, time.Unix(0, 0))
	d.ChargeLimitPercent, d.ChargeLimitSource = 80, ChargeLimitMacOSSetting
	if m := d.EstimatedTimeRemainingMinutes(); m == nil || *m != 90 {
		t.Errorf("time to limit: %v", m)
	}
	if d.EstimatedTimeRemainingLabel() != "Est. Time to 80%" || d.EstimatedTimeFootnote() != "Estimated to 80% charge limit (macOS setting)." {
		t.Errorf("label %q footnote %q", d.EstimatedTimeRemainingLabel(), d.EstimatedTimeFootnote())
	}
	if d.EstimatedTimeRemainingText() != "1h 30m" {
		t.Errorf("text %q", d.EstimatedTimeRemainingText())
	}
	d.StateOfCharge = ip(80)
	d.BatteryPowerW = -0.5
	if !d.IsAtChargeLimit() || d.EstimatedTimeRemainingMinutes() != nil {
		t.Error("holding at the limit shows no estimate")
	}
	if ValidBatteryTimeMinutes(ip(65_535)) != nil {
		t.Error("65535 is the invalid sentinel")
	}
}

func TestNotChargingReason(t *testing.T) {
	cases := map[int]string{0x01000000: "Optimized Battery Charging", 0x2: "Temperature Too High", 0x1: "Battery Fully Charged", 0x4: "Battery Abnormal", 0x80: "Charger Power Insufficient", 0x40: "Reason Code 64 (0x40)"}
	for reason, want := range cases {
		d := Data{NotChargingReason: ip(reason)}
		if got := d.NotChargingReasonDescription(); got != want {
			t.Errorf("%#x: %q", reason, got)
		}
	}
	if (Data{NotChargingReason: ip(0)}).NotChargingReasonDescription() != "" {
		t.Error("zero means charging normally")
	}
}
