//go:build darwin

package thermal

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"sync"
	"time"

	"powerfultop/internal/macos"
)

const (
	refreshInterval = 2 * time.Second
	modeKey         = "fanControlMode"
	// lastCustomModeKey is the custom setting restored when switching back from Auto or Full Blast, like Macs Fan Control.
	lastCustomModeKey = "fanControlLastCustomMode"
	restoreTimeout    = 3 * time.Second
)

// Monitor polls the SMC and, in a non-automatic mode, drives the fans through the helper.
type Monitor struct {
	helper *HelperClient
	prefs  macos.Preferences

	mu          sync.Mutex
	snap        Snapshot
	smc         *SMCConnection
	sensorKeys  map[SensorGroup][]string
	weEngaged   bool
	applying    bool
	lastRefresh time.Time
	listeners   []func()
	stop        chan struct{}
}

// NewMonitor restores the saved fan mode; bundledHelperPath is the helper inside the app's resources.
func NewMonitor(bundledHelperPath string) *Monitor {
	m := &Monitor{helper: NewHelperClient(bundledHelperPath), prefs: macos.AppPreferences()}
	m.snap.Mode = m.loadMode(modeKey, Automatic)
	m.snap.LastCustomMode = m.loadMode(lastCustomModeKey, DefaultConstant)
	return m
}

func (m *Monitor) loadMode(key string, fallback FanControlMode) FanControlMode {
	data, ok := m.prefs.Data(key)
	if !ok {
		return fallback
	}
	var mode FanControlMode
	if json.Unmarshal(data, &mode) != nil {
		return fallback
	}
	return mode
}

func (m *Monitor) saveMode(key string, mode FanControlMode) {
	if data, err := json.Marshal(mode); err == nil {
		_ = m.prefs.Set(key, data)
	}
}

// Start opens the SMC and polls it every 2 s; without an SMC the snapshot stays unavailable.
func (m *Monitor) Start() {
	m.mu.Lock()
	if m.stop != nil {
		m.mu.Unlock()
		return
	}
	smc, err := OpenSMC()
	if err != nil {
		m.mu.Unlock()
		return
	}
	m.smc = smc
	m.snap.Available = true
	m.stop = make(chan struct{})
	stop := m.stop
	m.mu.Unlock()

	go func() {
		status := m.helper.Status()
		keys := discoverSensorKeys(smc)
		m.mu.Lock()
		m.snap.HelperStatus = status
		m.sensorKeys = keys
		m.mu.Unlock()
		m.refresh()
		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				m.refresh()
			}
		}
	}()
}

// Stop ends polling without touching the fans.
func (m *Monitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stop != nil {
		close(m.stop)
		m.stop = nil
	}
}

// Shutdown runs on quit: fans must be back under macOS control before the process exits.
func (m *Monitor) Shutdown() {
	m.Stop()
	if m.helperHoldsFans() {
		_ = m.helper.RestoreAutomatic(restoreTimeout)
		m.mu.Lock()
		m.weEngaged = false
		m.mu.Unlock()
	}
}

// HandleSleep runs before the Mac sleeps: firmware drops the manual unlock across sleep, so hand
// control back now; the next refresh after wake re-engages.
func (m *Monitor) HandleSleep() { m.releaseFans() }

// OnChange registers fn, called on any goroutine after the snapshot changes.
func (m *Monitor) OnChange(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, fn)
}

func (m *Monitor) notify() {
	m.mu.Lock()
	listeners := slices.Clone(m.listeners)
	m.mu.Unlock()
	for _, fn := range listeners {
		fn()
	}
}

// Snapshot returns a copy of the latest reading and control state.
func (m *Monitor) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.snap
	s.Fans = slices.Clone(m.snap.Fans)
	s.Temperatures = maps.Clone(m.snap.Temperatures)
	s.Controlling = !s.Mode.IsAutomatic() && m.weEngaged
	return s
}

// SetMode switches fan control; custom modes are remembered for the Custom preset.
func (m *Monitor) SetMode(mode FanControlMode) {
	m.mu.Lock()
	m.snap.Mode = mode
	if mode.Kind == ModeConstant || mode.Kind == ModeSensor {
		m.snap.LastCustomMode = mode
	}
	m.snap.LastError = nil
	m.mu.Unlock()
	m.saveMode(modeKey, mode)
	if mode.Kind == ModeConstant || mode.Kind == ModeSensor {
		m.saveMode(lastCustomModeKey, mode)
	}
	if mode.IsAutomatic() {
		m.releaseFans()
	} else {
		go m.refresh()
	}
	m.notify()
}

// SelectPreset maps the Auto / Full Blast / Custom control to a mode.
func (m *Monitor) SelectPreset(preset int) {
	switch preset {
	case 0:
		m.SetMode(Automatic)
	case 1:
		m.SetMode(FullBlast)
	default:
		m.SetMode(m.Snapshot().LastCustomMode)
	}
}

// InstallHelper prompts for an administrator password in the background.
func (m *Monitor) InstallHelper() {
	m.mu.Lock()
	if m.snap.Installing {
		m.mu.Unlock()
		return
	}
	m.snap.Installing = true
	m.mu.Unlock()
	m.notify()
	go func() {
		err := m.helper.Install()
		status := m.helper.Status()
		m.mu.Lock()
		m.snap.Installing = false
		m.snap.HelperStatus = status
		if errors.Is(err, ErrCancelled) {
			err = nil
		}
		m.snap.LastError = err
		automatic := m.snap.Mode.IsAutomatic()
		m.mu.Unlock()
		// The helper restores a marker carried over from an older helper as it starts, but in
		// automatic mode nothing else would ask it to.
		if err == nil && automatic {
			m.releaseFans()
		}
		m.refresh()
	}()
}

