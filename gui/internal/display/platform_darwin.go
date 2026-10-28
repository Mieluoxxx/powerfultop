//go:build darwin

package display

import (
	"sync"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"powerfultop/internal/macos"
	"powerfultop/internal/system/sysapi"
)

var (
	bindOnce sync.Once

	cgGetOnlineDisplayList            func(max uint32, ids *uint32, count *uint32) int32
	cgDisplayMirrorsDisplay           func(id uint32) uint32
	cgDisplayIsBuiltin                func(id uint32) int32
	cgDisplayVendorNumber             func(id uint32) uint32
	cgDisplayModelNumber              func(id uint32) uint32
	cgDisplaySerialNumber             func(id uint32) uint32
	cgDisplayGammaTableCapacity       func(id uint32) uint32
	cgGetDisplayTransferByTable       func(id, capacity uint32, red, green, blue *float32, count *uint32) int32
	cgSetDisplayTransferByTable       func(id, size uint32, red, green, blue *float32) int32
	cgDisplayRestoreColorSyncSettings func()

	// Resolved at run time so a missing private symbol only disables its path.
	displayServicesGetBrightness func(id uint32, value *float32) int32
	displayServicesSetBrightness func(id uint32, value float32) int32
	avServiceCreateWithService   func(allocator uintptr, service uint32) uintptr
	avServiceWriteI2C            func(service uintptr, chip, offset uint32, buf *byte, length uint32) int32
	avServiceReadI2C             func(service uintptr, chip, offset uint32, buf *byte, length uint32) int32
)

func bind() {
	bindOnce.Do(func() {
		if cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
			purego.RegisterLibFunc(&cgGetOnlineDisplayList, cg, "CGGetOnlineDisplayList")
			purego.RegisterLibFunc(&cgDisplayMirrorsDisplay, cg, "CGDisplayMirrorsDisplay")
			purego.RegisterLibFunc(&cgDisplayIsBuiltin, cg, "CGDisplayIsBuiltin")
			purego.RegisterLibFunc(&cgDisplayVendorNumber, cg, "CGDisplayVendorNumber")
			purego.RegisterLibFunc(&cgDisplayModelNumber, cg, "CGDisplayModelNumber")
			purego.RegisterLibFunc(&cgDisplaySerialNumber, cg, "CGDisplaySerialNumber")
			purego.RegisterLibFunc(&cgDisplayGammaTableCapacity, cg, "CGDisplayGammaTableCapacity")
			purego.RegisterLibFunc(&cgGetDisplayTransferByTable, cg, "CGGetDisplayTransferByTable")
			purego.RegisterLibFunc(&cgSetDisplayTransferByTable, cg, "CGSetDisplayTransferByTable")
			purego.RegisterLibFunc(&cgDisplayRestoreColorSyncSettings, cg, "CGDisplayRestoreColorSyncSettings")
		}
		if ds, err := purego.Dlopen("/System/Library/PrivateFrameworks/DisplayServices.framework/DisplayServices", purego.RTLD_LAZY); err == nil {
			optional(&displayServicesGetBrightness, ds, "DisplayServicesGetBrightness")
			optional(&displayServicesSetBrightness, ds, "DisplayServicesSetBrightness")
		}
		if iokit := sysapi.IOKitHandle(); iokit != 0 {
			optional(&avServiceCreateWithService, iokit, "IOAVServiceCreateWithService")
			optional(&avServiceWriteI2C, iokit, "IOAVServiceWriteI2C")
			optional(&avServiceReadI2C, iokit, "IOAVServiceReadI2C")
		}
	})
}

func optional(fn any, lib uintptr, name string) {
	if _, err := purego.Dlsym(lib, name); err == nil {
		purego.RegisterLibFunc(fn, lib, name)
	}
}

func onlineDisplayList() []uint32 {
	bind()
	if cgGetOnlineDisplayList == nil {
		return nil
	}
	ids := make([]uint32, 16)
	var count uint32
	if cgGetOnlineDisplayList(uint32(len(ids)), &ids[0], &count) != 0 {
		return nil
	}
	return ids[:count]
}

