package barmanager

import "testing"

func TestDropPoints(t *testing.T) {
	stats := Rect{1176, -1076.5, 33, 24}
	if p := DropPointLeftOf(stats); p != (Point{1168, -1064.5}) {
		t.Errorf("left drop lands just left of the item: %v", p)
	}
	if p := DropPointRightOf(stats); p != (Point{1217, -1064.5}) {
		t.Errorf("right drop lands just right of the item: %v", p)
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
	existing := []float64{0, 512, 550, 805, 832, 928, 1000}
	if got := PositionLeftOf(928, existing); got != 964 {
		t.Errorf("lands between the anchor and the next icon left of it: %v", got)
	}
	if got := PositionLeftOf(1000, existing); got != 1016 {
		t.Errorf("leftmost anchor extends past the end: %v", got)
	}
	if got := PositionRightOf(805, existing); got != 677.5 {
		t.Errorf("lands between the anchor and the next icon right of it: %v", got)
	}
	if got := PositionRightOf(0, existing); got != 0 {
		t.Errorf("never right of the clock: %v", got)
	}
}
