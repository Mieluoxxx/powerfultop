package barmanager

import "testing"

var testApps = []appRef{
	{PID: 1, Name: "PowerfulTop", BundleID: "io.github.mieluoxxx.PowerfulTop", Owners: []string{"io.github.mieluoxxx.PowerfulTop", "PowerfulTop"}},
	{PID: 2, Name: "FineTune", BundleID: "com.finetuneapp.FineTune", Owners: []string{"com.finetuneapp.FineTune", "FineTune"}},
	{PID: 3, Name: "Stats", BundleID: "eu.exelban.Stats", Owners: []string{"eu.exelban.Stats", "Stats"}},
	{PID: 4, Name: "QQ", BundleID: "com.tencent.qq", Owners: []string{"com.tencent.qq", "QQ"}},
}

var externalFrame = Rect{-184, -1080, 1920, 30}

// The external bar measured on macOS 27.0.1, with room for everything; QQ is switched off.
func wideBars() []agentBar {
	return []agentBar{{Frame: externalFrame, Slots: []agentSlot{
		{"io.github.mieluoxxx.PowerfulTop", Rect{913, -1076.5, 218.5, 24}},
		{"com.finetuneapp.FineTune", Rect{1130, -1076.5, 40, 24}},
		{"eu.exelban.Stats", Rect{1176, -1076.5, 33, 24}},
		{"eu.exelban.Stats", Rect{1223, -1076.5, 33, 24}},
		{"com.apple.menuextra.wifi", Rect{1503, -1076.5, 22, 24}},
	}}}
}

var wideBar = barAnchors{row: &Rect{1600, -1076.5, 30, 24}}

var testOwn = ownItem{1, []string{"PowerfulTop.status"}}

func filePositions() map[string]float64 {
	return map[string]float64{
		"status:io.github.mieluoxxx.PowerfulTop::PowerfulTop.status": 700,
		// Left by an earlier version; only the live name counts.
		"status:PowerfulTop::PowerfulTop.MenuBarSection.separator": 928,
		"status:com.finetuneapp.FineTune::Item-0":                  500,
		"status:eu.exelban.Stats::CPU_mini":                        465,
		"status:eu.exelban.Stats::RAM_mini":                        418,
		"status:com.tencent.qq::Item-0":                            588,
		"module:WiFi":                                              159,
	}
}

func sections(items []Item) map[string]Section {
	got := map[string]Section{}
	for _, it := range items {
		got[it.ID] = it.Section
	}
	return got
}

func byID(items []Item) map[string]Item {
	out := map[string]Item{}
	for _, it := range items {
		out[it.ID] = it
	}
	return out
}

func TestModelWideBar(t *testing.T) {
	items := buildItems(modelInput{apps: testApps, bars: wideBars(), positions: filePositions(), off: map[string]bool{"com.tencent.qq": true}, own: testOwn}, externalFrame)
	want := map[string]Section{
		"io.github.mieluoxxx.PowerfulTop#PowerfulTop.status": Shown, "com.finetuneapp.FineTune#Item-0": Shown,
		"eu.exelban.Stats#CPU_mini": Shown, "eu.exelban.Stats#RAM_mini": Shown, "com.apple.menuextra.wifi#WiFi": Shown,
		"com.tencent.qq#Item-0": Absent,
	}
	if got := sections(items); len(got) != len(want) {
		t.Fatalf("items %v", got)
	}
	got := byID(items)
	for id, s := range want {
		if got[id].Section != s {
			t.Errorf("%s: got %v, want %v", id, got[id].Section, s)
		}
	}
	if it := got["io.github.mieluoxxx.PowerfulTop#PowerfulTop.status"]; it.LayoutKey != "status:io.github.mieluoxxx.PowerfulTop::PowerfulTop.status" || !it.Reachable {
		t.Errorf("own status item matches its live key: %+v", it)
	}
	if it := got["eu.exelban.Stats#RAM_mini"]; it.LayoutKey != "status:eu.exelban.Stats::RAM_mini" || it.Detail != "RAM_mini" {
		t.Errorf("several keys pair with items by order and name them: %+v", it)
	}
	if it := got["com.apple.menuextra.wifi#WiFi"]; it.LayoutKey != "module:WiFi" || it.Icon != "wifi" {
		t.Errorf("Apple's items come from MenuBarAgent: %+v", it)
	}
	if it := got["com.tencent.qq#Item-0"]; it.CanMove() || it.LayoutKey != "status:com.tencent.qq::Item-0" {
		t.Errorf("a switched-off app is absent, wherever it last was: %+v", it)
	}
	var order []string
	for _, it := range items {
		order = append(order, it.ID)
	}
	if order[0] != "io.github.mieluoxxx.PowerfulTop#PowerfulTop.status" || order[len(order)-1] != "com.tencent.qq#Item-0" {
		t.Errorf("left to right, absent last: %v", order)
	}
}

