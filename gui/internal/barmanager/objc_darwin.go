//go:build darwin

package barmanager

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

type nsPoint struct{ X, Y float64 }
type nsSize struct{ W, H float64 }
type nsRect struct {
	Origin nsPoint
	Size   nsSize
}

var (
	runtimeOnce sync.Once
	sels        sync.Map

	// Typed objc_msgSend variants: purego's variadic Send can't pass structs or floats reliably.
	msgFloat func(id objc.ID, sel objc.SEL, v float64) objc.ID
	msgSize  func(id objc.ID, sel objc.SEL, v nsSize)
	msgBool  func(id objc.ID, sel objc.SEL, v bool)
)

func loadRuntime() {
	runtimeOnce.Do(func() {
		lib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			panic(err)
		}
		purego.RegisterLibFunc(&msgFloat, lib, "objc_msgSend")
		purego.RegisterLibFunc(&msgSize, lib, "objc_msgSend")
		purego.RegisterLibFunc(&msgBool, lib, "objc_msgSend")
		for _, path := range []string{
			"/System/Library/Frameworks/Foundation.framework/Foundation",
			"/System/Library/Frameworks/AppKit.framework/AppKit",
		} {
			if _, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
				panic(err)
			}
		}
	})
}

// registerMsg binds a typed objc_msgSend variant into fn.
func registerMsg(fn any) {
	loadRuntime()
	lib, _ := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	purego.RegisterLibFunc(fn, lib, "objc_msgSend")
}

func sel(name string) objc.SEL {
	if s, ok := sels.Load(name); ok {
		return s.(objc.SEL)
	}
	s := objc.RegisterName(name)
	sels.Store(name, s)
	return s
}

func class(name string) objc.ID {
	loadRuntime()
	return objc.ID(objc.GetClass(name))
}

func send(id objc.ID, name string, args ...any) objc.ID { return id.Send(sel(name), args...) }

// nsString is autoreleased.
func nsString(s string) objc.ID {
	return send(class("NSString"), "stringWithUTF8String:", s)
}

func goString(id objc.ID) string {
	if id == 0 {
		return ""
	}
	return cString(objc.Send[*byte](id, sel("UTF8String")))
}

func cString(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

// nsData is autoreleased.
func nsData(b []byte) objc.ID {
	if len(b) == 0 {
		return send(class("NSData"), "data")
	}
	return send(class("NSData"), "dataWithBytes:length:", unsafe.Pointer(&b[0]), uint(len(b)))
}

func dataBytes(data objc.ID) []byte {
	if data == 0 {
		return nil
	}
	n := objc.Send[uint](data, sel("length"))
	if n == 0 {
		return nil
	}
	return append([]byte(nil), unsafe.Slice(objc.Send[*byte](data, sel("bytes")), n)...)
}

// withPool drains what fn autoreleases; AppKit work outside the run loop's own pool leaks otherwise.
func withPool(fn func()) {
	pool := send(class("NSAutoreleasePool"), "new")
	defer send(pool, "drain")
	fn()
}
