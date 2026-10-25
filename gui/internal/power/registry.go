package power

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"powerfultop/internal/macos"
)

func optInt(dict map[string]any, key string) *int {
	if dict == nil {
		return nil
	}
	v, ok := macos.Int(dict, key)
	if !ok {
		return nil
	}
	n := int(v)
	return &n
}

func optBool(dict map[string]any, key string) *bool {
	if dict == nil {
		return nil
	}
	v, ok := macos.Bool(dict, key)
	if !ok {
		return nil
	}
	return &v
}

func optString(dict map[string]any, key string) *string {
	if dict == nil {
		return nil
	}
	v, ok := macos.String(dict, key)
	if !ok {
		return nil
	}
	return &v
}

func optDict(dict map[string]any, key string) map[string]any {
	if dict == nil {
		return nil
	}
	v, _ := macos.Dict(dict, key)
	return v
}

func optIntArray(dict map[string]any, key string) []int {
	if dict == nil {
		return nil
	}
	items, ok := dict[key].([]any)
	if !ok {
		return nil
	}
	values := make([]int, 0, len(items))
	for _, item := range items {
		v, ok := macos.Int(map[string]any{"v": item}, "v")
		if !ok {
			return nil
		}
		values = append(values, int(v))
	}
	return values
}

func derefOr(v *int, fallback int) int {
	if v == nil {
		return fallback
	}
	return *v
}

// CapacitySnapshot holds the capacity and health fields of AppleSmartBattery; many live under
// BatteryData on Apple Silicon.
type CapacitySnapshot struct {
	DesignCapacityMAH        *int
	FullChargeCapacityMAH    *int
	RawMaxCapacityMAH        *int
	NominalChargeCapacityMAH *int
	RemainingCapacityMAH     *int
	StateOfCharge            *int
	DailyMinSoc              *int
	DailyMaxSoc              *int
	TemperatureCentidegrees  *int
}

func (s CapacitySnapshot) HealthPercent() *int {
	if s.DesignCapacityMAH == nil || *s.DesignCapacityMAH <= 0 {
		return nil
	}
	current := firstInt(s.RawMaxCapacityMAH, s.FullChargeCapacityMAH, s.NominalChargeCapacityMAH)
	if current == nil || *current <= 0 {
		return nil
	}
	h := min(100, *current*100 / *s.DesignCapacityMAH)
	return &h
}

// ReadCapacitySnapshot reads the pack; banks are AppleSmartBatteryBank entries, where some machines
// only report SoC and Qmax.
func ReadCapacitySnapshot(props map[string]any, banks []map[string]any) CapacitySnapshot {
	batteryData := optDict(props, "BatteryData")
	packOrBatteryData := func(key string) *int {
		return firstInt(optInt(props, key), optInt(batteryData, key))
	}
	var bankData map[string]any
	for _, bank := range banks {
		if d := optDict(bank, "BatteryData"); d != nil {
			bankData = d
			break
		}
	}
	return CapacitySnapshot{
		DesignCapacityMAH:        packOrBatteryData("DesignCapacity"),
		FullChargeCapacityMAH:    firstInt(optInt(batteryData, "FullChargeCapacity"), optInt(props, "FullChargeCapacity")),
		RawMaxCapacityMAH:        firstInt(packOrBatteryData("AppleRawMaxCapacity"), optInt(bankData, "AppleRawMaxCapacity")),
		NominalChargeCapacityMAH: packOrBatteryData("NominalChargeCapacity"),
		RemainingCapacityMAH:     firstInt(optInt(batteryData, "RemainingCapacity"), optInt(props, "RemainingCapacity")),
		StateOfCharge:            firstInt(optInt(batteryData, "StateOfCharge"), optInt(props, "StateOfCharge"), optInt(bankData, "StateOfCharge")),
		DailyMinSoc:              firstInt(optInt(batteryData, "DailyMinSoc"), optInt(props, "DailyMinSoc"), optInt(bankData, "DailyMinSoc")),
		DailyMaxSoc:              firstInt(optInt(batteryData, "DailyMaxSoc"), optInt(props, "DailyMaxSoc"), optInt(bankData, "DailyMaxSoc")),
		TemperatureCentidegrees:  firstInt(packOrBatteryData("Temperature"), optInt(bankData, "Temperature")),
	}
}

