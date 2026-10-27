//go:build darwin

package barmanager

import (
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"powerfultop/internal/macos"
)

// barWatcher hears MenuBarAgent lay a menu bar out again, on its application element: subscribing
// on the windows delivers nothing (measured by Chest on macOS 27.0.1). An item that appears or
// changes size posts AXLayoutChanged once per window; one that goes posts only
// AXUIElementDestroyed. launchd starts a new
// MenuBarAgent when one quits, and nothing announces it, so the watcher checks for a new one.
type barWatcher struct {
	mu        sync.Mutex
	listeners map[int]func()
	next      int
	started   bool

	// Main thread only.
	pid      int32
	observer uintptr
	source   uintptr
}

var (
	watcher     barWatcher
	observeOnce sync.Once

	axObserverCreate           func(pid int32, callback uintptr, observer *uintptr) int32
	axObserverAddNotification  func(observer, element, notification, refcon uintptr) int32
	axObserverGetRunLoopSource func(observer uintptr) uintptr
	cfRunLoopGetMain           func() uintptr
	cfRunLoopAddSource         func(loop, source, mode uintptr)
	cfRunLoopRemoveSource      func(loop, source, mode uintptr)
	kCFRunLoopCommonModes      uintptr
	layoutChangedCallback      uintptr
	barNotifications           []uintptr
)

// agentCheckInterval is how often the watcher looks for a restarted MenuBarAgent.
const agentCheckInterval = 5 * time.Second

func loadObserve() {
	observeOnce.Do(func() {
		loadAX()
		ax, err := purego.Dlopen("/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			panic(err)
		}
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			panic(err)
		}
		purego.RegisterLibFunc(&axObserverCreate, ax, "AXObserverCreate")
		purego.RegisterLibFunc(&axObserverAddNotification, ax, "AXObserverAddNotification")
		purego.RegisterLibFunc(&axObserverGetRunLoopSource, ax, "AXObserverGetRunLoopSource")
		purego.RegisterLibFunc(&cfRunLoopGetMain, cf, "CFRunLoopGetMain")
		purego.RegisterLibFunc(&cfRunLoopAddSource, cf, "CFRunLoopAddSource")
		purego.RegisterLibFunc(&cfRunLoopRemoveSource, cf, "CFRunLoopRemoveSource")
		// A CFStringRef global: the symbol is the address of the pointer.
		if sym, err := purego.Dlsym(cf, "kCFRunLoopCommonModes"); err == nil {
			kCFRunLoopCommonModes = **(**uintptr)(unsafe.Pointer(&sym))
		}
		for _, name := range []string{"AXLayoutChanged", "AXCreated", "AXUIElementDestroyed"} {
			barNotifications = append(barNotifications, macos.CFString(name))
		}
		// Created once: callbacks are never freed.
		layoutChangedCallback = purego.NewCallback(func(observer, element, notification, refcon uintptr) uintptr {
			watcher.notify()
			return 0
		})
	})
}

func (d *darwinItems) onBarChange(fn func()) (stop func()) {
	watcher.mu.Lock()
	if watcher.listeners == nil {
		watcher.listeners = map[int]func(){}
	}
	id := watcher.next
	watcher.next++
	watcher.listeners[id] = fn
	start := !watcher.started
	watcher.started = true
	watcher.mu.Unlock()
	if start {
		go watcher.run(d)
	}
	return func() {
		watcher.mu.Lock()
		delete(watcher.listeners, id)
		watcher.mu.Unlock()
	}
}

// watchLog records whether the watcher hears MenuBarAgent; it shares the tray's log.
var watchLog = func(string, ...any) {}

var (
	heard      int
	heardSince time.Time
)

func (w *barWatcher) notify() {
	heard++
	if time.Since(heardSince) > time.Minute {
		watchLog("watcher: heard MenuBarAgent %d times", heard)
		heard, heardSince = 0, time.Now()
	}
	w.notifyListeners()
}

func (w *barWatcher) notifyListeners() {
	w.mu.Lock()
	listeners := make([]func(), 0, len(w.listeners))
	for _, fn := range w.listeners {
		listeners = append(listeners, fn)
	}
	w.mu.Unlock()
	for _, fn := range listeners {
		fn()
	}
}

func (w *barWatcher) run(d *darwinItems) {
	loadObserve()
	for {
		if app, ok := d.runningApp(menuBarAgentID); ok {
			attached := false
			before := w.pid
			d.onMain(func() { attached = w.attach(app.PID) })
			if app.PID != before {
				watchLog("watcher: attach to MenuBarAgent %d: %v", app.PID, attached)
			}
			if attached {
				// A new MenuBarAgent lays every bar out as it starts, maybe before this listened.
				w.notifyListeners()
			}
		}
		time.Sleep(agentCheckInterval)
	}
}

// attach observes the MenuBarAgent with pid unless it already does; main thread only, where the
// observer's run loop source goes.
func (w *barWatcher) attach(pid int32) bool {
	if pid == w.pid || kCFRunLoopCommonModes == 0 {
		return false
	}
	if w.source != 0 {
		cfRunLoopRemoveSource(cfRunLoopGetMain(), w.source, kCFRunLoopCommonModes)
	}
	if w.observer != 0 {
		cfRelease(w.observer)
	}
	w.pid, w.observer, w.source = 0, 0, 0
	var observer uintptr
	if axObserverCreate(pid, layoutChangedCallback, &observer) != 0 || observer == 0 {
		return false
	}
	app := axUIElementCreateApplication(pid)
	if app == 0 {
		cfRelease(observer)
		return false
	}
	defer cfRelease(app)
	added := 0
	for _, n := range barNotifications {
		if axObserverAddNotification(observer, app, n, 0) == 0 {
			added++
		}
	}
	if added == 0 {
		cfRelease(observer)
		return false
	}
	w.pid, w.observer = pid, observer
	w.source = axObserverGetRunLoopSource(observer)
	cfRunLoopAddSource(cfRunLoopGetMain(), w.source, kCFRunLoopCommonModes)
	return true
}
