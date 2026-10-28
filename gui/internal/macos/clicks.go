//go:build darwin

package macos

import "github.com/ebitengine/purego/objc"

// NSEventMaskLeftMouseDown | NSEventMaskRightMouseDown
const mouseDownMask = uint64(1<<1 | 1<<3)

// ClickMonitor hears every mouse press: in other apps and the menu bar (global) and in this app (local).
//
// An accessory app's popover is meant to close as the user clicks elsewhere, but macOS 14+ no longer
// lets activateIgnoringOtherApps: activate it, so the popover never becomes key and AppKit's
// transient behaviour never fires; clicks on the status item while it shows are swallowed too.
type ClickMonitor struct {
	monitors []objc.ID
	blocks   []objc.Block
}

// StartClickMonitor must be called on the main thread; fn runs there, before the click is handled.
func StartClickMonitor(fn func()) *ClickMonitor {
	event := Class("NSEvent")
	global := objc.NewBlock(func(_ objc.Block, _ objc.ID) { fn() })
	local := objc.NewBlock(func(_ objc.Block, e objc.ID) objc.ID {
		fn()
		return e
	})
	m := &ClickMonitor{blocks: []objc.Block{global, local}}
	for _, add := range []struct {
		selector string
		block    objc.Block
	}{
		{"addGlobalMonitorForEventsMatchingMask:handler:", global},
		{"addLocalMonitorForEventsMatchingMask:handler:", local},
	} {
		if monitor := Send(event, add.selector, mouseDownMask, add.block); monitor != 0 {
			m.monitors = append(m.monitors, Send(monitor, "retain"))
		}
	}
	return m
}

// Stop removes the monitors; call it on the main thread.
func (m *ClickMonitor) Stop() {
	event := Class("NSEvent")
	for _, monitor := range m.monitors {
		Send(event, "removeMonitor:", monitor)
		Send(monitor, "release")
	}
	for _, block := range m.blocks {
		block.Release()
	}
	m.monitors, m.blocks = nil, nil
}
