package view

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/power"
)

// BatteryPage is the depth the overview leaves out: capacity history, the charge limit, charger
// negotiation, cells and lifetime statistics.
func BatteryPage(c *ui.Context, d power.Data, limit *power.ChargeLimiter, width float32) {
	columns := 1
	if width >= 760 {
		columns = 2
	}
	cards := []MasonryItem{
		{ID: "health", Weight: 10, View: func() { batteryHealthCard(c, d) }},
		{ID: "chargeLimit", Weight: 6, View: func() { ChargeLimitCard(c, d, limit) }},
	}
	if d.HasSettledAC() {
		cards = append(cards, MasonryItem{ID: "charger", Weight: 11, View: func() { batteryChargerCard(c, d) }})
	}
	if d.HasCellData() || d.BatteryVoltageMV != nil {
		rows := len(d.CellDisplayEntries())*2 + len(d.BatteryParallelCellCurrents) + 6
		cards = append(cards, MasonryItem{ID: "cells", Weight: float64(rows), View: func() { batteryCellsCard(c, d) }})
	}
	if d.HasLifetimeData() {
		cards = append(cards, MasonryItem{ID: "lifetime", Weight: 9, View: func() { batteryLifetimeCard(c, d) }})
	}
	Masonry(c, cards, columns, 16)
}

func batteryHealthCard(c *ui.Context, d power.Data) {
	health := BatteryHealthTint(d)
	Card(c, L("Battery Health", "电池健康"), "heart", health, func() {
		if d.BatteryHealthPercent != nil {
			Hero(c, fmt.Sprintf("%d%%", *d.BatteryHealthPercent), "", L("Health", "健康度"), 0)
		}
		capacityRow(c, d, L("Design Capacity", "设计容量"), d.DesignCapacityMAH, Gray)
		full := d.FullChargeCapacityMAH
		if full == nil {
			full = d.RawMaxCapacityMAH
		}
		capacityRow(c, d, L("Current Full Charge", "当前满充电量"), full, health)
		capacityRow(c, d, L("Calibrated Full Charge", "系统标定满电量"), d.NominalChargeCapacityMAH, health)
		capacityRow(c, d, L("Remaining Charge", "剩余电量"), d.RemainingCapacityMAH, BatteryLevelTint(d))
		if d.DailyMinSoc != nil && d.DailyMaxSoc != nil {
			Row(c, L("Optimized Charging Range", "优化充电保持范围"), fmt.Sprintf("%d%% – %d%%", *d.DailyMinSoc, *d.DailyMaxSoc), "", false)
		}
		if d.CycleCount != nil {
			rated := ""
			if d.DesignCycleCount != nil {
				rated = fmt.Sprint(*d.DesignCycleCount)
			}
			Row(c, L("Cycle Count", "循环次数"), fmt.Sprint(*d.CycleCount), rated, false)
		}
		if d.PermanentFailureStatus != nil {
			value := L("Normal", "正常")
			if *d.PermanentFailureStatus != 0 {
				value = Lf("Abnormal (%d)", "异常 (%d)", *d.PermanentFailureStatus)
			}
			Row(c, L("Permanent Failure Status", "永久故障状态"), value, "", false)
		}
		if d.AtCriticalLevel != nil {
			Row(c, L("Critical Low Battery", "临界低电量"), yesNo(*d.AtCriticalLevel), "", false)
		}
	})
}

// ChargeLimitCard edits the macOS charge limit itself, so System Settings and PowerfulTop always agree.
func ChargeLimitCard(c *ui.Context, d power.Data, limit *power.ChargeLimiter) {
	s := limit.State()
	Card(c, L("Charge Limit", "充电上限"), "battery-charging", Green, func() {
		if !s.Supported {
			Row(c, L("Charge Limit", "充电上限"), d.ChargeLimitText(), "", true)
			if s.Loaded {
				Note(c, L("This Mac has no built-in charge limit; it needs macOS 26.4 or later on Apple silicon.",
					"此 Mac 不支持系统充电上限，需要 Apple 芯片并运行 macOS 26.4 或更高版本。"))
			}
			if s.Err != nil {
				Status(c, "octagon-x", Red, L("Could not read the charge limit", "无法读取充电上限"), s.Err.Error())
			}
			return
		}
		caption := Lf("Charging stops at %d%%", "充到 %d%% 停止", s.Percent)
		value := fmt.Sprintf("%d%%", s.Percent)
		if s.Percent >= 100 {
			caption = L("Charges to 100%", "不限制，充满至 100%")
			value = L("Off", "关闭")
		}
		ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
			ui.Column(c).Grow(1).Children(func() { Hero(c, value, "", caption, 0) })
			if s.Pending {
				ui.Spinner(c)
			}
		})
		selected := slices.Index(s.Choices, s.Percent)
		segments := make([]segment, len(s.Choices))
		for i, choice := range s.Choices {
			segments[i] = segment{label: fmt.Sprintf("%d%%", choice)}
		}
		if e := segmented(c, L("Charge Limit", "充电上限"), &selected, segments, true, s.Pending); !s.Pending && e.Changed() {
			limit.Set(s.Choices[selected])
		}
		switch {
		case s.Err != nil:
			Status(c, "octagon-x", Red, L("Could not change the charge limit", "无法修改充电上限"), s.Err.Error())
		case s.AlDenteRunning:
			Status(c, "triangle-alert", Orange, L("AlDente is running and may override this limit", "AlDente 正在运行，可能会改写这里的上限"),
				L("Quit AlDente, or turn off its charge limit, so it doesn't fight over charging.",
					"请退出 AlDente 或关闭它的充电限制，避免两边争抢充电控制。"))
		}
		Note(c, L("This is the charge limit in System Settings › Battery. macOS enforces it in firmware, so it keeps working while PowerfulTop is closed or the Mac sleeps.",
			"即系统设置 › 电池中的充电上限，由 macOS 在固件层执行，PowerfulTop 关闭或 Mac 睡眠时依然有效。"))
	})
}

