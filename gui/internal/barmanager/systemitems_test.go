package barmanager

import "testing"

func TestSwitchingCheckbox(t *testing.T) {
	cases := []struct {
		name          string
		shown         bool
		current       int
		has           bool
		saved         int
		hasSaved      bool
		want          checkboxWrite
		wantKeep      int
		wantKeepSaved bool
	}{
		{name: "hide shown when active", current: 2, has: true, want: checkboxWrite{value: 8}, wantKeep: 2, wantKeepSaved: true},
		{name: "hide default", want: checkboxWrite{value: 8}, wantKeep: missingValue, wantKeepSaved: true},
		{name: "hide hidden keeps what was saved", current: 8, has: true, saved: 18, hasSaved: true, want: checkboxWrite{keep: true}, wantKeep: 18, wantKeepSaved: true},
		{name: "show puts the saved choice back", shown: true, current: 8, has: true, saved: 18, hasSaved: true, want: checkboxWrite{value: 18}},
		{name: "show back to default", shown: true, current: 8, has: true, saved: missingValue, hasSaved: true, want: checkboxWrite{remove: true}},
		{name: "show hidden by the user before", shown: true, current: 8, has: true, want: checkboxWrite{value: 2}},
		{name: "shown meanwhile from System Settings", shown: true, current: 18, has: true, saved: 2, hasSaved: true, want: checkboxWrite{keep: true}},
	}
	for _, tc := range cases {
		w, keep, keepSaved := switchingCheckbox(tc.shown, tc.current, tc.has, tc.saved, tc.hasSaved)
		if w != tc.want || keep != tc.wantKeep || keepSaved != tc.wantKeepSaved {
			t.Errorf("%s: got %+v %d %v", tc.name, w, keep, keepSaved)
		}
	}
}

func TestSystemItemLookup(t *testing.T) {
	wifi, ok := systemItem("com.apple.menuextra.wifi")
	if !ok || wifi.PositionKey() != "module:WiFi" {
		t.Fatalf("wifi: %+v", wifi)
	}
	if s, ok := systemItemByPositionKey("module:Sound"); !ok || s.ID != "com.apple.menuextra.sound" {
		t.Errorf("sound: %+v", s)
	}
	if _, ok := systemItemByPositionKey(""); ok {
		t.Error("items without a key have no position")
	}
	if _, ok := systemItem("com.apple.menuextra.clock"); ok {
		t.Error("the clock has no switch")
	}
}
