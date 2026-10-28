package view

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/ELY-Open-Source/CrychicUI/button"
	"github.com/ELY-Open-Source/CrychicUI/choice"
	"github.com/ELY-Open-Source/CrychicUI/feedback"
	lucide "github.com/ELY-Open-Source/CrychicUI/icon"
	"github.com/ELY-Open-Source/CrychicUI/layout"
	"powerfultop/internal/display"
	"powerfultop/internal/power"
	"powerfultop/internal/system"
	"powerfultop/internal/thermal"
)

// PopoverWidth matches the Swift popover.
const PopoverWidth = 300

// PopoverData is what the menu bar popover shows.
type PopoverData struct {
	Power     power.Data
	Available bool
	Thermal   thermal.Snapshot
	// Fans is the live monitor the fan control drives; nil hides the fan section.
	Fans     *thermal.Monitor
	System   system.Snapshot
	Displays []display.Brightness
	// ChargeLimit edits the macOS charge limit in place; nil leaves a row that opens the Battery page.
	ChargeLimit *power.ChargeLimiter

	LaunchAtLogin      bool
	LaunchAtLoginError string
}

// PopoverActions are what the popover's controls do.
type PopoverActions struct {
	OpenDetails      func(PageID)
	SetBrightness    func(id uint32, level float64)
	SetLaunchAtLogin func(bool)
	Quit             func()
}

// BatteryTemperatureC prefers the battery's own reading; some machines drop it, and the SMC pack
// thermistors still report one.
func BatteryTemperatureC(d power.Data, t thermal.Snapshot) (float64, bool) {
	if d.BatteryTemperatureC != nil {
		return *d.BatteryTemperatureC, true
	}
	if s, ok := t.Temperatures[thermal.GroupBattery]; ok {
		return s.AverageC, true
	}
	return 0, false
}

// Popover is the menu bar popover, ported section by section from the Swift PopoverView.
func Popover(c *ui.Context, d PopoverData, a PopoverActions) ui.Element {
	return ui.Column(c).Width(PopoverWidth).Children(func() {
		if !d.Available {
			ui.Row(c).Gap(6).Padding(10, 14).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, lucide.Must("triangle-alert")).Size(13, 13).TextColor(Orange.Color(c))
				ui.Text(c, L("Built-in battery not detected. PowerfulTop requires a MacBook.", "未检测到内置电池。PowerfulTop 仅支持 MacBook。")).
					FontSize(11).TextColor(c.Theme().TextMuted)
			})
		}
		section := func(top, bottom float32, fn func()) {
			ui.Column(c).Padding(top, 14, bottom, 14).Children(fn)
		}
		divider := func() { ui.Column(c).PaddingX(12).Children(func() { layout.Divider(c, layout.DividerOptions{}) }) }

		section(14, 12, func() { popoverHeader(c, d.Power, a) })
		divider()
		section(12, 12, func() { popoverFlow(c, d.Power) })
		divider()
		section(10, 10, func() { popoverSystem(c, d.System, d.Thermal) })
		if len(d.Displays) > 0 {
			divider()
			section(10, 10, func() {
				BrightnessSection(c, d.Displays, d.Thermal.DisplayPowerW, a.SetBrightness)
			})
		}
		if d.Fans != nil && d.Thermal.HasFans() {
			divider()
			section(10, 10, func() { FanSection(c, d.Fans) })
		}
		temp, hasTemp := BatteryTemperatureC(d.Power, d.Thermal)
		if d.Power.BatteryHealthPercent != nil || hasTemp || d.Power.CycleCount != nil {
			divider()
			section(10, 0, func() { popoverStats(c, d.Power, temp, hasTemp) })
			section(8, 10, func() { popoverChargeLimit(c, d.Power, d.ChargeLimit, a) })
		}
		divider()
		ui.Column(c).Padding(6, 10).Children(func() { popoverFooter(c, d, a) })
	})
}

