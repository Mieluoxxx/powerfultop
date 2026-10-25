package view

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestDetailsSwitchesPages(t *testing.T) {
	var saved PageID
	s := &DetailsState{Selected: PageOverview, Available: true, UpdatedAt: time.Date(2026, 10, 26, 1, 5, 8, 0, time.Local), OnSelect: func(id PageID) { saved = id }}
	pages := []WindowPage{
		{ID: PageOverview, Title: "Overview", Icon: "layout-grid", Body: func(c *ui.Context, _ float32) { ui.Text(c, "overview body") }},
		{ID: PageCooling, Title: "Cooling", Icon: "fan", Attention: true, Hint: "fan helper", Body: func(c *ui.Context, w float32) {
			if w <= 0 {
				t.Error("pages get a positive width")
			}
			ui.Text(c, "cooling body")
		}},
	}
	tt := ui.NewTester(func(c *ui.Context) { Details(c, pages, s) }, 1000, 720)
	if !tt.HasText("overview body") || !tt.HasText("Updated at 01:05:08") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("Cooling"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("cooling body") || s.Selected != PageCooling || saved != PageCooling {
		t.Errorf("selected %q saved %q texts %q", s.Selected, saved, tt.Texts())
	}
}

func TestDetailsWithoutBattery(t *testing.T) {
	s := &DetailsState{Selected: PageOverview}
	pages := []WindowPage{{ID: PageOverview, Title: "Overview", Icon: "layout-grid", Body: func(c *ui.Context, _ float32) { ui.Text(c, "body") }}}
	tt := ui.NewTester(func(c *ui.Context) { Details(c, pages, s) }, 1000, 720)
	if tt.HasText("body") || !tt.HasText("Built-in battery not detected. PowerfulTop requires a MacBook.") {
		t.Errorf("texts %q", tt.Texts())
	}
}
