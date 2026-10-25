package view

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/history"
	"powerfultop/internal/system"
)

// SystemPage shows CPU and memory in the style of Stats: rings up top, history, a breakdown, then top
// processes. The process lists only fill while sampling is on: the caller turns
// system.Monitor.SetProcessSamplingActive on while this page is on screen.
func SystemPage(c *ui.Context, s system.Snapshot, cpuTempC func() (float64, bool), hist *history.History, width float32) {
	columns := 1
	if width >= 760 {
		columns = 2
	}
	ui.Grid(c).Columns(columns).Gap(16).AlignItems(ui.Start).Children(func() {
		sysCpuCard(c, s, cpuTempC, hist)
		sysMemoryCard(c, s, hist)
	})
}

func sysCpuCard(c *ui.Context, s system.Snapshot, cpuTempC func() (float64, bool), hist *history.History) {
	cpu := s.CPU
	Card(c, L("CPU", "CPU"), "cpu", Blue, func() {
		ui.Row(c).Gap(18).AlignItems(ui.Center).PaddingY(4).Children(func() {
			ui.Spacer(c)
			if cpuTempC != nil {
				if temp, ok := cpuTempC(); ok {
					RingGauge(c, temp/100, fmt.Sprintf("%.0f°", temp), L("Temperature", "温度"), Orange, 64)
				}
			}
			RingGauge(c, cpu.Usage(), sysPercent(cpu.Usage()), L("Usage", "占用"), Blue, 88)
			if len(s.LoadAverage) > 0 {
				oneMinute := s.LoadAverage[0]
				RingGauge(c, oneMinute/float64(max(len(cpu.PerCore), 1)), fmt.Sprintf("%.2f", oneMinute), L("Load", "负载"), Purple, 64)
			}
			ui.Spacer(c)
		})

		sysMiniChart(c, sysSeries(hist, history.CPUUsage), Blue)
		sysCoreBars(c, cpu.PerCore, s.CoreKinds)

		sysSectionLabel(c, L("Details", "详细参数"))
		sysLegendRow(c, Red, L("System", "系统"), sysPercent(cpu.System))
		sysLegendRow(c, Blue, L("User", "用户"), sysPercent(cpu.User))
		sysLegendRow(c, Gray, L("Idle", "空闲"), sysPercent(cpu.Idle))
		if cpu.HasEfficiency {
			sysLegendRow(c, Teal, L("Efficiency Cores", "能效核心"), sysPercent(cpu.Efficiency))
		}
		if cpu.HasPerformance {
			sysLegendRow(c, Indigo, L("Performance Cores", "性能核心"), sysPercent(cpu.Performance))
		}
		if !s.BootTime.IsZero() {
			Row(c, L("Uptime", "启动时间"), sysUptimeText(time.Since(s.BootTime)), "", false)
		}

		if len(s.LoadAverage) == 3 {
			sysSectionLabel(c, L("Load Average", "平均负载"))
			Row(c, L("1 min", "1 分钟"), fmt.Sprintf("%.2f", s.LoadAverage[0]), "", false)
			Row(c, L("5 min", "5 分钟"), fmt.Sprintf("%.2f", s.LoadAverage[1]), "", false)
			Row(c, L("15 min", "15 分钟"), fmt.Sprintf("%.2f", s.LoadAverage[2]), "", false)
		}

		sysSectionLabel(c, L("Top Processes", "高占用进程"))
		sysProcessList(c, s.TopCPU, func(v float64) string { return fmt.Sprintf("%.1f%%", v) })
	})
}

func sysMemoryCard(c *ui.Context, s system.Snapshot, hist *history.History) {
	Card(c, L("Memory", "内存"), "memory-stick", Green, func() {
		if m := s.Memory; m != nil {
			ui.Row(c).AlignItems(ui.Center).PaddingY(4).Children(func() {
				ui.Spacer(c)
				RingGauge(c, m.UsedFraction(), sysPercent(m.UsedFraction()), sysPressureText(m.Pressure), sysPressureTint(m.Pressure), 88)
				ui.Spacer(c)
			})

			sysMiniChart(c, sysSeries(hist, history.MemoryUsage), Green)

			sysSectionLabel(c, L("Details", "详细参数"))
			Row(c, L("Used", "已用"), system.FormatBytes(float64(m.UsedBytes())), system.FormatBytes(float64(m.TotalBytes)), true)
			sysStackedBar(c, []sysBarSegment{
				{float64(m.AppBytes), Blue},
				{float64(m.WiredBytes), Orange},
				{float64(m.CompressedBytes), Red},
			}, float64(m.TotalBytes))
			sysLegendRow(c, Blue, L("App Memory", "App 内存"), system.FormatBytes(float64(m.AppBytes)))
			sysLegendRow(c, Orange, L("Wired Memory", "联动内存"), system.FormatBytes(float64(m.WiredBytes)))
			sysLegendRow(c, Red, L("Compressed", "被压缩"), system.FormatBytes(float64(m.CompressedBytes)))
			sysLegendRow(c, Gray, L("Available", "可用"), system.FormatBytes(float64(m.AvailableBytes())))
			Row(c, L("Swap Used", "交换区"), system.FormatBytes(float64(m.SwapUsedBytes)), "", false)
		}

		sysSectionLabel(c, L("Top Processes", "高占用进程"))
		sysProcessList(c, s.TopMemory, system.FormatBytes)
	})
}

func sysPercent(fraction float64) string { return fmt.Sprintf("%.0f%%", fraction*100) }

func sysPressureText(p system.MemoryPressure) string {
	switch p {
	case system.PressureWarning:
		return L("Pressure Warning", "压力警告")
	case system.PressureCritical:
		return L("Pressure Critical", "压力严重")
	}
	return L("Pressure Normal", "压力正常")
}