// CellTelemetry is the per-cell reading in one of the two layouts macOS uses.
type CellTelemetry struct {
	Layout               CellLayout
	VoltagesMV           []int
	QmaxMAH              []int
	ParallelCellCurrents []ParallelCellCurrent
}

type cellSlot struct{ bankID, cellID int }

// ReadCellTelemetry reads cell data. Apple Silicon exposes series groups via bank nodes; pack-level
// arrays are a fallback when each entry is a distinct physical cell.
func ReadCellTelemetry(packBatteryData map[string]any, banks, cells []map[string]any) *CellTelemetry {
	type bankReading struct {
		bankID    int
		voltageMV *int
		qmaxMAH   *int
	}
	var readings []bankReading
	for i, props := range banks {
		data := optDict(props, "BatteryData")
		voltage, qmax := optInt(data, "CellVoltage"), optInt(data, "Qmax")
		if voltage == nil && qmax == nil {
			continue
		}
		readings = append(readings, bankReading{derefOr(optInt(props, "BankID"), i), voltage, qmax})
	}
	if len(readings) > 0 {
		sort.SliceStable(readings, func(a, b int) bool { return readings[a].bankID < readings[b].bankID })
		var voltages, qmax []int
		for _, r := range readings {
			if r.voltageMV != nil {
				voltages = append(voltages, *r.voltageMV)
			}
			if r.qmaxMAH != nil {
				qmax = append(qmax, *r.qmaxMAH)
			}
		}
		if len(voltages) > 0 && len(voltages) == len(qmax) {
			currents := readParallelCellCurrents(cells)
			slots := readCellSlots(cells)
			parallel := inferredParallelCount(currents, slots)
			return &CellTelemetry{
				Layout: CellLayout{
					Kind:               SeriesParallel,
					SeriesCount:        len(voltages),
					ParallelCount:      parallel,
					ParallelCountKnown: parallel > 1 || len(currents) > 0 || len(slots) > 0,
				},
				VoltagesMV:           voltages,
				QmaxMAH:              qmax,
				ParallelCellCurrents: currents,
			}
		}
	}
	voltages, qmax := optIntArray(packBatteryData, "CellVoltage"), optIntArray(packBatteryData, "Qmax")
	if len(voltages) > 0 && len(voltages) == len(qmax) {
		return &CellTelemetry{Layout: CellLayout{Kind: PerCellArrays}, VoltagesMV: voltages, QmaxMAH: qmax}
	}
	return nil
}

func inferredParallelCount(currents []ParallelCellCurrent, slots []cellSlot) int {
	counts := map[int]int{}
	if len(currents) > 0 {
		for _, c := range currents {
			counts[c.BankID]++
		}
	} else {
		for _, s := range slots {
			counts[s.bankID]++
		}
	}
	best := 1
	if len(counts) > 0 {
		best = 0
		for _, n := range counts {
			best = max(best, n)
		}
	}
	return best
}

func readCellSlots(cells []map[string]any) []cellSlot {
	var slots []cellSlot
	for _, props := range cells {
		bank, cell := optInt(props, "BankID"), optInt(props, "CellID")
		if bank == nil || cell == nil {
			continue
		}
		slots = append(slots, cellSlot{*bank, *cell})
	}
	sort.SliceStable(slots, func(a, b int) bool {
		if slots[a].bankID != slots[b].bankID {
			return slots[a].bankID < slots[b].bankID
		}
		return slots[a].cellID < slots[b].cellID
	})
	return slots
}

