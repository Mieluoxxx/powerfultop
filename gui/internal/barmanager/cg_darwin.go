//go:build darwin

package barmanager

import (
	"sync"
	"time"

	"github.com/ebitengine/purego"
)

type nsPoint struct{ X, Y float64 }
type nsSize struct{ W, H float64 }

var (
	cgOnce sync.Once

	cgEventCreate               func(source uintptr) uintptr
	cgEventGetLocation          func(event uintptr) nsPoint
	cgEventCreateMouseEvent     func(source uintptr, typ uint32, at nsPoint, button uint32) uintptr
	cgEventSetFlags             func(event uintptr, flags uint64)
	cgEventPost                 func(tap uint32, event uintptr)
	cgWarpMouseCursorPosition   func(at nsPoint) int32
	cgEventSourceCreate         func(state int32) uintptr
	cgEventSetIntegerValueField func(event uintptr, field uint32, value int64)
	cgWindowListCopyWindowInfo  func(options uint32, relativeTo uint32) uintptr
	cgWindowLevelForKey         func(key int32) int32

	cfRelease              func(ref uintptr)
	cfRetain               func(ref uintptr) uintptr
	cfArrayGetCount        func(array uintptr) int
	cfArrayGetValueAtIndex func(array uintptr, index int) uintptr
)

func loadCG() {
	cgOnce.Do(func() {
		cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			panic(err)
		}
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			panic(err)
		}
		purego.RegisterLibFunc(&cgEventCreate, cg, "CGEventCreate")
		purego.RegisterLibFunc(&cgEventGetLocation, cg, "CGEventGetLocation")
		purego.RegisterLibFunc(&cgEventCreateMouseEvent, cg, "CGEventCreateMouseEvent")
		purego.RegisterLibFunc(&cgEventSetFlags, cg, "CGEventSetFlags")
		purego.RegisterLibFunc(&cgEventPost, cg, "CGEventPost")
		purego.RegisterLibFunc(&cgWarpMouseCursorPosition, cg, "CGWarpMouseCursorPosition")
		purego.RegisterLibFunc(&cgEventSourceCreate, cg, "CGEventSourceCreate")
		purego.RegisterLibFunc(&cgEventSetIntegerValueField, cg, "CGEventSetIntegerValueField")
		purego.RegisterLibFunc(&cgWindowListCopyWindowInfo, cg, "CGWindowListCopyWindowInfo")
		purego.RegisterLibFunc(&cgWindowLevelForKey, cg, "CGWindowLevelForKey")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
		purego.RegisterLibFunc(&cfRetain, cf, "CFRetain")
		purego.RegisterLibFunc(&cfArrayGetCount, cf, "CFArrayGetCount")
		purego.RegisterLibFunc(&cfArrayGetValueAtIndex, cf, "CFArrayGetValueAtIndex")
	})
}

// commandDrag ⌘-drags from start to end as a user would, then puts the pointer back.
func commandDrag(start, end Point) {
	loadCG()
	const (
		leftMouseDown    = 1
		leftMouseUp      = 2
		mouseMoved       = 5
		leftMouseDragged = 6
		maskCommand      = 1 << 20
		hidEventTap      = 0
	)
	var restore *nsPoint
	if e := cgEventCreate(0); e != 0 {
		p := cgEventGetLocation(e)
		restore = &p
		cfRelease(e)
	}
	post := func(typ uint32, at Point) {
		e := cgEventCreateMouseEvent(0, typ, nsPoint{at.X, at.Y}, 0)
		if e == 0 {
			return
		}
		cgEventSetFlags(e, maskCommand)
		cgEventPost(hidEventTap, e)
		cfRelease(e)
		time.Sleep(15 * time.Millisecond)
	}
	post(mouseMoved, start)
	post(leftMouseDown, start)
	// The bar only picks an item up after the button has been held for a moment.
	time.Sleep(80 * time.Millisecond)
	const steps = 12
	for step := 1; step <= steps; step++ {
		f := float64(step) / steps
		post(leftMouseDragged, Point{start.X + (end.X-start.X)*f, start.Y + (end.Y-start.Y)*f})
	}
	time.Sleep(80 * time.Millisecond)
	post(leftMouseUp, end)
	if restore != nil {
		cgWarpMouseCursorPosition(*restore)
	}
}
