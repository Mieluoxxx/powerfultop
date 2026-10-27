package barmanager

import (
	"math"
	"sort"
	"strconv"
)

// The editor's model comes from three sources that don't depend on which display is active:
// MenuBarAgent's windows, one per display's menu bar, list what each bar draws and where; the
// layout file orders every item, drawn or not; and the switches say what is switched off. Apps'
// own Accessibility frames follow whichever display is active and flip the bar the editor shows,
// so they aren't asked.

// agentSlot is an item MenuBarAgent draws in one menu bar: an app's bundle identifier, or a menu
// extra's identifier for its own items.
type agentSlot struct {
	ID    string
	Frame Rect
}

// agentBar is MenuBarAgent's window for one display's menu bar.
type agentBar struct {
	Frame Rect
	Slots []agentSlot
}

// Display is a menu bar the editor can show.
type Display struct {
	Name     string
	Frame    Rect
	Internal bool
}

type modelInput struct {
	apps      []appRef
	bars      []agentBar
	positions map[string]float64
	// off are the apps switched off in the allow list, by bundle identifier.
	off map[string]bool
	own ownItem
	// overflow is the « of the active display, when it shows; other bars' « isn't read.
	overflow *Rect
}

// barFor is the bar of the display at frame, matched loosely: frames come from two APIs.
func barFor(bars []agentBar, frame Rect) (agentBar, bool) {
	for _, b := range bars {
		if math.Abs(b.Frame.X-frame.X) < 2 && math.Abs(b.Frame.Y-frame.Y) < 2 && math.Abs(b.Frame.W-frame.W) < 2 {
			return b, true
		}
	}
	return agentBar{}, false
}

// overflowed are the slots of bar that « holds. They are stacked on top of one another where « is,
// a few points apart, while neighbours in sight overlap by a point or two at most; on the active
// display they also sit left of the « button.
func overflowed(bar agentBar, anchor *Rect) map[int]bool {
	order := make([]int, len(bar.Slots))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return bar.Slots[order[a]].Frame.X < bar.Slots[order[b]].Frame.X })
	out := map[int]bool{}
	for k := 0; k+1 < len(order); k++ {
		cur, next := bar.Slots[order[k]].Frame, bar.Slots[order[k+1]].Frame
		if next.X >= cur.X+min(cur.W, next.W)/2 {
			break
		}
		out[order[k]], out[order[k+1]] = true, true
	}
	if anchor != nil && anchor.MidY() >= bar.Frame.Y && anchor.MidY() <= bar.Frame.Y+bar.Frame.H &&
		anchor.X >= bar.Frame.X && anchor.X <= bar.Frame.MaxX() {
		for i, s := range bar.Slots {
			if s.Frame.MidX() <= anchor.MaxX() {
				out[i] = true
			}
		}
	}
	return out
}

// buildItems lists, left to right, the items as the menu bar of the display at frame shows them:
// drawn there, held by its «, or switched off.
func buildItems(in modelInput, frame Rect) []Item {
	bar, _ := barFor(in.bars, frame)
	hidden := overflowed(bar, in.overflow)
	here := map[string][]int{}
	for i, s := range bar.Slots {
		here[s.ID] = append(here[s.ID], i)
	}
	// drawnCount is how many items an app has, from the bar that draws the most of them.
	drawnCount := map[string]int{}
	for _, b := range in.bars {
		count := map[string]int{}
		for _, s := range b.Slots {
			count[s.ID]++
		}
		for id, n := range count {
			drawnCount[id] = max(drawnCount[id], n)
		}
	}

	type entry struct {
		key      string
		name     string
		position float64
	}
	keysOf := func(owners []string, names func(string) bool) []entry {
		var out []entry
		for key, v := range in.positions {
			if owner, name, ok := ParseKey(key); ok && contains(owners, owner) && names(name) {
				out = append(out, entry{key, name, v})
			}
		}
		// Larger positions sit further left.
		sort.Slice(out, func(i, j int) bool { return out[i].position > out[j].position })
		return out
	}

	var items []Item
	add := func(id, name, appPath, icon string, keys []entry, slots []int, off bool) {
		sort.Slice(slots, func(a, b int) bool { return bar.Slots[slots[a]].Frame.X < bar.Slots[slots[b]].Frame.X })
		// A switched-off app has an item for each key it left; layout keys outlive items, so a
		// drawn app has as many as MenuBarAgent draws.
		n := drawnCount[id]
		if off {
			n = max(len(keys), 1)
		}
		// Keys pair with items only when the counts agree; positions order them as frames do.
		paired := len(keys) == n
		for i := range n {
			// An item keeps its key's name as it moves; its place among the app's items doesn't.
			it := Item{ID: id + "#" + strconv.Itoa(i), Name: name, BundleID: id, AppPath: appPath, Icon: icon}
			if paired && i < len(keys) {
				it.ID = id + "#" + keys[i].name
				it.LayoutKey, it.Position, it.HasPosition = keys[i].key, keys[i].position, true
				if n > 1 {
					it.Detail = keys[i].name
				}
			} else if n > 1 {
				it.Detail = strconv.Itoa(i + 1)
			}
			switch {
			case off:
				it.Section = Absent
			case i < len(slots):
				it.Frame = bar.Slots[slots[i]].Frame
				it.Section = Shown
				if hidden[slots[i]] {
					it.Section = Overflow
				} else {
					it.Reachable = true
				}
			default:
				// Drawn on another display but not on this one: this one's « holds it.
				it.Section = Overflow
			}
			items = append(items, it)
		}
	}

	for _, app := range in.apps {
		off := in.off[app.BundleID]
		if app.BundleID == "" || app.BundleID == menuBarAgentID || (drawnCount[app.BundleID] == 0 && !off) {
			continue
		}
		names := func(string) bool { return true }
		if app.PID == in.own.pid {
			// Earlier versions left other keys of this app in the layout file.
			names = func(n string) bool { return contains(in.own.names, n) }
		}
		add(app.BundleID, app.Name, app.Path, "", keysOf(app.Owners, names), append([]int(nil), here[app.BundleID]...), off)
	}
	for _, sys := range SystemItems {
		if sys.Flag != nil || sys.LoginItem != nil || drawnCount[sys.ID] == 0 {
			// The others are apps' items and come with the apps.
			continue
		}
		var keys []entry
		if v, ok := in.positions[sys.PositionKey()]; ok {
			keys = []entry{{sys.PositionKey(), sys.Key, v}}
		}
		add(sys.ID, sys.Name, "", sys.Icon, keys, append([]int(nil), here[sys.ID]...), false)
	}
	return sortItems(items)
}
