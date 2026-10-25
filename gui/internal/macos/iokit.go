//go:build darwin

// Package macos reaches the macOS APIs MyGo doesn't wrap, through purego so the app needs no cgo.
package macos

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

var (
	loadOnce sync.Once
	loadErr  error

	ioServiceMatching                 func(name string) uintptr
	ioServiceGetMatchingServices      func(mainPort uint32, matching uintptr, iterator *uint32) int32
	ioIteratorNext                    func(iterator uint32) uint32
	ioObjectRelease                   func(object uint32) int32
	ioRegistryEntryCreateCFProperties func(entry uint32, properties *uintptr, allocator uintptr, options uint32) int32

	cfPropertyListCreateData func(allocator, plist uintptr, format int, options uint, err uintptr) uintptr
	cfDataGetLength          func(data uintptr) int
	cfDataGetBytePtr         func(data uintptr) *byte
	cfRelease                func(object uintptr)
)

// kCFPropertyListXMLFormat_v1_0: CF objects cross into Go as XML, which saves wrapping every CF type.
const xmlPropertyListFormat = 100

func load() error {
	loadOnce.Do(func() {
		iokit, err := purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			loadErr = fmt.Errorf("load IOKit: %w", err)
			return
		}
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			loadErr = fmt.Errorf("load CoreFoundation: %w", err)
			return
		}
		purego.RegisterLibFunc(&ioServiceMatching, iokit, "IOServiceMatching")
		purego.RegisterLibFunc(&ioServiceGetMatchingServices, iokit, "IOServiceGetMatchingServices")
		purego.RegisterLibFunc(&ioIteratorNext, iokit, "IOIteratorNext")
		purego.RegisterLibFunc(&ioObjectRelease, iokit, "IOObjectRelease")
		purego.RegisterLibFunc(&ioRegistryEntryCreateCFProperties, iokit, "IORegistryEntryCreateCFProperties")
		purego.RegisterLibFunc(&cfPropertyListCreateData, cf, "CFPropertyListCreateData")
		purego.RegisterLibFunc(&cfDataGetLength, cf, "CFDataGetLength")
		purego.RegisterLibFunc(&cfDataGetBytePtr, cf, "CFDataGetBytePtr")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
	})
	return loadErr
}

// ServiceProperties returns the registry properties of every service of an IOKit class, in registry order.
func ServiceProperties(class string) ([]map[string]any, error) {
	if err := load(); err != nil {
		return nil, err
	}
	var iterator uint32
	// IOServiceGetMatchingServices consumes the matching dictionary, so it is never released here.
	if kr := ioServiceGetMatchingServices(0, ioServiceMatching(class), &iterator); kr != 0 {
		return nil, fmt.Errorf("IOServiceGetMatchingServices(%s): %#x", class, kr)
	}
	defer ioObjectRelease(iterator)

	var result []map[string]any
	for {
		service := ioIteratorNext(iterator)
		if service == 0 {
			break
		}
		props, err := properties(service)
		ioObjectRelease(service)
		if err == nil {
			result = append(result, props)
		}
	}
	return result, nil
}

func properties(service uint32) (map[string]any, error) {
	var dict uintptr
	if kr := ioRegistryEntryCreateCFProperties(service, &dict, 0, 0); kr != 0 || dict == 0 {
		return nil, fmt.Errorf("IORegistryEntryCreateCFProperties: %#x", kr)
	}
	defer cfRelease(dict)
	data := cfPropertyListCreateData(0, dict, xmlPropertyListFormat, 0, 0)
	if data == 0 {
		return nil, errors.New("CFPropertyListCreateData failed")
	}
	defer cfRelease(data)
	xml := unsafe.Slice(cfDataGetBytePtr(data), cfDataGetLength(data))
	value, err := ParsePlist(xml)
	if err != nil {
		return nil, err
	}
	dictValue, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("registry properties are not a dictionary")
	}
	return dictValue, nil
}
