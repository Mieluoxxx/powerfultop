package view

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/history"
	"powerfultop/internal/power"
	"powerfultop/internal/system"
	"powerfultop/internal/thermal"
)

// OverviewData is what the overview page shows.
type OverviewData struct {
	Power   power.Data
	Thermal thermal.Snapshot
	System  system.Snapshot
	History *history.History
}

// OverviewPage is the glanceable dashboard: what the popover shows, plus specs and the last 30
// minutes as charts.
func OverviewPage(c *ui.Context, d OverviewData, width float32) {
	p := d.Power
	ui.Column(c).Gap(16).Children(func() {
		PowerStrip(c, p, d.Thermal.DisplayPowerW, width)

		ui.Row(c).Gap(16).AlignItems(ui.Stretch).Children(func() {
			spec := func(key string, fn func()) {
				ui.Column(c.Key(key)).Grow(1).Width(0).Children(fn)
			}
			spec("battery", func() { overviewBatteryCard(c, p, d.Thermal) })
			spec("health", func() { overviewHealthCard(c, p) })
			spec("adapter", func() { overviewAdapterCard(c, p) })
		})

		collecting := L("Collecting data…", "正在收集数据…")
		h := d.History
		series := func(m history.Metric) []Point {
			if h == nil {
				return nil
			}
			return h.Series(m)
		}
		var panels []func()
		panels = append(panels, func() {
			detail := ""
			if t := p.EstimatedTimeRemainingText(); t != "" {
				detail = p.EstimatedTimeRemainingLabel() + " " + t
			}
			ChartPanel(c, L("Battery Level", "电量"), fmt.Sprintf("%d%%", p.DisplayBatteryPercent()), detail,
				BatteryLevelIcon(p.DisplayBatteryPercent()), Green, series(history.BatteryPercent), collecting)
		}, func() {
			detail := ""
			if avg := p.AverageSystemPowerW; avg != nil {
				detail = Lf("Average System Power: %s", "平均系统功耗：%s", fmt.Sprintf("%.1f W", *avg))
			}
			ChartPanel(c, L("System Power", "系统功耗"), fmt.Sprintf("%.1f W", p.SystemPowerW), detail,
				"zap", Purple, series(history.SystemPower), collecting)
		}, func() {
			detail := ""
			if len(d.System.LoadAverage) > 0 {
				detail = Lf("Load %.2f", "负载 %.2f", d.System.LoadAverage[0])
			}
			ChartPanel(c, L("CPU Usage", "CPU 占用"), fmt.Sprintf("%.0f%%", d.System.CPU.Usage()*100), detail,
				"cpu", Blue, series(history.CPUUsage), collecting)
		})
		if m := d.System.Memory; m != nil {
			panels = append(panels, func() {
				ChartPanel(c, L("Memory", "内存"), fmt.Sprintf("%.0f%%", m.UsedFraction()*100),
					system.FormatBytes(float64(m.UsedBytes()))+" / "+system.FormatBytes(float64(m.TotalBytes)),
					"memory-stick", Green, series(history.MemoryUsage), collecting)
			})
		}
		if cpu, ok := d.Thermal.Temperatures[thermal.GroupCPU]; ok {
			panels = append(panels, func() {
				ChartPanel(c, L("CPU Average", "CPU 平均温度"), fmt.Sprintf("%.1f°C", cpu.AverageC),
					Lf("Hottest core %s", "最热核心 %s", fmt.Sprintf("%.0f°C", cpu.MaximumC)),
					"cpu", Orange, series(history.CPUTemperature), collecting)
			})
		}
		if d.Thermal.HasFans() {
			panels = append(panels, func() {
				detail := L("Controlled by macOS", "由系统控制")
				if d.Thermal.Controlling {
					detail = L("Controlled by PowerfulTop", "由 PowerfulTop 控制")
				}
				maxRPM := 1.0
				for _, f := range d.Thermal.Fans {
					maxRPM = max(maxRPM, f.MaxRPM)
				}
				ChartPanel(c, L("Fan Speed", "风扇转速"), fmt.Sprintf("%.0f RPM", d.Thermal.MaxActualRPM()), detail,
					"fan", Teal, series(history.FanRPM), collecting)
			})
		} else if temp, ok := BatteryTemperatureC(p, d.Thermal); ok {
			panels = append(panels, func() {
				ChartPanel(c, L("Battery Temp", "电池温度"), fmt.Sprintf("%.1f°C", temp), "",
					"thermometer", Blue, series(history.BatteryTemp), collecting)
			})
		}
		for i := 0; i < len(panels); i += 2 {
			ui.Row(c.Key(i)).Gap(16).Children(func() {
				for j := i; j < min(i+2, len(panels)); j++ {
					ui.Column(c.Key(j)).Grow(1).Width(0).Children(panels[j])
				}
				if i+1 >= len(panels) {
					ui.Box(c).Grow(1).Width(0)
				}
			})
		}
	})
}

func overviewBatteryCard(c *ui.Context, p power.Data, th thermal.Snapshot) {
	Card(c, L("Battery", "电池"), "zap", Yellow, func() {
		if a := p.BatteryAmperageMA; a != nil {
			Row(c, L("Battery Current", "电池电流"), power.BatteryCurrentText(*a), "", false)
		}
		if v := p.BatteryVoltageMV; v != nil {
			Row(c, L("Battery Voltage", "电池电压"), fmt.Sprintf("%.2f V", float64(*v)/1000), "", false)
		}
		Row(c, L("Battery Power", "电池功率"), SignedBatteryPowerText(p), "", false)
		if r := p.RemainingCapacityMAH; r != nil {
			Row(c, L("Remaining Charge", "剩余电量"), fmt.Sprintf("%d mAh", *r), "", false)
		}
		if t, ok := BatteryTemperatureC(p, th); ok {
			Row(c, L("Battery Temp", "电池温度"), fmt.Sprintf("%.1f°C", t), "", false)
		}
	})
}

