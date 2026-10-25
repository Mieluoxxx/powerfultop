package view

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/thermal"
)

func TestCustomControlsSnapshot(t *testing.T) {
	dir := os.Getenv("FAN_SNAPSHOT")
	if dir == "" {
		t.Skip()
	}
	fans := []thermal.FanReading{{ActualRPM: 3000, MinRPM: 2317, MaxRPM: 7826, Mode: 1}}
	for name, mode := range map[string]thermal.FanControlMode{"constant": thermal.Constant(4000), "sensor": thermal.DefaultSensor} {
		m := thermal.MonitorWithSnapshot(thermal.Snapshot{Fans: fans, Available: true, Mode: mode, Controlling: true, HelperStatus: thermal.HelperReady})
		var state CoolingState
		tt := ui.NewTester(func(c *ui.Context) {
			c.SetTheme(ui.DarkTheme())
			ui.Column(c).Fill().Padding(12).Background(ui.Hex("#1e1e20")).Children(func() { FanControlCard(c, m, m.Snapshot(), &state) })
		}, 460, 460)
		f, _ := os.Create(dir + "/custom_" + name + ".png")
		_ = png.Encode(f, tt.Image())
		f.Close()
	}
}
