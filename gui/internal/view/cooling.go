package view

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/history"
	"powerfultop/internal/icons"
	"powerfultop/internal/thermal"
)

// CoolingState is the Cooling page's own UI state; keep one per window.
type CoolingState struct {
	mu       sync.Mutex
	draftRPM *float64
	commit   *time.Timer
}

// sliderSettle is how long the RPM slider must rest before its value is applied: the slider reports
// no release, and every applied value is a helper request.
const sliderSettle = 400 * time.Millisecond

// CoolingPage shows temperatures, fans and their control, and the recent history of both.
func CoolingPage(c *ui.Context, m *thermal.Monitor, h *history.History, state *CoolingState, width float32) {
	s := m.Snapshot()
	columns := 1
	if width >= 680 {
		columns = 2
	}
	ui.Column(c).Gap(16).Children(func() {
		ui.Grid(c).Columns(columns).Gap(16).Children(func() {
			if len(s.Temperatures) > 0 {
				temperatureCard(c, s)
			}
			if s.HasFans() {
				FanControlCard(c, m, s, state)
			}
		})
		ui.Grid(c).Columns(columns).Gap(16).Children(func() {
			if cpu, ok := s.Temperatures[thermal.GroupCPU]; ok {
				ChartPanel(c, L("CPU Average", "CPU 平均温度"), thermal.FormatCelsius(cpu.AverageC, 1), "", "cpu", Orange,
					h.Series(history.CPUTemperature), nil, L("Collecting data…", "正在收集数据…"))
			}
			if s.HasFans() {
				top := 1.0
				for _, f := range s.Fans {
					top = math.Max(top, f.MaxRPM)
				}
				ChartPanel(c, L("Fan Speed", "风扇转速"), thermal.FormatRPM(s.MaxActualRPM()), "", "fan", Teal,
					h.Series(history.FanRPM), &[2]float64{0, top}, L("Collecting data…", "正在收集数据…"))
			}
		})
	})
}

// GroupTitle names a sensor group.
func GroupTitle(g thermal.SensorGroup) string {
	switch g {
	case thermal.GroupGPU:
		return "GPU"
	case thermal.GroupBattery:
		return L("Battery", "电池")
	}
	return "CPU"
}

func temperatureCard(c *ui.Context, s thermal.Snapshot) {
	Card(c, L("Temperatures", "温度"), "thermometer", Orange, func() {
		if cpu, ok := s.Temperatures[thermal.GroupCPU]; ok {
			Hero(c, thermal.FormatCelsius(cpu.AverageC, 1), "", L("CPU Average", "CPU 平均温度"), 0)
		}
		for _, g := range thermal.SensorGroups {
			summary, ok := s.Temperatures[g]
			if !ok {
				continue
			}
			tint := Green
			switch {
			case summary.MaximumC >= 90:
				tint = Red
			case summary.MaximumC >= 75:
				tint = Orange
			}
			fraction := summary.MaximumC / thermal.FailsafeCelsius
			BarRow(c, GroupTitle(g),
				Lf("%s avg · %s max", "平均 %s · 最高 %s", thermal.FormatCelsius(summary.AverageC, 1), thermal.FormatCelsius(summary.MaximumC, 1)),
				&fraction, tint)
		}
		Note(c, L("Readings are smoothed over about 10 seconds; individual cores spike briefly under load.",
			"读数经过约 10 秒平滑；负载下单个核心会短暂冲高。"))
	})
}

// Preset indexes of the Auto / Full Blast / Custom control.
const (
	presetAutomatic = iota
	presetFullBlast
	presetCustom
)

func presetOf(mode thermal.FanControlMode) int {
	switch mode.Kind {
	case thermal.ModeAutomatic:
		return presetAutomatic
	case thermal.ModeFullBlast:
		return presetFullBlast
	}
	return presetCustom
}

// ControlledText says who drives the fans right now.
func ControlledText(s thermal.Snapshot) string {
	if s.Controlling {
		return L("Controlled by PowerfulTop", "由 PowerfulTop 控制")
	}
	return L("Controlled by macOS", "由系统控制")
}

