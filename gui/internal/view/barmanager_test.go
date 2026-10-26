package view

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/barmanager"
)

func managerTester(s *barmanager.ItemStore) *ui.Tester {
	state := &ManagerPageState{}
	return ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().Children(func() { MenuBarManagerPage(c, s, state, 760) })
	}, 800, 900)
}

func TestManagerPageOrderEditor(t *testing.T) {
	s := barmanager.NewPreview(barmanager.StoreSnapshot{
		Supported: true, Trusted: true, HasLayoutAccess: true, Error: barmanager.ErrDidNotMove,
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
	if !tt.HasText("Requires macOS 27 or later.") || tt.HasText("Layout file allowed") {
		t.Errorf("unsupported: %q", tt.Texts())
	}

	tt = managerTester(barmanager.NewPreview(barmanager.StoreSnapshot{Supported: true}))
	if !tt.HasText("Grant Accessibility Access…") {
		t.Errorf("untrusted: %q", tt.Texts())
	}

	tt = managerTester(barmanager.NewPreview(barmanager.StoreSnapshot{Supported: true, Trusted: true}))
	if !tt.HasText("Reading the menu bar…") || !tt.HasText("Allow Layout File…") {
		t.Errorf("scanning without the layout file: %q", tt.Texts())
	}
	if tt.HasText("Not in Menu Bar") {
		t.Error("no absent panel without absent items")
	}
}
