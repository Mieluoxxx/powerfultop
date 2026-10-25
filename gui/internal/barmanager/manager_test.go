package barmanager

import (
	"sync"
	"testing"
	"time"
)

type fakeBar struct {
	mu        sync.Mutex
	installed bool
	collapsed bool
	unit      float64
	active    int
	inUse     bool
}

func (b *fakeBar) install()   { b.mu.Lock(); b.installed = true; b.mu.Unlock() }
func (b *fakeBar) uninstall() { b.mu.Lock(); b.installed = false; b.mu.Unlock() }
func (b *fakeBar) apply(collapsed bool, unit float64, active int) {
	b.mu.Lock()
	b.collapsed, b.unit, b.active = collapsed, unit, active
	b.mu.Unlock()
}
func (b *fakeBar) setToggle(bool) {}
func (b *fakeBar) displays() []Display {
	return []Display{NewDisplay(1512, 663.5), NewDisplay(1920, 0)}
}
func (b *fakeBar) menuBarInUse() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.inUse }

type fakePrefs map[string]any

func (p fakePrefs) Bool(k string) (bool, bool)     { v, ok := p[k].(bool); return v, ok }
func (p fakePrefs) Float(k string) (float64, bool) { v, ok := p[k].(float64); return v, ok }
func (p fakePrefs) Set(k string, v any) error      { p[k] = v; return nil }

// The main thread is simulated by one lock, as timers call back from other goroutines.
func newTestManager(prefs fakePrefs) (*Manager, *fakeBar, *sync.Mutex) {
	b := &fakeBar{}
	var main sync.Mutex
	m := newManager(b, prefs, func(fn func()) { main.Lock(); defer main.Unlock(); fn() }, true)
	return m, b, &main
}

// onMain runs test steps as the main thread would.
func onMain(m *Manager, fn func()) { m.onMain(fn) }

func TestEnableCollapseExpand(t *testing.T) {
	prefs := fakePrefs{}
	m, b, _ := newTestManager(prefs)
	if m.Snapshot().Enabled || m.Snapshot().AutoHideDelay != defaultAutoHide {
		t.Fatalf("defaults: %+v", m.Snapshot())
	}
	onMain(m, func() { m.SetAutoHideDelay(0) })
	onMain(m, func() { m.SetEnabled(true) })
	if !b.installed || prefs[enabledKey] != true {
		t.Fatal("enabling installs the items and persists")
	}
	onMain(m, func() { m.Collapse() })
	if !b.collapsed || b.unit != 433 || b.active != 4 {
		t.Errorf("collapsed geometry: %+v", b)
	}
	onMain(m, func() { m.Toggle() })
	if b.collapsed || m.Snapshot().Collapsed {
		t.Error("toggle expands")
	}
	onMain(m, func() { m.SetEnabled(false) })
	if b.installed {
		t.Error("disabling removes the items")
	}
}

func TestUnsupportedNeverInstalls(t *testing.T) {
	b := &fakeBar{}
	m := newManager(b, fakePrefs{enabledKey: true}, func(fn func()) { fn() }, false)
	onMain(m, func() { m.Start() })
	if b.installed {
		t.Error("macOS 26 and earlier have no overflow to use")
	}
}

func TestEditingRestoresCollapse(t *testing.T) {
	m, b, _ := newTestManager(fakePrefs{enabledKey: true, autoHideDelayKey: 0.0})
	onMain(m, func() { m.Start() })
	onMain(m, func() { m.Collapse() })
	onMain(m, func() { m.BeginEditing() })
	onMain(m, func() { m.BeginEditing() })
	if b.collapsed {
		t.Fatal("editing expands the bar")
	}
	onMain(m, func() { m.EndEditing() })
	if b.collapsed {
		t.Fatal("still editing")
	}
	onMain(m, func() { m.EndEditing() })
	if !b.collapsed {
		t.Error("ending the edit collapses again")
	}
}

func TestAutoHideWaitsForMenuBar(t *testing.T) {
	m, b, main := newTestManager(fakePrefs{enabledKey: true})
	onMain(m, func() { m.Start() })
	b.inUse = true
	main.Lock()
	m.SetAutoHideDelay(20 * time.Millisecond)
	main.Unlock()
	time.Sleep(80 * time.Millisecond)
	if m.Snapshot().Collapsed {
		t.Fatal("must not collapse while the menu bar is in use")
	}
	b.mu.Lock()
	b.inUse = false
	b.mu.Unlock()
	deadline := time.Now().Add(3 * time.Second)
	for !m.Snapshot().Collapsed && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !m.Snapshot().Collapsed {
		t.Error("collapses once the menu bar is free")
	}
}
