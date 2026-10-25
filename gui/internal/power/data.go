package power

import (
	"fmt"
	"math"
	"time"
)

// DataSource tells whether power came from PowerTelemetryData or the Amperage × Voltage fallback.
type DataSource int

const (
	SourceTelemetry DataSource = iota
	SourceLegacy
)

// CellLayoutKind is how macOS exposes per-cell battery telemetry on this machine.
type CellLayoutKind int

const (
	// PerCellArrays: pack-level CellVoltage / Qmax arrays, one entry per physical cell.
	PerCellArrays CellLayoutKind = iota
	// SeriesParallel: bank/cell nodes, voltage and Qmax per series group, currents per parallel cell.
	SeriesParallel
)

// CellLayout describes the pack; ParallelCountKnown is false when no cell nodes were found.
type CellLayout struct {
	Kind               CellLayoutKind
	SeriesCount        int
	ParallelCount      int
	ParallelCountKnown bool
}

type ParallelCellCurrent struct {
	BankID    int
	CellID    int
	CurrentMA int
}

// ConnectionPhase is the user-visible connection phase driven by IOPS events and telemetry convergence.
type ConnectionPhase int

const (
	PhaseOnBattery ConnectionPhase = iota
	PhaseConnectingAC
	PhaseOnAC
)

// StateKind classifies one snapshot plus the connection phase.
type StateKind int

const (
	StateOnBattery StateKind = iota
	StateConnectingAC
	StateACIdle
	StateCharging
	// StateSupplementing: AC is connected but the battery still discharges to cover the load.
	StateSupplementing
)

// State is the settled power situation; RateW is set for charging and supplementing.
type State struct {
	Kind  StateKind
	RateW float64
}

// Data is one display snapshot of the built-in battery and power flow (the Swift PowerData).
// Optional readings are nil when this Mac doesn't report them.
type Data struct {
	SystemPowerW     float64 // SystemLoad / 1000, actual system consumption
	BatteryPowerW    float64 // positive = discharge, negative = charge
	ACInputW         float64 // SystemPowerIn / 1000, total DC from the charger
	ACAdapterWattage int
	// Negotiated USB-PD contract from AdapterDetails.
	AdapterRatedVoltageMV  *int
	AdapterRatedCurrentMA  *int
	BatteryPercent         int
	IsOnAC                 bool
	IsCharging             bool
	FullyCharged           bool
	WallPowerW             *float64
	AdapterEfficiencyLossW *float64
	SystemVoltageMV        *int
	SystemCurrentMA        *int
	BatteryVoltageMV       *int
	BatteryAmperageMA      *int
	BatteryTemperatureC    *float64
	CycleCount             *int
	AdapterDescription     *string
	DataSource             DataSource
	Timestamp              time.Time
	ConnectionPhase        ConnectionPhase

	BatteryHealthPercent     *int
	DesignCapacityMAH        *int
	RawMaxCapacityMAH        *int
	NominalChargeCapacityMAH *int
	DesignCycleCount         *int

	ChargingVoltageMV *int
	ChargingCurrentMA *int
	NotChargingReason *int
	VacVoltageLimit   *int

	CellVoltagesMV              []int
	StateOfCharge               *int
	QmaxMAH                     []int
	BatteryCellLayout           *CellLayout
	BatteryParallelCellCurrents []ParallelCellCurrent
	DailyMinSoc                 *int
	DailyMaxSoc                 *int
	// ChargeLimitPercent is the effective charge target (1–100); 100 means physical full charge.
	ChargeLimitPercent int
	ChargeLimitSource  ChargeLimitSource

	TotalOperatingTimeMin         *int
	LifetimeMaxTempC              *int
	LifetimeMinTempC              *int
	LifetimeAvgTempC              *int // decidegrees, as IOKit reports it
	LifetimeMaxPackVoltageMV      *int
	LifetimeMinPackVoltageMV      *int
	LifetimeMaxChargeCurrentMA    *int
	LifetimeMaxDischargeCurrentMA *int

	BatterySerial              *string
	DeviceName                 *string
	InstantAmperageMA          *int
	AtCriticalLevel            *bool
	PermanentFailureStatus     *int
	BatteryCellDisconnectCount *int

	AvgTimeToEmptyMinutes  *int
	AvgTimeToFullMinutes   *int
	RemainingCapacityMAH   *int
	FullChargeCapacityMAH  *int
	AverageSystemPowerW    *float64
	BatteryManufactureDate *string
	// EMA-smoothed discharge/charge power for time estimates, set by the Monitor.
	SmoothedDischargePowerW *float64
	SmoothedChargePowerW    *float64
}