// FanPresetControl is the Auto / Full Blast / Custom switch, shared by the page and the popover: equal
// segments across the width, each with an icon. While another tool holds the fans a choice would
// only be stored, not applied, so the control is disabled then.
func FanPresetControl(c *ui.Context, m *thermal.Monitor, s thermal.Snapshot) ui.Element {
	t := c.Theme()
	preset := presetOf(s.Mode)
	locked := s.ConflictDetected && !s.Controlling
	segments := []struct{ icon, label string }{
		{"fan", L("Auto", "自动")},
		{"zap", L("Full Blast", "全速")},
		{"sliders-horizontal", L("Custom", "自定义")},
	}
	seg := ui.SegmentedBase(c, &preset, len(segments))
	track := seg.Track.Label(L("Fans", "风扇")).Gap(2).Padding(2).Radius(9).Background(t.Text.Alpha(0.06)).Disabled(locked)
	if locked {
		track.Opacity(0.5)
	}
	track.Children(func() {
		for i, sg := range segments {
			chosen := i == preset
			e := seg.Segment(i).Label(sg.label).Grow(1).Width(0).PaddingY(5).Radius(7)
			if chosen {
				e.Background(CardSurface(c)).Border(1, CardBorder(c))
			}
			e.Children(func() {
				ui.Row(c).Gap(5).Center().Children(func() {
					color := t.TextMuted
					if chosen {
						color = t.Text
					}
					ui.Icon(c, icons.Get(sg.icon)).Size(12, 12).TextColor(color)
					text := ui.Text(c, sg.label).FontSize(12).TextColor(color).NoWrap()
					if chosen {
						text.FontWeight(600)
					}
				})
			})
		}
	})
	if !locked && seg.Track.Changed() {
		m.SelectPreset(preset)
	}
	return track
}

// FanConflictBanner says another tool holds the fans, naming it when it is known.
func FanConflictBanner(c *ui.Context, s thermal.Snapshot) {
	text := L("Another fan tool is in control", "其他风扇工具正在控制")
	if s.ConflictingTool != "" {
		text = Lf("%s is running, so PowerfulTop leaves the fans alone", "%s 正在运行，PowerfulTop 暂不接管风扇", s.ConflictingTool)
	}
	col := Orange.Color(c)
	ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(6, 8).Radius(7).Background(col.Alpha(0.12)).
		Tooltip(L("Another fan tool (such as Macs Fan Control) is controlling the fans. Quit it to let PowerfulTop take over; until then PowerfulTop only monitors.",
			"检测到其他风扇工具（如 Macs Fan Control）正在控制风扇。退出它之后 PowerfulTop 才会接管，在此之前只做监控。")).
		Children(func() {
			ui.Icon(c, icons.Get("triangle-alert")).Size(12, 12).TextColor(col).Shrink(0)
			ui.Text(c, text).FontSize(11).TextColor(c.Theme().Text.Alpha(0.85))
		})
}

// FanControlCard mirrors Macs Fan Control: Auto hands fans to macOS; Custom is a constant RPM or a temperature ramp.
func FanControlCard(c *ui.Context, m *thermal.Monitor, s thermal.Snapshot, state *CoolingState) {
	Card(c, L("Fans", "风扇"), "fan", Teal, func() {
		Hero(c, thermal.FormatRPM(s.MaxActualRPM()), "", ControlledText(s), 0)
		for _, fan := range s.Fans {
			var fraction *float64
			if fan.MaxRPM > 0 {
				f := fan.ActualRPM / fan.MaxRPM
				fraction = &f
			}
			BarRow(c, Lf("Fan %d", "风扇 %d", fan.Index+1), fmt.Sprintf("%.0f / %.0f RPM", fan.ActualRPM, fan.MaxRPM), fraction, Teal)
		}
		ui.Box(c).Height(2)
		FanPresetControl(c, m, s)
		if s.Mode.Kind == thermal.ModeConstant || s.Mode.Kind == thermal.ModeSensor {
			customControls(c, m, s, state)
		}
		fanStatusNotes(c, m, s)
	})
}

func customControls(c *ui.Context, m *thermal.Monitor, s thermal.Snapshot, state *CoolingState) {
	kind := 0
	if s.Mode.Kind == thermal.ModeSensor {
		kind = 1
	}
	if ui.Segmented(c, &kind, L("Constant RPM", "固定转速"), L("Follow Temperature", "跟随温度")).Changed() {
		if kind == 1 {
			m.SetMode(lastOrDefault(s.LastCustomMode, thermal.ModeSensor, thermal.DefaultSensor))
		} else {
			m.SetMode(lastOrDefault(s.LastCustomMode, thermal.ModeConstant, thermal.DefaultConstant))
		}
	}

	switch s.Mode.Kind {
	case thermal.ModeConstant:
		lo, hi := fanSpan(s)
		rpm := state.draft(s.Mode.RPM)
		Row(c, L("Target Speed", "目标转速"), thermal.FormatRPM(rpm), "", true)
		if ui.StepSlider(c, &rpm, lo, math.Max(hi, lo+100), 100).Label(L("Target Speed", "目标转速")).Changed() {
			state.setDraft(rpm, func() { m.SetMode(thermal.Constant(rpm)) })
		}
	case thermal.ModeSensor:
		mode := s.Mode
		group := 0
		if mode.Group == thermal.GroupGPU {
			group = 1
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, L("Sensor", "传感器")).FontSize(12).TextColor(c.Theme().TextMuted).Grow(1)
			if ui.Segmented(c, &group, "CPU", "GPU").Label(L("Sensor", "传感器")).Changed() {
				mode.Group = []thermal.SensorGroup{thermal.GroupCPU, thermal.GroupGPU}[group]
				m.SetMode(mode)
			}
		})
		lower, upper := mode.LowerC, mode.UpperC
		stepperRow(c, Lf("Start ramping at %.0f°C", "%.0f°C 开始提速", lower), &lower, 30, 90, func() {
			m.SetMode(thermal.Sensor(mode.Group, lower, math.Max(upper, lower+5)))
		})
		stepperRow(c, Lf("Full speed at %.0f°C", "%.0f°C 时全速", upper), &upper, 40, 100, func() {
			m.SetMode(thermal.Sensor(mode.Group, math.Min(lower, upper-5), upper))
		})
	}
}

