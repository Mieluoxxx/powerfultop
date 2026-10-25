package barmanager

import "testing"

const own = int32(1)

var testApps = []appRef{
	{PID: own, Name: "PowerfulTop", Owners: []string{"PowerfulTop"}},
	{PID: 2, Name: "FineTune", BundleID: "com.finetuneapp.FineTune", Owners: []string{"com.finetuneapp.FineTune", "FineTune"}},
	{PID: 3, Name: "Stats", BundleID: "eu.exelban.Stats", Owners: []string{"eu.exelban.Stats", "Stats"}},
	{PID: 4, Name: "QQ", BundleID: "com.tencent.qq", Owners: []string{"com.tencent.qq", "QQ"}},
}

// The expanded external bar measured on macOS 27.0.1.
func expandedRaw() []rawItem {
	return []rawItem{
		{PID: own, Identifier: SeparatorIdentifier, Frame: Rect{728, -1076.5, 26, 24}},
		{PID: own, Index: 1, Identifier: SpacerIdentifier, Frame: Rect{760, -1076.5, 2, 24}},
		{PID: own, Index: 2, Identifier: ToggleIdentifier, Frame: Rect{848, -1076.5, 28.5, 24}},
		{PID: own, Index: 3, Title: "85% 10W", Frame: Rect{913, -1076.5, 218.5, 24}},
		{PID: 2, Frame: Rect{1130, -1076.5, 40, 24}},
		{PID: 3, Help: "CPU", Frame: Rect{1176, -1076.5, 33, 24}},
		{PID: 3, Index: 1, Help: "RAM", Frame: Rect{1223, -1076.5, 33, 24}},
		{PID: 4, Frame: Rect{-1, 970, 34, 24}},
	}
}

func TestClassifyFromFrames(t *testing.T) {
	r := classify(testApps, expandedRaw(), nil, nil, own)
	if r.crowded || r.frames == nil || r.file != nil {
		t.Fatalf("anchors: %+v", r)
	}
	got := map[string]Section{}
	for _, it := range r.items {
		got[it.ID] = it.Section
		if it.ID == "eu.exelban.Stats#0" && it.Detail != "CPU" {
			t.Errorf("several items of one app are told apart by their help: %q", it.Detail)
		}
	}
	want := map[string]Section{"1#3": Shown, "com.finetuneapp.FineTune#0": Shown, "eu.exelban.Stats#0": Shown, "eu.exelban.Stats#1": Shown, "com.tencent.qq#0": Absent}
	for id, s := range want {
		if got[id] != s {
			t.Errorf("%s: got %v, want %v", id, got[id], s)
		}
	}
	if len(r.items) != len(want) {
		t.Errorf("own spacers and anchors are not listed: %d items", len(r.items))
	}
}

func TestClassifyFromLayoutFile(t *testing.T) {
	positions := map[string]float64{
		"status:PowerfulTop::PowerfulTop.MenuBarSection.separator": 928,
		"status:PowerfulTop::PowerfulTop.MenuBarSection.spacer0":   832,
		"status:PowerfulTop::PowerfulTop.MenuBarSection.toggle":    805,
		"status:PowerfulTop::Item-1":                               550,
		"status:com.finetuneapp.FineTune::Item-0":                  941.5,
		"status:eu.exelban.Stats::CPU_mini":                        465,
		"status:eu.exelban.Stats::RAM_mini":                        418,
		"status:eu.exelban.Stats::CPU":                             554,
		"status:com.tencent.qq::Item-0":                            588,
	}
	// The built-in bar: everything up to the toggle sits behind « at 877...894.5.
	raw := expandedRaw()
	raw[0].Frame, raw[1].Frame, raw[2].Frame = Rect{512, 4.5, 435, 24}, Rect{536, 4.5, 2, 24}, Rect{866, 4.5, 28.5, 24}
	raw[3].Frame = Rect{676, 4.5, 218.5, 24}
	raw[4].Frame = Rect{855, 4.5, 40, 24}
	raw[5].Frame, raw[6].Frame = Rect{947, 4.5, 33, 24}, Rect{994, 4.5, 33, 24}
	overflow := &Rect{877, 1, 17.5, 30}
	r := classify(testApps, raw, overflow, positions, own)
	if r.crowded || r.file == nil || r.frames != nil {
		t.Fatalf("file anchors should stand in for unusable frames: %+v", r)
	}
	byID := map[string]Item{}
	for _, it := range r.items {
		byID[it.ID] = it
	}
	if it := byID["com.finetuneapp.FineTune#0"]; it.Section != Hidden || it.LayoutKey == "" || !it.CanMove() {
		t.Errorf("file position decides even behind «: %+v", it)
	}
	if it := byID["eu.exelban.Stats#0"]; it.Section != Shown || it.LayoutKey != "" {
		t.Errorf("3 keys for 2 items stay unmatched; visible right of « means shown: %+v", it)
	}
	if it := byID["1#3"]; it.Section != Shown || it.LayoutKey != "status:PowerfulTop::Item-1" {
		t.Errorf("own status item matches its key: %+v", it)
	}
}

func TestClassifyCrowded(t *testing.T) {
	raw := expandedRaw()
	raw[0].Frame, raw[2].Frame = Rect{866, 4.5, 26, 24}, Rect{870, 4.5, 28.5, 24}
	r := classify(testApps, raw, &Rect{877, 1, 17.5, 30}, nil, own)
	if !r.crowded || len(r.items) != 0 {
		t.Errorf("without the file, an overflowed separator leaves nothing to place: %+v", r)
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
