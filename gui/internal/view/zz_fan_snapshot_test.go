package view

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/thermal"
)

func TestFanSectionSnapshot(t *testing.T) {
	dir := os.Getenv("FAN_SNAPSHOT")
	if dir == "" {
		t.Skip()
	}
	fans := []thermal.FanReading{{ActualRPM: 0, MinRPM: 2317, MaxRPM: 7826, Mode: 3}, {MinRPM: 2317, MaxRPM: 7826, Mode: 3}}
	states := map[string]thermal.Snapshot{
		"system":      {Fans: fans, Available: true},
		"conflict":    {Fans: fans, Available: true, ConflictDetected: true, ConflictingTool: "Macs Fan Control"},
		"controlling": {Fans: []thermal.FanReading{{ActualRPM: 7826, MaxRPM: 7826, Mode: 1}}, Available: true, Controlling: true, Mode: thermal.FullBlast, HelperStatus: thermal.HelperReady},
	}
	for name, s := range states {
		m := thermal.MonitorWithSnapshot(s)
		tt := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(ui.DarkTheme())
			ui.Column(c).Fill().Padding(10, 14).Background(ui.Hex("#1e1e20")).Children(func() { FanSection(c, m) })
		}, PopoverWidth, 130)
		f, _ := os.Create(dir + "/fan_" + name + ".png")
		_ = png.Encode(f, tt.Image())
		f.Close()
	}
}