func stepperRow(c *ui.Context, label string, value *float64, lo, hi float64, changed func()) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(12).Grow(1)
		if ui.Stepper(c, value, lo, hi, 5).Label(label).Changed() {
			changed()
		}
	})
}

// fanSpan is the range every fan accepts: the highest minimum to the lowest maximum.
func fanSpan(s thermal.Snapshot) (float64, float64) {
	lo, hi := 0.0, math.Inf(1)
	for _, f := range s.Fans {
		lo, hi = math.Max(lo, f.MinRPM), math.Min(hi, f.MaxRPM)
	}
	if math.IsInf(hi, 1) {
		hi = lo
	}
	return lo, hi
}

func lastOrDefault(last thermal.FanControlMode, kind thermal.FanControlModeKind, fallback thermal.FanControlMode) thermal.FanControlMode {
	if last.Kind == kind {
		return last
	}
	return fallback
}

func (s *CoolingState) draft(applied float64) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draftRPM == nil {
		return applied
	}
	if s.commit == nil && *s.draftRPM == applied {
		s.draftRPM = nil
		return applied
	}
	return *s.draftRPM
}

func (s *CoolingState) setDraft(rpm float64, apply func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.draftRPM = &rpm
	if s.commit != nil {
		s.commit.Stop()
	}
	s.commit = time.AfterFunc(sliderSettle, func() {
		s.mu.Lock()
		s.commit = nil
		s.mu.Unlock()
		apply()
	})
}

func fanStatusNotes(c *ui.Context, m *thermal.Monitor, s thermal.Snapshot) {
	switch {
	case s.ConflictDetected && !s.Controlling:
		FanConflictBanner(c, s)
	case !s.Mode.IsAutomatic() && s.HelperStatus != thermal.HelperReady:
		FanHelperInstallButton(c, m, s)
	case s.Mode.IsAutomatic() && allFansResting(s):
		Status(c, "moon", Gray, L("Fans are resting", "风扇已停转"),
			L("macOS stops the fans while temperatures are low; they spin up on their own when cooling is needed.",
				"温度较低时 macOS 会让风扇停转，需要散热时会自动启动。"))
	case !s.Mode.IsAutomatic():
		Status(c, "shield-check", Green, L("Safety handback armed", "安全交还已就绪"),
			Lf("Fans return to macOS control when PowerfulTop quits, the Mac sleeps, or the CPU or GPU reaches %.0f°C.",
				"PowerfulTop 退出、Mac 睡眠，或 CPU/GPU 达到 %.0f°C 时，风扇会自动交还系统控制。", thermal.FailsafeCelsius))
	}
	if s.LastError != nil {
		Status(c, "octagon-x", Red, HelperErrorText(s.LastError), "")
	}
}

func allFansResting(s thermal.Snapshot) bool {
	for _, f := range s.Fans {
		if f.ActualRPM != 0 {
			return false
		}
	}
	return true
}

// FanHelperInstallButton asks to install or re-authorize the helper.
func FanHelperInstallButton(c *ui.Context, m *thermal.Monitor, s thermal.Snapshot) {
	ui.Column(c).Gap(6).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.PrimaryButton(c, HelperActionText(s.HelperStatus)).Disabled(s.Installing).Clicked() {
				m.InstallHelper()
			}
			if s.Installing {
				ui.Spinner(c)
			}
		})
		if s.HelperStatus == thermal.HelperNeedsUpdate {
			Note(c, L("PowerfulTop was updated, so the fan helper must be authorized again.", "PowerfulTop 已更新，需要重新授权风扇控制组件。"))
		} else {
			Note(c, L("Controlling fans needs a small helper that runs as root. macOS asks for an administrator password once.",
				"控制风扇需要安装一个以 root 运行的小组件，macOS 会请求一次管理员密码。"))
		}
	})
}

// HelperActionText is the install button's title for a status.
func HelperActionText(status thermal.HelperStatus) string {
	if status == thermal.HelperNeedsUpdate {
		return L("Update Fan Helper", "更新风扇控制组件")
	}
	return L("Install Fan Helper", "安装风扇控制组件")
}