func readParallelCellCurrents(cells []map[string]any) []ParallelCellCurrent {
	var currents []ParallelCellCurrent
	for _, props := range cells {
		bank, cell := optInt(props, "BankID"), optInt(props, "CellID")
		current := optInt(optDict(props, "CellData"), "CellCurrent")
		if bank == nil || cell == nil || current == nil {
			continue
		}
		currents = append(currents, ParallelCellCurrent{*bank, *cell, *current})
	}
	sort.SliceStable(currents, func(a, b int) bool {
		if currents[a].BankID != currents[b].BankID {
			return currents[a].BankID < currents[b].BankID
		}
		return currents[a].CellID < currents[b].CellID
	})
	return currents
}

// ReadManufactureDate reads only the explicit ManufactureDate field; ManufacturerData is vendor-defined.
func ReadManufactureDate(props map[string]any, now time.Time) *string {
	rootGauge := optString(props, "DeviceName")
	if rootGauge != nil {
		if date := ParseManufactureDate(props, *rootGauge, now); date != nil {
			return date
		}
	}
	packs, _ := macos.ServiceProperties("AppleSmartBatteryPack")
	if len(packs) == 0 {
		return nil
	}
	gauge := firstString(optString(packs[0], "DeviceName"), rootGauge)
	if gauge == nil {
		return nil
	}
	return ParseManufactureDate(packs[0], *gauge, now)
}

func firstString(values ...*string) *string {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func ParseManufactureDate(props map[string]any, gaugeName string, now time.Time) *string {
	if encoded, ok := macos.Int(props, "ManufactureDate"); ok {
		if date := FormatManufactureDate(encoded, gaugeName, now); date != nil {
			return date
		}
	}
	encoded, ok := macos.Int(optDict(props, "BatteryData"), "ManufactureDate")
	if !ok {
		return nil
	}
	return FormatManufactureDate(encoded, gaugeName, now)
}

// FormatManufactureDate decodes the six-byte ASCII date known Mac battery gauges use: the
// least-significant bytes read as YYMMDD, YY being years since 1992.
func FormatManufactureDate(encoded int64, gaugeName string, now time.Time) *string {
	if !isKnownManufactureDateGauge(gaugeName) || encoded <= 0 {
		return nil
	}
	raw := uint64(encoded)
	if raw > 0xFFFF_FFFF_FFFF {
		return nil
	}
	var digits [6]int
	for i := range digits {
		b := byte(raw >> (8 * i))
		if b < '0' || b > '9' {
			return nil
		}
		digits[i] = int(b - '0')
	}
	year := 1992 + digits[0]*10 + digits[1]
	month := digits[2]*10 + digits[3]
	day := digits[4]*10 + digits[5]
	if year < 2005 || year > 2091 || month < 1 || month > 12 || day < 1 {
		return nil
	}
	if time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC).Day() != day {
		return nil
	}
	now = now.UTC()
	if year*12+month > now.Year()*12+int(now.Month()) {
		return nil
	}
	// Day-level precision is not consistent across independent tools, so only the month is shown.
	s := fmt.Sprintf("%04d-%02d", year, month)
	return &s
}

func isKnownManufactureDateGauge(gaugeName string) bool {
	name := strings.ToLower(gaugeName)
	switch name {
	case "a1953", "a1964", "a1965":
		return true
	}
	if len(name) < 6 || name[0] != 'b' || name[1] != 'q' || name[2] < '0' || name[2] > '9' {
		return false
	}
	for _, c := range []byte(name[2:]) {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z') {
			return false
		}
	}
	return true
}

// ReadData takes one full snapshot of the built-in battery; available is false without one.
func ReadData() (Data, bool) {
	services, err := macos.ServiceProperties("AppleSmartBattery")
	if err != nil || len(services) == 0 {
		return Empty, false
	}
	// Bank and cell nodes are read once per refresh and shared by every parser below.
	banks, _ := macos.ServiceProperties("AppleSmartBatteryBank")
	cells, _ := macos.ServiceProperties("AppleSmartBatteryCell")
	now := time.Now()
	return dataFromRegistry(services[0], banks, cells, now, ResolveChargeLimit, ReadManufactureDate), true
}

