//go:build darwin

package barmanager

import (
	"sync"
	"time"

	"powerfultop/internal/macos"
)

// MenuBarAgent draws every item into one window per menu bar and Space. Each child of such a
// window is a slot carrying the item's frame; an app's slot holds an element owned by the app,
// whose extras menu bar holds the item, while MenuBarAgent's own items hold a hosting view whose
// menu extra's identifier names them. Either way the item, an AXMenuExtra, is two levels down.

// forEachSlot calls fn with every slot of every menu bar window: id is the owning app's bundle
// identifier, or a menu extra's identifier for MenuBarAgent's own items. element is borrowed.
func (d *darwinItems) forEachSlot(fn func(window, slot Rect, id string, element uintptr)) {
	loadAX()
	apps := d.runningApps()
	agent := int32(0)
	bundles := map[int32]string{}
	for _, a := range apps {
		bundles[a.PID] = a.BundleID
		if a.BundleID == menuBarAgentID {
			agent = a.PID
		}
	}
	if agent == 0 {
		return
	}
	app := axUIElementCreateApplication(agent)
	if app == 0 {
		return
	}
	defer cfRelease(app)
	axUIElementSetMessagingTimeout(app, 1)
	windows := copyAttribute(app, "AXWindows")
	if windows == 0 {
		return
	}
	defer cfRelease(windows)
	for i := range cfArrayGetCount(windows) {
		window := cfArrayGetValueAtIndex(windows, i)
		windowFrame := frameOf(window)
		eachChild(window, func(slot uintptr) {
			f := frameOf(slot)
			if f.W <= 0 {
				return
			}
			first := true
			// The owner lives only as long as the children array eachChild holds, so it is used in here.
			eachChild(slot, func(owner uintptr) {
				if !first {
					return
				}
				first = false
				var pid int32
				if axUIElementGetPid(owner, &pid) != 0 {
					return
				}
				if pid != agent {
					if id := bundles[pid]; id != "" {
						fn(windowFrame, f, id, owner)
					}
					return
				}
				id := ""
				eachChild(owner, func(c uintptr) {
					if id == "" {
						id = stringAttribute(c, "AXIdentifier")
					}
				})
				if id != "" {
					fn(windowFrame, f, id, slot)
				}
			})
		})
	}
}

func eachChild(element uintptr, fn func(child uintptr)) {
	children := copyAttribute(element, "AXChildren")
	if children == 0 {
		return
	}
	defer cfRelease(children)
	for i := range cfArrayGetCount(children) {
		fn(cfArrayGetValueAtIndex(children, i))
	}
}

// drawn keeps the first frame of each item: it shows in every Space's window, in the same place.
func (d *darwinItems) drawn() map[string]Rect {
	out := map[string]Rect{}
	d.forEachSlot(func(_, slot Rect, id string, _ uintptr) {
		if _, seen := out[id]; !seen {
			out[id] = slot
		}
	})
	return out
}

// menuExtraUnder is the first AXMenuExtra two levels under element, retained.
func menuExtraUnder(element uintptr) axRef {
	var found axRef
	eachChild(element, func(level uintptr) {
		eachChild(level, func(item uintptr) {
			if found == 0 && stringAttribute(item, "AXSubrole") == "AXMenuExtra" {
				found = axRef(cfRetain(item))
			}
		})
	})
	return found
}

func (d *darwinItems) onBar(id string) axRef {
	var found axRef
	d.forEachSlot(func(_, _ Rect, owner string, element uintptr) {
		if found == 0 && owner == id {
			found = menuExtraUnder(element)
		}
	})
	return found
}

// extra asks the app itself, not MenuBarAgent: the app keeps its status item, with its menu,
// while the item is switched off, and pressing it opens that where the item was last drawn.
func (d *darwinItems) extra(bundleID string) axRef {
	loadAX()
	app, ok := d.runningApp(bundleID)
	if !ok {
		return 0
	}
	element := axUIElementCreateApplication(app.PID)
	if element == 0 {
		return 0
	}
	defer cfRelease(element)
	axUIElementSetMessagingTimeout(element, 1)
	bar := copyAttribute(element, "AXExtrasMenuBar")
	if bar == 0 {
		return 0
	}
	defer cfRelease(bar)
	var found axRef
	eachChild(bar, func(item uintptr) {
		if found == 0 && stringAttribute(item, "AXSubrole") == "AXMenuExtra" {
			found = axRef(cfRetain(item))
		}
	})
	return found
}

// menu reads the item's menu, which makes the app update it as it would before opening it; nil
// when the item opens a popover or a window instead.
func (d *darwinItems) menu(el axRef) []MenuEntry {
	if el == 0 {
		return nil
	}
	axUIElementSetMessagingTimeout(uintptr(el), 1)
	var entries []MenuEntry
	found := false
	eachChild(uintptr(el), func(c uintptr) {
		if !found && stringAttribute(c, "AXRole") == "AXMenu" {
			found = true
			entries = menuEntries(c, 0)
		}
	})
	return entries
}

