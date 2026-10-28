package power

import (
	"fmt"
	"math"
	"strings"

	"powerfultop/internal/lang"
)

// Derived descriptions for the details window, kept out of the views so they can be tested.

func (d Data) CapacityFractionOfDesign(value *int) *float64 {
	if value == nil || d.DesignCapacityMAH == nil || *d.DesignCapacityMAH <= 0 {
		return nil
	}
	return ptr(float64(*value) / float64(*d.DesignCapacityMAH))
}

// BatteryCurrentText formats raw IOKit amperage: positive = charging, negative = discharging.
func BatteryCurrentText(milliamps int) string {
	direction := lang.L("Idle", "空闲")
	if milliamps > 0 {
		direction = lang.L("Charging", "充电")
	} else if milliamps < 0 {
		direction = lang.L("Discharging", "放电")
	}
	return fmt.Sprintf("%.2f A (%s)", math.Abs(float64(milliamps))/1000, direction)
}

// CellVoltageFraction places a cell in the Li-ion working window, only to draw comparable bars.
func CellVoltageFraction(millivolts int) float64 {
	return (float64(millivolts) - 3000) / (4400 - 3000)
}

func (d Data) HasCellData() bool {
	return len(d.CellDisplayEntries()) > 0 || len(d.BatteryParallelCellCurrents) > 0
}

func (d Data) UsesSeriesParallelLayout() bool {
	return d.BatteryCellLayout != nil && d.BatteryCellLayout.Kind == SeriesParallel
}

func (d Data) CellTopologyDescription() string {
	layout := d.BatteryCellLayout
	if layout == nil {
		return ""
	}
	if layout.Kind == SeriesParallel {
		if layout.ParallelCountKnown && layout.ParallelCount > 1 {
			return lang.Lf("Battery topology: %dS%dP (%d cells)", "电芯拓扑：%d 串 × %d 并（共 %d 节）",
				layout.SeriesCount, layout.ParallelCount, layout.SeriesCount*layout.ParallelCount)
		}
		return lang.Lf("Battery topology: %d series groups", "电芯拓扑：%d 个串联组", layout.SeriesCount)
	}
	if n := len(d.CellVoltagesMV); n > 0 {
		return lang.Lf("Battery topology: %d cells", "电芯拓扑：共 %d 节", n)
	}
	return ""
}

// CellEntry pairs a cell's voltage and Qmax; either may be missing.
type CellEntry struct {
	VoltageMV *int
	QmaxMAH   *int
}

func (d Data) CellDisplayEntries() []CellEntry {
	count := max(len(d.CellVoltagesMV), len(d.QmaxMAH))
	entries := make([]CellEntry, count)
	for i := range entries {
		if i < len(d.CellVoltagesMV) {
			entries[i].VoltageMV = &d.CellVoltagesMV[i]
		}
		if i < len(d.QmaxMAH) {
			entries[i].QmaxMAH = &d.QmaxMAH[i]
		}
	}
	return entries
}

func CellSummary(e CellEntry) string {
	var parts []string
	if e.VoltageMV != nil {
		parts = append(parts, fmt.Sprintf("%.3f V", float64(*e.VoltageMV)/1000))
	}
	if e.QmaxMAH != nil {
		parts = append(parts, fmt.Sprintf("%d mAh", *e.QmaxMAH))
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, " · ")
}

func ParallelCellLabel(bankID, cellID int) string {
	return lang.Lf("Series Group %d · Cell %d", "串联组 %d · 并联节 %d", bankID+1, cellID+1)
}

func (d Data) HasLifetimeData() bool {
	return d.TotalOperatingTimeMin != nil || d.LifetimeMaxTempC != nil || d.LifetimeMinTempC != nil ||
		d.LifetimeAvgTempC != nil || d.LifetimeMaxPackVoltageMV != nil || d.LifetimeMinPackVoltageMV != nil ||
		d.LifetimeMaxChargeCurrentMA != nil || d.LifetimeMaxDischargeCurrentMA != nil || d.BatteryCellDisconnectCount != nil
}

// ChargeLimitText reads "85% (macOS setting)", or "Off" when nothing caps charging.
func (d Data) ChargeLimitText() string {
	if d.ChargeLimitPercent >= 100 {
		return lang.L("Off", "关闭")
	}
	if source := ChargeLimitSourceLabel(d.ChargeLimitSource); source != "" {
		return fmt.Sprintf("%d%% (%s)", d.ChargeLimitPercent, source)
	}
	return fmt.Sprintf("%d%%", d.ChargeLimitPercent)
}

func SeriesVoltageBalanceSummary(voltagesMV []int) string {
	if len(voltagesMV) < 2 {
		return ""
	}
	lo, hi := voltagesMV[0], voltagesMV[0]
	for _, v := range voltagesMV {
		lo, hi = min(lo, v), max(hi, v)
	}
	delta := hi - lo
	return lang.Lf("%d mV spread — %s", "压差 %d mV — %s", delta, balanceStatus(delta, 50, 100))
}

func ParallelCurrentBalanceSummary(currents []ParallelCellCurrent) string {
	byBank := map[int][]int{}
	for _, c := range currents {
		byBank[c.BankID] = append(byBank[c.BankID], c.CurrentMA)
	}
	worst := -1
	for _, values := range byBank {
		if len(values) < 2 {
			continue
		}
		lo, hi := values[0], values[0]
		for _, v := range values {
			lo, hi = min(lo, v), max(hi, v)
		}
		worst = max(worst, hi-lo)
	}
	if worst < 0 {
		return ""
	}
	return lang.Lf("%d mA max within-group spread — %s", "组内最大电流差 %d mA — %s", worst, balanceStatus(worst, 80, 150))
}

func balanceStatus(delta, normal, fair int) string {
	switch {
	case delta <= normal:
		return lang.L("Normal", "正常")
	case delta <= fair:
		return lang.L("Fair", "一般")
	}
	return lang.L("Worth Checking", "建议关注")
}

func ProtectionTriggerLabel(count int) string {
	if count == 0 {
		return lang.L("0 (normal)", "0（正常）")
	}
	return fmt.Sprint(count)
}

// LifetimeAvgCelsius converts AverageTemperature, which IOKit reports in decidegrees.
func LifetimeAvgCelsius(decidegrees int) float64 { return float64(decidegrees) / 10 }

// InstantCelsius converts the pack's Temperature, which IOKit reports in centidegrees.
func InstantCelsius(centidegrees int) float64 { return float64(centidegrees) / 100 }
