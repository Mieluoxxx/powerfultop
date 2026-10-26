package barmanager

import (
	"math"
	"sort"
	"strconv"
)

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
}

func (i Item) CanMove() bool {
	return i.Section != Absent && (i.LayoutKey != "" || i.Reachable)
}

type appRef struct {
	PID      int32
	Name     string
	BundleID string
	Path     string
	// Owners are every name macOS may file the app's layout keys under.
	Owners []string
}

type rawItem struct {
	PID        int32
	Index      int
	Title      string
	Help       string
	Identifier string
	Frame      Rect
}

// barAnchors are what MenuBarAgent's own items say about the bar being read.
type barAnchors struct {
	// overflow is the system « button's frame while the bar overflows.
	overflow *Rect
	// row is a frame on the bar's row; items parked elsewhere aren't in the menu bar.
	row *Rect
}

// ownItem is PowerfulTop's own status item: earlier versions left other keys of this app in the
// layout file, and only the one with this autosave name is live.
type ownItem struct {
	pid  int32
	name string
}

// classify turns what Accessibility and the layout file report into the editor's list, left to
// right. positions is nil without layout-file access.
func classify(apps []appRef, raw []rawItem, bar barAnchors, positions map[string]float64, own ownItem) []Item {
	onRow := func(f Rect) bool {
		// macOS 27 parks items it isn't drawing at a fixed spot off the bar's row, e.g. (-1, 970).
		return bar.row == nil || math.Abs(f.MidY()-bar.row.MidY()) <= bar.row.H/2
	}
	// Items behind « report stacked placeholder frames, so only those right of it are real.
	reachable := func(f Rect) bool {
		return onRow(f) && (bar.overflow == nil || f.MidX() > bar.overflow.MaxX())
	}

	appsByPID := map[int32]appRef{}
	for _, a := range apps {
		appsByPID[a.PID] = a
	}
	type entry struct {
		key, owner, name string
		position         float64
	}
	var parsed []entry
	for key, value := range positions {
		if owner, name, ok := ParseKey(key); ok {
			parsed = append(parsed, entry{key, owner, name, value})
		}
	}

	byPID := map[int32][]rawItem{}
	var pids []int32
	for _, r := range raw {
		if _, seen := byPID[r.PID]; !seen {
			pids = append(pids, r.PID)
		}
		byPID[r.PID] = append(byPID[r.PID], r)
	}

	var items []Item
	for _, pid := range pids {
		app, ok := appsByPID[pid]
		if !ok {
			continue
		}
		live := byPID[pid]
		var entries []entry
		for _, e := range parsed {
			if contains(app.Owners, e.owner) && (pid != own.pid || e.name == own.name) {
				entries = append(entries, e)
			}
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].position > entries[j].position })
		// Keys can only be paired with items when the counts agree, and for several items only
		// when their frames give the left-to-right order to pair the positions with.
		keys := map[int]entry{}
		allReachable := true
		for _, r := range live {
			allReachable = allReachable && reachable(r.Frame)
		}
		if len(entries) == len(live) && (len(live) == 1 || allReachable) {
			ordered := append([]rawItem{}, live...)
			sort.Slice(ordered, func(i, j int) bool { return ordered[i].Frame.X < ordered[j].Frame.X })
			for i, r := range ordered {
				keys[r.Index] = entries[i]
			}
		}
		for _, r := range live {
			e, keyed := keys[r.Index]
			section := Shown
			switch {
			case !onRow(r.Frame):
				section = Absent
			case !reachable(r.Frame):
				section = Overflow
			}
			id := app.BundleID
			if id == "" {
				id = strconv.Itoa(int(pid))
			}
			item := Item{
				ID:        id + "#" + strconv.Itoa(r.Index),
				Name:      app.Name,
				BundleID:  app.BundleID,
				AppPath:   app.Path,
				Frame:     r.Frame,
				Section:   section,
				Reachable: reachable(r.Frame),
			}
			if len(live) > 1 {
				item.Detail = firstNonEmpty(r.Help, r.Title, strconv.Itoa(r.Index+1))
			}
			if keyed {
				item.LayoutKey, item.Position, item.HasPosition = e.key, e.position, true
			}
			items = append(items, item)
		}
	}
	return sortItems(items)
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

func withoutAgent(apps []appRef) []appRef {
	out := apps[:0:0]
	for _, a := range apps {
		if a.BundleID != menuBarAgentID {
			out = append(out, a)
		}
	}
	return out
}

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
