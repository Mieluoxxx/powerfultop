package barmanager

import (
	"math"
	"sort"
	"strconv"
	"strings"
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

type fileAnchors struct{ separator, toggle float64 }

type frameAnchors struct{ separator, toggle Rect }

type scanResult struct {
	items   []Item
	crowded bool
	frames  *frameAnchors
	file    *fileAnchors
}

// classify turns what Accessibility and the layout file report into the editor's rows. overflow is
// the system « button's frame when the bar overflows; positions is nil without layout-file access.
func classify(apps []appRef, raw []rawItem, overflow *Rect, positions map[string]float64, ownPID int32) scanResult {
	// Items behind « report stacked placeholder frames, so only those right of it are real.
	reachable := func(f Rect) bool {
		if overflow == nil || math.Abs(f.MidY()-overflow.MidY()) >= overflow.H {
			return true
		}
		return f.MidX() > overflow.MaxX()
	}

	var separator, toggle *Rect
	for i := range raw {
		if raw[i].PID != ownPID {
			continue
		}
		switch raw[i].Identifier {
		case SeparatorIdentifier:
			separator = &raw[i].Frame
		case ToggleIdentifier:
			toggle = &raw[i].Frame
		}
	}
	if separator == nil || toggle == nil {
		return scanResult{}
	}
	var result scanResult
	framesUsable := reachable(*separator) && reachable(*toggle)
	if framesUsable {
		result.frames = &frameAnchors{*separator, *toggle}
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
	ownOwners := appsByPID[ownPID].Owners
	var blockEdge float64
	hasBlock := false
	var sepPos, togglePos *float64
	for _, e := range parsed {
		if !contains(ownOwners, e.owner) || !strings.HasPrefix(e.name, AutosavePrefix) {
			continue
		}
		e := e
		switch {
		case e.name == SeparatorIdentifier:
			sepPos = &e.position
		case e.name == ToggleIdentifier:
			togglePos = &e.position
			continue
		}
		if !hasBlock || e.position < blockEdge {
			blockEdge, hasBlock = e.position, true
		}
	}
	if sepPos != nil && togglePos != nil {
		result.file = &fileAnchors{*sepPos, *togglePos}
	}

	result.crowded = result.file == nil && !framesUsable
	if result.crowded {
		return result
	}

	own := map[string]bool{SeparatorIdentifier: true, ToggleIdentifier: true, SpacerIdentifier: true}
	byPID := map[int32][]rawItem{}
	var pids []int32
	for _, r := range raw {
		if own[r.Identifier] || (r.PID == ownPID && r.Frame.W < 4) {
			continue
		}
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
			if contains(app.Owners, e.owner) && !strings.HasPrefix(e.name, "PowerfulTop.") {
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
			onScreen := SectionOf(r.Frame, *separator, *toggle)
			var section Section
			switch {
			case onScreen == Absent:
				section = Absent
			case keyed && hasBlock && result.file != nil:
				section = SectionOfPosition(e.position, blockEdge)
			case reachable(r.Frame):
				// With the chevron itself behind «, anything still on screen sits right of it.
				if framesUsable {
					section = onScreen
				} else {
					section = Shown
				}
			default:
				// Overflowed with no layout entry: the bar isn't showing it, so it is hidden in practice.
				section = Hidden
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
				Reachable: framesUsable && reachable(r.Frame),
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
	result.items = sortItems(items)
	return result
}

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