// Empty is the snapshot before the first reading or without a battery.
var Empty = Data{ChargeLimitPercent: 100}

// normalized clamps the charge limit as the Swift initializer does.
func (d Data) normalized() Data {
	if d.ChargeLimitPercent == 0 {
		d.ChargeLimitPercent = 100
	}
	d.ChargeLimitPercent = min(max(d.ChargeLimitPercent, 1), 100)
	return d
}

// DisplayBatteryPercent is the percent as macOS shows it in the menu bar, clamped for display.
func (d Data) DisplayBatteryPercent() int {
	return min(100, max(0, d.BatteryPercent))
}

// PowerState is the one classification every view and estimate derives from.
func (d Data) PowerState() State {
	// ExternalConnected flips before telemetry refreshes; trust the phase machine first.
	if d.ConnectionPhase == PhaseOnBattery || !d.IsOnAC {
		return State{Kind: StateOnBattery}
	}
	if d.ConnectionPhase == PhaseConnectingAC {
		return State{Kind: StateConnectingAC}
	}
	if d.BatteryPowerW > BatteryFlowThresholdW {
		// Battery outflow without measured AC input is a stale snapshot, not a supplement.
		if d.ACInputW > 0 {
			return State{Kind: StateSupplementing, RateW: d.BatteryPowerW}
		}
		return State{Kind: StateACIdle}
	}
	if d.BatteryPowerW < -BatteryFlowThresholdW {
		return State{Kind: StateCharging, RateW: -d.BatteryPowerW}
	}
	// Trickle charging near the charge limit reports IsCharging with negligible flow.
	if d.IsCharging {
		return State{Kind: StateCharging, RateW: max(0, -d.BatteryPowerW)}
	}
	return State{Kind: StateACIdle}
}

func (d Data) IsConnectingAC() bool  { return d.PowerState().Kind == StateConnectingAC }
func (d Data) EffectiveIsOnAC() bool { return d.PowerState().Kind != StateOnBattery }

// HasSettledAC is plugged in and past the connecting window, so adapter readings are current.
func (d Data) HasSettledAC() bool {
	switch d.PowerState().Kind {
	case StateACIdle, StateCharging, StateSupplementing:
		return true
	}
	return false
}

func (d Data) IsBatteryCharging() bool       { return d.PowerState().Kind == StateCharging }
func (d Data) IsSupplementalDischarge() bool { return d.PowerState().Kind == StateSupplementing }

// BatteryChargeRateW is the battery charge rate in watts (positive).
func (d Data) BatteryChargeRateW() float64 {
	if s := d.PowerState(); s.Kind == StateCharging {
		return s.RateW
	}
	return 0
}

// BatterySupplementalW is how much power the battery contributes during supplemental discharge.
func (d Data) BatterySupplementalW() float64 {
	if s := d.PowerState(); s.Kind == StateSupplementing {
		return s.RateW
	}
	return 0
}

// EffectiveACInputW is AC input once the connection has settled; zero while unplugged or connecting.
func (d Data) EffectiveACInputW() float64 {
	if d.HasSettledAC() {
		return d.ACInputW
	}
	return 0
}

// EffectiveACOutputW is the charger output, falling back to load + charge without SystemPowerIn.
func (d Data) EffectiveACOutputW() float64 {
	s := d.PowerState()
	switch s.Kind {
	case StateACIdle, StateSupplementing:
		if d.ACInputW > 0 {
			return d.ACInputW
		}
		return d.SystemPowerW
	case StateCharging:
		if d.ACInputW > 0 {
			return d.ACInputW
		}
		return d.SystemPowerW + s.RateW
	}
	return 0
}

