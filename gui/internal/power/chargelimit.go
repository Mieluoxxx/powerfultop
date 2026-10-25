package power

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/ebitengine/purego/objc"

	"powerfultop/internal/macos"
)

type ChargeLimitSource int

const (
	ChargeLimitNone ChargeLimitSource = iota
	ChargeLimitMacOSSetting
	ChargeLimitOptimizedCharging
	ChargeLimitAlDente
)

type ResolvedChargeLimit struct {
	Percent int
	Source  ChargeLimitSource
}

var FullCharge = ResolvedChargeLimit{Percent: 100, Source: ChargeLimitNone}

const optimizedChargingPauseReason = 0x0100_0000

// ResolveChargeLimit returns the effective charge target for time-to-full estimates.
func ResolveChargeLimit(dailyMaxSoc, notChargingReason *int) ResolvedChargeLimit {
	return resolveChargeLimit(readMacOSChargeLimit(), readAlDenteChargeLimit(), dailyMaxSoc, notChargingReason)
}

func resolveChargeLimit(macOS, alDente, dailyMaxSoc, notChargingReason *int) ResolvedChargeLimit {
	var best *ResolvedChargeLimit
	for _, c := range []struct {
		limit  *int
		source ChargeLimitSource
	}{{macOS, ChargeLimitMacOSSetting}, {alDente, ChargeLimitAlDente}} {
		if c.limit != nil && (best == nil || *c.limit < best.Percent) {
			best = &ResolvedChargeLimit{Percent: *c.limit, Source: c.source}
		}
	}
	if best != nil {
		return *best
	}
	if notChargingReason != nil && *notChargingReason&optimizedChargingPauseReason != 0 &&
		dailyMaxSoc != nil && *dailyMaxSoc >= 1 && *dailyMaxSoc < 100 {
		return ResolvedChargeLimit{Percent: *dailyMaxSoc, Source: ChargeLimitOptimizedCharging}
	}
	return FullCharge
}

func ChargeLimitSourceLabel(source ChargeLimitSource) string {
	switch source {
	case ChargeLimitMacOSSetting:
		return l("macOS setting", "系统设置")
	case ChargeLimitOptimizedCharging:
		return l("Optimized Charging", "优化充电")
	case ChargeLimitAlDente:
		return "AlDente"
	}
	return ""
}

var chargeSocLimit = regexp.MustCompile(`chargeSocLimitSoc\s*=\s*(\d+);`)

// ParsePowerdChargeLimit reads `pmset -g battlimit`, which lists the SOC limits powerd enforces;
// the strictest one applies.
func ParsePowerdChargeLimit(output string) *int {
	var strictest *int
	for _, m := range chargeSocLimit.FindAllStringSubmatch(output, -1) {
		v, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if n := normalizedChargeLimit(&v); n != nil && (strictest == nil || *n < *strictest) {
			strictest = n
		}
	}
	return strictest
}

func normalizedChargeLimit(v *int) *int {
	if v == nil || *v < 1 || *v >= 100 {
		return nil
	}
	return v
}

// powerd enforces the native limit; `prior.limit` in the batteryui domain lingers after the limit
// is switched off, so it can't tell whether a limit is active. Spawning pmset on every 2 s
// refresh is wasteful and the limit only changes when the user edits it, so it is cached.
var powerdLimitCache struct {
	sync.Mutex
	limit  *int
	readAt time.Time
}

const powerdLimitLifetime = 30 * time.Second

func invalidateMacOSChargeLimit() {
	powerdLimitCache.Lock()
	defer powerdLimitCache.Unlock()
	powerdLimitCache.readAt = time.Time{}
}

func readMacOSChargeLimit() *int {
	powerdLimitCache.Lock()
	defer powerdLimitCache.Unlock()
	if time.Since(powerdLimitCache.readAt) < powerdLimitLifetime {
		return powerdLimitCache.limit
	}
	out, err := exec.Command("/usr/bin/pmset", "-g", "battlimit").Output()
	powerdLimitCache.limit = nil
	if err == nil {
		powerdLimitCache.limit = ParsePowerdChargeLimit(string(out))
	}
	powerdLimitCache.readAt = time.Now()
	return powerdLimitCache.limit
}

const (
	alDenteProBundleID = "com.apphousekitchen.aldente-pro"
	alDenteBundleID    = "com.davidwernhart.AlDente"
)

// AlDenteRunning reports whether either AlDente edition runs; it keeps its last limit in
// preferences after it quits, so only a running AlDente's limit counts.
func AlDenteRunning() bool {
	return isRunning(alDenteProBundleID) || isRunning(alDenteBundleID)
}

func readAlDenteChargeLimit() *int {
	if !AlDenteRunning() {
		return nil
	}
	if useNative, ok := preferenceBool(alDenteProBundleID, "useTahoeNativeLimit"); !(ok && useNative) {
		if limit := normalizedChargeLimit(preferenceNumber(alDenteProBundleID, "chargeVal")); limit != nil {
			return limit
		}
	}
	return normalizedChargeLimit(preferenceNumber(alDenteBundleID, "chargeVal"))
}

func isRunning(bundleID string) bool {
	// Autoreleased objects belong to the thread's pool, which must be popped on the thread that pushed it.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	cstr := append([]byte(bundleID), 0)
	id := objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), &cstr[0])
	runtime.KeepAlive(cstr)
	apps := objc.ID(objc.GetClass("NSRunningApplication")).Send(objc.RegisterName("runningApplicationsWithBundleIdentifier:"), id)
	return apps != 0 && objc.Send[uint](apps, objc.RegisterName("count")) > 0
}

// Third-party apps may write their plists directly without cfprefsd seeing it, so the file is a fallback.
func preferenceValue(domain, key string) (any, bool) {
	if v, ok := macos.DomainPreferences(domain).Value(key); ok {
		return v, true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, false
	}
	path := filepath.Join(home, "Library/Preferences", domain+".plist")
	if _, err := os.Stat(path); err != nil {
		return nil, false
	}
	// Preference files are usually binary plists, which plutil turns into the XML ParsePlist reads.
	xml, err := exec.Command("/usr/bin/plutil", "-convert", "xml1", "-o", "-", path).Output()
	if err != nil {
		return nil, false
	}
	parsed, err := macos.ParsePlist(xml)
	dict, ok := parsed.(map[string]any)
	if err != nil || !ok {
		return nil, false
	}
	v, ok := dict[key]
	return v, ok
}

func preferenceNumber(domain, key string) *int {
	v, ok := preferenceValue(domain, key)
	if !ok {
		return nil
	}
	var n int
	switch x := v.(type) {
	case int64:
		n = int(x)
	case float64:
		n = int(math.Round(x))
	case bool:
		if x {
			n = 1
		}
	default:
		return nil
	}
	return &n
}

func preferenceBool(domain, key string) (bool, bool) {
	v, ok := preferenceValue(domain, key)
	if !ok {
		return false, false
	}
	switch x := v.(type) {
	case bool:
		return x, true
	case int64:
		return x != 0, true
	case float64:
		return x != 0, true
	}
	return false, false
}