func overviewHealthCard(c *ui.Context, p power.Data) {
	Card(c, L("Battery Health", "电池健康"), "heart", BatteryHealthTint(p), func() {
		if design := p.DesignCapacityMAH; design != nil {
			Row(c, L("Design Capacity", "设计容量"), fmt.Sprintf("%d mAh", *design), "100%", false)
		}
		full := p.FullChargeCapacityMAH
		if full == nil {
			full = p.RawMaxCapacityMAH
		}
		if full != nil {
			rated := ""
			if f := p.CapacityFractionOfDesign(full); f != nil {
				rated = fmt.Sprintf("%.0f%%", *f*100)
			}
			Row(c, L("Current Full Charge", "当前满充电量"), fmt.Sprintf("%d mAh", *full), rated, false)
		}
		if h := p.BatteryHealthPercent; h != nil {
			Row(c, L("Health", "健康度"), fmt.Sprintf("%d%%", *h), "", true)
		}
		if cycles := p.CycleCount; cycles != nil {
			rated := ""
			if d := p.DesignCycleCount; d != nil {
				rated = fmt.Sprint(*d)
			}
			Row(c, L("Cycle Count", "循环次数"), fmt.Sprint(*cycles), rated, false)
		}
		Row(c, L("Charge Limit", "充电上限"), p.ChargeLimitText(), "", false)
	})
}

func overviewAdapterCard(c *ui.Context, p power.Data) {
	Card(c, L("Power Adapter", "电源适配器"), "plug-zap", Green, func() {
		if !p.HasSettledAC() {
			text := L("Not connected to power", "未连接电源")
			if p.IsConnectingAC() {
				text = L("AC Connecting", "AC 连接中")
			}
			Status(c, "plug", Gray, text, "")
			return
		}
		if sc := p.SystemCurrentMA; sc != nil {
			rated := ""
			if r := p.AdapterRatedCurrentMA; r != nil {
				rated = fmt.Sprintf("%.1f A", float64(*r)/1000)
			}
			Row(c, L("Current", "电流"), fmt.Sprintf("%.2f A", float64(*sc)/1000), rated, false)
		}
		if sv := p.SystemVoltageMV; sv != nil {
			rated := ""
			if r := p.AdapterRatedVoltageMV; r != nil {
				rated = fmt.Sprintf("%.0f V", float64(*r)/1000)
			}
			Row(c, L("Voltage", "电压"), fmt.Sprintf("%.2f V", float64(*sv)/1000), rated, false)
		}
		rated := ""
		if p.ACAdapterWattage > 0 {
			rated = fmt.Sprintf("%d W", p.ACAdapterWattage)
		}
		Row(c, L("Power", "功率"), fmt.Sprintf("%.1f W", p.EffectiveACOutputW()), rated, true)
		if wall := p.WallPowerW; wall != nil {
			Row(c, L("Wall Outlet Power", "插座取电（估算）"), fmt.Sprintf("%.1f W", *wall), "", false)
		}
	})
}

// PowerStrip is the top summary: level, flow and time remaining in one line, falling back to icons
// with values when the full labels don't fit.
func PowerStrip(c *ui.Context, p power.Data, displayPowerW *float64, width float32) {
	compact := width < 900
	batteryIcon, batteryTint := BatteryLevelIcon(p.DisplayBatteryPercent()), Gray
	switch p.PowerState().Kind {
	case power.StateCharging:
		batteryIcon, batteryTint = "battery-charging", Blue
	case power.StateOnBattery, power.StateSupplementing:
		batteryTint = Orange
	}
	watts := func(w float64) string { return fmt.Sprintf("%.1f W", w) }
	Surface(c, ui.Row(c).PaddingX(16).PaddingY(12).AlignItems(ui.Center), 14).Children(func() {
		StripItem(c, BatteryLevelIcon(p.DisplayBatteryPercent()), BatteryLevelTint(p), fmt.Sprintf("%d%%", p.DisplayBatteryPercent()), p.PowerSourceDescription(), false, "")
		StripDivider(c)
		if p.HasSettledAC() {
			StripItem(c, "plug-zap", Green, L("Charger", "充电器"), watts(p.EffectiveACOutputW()), compact, "")
			StripDivider(c)
		}
		StripItem(c, "laptop", Gray, L("System", "系统"), watts(p.SystemPowerW), compact, "")
		StripDivider(c)
		if displayPowerW != nil {
			StripItem(c, "sun", Yellow, L("Screen", "屏幕"), watts(*displayPowerW), compact, L("Built-in display, included in system power", "内建屏幕功耗，已包含在系统功耗中"))
			StripDivider(c)
		}
		StripItem(c, batteryIcon, batteryTint, L("Battery", "电池"), SignedBatteryPowerText(p), compact, "")
		if t := p.EstimatedTimeRemainingText(); t != "" {
			StripDivider(c)
			StripItem(c, "clock", Gray, p.EstimatedTimeRemainingLabel(), t, compact, p.EstimatedTimeFootnote())
		}
		if p.DataSource == power.SourceLegacy {
			StripDivider(c)
			StripItem(c, "triangle-alert", Yellow, L("Estimation Mode", "估算模式"), "", compact, L("Estimation Mode — telemetry unavailable", "估算模式 — 遥测数据不可用"))
		}
	})
}