func menuEntries(menu uintptr, depth int) []MenuEntry {
	var out []MenuEntry
	eachChild(menu, func(item uintptr) {
		if stringAttribute(item, "AXRole") != "AXMenuItem" {
			return
		}
		e := MenuEntry{
			Title:   stringAttribute(item, "AXTitle"),
			Enabled: true,
			Mark:    stringAttribute(item, "AXMenuItemMarkChar"),
			Key:     stringAttribute(item, "AXMenuItemCmdChar"),
		}
		if v, ok := scalarAttribute(item, "AXEnabled").(bool); ok {
			e.Enabled = v
		}
		if v, ok := scalarAttribute(item, "AXMenuItemCmdModifiers").(int64); ok {
			e.Modifiers = int(v)
		}
		if depth < 4 {
			eachChild(item, func(c uintptr) {
				if e.Submenu == nil && stringAttribute(c, "AXRole") == "AXMenu" {
					e.Submenu = menuEntries(c, depth+1)
					if e.Submenu == nil {
						e.Submenu = []MenuEntry{}
					}
				}
			})
		}
		e.element = axRef(cfRetain(item))
		out = append(out, e)
	})
	return out
}

func scalarAttribute(element uintptr, name string) any {
	ref := copyAttribute(element, name)
	if ref == 0 {
		return nil
	}
	defer cfRelease(ref)
	v, _ := macos.FromCF(ref)
	return v
}

// press acts as a click would. Opening a menu keeps the call waiting until the menu closes, so
// the timeout is short and its error means nothing.
func (d *darwinItems) press(el axRef) {
	if el == 0 {
		return
	}
	loadAX()
	axUIElementSetMessagingTimeout(uintptr(el), 0.25)
	axUIElementPerformAction(uintptr(el), axAttributes["AXPress"])
}

func (d *darwinItems) release(el axRef) {
	if el != 0 {
		cfRelease(uintptr(el))
	}
}

// click posts a real click: MenuBarAgent ignores synthetic ones without a HID source and a click
// state, and a few items ignore AXPress.
func (d *darwinItems) click(at Point) {
	loadCG()
	const (
		leftMouseDown   = 1
		leftMouseUp     = 2
		hidSystemState  = 1
		hidEventTap     = 0
		mouseClickState = 1 // kCGMouseEventClickState
	)
	var restore *nsPoint
	if e := cgEventCreate(0); e != 0 {
		p := cgEventGetLocation(e)
		restore = &p
		cfRelease(e)
	}
	source := cgEventSourceCreate(hidSystemState)
	for _, typ := range []uint32{leftMouseDown, leftMouseUp} {
		e := cgEventCreateMouseEvent(source, typ, nsPoint{at.X, at.Y}, 0)
		if e == 0 {
			continue
		}
		cgEventSetIntegerValueField(e, mouseClickState, 1)
		cgEventPost(hidEventTap, e)
		cfRelease(e)
		time.Sleep(40 * time.Millisecond)
	}
	if source != 0 {
		cfRelease(source)
	}
	if restore != nil {
		cgWarpMouseCursorPosition(*restore)
	}
}

type windowInfo struct {
	pid   int32
	layer int64
	frame Rect
}

// onScreenWindows reads the window list, which needs no permission for frames and owners.
func onScreenWindows() []windowInfo {
	loadCG()
	const onScreenOnly, excludeDesktop = 1 << 0, 1 << 4
	list := cgWindowListCopyWindowInfo(onScreenOnly|excludeDesktop, 0)
	if list == 0 {
		return nil
	}
	defer cfRelease(list)
	value, err := macos.FromCF(list)
	if err != nil {
		return nil
	}
	items, _ := value.([]any)
	out := make([]windowInfo, 0, len(items))
	for _, item := range items {
		dict, ok := item.(map[string]any)
		if !ok {
			continue
		}
		pid, _ := macos.Int(dict, "kCGWindowOwnerPID")
		layer, _ := macos.Int(dict, "kCGWindowLayer")
		bounds, _ := macos.Dict(dict, "kCGWindowBounds")
		number := func(key string) float64 {
			switch v := bounds[key].(type) {
			case float64:
				return v
			case int64:
				return float64(v)
			}
			return 0
		}
		out = append(out, windowInfo{int32(pid), layer, Rect{number("X"), number("Y"), number("Width"), number("Height")}})
	}
	return out
}

var (
	barsMu    sync.Mutex
	barsCache []Rect
	barsAt    time.Time
)

// menuBars are the menu bar windows' frames, kept for a moment: showingUI polls several times a second.
func (d *darwinItems) menuBars() []Rect {
	barsMu.Lock()
	defer barsMu.Unlock()
	if time.Since(barsAt) < 2*time.Second {
		return barsCache
	}
	var bars []Rect
	seen := map[Rect]bool{}
	d.forEachSlot(func(window, _ Rect, _ string, _ uintptr) {
		if !seen[window] {
			seen[window] = true
			bars = append(bars, window)
		}
	})
	barsCache, barsAt = bars, time.Now()
	return bars
}

// showingUI: another app's menu hangs from a menu bar, or pid shows a window above the normal
// level, as a panel opened from the menu bar does.
func (d *darwinItems) showingUI(pid int32) bool {
	loadCG()
	popUpMenu := int64(cgWindowLevelForKey(11)) // kCGPopUpMenuWindowLevelKey
	own := d.ownPID()
	bars := d.menuBars()
	// A menu hanging from a menu bar has its top edge at, or a few points below, the bar's bottom.
	const menuGap = 12
	for _, w := range onScreenWindows() {
		if w.pid == own {
			continue
		}
		if pid != 0 && w.pid == pid && w.layer > 0 {
			return true
		}
		if w.layer != popUpMenu {
			continue
		}
		for _, bar := range bars {
			top := w.frame.Y
			if top >= bar.Y && top <= bar.Y+bar.H+menuGap && w.frame.MaxX() > bar.X && w.frame.X < bar.MaxX() {
				return true
			}
		}
	}
	return false
}
