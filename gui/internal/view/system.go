package view

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/history"
	"powerfultop/internal/icons"
	"powerfultop/internal/system"
)

// SystemPage reads top to bottom from glance to detail: CPU and memory panels with their history,
// a strip of secondary figures, then the processes behind the numbers. The process lists only fill
// while sampling is on: the caller turns system.Monitor.SetProcessSamplingActive on while this
// page is on screen.
func SystemPage(c *ui.Context, s system.Snapshot, cpuTempC func() (float64, bool), hist *history.History, width float32) {
	columns := 1
	if width >= 760 {
		columns = 2
	}
	ui.Column(c).Gap(16).Children(func() {
		ui.Grid(c).Columns(columns).Gap(16).Children(func() {
			sysCPUPanel(c, s, cpuTempC, hist)
			sysMemoryPanel(c, s, hist)
		})
		ui.Grid(c).Columns(3).Gap(16).Children(func() { sysStatTiles(c, s) })
		ui.Grid(c).Columns(columns).Gap(16).AlignItems(ui.Start).Children(func() {
			sysProcessPanel(c, L("Top CPU", "CPU 占用最高"), "cpu", Blue, s.TopCPU, func(v float64) string { return fmt.Sprintf("%.1f%%", v) })
			sysProcessPanel(c, L("Top Memory", "内存占用最高"), "memory-stick", Green, s.TopMemory, system.FormatBytes)
		})
	})
}

func sysCPUPanel(c *ui.Context, s system.Snapshot, cpuTempC func() (float64, bool), hist *history.History) {
	cpu := s.CPU
	Surface(c, ui.Column(c).Padding(18).Gap(14), 18).Children(func() {
		PanelHeader(c, "cpu", "CPU", Blue, func() {
			if cpuTempC == nil {
				return
			}
			if temp, ok := cpuTempC(); ok {
				tint := Orange
				if temp < 60 {
					tint = Gray
				}
				Pill(c, "thermometer", fmt.Sprintf("%.0f°C", temp), tint)
			}
		})
		Hero(c, sysPercent(cpu.Usage()), "", Lf("User %s · System %s · Idle %s", "用户 %s · 系统 %s · 空闲 %s",
			sysPercent(cpu.User), sysPercent(cpu.System), sysPercent(cpu.Idle)), 30)
		sysChart(c, sysSeries(hist, history.CPUUsage), Blue)
		sysCoreClusters(c, cpu, s.CoreKinds)
	})
}

func sysMemoryPanel(c *ui.Context, s system.Snapshot, hist *history.History) {
	m := s.Memory
	Surface(c, ui.Column(c).Padding(18).Gap(14), 18).Children(func() {
		PanelHeader(c, "memory-stick", L("Memory", "内存"), Green, func() {
			if m != nil {
				Pill(c, "gauge", sysPressureText(m.Pressure), sysPressureTint(m.Pressure))
			}
		})
		if m == nil {
			ui.Text(c, L("Collecting data…", "正在收集数据…")).FontSize(11).TextColor(c.Theme().TextMuted)
			return
		}
		Hero(c, system.FormatBytes(float64(m.UsedBytes())), system.FormatBytes(float64(m.TotalBytes)),
			Lf("%s used", "已用 %s", sysPercent(m.UsedFraction())), 30)
		sysChart(c, sysSeries(hist, history.MemoryUsage), Green)
		parts := []sysPart{
			{L("App Memory", "App 内存"), float64(m.AppBytes), Blue},
			{L("Wired Memory", "联动内存"), float64(m.WiredBytes), Orange},
			{L("Compressed", "被压缩"), float64(m.CompressedBytes), Purple},
			{L("Available", "可用"), float64(m.AvailableBytes()), Gray},
		}
		// The composition sits at the bottom, level with the CPU panel's core bars beside it.
		ui.Spacer(c)
		ui.Column(c).Gap(10).Children(func() {
			sysStackedBar(c, parts[:3], float64(m.TotalBytes))
			ui.Grid(c).Columns(2).Gap(8).Children(func() {
				for _, p := range parts {
					sysLegend(c, p.tint, p.label, system.FormatBytes(p.value))
				}
			})
		})
	})
}

