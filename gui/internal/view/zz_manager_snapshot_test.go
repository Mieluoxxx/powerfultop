package view

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/barmanager"
)

// Renders the menu bar manager with real app logos when MANAGER_SNAPSHOT names a PNG to write.
func TestManagerSnapshot(t *testing.T) {
	path := os.Getenv("MANAGER_SNAPSHOT")
	if path == "" {
		t.Skip()
	}
	app := func(id, name, section string, detail string) barmanager.Item {
		s := map[string]barmanager.Section{"shown": barmanager.Shown, "overflow": barmanager.Overflow, "absent": barmanager.Absent}[section]
		return barmanager.Item{ID: id, Name: name, Detail: detail, AppPath: "/Applications/" + name + ".app", Section: s, LayoutKey: id, Reachable: s == barmanager.Shown}
	}
	items := []barmanager.Item{
		app("1", "Stats", "shown", "CPU"), app("2", "Stats", "shown", "RAM"), app("3", "ClashMac", "shown", ""),
		app("4", "FineTune", "shown", ""), app("5", "AlDente", "shown", ""), app("6", "OrbStack", "shown", ""),
		app("7", "WeChat", "overflow", ""), app("8", "ego lite", "overflow", ""), app("9", "QQ", "absent", ""),
		app("10", "wpsoffice", "absent", ""),
	}
	s := barmanager.NewPreview(barmanager.StoreSnapshot{Supported: true, Trusted: true, HasLayoutAccess: true, Items: items})
	// Logos come as PNGs named after the app, since reading them from AppKit needs the app's run loop.
	for _, it := range items {
		if data, err := os.ReadFile(filepath.Join(os.Getenv("MANAGER_ICONS"), it.Name+".png")); err == nil {
			s.SetPreviewIcon(it.AppPath, data)
		}
	}
	tray := barmanager.NewTrayPreview(barmanager.TraySnapshot{Supported: true, Enabled: true, HasAllowListAccess: true,
		Entries: []barmanager.TrayEntry{
			{ID: "com.tencent.qq", Name: "QQ", AppPath: "/Applications/QQ.app", Running: true},
			{ID: "com.apple.menuextra.wifi", Name: "Wi-Fi", NameZh: "Wi-Fi", Icon: "wifi", Running: true},
			{ID: "com.apple.menuextra.sound", Name: "Sound", NameZh: "声音", Icon: "volume-2", Running: true},
		}})
	if data, err := os.ReadFile(filepath.Join(os.Getenv("MANAGER_ICONS"), "QQ.png")); err == nil {
		tray.SetPreviewIcon("/Applications/QQ.app", data)
	}
	state := &ManagerPageState{}
	for _, dark := range []bool{false, true} {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Fill().Padding(24).Background(c.Theme().Background).Children(func() {
				MenuBarManagerPage(c, s, tray, state, 760)
			})
		}, 820, 1250)
		tt.SetDark(dark)
		tt.Frame()
		name := path
		if dark {
			name = path[:len(path)-4] + "-dark.png"
		}
		f, _ := os.Create(name)
		_ = png.Encode(f, tt.Image())
		f.Close()
	}
}
