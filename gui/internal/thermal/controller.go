package thermal

import (
	"fmt"
	"math"
	"os"
	"time"
)

// The Ftst unlock and FnMd/FnTg handoff follow the research in macos-smc-fan
// (https://github.com/agoodkind/macos-smc-fan, MIT License, © 2026 Alexander Goodkind).

// FanController is the helper's root-only SMC writer. It accepts fan keys exclusively and always
// fails towards macOS control. It is not safe for concurrent use; the helper serializes calls.
type FanController struct {
	smc              SMC
	markerPath       string
	fanCount         int
	modeKeys         []string
	failsafeKeys     []string
	hasDiagnosticKey bool
	engaged          bool
	lastRequest      time.Time
	cooldownUntil    time.Time
	// overheatTicks counts consecutive 1 s ticks at or above the failsafe; a single core spike must not trip it.
	overheatTicks int

	now   func() time.Time
	sleep func(time.Duration)
}

// NewFanController reads the fan layout and, if a marker survived a crash, hands the fans back first.
func NewFanController(smc SMC, markerPath string) *FanController {
	c := &FanController{smc: smc, markerPath: markerPath, now: time.Now, sleep: time.Sleep}
	count, _ := readFloat(smc, "FNum")
	c.fanCount = int(count)
	for i := range c.fanCount {
		// M1–M4 use F%dMd; M5 renamed it to F%dmd.
		key := fmt.Sprintf("F%dMd", i)
		if _, ok := smc.Read(key); !ok {
			key = fmt.Sprintf("F%dmd", i)
		}
		c.modeKeys = append(c.modeKeys, key)
	}
	_, c.hasDiagnosticKey = smc.Read("Ftst")
	for i := range smc.KeyCount() {
		if key, ok := smc.KeyAt(i); ok && (GroupCPU.Matches(key) || GroupGPU.Matches(key)) {
			c.failsafeKeys = append(c.failsafeKeys, key)
		}
	}
	if _, err := os.Stat(markerPath); err == nil {
		c.RestoreAutomatic()
	}
	return c
}

func (c *FanController) Engaged() bool { return c.engaged }

// Apply sets every fan's target, engaging manual mode first if needed.
func (c *FanController) Apply(rpms []float64) error {
	c.lastRequest = c.now()
	if c.now().Before(c.cooldownUntil) {
		return fmt.Errorf("fan control paused after overheating")
	}
	if c.fanCount == 0 {
		return fmt.Errorf("no fans found")
	}
	if !c.engaged {
		if err := c.engage(); err != nil {
			c.RestoreAutomatic()
			return err
		}
	}
	for i := range min(c.fanCount, len(rpms)) {
		lo, okMin := readFloat(c.smc, fmt.Sprintf("F%dMn", i))
		hi, okMax := readFloat(c.smc, fmt.Sprintf("F%dMx", i))
		if !okMin || !okMax || hi <= lo {
			continue
		}
		c.smc.Write(fmt.Sprintf("F%dTg", i), FloatBytes(math.Min(math.Max(rpms[i], lo), hi)))
	}
	return nil
}

// RestoreAutomatic hands every fan back to macOS.
func (c *FanController) RestoreAutomatic() {
	for _, key := range c.modeKeys {
		c.smc.Write(key, []byte{0})
	}
	// Clearing Ftst returns fans to system mode even where the mode write was rejected.
	if c.hasDiagnosticKey {
		c.smc.Write("Ftst", []byte{0})
	}
	c.engaged = false
	_ = os.Remove(c.markerPath)
}

// Tick runs once a second: it hands the fans back when the app stops checking in or the chip overheats.
func (c *FanController) Tick() {
	if !c.engaged {
		return
	}
	if c.now().Sub(c.lastRequest) > HeartbeatTimeout {
		c.RestoreAutomatic()
		return
	}
	hottest := 0.0
	for _, key := range c.failsafeKeys {
		if v, ok := readFloat(c.smc, key); ok && IsPlausibleTemperature(v) {
			hottest = math.Max(hottest, v)
		}
	}
	if hottest >= FailsafeCelsius {
		c.overheatTicks++
	} else {
		c.overheatTicks = 0
	}
	if c.overheatTicks >= 3 {
		c.overheatTicks = 0
		c.RestoreAutomatic()
		c.cooldownUntil = c.now().Add(time.Minute)
	}
}

// engage puts the fans in manual mode. M3+ firmware rejects manual mode while thermalmonitord holds
// the fans (mode 3); Ftst = 1 makes it yield, after which the mode write lands within a few seconds.
func (c *FanController) engage() error {
	// Written before the unlock so a crash mid-engage still leads to a restore on the next start.
	if f, err := os.Create(c.markerPath); err == nil {
		f.Close()
	}
	if c.hasDiagnosticKey {
		if result, ok := c.smc.Write("Ftst", []byte{1}); !ok || result != 0 {
			return fmt.Errorf("SMC rejected the diagnostic unlock")
		}
	}
	deadline := c.now().Add(EngageTimeout)
	pending := map[string]bool{}
	for _, key := range c.modeKeys {
		pending[key] = true
	}
	for len(pending) > 0 && c.now().Before(deadline) {
		for key := range pending {
			if result, ok := c.smc.Write(key, []byte{1}); ok && result == 0 {
				if v, ok := readFloat(c.smc, key); ok && v == 1 {
					delete(pending, key)
				}
			}
		}
		if len(pending) > 0 {
			c.sleep(200 * time.Millisecond)
		}
	}
	if len(pending) > 0 {
		return fmt.Errorf("fans did not enter manual mode")
	}
	c.engaged = true
	return nil
}
