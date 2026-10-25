//go:build darwin

package thermal

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"powerfultop/internal/macos"
)

var (
	secOnce sync.Once
	secErr  error

	secCodeCopySelf                func(flags uint32, code *uintptr) int32
	secCodeCopyStaticCode          func(code uintptr, flags uint32, static *uintptr) int32
	secCodeCopySigningInformation  func(static uintptr, flags uint32, info *uintptr) int32
	secCodeCopyGuestWithAttributes func(host, attributes uintptr, flags uint32, guest *uintptr) int32
	secRequirementCreateWithString func(text uintptr, flags uint32, requirement *uintptr) int32
	secCodeCheckValidity           func(code uintptr, flags uint32, requirement uintptr) int32
	cfDictionaryGetValue           func(dict, key uintptr) uintptr
	cfDataGetLength                func(data uintptr) int
	cfDataGetBytePtr               func(data uintptr) *byte
	cfRelease                      func(object uintptr)
	getsockopt                     func(fd, level, name int32, value unsafe.Pointer, length *uint32) int32
)

const (
	kSecCSSigningInformation = 1 << 1
	solLocal                 = 0 // SOL_LOCAL
	localPeerToken           = 6 // LOCAL_PEERTOKEN: the peer's audit_token_t
	auditTokenSize           = 32
)

func loadSecurity() error {
	secOnce.Do(func() {
		security, err := purego.Dlopen("/System/Library/Frameworks/Security.framework/Security", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			secErr = err
			return
		}
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			secErr = err
			return
		}
		system, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			secErr = err
			return
		}
		purego.RegisterLibFunc(&secCodeCopySelf, security, "SecCodeCopySelf")
		purego.RegisterLibFunc(&secCodeCopyStaticCode, security, "SecCodeCopyStaticCode")
		purego.RegisterLibFunc(&secCodeCopySigningInformation, security, "SecCodeCopySigningInformation")
		purego.RegisterLibFunc(&secCodeCopyGuestWithAttributes, security, "SecCodeCopyGuestWithAttributes")
		purego.RegisterLibFunc(&secRequirementCreateWithString, security, "SecRequirementCreateWithString")
		purego.RegisterLibFunc(&secCodeCheckValidity, security, "SecCodeCheckValidity")
		purego.RegisterLibFunc(&cfDictionaryGetValue, cf, "CFDictionaryGetValue")
		purego.RegisterLibFunc(&cfDataGetLength, cf, "CFDataGetLength")
		purego.RegisterLibFunc(&cfDataGetBytePtr, cf, "CFDataGetBytePtr")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
		purego.RegisterLibFunc(&getsockopt, system, "getsockopt")
	})
	return secErr
}

// ClientRequirement pins the helper to this exact build by its cdhash: ad-hoc signatures have no
// team identity to check instead.
func ClientRequirement() (string, error) {
	if err := loadSecurity(); err != nil {
		return "", err
	}
	var code, static, info uintptr
	if secCodeCopySelf(0, &code) != 0 || code == 0 {
		return "", errors.New("SecCodeCopySelf failed")
	}
	defer cfRelease(code)
	if secCodeCopyStaticCode(code, 0, &static) != 0 || static == 0 {
		return "", errors.New("SecCodeCopyStaticCode failed")
	}
	defer cfRelease(static)
	if secCodeCopySigningInformation(static, kSecCSSigningInformation, &info) != 0 || info == 0 {
		return "", errors.New("SecCodeCopySigningInformation failed")
	}
	defer cfRelease(info)
	// The dictionary also holds URLs and code objects, which aren't plist types, so read the one key.
	key := macos.CFString("unique") // kSecCodeInfoUnique
	defer cfRelease(key)
	data := cfDictionaryGetValue(info, key)
	if data == 0 {
		return "", errors.New("the app is not code-signed")
	}
	cdhash := unsafe.Slice(cfDataGetBytePtr(data), cfDataGetLength(data))
	return fmt.Sprintf(`cdhash H"%s"`, hex.EncodeToString(cdhash)), nil
}

// PeerAuditToken returns the audit token of the process at the other end of a Unix socket; unlike
// a pid it can't be recycled by another process between the check and the request.
func PeerAuditToken(conn *net.UnixConn) ([]byte, error) {
	if err := loadSecurity(); err != nil {
		return nil, err
	}
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}
	token := make([]byte, auditTokenSize)
	var callErr error
	err = raw.Control(func(fd uintptr) {
		length := uint32(auditTokenSize)
		if getsockopt(int32(fd), solLocal, localPeerToken, unsafe.Pointer(&token[0]), &length) != 0 || length != auditTokenSize {
			callErr = errors.New("getsockopt(LOCAL_PEERTOKEN) failed")
		}
	})
	if err != nil {
		return nil, err
	}
	return token, callErr
}

// VerifyAuditToken checks that the process behind token satisfies the code requirement text.
func VerifyAuditToken(token []byte, requirement string) error {
	if err := loadSecurity(); err != nil {
		return err
	}
	attributes, err := macos.ToCF(map[string]any{"audit": token}) // kSecGuestAttributeAudit
	if err != nil {
		return err
	}
	defer cfRelease(attributes)
	var guest uintptr
	if status := secCodeCopyGuestWithAttributes(0, attributes, 0, &guest); status != 0 || guest == 0 {
		return fmt.Errorf("unknown client (%d)", status)
	}
	defer cfRelease(guest)
	text := macos.CFString(requirement)
	defer cfRelease(text)
	var req uintptr
	if status := secRequirementCreateWithString(text, 0, &req); status != 0 || req == 0 {
		return fmt.Errorf("invalid client requirement (%d)", status)
	}
	defer cfRelease(req)
	if status := secCodeCheckValidity(guest, 0, req); status != 0 {
		return fmt.Errorf("client rejected (%d)", status)
	}
	return nil
}