func popoverHeader(c *ui.Context, p power.Data, a PopoverActions) {
	t := c.Theme()
	state := p.PowerState()
	icon, tint := BatteryLevelIcon(p.DisplayBatteryPercent()), Gray
	switch state.Kind {
	case power.StateOnBattery:
		if p.DisplayBatteryPercent() <= 20 {
			tint = Red
		}
	case power.StateConnectingAC:
		icon = "plug"
	case power.StateACIdle:
		icon, tint = "plug-zap", Green
	case power.StateCharging:
		icon, tint = "zap", Green
	case power.StateSupplementing:
		icon, tint = "triangle-alert", Orange
	}
	status := p.PowerSourceDescription()
	if remaining := p.EstimatedTimeRemainingText(); remaining != "" {
		status += " · " + p.EstimatedTimeRemainingLabel() + " " + remaining
	}
	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).AlignItems(ui.Start).Children(func() {
			ui.Textf(c, "%d%%", p.DisplayBatteryPercent()).FontSize(30).Bold().FontFeatures("tnum").Grow(1)
			if button.Ghost(c, L("Details", "详细参数")).Clicked() && a.OpenDetails != nil {
				a.OpenDetails(PageOverview)
			}
		})
		LevelBar(c, float64(p.DisplayBatteryPercent())/100, BatteryLevelTint(p), 6)
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, lucide.Must(icon)).Size(12, 12).TextColor(tint.Color(c)).Shrink(0)
			ui.Text(c, status).FontSize(12).TextColor(t.TextMuted)
		})
		if reason := p.NotChargingReasonDescription(); reason != "" && (state.Kind == power.StateACIdle || p.IsSupplementalDischarge()) {
			ui.Text(c, reason).FontSize(11).TextColor(t.TextMuted.Alpha(0.75))
		}
	})
}

type flowNode struct {
	icon, title, value, capacity string
	tint                         Tint
}

func popoverFlow(c *ui.Context, p power.Data) {
	watts := func(w float64) string { return fmt.Sprintf("%.1f W", w) }
	charger := flowNode{icon: "plug-zap", title: L("Charger", "充电器"), value: watts(p.EffectiveACOutputW()), tint: Green}
	if p.ACAdapterWattage > 0 {
		charger.capacity = fmt.Sprintf("%d W", p.ACAdapterWattage)
	}
	systemNode := flowNode{icon: "laptop", title: L("System", "系统"), value: watts(p.SystemPowerW), tint: Gray}
	battery := L("Battery", "电池")
	var sources, destinations []flowNode
	state := p.PowerState()
	switch state.Kind {
	case power.StateOnBattery:
		sources = []flowNode{{icon: BatteryLevelIcon(p.DisplayBatteryPercent()), title: battery, tint: Orange}}
		destinations = []flowNode{systemNode}
	case power.StateConnectingAC:
		sources = []flowNode{{icon: "plug", title: L("AC Connecting", "AC 连接中"), tint: Gray}}
		destinations = []flowNode{systemNode}
	case power.StateACIdle:
		sources, destinations = []flowNode{charger}, []flowNode{systemNode}
	case power.StateCharging:
		sources = []flowNode{charger}
		destinations = []flowNode{systemNode, {icon: "battery-charging", title: battery, value: watts(state.RateW), tint: Blue}}
	case power.StateSupplementing:
		sources = []flowNode{charger, {icon: "battery-medium", title: battery, value: watts(state.RateW), tint: Orange}}
		destinations = []flowNode{systemNode}
	}
	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			column := func(nodes []flowNode) {
				ui.Column(c).Gap(6).Grow(1).Width(0).Children(func() {
					for _, n := range nodes {
						popoverFlowNode(c, n)
					}
				})
			}
			column(sources)
			ui.Icon(c, lucide.Must("arrow-right")).Size(11, 11).TextColor(c.Theme().TextMuted.Alpha(0.6))
			column(destinations)
		})
		if p.DataSource == power.SourceLegacy {
			Status(c, "triangle-alert", Yellow, L("Estimation Mode", "估算模式"), "")
		}
	})
}