func mirrorsDisplay(id uint32) uint32 { bind(); return cgDisplayMirrorsDisplay(id) }
func isBuiltIn(id uint32) bool        { bind(); return cgDisplayIsBuiltin(id) != 0 }
func vendorNumber(id uint32) uint32   { bind(); return cgDisplayVendorNumber(id) }
func modelNumber(id uint32) uint32    { bind(); return cgDisplayModelNumber(id) }
func serialNumber(id uint32) uint32   { bind(); return cgDisplaySerialNumber(id) }

func restoreColorSync() {
	bind()
	if cgDisplayRestoreColorSyncSettings != nil {
		cgDisplayRestoreColorSyncSettings()
	}
}

func nativeBrightness(id uint32) (float64, bool) {
	bind()
	if displayServicesGetBrightness == nil {
		return 0, false
	}
	value := float32(-1)
	if displayServicesGetBrightness(id, &value) != 0 || value < 0 {
		return 0, false
	}
	return float64(value), true
}

func setNativeBrightness(id uint32, level float64) {
	bind()
	if displayServicesSetBrightness != nil {
		displayServicesSetBrightness(id, float32(level))
	}
}

// MARK: gamma

type gammaTable struct {
	red, green, blue []float32
}

func readGamma(id uint32) *gammaTable {
	bind()
	capacity := cgDisplayGammaTableCapacity(id)
	if capacity == 0 {
		return nil
	}
	t := &gammaTable{make([]float32, capacity), make([]float32, capacity), make([]float32, capacity)}
	var count uint32
	if cgGetDisplayTransferByTable(id, capacity, &t.red[0], &t.green[0], &t.blue[0], &count) != 0 || count == 0 {
		return nil
	}
	t.red, t.green, t.blue = t.red[:count], t.green[:count], t.blue[:count]
	return t
}

func (t *gammaTable) scaled(factor float64) *gammaTable {
	f := float32(factor)
	scale := func(values []float32) []float32 {
		out := make([]float32, len(values))
		for i, v := range values {
			out[i] = v * f
		}
		return out
	}
	return &gammaTable{scale(t.red), scale(t.green), scale(t.blue)}
}

func (t *gammaTable) apply(id uint32) {
	if len(t.red) > 0 {
		cgSetDisplayTransferByTable(id, uint32(len(t.red)), &t.red[0], &t.green[0], &t.blue[0])
	}
}

// MARK: DDC

// ddcLink is one external monitor's I²C channel. Writes are serialised and coalesced: a slider drag
// produces far more values than DDC (tens of milliseconds per transaction) can deliver.
type ddcLink struct {
	service  uintptr
	location int
	maxValue uint16

	mu      sync.Mutex
	level   float64
	pending *uint16
	writing bool
}

func (l *ddcLink) currentLevel() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

func (l *ddcLink) release() { sysapi.CFRelease(l.service) }

func (l *ddcLink) send(level float64) {
	value := uint16(level*float64(l.maxValue) + 0.5)
	l.mu.Lock()
	l.level = level
	l.pending = &value
	start := !l.writing
	l.writing = true
	l.mu.Unlock()
	if !start {
		return
	}
	go func() {
		for {
			l.mu.Lock()
			if l.pending == nil {
				l.writing = false
				l.mu.Unlock()
				return
			}
			value := *l.pending
			l.pending = nil
			l.mu.Unlock()
			transact(l.service, setVCPPacket(vcpLuminance, value), 0)
		}
	}()
}

type externalService struct {
	service     uintptr
	location    int
	edidUUID    string
	productName string
}

// connectDDC finds the external DCPAVServiceProxy whose framebuffer carries the display's EDID and
// asks it for VCP 0x10; a monitor with DDC/CI disabled answers with null messages and yields nil.
func connectDDC(edidPrefix, productName string, claimed map[int]bool) *ddcLink {
	bind()
	if avServiceCreateWithService == nil || avServiceWriteI2C == nil || avServiceReadI2C == nil {
		return nil
	}
	all := externalServices()
	var candidates []externalService
	for _, s := range all {
		if len(s.edidUUID) >= len(edidPrefix) && s.edidUUID[:len(edidPrefix)] == edidPrefix && !claimed[s.location] {
			candidates = append(candidates, s)
		}
	}
	// The exact product name first, so two identical monitors keep their own links.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].productName == productName && candidates[j-1].productName != productName; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}
	var link *ddcLink
	for _, c := range candidates {
		if reply := transact(c.service, getVCPPacket(vcpLuminance), 11); reply != nil {
			if current, max, ok := parseVCPReply(reply, vcpLuminance); ok {
				link = &ddcLink{service: c.service, location: c.location, maxValue: max, level: float64(current) / float64(max)}
				break
			}
		}
	}
	for _, s := range all {
		if link == nil || s.service != link.service {
			sysapi.CFRelease(s.service)
		}
	}
	return link
}

