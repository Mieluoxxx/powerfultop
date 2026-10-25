package power

import (
	"sync"
	"time"
)

const (
	pollInterval = 2 * time.Second
	// A poll gap this long means the Mac slept; smoothing across it would mix two sessions.
	sleepGap            = 3 * pollInterval
	connectingACTimeout = 3 * time.Second
	// A fresh unplug can still read ExternalConnected=true for a moment.
	unplugSettle = 1500 * time.Millisecond
)

// Plug and unplug events precede the telemetry that reflects them, so a burst of refreshes follows each.
var coalescedRefreshDelays = []time.Duration{
	100 * time.Millisecond, 300 * time.Millisecond, 600 * time.Millisecond, time.Second,
	1500 * time.Millisecond, 2 * time.Second, 3 * time.Second, 5 * time.Second,
}

type machinePhase struct {
	phase ConnectionPhase
	since time.Time // when connectingAC began
}

// Monitor polls the battery every 2 s and on power-source events, keeping the display snapshot.
type Monitor struct {
	mu        sync.Mutex
	data      Data
	available bool
	started   bool
	stop      chan struct{}
	listeners []func()

	estimator    Estimator
	machine      machinePhase
	lastExternal *bool
	lastUnplugAt time.Time
	refreshGen   int

	read func() (Data, bool)
	now  func() time.Time
}

func NewMonitor() *Monitor {
	return &Monitor{data: Empty, available: true, read: ReadData, now: time.Now}
}

// Snapshot is the latest display data, phase and smoothing applied.
func (m *Monitor) Snapshot() Data {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data
}

// Available is false when the Mac has no built-in battery to read.
func (m *Monitor) Available() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.available
}

// OnChange registers fn, called on any goroutine after each refresh.
func (m *Monitor) OnChange(fn func()) {
	m.mu.Lock()
	m.listeners = append(m.listeners, fn)
	m.mu.Unlock()
}

func (m *Monitor) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.stop = make(chan struct{})
	stop := m.stop
	m.mu.Unlock()

	raw, ok := m.read()
	m.mu.Lock()
	m.available = ok
	external := raw.IsOnAC
	m.lastExternal = &external
	m.machine = machinePhase{phase: PhaseOnBattery}
	if raw.IsOnAC {
		m.machine.phase = PhaseOnAC
	}
	m.publishLocked(raw)
	m.mu.Unlock()
	m.notify()

	go m.loop(stop)
	watchPowerSources(m)
}

func (m *Monitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started {
		return
	}
	m.started = false
	close(m.stop)
	m.refreshGen++
	m.estimator.Reset()
	unwatchPowerSources(m)
}

// RefreshNow reads again without waiting for the next poll.
func (m *Monitor) RefreshNow() { go m.update() }

func (m *Monitor) loop(stop chan struct{}) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	last := m.now()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			now := m.now()
			if now.Sub(last) > sleepGap {
				m.mu.Lock()
				m.estimator.Reset()
				m.mu.Unlock()
			}
			last = now
			m.update()
		}
	}
}

func (m *Monitor) update() {
	raw, ok := m.read()
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return
	}
	m.available = ok
	m.publishLocked(raw)
	m.mu.Unlock()
	m.notify()
}

// powerSourceChanged runs on a power-source notification: plug, unplug or adapter change.
func (m *Monitor) powerSourceChanged() {
	raw, ok := m.read()
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return
	}
	if ok {
		connected := raw.IsOnAC
		if m.lastExternal != nil && *m.lastExternal != connected {
			if connected {
				m.machine = machinePhase{phase: PhaseConnectingAC, since: m.now()}
			} else {
				m.machine = machinePhase{phase: PhaseOnBattery}
				m.lastUnplugAt = m.now()
			}
		}
		m.lastExternal = &connected
	}
	m.available = ok
	m.publishLocked(raw)
	m.refreshGen++
	gen := m.refreshGen
	m.mu.Unlock()
	m.notify()

	for _, delay := range coalescedRefreshDelays {
		time.AfterFunc(delay, func() {
			m.mu.Lock()
			current := m.refreshGen == gen
			m.mu.Unlock()
			if current {
				m.update()
			}
		})
	}
}

func (m *Monitor) publishLocked(raw Data) {
	m.advancePhaseLocked(raw)
	phased := raw.WithConnectionPhase(m.machine.phase)
	discharge, charge := m.estimator.SmoothedPower(phased)
	m.data = phased.WithSmoothedPower(discharge, charge)
}

func (m *Monitor) advancePhaseLocked(raw Data) {
	now := m.now()
	switch m.machine.phase {
	case PhaseOnBattery:
		if !m.lastUnplugAt.IsZero() && now.Sub(m.lastUnplugAt) < unplugSettle {
			return
		}
		if raw.IsOnAC {
			m.machine = machinePhase{phase: PhaseConnectingAC, since: now}
			m.lastUnplugAt = time.Time{}
		}
	case PhaseConnectingAC:
		if !raw.IsOnAC {
			m.machine = machinePhase{phase: PhaseOnBattery}
		} else if resolvedWhileConnecting(raw) || now.Sub(m.machine.since) >= connectingACTimeout {
			m.machine = machinePhase{phase: PhaseOnAC}
			m.lastUnplugAt = time.Time{}
		}
	case PhaseOnAC:
		if !raw.IsOnAC {
			m.machine = machinePhase{phase: PhaseOnBattery}
		}
	}
}

// resolvedWhileConnecting: stale pre-unplug SystemPowerIn must not alone prove convergence.
func resolvedWhileConnecting(raw Data) bool {
	if !raw.IsOnAC {
		return false
	}
	if raw.IsCharging && raw.BatteryPowerW < -0.3 {
		return true
	}
	// Raw IOKit amperage: positive = charging.
	if raw.BatteryAmperageMA != nil && *raw.BatteryAmperageMA > 50 {
		return true
	}
	if raw.ChargingCurrentMA != nil && *raw.ChargingCurrentMA > 0 && raw.BatteryPowerW < -0.3 {
		return true
	}
	if raw.ACInputW <= 0.3 || raw.SystemPowerW <= 0 {
		return false
	}
	return raw.BatteryPowerW < -0.3 || raw.ACInputW >= raw.SystemPowerW-0.5
}

func (m *Monitor) notify() {
	m.mu.Lock()
	listeners := append([]func(){}, m.listeners...)
	m.mu.Unlock()
	for _, fn := range listeners {
		fn()
	}
}