// Measured with two displays at once: every display's bar is MenuBarAgent's own window, read
// whichever display is active. On the built-in one « holds Passwords and WPS, stacked where it is.
func TestModelShowsEachDisplaysOwnBar(t *testing.T) {
	apps := []appRef{
		{PID: 1, Name: "PowerfulTop", BundleID: "io.github.mieluoxxx.PowerfulTop", Owners: []string{"io.github.mieluoxxx.PowerfulTop", "PowerfulTop"}},
		{PID: 5, Name: "Passwords", BundleID: "com.apple.Passwords.MenuBarExtra", Owners: []string{"com.apple.Passwords.MenuBarExtra"}},
		{PID: 6, Name: "WPS Office Service", BundleID: "cn.wps.wpscloudsvr", Owners: []string{"cn.wps.wpscloudsvr"}},
		{PID: 2, Name: "FineTune", BundleID: "com.finetuneapp.FineTune", Owners: []string{"com.finetuneapp.FineTune"}},
	}
	builtIn := Rect{0, 0, 1512, 33}
	bars := []agentBar{
		{Frame: externalFrame, Slots: []agentSlot{
			{"com.apple.Passwords.MenuBarExtra", Rect{1068, -1076.5, 39, 24}},
			{"cn.wps.wpscloudsvr", Rect{1105, -1076.5, 36, 24}},
			{"io.github.mieluoxxx.PowerfulTop", Rect{1139, -1076.5, 34, 24}},
			{"io.github.mieluoxxx.PowerfulTop", Rect{1171, -1076.5, 205, 24}},
			{"com.finetuneapp.FineTune", Rect{1375, -1076.5, 40, 24}},
		}},
		{Frame: builtIn, Slots: []agentSlot{
			{"com.apple.Passwords.MenuBarExtra", Rect{870, 4.5, 39, 24}},
			{"cn.wps.wpscloudsvr", Rect{873, 4.5, 36, 24}},
			{"io.github.mieluoxxx.PowerfulTop", Rect{915, 4.5, 34, 24}},
			{"io.github.mieluoxxx.PowerfulTop", Rect{947, 4.5, 205, 24}},
			{"com.finetuneapp.FineTune", Rect{1151, 4.5, 40, 24}},
		}},
	}
	own := ownItem{1, []string{"PowerfulTop.status", "PowerfulTop.overflow"}}
	for _, active := range []*Rect{nil, {890, 1, 17.5, 30}} {
		in := modelInput{apps: apps, bars: bars, own: own, overflow: active}
		external := sections(buildItems(in, externalFrame))
		internal := sections(buildItems(in, builtIn))
		for _, id := range []string{"com.apple.Passwords.MenuBarExtra#0", "cn.wps.wpscloudsvr#0"} {
			if external[id] != Shown || internal[id] != Overflow {
				t.Errorf("active «%v: %s external %v built-in %v", active, id, external[id], internal[id])
			}
		}
		for _, id := range []string{"io.github.mieluoxxx.PowerfulTop#0", "io.github.mieluoxxx.PowerfulTop#1", "com.finetuneapp.FineTune#0"} {
			if external[id] != Shown || internal[id] != Shown {
				t.Errorf("active «%v: %s external %v built-in %v", active, id, external[id], internal[id])
			}
		}
	}
}

// One item behind « isn't stacked on another; the active display's « button tells.
func TestModelOverflowLeftOfTheActiveDisplaysChevron(t *testing.T) {
	bars := []agentBar{{Frame: Rect{0, 0, 1512, 33}, Slots: []agentSlot{
		{"com.finetuneapp.FineTune", Rect{877, 4.5, 17.5, 24}},
		{"io.github.mieluoxxx.PowerfulTop", Rect{900, 4.5, 218.5, 24}},
	}}}
	got := sections(buildItems(modelInput{apps: testApps, bars: bars, own: testOwn, overflow: &Rect{877, 1, 17.5, 30}}, bars[0].Frame))
	if got["com.finetuneapp.FineTune#0"] != Overflow || got["io.github.mieluoxxx.PowerfulTop#0"] != Shown {
		t.Errorf("sections %v", got)
	}
	// The « of another display says nothing about this one.
	got = sections(buildItems(modelInput{apps: testApps, bars: bars, own: testOwn, overflow: &Rect{877, -1077, 17.5, 30}}, bars[0].Frame))
	if got["com.finetuneapp.FineTune#0"] != Shown {
		t.Errorf("sections %v", got)
	}
}

func TestModelItemOnlyOnAnotherDisplayIsHeldByThisOnesChevron(t *testing.T) {
	bars := append(wideBars(), agentBar{Frame: Rect{0, 0, 1512, 33}, Slots: []agentSlot{
		{"io.github.mieluoxxx.PowerfulTop", Rect{900, 4.5, 218.5, 24}},
	}})
	got := byID(buildItems(modelInput{apps: testApps, bars: bars, own: testOwn}, bars[1].Frame))
	if it := got["com.finetuneapp.FineTune#0"]; it.Section != Overflow || it.Reachable {
		t.Errorf("%+v", it)
	}
	if it := got["eu.exelban.Stats#1"]; it.Section != Overflow {
		t.Errorf("an app keeps as many items as the busiest bar draws: %+v", it)
	}
}

func TestModelLeavesStaleKeysOut(t *testing.T) {
	positions := filePositions()
	// Stats renamed its items once; the old keys stay in the file.
	positions["status:eu.exelban.Stats::CPU"] = 554
	items := buildItems(modelInput{apps: testApps, bars: wideBars(), positions: positions, own: testOwn}, externalFrame)
	got := byID(items)
	if _, ok := got["eu.exelban.Stats#2"]; ok {
		t.Error("only what MenuBarAgent draws is an item")
	}
	if it := got["eu.exelban.Stats#0"]; it.LayoutKey != "" || !it.CanMove() {
		t.Errorf("keys that can't be paired aren't guessed at; the item still moves by dragging: %+v", it)
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