// externalServices walks the service plane, where each framebuffer precedes the AV service proxy
// that drives its port.
func externalServices() []externalService {
	root := sysapi.RootEntry()
	if root == 0 {
		return nil
	}
	defer root.Release()
	iterator := root.Descendants("IOService")
	if iterator == 0 {
		return nil
	}
	defer iterator.Release()
	var services []externalService
	var edidUUID, productName string
	location := 0
	for entry := iterator.Next(); entry != 0; entry = iterator.Next() {
		switch entry.Name() {
		case "AppleCLCD2", "IOMobileFramebufferShim":
			location++
			edidUUID, productName = "", ""
			if v, ok := entry.Property("EDID UUID"); ok {
				edidUUID, _ = v.(string)
			}
			if v, ok := entry.Property("DisplayAttributes"); ok {
				if attrs, ok := v.(map[string]any); ok {
					if product, ok := attrs["ProductAttributes"].(map[string]any); ok {
						productName, _ = product["ProductName"].(string)
					}
				}
			}
		case "DCPAVServiceProxy":
			if v, ok := entry.Property("Location"); ok && v == "External" {
				if service := avServiceCreateWithService(0, uint32(entry)); service != 0 {
					services = append(services, externalService{service, location, edidUUID, productName})
				}
			}
		}
		entry.Release()
	}
	return services
}

// transact retries with MonitorControl's timings: monitors drop transactions that arrive back to back.
func transact(service uintptr, packet []byte, replyLength int) []byte {
	packet = append([]byte(nil), packet...)
	for range 5 {
		written := false
		for range 2 {
			time.Sleep(10 * time.Millisecond)
			written = avServiceWriteI2C(service, ddcChipAddress, ddcDataAddress, &packet[0], uint32(len(packet))) == 0
		}
		if replyLength == 0 {
			if written {
				return []byte{}
			}
		} else {
			time.Sleep(50 * time.Millisecond)
			reply := make([]byte, replyLength)
			if avServiceReadI2C(service, ddcChipAddress, 0, &reply[0], uint32(replyLength)) == 0 {
				if _, _, ok := parseVCPReply(reply, packet[2]); ok {
					return reply
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil
}

// MARK: screens

// screenNames maps display IDs to NSScreen.localizedName.
func screenNames() map[uint32]string {
	bind()
	names := map[uint32]string{}
	macos.WithPool(func() {
		key := macos.NSString("NSScreenNumber")
		screens := macos.Send(macos.Class("NSScreen"), "screens")
		for i := range objc.Send[uint64](screens, macos.Sel("count")) {
			screen := macos.Send(screens, "objectAtIndex:", i)
			number := macos.Send(macos.Send(screen, "deviceDescription"), "objectForKey:", key)
			if number == 0 {
				continue
			}
			id := objc.Send[uint32](number, macos.Sel("unsignedIntValue"))
			if name := macos.GoString(macos.Send(screen, "localizedName")); name != "" {
				names[id] = name
			}
		}
	})
	return names
}

// observeScreenChanges calls fn on the main queue after display parameters change, as
// NSApplication.didChangeScreenParametersNotification does; the returned func removes the observer.
func observeScreenChanges(fn func()) func() {
	center := macos.Send(macos.Class("NSNotificationCenter"), "defaultCenter")
	queue := macos.Send(macos.Class("NSOperationQueue"), "mainQueue")
	name := macos.NSString("NSApplicationDidChangeScreenParametersNotification")
	if center == 0 || queue == 0 {
		return func() {}
	}
	block := objc.NewBlock(func(_ objc.Block, _ objc.ID) { fn() })
	token := macos.Send(center, "addObserverForName:object:queue:usingBlock:", name, objc.ID(0), queue, block)
	macos.Send(token, "retain")
	return func() {
		macos.Send(center, "removeObserver:", token)
		macos.Send(token, "release")
		block.Release()
	}
}
