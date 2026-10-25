//go:build darwin

package thermal

import (
	"runtime"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"powerfultop/internal/macos"
)

var (
	poolOnce            sync.Once
	autoreleasePoolPush func() uintptr
	autoreleasePoolPop  func(pool uintptr)
)

// isOtherFanToolRunning asks NSRunningApplication, which is callable from any thread.
func isOtherFanToolRunning() bool {
	poolOnce.Do(func() {
		lib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		// NSRunningApplication lives in AppKit, which only the app itself has loaded already.
		if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			return
		}
		purego.RegisterLibFunc(&autoreleasePoolPush, lib, "objc_autoreleasePoolPush")
		purego.RegisterLibFunc(&autoreleasePoolPop, lib, "objc_autoreleasePoolPop")
	})
	if autoreleasePoolPush == nil || loadSecurity() != nil {
		return false
	}
	// A pool belongs to the thread that pushed it, so the goroutine must not move until it pops.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := autoreleasePoolPush()
	defer autoreleasePoolPop(pool)

	class := objc.ID(objc.GetClass("NSRunningApplication"))
	selector := objc.RegisterName("runningApplicationsWithBundleIdentifier:")
	count := objc.RegisterName("count")
	for _, id := range conflictingBundleIDs {
		name := macos.CFString(id)
		apps := class.Send(selector, objc.ID(name))
		cfRelease(name)
		if apps != 0 && objc.Send[uint](apps, count) > 0 {
			return true
		}
	}
	return false
}