// sysStatTiles are the secondary figures that don't need a chart of their own.
func sysStatTiles(c *ui.Context, s system.Snapshot) {
	cores := max(len(s.CPU.PerCore), 1)
	if len(s.LoadAverage) == 3 {
		tint := Green
		if perCore := s.LoadAverage[0] / float64(cores); perCore >= 1 {
			tint = Red
		} else if perCore >= 0.7 {
			tint = Orange
		}
		sysTile(c, "activity", tint, L("Load Average", "平均负载"), fmt.Sprintf("%.2f", s.LoadAverage[0]),
			Lf("5 min %.2f · 15 min %.2f", "5 分钟 %.2f · 15 分钟 %.2f", s.LoadAverage[1], s.LoadAverage[2]),
			Lf("Runnable threads averaged over 1, 5 and 15 minutes; %d cores keep up with a load of %d.",
				"1、5、15 分钟内可运行线程的平均数；%d 个核心可承载 %d 的负载。", cores, cores))
	}
	if !s.BootTime.IsZero() {
		sysTile(c, "clock", Blue, L("Uptime", "已运行"), sysUptimeText(time.Since(s.BootTime)),
			Lf("Since %s", "启动于 %s", s.BootTime.Format("01-02 15:04")), "")
	}
	if m := s.Memory; m != nil {
		tint := Gray
		if m.SwapUsedBytes > 0 {
			tint = Orange
		}
		sysTile(c, "hard-drive", tint, L("Swap Used", "交换区"), system.FormatBytes(float64(m.SwapUsedBytes)),
			L("Memory paged out to disk", "换出到磁盘的内存"), "")
	}
}

func sysTile(c *ui.Context, icon string, tint Tint, label, value, detail, tooltip string) {
	t := c.Theme()
	tile := Surface(c, ui.Column(c).Padding(14).Gap(4), 14)
	if tooltip != "" {
		tile.Tooltip(tooltip)
	}
	tile.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Get(icon)).Size(12, 12).TextColor(tint.Color(c))
			ui.Text(c, label).FontSize(11).TextColor(t.TextMuted).NoWrap()
		})
		ui.Text(c, value).FontSize(20).Bold().FontFeatures("tnum").NoWrap()
		ui.Text(c, detail).FontSize(10).TextColor(t.TextMuted.Alpha(0.8)).FontFeatures("tnum").SingleLine()
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

func sysChart(c *ui.Context, points []Point, tint Tint) {
	if len(points) >= 2 {
		AreaChart(c, points, tint, &[2]float64{0, 100}, 96)
		return
	}
	ui.Column(c).Height(96).Center().Radius(10).Background(c.Theme().Text.Alpha(0.03)).Children(func() {
		ui.Text(c, L("Collecting data…", "正在收集数据…")).FontSize(11).TextColor(c.Theme().TextMuted.Alpha(0.75))
	})
}

// sysCoreClusters shows each core as a bar, grouped by cluster so efficiency and performance cores
// read as the two pools macOS schedules onto.
func sysCoreClusters(c *ui.Context, cpu system.CPULoad, kinds []system.CoreKind) {
	type cluster struct {
		title string
		share float64
		tint  Tint
		kind  system.CoreKind
	}
	var clusters []cluster
	if cpu.HasEfficiency {
		clusters = append(clusters, cluster{L("Efficiency Cores", "能效核心"), cpu.Efficiency, Teal, system.Efficiency})
	}
	if cpu.HasPerformance {
		clusters = append(clusters, cluster{L("Performance Cores", "性能核心"), cpu.Performance, Indigo, system.Performance})
	}
	if len(clusters) == 0 || len(kinds) != len(cpu.PerCore) {
		clusters = []cluster{{L("Cores", "核心"), cpu.Usage(), Indigo, 0}}
	}
	ui.Column(c).Gap(12).Children(func() {
		for _, cl := range clusters {
			var usage []float64
			var numbers []int
			for i, u := range cpu.PerCore {
				if cl.kind == 0 || kinds[i] == cl.kind {
					usage, numbers = append(usage, u), append(numbers, i+1)
				}
			}
			ui.Column(c.Key(cl.title)).Gap(6).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(8, 8).Radius(2).Background(cl.tint.Color(c))
					ui.Text(c, cl.title).FontSize(11).TextColor(c.Theme().TextMuted).Grow(1)
					ui.Text(c, Lf("%d cores · %s", "%d 核 · %s", len(usage), sysPercent(cl.share))).
						FontSize(11).FontWeight(600).FontFeatures("tnum").NoWrap()
				})
				sysCoreBars(c, usage, numbers, cl.tint)
			})
		}
	})
}