// UninstallHelper hands the fans back and removes the helper, in the background.
func (m *Monitor) UninstallHelper() {
	m.SetMode(Automatic)
	go func() {
		_ = m.helper.RestoreAutomatic(restoreTimeout)
		err := m.helper.Uninstall()
		status := m.helper.Status()
		m.mu.Lock()
		m.weEngaged = false
		m.snap.HelperStatus = status
		if errors.Is(err, ErrCancelled) {
			err = nil
		}
		m.snap.LastError = err
		m.mu.Unlock()
		m.notify()
	}()
}

// helperHoldsFans: a failed request clears weEngaged, but the helper may still hold the fans; the marker is authoritative.
func (m *Monitor) helperHoldsFans() bool {
	m.mu.Lock()
	engaged := m.weEngaged
	m.mu.Unlock()
	return engaged || exists(EngagedMarkerPath)
}

func (m *Monitor) releaseFans() {
	if !m.helperHoldsFans() {
		return
	}
	m.mu.Lock()
	m.weEngaged = false
	m.mu.Unlock()
	go func() {
		err := m.helper.RestoreAutomatic(restoreTimeout)
		m.mu.Lock()
		m.snap.LastError = err
		m.mu.Unlock()
		m.notify()
	}()
}

func (m *Monitor) refresh() {
	m.mu.Lock()
	smc, keys := m.smc, m.sensorKeys
	previous, last := m.snap.Temperatures, m.lastRefresh
	m.mu.Unlock()
	if smc == nil {
		return
	}
	next := readSnapshot(smc, keys)
	now := time.Now()
	elapsed := 0.0
	if !last.IsZero() {
		elapsed = now.Sub(last).Seconds()
	}
	next.Temperatures = SmoothTemperatures(previous, next.Temperatures, elapsed)
	conflict := IsForeignFanControl(next, exists(EngagedMarkerPath), isOtherFanToolRunning())

	m.mu.Lock()
	m.lastRefresh = now
	m.snap.Fans, m.snap.Temperatures = next.Fans, next.Temperatures
	m.snap.DiagnosticUnlocked, m.snap.DisplayPowerW = next.DiagnosticUnlocked, next.DisplayPowerW
	m.snap.ConflictDetected = conflict
	m.mu.Unlock()
	m.applyControl()
	m.notify()
}

func (m *Monitor) applyControl() {
	m.mu.Lock()
	s := m.snap
	if s.Mode.IsAutomatic() || len(s.Fans) == 0 || s.ConflictDetected || s.HelperStatus != HelperReady || m.applying {
		m.mu.Unlock()
		return
	}
	targets := make([]float64, len(s.Fans))
	for i, fan := range s.Fans {
		target, ok := FanTargetRPM(s.Mode, fan, s.Temperatures)
		if !ok {
			target = fan.MinRPM
		}
		targets[i] = target
	}
	m.weEngaged = true
	// Engaging can take 20 s; requests don't pile up behind it, the next refresh is the next heartbeat.
	m.applying = true
	m.mu.Unlock()
	go func() {
		err := m.helper.SetFanTargets(targets)
		m.mu.Lock()
		m.applying = false
		m.snap.LastError = err
		if err != nil {
			m.weEngaged = false
		}
		m.mu.Unlock()
		m.notify()
	}()
}

// discoverSensorKeys walks every SMC key once; it takes a few seconds, so it runs off the poll.
func discoverSensorKeys(smc SMC) map[SensorGroup][]string {
	var keys []string
	for i := range smc.KeyCount() {
		key, ok := smc.KeyAt(i)
		if !ok || len(key) == 0 || key[0] != 'T' {
			continue
		}
		if v, ok := smc.Read(key); ok && v.Type == "flt " {
			keys = append(keys, key)
		}
	}
	return GroupSensorKeys(keys)
}

func readSnapshot(smc SMC, sensorKeys map[SensorGroup][]string) Snapshot {
	var s Snapshot
	count, _ := readFloat(smc, "FNum")
	for i := range int(count) {
		maxRPM, ok := readFloat(smc, fmt.Sprintf("F%dMx", i))
		if !ok || maxRPM <= 0 {
			continue
		}
		mode := -1
		if v, ok := readFloat(smc, fmt.Sprintf("F%dMd", i)); ok {
			mode = int(v)
		} else if v, ok := readFloat(smc, fmt.Sprintf("F%dmd", i)); ok {
			mode = int(v)
		}
		actual, _ := readFloat(smc, fmt.Sprintf("F%dAc", i))
		minRPM, _ := readFloat(smc, fmt.Sprintf("F%dMn", i))
		target, _ := readFloat(smc, fmt.Sprintf("F%dTg", i))
		s.Fans = append(s.Fans, FanReading{Index: i, ActualRPM: actual, MinRPM: minRPM, MaxRPM: maxRPM, TargetRPM: target, Mode: mode})
	}
	s.Temperatures = map[SensorGroup]TemperatureSummary{}
	for group, keys := range sensorKeys {
		var readings []float64
		for _, key := range keys {
			if v, ok := readFloat(smc, key); ok && IsPlausibleTemperature(v) {
				readings = append(readings, v)
			}
		}
		if summary, ok := Summarize(readings); ok {
			s.Temperatures[group] = summary
		}
	}
	if v, ok := readFloat(smc, "Ftst"); ok && v == 1 {
		s.DiagnosticUnlocked = true
	}
	if v, ok := readFloat(smc, "PDBR"); ok && !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 {
		s.DisplayPowerW = &v
	}
	return s
}

// MonitorWithSnapshot returns a monitor that never polls and shows s, for previews and view tests.
func MonitorWithSnapshot(s Snapshot) *Monitor {
	return &Monitor{helper: NewHelperClient(""), snap: s}
}
