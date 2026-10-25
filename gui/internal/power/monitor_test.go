package power

import (
	"sync"
	"testing"
	"time"
)

type fakeBattery struct {
	mu  sync.Mutex
	d   Data
	now time.Time
}

func (f *fakeBattery) read() (Data, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.d, true
}

func (f *fakeBattery) clock() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeBattery) set(fn func(*fakeBattery)) {
	f.mu.Lock()
	fn(f)
	f.mu.Unlock()
}

func newTestMonitor(f *fakeBattery) *Monitor {
	m := NewMonitor()
	m.read, m.now = f.read, f.clock
	return m
}

func TestMonitorConnectingPhase(t *testing.T) {
	f := &fakeBattery{d: data(10, 10, 0, false, PhaseOnBattery), now: time.Unix(1000, 0)}
	m := newTestMonitor(f)
	changes := 0
	m.OnChange(func() { changes++ })
	m.Start()
	defer m.Stop()
	if m.Snapshot().PowerState().Kind != StateOnBattery || changes != 1 {
		t.Fatalf("starts on battery and notifies once: %+v %d", m.Snapshot().PowerState(), changes)
	}

	// Plugged in, but telemetry still shows the pre-plug discharge: connecting.
	f.set(func(f *fakeBattery) { f.d = data(10, 10, 0, true, PhaseOnBattery); f.now = f.now.Add(time.Second) })
	m.update()
	if got := m.Snapshot().PowerState().Kind; got != StateConnectingAC {
		t.Fatalf("stale telemetry while connecting: %v", got)
	}

	// Charging telemetry arrives: settled on AC.
	f.set(func(f *fakeBattery) {
		f.d = makeData(10, -20, 30, true, true, false, PhaseOnBattery, f.now)
		f.now = f.now.Add(time.Second)
	})
	m.update()
	if got := m.Snapshot().PowerState(); got.Kind != StateCharging || got.RateW != 20 {
		t.Fatalf("resolved charging: %+v", got)
	}

	// Unplugged.
	f.set(func(f *fakeBattery) { f.d = data(12, 12, 0, false, PhaseOnBattery); f.now = f.now.Add(time.Second) })
	m.update()
	if got := m.Snapshot().PowerState().Kind; got != StateOnBattery {
		t.Fatalf("unplugged: %v", got)
	}
}

func TestMonitorConnectingTimesOut(t *testing.T) {
	f := &fakeBattery{d: data(10, 10, 0, false, PhaseOnBattery), now: time.Unix(1000, 0)}
	m := newTestMonitor(f)
	m.Start()
	defer m.Stop()
	f.set(func(f *fakeBattery) { f.d = data(10, 0, 0, true, PhaseOnBattery) })
	m.update()
	if m.Snapshot().PowerState().Kind != StateConnectingAC {
		t.Fatal("connecting")
	}
	f.set(func(f *fakeBattery) { f.now = f.now.Add(connectingACTimeout) })
	m.update()
	if m.Snapshot().PowerState().Kind != StateACIdle {
		t.Errorf("settles after the timeout: %v", m.Snapshot().PowerState())
	}
}

func TestMonitorUnplugSettles(t *testing.T) {
	f := &fakeBattery{d: data(10, 0, 10, true, PhaseOnBattery), now: time.Unix(1000, 0)}
	m := newTestMonitor(f)
	m.Start()
	defer m.Stop()
	// The unplug event arrives while the registry still says ExternalConnected.
	f.set(func(f *fakeBattery) { f.d = data(10, 10, 0, false, PhaseOnBattery) })
	m.powerSourceChanged()
	f.set(func(f *fakeBattery) {
		f.d = data(10, 10, 0, true, PhaseOnBattery)
		f.now = f.now.Add(500 * time.Millisecond)
	})
	m.update()
	if m.Snapshot().PowerState().Kind != StateOnBattery {
		t.Errorf("a lagging ExternalConnected right after unplug must not flip to connecting: %v", m.Snapshot().PowerState())
	}
}