func popoverFlowNode(c *ui.Context, n flowNode) {
	t := c.Theme()
	col := n.tint.Color(c)
	if n.tint == Gray {
		col = t.Text
	}
	// Value-less nodes (battery-only source) must line up with their valued counterpart.
	ui.Row(c.Key(n.icon+n.title)).Gap(6).AlignItems(ui.Center).Padding(7, 8).MinHeight(46).Radius(8).Background(col.Alpha(0.1)).Children(func() {
		ui.Icon(c, lucide.Must(n.icon)).Size(14, 14).TextColor(col).Shrink(0)
		ui.Column(c).Gap(1).Grow(1).Children(func() {
			ui.Row(c).Gap(4).Children(func() {
				ui.Text(c, n.title).FontSize(10).TextColor(t.TextMuted).NoWrap().Grow(1)
				if n.capacity != "" {
					ui.Text(c, n.capacity).FontSize(10).TextColor(t.TextMuted.Alpha(0.7)).FontFeatures("tnum").NoWrap()
				}
			})
			if n.value != "" {
				ui.Text(c, n.value).FontSize(14).FontWeight(600).FontFeatures("tnum").NoWrap()
			}
		})
	})
}

func popoverSystem(c *ui.Context, s system.Snapshot, th thermal.Snapshot) {
	ui.Row(c).Gap(14).Children(func() {
		cpuDetail := ""
		if cpu, ok := th.Temperatures[thermal.GroupCPU]; ok {
			cpuDetail = fmt.Sprintf("%.0f°C", cpu.AverageC)
		}
		usageMeter(c, "cpu", L("CPU", "CPU"), s.CPU.Usage(), Blue, cpuDetail)
		memFraction, memTint, memDetail := 0.0, Green, ""
		if m := s.Memory; m != nil {
			memFraction = m.UsedFraction()
			if m.Pressure != system.PressureNormal {
				memTint = Orange
			}
			memDetail = system.FormatBytes(float64(m.UsedBytes())) + " / " + system.FormatBytes(float64(m.TotalBytes))
		}
		usageMeter(c, "memory-stick", L("Memory", "内存"), memFraction, memTint, memDetail)
	})
}

// usageMeter is the popover's compact usage bar: icon, title, percent and a secondary reading.
func usageMeter(c *ui.Context, icon, title string, fraction float64, tint Tint, detail string) {
	t := c.Theme()
	ui.Column(c.Key(icon)).Gap(5).Grow(1).Width(0).Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, lucide.Must(icon)).Size(12, 12).TextColor(tint.Color(c))
			ui.Text(c, title).FontSize(12).TextColor(t.TextMuted).Grow(1).NoWrap()
			ui.Textf(c, "%.0f%%", fraction*100).FontSize(12).Bold().FontFeatures("tnum").NoWrap()
		})
		LevelBar(c, fraction, tint, 5)
		if detail != "" {
			ui.Text(c, detail).FontSize(10).TextColor(t.TextMuted.Alpha(0.75)).FontFeatures("tnum").NoWrap()
		}
	})
}

func popoverStats(c *ui.Context, p power.Data, temp float64, hasTemp bool) {
	t := c.Theme()
	cell := func(key, value, label string, tint ui.Color) {
		ui.Column(c.Key(key)).Gap(2).Grow(1).Width(0).AlignItems(ui.Center).Children(func() {
			ui.Text(c, value).FontSize(13).FontWeight(600).FontFeatures("tnum").TextColor(tint).NoWrap()
			ui.Text(c, label).FontSize(10).TextColor(t.TextMuted).NoWrap()
		})
	}
	ui.Row(c).Children(func() {
		if h := p.BatteryHealthPercent; h != nil {
			tint := t.Text
			switch {
			case *h < 60:
				tint = Red.Color(c)
			case *h < 80:
				tint = Orange.Color(c)
			}
			cell("health", fmt.Sprintf("%d%%", *h), L("Health", "健康度"), tint)
		}
		if hasTemp {
			tint := t.Text
			if temp > 40 {
				tint = Red.Color(c)
			}
			cell("temp", fmt.Sprintf("%.1f°", temp), L("Battery Temp", "电池温度"), tint)
		}
		if cycles := p.CycleCount; cycles != nil {
			cell("cycles", fmt.Sprint(*cycles), L("Cycle Count", "循环次数"), t.Text)
		}
	})
}

