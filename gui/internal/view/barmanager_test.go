package view

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/barmanager"
)

func managerTester(s *barmanager.ItemStore) *ui.Tester {
	return trayManagerTester(s, barmanager.NewTrayPreview(barmanager.TraySnapshot{}))
}

func trayManagerTester(s *barmanager.ItemStore, tray *barmanager.TrayStore) *ui.Tester {
	state := &ManagerPageState{}
	return ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().Children(func() { MenuBarManagerPage(c, s, tray, state, 760) })
	}, 800, 1100)
}

func TestManagerPageOrderEditor(t *testing.T) {
	s := barmanager.NewPreview(barmanager.StoreSnapshot{
		Trusted: true, HasLayoutAccess: true, Error: barmanager.ErrDidNotMove,
		Items: []barmanager.Item{
			{ID: "w#0", Name: "WeChat", Section: barmanager.Overflow, LayoutKey: "w"},
			{ID: "a#0", Name: "FineTune", Section: barmanager.Shown, LayoutKey: "k", Reachable: true},
			{ID: "b#0", Name: "Stats", Detail: "CPU", Section: barmanager.Shown, Reachable: true},
			{ID: "c#0", Name: "QQ", Section: barmanager.Absent},
		},
	})
	tt := managerTester(s)
	for _, want := range []string{
		"Menu Bar Order", "Layout file allowed", "«", "9:41",
		"Drop on an icon to place it to its left", "Further right stays in sight longer",
		"The icon didn't move. Icons behind « can only be moved once the layout file is allowed.",
		"Not in Menu Bar", "1", "These apps' icons are switched off in System Settings › Menu Bar.", "Open System Settings…",
	} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	for _, gone := range []string{"Enable Menu Bar Manager", "Hide Again After", "Show Hidden Items", "Hidden Icons"} {
		if tt.HasText(gone) {
			t.Errorf("%q belonged to PowerfulTop's own hiding, which is gone", gone)
		}
	}
}

func TestManagerPageStates(t *testing.T) {
	tt := managerTester(barmanager.NewPreview(barmanager.StoreSnapshot{}))
	if !tt.HasText("Grant Accessibility Access…") || tt.HasText("Layout file allowed") {
		t.Errorf("untrusted: %q", tt.Texts())
	}

	tt = managerTester(barmanager.NewPreview(barmanager.StoreSnapshot{Trusted: true}))
	if !tt.HasText("Reading the menu bar…") || !tt.HasText("Allow Layout File…") {
		t.Errorf("scanning without the layout file: %q", tt.Texts())
	}
	if tt.HasText("Not in Menu Bar") {
		t.Error("no absent panel without absent items")
	}
}

func TestManagerPageTray(t *testing.T) {
	store := barmanager.NewPreview(barmanager.StoreSnapshot{
		Trusted: true, HasLayoutAccess: true,
		Items: []barmanager.Item{
			{ID: "s#0", Name: "Stats", BundleID: "eu.exelban.Stats", Section: barmanager.Shown, LayoutKey: "k", Reachable: true},
			// Switched off by the tray, which the absent panel must not blame on System Settings.
			{ID: "w#0", Name: "WeChat", BundleID: "com.tencent.xinWeChat", Section: barmanager.Absent},
		},
	})
	tray := barmanager.NewTrayPreview(barmanager.TraySnapshot{
		Enabled: true, Error: barmanager.ErrTrayUntracked, ErrorID: "x",
		Entries: []barmanager.TrayEntry{
			{ID: "com.tencent.xinWeChat", Name: "WeChat", Running: true},
			{ID: "com.apple.menuextra.wifi", Name: "Wi-Fi", NameZh: "Wi-Fi", Icon: "wifi", Running: true},
		},
	})
	tt := trayManagerTester(store, tray)
	for _, want := range []string{
		"Tray", "Allow Switch File…", "macOS hasn't listed this app under Allow in the Menu Bar yet.", "Refresh Menu Bar", "Menu Bar Order",
	} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	if tt.HasText("Not in Menu Bar") {
		t.Error("the tray's own apps aren't switched off by the user")
	}
	if tt.HasText("Drag icons here from the menu bar below, or right-click one there.") {
		t.Error("the empty hint shows only without entries")
	}

	tt = trayManagerTester(store, barmanager.NewTrayPreview(barmanager.TraySnapshot{HasAllowListAccess: true}))
	if tt.HasText("Drag icons here from the menu bar below, or right-click one there.") || tt.HasText("Switch file allowed") {
		t.Errorf("a disabled tray shows only its switch: %q", tt.Texts())
	}
	tt = trayManagerTester(store, barmanager.NewTrayPreview(barmanager.TraySnapshot{Enabled: true, HasAllowListAccess: true}))
	if !tt.HasText("Drag icons here from the menu bar below, or right-click one there.") || !tt.HasText("Switch file allowed") {
		t.Errorf("empty tray: %q", tt.Texts())
	}
}