// HelperStatusText describes a status, as the Settings page shows it.
func HelperStatusText(status thermal.HelperStatus) string {
	switch status {
	case thermal.HelperReady:
		return L("Installed", "已安装")
	case thermal.HelperNeedsUpdate:
		return L("Needs authorization", "需要重新授权")
	}
	return L("Not installed", "未安装")
}

// HelperErrorText localizes the helper's known failures; others come from the helper or AppleScript as they are.
func HelperErrorText(err error) string {
	switch {
	case errors.Is(err, thermal.ErrMissingHelper):
		return L("The fan helper is missing from the app bundle.", "app 包中缺少风扇控制组件。")
	case errors.Is(err, thermal.ErrUnsignedApp):
		return L("PowerfulTop must be code-signed to control fans.", "PowerfulTop 需要代码签名才能控制风扇。")
	case errors.Is(err, thermal.ErrCancelled):
		return L("Authorization was cancelled.", "已取消授权。")
	case errors.Is(err, thermal.ErrConnectionFailed):
		return L("Could not connect to the fan helper.", "无法连接风扇控制组件。")
	}
	return err.Error()
}

// FanSection is the popover's compact fan control: speed, who controls it, the preset switch and one status line.
func FanSection(c *ui.Context, m *thermal.Monitor) {
	s := m.Snapshot()
	if !s.HasFans() {
		return
	}
	t := c.Theme()
	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Get("fan")).Size(14, 14).TextColor(Teal.Color(c))
			ui.Text(c, thermal.FormatRPM(s.MaxActualRPM())).FontSize(14).Bold().FontFeatures("tnum").NoWrap().Grow(1)
			// Who drives the fans, as a pill: tinted while PowerfulTop does.
			pill, pillText := t.Text.Alpha(0.07), t.TextMuted
			if s.Controlling {
				pill, pillText = Teal.Color(c).Alpha(0.16), Teal.Color(c)
			}
			ui.Text(c, ControlledText(s)).FontSize(10).FontWeight(500).TextColor(pillText).
				Padding(2, 7).Radius(8).Background(pill).NoWrap()
		})
		FanPresetControl(c, m, s)
		switch {
		case s.ConflictDetected && !s.Controlling:
			FanConflictBanner(c, s)
		case !s.Mode.IsAutomatic() && s.HelperStatus != thermal.HelperReady:
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, HelperActionText(s.HelperStatus)).Disabled(s.Installing).Clicked() {
					m.InstallHelper()
				}
				if s.Installing {
					ui.Spinner(c)
				}
			})
		case s.LastError != nil:
			Status(c, "octagon-x", Red, HelperErrorText(s.LastError), "")
		case s.Mode.Kind == thermal.ModeConstant:
			Status(c, "gauge", Gray, Lf("Holding %.0f RPM", "保持 %.0f RPM", s.Mode.RPM), "")
		case s.Mode.Kind == thermal.ModeSensor:
			Status(c, "thermometer", Gray, Lf("%s ramp %.0f–%.0f°C", "%s %.0f–%.0f°C 提速", GroupTitle(s.Mode.Group), s.Mode.LowerC, s.Mode.UpperC), "")
		case s.Mode.IsAutomatic() && allFansResting(s):
			Status(c, "moon", Gray, L("Fans are resting", "风扇已停转"), "")
		}
	})
}

// FanHelperCard is the Settings page's helper card: status, install / update and uninstall.
func FanHelperCard(c *ui.Context, m *thermal.Monitor) {
	s := m.Snapshot()
	if !s.HasFans() {
		return
	}
	Card(c, L("Fan Helper", "风扇控制组件"), "shield", Teal, func() {
		Row(c, L("Status", "状态"), HelperStatusText(s.HelperStatus), "", true)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if s.HelperStatus != thermal.HelperReady {
				if ui.Button(c, HelperActionText(s.HelperStatus)).Disabled(s.Installing).Clicked() {
					m.InstallHelper()
				}
			}
			if s.HelperStatus != thermal.HelperNotInstalled {
				if ui.Button(c, L("Uninstall", "卸载")).Disabled(s.Installing).Clicked() {
					m.UninstallHelper()
				}
			}
			if s.Installing {
				ui.Spinner(c)
			}
		})
		if s.LastError != nil {
			Status(c, "octagon-x", Red, HelperErrorText(s.LastError), "")
		}
		Note(c, L("The helper runs as root only to write fan speeds. It accepts requests from this exact PowerfulTop build and hands fans back to macOS whenever PowerfulTop stops checking in.",
			"该组件以 root 运行，仅用于写入风扇转速；只接受当前这个 PowerfulTop 版本的请求，PowerfulTop 停止响应时会自动把风扇交还系统。"))
	})
}
