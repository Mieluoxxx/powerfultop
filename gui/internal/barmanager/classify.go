package barmanager

import "sort"

// Item is one app's menu bar item as the layout editor shows it.
type Item struct {
	ID       string
	Name     string
	BundleID string
	// AppPath locates the app's icon.
	AppPath string
	// Detail tells several items of one app apart.
	Detail  string
	Frame   Rect
	Section Section
	// LayoutKey is the item's entry in the macOS 27 layout file, when it could be matched unambiguously.
	LayoutKey   string
	Position    float64
	HasPosition bool
	// Reachable: on screen right now, so a ⌘-drag can reach it.
	Reachable bool
	// Icon is a lucide icon for Apple's items, which have no app logo.
	Icon string
}

func (i Item) CanMove() bool {
	return i.Section != Absent && (i.LayoutKey != "" || i.Reachable)
}

type appRef struct {
	PID      int32
	Name     string
	BundleID string
	Path     string
	// Executable is what the allow list names an ad-hoc signed app by.
	Executable string
	// Owners are every name macOS may file the app's layout keys under.
	Owners []string
}

// barAnchors are what MenuBarAgent's own extras menu bar says about the active display's bar.
type barAnchors struct {
	// overflow is the system « button's frame while the bar overflows.
	overflow *Rect
	// row is a frame on the bar's row, which tells which display is active.
	row *Rect
}

// ownItem is PowerfulTop's own status items: earlier versions left other keys of this app in the
// layout file, and only those with these autosave names are live.
type ownItem struct {
	pid   int32
	names []string
}

var sectionRank = map[Section]int{Overflow: 0, Shown: 1, Absent: 2}

// sortItems orders items left to right as in the bar: layout positions grow leftwards, and
// unmatched items borrow a position between their matched on-screen neighbours.
func sortItems(items []Item) []Item {
	var matched []Item
	for _, it := range items {
		if it.HasPosition && it.Reachable {
			matched = append(matched, it)
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].Frame.X < matched[j].Frame.X })
	sortKey := func(it Item) (float64, bool) {
		if it.HasPosition {
			return it.Position, true
		}
		if !it.Reachable {
			return 0, false
		}
		var left, right *float64
		for i := range matched {
			if matched[i].Frame.X < it.Frame.X {
				left = &matched[i].Position
			} else if right == nil && matched[i].Frame.X > it.Frame.X {
				right = &matched[i].Position
			}
		}
		switch {
		case left != nil && right != nil:
			return (*left + *right) / 2, true
		case left != nil:
			return *left - 0.5, true
		case right != nil:
			return *right + 0.5, true
		}
		return 0, false
	}
	sort.SliceStable(items, func(i, j int) bool {
		// Switched-off items keep their place in the file but aren't in any bar.
		if ai, aj := items[i].Section == Absent, items[j].Section == Absent; ai != aj {
			return aj
		}
		a, okA := sortKey(items[i])
		b, okB := sortKey(items[j])
		switch {
		case okA && okB:
			return a > b
		case okA != okB:
			return okA
		}
		// Without positions the section says more than a placeholder frame: « holds the leftmost items.
		if ra, rb := sectionRank[items[i].Section], sectionRank[items[j].Section]; ra != rb {
			return ra < rb
		}
		return items[i].Frame.X < items[j].Frame.X
	})
	return items
}

// menuBarAgentID owns Apple's own controls and the « button, which the editor doesn't manage.
const menuBarAgentID = "com.apple.MenuBarAgent"

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
