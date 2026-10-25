//go:build darwin

package power

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// The SMC charge keys need an Apple-private entitlement on macOS 27, but PowerUIAgent holds it and
// serves the System Settings charge limit over XPC to any local client, so PowerfulTop drives the
// same setting through the private PowerUI framework without root.
const powerUIPath = "/System/Library/PrivateFrameworks/PowerUI.framework/Versions/A/PowerUI"

var errPowerUIUnavailable = errors.New("PowerUI charge limit is unavailable")

type powerUI struct {
	once   sync.Once
	client objc.ID

	// Typed objc_msgSend variants: the methods return a BOOL, an unsigned char or an unsigned long
	// long, which the variadic Send can't express.
	msgBool     func(objc.ID, objc.SEL) bool
	msgUInt8Err func(objc.ID, objc.SEL, *objc.ID) uint8
	msgUInt64   func(objc.ID, objc.SEL, *objc.ID) uint64
	msgIDErr    func(objc.ID, objc.SEL, *objc.ID) objc.ID
	msgSetErr   func(objc.ID, objc.SEL, uint8, *objc.ID) bool
	msgBoolErr  func(objc.ID, objc.SEL, *objc.ID) bool
}

func (p *powerUI) load() objc.ID {
	p.once.Do(func() {
		if _, err := purego.Dlopen(powerUIPath, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			return
		}
		lib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		for _, fn := range []any{&p.msgBool, &p.msgUInt8Err, &p.msgUInt64, &p.msgIDErr, &p.msgSetErr, &p.msgBoolErr} {
			purego.RegisterLibFunc(fn, lib, "objc_msgSend")
		}
		cls := objc.ID(objc.GetClass("PowerUISmartChargeClient"))
		if cls == 0 {
			return
		}
		client := cls.Send(objc.RegisterName("alloc"))
		client = client.Send(objc.RegisterName("initWithClientName:"), nsString("PowerfulTop"))
		for _, name := range []string{"isMCLSupported", "availableChargeLimitsWithError:", "getMCLLimitWithError:",
			"isMCLCurrentlyEnabled:", "setMCLLimit:error:", "disableMCL:"} {
			if !objc.Send[bool](client, objc.RegisterName("respondsToSelector:"), objc.RegisterName(name)) {
				return
			}
		}
		p.client = client
	})
	return p.client
}

// call runs fn inside an autorelease pool on one OS thread, since the pool must be popped where it was pushed.
func (p *powerUI) call(fn func(client objc.ID) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	client := p.load()
	if client == 0 {
		return errPowerUIUnavailable
	}
	return fn(client)
}

func (p *powerUI) read() (nativeLimitState, error) {
	var s nativeLimitState
	err := p.call(func(client objc.ID) error {
		s.supported = p.msgBool(client, objc.RegisterName("isMCLSupported"))
		if !s.supported {
			return nil
		}
		var nsErr objc.ID
		limits := p.msgIDErr(client, objc.RegisterName("availableChargeLimitsWithError:"), &nsErr)
		if limits == 0 {
			return nsError(nsErr, "no charge limits available")
		}
		for i := range objc.Send[uint](limits, objc.RegisterName("count")) {
			n := limits.Send(objc.RegisterName("objectAtIndex:"), i)
			s.limits = append(s.limits, int(objc.Send[int64](n, objc.RegisterName("longLongValue"))))
		}
		nsErr = 0
		s.limit = int(p.msgUInt8Err(client, objc.RegisterName("getMCLLimitWithError:"), &nsErr))
		if nsErr != 0 {
			return nsError(nsErr, "")
		}
		s.enabled = p.msgUInt64(client, objc.RegisterName("isMCLCurrentlyEnabled:"), &nsErr) != 0
		if nsErr != 0 {
			return nsError(nsErr, "")
		}
		return nil
	})
	return s, err
}

func (p *powerUI) set(percent int) error {
	return p.call(func(client objc.ID) error {
		var nsErr objc.ID
		ok := false
		if percent >= 100 {
			ok = p.msgBoolErr(client, objc.RegisterName("disableMCL:"), &nsErr)
		} else {
			ok = p.msgSetErr(client, objc.RegisterName("setMCLLimit:error:"), uint8(percent), &nsErr)
		}
		if !ok {
			return nsError(nsErr, "PowerUIAgent refused the charge limit")
		}
		return nil
	})
}

func nsString(s string) objc.ID {
	return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), s)
}

func nsError(e objc.ID, fallback string) error {
	if e == 0 {
		if fallback == "" {
			fallback = "unknown PowerUI error"
		}
		return errors.New(fallback)
	}
	p := objc.Send[*byte](e.Send(objc.RegisterName("localizedDescription")), objc.RegisterName("UTF8String"))
	if p == nil {
		return errors.New(fallback)
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return errors.New(string(unsafe.Slice(p, n)))
}