func dataFromRegistry(
	props map[string]any, banks, cells []map[string]any, now time.Time,
	chargeLimit func(dailyMaxSoc, notChargingReason *int) ResolvedChargeLimit,
	manufactureDate func(map[string]any, time.Time) *string,
) Data {
	onAC := derefBool(optBool(props, "ExternalConnected"))
	current := derefOr(optInt(props, "CurrentCapacity"), 0)
	maxCapacity := derefOr(optInt(props, "MaxCapacity"), 100)
	percent := 0
	if maxCapacity > 0 {
		percent = current * 100 / maxCapacity
	}
	voltage, amperage := optInt(props, "Voltage"), optInt(props, "Amperage")
	// Telemetry is a snapshot taken at UpdateTime, not at poll time.
	sampleDate := now
	if t := optInt(props, "UpdateTime"); t != nil && *t > 0 {
		sampleDate = time.Unix(int64(*t), 0)
	}

	capacity := ReadCapacitySnapshot(props, banks)
	adapter := optDict(props, "AdapterDetails")
	charger := optDict(props, "ChargerData")
	batteryData := optDict(props, "BatteryData")
	cellTelemetry := ReadCellTelemetry(batteryData, banks, cells)
	notChargingReason := optInt(charger, "NotChargingReason")
	limit := chargeLimit(capacity.DailyMaxSoc, notChargingReason)
	lifetime := optDict(batteryData, "LifetimeData")

	d := Data{
		ACAdapterWattage:      derefOr(optInt(adapter, "Watts"), 0),
		AdapterRatedVoltageMV: optInt(adapter, "AdapterVoltage"),
		AdapterRatedCurrentMA: optInt(adapter, "Current"),
		AdapterDescription:    optString(adapter, "Description"),
		BatteryPercent:        percent,
		IsOnAC:                onAC,
		IsCharging:            derefBool(optBool(props, "IsCharging")),
		FullyCharged:          derefBool(optBool(props, "FullyCharged")),
		BatteryVoltageMV:      voltage,
		BatteryAmperageMA:     amperage,
		CycleCount:            optInt(props, "CycleCount"),
		Timestamp:             sampleDate,

		BatteryHealthPercent:     capacity.HealthPercent(),
		DesignCapacityMAH:        capacity.DesignCapacityMAH,
		RawMaxCapacityMAH:        capacity.RawMaxCapacityMAH,
		NominalChargeCapacityMAH: capacity.NominalChargeCapacityMAH,
		DesignCycleCount:         firstInt(optInt(props, "DesignCycleCount9C"), optInt(props, "DesignCycleCount70")),

		ChargingVoltageMV: optInt(charger, "ChargingVoltage"),
		ChargingCurrentMA: optInt(charger, "ChargingCurrent"),
		NotChargingReason: notChargingReason,
		VacVoltageLimit:   optInt(charger, "VacVoltageLimit"),

		StateOfCharge:      capacity.StateOfCharge,
		DailyMinSoc:        capacity.DailyMinSoc,
		DailyMaxSoc:        capacity.DailyMaxSoc,
		ChargeLimitPercent: limit.Percent,
		ChargeLimitSource:  limit.Source,

		TotalOperatingTimeMin:         optInt(lifetime, "TotalOperatingTime"),
		LifetimeMaxTempC:              optInt(lifetime, "MaximumTemperature"),
		LifetimeMinTempC:              optInt(lifetime, "MinimumTemperature"),
		LifetimeAvgTempC:              optInt(lifetime, "AverageTemperature"),
		LifetimeMaxPackVoltageMV:      optInt(lifetime, "MaximumPackVoltage"),
		LifetimeMinPackVoltageMV:      optInt(lifetime, "MinimumPackVoltage"),
		LifetimeMaxChargeCurrentMA:    optInt(lifetime, "MaximumChargeCurrent"),
		LifetimeMaxDischargeCurrentMA: optInt(lifetime, "MaximumDischargeCurrent"),

		BatterySerial:              optString(props, "Serial"),
		DeviceName:                 optString(props, "DeviceName"),
		InstantAmperageMA:          optInt(props, "InstantAmperage"),
		AtCriticalLevel:            optBool(props, "AtCriticalLevel"),
		PermanentFailureStatus:     optInt(props, "PermanentFailureStatus"),
		BatteryCellDisconnectCount: optInt(props, "BatteryCellDisconnectCount"),

		AvgTimeToEmptyMinutes:  firstInt(optInt(batteryData, "AvgTimeToEmpty"), optInt(props, "AvgTimeToEmpty")),
		AvgTimeToFullMinutes:   firstInt(optInt(batteryData, "AvgTimeToFull"), optInt(props, "AvgTimeToFull")),
		RemainingCapacityMAH:   capacity.RemainingCapacityMAH,
		FullChargeCapacityMAH:  capacity.FullChargeCapacityMAH,
		BatteryManufactureDate: manufactureDate(props, now),
	}
	if capacity.TemperatureCentidegrees != nil {
		d.BatteryTemperatureC = ptr(InstantCelsius(*capacity.TemperatureCentidegrees))
	}
	if cellTelemetry != nil {
		layout := cellTelemetry.Layout
		d.CellVoltagesMV = cellTelemetry.VoltagesMV
		d.QmaxMAH = cellTelemetry.QmaxMAH
		d.BatteryCellLayout = &layout
		d.BatteryParallelCellCurrents = cellTelemetry.ParallelCellCurrents
	}

	var amperageW *float64
	if amperage != nil && voltage != nil && *amperage != 0 && *voltage > 0 {
		amperageW = ptr(float64(*amperage) * float64(*voltage) / 1e6)
	}

	telemetry := optDict(props, "PowerTelemetryData")
	if telemetry == nil {
		flow := ResolveFlow(onAC, 0, nil, nil, amperageW, nil)
		d.SystemPowerW, d.BatteryPowerW, d.DataSource = flow.SystemW, flow.BatteryW, SourceLegacy
		return d.normalized()
	}

	if count := derefOr(optInt(telemetry, "SystemLoadAccumulatorCount"), 0); count > 0 {
		d.AverageSystemPowerW = ptr(float64(derefOr(optInt(telemetry, "AccumulatedSystemLoad"), 0)) / float64(count) / 1000)
	}
	// SystemPowerIn goes stale after unplugging; ExternalConnected is authoritative.
	if onAC {
		d.ACInputW = float64(derefOr(optInt(telemetry, "SystemPowerIn"), 0)) / 1000
	}
	d.SystemVoltageMV, d.SystemCurrentMA = optInt(telemetry, "SystemVoltageIn"), optInt(telemetry, "SystemCurrentIn")
	var systemInputW *float64
	if d.SystemVoltageMV != nil && d.SystemCurrentMA != nil && *d.SystemVoltageMV > 0 && *d.SystemCurrentMA > 0 {
		systemInputW = ptr(float64(*d.SystemVoltageMV) * float64(*d.SystemCurrentMA) / 1e6)
	}
	if v := optInt(telemetry, "WallEnergyEstimate"); v != nil {
		d.WallPowerW = ptr(TelemetryEnergyToWatts(*v))
	}
	if v := optInt(telemetry, "AdapterEfficiencyLoss"); v != nil {
		d.AdapterEfficiencyLossW = ptr(TelemetryEnergyToWatts(*v))
	}
	flow := ResolveFlow(onAC, d.ACInputW,
		ptr(float64(derefOr(optInt(telemetry, "SystemLoad"), 0))/1000),
		ptr(float64(derefOr(optInt(telemetry, "BatteryPower"), 0))/1000),
		amperageW, systemInputW)
	d.SystemPowerW, d.BatteryPowerW, d.DataSource = flow.SystemW, flow.BatteryW, SourceTelemetry
	return d.normalized()
}

func derefBool(v *bool) bool { return v != nil && *v }