func TestTrayFlyout(t *testing.T) {
	tray := barmanager.NewTrayPreview(barmanager.TraySnapshot{Enabled: true,
		Entries: []barmanager.TrayEntry{{ID: "a", Name: "Chat", Running: true}, {ID: "b", Name: "VPN"}},
	})
	var opened string
	state := &TrayFlyoutState{}
	tt := ui.NewTester(func(c *ui.Context) {
		TrayFlyout(c, tray, state, TrayFlyoutActions{Open: func(id string, _ ui.Rect) { opened = id }})
	}, TrayFlyoutWidth, 200)
	if !tt.HasText("Menu Bar Manager…") || tt.HasText("No icons in the tray") {
		t.Errorf("texts %q", tt.Texts())
	}
	if err := tt.Click("Chat"); err != nil || opened != "a" {
		t.Errorf("opened %q", opened)
	}

	empty := ui.NewTester(func(c *ui.Context) {
		TrayFlyout(c, barmanager.NewTrayPreview(barmanager.TraySnapshot{Enabled: true}), state, TrayFlyoutActions{})
	}, TrayFlyoutWidth, 200)
	if !empty.HasText("No icons in the tray") {
		t.Errorf("empty: %q", empty.Texts())
	}
}

func TestManagerPagePicksADisplay(t *testing.T) {
	s := barmanager.NewPreview(barmanager.StoreSnapshot{
		Trusted: true, HasLayoutAccess: true, Display: "P275MV MAX",
		Displays: []barmanager.Display{{Name: "Built-in Retina Display", Internal: true}, {Name: "P275MV MAX"}},
		Items:    []barmanager.Item{{ID: "a#0", Name: "FineTune", Section: barmanager.Shown, Reachable: true}},
	})
	tt := managerTester(s)
	if !tt.HasText("Built-in Retina Display") || !tt.HasText("P275MV MAX") {
		t.Errorf("both displays offered: %q", tt.Texts())
	}
	one := managerTester(barmanager.NewPreview(barmanager.StoreSnapshot{
		Trusted: true, Displays: []barmanager.Display{{Name: "Built-in Retina Display"}},
		Items: []barmanager.Item{{ID: "a#0", Name: "FineTune", Section: barmanager.Shown}},
	}))
	if one.HasText("Built-in Retina Display") {
		t.Error("nothing to pick with one display")
	}
}

func TestManagerPageFlagsIconsSwitchedOnOutsideTheTray(t *testing.T) {
	store := barmanager.NewPreview(barmanager.StoreSnapshot{Trusted: true})
	tray := barmanager.NewTrayPreview(barmanager.TraySnapshot{Enabled: true, HasAllowListAccess: true,
		Entries: []barmanager.TrayEntry{{ID: "com.tencent.qq", Name: "QQ", Running: true, SwitchedOn: true}}})
	tt := trayManagerTester(store, tray)
	if !tt.HasText("Some icons were switched back on in System Settings. Right-click one to hide it again or put it back.") {
		t.Errorf("texts %q", tt.Texts())
	}
}
