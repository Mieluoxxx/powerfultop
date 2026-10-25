package power

import (
	"slices"
	"sync"
)

// nativeLimitState is what PowerUIAgent reports: the limits it accepts and the configured one.
type nativeLimitState struct {
	supported bool
	limits    []int
	limit     int
	enabled   bool
}

type nativeLimit interface {
	read() (nativeLimitState, error)
	// set enables the limit at percent, or turns it off at 100.
	set(percent int) error
}

// ChargeLimitState is the native charge limit as PowerfulTop shows and edits it.
type ChargeLimitState struct {
	// Loaded is false until PowerUIAgent first answers.
	Loaded    bool
	Supported bool
	// Choices are the limits macOS accepts, ascending; 100 means no limit.
	Choices []int
	// Percent is the enforced limit; 100 when the limit is off.
	Percent int
	// Pending is true while a change is being applied.
	Pending bool
	Err     error
	// AlDenteRunning warns that AlDente may override the limit.
	AlDenteRunning bool
}

// ChargeLimiter edits the macOS charge limit, the same setting as System Settings › Battery, so
// firmware keeps enforcing it while PowerfulTop is closed or the Mac sleeps.
type ChargeLimiter struct {
	native nativeLimit

	mu        sync.Mutex
	state     ChargeLimitState
	listeners []func()
}

// NewChargeLimiter talks to PowerUIAgent; call Refresh to load the current limit.
func NewChargeLimiter() *ChargeLimiter {
	return &ChargeLimiter{native: &powerUI{}, state: ChargeLimitState{Percent: 100}}
}

// ChargeLimiterWithState returns a limiter that only shows s and keeps edits in memory, for previews and tests.
func ChargeLimiterWithState(s ChargeLimitState) *ChargeLimiter {
	return &ChargeLimiter{native: &memoryLimit{s: s}, state: s}
}

// State returns a copy of the current state.
func (l *ChargeLimiter) State() ChargeLimitState {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.state
	s.Choices = slices.Clone(s.Choices)
	return s
}

// OnChange registers fn, called on any goroutine after the state changes.
func (l *ChargeLimiter) OnChange(fn func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.listeners = append(l.listeners, fn)
}

func (l *ChargeLimiter) notify() {
	l.mu.Lock()
	listeners := slices.Clone(l.listeners)
	l.mu.Unlock()
	for _, fn := range listeners {
		fn()
	}
}

// Refresh rereads the limit in the background; System Settings or another tool may have changed it.
func (l *ChargeLimiter) Refresh() {
	go l.reload(nil)
}

// Set applies percent in the background; 100 turns the limit off.
func (l *ChargeLimiter) Set(percent int) {
	l.mu.Lock()
	if l.state.Pending || !l.state.Supported || !slices.Contains(l.state.Choices, percent) {
		l.mu.Unlock()
		return
	}
	l.state.Pending = true
	l.state.Percent = percent
	l.mu.Unlock()
	l.notify()
	go func() {
		err := l.native.set(percent)
		// powerd's cached limit drives the time-to-full estimate, so it must not outlive the change.
		invalidateMacOSChargeLimit()
		l.reload(err)
	}()
}

func (l *ChargeLimiter) reload(setErr error) {
	native, err := l.native.read()
	_, preview := l.native.(*memoryLimit)
	alDente := !preview && AlDenteRunning()
	l.mu.Lock()
	if !preview {
		l.state.AlDenteRunning = alDente
	}
	if err == nil {
		l.state.Loaded = true
		l.state.Supported = native.supported
		l.state.Choices = native.limits
		l.state.Percent = 100
		if native.enabled && native.limit > 0 {
			l.state.Percent = native.limit
		}
	}
	l.state.Pending = false
	l.state.Err = setErr
	if setErr == nil {
		l.state.Err = err
	}
	l.mu.Unlock()
	l.notify()
}

type memoryLimit struct {
	mu sync.Mutex
	s  ChargeLimitState
}

func (m *memoryLimit) read() (nativeLimitState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return nativeLimitState{supported: m.s.Supported, limits: m.s.Choices, limit: m.s.Percent, enabled: m.s.Percent < 100}, nil
}

func (m *memoryLimit) set(percent int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s.Percent = percent
	return nil
}
