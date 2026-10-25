package view

import (
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/barmanager"
)

func managerTester(m *barmanager.Manager, s *barmanager.ItemStore) *ui.Tester {
	state := &ManagerPageState{}
	return ui.NewTester(func(c *ui.Context) {
		ui.Scroll(c).Fill().Children(func() { MenuBarManagerPage(c, m, s, state, 600) })
	}, 640, 1400)
}

func TestManagerPageLayoutEditor(t *testing.T) {
	m, s := barmanager.NewPreview(true, true, true, barmanager.StoreSnapshot{
		Trusted: true, HasLayoutAccess: true, Error: barmanager.ErrDidNotMove,
		Items: []barmanager.Item{
			{ID: "a#0", Name: "FineTune", Section: barmanager.Shown, LayoutKey: "k"},
			{ID: "b#0", Name: "Stats", Detail: "CPU", Section: barmanager.Hidden, Reachable: true},
			{ID: "c#0", Name: "QQ", Section: barmanager.Absent},
		},
	})
	tt := managerTester(m, s)
	for _, want := range []string{
		"Enable Menu Bar Manager", "Icons Hidden", "Hide Again After", "Show Hidden Items",
		"Menu Bar Layout", "Shown Icons", "Hidden Icons", "Not in Menu Bar",
		"Using the menu bar layout file: icons move without the pointer, even behind the notch.",
		"The icon didn't move. It may be covered by the notch or the « overflow; try again on a wider display.",
		"How It Works", "Open System Settings…",
	} {
		if !tt.HasText(want) {
			t.Errorf("missing %q", want)
		}
	}
	if err := tt.Click("Show Hidden Items"); err != nil {
		t.Fatal(err)
	}
	if m.Snapshot().Collapsed {
		t.Error("the button expands the bar")
	}
}

func TestManagerPageStates(t *testing.T) {
	m, s := barmanager.NewPreview(false, false, false, barmanager.StoreSnapshot{})
	tt := managerTester(m, s)
	if !tt.HasText("Requires macOS 27 or later.") || tt.HasText("Menu Bar Layout") {
		t.Errorf("unsupported: %q", tt.Texts())
	}

	m, s = barmanager.NewPreview(true, true, false, barmanager.StoreSnapshot{Trusted: false})
	tt = managerTester(m, s)
	if !tt.HasText("Grant Accessibility Access…") || !tt.HasText("Hide Items") {
		t.Errorf("untrusted: %q", tt.Texts())
	}

	m, s = barmanager.NewPreview(true, true, false, barmanager.StoreSnapshot{Trusted: true, Crowded: true})
	tt = managerTester(m, s)
	if !tt.HasText("The active menu bar is too crowded to show the separator.") || !tt.HasText("Allow…") {
		t.Errorf("crowded: %q", tt.Texts())
	}
	if tt.HasText("Shown Icons") {
		t.Error("no rows while crowded")
	}
}

func TestManagerAutoHideLabels(t *testing.T) {
	cases := map[time.Duration]string{0: "Never", 5 * time.Second: "5 seconds", time.Minute: "1 minute"}
	for d, want := range cases {
		if got := managerAutoHideLabel(d); got != want {
			t.Errorf("%v: %q", d, got)
		}
	}
}

func TestManagerPageSnapshot(t *testing.T) {
	path := os.Getenv("MANAGER_SNAPSHOT")
	if path == "" {
		t.Skip("set MANAGER_SNAPSHOT to a PNG path to render the page")
	}
	m, s := barmanager.NewPreview(true, true, false, barmanager.StoreSnapshot{
		Trusted: true, HasLayoutAccess: true,
		Items: []barmanager.Item{
			{ID: "a#0", Name: "FineTune", Section: barmanager.Shown, LayoutKey: "k"},
			{ID: "b#0", Name: "Stats", Section: barmanager.Shown, Reachable: true},
			{ID: "c#0", Name: "QQ", Section: barmanager.Absent},
		},
	})
	state := &ManagerPageState{}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(20).Background(c.Theme().Background).Children(func() {
			MenuBarManagerPage(c, m, s, state, 560)
		})
	}, 620, 1100)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