func (d Data) PowerSourceDescription() string {
	switch d.PowerState().Kind {
	case StateOnBattery:
		return l("Battery Discharging", "电池放电")
	case StateConnectingAC:
		return l("AC Connecting", "AC 连接中")
	case StateACIdle:
		return l("AC Powered", "AC 供电")
	case StateCharging:
		return l("AC Charging", "AC 充电中")
	default:
		return l("AC + Battery Supplement", "AC + 电池补充供电")
	}
}

// MenuBarPowerW is the raw system load shown in the menu bar across all power states.
func (d Data) MenuBarPowerW() float64 { return d.SystemPowerW }

// MenuBarPowerRoundedW is the rounded menu bar power, capped at 99 W.
func (d Data) MenuBarPowerRoundedW() int {
	return min(99, max(0, int(math.Round(d.MenuBarPowerW()))))
}

func (d Data) MenuBarPowerExceedsCap() bool { return d.MenuBarPowerW() > 99 }

// MenuBarPowerShowsBatteryWarning: battery still discharging despite a connected charger.
func (d Data) MenuBarPowerShowsBatteryWarning() bool { return d.IsSupplementalDischarge() }

// MenuBarPowerShowsWarning: the menu bar prefixes a warning symbol, as colors don't show there.
func (d Data) MenuBarPowerShowsWarning() bool {
	return d.MenuBarPowerShowsBatteryWarning() || d.MenuBarPowerExceedsCap()
}

// MenuBarPowerText is the compact label, e.g. "19W" or "⚠ 33W".
func (d Data) MenuBarPowerText() string {
	power := fmt.Sprintf("%dW", d.MenuBarPowerRoundedW())
	if d.MenuBarPowerShowsWarning() {
		return "⚠ " + power
	}
	return power
}

func (d Data) ValidAvgTimeToEmptyMinutes() *int {
	return ValidBatteryTimeMinutes(d.AvgTimeToEmptyMinutes)
}
func (d Data) ValidAvgTimeToFullMinutes() *int {
	return ValidBatteryTimeMinutes(d.AvgTimeToFullMinutes)
}

