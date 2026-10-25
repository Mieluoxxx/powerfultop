package barmanager

import "testing"

func TestCollapseGeometry(t *testing.T) {
	notched := NewDisplay(1512, 663.5)
	external := NewDisplay(1920, 0)
	if got := UnitLength([]Display{notched, external}); got != 433 {
		t.Errorf("notched display sets the unit under its cliff: %v", got)
	}
	if got := ActiveSpacers(433, []Display{notched, external}); got != 4 {
		t.Errorf("spacers cover the widest display: %v", got)
	}
	if 433*5 < external.Width {
		t.Error("separator plus spacers span the external bar")
	}
	wide := NewDisplay(2056, 0)
	if got := UnitLength([]Display{wide}); got != 964 {
		t.Errorf("unnotched cliff is half the width: %v", got)
	}
	if got := ActiveSpacers(964, []Display{wide}); got != 2 {
		t.Errorf("three units cover 2056 pt: %v", got)
	}
	if got := UnitLength([]Display{NewDisplay(300, 0)}); got != MinimumUnit {
		t.Errorf("unit never drops below the minimum: %v", got)
	}
	if got := UnitLength(nil); got != MinimumUnit {
		t.Errorf("no displays falls back to the minimum: %v", got)
	}
	if got := ActiveSpacers(433, nil); got != 0 {
		t.Errorf("no displays need no spacers: %v", got)
	}
	if got := ActiveSpacers(433, []Display{notched, NewDisplay(6016, 0)}); got != SpacerCount {
		t.Errorf("spacers are capped at the registered count: %v", got)
	}
}

func TestLayoutSections(t *testing.T) {
	// Frames measured on macOS 27.0.1 with the bar expanded.
	separator := Rect{723, -1076.5, 26, 24}
	toggle := Rect{843, -1076.5, 28.5, 24}
	cases := []struct {
		frame     Rect
		separator Rect
		want      Section
		name      string
	}{
		{Rect{870, -1076.5, 40, 24}, separator, Shown, "right of the chevron"},
		// After a hide the dropped item overlaps the separator, which slid right to make room.
		{Rect{723, -1076.5, 40, 24}, Rect{761, -1076.5, 26, 24}, Hidden, "overlapping the separator edge"},
		{Rect{300, -1076.5, 218, 24}, separator, Hidden, "left of the separator"},
		{Rect{-1, 970, 28, 24}, separator, Absent, "parked item"},
	}
	for _, c := range cases {
		if got := SectionOf(c.frame, c.separator, toggle); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	if p, _ := DropPoint(Shown, separator, toggle); p != (Point{875.5, -1064.5}) {
		t.Errorf("shown drop lands just right of the chevron: %v", p)
	}
	if p, _ := DropPoint(Hidden, separator, toggle); p != (Point{715, -1064.5}) {
		t.Errorf("hidden drop lands just left of the separator: %v", p)
	}
	if _, ok := DropPoint(Absent, separator, toggle); ok {
		t.Error("absent items have no drop point")
	}
}

func TestPreferredPositions(t *testing.T) {
	if owner, name, ok := ParseKey("status:com.tencent.qq::Item-0"); !ok || owner != "com.tencent.qq" || name != "Item-0" {
		t.Errorf("parse: %q %q %v", owner, name, ok)
	}
	if _, name, _ := ParseKey("status:eu.exelban.Stats::CPU_mini"); name != "CPU_mini" {
		t.Error("named items keep their autosave name")
	}
	if _, _, ok := ParseKey("module:Clock"); ok {
		t.Error("Apple modules aren't status items")
	}
	// Measured layout: toggle 805, spacers 832...912, separator 928.
	if SectionOfPosition(1000, 832) != Hidden || SectionOfPosition(512, 832) != Shown {
		t.Error("left of the block is hidden, right of it shown")
	}
	existing := []float64{0, 512, 550, 805, 832, 928, 1000}
	if got := PositionLeftOf(928, existing); got != 964 {
		t.Errorf("hide lands between separator and the next icon: %v", got)
	}
	if got := PositionLeftOf(1000, existing); got != 1016 {
		t.Errorf("leftmost anchor extends past the end: %v", got)
	}
	if got := PositionRightOf(805, existing); got != 677.5 {
		t.Errorf("show lands between chevron and the next icon: %v", got)
	}
	if got := PositionRightOf(0, existing); got != 0 {
		t.Errorf("never right of the clock: %v", got)
	}
}
