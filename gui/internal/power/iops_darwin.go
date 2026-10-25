//go:build darwin

package power

import (
	"sync"

	"github.com/ebitengine/purego"

	"powerfultop/internal/macos"
)

// IOPSNotificationCreateRunLoopSource reports plug and unplug at once, where polling would lag up
// to 2 s. Its callback fires on the main run loop, which MyGo's app runs; without one (tests) the
// monitor just polls.
var iops struct {
	sync.Mutex
	loaded   bool
	source   uintptr
	monitors map[*Monitor]struct{}

	createSource func(callback, context uintptr) uintptr
	getMain      func() uintptr
	addSource    func(loop, source, mode uintptr)
	removeSource func(loop, source, mode uintptr)
	mode         uintptr
}

func loadIOPS() bool {
	if iops.loaded {
		return iops.createSource != nil
	}
	iops.loaded = true
	iokit, err := purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return false
	}
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return false
	}
	purego.RegisterLibFunc(&iops.createSource, iokit, "IOPSNotificationCreateRunLoopSource")
	purego.RegisterLibFunc(&iops.getMain, cf, "CFRunLoopGetMain")
	purego.RegisterLibFunc(&iops.addSource, cf, "CFRunLoopAddSource")
	purego.RegisterLibFunc(&iops.removeSource, cf, "CFRunLoopRemoveSource")
	// kCFRunLoopCommonModes is matched by pointer, which purego can only read through unsafe
	// pointer arithmetic; named modes are matched by CFEqual. The default mode misses events during
	// menu tracking, which the 2 s poll covers.
	iops.mode = macos.CFString("kCFRunLoopDefaultMode")
	return true
}

// One callback serves every monitor: purego callbacks are never freed.
var iopsCallback = sync.OnceValue(func() uintptr {
	return purego.NewCallback(func(_ uintptr) {
		iops.Lock()
		monitors := make([]*Monitor, 0, len(iops.monitors))
		for m := range iops.monitors {
			monitors = append(monitors, m)
		}
		iops.Unlock()
		for _, m := range monitors {
			go m.powerSourceChanged()
		}
	})
})

func watchPowerSources(m *Monitor) {
	iops.Lock()
	defer iops.Unlock()
	if !loadIOPS() {
		return
	}
	if iops.monitors == nil {
		iops.monitors = map[*Monitor]struct{}{}
	}
	iops.monitors[m] = struct{}{}
	if iops.source != 0 {
		return
	}
	iops.source = iops.createSource(iopsCallback(), 0)
	if iops.source != 0 {
		iops.addSource(iops.getMain(), iops.source, iops.mode)
	}
}

func unwatchPowerSources(m *Monitor) {
	iops.Lock()
	defer iops.Unlock()
	delete(iops.monitors, m)
	if len(iops.monitors) == 0 && iops.source != 0 {
		iops.removeSource(iops.getMain(), iops.source, iops.mode)
		iops.source = 0
	}
}
