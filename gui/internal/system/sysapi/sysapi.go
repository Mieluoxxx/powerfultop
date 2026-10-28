//go:build darwin

// Package sysapi binds the libSystem, IOKit and CoreFoundation calls the system and display
// packages share, through purego so the app needs no cgo.
package sysapi

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"powerfultop/internal/macos"
)

var (
	loadOnce sync.Once
	loadErr  error

	libSystem uintptr
	iokit     uintptr

	sysctlbyname       func(name string, oldp unsafe.Pointer, oldlenp *uintptr, newp unsafe.Pointer, newlen uintptr) int32
	getloadavg         func(loadavg *float64, nelem int32) int32
	machHostSelf       func() uint32
	hostProcessorInfo  func(host uint32, flavor int32, count *uint32, info *unsafe.Pointer, infoCount *uint32) int32
	hostStatistics64   func(host uint32, flavor int32, info unsafe.Pointer, count *uint32) int32
	vmDeallocate       func(task uint32, address uintptr, size uintptr) int32
	machTaskSelfGlobal uintptr

	ioRegistryEntryFromPath         func(mainPort uint32, path string) uint32
	ioRegistryEntryGetChildIterator func(entry uint32, plane string, iterator *uint32) int32
	ioRegistryGetRootEntry          func(mainPort uint32) uint32
	ioRegistryEntryCreateIterator   func(entry uint32, plane string, options uint32, iterator *uint32) int32
	ioRegistryEntryGetName          func(entry uint32, name *byte) int32
	ioRegistryEntryCreateCFProperty func(entry uint32, key uintptr, allocator uintptr, options uint32) uintptr
	ioIteratorNext                  func(iterator uint32) uint32
	ioObjectRelease                 func(object uint32) int32
	cfRelease                       func(object uintptr)
)

func load() error {
	loadOnce.Do(func() {
		var err error
		if libSystem, err = purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			loadErr = fmt.Errorf("load libSystem: %w", err)
			return
		}
		if iokit, err = purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			loadErr = fmt.Errorf("load IOKit: %w", err)
			return
		}
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			loadErr = fmt.Errorf("load CoreFoundation: %w", err)
			return
		}
		purego.RegisterLibFunc(&sysctlbyname, libSystem, "sysctlbyname")
		purego.RegisterLibFunc(&getloadavg, libSystem, "getloadavg")
		purego.RegisterLibFunc(&machHostSelf, libSystem, "mach_host_self")
		purego.RegisterLibFunc(&hostProcessorInfo, libSystem, "host_processor_info")
		purego.RegisterLibFunc(&hostStatistics64, libSystem, "host_statistics64")
		purego.RegisterLibFunc(&vmDeallocate, libSystem, "vm_deallocate")
		// mach_task_self() is a macro over this global.
		if machTaskSelfGlobal, err = purego.Dlsym(libSystem, "mach_task_self_"); err != nil {
			loadErr = err
			return
		}
		purego.RegisterLibFunc(&ioRegistryEntryFromPath, iokit, "IORegistryEntryFromPath")
		purego.RegisterLibFunc(&ioRegistryEntryGetChildIterator, iokit, "IORegistryEntryGetChildIterator")
		purego.RegisterLibFunc(&ioRegistryGetRootEntry, iokit, "IORegistryGetRootEntry")
		purego.RegisterLibFunc(&ioRegistryEntryCreateIterator, iokit, "IORegistryEntryCreateIterator")
		purego.RegisterLibFunc(&ioRegistryEntryGetName, iokit, "IORegistryEntryGetName")
		purego.RegisterLibFunc(&ioRegistryEntryCreateCFProperty, iokit, "IORegistryEntryCreateCFProperty")
		purego.RegisterLibFunc(&ioIteratorNext, iokit, "IOIteratorNext")
		purego.RegisterLibFunc(&ioObjectRelease, iokit, "IOObjectRelease")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
	})
	return loadErr
}

// IOKitHandle is the dlopen handle of IOKit, for packages resolving their own symbols in it.
func IOKitHandle() uintptr {
	if load() != nil {
		return 0
	}
	return iokit
}

// Sysctl reads a sysctl into dst, which must point at a value of the kernel's layout.
func Sysctl[T any](name string, dst *T) error {
	if err := load(); err != nil {
		return err
	}
	size := unsafe.Sizeof(*dst)
	if sysctlbyname(name, unsafe.Pointer(dst), &size, nil, 0) != 0 {
		return fmt.Errorf("sysctl %s failed", name)
	}
	return nil
}

// LoadAverage returns the 1, 5 and 15 minute load averages.
func LoadAverage() ([]float64, bool) {
	if load() != nil {
		return nil, false
	}
	avg := make([]float64, 3)
	if getloadavg(&avg[0], 3) != 3 {
		return nil, false
	}
	return avg, true
}