func popoverChargeLimit(c *ui.Context, p power.Data, limit *power.ChargeLimiter, a PopoverActions) {
	t := c.Theme()
	if limit != nil {
		if s := limit.State(); s.Supported {
			popoverChargeLimitControl(c, s, limit)
			return
		}
	}
	row := ui.ButtonBase(c).AlignSelf(ui.Stretch).Label(L("Charge Limit", "充电上限")).
		Tooltip(L("Adjust the charge limit on the Battery page", "在电池页调整充电上限")).
		Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Grow(1).Children(func() {
				ui.Icon(c, lucide.Must("battery-charging")).Size(12, 12).TextColor(Green.Color(c))
				ui.Text(c, L("Charge Limit", "充电上限")).FontSize(12).TextColor(t.TextMuted).Grow(1)
				ui.Text(c, p.ChargeLimitText()).FontSize(12).FontWeight(500).NoWrap()
				ui.Icon(c, lucide.Must("chevron-right")).Size(9, 9).TextColor(t.TextMuted.Alpha(0.6))
			})
		})
	if row.Clicked() && a.OpenDetails != nil {
		a.OpenDetails(PageBattery)
	}
}

// popoverChargeLimitControl sets the limit right in the popover: the same choices as the Battery page.
func popoverChargeLimitControl(c *ui.Context, s power.ChargeLimitState, limit *power.ChargeLimiter) {
	t := c.Theme()
	ui.Column(c).Gap(7).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, lucide.Must("battery-charging")).Size(12, 12).TextColor(Green.Color(c))
			ui.Text(c, L("Charge Limit", "充电上限")).FontSize(12).TextColor(t.TextMuted).Grow(1)
			if s.Pending {
				feedback.Spin(c, L("Applying the charge limit…", "正在应用充电上限…"), feedback.SpinOptions{Size: feedback.SpinSmall})
			}
			value := fmt.Sprintf("%d%%", s.Percent)
			if s.Percent >= 100 {
				value = L("Off", "关闭")
			}
			ui.Text(c, value).FontSize(12).FontWeight(600).FontFeatures("tnum").NoWrap()
		})
		// A limit macOS reports outside its own choices is shown above but not offered as a choice.
		selected := slices.Index(s.Choices, s.Percent)
		if selected >= 0 && !s.Pending {
			labels := make([]string, len(s.Choices))
			for i, choice := range s.Choices {
				labels[i] = fmt.Sprintf("%d%%", choice)
			}
			if choice.Segments(c, L("Charge Limit", "充电上限"), labels, &selected).Changed() {
				limit.Set(s.Choices[selected])
			}
		}
		switch {
		case s.Err != nil:
			Status(c, "octagon-x", Red, L("Could not change the charge limit", "无法修改充电上限"), s.Err.Error())
		case s.AlDenteRunning:
			Status(c, "triangle-alert", Orange, L("AlDente may override this limit", "AlDente 可能会改写这个上限"), "")
		}
	})
}

func popoverFooter(c *ui.Context, d PopoverData, a PopoverActions) {
	ui.Column(c).Gap(4).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).PaddingStart(4).Children(func() {
			label := L("Launch at Login", "开机自启")
			on := d.LaunchAtLogin
			if choice.Toggle(c, label, &on).Changed() && a.SetLaunchAtLogin != nil {
				a.SetLaunchAtLogin(on)
			}
			ui.Spacer(c)
			if button.Ghost(c, L("Quit", "退出")).Clicked() && a.Quit != nil {
				a.Quit()
			}
		})
		if d.LaunchAtLoginError != "" {
			ui.Text(c, d.LaunchAtLoginError).FontSize(10).TextColor(Red.Color(c)).PaddingX(4)
		}
	})
}
