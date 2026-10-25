package view

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/display"
	"powerfultop/internal/history"
	"powerfultop/internal/system"
)

func TestSystemPageRenders(t *testing.T) {
	const gib, page = uint64(1 << 30), uint64(16_384)
	mem := system.NewMemoryStats(128*gib, 46*gib/page, 2*gib/page, 7*gib/page, 3*gib/page, page, 0, system.PressureWarning)
	s := system.Snapshot{
		CPU: system.CPULoad{User: 0.35, System: 0.10, Idle: 0.55, PerCore: []float64{0.8, 0.1},
			Efficiency: 0.8, HasEfficiency: true, Performance: 0.1, HasPerformance: true},
		Memory:      &mem,
		LoadAverage: []float64{2.5, 3, 3.5},
		BootTime:    time.Now().Add(-26 * time.Hour),
		TopCPU:      []system.ProcessUsage{{PID: 410, Name: "WindowServer", Value: 33.5}},
		CoreKinds:   []system.CoreKind{system.Efficiency, system.Performance},
	}
	var hist history.History
	now := time.Now()
	for i := range 3 {
		hist.Append(history.Sample{Time: now.Add(time.Duration(i) * history.SampleInterval), Values: map[history.Metric]float64{history.CPUUsage: 40, history.MemoryUsage: 42}})
	}
	temp := func() (float64, bool) { return 48, true }
	for _, width := range []float32{1000, 600} {
		tt := ui.NewTester(func(c *ui.Context) { SystemPage(c, s, temp, &hist, width) }, int(width), 1400)
		for _, want := range []string{"CPU", "45%", "48°", "2.50", "Efficiency Cores", "80%", "Performance Cores", "1 days, 2 hr",
			"Load Average", "3.50", "WindowServer", "33.5%", "Memory", "54 GB", "/ 128 GB", "Pressure Warning", "App Memory", "44 GB",
			"Collecting data…"} {
			if !tt.HasText(want) {
				t.Errorf("width %v: missing %q in %q", width, want, tt.Texts())
			}
		}
	}
}

func TestBrightnessRow(t *testing.T) {
	var got []float64
	power := 2.4
	displays := []display.Brightness{
		{ID: 1, Name: "Built-in Retina Display", BuiltIn: true, Method: display.Native, Level: 0.62},
		{ID: 4, Name: "", Method: display.Software, Level: 1},
	}
	tt := ui.NewTester(func(c *ui.Context) {
		BrightnessSection(c, displays, &power, func(id uint32, level float64) {
			if id == 1 {
				got = append(got, level)
			}
		})
	}, 320, 200)
	for _, want := range []string{"Built-in Retina Display", "Native", "2.4 W", "62%", "Display", "Software", "100%"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	// The slider sits on the row under the display's name; pressing near its right end raises the level.
	name, ok := tt.Find("Built-in Retina Display")
	if !ok {
		t.Fatal("no name")
	}
	tt.ClickAt(280, name.Y+name.H+12)
	if len(got) == 0 || got[len(got)-1] <= 0.62 {
		t.Errorf("the slider should report a higher level, got %v", got)
	}
}