// CPUStateMax is CPU_STATE_MAX: user, system, idle and nice ticks per core.
const CPUStateMax = 4

// ProcessorLoadTicks returns host_processor_info(PROCESSOR_CPU_LOAD_INFO) as CPUStateMax counters per core.
func ProcessorLoadTicks() ([][CPUStateMax]uint32, bool) {
	if load() != nil {
		return nil, false
	}
	const processorCPULoadInfo = 2
	var count, infoCount uint32
	var info unsafe.Pointer
	if hostProcessorInfo(machHostSelf(), processorCPULoadInfo, &count, &info, &infoCount) != 0 || info == nil {
		return nil, false
	}
	task := *(*uint32)(unsafe.Add(nil, machTaskSelfGlobal))
	defer vmDeallocate(task, uintptr(info), uintptr(infoCount)*4)
	raw := unsafe.Slice((*uint32)(info), int(infoCount))
	ticks := make([][CPUStateMax]uint32, count)
	for core := range ticks {
		copy(ticks[core][:], raw[core*CPUStateMax:(core+1)*CPUStateMax])
	}
	return ticks, true
}

// VMStatistics64 mirrors struct vm_statistics64 from <mach/vm_statistics.h>.
type VMStatistics64 struct {
	FreeCount                          uint32
	ActiveCount                        uint32
	InactiveCount                      uint32
	WireCount                          uint32
	ZeroFillCount                      uint64
	Reactivations                      uint64
	Pageins                            uint64
	Pageouts                           uint64
	Faults                             uint64
	CowFaults                          uint64
	Lookups                            uint64
	Hits                               uint64
	Purges                             uint64
	PurgeableCount                     uint32
	SpeculativeCount                   uint32
	Decompressions                     uint64
	Compressions                       uint64
	Swapins                            uint64
	Swapouts                           uint64
	CompressorPageCount                uint32
	ThrottledCount                     uint32
	ExternalPageCount                  uint32
	InternalPageCount                  uint32
	TotalUncompressedPagesInCompressor uint64
}

// HostVMInfo64 returns host_statistics64(HOST_VM_INFO64).
func HostVMInfo64() (VMStatistics64, bool) {
	var stats VMStatistics64
	if load() != nil {
		return stats, false
	}
	const hostVMInfo64 = 4
	count := uint32(unsafe.Sizeof(stats) / 4)
	return stats, hostStatistics64(machHostSelf(), hostVMInfo64, unsafe.Pointer(&stats), &count) == 0
}

// Entry is a retained IORegistry entry or iterator; Release it.
type Entry uint32

func (e Entry) Release() {
	if e != 0 {
		ioObjectRelease(uint32(e))
	}
}

// EntryFromPath looks an entry up by path, e.g. "IODeviceTree:/cpus".
func EntryFromPath(path string) Entry {
	if load() != nil {
		return 0
	}
	return Entry(ioRegistryEntryFromPath(0, path))
}

// RootEntry is the registry's root.
func RootEntry() Entry {
	if load() != nil {
		return 0
	}
	return Entry(ioRegistryGetRootEntry(0))
}

// Children iterates the entry's children in a plane, e.g. "IODeviceTree".
func (e Entry) Children(plane string) Entry {
	var iterator uint32
	if ioRegistryEntryGetChildIterator(uint32(e), plane, &iterator) != 0 {
		return 0
	}
	return Entry(iterator)
}

// Descendants iterates every entry under e in a plane, depth first in registry order.
func (e Entry) Descendants(plane string) Entry {
	const iterateRecursively = 1
	var iterator uint32
	if ioRegistryEntryCreateIterator(uint32(e), plane, iterateRecursively, &iterator) != 0 {
		return 0
	}
	return Entry(iterator)
}

// Next returns the iterator's next entry, 0 at the end; Release each.
func (e Entry) Next() Entry { return Entry(ioIteratorNext(uint32(e))) }

// Name is the entry's name in the registry.
func (e Entry) Name() string {
	var buf [128]byte
	if ioRegistryEntryGetName(uint32(e), &buf[0]) != 0 {
		return ""
	}
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return string(buf[:n])
}

// Property reads one property as Go values (see macos.ParsePlist).
func (e Entry) Property(key string) (any, bool) {
	k := macos.CFString(key)
	if k == 0 {
		return nil, false
	}
	defer cfRelease(k)
	ref := ioRegistryEntryCreateCFProperty(uint32(e), k, 0, 0)
	if ref == 0 {
		return nil, false
	}
	defer cfRelease(ref)
	v, err := macos.FromCF(ref)
	return v, err == nil
}

// CFRelease releases a CF object returned by a Create or Copy function.
func CFRelease(ref uintptr) {
	if ref != 0 && load() == nil {
		cfRelease(ref)
	}
}