func batteryChargerCard(c *ui.Context, d power.Data) {
	Card(c, L("Charger & Charging", "充电器与充电"), "plug", Green, func() {
		rated := ""
		if d.ACAdapterWattage > 0 {
			rated = fmt.Sprintf("%d W", d.ACAdapterWattage)
		}
		Hero(c, watts(d.EffectiveACOutputW()), rated, L("AC Adapter Output", "AC 适配器输出"), 0)
		if d.SystemCurrentMA != nil {
			rated := ""
			if d.AdapterRatedCurrentMA != nil {
				rated = fmt.Sprintf("%.1f A", float64(*d.AdapterRatedCurrentMA)/1000)
			}
			Row(c, L("Charger Output Current", "充电器输出电流"), fmt.Sprintf("%.2f A", float64(*d.SystemCurrentMA)/1000), rated, false)
		}
		if d.SystemVoltageMV != nil {
			rated := ""
			if d.AdapterRatedVoltageMV != nil {
				rated = fmt.Sprintf("%.0f V", float64(*d.AdapterRatedVoltageMV)/1000)
			}
			Row(c, L("Charger Output Voltage", "充电器输出电压"), fmt.Sprintf("%.2f V", float64(*d.SystemVoltageMV)/1000), rated, false)
		}
		if d.ACAdapterWattage > 0 {
			usage := d.EffectiveACOutputW() / float64(d.ACAdapterWattage)
			tint := Green
			if usage > 0.9 {
				tint = Orange
			}
			BarRow(c, L("Charger Load Rate", "充电器负载率"), fmt.Sprintf("%.0f%%", usage*100), &usage, tint)
		}
		if d.WallPowerW != nil {
			Row(c, L("Wall Outlet Power", "插座取电（估算）"), watts(*d.WallPowerW), "", false)
		}
		if d.AdapterEfficiencyLossW != nil {
			Row(c, L("Adapter Loss", "充电器转换损耗"), watts(*d.AdapterEfficiencyLossW), "", false)
		}
		ui.Column(c).PaddingY(2).Children(func() { ui.Divider(c) })
		status := L("Not Charging", "未在充电")
		if d.IsBatteryCharging() {
			status = L("Charging", "充电")
		}
		Row(c, L("Status", "状态"), status, "", true)
		if d.FullyCharged {
			Row(c, L("Fully Charged", "已充满"), L("Yes", "是"), "", false)
		}
		if reason := d.NotChargingReasonDescription(); reason != "" {
			Row(c, L("Not Charging Reason", "未充电原因"), reason, "", false)
		}
		if d.ChargingVoltageMV != nil {
			Row(c, L("Cell Charging Voltage", "电芯充电电压"), fmt.Sprintf("%.3f V", float64(*d.ChargingVoltageMV)/1000), "", false)
		}
		if d.ChargingCurrentMA != nil {
			Row(c, L("Charging Current", "充电电流"), fmt.Sprintf("%d mA", *d.ChargingCurrentMA), "", false)
		}
		if d.VacVoltageLimit != nil {
			Row(c, L("Max Charging Voltage Limit", "最大充电电压限制"), fmt.Sprintf("%.3f V", float64(*d.VacVoltageLimit)/1000), "", false)
		}
		if d.AdapterDescription != nil {
			Note(c, *d.AdapterDescription)
		}
	})
}

