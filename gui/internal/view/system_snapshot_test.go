//go:build darwin

package view

import (
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/display"
	"powerfultop/internal/history"
	"powerfultop/internal/system"
)

// Renders the system page with live data when SYSTEM_SNAPSHOT names a PNG to write.
func TestSystemSnapshot(t *testing.T) {
	path := os.Getenv("SYSTEM_SNAPSHOT")
	if path == "" {
		t.Skip()
	}
	m := system.NewMonitor()
	m.Start()
	defer m.Stop()
	m.SetProcessSamplingActive(true)
	time.Sleep(3 * time.Second)
	var hist history.History
	now := time.Now()
	for i := range 40 {
		hist.Append(history.Sample{Time: now.Add(time.Duration(i) * history.SampleInterval),
			Values: map[history.Metric]float64{history.CPUUsage: 20 + float64(i%9)*3, history.MemoryUsage: 35 + float64(i%4)}})
	}
	power := 2.4
	displays := []display.Brightness{
		{ID: 1, Name: "Built-in Retina Display", BuiltIn: true, Method: display.Native, Level: 0.62},
		{ID: 4, Name: "P275MV MAX", Method: display.DDC, Level: 1},
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(24).Gap(16).Background(c.Theme().Background).Children(func() {
			SystemPage(c, m.Snapshot(), func() (float64, bool) { return 47, true }, &hist, 900)
			ui.Column(c).Width(300).Padding(12).Children(func() {
				BrightnessSection(c, displays, &power, func(uint32, float64) {})
			})
		})
	}, 1000, 1150)
	f, _ := os.Create(path)
	defer f.Close()
	_ = png.Encode(f, tt.Image())
}
