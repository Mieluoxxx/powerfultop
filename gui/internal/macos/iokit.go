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

	cfPropertyListCreateWithData func(allocator, data uintptr, options uint, format, err uintptr) uintptr
	cfDataCreate                 func(allocator uintptr, bytes []byte, length int) uintptr
	cfStringCreateWithCString    func(allocator uintptr, s string, encoding uint32) uintptr
	cfPreferencesCopyAppValue    func(key, app uintptr) uintptr
	cfPreferencesSetAppValue     func(key, value, app uintptr)
	cfPreferencesAppSynchronize  func(app uintptr) bool
	cfBundleGetMainBundle        func() uintptr
	cfBundleGetIdentifier        func(bundle uintptr) uintptr
	cfPreferencesCopyKeyList     func(app, user, host uintptr) uintptr

	kCFPreferencesCurrentUser uintptr
	kCFPreferencesAnyHost     uintptr
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
		purego.RegisterLibFunc(&cfPropertyListCreateWithData, cf, "CFPropertyListCreateWithData")
		purego.RegisterLibFunc(&cfDataCreate, cf, "CFDataCreate")
		purego.RegisterLibFunc(&cfStringCreateWithCString, cf, "CFStringCreateWithCString")
		purego.RegisterLibFunc(&cfPreferencesCopyAppValue, cf, "CFPreferencesCopyAppValue")
		purego.RegisterLibFunc(&cfPreferencesSetAppValue, cf, "CFPreferencesSetAppValue")
		purego.RegisterLibFunc(&cfPreferencesAppSynchronize, cf, "CFPreferencesAppSynchronize")
		purego.RegisterLibFunc(&cfBundleGetMainBundle, cf, "CFBundleGetMainBundle")
		purego.RegisterLibFunc(&cfBundleGetIdentifier, cf, "CFBundleGetIdentifier")
		purego.RegisterLibFunc(&cfPreferencesCopyKeyList, cf, "CFPreferencesCopyKeyList")
		// These are CFStringRef globals, so the symbol is the address of the pointer.
		for name, dst := range map[string]*uintptr{
			"kCFPreferencesCurrentUser": &kCFPreferencesCurrentUser,
			"kCFPreferencesAnyHost":     &kCFPreferencesAnyHost,
		} {
			if sym, err := purego.Dlsym(cf, name); err == nil {
				*dst = **(**uintptr)(unsafe.Pointer(&sym))
			}
		}
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
	value, err := FromCF(dict)
	if err != nil {
		return nil, err
	}
	dictValue, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("registry properties are not a dictionary")
	}
	return dictValue, nil
}

// FromCF converts any property-list CF object (dictionary, array, string, number, boolean, data,
// date) to Go values, as ParsePlist returns them. It does not release ref.
func FromCF(ref uintptr) (any, error) {
	if err := load(); err != nil {
		return nil, err
	}
	data := cfPropertyListCreateData(0, ref, xmlPropertyListFormat, 0, 0)
	if data == 0 {
		return nil, errors.New("CFPropertyListCreateData failed")
	}
	defer cfRelease(data)
	return ParsePlist(unsafe.Slice(cfDataGetBytePtr(data), cfDataGetLength(data)))
}

// ToCF converts Go values to a CF property-list object, which the caller releases.
func ToCF(value any) (uintptr, error) {
	if err := load(); err != nil {
		return 0, err
	}
	xml, err := EncodePlist(value)
	if err != nil {
		return 0, err
	}
	data := cfDataCreate(0, xml, len(xml))
	if data == 0 {
		return 0, errors.New("CFDataCreate failed")
	}
	defer cfRelease(data)
	ref := cfPropertyListCreateWithData(0, data, 0, 0, 0)
	if ref == 0 {
		return 0, errors.New("CFPropertyListCreateWithData failed")
	}
	return ref, nil
}

// CFString returns a CFString the caller releases.
func CFString(s string) uintptr {
	if load() != nil {
		return 0
	}
	const utf8 = 0x08000100 // kCFStringEncodingUTF8
	return cfStringCreateWithCString(0, s, utf8)
}

// MainBundleID is the running app's bundle identifier, "" outside a bundle (as in tests).
func MainBundleID() string {
	if load() != nil {
		return ""
	}
	bundle := cfBundleGetMainBundle()
	if bundle == 0 {
		return ""
	}
	id := cfBundleGetIdentifier(bundle)
	if id == 0 {
		return ""
	}
	value, _ := FromCF(id)
	s, _ := value.(string)
	return s
}
