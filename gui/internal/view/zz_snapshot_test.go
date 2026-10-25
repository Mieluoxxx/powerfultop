package view

import (
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSnapshot(t *testing.T) {
	if os.Getenv("SNAPSHOT") == "" {
		t.Skip()
	}
	now := time.Unix(1_700_000_000, 0)
	var points []Point
	for i := range 30 {
		points = append(points, Point{Time: now.Add(time.Duration(i) * time.Minute), Value: 10 + float64(i%7)})
	}
	half := 0.62
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Gap(12).Padding(16).Background(c.Theme().Background).Children(func() {
			Card(c, "电池", "battery-full", Green, func() {
				Hero(c, "48.2 W", "90 W", "系统功耗", 0)
				Row(c, "健康度", "99%", "", true)
				BarRow(c, "CPU", "62%", &half, Blue)
				Note(c, "macOS 在固件中执行充电上限。")
				Status(c, "triangle-alert", Orange, "其他风扇工具正在控制", "")
			})
			ChartPanel(c, "系统功耗", "11.2 W", "均值 10 W", "activity", Blue, points, nil, "")
		})
	}, 420, 520)
	f, _ := os.Create(os.Getenv("SNAPSHOT"))
	defer f.Close()
	_ = png.Encode(f, tt.Image())
}
