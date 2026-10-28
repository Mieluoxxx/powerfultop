//go:build darwin

package power

import (
	"errors"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"powerfultop/internal/macos"
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
		if macos.BindMsgSend(&p.msgBool, &p.msgUInt8Err, &p.msgUInt64, &p.msgIDErr, &p.msgSetErr, &p.msgBoolErr) != nil {
			return
		}
		cls := macos.Class("PowerUISmartChargeClient")
		if cls == 0 {
			return
		}
		client := macos.Send(macos.Send(cls, "alloc"), "initWithClientName:", macos.NSString("PowerfulTop"))
		for _, name := range []string{"isMCLSupported", "availableChargeLimitsWithError:", "getMCLLimitWithError:",
			"isMCLCurrentlyEnabled:", "setMCLLimit:error:", "disableMCL:"} {
			if !objc.Send[bool](client, macos.Sel("respondsToSelector:"), macos.Sel(name)) {
				return
			}
		}
		p.client = client
	})
	return p.client
}

func (p *powerUI) call(fn func(client objc.ID) error) error {
	err := errPowerUIUnavailable
	macos.WithPool(func() {
		if client := p.load(); client != 0 {
			err = fn(client)
		}
	})
	return err
}

func (p *powerUI) read() (nativeLimitState, error) {
	var s nativeLimitState
	err := p.call(func(client objc.ID) error {
		s.supported = p.msgBool(client, macos.Sel("isMCLSupported"))
		if !s.supported {
			return nil
		}
		var nsErr objc.ID
		limits := p.msgIDErr(client, macos.Sel("availableChargeLimitsWithError:"), &nsErr)
		if limits == 0 {
			return nsError(nsErr, "no charge limits available")
		}
		for i := range objc.Send[uint](limits, macos.Sel("count")) {
			n := limits.Send(macos.Sel("objectAtIndex:"), i)
			s.limits = append(s.limits, int(objc.Send[int64](n, macos.Sel("longLongValue"))))
		}
		nsErr = 0
		s.limit = int(p.msgUInt8Err(client, macos.Sel("getMCLLimitWithError:"), &nsErr))
		if nsErr != 0 {
			return nsError(nsErr, "")
		}
		s.enabled = p.msgUInt64(client, macos.Sel("isMCLCurrentlyEnabled:"), &nsErr) != 0
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
			ok = p.msgBoolErr(client, macos.Sel("disableMCL:"), &nsErr)
		} else {
			ok = p.msgSetErr(client, macos.Sel("setMCLLimit:error:"), uint8(percent), &nsErr)
		}
		if !ok {
			return nsError(nsErr, "PowerUIAgent refused the charge limit")
		}
		return nil
	})
}

func nsError(e objc.ID, fallback string) error {
	if e == 0 {
		if fallback == "" {
			fallback = "unknown PowerUI error"
		}
		return errors.New(fallback)
	}
	if msg := macos.GoString(macos.Send(e, "localizedDescription")); msg != "" {
		return errors.New(msg)
	}
	return errors.New(fallback)
}
