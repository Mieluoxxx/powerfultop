package barmanager

import "testing"

var testApps = []appRef{
	{PID: 1, Name: "PowerfulTop", BundleID: "io.github.mieluoxxx.PowerfulTop", Owners: []string{"io.github.mieluoxxx.PowerfulTop", "PowerfulTop"}},
	{PID: 2, Name: "FineTune", BundleID: "com.finetuneapp.FineTune", Owners: []string{"com.finetuneapp.FineTune", "FineTune"}},
	{PID: 3, Name: "Stats", BundleID: "eu.exelban.Stats", Owners: []string{"eu.exelban.Stats", "Stats"}},
	{PID: 4, Name: "QQ", BundleID: "com.tencent.qq", Owners: []string{"com.tencent.qq", "QQ"}},
}

// The external bar measured on macOS 27.0.1, with room for everything.
func wideRaw() []rawItem {
	return []rawItem{
		{PID: 1, Title: "85% 10W", Frame: Rect{913, -1076.5, 218.5, 24}},
		{PID: 2, Frame: Rect{1130, -1076.5, 40, 24}},
		{PID: 3, Help: "CPU", Frame: Rect{1176, -1076.5, 33, 24}},
		{PID: 3, Index: 1, Help: "RAM", Frame: Rect{1223, -1076.5, 33, 24}},
		{PID: 4, Frame: Rect{-1, 970, 34, 24}},
	}
}

var wideBar = barAnchors{row: &Rect{1600, -1076.5, 30, 24}}

func sections(items []Item) map[string]Section {
	got := map[string]Section{}
	for _, it := range items {
		got[it.ID] = it.Section
	}
	return got
}

func TestClassifyWideBar(t *testing.T) {
	items := classify(testApps, wideRaw(), wideBar, nil, ownItem{1, "PowerfulTop.status"})
	want := map[string]Section{
		"io.github.mieluoxxx.PowerfulTop#0": Shown, "com.finetuneapp.FineTune#0": Shown,
		"eu.exelban.Stats#0": Shown, "eu.exelban.Stats#1": Shown, "com.tencent.qq#0": Absent,
	}
	got := sections(items)
	for id, s := range want {
		if got[id] != s {
			t.Errorf("%s: got %v, want %v", id, got[id], s)
		}
	}
	if items[0].ID != "io.github.mieluoxxx.PowerfulTop#0" || items[3].ID != "eu.exelban.Stats#1" {
		t.Errorf("items run left to right: %v", items)
	}
	for _, it := range items {
		if it.ID == "eu.exelban.Stats#0" && it.Detail != "CPU" {
			t.Errorf("several items of one app are told apart by their help: %q", it.Detail)
		}
	}
}

func TestClassifyOverflowWithLayoutFile(t *testing.T) {
	positions := map[string]float64{
		"status:PowerfulTop::PowerfulTop.MenuBarSection.separator":   928,
		"status:io.github.mieluoxxx.PowerfulTop::PowerfulTop.status": 550,
		"status:com.finetuneapp.FineTune::Item-0":                    941.5,
		"status:eu.exelban.Stats::CPU_mini":                          465,
		"status:eu.exelban.Stats::RAM_mini":                          418,
		"status:com.tencent.qq::Item-0":                              588,
	}
	// The built-in bar: FineTune didn't fit and sits behind « at 877...894.5.
	raw := wideRaw()
	raw[0].Frame = Rect{900, 4.5, 218.5, 24}
	raw[1].Frame = Rect{877, 4.5, 17.5, 24}
	raw[2].Frame, raw[3].Frame = Rect{1124, 4.5, 33, 24}, Rect{1171, 4.5, 33, 24}
	bar := barAnchors{overflow: &Rect{877, 1, 17.5, 30}, row: &Rect{1400, 4.5, 30, 24}}
	byID := map[string]Item{}
	for _, it := range classify(testApps, raw, bar, positions, ownItem{1, "PowerfulTop.status"}) {
		byID[it.ID] = it
	}
	if it := byID["com.finetuneapp.FineTune#0"]; it.Section != Overflow || it.Reachable || it.LayoutKey == "" || !it.CanMove() {
		t.Errorf("behind « is overflow, still movable through the file: %+v", it)
	}
	if it := byID["io.github.mieluoxxx.PowerfulTop#0"]; it.Section != Shown || it.LayoutKey != "status:io.github.mieluoxxx.PowerfulTop::PowerfulTop.status" {
		t.Errorf("own status item matches its live key, not keys earlier versions left: %+v", it)
	}
	if it := byID["eu.exelban.Stats#1"]; it.LayoutKey != "status:eu.exelban.Stats::RAM_mini" {
		t.Errorf("several keys pair with items by on-screen order: %+v", it)
	}
	if it := byID["com.tencent.qq#0"]; it.Section != Absent || it.CanMove() {
		t.Errorf("parked items are absent and can't move: %+v", it)
	}
}

func TestSortItemsInterpolatesUnmatched(t *testing.T) {
	items := sortItems([]Item{
		{ID: "stats", Frame: Rect{X: 1176}, Reachable: true},
		{ID: "aldente", Frame: Rect{X: 1399}, Position: 181, HasPosition: true, Reachable: true},
		{ID: "finetune", Frame: Rect{X: 1130}, Position: 512, HasPosition: true, Reachable: true},
		{ID: "hidden", Frame: Rect{X: 1}, Reachable: false},
	})
	var order []string
	for _, it := range items {
		order = append(order, it.ID)
	}
	if got := order; got[0] != "finetune" || got[1] != "stats" || got[2] != "aldente" || got[3] != "hidden" {
		t.Errorf("order %v", got)
	}
}