func sysPressureTint(p system.MemoryPressure) Tint {
	switch p {
	case system.PressureWarning:
		return Yellow
	case system.PressureCritical:
		return Red
	}
	return Green
}

// sysUptimeText follows DateComponentsFormatter's short style with days, hours and minutes, two units at most.
func sysUptimeText(d time.Duration) string {
	minutes := int(d.Minutes())
	days, hours, mins := minutes/(24*60), minutes/60%24, minutes%60
	switch {
	case days > 0:
		return Lf("%d days, %d hr", "%d天%d小时", days, hours)
	case hours > 0:
		return Lf("%d hr, %d min", "%d小时%d分钟", hours, mins)
	}
	return Lf("%d min", "%d分钟", mins)
}

func sysSeries(hist *history.History, m history.Metric) []Point {
	if hist == nil {
		return nil
	}
	return hist.Series(m)
}

// RingGauge is a circular level with its value in the middle and a caption under it.
func RingGauge(c *ui.Context, fraction float64, label, caption string, tint Tint, size float32) {
	t := c.Theme()
	col := tint.Color(c)
	track := t.Text.Alpha(0.1)
	f := float32(math.Min(math.Max(fraction, 0), 1))
	ui.Column(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(size, size).Center().Draw(func(p *ui.Painter, r ui.Rect) {
			line := size * 0.1
			radius := (min(r.W, r.H) - line) / 2
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			p.StrokePath(new(ui.Path).Circle(cx, cy, radius), line, track)
			if f > 0 {
				p.StrokePath(sysArc(cx, cy, radius, f), line, col)
			}
		}).Children(func() {
			ui.Text(c, label).FontSize(size * 0.22).Bold().FontFeatures("tnum").NoWrap()
		})
		ui.Text(c, caption).FontSize(10).TextColor(t.TextMuted).NoWrap()
	})
}

// sysArc runs clockwise from twelve o'clock over fraction of the circle.
func sysArc(cx, cy, radius, fraction float32) *ui.Path {
	path := new(ui.Path)
	steps := max(2, int(64*fraction))
	for i := 0; i <= steps; i++ {
		angle := -math.Pi/2 + 2*math.Pi*float64(fraction)*float64(i)/float64(steps)
		x := cx + radius*float32(math.Cos(angle))
		y := cy + radius*float32(math.Sin(angle))
		if i == 0 {
			path.MoveTo(x, y)
		} else {
			path.LineTo(x, y)
		}
	}
	return path
}

func sysMiniChart(c *ui.Context, points []Point, tint Tint) {
	ui.Column(c).Padding(8).Radius(10).Background(c.Theme().Text.Alpha(0.04)).Children(func() {
		if len(points) >= 2 {
			AreaChart(c, points, tint, &[2]float64{0, 100}, 70)
		} else {
			ui.Box(c).Height(70)
		}
	})
}

func sysCoreBars(c *ui.Context, perCore []float64, kinds []system.CoreKind) {
	t := c.Theme()
	ui.Row(c).Gap(3).Padding(8).Radius(10).Background(t.Text.Alpha(0.04)).Children(func() {
		for i, usage := range perCore {
			tint := Indigo
			if i < len(kinds) && kinds[i] == system.Efficiency {
				tint = Teal
			}
			col := tint.Color(c).Alpha(0.85)
			track := t.Text.Alpha(0.06)
			u := float32(math.Min(math.Max(usage, 0), 1))
			ui.Box(c).Key(i).Grow(1).Height(44).Draw(func(p *ui.Painter, r ui.Rect) {
				p.Fill(r, track, 2)
				h := max(2, r.H*u)
				p.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - h, W: r.W, H: h}, col, 2)
			})
		}
	})
}

type sysBarSegment struct {
	value float64
	tint  Tint
}

func sysStackedBar(c *ui.Context, segments []sysBarSegment, total float64) {
	colors := make([]ui.Color, len(segments))
	for i, s := range segments {
		colors[i] = s.tint.Color(c)
	}
	track := c.Theme().Text.Alpha(0.1)
	ui.Box(c).Height(8).Radius(4).Clip().Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, track, 0)
		if total <= 0 {
			return
		}
		x := r.X
		for i, s := range segments {
			w := r.W * float32(s.value/total)
			p.Fill(ui.Rect{X: x, Y: r.Y, W: max(0, w-1), H: r.H}, colors[i], 0)
			x += w
		}
	})
}

func sysLegendRow(c *ui.Context, tint Tint, label, value string) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(10, 10).Radius(3).Background(tint.Color(c)).Shrink(0)
		ui.Column(c).Grow(1).Children(func() {
			Row(c, label, value, "", false)
		})
	})
}

func sysSectionLabel(c *ui.Context, text string) {
	line := c.Theme().Text.Alpha(0.1)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(6, 0, 0, 0).Children(func() {
		ui.Box(c).Height(1).Grow(1).Background(line)
		ui.Text(c, text).FontSize(10).Bold().TextColor(c.Theme().TextMuted.Alpha(0.75)).NoWrap()
		ui.Box(c).Height(1).Grow(1).Background(line)
	})
}

func sysProcessList(c *ui.Context, processes []system.ProcessUsage, format func(float64) string) {
	if len(processes) == 0 {
		ui.Text(c, L("Collecting data…", "正在收集数据…")).FontSize(11).TextColor(c.Theme().TextMuted.Alpha(0.75))
		return
	}
	for _, p := range processes {
		ui.Column(c).Key(p.PID).Children(func() {
			Row(c, p.Name, format(p.Value), "", false)
		})
	}
}
