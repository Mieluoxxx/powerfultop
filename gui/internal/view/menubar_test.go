//go:build darwin

package view

import (
	"image/png"
	"os"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/macos"
	"powerfultop/internal/menubar"
)

func menuBarTestSettings(t *testing.T) *menubar.Settings {
	p := macos.DomainPreferences("io.github.mieluoxxx.PowerfulTop.gotest.menubarpage")
	_ = p.Set("menuBarConfiguration", nil)
	t.Cleanup(func() { _ = p.Set("menuBarConfiguration", nil) })
	s := menubar.NewSettings(p)
	s.Set(menubar.Configuration{IconStyle: menubar.IconMacOS, Items: []menubar.Item{menubar.ItemBatteryPercent, menubar.ItemSystemPower}, Spacing: 6, UpdateInterval: 2})
	return s
}

func menuBarTestContext() menubar.Context {
	cpu := 0.12
	return menubar.Context{DataAvailable: true, BatteryPercent: 64, SystemPowerW: 11.4, CPUUsage: &cpu}
}

func TestMenuBarPage(t *testing.T) {
	settings := menuBarTestSettings(t)
	state := &MenuBarPageState{}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().Padding(16).Children(func() {
			MenuBarPage(c, settings, menuBarTestContext(), state, 760)
		})
	}, 800, 1100)
	for _, want := range []string{"Selected Items", "64%", "11W", "Item Catalog", "Main Icon", "macOS Native", "Fan Speed", "Layout", "6 pt", "2 s"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}

	if err := tt.Click("CPU Usage"); err != nil {
		t.Fatal(err)
	}
	if !settings.Configuration().Contains(menubar.ItemCPUUsage) || !tt.HasText("12%") {
		t.Errorf("a catalog chip selects its item with a live value: %+v", settings.Configuration())
	}
	if err := tt.Click("Ring"); err != nil {
		t.Fatal(err)
	}
	if settings.Configuration().IconStyle != menubar.IconRing {
		t.Errorf("a style chip picks the style: %+v", settings.Configuration())
	}

	// Drag the CPU chip onto the percentage chip: it moves before it.
	from, _ := tt.Find("12%")
	to, _ := tt.Find("64%")
	tt.Press(from.X+from.W/2, from.Y+from.H/2)
	for i := 1; i <= 8; i++ {
		f := float32(i) / 8
		tt.Move(from.X+from.W/2+(to.X+2-from.X-from.W/2)*f, from.Y+from.H/2)
	}
	tt.Release(to.X+2, to.Y+to.H/2)
	if got := settings.Configuration().Items; !slices.Equal(got, []menubar.Item{menubar.ItemCPUUsage, menubar.ItemBatteryPercent, menubar.ItemSystemPower}) {
		t.Errorf("dragging reorders: %v", got)
	}

	if err := tt.Click("Clear All"); err != nil {
		t.Fatal(err)
	}
	if len(settings.Configuration().Items) != 0 {
		t.Error("Clear All empties the selection")
	}
	if err := tt.Click("Reset"); err != nil {
		t.Fatal(err)
	}
	if !settings.Configuration().Equal(menubar.DefaultConfiguration()) {
		t.Errorf("Reset restores the default: %+v", settings.Configuration())
	}

	if dir := os.Getenv("SNAPSHOT"); dir != "" {
		tt.SetScale(2)
		f, _ := os.Create(dir + "/menubar-page.png")
		_ = png.Encode(f, tt.Image())
		f.Close()
	}
}

func TestMenuBarChipRemove(t *testing.T) {
	settings := menuBarTestSettings(t)
	tt := ui.NewTester(func(c *ui.Context) {
		MenuBarPage(c, settings, menuBarTestContext(), &MenuBarPageState{}, 760)
	}, 800, 1100)
	if err := tt.Click("Remove"); err != nil {
		t.Fatal(err)
	}
	if got := settings.Configuration().Items; !slices.Equal(got, []menubar.Item{menubar.ItemSystemPower}) {
		t.Errorf("the first chip's remove button removes it: %v", got)
	}
}