func firstInt(values ...*int) *int {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func (d Data) soc() int {
	if d.StateOfCharge != nil {
		return *d.StateOfCharge
	}
	return d.BatteryPercent
}

// RemainingEnergyWh is the stored energy from coulomb count × pack voltage.
func (d Data) RemainingEnergyWh() *float64 {
	if d.RemainingCapacityMAH != nil && d.BatteryVoltageMV != nil && *d.RemainingCapacityMAH > 0 && *d.BatteryVoltageMV > 0 {
		return ptr(float64(*d.RemainingCapacityMAH) * float64(*d.BatteryVoltageMV) / 1e6)
	}
	capacity := firstInt(d.FullChargeCapacityMAH, d.RawMaxCapacityMAH, d.NominalChargeCapacityMAH)
	if capacity == nil || *capacity <= 0 || d.BatteryVoltageMV == nil || *d.BatteryVoltageMV <= 0 {
		return nil
	}
	soc := d.soc()
	if soc <= 0 {
		return nil
	}
	return ptr(float64(*capacity) * float64(soc) / 100 * float64(*d.BatteryVoltageMV) / 1e6)
}

func (d Data) IsAtChargeLimit() bool {
	if !d.IsBatteryCharging() || d.soc() < d.ChargeLimitPercent {
		return false
	}
	// SOC can read above the hold point while the pack is still accepting charge.
	return d.BatteryChargeRateW() < 1.0
}

// EnergyToFullWh is the energy still needed to reach the effective charge target.
func (d Data) EnergyToFullWh() *float64 {
	if !d.IsBatteryCharging() || d.IsAtChargeLimit() {
		return nil
	}
	full := firstInt(d.FullChargeCapacityMAH, d.RawMaxCapacityMAH, d.NominalChargeCapacityMAH)
	if full == nil || *full <= 0 || d.BatteryVoltageMV == nil || *d.BatteryVoltageMV <= 0 {
		return nil
	}
	voltage := float64(*d.BatteryVoltageMV)
	soc := d.soc()
	if soc >= d.ChargeLimitPercent {
		return nil
	}
	target := *full * d.ChargeLimitPercent / 100
	if d.RemainingCapacityMAH != nil && *d.RemainingCapacityMAH > 0 {
		if delta := target - *d.RemainingCapacityMAH; delta > 0 {
			return ptr(float64(delta) * voltage / 1e6)
		}
	}
	if socDelta := d.ChargeLimitPercent - soc; socDelta > 0 {
		return ptr(float64(*full) * float64(socDelta) / 100 * voltage / 1e6)
	}
	return nil
}

func (d Data) dischargePowerForEstimateW() *float64 {
	if d.SmoothedDischargePowerW != nil && *d.SmoothedDischargePowerW >= 0.2 {
		return d.SmoothedDischargePowerW
	}
	if d.IsSupplementalDischarge() {
		if p := d.BatterySupplementalW(); p >= 0.2 {
			return ptr(p)
		}
		return nil
	}
	if !d.EffectiveIsOnAC() && d.SystemPowerW >= 0.2 {
		return ptr(d.SystemPowerW)
	}
	return nil
}

func (d Data) chargePowerForEstimateW() *float64 {
	if d.SmoothedChargePowerW != nil && *d.SmoothedChargePowerW >= 0.2 {
		return d.SmoothedChargePowerW
	}
	if rate := d.BatteryChargeRateW(); rate >= 0.2 {
		return ptr(rate)
	}
	return nil
}

func (d Data) computedTimeToEmptyMinutes() *int {
	if d.IsConnectingAC() || d.IsBatteryCharging() {
		return nil
	}
	if d.EffectiveIsOnAC() && !d.IsSupplementalDischarge() {
		return nil
	}
	energy, power := d.RemainingEnergyWh(), d.dischargePowerForEstimateW()
	if energy == nil || power == nil {
		return nil
	}
	return minutesFromEnergy(*energy, *power)
}

func (d Data) computedTimeToFullMinutes() *int {
	energy, power := d.EnergyToFullWh(), d.chargePowerForEstimateW()
	if energy == nil || power == nil {
		return nil
	}
	return minutesFromEnergy(*energy, *power)
}

// scaledAvgTimeToFullForLimit scales macOS time-to-full (100%) down when a lower target applies.
func (d Data) scaledAvgTimeToFullForLimit() *int {
	avg := d.ValidAvgTimeToFullMinutes()
	if avg == nil {
		return nil
	}
	if d.ChargeLimitPercent >= 100 {
		return avg
	}
	soc := d.soc()
	if soc >= d.ChargeLimitPercent {
		return nil
	}
	toFull, toTarget := 100-soc, d.ChargeLimitPercent-soc
	if toFull <= 0 || toTarget <= 0 {
		return nil
	}
	m := cappedMinutes(*avg * toTarget / toFull)
	return &m
}

func (d Data) EstimatedTimeRemainingMinutes() *int {
	if d.IsBatteryCharging() {
		if d.IsAtChargeLimit() {
			return nil
		}
		if d.ChargeLimitPercent < 100 {
			return firstInt(d.computedTimeToFullMinutes(), d.scaledAvgTimeToFullForLimit(), d.ValidAvgTimeToFullMinutes())
		}
		return firstInt(d.ValidAvgTimeToFullMinutes(), d.computedTimeToFullMinutes())
	}
	if !d.EffectiveIsOnAC() || d.IsSupplementalDischarge() {
		return firstInt(d.ValidAvgTimeToEmptyMinutes(), d.computedTimeToEmptyMinutes())
	}
	return nil
}

// EstimatedTimeIsComputed: macOS supplied no valid AvgTimeTo* value and our own model was used.
func (d Data) EstimatedTimeIsComputed() bool {
	if d.EstimatedTimeRemainingMinutes() == nil {
		return false
	}
	if d.IsBatteryCharging() {
		if d.computedTimeToFullMinutes() != nil {
			return true
		}
		if d.ChargeLimitPercent < 100 && d.scaledAvgTimeToFullForLimit() != nil {
			return true
		}
		return d.ValidAvgTimeToFullMinutes() == nil
	}
	return d.ValidAvgTimeToEmptyMinutes() == nil
}

func (d Data) EstimatedTimeRemainingText() string {
	if m := d.EstimatedTimeRemainingMinutes(); m != nil {
		return FormatDuration(*m)
	}
	return ""
}

func (d Data) EstimatedTimeRemainingLabel() string {
	if d.IsBatteryCharging() {
		if d.ChargeLimitPercent < 100 {
			return lf("Est. Time to %d%%", "预计充至 %d%%", d.ChargeLimitPercent)
		}
		return l("Est. Time to Full", "预计充满时间")
	}
	return l("Est. Time to Empty", "预计剩余时间")
}

func (d Data) EstimatedTimeFootnote() string {
	if !d.IsBatteryCharging() || !d.EstimatedTimeIsComputed() {
		return ""
	}
	if source := ChargeLimitSourceLabel(d.ChargeLimitSource); d.ChargeLimitPercent < 100 && source != "" {
		return lf("Estimated to %d%% charge limit (%s).", "按 %d%% 充电上限估算（%s）。", d.ChargeLimitPercent, source)
	}
	return l("Estimated from remaining energy and smoothed power (macOS no longer provides this).",
		"根据剩余电量与平滑功耗估算（macOS 已不再提供此数据）。")
}

// FormatDuration writes minutes as "1h 5m", or "5m" under an hour.
func FormatDuration(minutes int) string {
	hours, mins := minutes/60, minutes%60
	if hours > 0 {
		return lf("%dh %dm", "%d 小时 %d 分钟", hours, mins)
	}
	return lf("%dm", "%d 分钟", mins)
}

func minutesFromEnergy(energyWh, powerW float64) *int {
	if powerW < 0.2 || energyWh <= 0 {
		return nil
	}
	m := cappedMinutes(int(energyWh / powerW * 60))
	return &m
}

func cappedMinutes(minutes int) int {
	return min(max(0, minutes), 99*60+59)
}

func (d Data) NotChargingReasonDescription() string {
	if d.NotChargingReason == nil || *d.NotChargingReason == 0 {
		return ""
	}
	reason := *d.NotChargingReason
	switch {
	case reason&0x01000000 != 0:
		return l("Optimized Battery Charging", "优化电池充电")
	case reason&0x00000002 != 0:
		return l("Temperature Too High", "温度过高")
	case reason&0x00000001 != 0:
		return l("Battery Fully Charged", "电池已充满")
	case reason&0x00000004 != 0:
		return l("Battery Abnormal", "电池异常")
	case reason&0x00000080 != 0:
		return l("Charger Power Insufficient", "充电器功率不足")
	}
	return lf("Reason Code %d (0x%X)", "原因代码 %d (0x%X)", reason, reason)
}

func (d Data) WithConnectionPhase(phase ConnectionPhase) Data {
	d.ConnectionPhase = phase
	return d
}

func (d Data) WithSmoothedPower(dischargeW, chargeW *float64) Data {
	d.SmoothedDischargePowerW, d.SmoothedChargePowerW = dischargeW, chargeW
	return d
}

// ValidBatteryTimeMinutes drops macOS's invalid sentinel (65535) for battery time estimates.
func ValidBatteryTimeMinutes(minutes *int) *int {
	if minutes == nil || *minutes <= 0 || *minutes >= 65_535 {
		return nil
	}
	return minutes
}

// TelemetryEnergyToWatts converts WallEnergyEstimate, AdapterEfficiencyLoss and SystemEnergyConsumed,
// whose unit equals mW / 3.6 (observed: SystemEnergyConsumed × 3.6 == SystemPowerIn).
func TelemetryEnergyToWatts(value int) float64 {
	return float64(value) * 3.6 / 1000
}
