package barmanager

import (
	"sync"
	"time"
)

const (
	AutosavePrefix = "PowerfulTop.MenuBarSection."
	// The identifiers let the item scanner tell this app's own layout items apart from everyone's icons.
	SeparatorIdentifier = AutosavePrefix + "separator"
	ToggleIdentifier    = AutosavePrefix + "toggle"
	SpacerIdentifier    = AutosavePrefix + "spacer"

	enabledKey       = "menuBarManagerEnabled"
	autoHideDelayKey = "menuBarManagerAutoHideDelay"

	// Apps that register late in a login storm would otherwise be swallowed before they appear.
	launchCollapseDelay = 2500 * time.Millisecond
	menuBarPollInterval = time.Second
	defaultAutoHide     = 10 * time.Second
)

// AutoHideChoices are the delays the settings offer; 0 keeps revealed icons until the next click.
var AutoHideChoices = []time.Duration{0, 5 * time.Second, 10 * time.Second, 30 * time.Second, time.Minute}

// bar is the status items and the screen state the manager drives; AppKit on macOS, a fake in tests.
type bar interface {
	install()
	uninstall()
	// apply lays the items out; unit and active only matter while collapsed.
	apply(collapsed bool, unit float64, active int)
	setToggle(collapsed bool)
	displays() []Display
	// menuBarInUse: collapsing under an open menu or the pointer would yank away the item being used.
	menuBarInUse() bool
}

type prefStore interface {
	Bool(key string) (bool, bool)
	Float(key string) (float64, bool)
	Set(key string, value any) error
}

// Snapshot is the manager's state for views; safe to read from any goroutine.
type Snapshot struct {
	// Supported is false before macOS 27, which has no overflow to hand hidden icons to.
	Supported     bool
	Enabled       bool
	Collapsed     bool
	AutoHideDelay time.Duration
}

// Manager hides the icons ⌘-dragged left of the separator: when collapsed, blank spacers push them
// into macOS 27's own « overflow, without screen capture, click forwarding or any permission.
// The « reveal shows the blank spacers first, so the chevron is the way back.
//
// Every method except Snapshot and OnChange must run on the main thread.
type Manager struct {
	// OpenSettings is called from the chevron's menu; main sets it to show the manager's page.
	OpenSettings func()

	bar   bar
	prefs prefStore
	// onMain runs fn on the main thread, waiting for it; timers fire on other goroutines.
	onMain func(fn func())

	mu        sync.Mutex
	snap      Snapshot
	listeners []func()

	installed              bool
	editing                int
	collapsedBeforeEditing bool
	// Timers are invalidated by bumping generation rather than stopped, since one may already be
	// waiting for the main thread when it is cancelled.
	generation       int
	launchCollapsing bool
}

func newManager(b bar, prefs prefStore, onMain func(func()), supported bool) *Manager {
	m := &Manager{bar: b, prefs: prefs, onMain: onMain}
	m.snap.Supported = supported
	m.snap.Enabled, _ = prefs.Bool(enabledKey)
	m.snap.AutoHideDelay = defaultAutoHide
	if seconds, ok := prefs.Float(autoHideDelayKey); ok {
		m.snap.AutoHideDelay = time.Duration(seconds * float64(time.Second))
	}
	return m
}

func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snap
}

// OnChange registers fn, called on the main thread after the state changes.
func (m *Manager) OnChange(fn func()) {
	m.mu.Lock()
	m.listeners = append(m.listeners, fn)
	m.mu.Unlock()
}

func (m *Manager) update(change func(*Snapshot)) {
	m.mu.Lock()
	change(&m.snap)
	listeners := append([]func(){}, m.listeners...)
	m.mu.Unlock()
	for _, fn := range listeners {
		fn()
	}
}

// Start installs the items if enabled and collapses once launch has settled.
func (m *Manager) Start() {
	if m.Snapshot().Enabled {
		m.install(true)
	}
}

func (m *Manager) Shutdown() { m.uninstall() }

