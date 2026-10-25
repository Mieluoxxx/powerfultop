package menubar

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"testing"
)

// SNAPSHOT=dir writes the labels over light and dark bars, to look at them.
func TestSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT")
	if dir == "" {
		t.Skip()
	}
	ctx := sampleContext()
	var rows []*image.RGBA
	for _, style := range IconStyles {
		for _, charging := range []bool{false, true} {
			ctx.OnAC = charging
			cfg := DefaultConfiguration()
			cfg.IconStyle = style
			cfg.Items = []Item{ItemBatteryPercent, ItemSystemPower, ItemCPUUsage, ItemFanSpeed}
			l := NewLabel(cfg, ctx)
			for _, dark := range []bool{false, true} {
				img := RenderLabelImage(l, cfg.Spacing, dark)
				bg := color.RGBA{236, 236, 236, 255}
				if dark {
					bg = color.RGBA{40, 40, 40, 255}
				}
				if l.IsTemplate() && dark {
					img = Tint(img, white)
				}
				row := image.NewRGBA(image.Rect(0, 0, 420, 44))
				draw.Draw(row, row.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
				draw.Draw(row, img.Bounds().Add(image.Pt(8, 4)), img, image.Point{}, draw.Over)
				rows = append(rows, row)
			}
		}
	}
	sheet := image.NewRGBA(image.Rect(0, 0, 420, 44*len(rows)))
	for i, r := range rows {
		draw.Draw(sheet, r.Bounds().Add(image.Pt(0, 44*i)), r, image.Point{}, draw.Src)
	}
	f, _ := os.Create(dir + "/labels.png")
	defer f.Close()
	_ = png.Encode(f, sheet)
}