func sysCoreBars(c *ui.Context, usage []float64, numbers []int, tint Tint) {
	track := c.Theme().Text.Alpha(0.07)
	col := tint.Color(c)
	ui.Row(c).Gap(3).Children(func() {
		for i, u := range usage {
			f := float32(math.Min(math.Max(u, 0), 1))
			ui.Box(c.Key(numbers[i])).Grow(1).Width(0).Height(28).
				Tooltip(Lf("Core %d · %s", "核心 %d · %s", numbers[i], sysPercent(u))).
				Draw(func(p *ui.Painter, r ui.Rect) {
					p.Fill(r, track, 3)
					h := max(2, r.H*f)
					// Busier cores are more saturated, so a hot core stands out at a glance.
					p.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - h, W: r.W, H: h}, col.Alpha(0.45+0.55*f), 3)
				})
		}
	})
}

type sysPart struct {
	label string
	value float64
	tint  Tint
}

func sysStackedBar(c *ui.Context, parts []sysPart, total float64) {
	colors := make([]ui.Color, len(parts))
	for i, s := range parts {
		colors[i] = s.tint.Color(c)
	}
	track := c.Theme().Text.Alpha(0.08)
	ui.Box(c).Height(10).Radius(5).Clip().Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, track, 0)
		if total <= 0 {
			return
		}
		x := r.X
		for i, s := range parts {
			w := r.W * float32(s.value/total)
			p.Fill(ui.Rect{X: x, Y: r.Y, W: max(0, w-1.5), H: r.H}, colors[i], 0)
			x += w
		}
	})
}

func sysLegend(c *ui.Context, tint Tint, label, value string) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(8, 8).Radius(2).Background(tint.Color(c)).Shrink(0)
		ui.Text(c, label).FontSize(11).TextColor(c.Theme().TextMuted).Grow(1).NoWrap()
		ui.Text(c, value).FontSize(11).FontWeight(600).FontFeatures("tnum").NoWrap()
	})
}

// sysProcessPanel ranks processes with a fill behind each row scaled to the busiest one, the way
// Activity Monitor's heat columns read.
func sysProcessPanel(c *ui.Context, title, icon string, tint Tint, processes []system.ProcessUsage, format func(float64) string) {
	t := c.Theme()
	Surface(c, ui.Column(c).Padding(18).Gap(10), 18).Children(func() {
		PanelHeader(c, icon, title, tint, func() {})
		if len(processes) == 0 {
			ui.Text(c, L("Collecting data…", "正在收集数据…")).FontSize(11).TextColor(t.TextMuted.Alpha(0.75))
			return
		}
		top := math.Max(processes[0].Value, 1e-9)
		fill := tint.Color(c).Alpha(0.13)
		ui.Column(c).Gap(3).Children(func() {
			for i, p := range processes {
				share := float32(math.Min(p.Value/top, 1))
				ui.Row(c.Key(p.PID)).Gap(10).AlignItems(ui.Center).PaddingX(8).PaddingY(5).
					Draw(func(pt *ui.Painter, r ui.Rect) {
						pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: max(r.H, r.W*share), H: r.H}, fill, 6)
					}).
					Children(func() {
						ui.Text(c, fmt.Sprint(i+1)).FontSize(11).TextColor(t.TextMuted.Alpha(0.7)).FontFeatures("tnum").Width(14)
						ui.Text(c, p.Name).FontSize(12).Grow(1).Shrink(1).SingleLine()
						ui.Text(c, format(p.Value)).FontSize(12).FontWeight(600).FontFeatures("tnum").NoWrap()
					})
			}
		})
	})
}