func (m *Manager) SetEnabled(on bool) {
	if m.Snapshot().Enabled == on {
		return
	}
	_ = m.prefs.Set(enabledKey, on)
	m.update(func(s *Snapshot) { s.Enabled = on })
	if on {
		m.install(false)
	} else {
		m.uninstall()
	}
}

func (m *Manager) SetAutoHideDelay(d time.Duration) {
	_ = m.prefs.Set(autoHideDelayKey, d.Seconds())
	m.update(func(s *Snapshot) { s.AutoHideDelay = d })
	m.scheduleAutoHide()
}

func (m *Manager) Toggle() {
	if m.Snapshot().Collapsed {
		m.Expand()
	} else {
		m.Collapse()
	}
}

func (m *Manager) Collapse() {
	if !m.installed {
		return
	}
	m.cancelPending()
	m.update(func(s *Snapshot) { s.Collapsed = true })
	m.applyGeometry()
}

func (m *Manager) Expand() {
	if !m.installed {
		return
	}
	m.cancelPending()
	m.update(func(s *Snapshot) { s.Collapsed = false })
	m.applyGeometry()
	m.scheduleAutoHide()
}

// BeginEditing holds the bar expanded for the layout editor, whose item frames are only
// meaningful then; EndEditing puts it back the way it was.
func (m *Manager) BeginEditing() {
	m.editing++
	if m.editing != 1 {
		return
	}
	m.collapsedBeforeEditing = m.Snapshot().Collapsed || m.launchCollapsing
	m.Expand()
}

func (m *Manager) EndEditing() {
	if m.editing == 0 {
		return
	}
	m.editing--
	if m.editing != 0 {
		return
	}
	if m.collapsedBeforeEditing {
		m.Collapse()
	} else {
		m.scheduleAutoHide()
	}
}

// ScreensChanged re-lays the collapsed block: one unit length serves every display, so a newly
// attached narrow one can push it past its cliff.
func (m *Manager) ScreensChanged() {
	if m.installed && m.Snapshot().Collapsed {
		m.applyGeometry()
	}
}

func (m *Manager) install(collapseAfterLaunch bool) {
	if !m.Snapshot().Supported || m.installed {
		return
	}
	m.bar.install()
	m.installed = true
	m.update(func(s *Snapshot) { s.Collapsed = false })
	m.applyGeometry()
	if collapseAfterLaunch {
		m.launchCollapsing = true
		gen := m.generation
		time.AfterFunc(launchCollapseDelay, func() {
			m.onMain(func() {
				if gen == m.generation {
					m.Collapse()
				}
			})
		})
	}
}

func (m *Manager) uninstall() {
	m.cancelPending()
	if m.installed {
		m.bar.uninstall()
		m.installed = false
	}
	m.update(func(s *Snapshot) { s.Collapsed = false })
}

func (m *Manager) applyGeometry() {
	collapsed := m.Snapshot().Collapsed
	unit, active := 0.0, 0
	if collapsed {
		displays := m.bar.displays()
		unit = UnitLength(displays)
		active = ActiveSpacers(unit, displays)
	}
	m.bar.apply(collapsed, unit, active)
	m.bar.setToggle(collapsed)
}

func (m *Manager) scheduleAutoHide() {
	m.generation++
	m.launchCollapsing = false
	s := m.Snapshot()
	if !m.installed || s.Collapsed || s.AutoHideDelay <= 0 || m.editing > 0 {
		return
	}
	gen := m.generation
	var wait func(time.Duration)
	wait = func(d time.Duration) {
		time.AfterFunc(d, func() {
			m.onMain(func() {
				if gen != m.generation {
					return
				}
				if m.bar.menuBarInUse() {
					wait(menuBarPollInterval)
					return
				}
				m.Collapse()
			})
		})
	}
	wait(s.AutoHideDelay)
}

func (m *Manager) cancelPending() {
	m.generation++
	m.launchCollapsing = false
}
