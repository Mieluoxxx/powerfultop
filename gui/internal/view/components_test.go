package view

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestComponentsRender(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	points := []Point{{Time: now, Value: 10}, {Time: now.Add(time.Minute), Value: 12}, {Time: now.Add(2 * time.Minute), Value: 11}}
	half := 0.5
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Gap(8).Padding(8).Children(func() {
			Card(c, "Battery", "battery-full", Green, func() {
				Hero(c, "48.2 W", "90 W", "System", 0)
				Row(c, "Health", "99%", "", true)
				BarRow(c, "CPU", "12%", &half, Blue)
				Note(c, "A note")
				Status(c, "triangle-alert", Orange, "Warning", "")
			})
			ui.Row(c).Children(func() {
				StripItem(c, "zap", Yellow, "Power", "12 W", false, "")
				StripDivider(c)
			})
			ChartPanel(c, "System", "11 W", "avg", "activity", Blue, points, "Collecting data…")
			ChartPanel(c, "Empty", "–", "", "activity", Blue, nil, "Collecting data…")
		})
	}, 480, 900)
	for _, want := range []string{"Battery", "48.2 W", "/ 90 W", "Health", "99%", "A note", "Warning", "12 W", "Collecting data…"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	if img := tt.Image(); img.Bounds().Dx() == 0 {
		t.Error("nothing rendered")
	}
}