func batteryCellsCard(c *ui.Context, d power.Data) {
	Card(c, L("Cells & Live Readings", "电芯与实时读数"), "layers", Cyan, func() {
		if d.BatteryVoltageMV != nil {
			Row(c, L("Battery Voltage", "电池电压"), fmt.Sprintf("%.2f V", float64(*d.BatteryVoltageMV)/1000), "", false)
		}
		if d.BatteryAmperageMA != nil {
			Row(c, L("Battery Current", "电池电流"), power.BatteryCurrentText(*d.BatteryAmperageMA), "", false)
		}
		if d.InstantAmperageMA != nil {
			Row(c, L("Battery Instant Current", "电池瞬时电流"), power.BatteryCurrentText(*d.InstantAmperageMA), "", false)
		}
		if topology := d.CellTopologyDescription(); topology != "" {
			Note(c, topology)
		}
		if balance := power.SeriesVoltageBalanceSummary(d.CellVoltagesMV); balance != "" {
			Row(c, L("Series Voltage Balance", "串联组电压均衡"), balance, "", false)
		}
		for i, entry := range d.CellDisplayEntries() {
			label := Lf("Cell %d", "电芯 %d", i+1)
			if d.UsesSeriesParallelLayout() {
				label = Lf("Series Group %d", "串联组 %d", i+1)
			}
			var fraction *float64
			if entry.VoltageMV != nil {
				f := power.CellVoltageFraction(*entry.VoltageMV)
				fraction = &f
			}
			ui.Column(c.Key(fmt.Sprint("cell", i))).Children(func() {
				BarRow(c, label, power.CellSummary(entry), fraction, Cyan)
			})
		}
		if d.UsesSeriesParallelLayout() && len(d.BatteryParallelCellCurrents) > 0 {
			if balance := power.ParallelCurrentBalanceSummary(d.BatteryParallelCellCurrents); balance != "" {
				Row(c, L("Parallel Current Balance", "并联节电流均衡"), balance, "", false)
			}
			for _, cell := range d.BatteryParallelCellCurrents {
				ui.Column(c.Key(fmt.Sprint("parallel", cell.BankID, ".", cell.CellID))).Children(func() {
					Row(c, power.ParallelCellLabel(cell.BankID, cell.CellID), fmt.Sprintf("%d mA", cell.CurrentMA), "", false)
				})
			}
			Note(c, L("Instantaneous current per parallel cell; unequal split within a group is normal.",
				"各并联节的瞬时电流；同组内两节电流不均是正常现象。"))
		}
	})
}

func batteryLifetimeCard(c *ui.Context, d power.Data) {
	Card(c, L("Lifetime Statistics", "生命周期统计"), "history", Purple, func() {
		if d.TotalOperatingTimeMin != nil {
			Row(c, L("Total Operating Time", "总运行时间"), fmt.Sprintf("%d %s", *d.TotalOperatingTimeMin/60, L("hours", "小时")), "", false)
		}
		if d.LifetimeMaxTempC != nil {
			Row(c, L("Battery Max Temperature", "电池历史最高温度"), fmt.Sprintf("%d °C", *d.LifetimeMaxTempC), "", false)
		}
		if d.LifetimeMinTempC != nil {
			Row(c, L("Battery Min Temperature", "电池历史最低温度"), fmt.Sprintf("%d °C", *d.LifetimeMinTempC), "", false)
		}
		if d.LifetimeAvgTempC != nil {
			Row(c, L("Battery Avg Temperature", "电池历史平均温度"), fmt.Sprintf("%.1f °C", power.LifetimeAvgCelsius(*d.LifetimeAvgTempC)), "", false)
		}
		if d.LifetimeMaxPackVoltageMV != nil {
			Row(c, L("Max Pack Voltage", "历史最高电池组电压"), fmt.Sprintf("%.3f V", float64(*d.LifetimeMaxPackVoltageMV)/1000), "", false)
		}
		if d.LifetimeMinPackVoltageMV != nil {
			Row(c, L("Min Pack Voltage", "历史最低电池组电压"), fmt.Sprintf("%.3f V", float64(*d.LifetimeMinPackVoltageMV)/1000), "", false)
		}
		// Gauges report 0xFFFF-style garbage here on some packs.
		if v := d.LifetimeMaxChargeCurrentMA; v != nil && *v < 100_000 {
			Row(c, L("Max Charging Current", "历史最大充电电流"), fmt.Sprintf("%d mA", *v), "", false)
		}
		if v := d.LifetimeMaxDischargeCurrentMA; v != nil && *v < 100_000 {
			Row(c, L("Max Discharging Current", "历史最大放电电流"), fmt.Sprintf("%d mA", *v), "", false)
		}
		if d.BatteryCellDisconnectCount != nil {
			Row(c, L("Protection Trigger Count", "保护触发次数"), power.ProtectionTriggerLabel(*d.BatteryCellDisconnectCount), "", false)
		}
	})
}

func capacityRow(c *ui.Context, d power.Data, label string, value *int, tint Tint) {
	if value == nil {
		return
	}
	fraction := d.CapacityFractionOfDesign(value)
	text := fmt.Sprintf("%d mAh", *value)
	if fraction != nil {
		text = fmt.Sprintf("%d mAh · %.0f%%", *value, *fraction*100)
	}
	ui.Column(c.Key(label)).Children(func() {
		BarRow(c, label, text, fraction, tint)
	})
}

func yesNo(v bool) string {
	if v {
		return L("Yes", "是")
	}
	return L("No", "否")
}

func watts(w float64) string { return fmt.Sprintf("%.1f W", w) }
