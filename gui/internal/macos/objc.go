//go:build darwin

package macos

import (
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	appKitOnce sync.Once
	appKitErr  error
	libobjc    uintptr
	selectors  sync.Map
)

// LoadAppKit loads libobjc, Foundation and AppKit, which a test binary or the fan helper doesn't
// load on its own; the app's classes resolve to nil until they are.
func LoadAppKit() error {
	appKitOnce.Do(func() {
		for _, path := range []string{
			"/usr/lib/libobjc.A.dylib",
			"/System/Library/Frameworks/Foundation.framework/Foundation",
			"/System/Library/Frameworks/AppKit.framework/AppKit",
		} {
			lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err != nil {
				appKitErr = err
				return
			}
			if libobjc == 0 {
				libobjc = lib
			}
		}
	})
	return appKitErr
}

// BindMsgSend binds typed objc_msgSend variants into fns: purego's variadic Send can't pass
// structs or floats, nor return BOOL or small integers, reliably.
func BindMsgSend(fns ...any) error {
	if err := LoadAppKit(); err != nil {
		return err
	}
	for _, fn := range fns {
		purego.RegisterLibFunc(fn, libobjc, "objc_msgSend")
	}
	return nil
}

func Sel(name string) objc.SEL {
	if s, ok := selectors.Load(name); ok {
		return s.(objc.SEL)
	}
	s := objc.RegisterName(name)
	selectors.Store(name, s)
	return s
}

// Class is 0 when no loaded framework defines name.
func Class(name string) objc.ID {
	_ = LoadAppKit()
	return objc.ID(objc.GetClass(name))
}

func Send(id objc.ID, name string, args ...any) objc.ID { return id.Send(Sel(name), args...) }

// NSString is autoreleased.
func NSString(s string) objc.ID {
	return Send(Class("NSString"), "stringWithUTF8String:", s)
}

func GoString(id objc.ID) string {
	if id == 0 {
		return ""
	}
	return CString(objc.Send[*byte](id, Sel("UTF8String")))
}

// CString copies a NUL-terminated C string.
func CString(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

// NSData is autoreleased.
func NSData(b []byte) objc.ID {
	if len(b) == 0 {
		return Send(Class("NSData"), "data")
	}
	return Send(Class("NSData"), "dataWithBytes:length:", unsafe.Pointer(&b[0]), uint(len(b)))
}

func DataBytes(data objc.ID) []byte {
	if data == 0 {
		return nil
	}
	n := objc.Send[uint](data, Sel("length"))
	if n == 0 {
		return nil
	}
	return append([]byte(nil), unsafe.Slice(objc.Send[*byte](data, Sel("bytes")), n)...)
}

// WithPool drains what fn autoreleases; Cocoa work outside the run loop's own pool leaks otherwise.
// A pool belongs to its thread, so the goroutine stays on one until the pool drains.
func WithPool(fn func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := Send(Class("NSAutoreleasePool"), "new")
	defer Send(pool, "drain")
	fn()
}

// AppRunning reports whether an app with bundleID runs; NSRunningApplication is callable from any thread.
func AppRunning(bundleID string) bool {
	running := false
	WithPool(func() {
		apps := Send(Class("NSRunningApplication"), "runningApplicationsWithBundleIdentifier:", NSString(bundleID))
		running = apps != 0 && objc.Send[uint](apps, Sel("count")) > 0
	})
	return running
}
