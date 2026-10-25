package barmanager

import (
	"sync"
	"time"
)

// StoreError is why the last move failed; the page words it.
type StoreError int

const (
	NoError StoreError = iota
	// ErrNotInMenuBar: the item isn't drawn in any menu bar.
	ErrNotInMenuBar
	// ErrDidNotMove: the move ran but the item didn't land, e.g. behind the notch or «.
	ErrDidNotMove
	// ErrNoLayoutAccess: the user picked something other than the layout file.
	ErrNoLayoutAccess
)

// StoreSnapshot is what the layout editor shows; safe to read from any goroutine.
type StoreSnapshot struct {
	Items   []Item
	Trusted bool
	// Crowded: the bar being read can't fit the separator, so no item can be placed.
	Crowded         bool
	HasLayoutAccess bool
	Moving          bool
	Error           StoreError
}

type itemPlatform interface {
	trusted() bool
	requestTrust() bool
	runningApps() []appRef
	readItems(pids []int32) []rawItem
	overflowButton(apps []appRef) *Rect
	// positions is nil without access to the layout file.
	positions() map[string]float64
	setPosition(key string, value float64) bool
	requestLayoutAccess() bool
	commandDrag(start, end Point)
	ownPID() int32
	appIcon(path string) []byte
}

const (
	pollInterval = 3 * time.Second
	// The bar needs a moment to lay out after expanding or after a drop.
	settleDelay = 400 * time.Millisecond
)

// ItemStore lists every app's menu bar items for the layout editor. Items come from
// Accessibility; their order comes from the macOS 27 layout file when the user has granted it,
// which also moves them without the pointer. Otherwise they are classified by on-screen frames
// and moved with the ⌘-drag a user would do, which only reaches items the active bar shows.
type ItemStore struct {
	manager *Manager
	p       itemPlatform

	mu        sync.Mutex
	snap      StoreSnapshot
	frames    *frameAnchors
	file      *fileAnchors
	visible   bool
	holdsBar  bool
	stop      chan struct{}
	listeners []func()
	icons     map[string][]byte

	// work serializes scans and moves, which both read and drive the bar.
	work sync.Mutex
}

func newItemStore(m *Manager, p itemPlatform) *ItemStore {
	return &ItemStore{manager: m, p: p, icons: map[string][]byte{}}
}

func (s *ItemStore) Snapshot() StoreSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := s.snap
	snap.Items = append([]Item(nil), s.snap.Items...)
	return snap
}

// OnChange registers fn, called from the store's goroutine after the snapshot changes.
func (s *ItemStore) OnChange(fn func()) {
	s.mu.Lock()
	s.listeners = append(s.listeners, fn)
	s.mu.Unlock()
}

func (s *ItemStore) update(change func(*StoreSnapshot)) {
	s.mu.Lock()
	change(&s.snap)
	listeners := append([]func(){}, s.listeners...)
	s.mu.Unlock()
	for _, fn := range listeners {
		fn()
	}
}

// Icon is the PNG of the app owning item, once a scan has fetched it.
func (s *ItemStore) Icon(item Item) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.icons[item.AppPath]
}

// SetVisible reports the editor's on-screen state; polling runs only while someone is looking.
// Safe from any goroutine.
func (s *ItemStore) SetVisible(visible bool) {
	s.mu.Lock()
	if visible == s.visible {
		s.mu.Unlock()
		return
	}
	s.visible = visible
	if s.stop != nil {
		close(s.stop)
		s.stop = nil
	}
	var stop chan struct{}
	if visible {
		stop = make(chan struct{})
		s.stop = stop
	}
	s.mu.Unlock()

	if !visible {
		go s.releaseBar()
		return
	}
	go func() {
		for {
			s.refresh()
			select {
			case <-stop:
				return
			case <-time.After(pollInterval):
			}
		}
	}()
}

// Refresh rescans now, e.g. after the manager was switched on.
func (s *ItemStore) Refresh() { go s.refresh() }

func (s *ItemStore) refresh() {
	s.work.Lock()
	defer s.work.Unlock()
	trusted := s.p.trusted()
	s.update(func(snap *StoreSnapshot) { snap.Trusted = trusted })

	s.mu.Lock()
	wantsBar := s.visible && trusted && s.manager.Snapshot().Enabled
	holds := s.holdsBar
	s.mu.Unlock()
	if wantsBar && !holds {
		s.manager.onMain(s.manager.BeginEditing)
		s.mu.Lock()
		s.holdsBar = true
		s.mu.Unlock()
		time.Sleep(settleDelay)
	} else if !wantsBar && holds {
		s.releaseBarLocked()
	}
	if !wantsBar {
		s.update(func(snap *StoreSnapshot) { snap.Items = nil })
		return
	}
	s.scan()
}

func (s *ItemStore) releaseBar() {
	s.work.Lock()
	defer s.work.Unlock()
	s.releaseBarLocked()
}

func (s *ItemStore) releaseBarLocked() {
	s.mu.Lock()
	holds := s.holdsBar
	s.holdsBar = false
	s.mu.Unlock()
	if holds {
		s.manager.onMain(s.manager.EndEditing)
	}
	s.update(func(snap *StoreSnapshot) { snap.Items = nil })
}

func (s *ItemStore) scan() {
	all := s.p.runningApps()
	overflow := s.p.overflowButton(all)
	apps := withoutAgent(all)
	pids := make([]int32, len(apps))
	for i, a := range apps {
		pids[i] = a.PID
	}
	raw := s.p.readItems(pids)
	positions := s.p.positions()
	result := classify(apps, raw, overflow, positions, s.p.ownPID())

	for _, item := range result.items {
		s.mu.Lock()
		_, have := s.icons[item.AppPath]
		s.mu.Unlock()
		if !have && item.AppPath != "" {
			icon := s.p.appIcon(item.AppPath)
			s.mu.Lock()
			s.icons[item.AppPath] = icon
			s.mu.Unlock()
		}
	}

	s.mu.Lock()
	s.frames, s.file = result.frames, result.file
	s.mu.Unlock()
	s.update(func(snap *StoreSnapshot) {
		snap.Items = result.items
		snap.Crowded = result.crowded
		snap.HasLayoutAccess = positions != nil
	})
}

// Move puts an item into section, immediately left of the item with ID before when given, else
// at the section's edge next to the chevron. It runs in the background.
func (s *ItemStore) Move(id string, section Section, before string) {
	go s.move(id, section, before)
}

func (s *ItemStore) move(id string, section Section, before string) {
	s.work.Lock()
	defer s.work.Unlock()
	s.mu.Lock()
	holds := s.holdsBar
	s.mu.Unlock()
	if !holds || id == before || s.Snapshot().Moving {
		return
	}
	s.update(func(snap *StoreSnapshot) { snap.Moving = true })
	defer s.update(func(snap *StoreSnapshot) { snap.Moving = false })

	// Positions shift whenever any app's item changes width, so measure right before moving.
	s.scan()
	snap := s.Snapshot()
	item, ok := find(snap.Items, id)
	if !ok {
		return
	}
	anchor, hasAnchor := find(snap.Items, before)
	hasAnchor = hasAnchor && anchor.Section == section
	if !hasAnchor && item.Section == section {
		return
	}
	if !item.CanMove() {
		s.setError(ErrNotInMenuBar)
		return
	}

	s.mu.Lock()
	file, frames := s.file, s.frames
	s.mu.Unlock()

	if positions := s.p.positions(); item.LayoutKey != "" && file != nil && positions != nil {
		var existing []float64
		for key, v := range positions {
			if key != item.LayoutKey {
				existing = append(existing, v)
			}
		}
		var target float64
		switch {
		case hasAnchor && anchor.HasPosition:
			target = PositionLeftOf(anchor.Position, existing)
		case section == Hidden:
			target = PositionLeftOf(file.separator, existing)
		default:
			target = PositionRightOf(file.toggle, existing)
		}
		if s.p.setPosition(item.LayoutKey, target) {
			time.Sleep(settleDelay)
			s.scan()
			moved, _ := find(s.Snapshot().Items, id)
			s.setErrorUnless(moved.Section == section)
			return
		}
	}

	if !item.Reachable || frames == nil {
		s.setError(ErrDidNotMove)
		return
	}
	target, _ := DropPoint(section, frames.separator, frames.toggle)
	if hasAnchor && anchor.Reachable {
		target = DropPointLeftOf(anchor.Frame)
	}
	s.p.commandDrag(Point{item.Frame.MidX(), item.Frame.MidY()}, target)
	time.Sleep(settleDelay)
	s.scan()
	after := s.Snapshot().Items
	moved, _ := find(after, id)
	landed := moved.Section == section
	if hasAnchor {
		if a, ok := find(after, anchor.ID); ok {
			landed = landed && moved.Frame.MidX() < a.Frame.MidX()
		}
	}
	s.setErrorUnless(landed)
}

func (s *ItemStore) setError(e StoreError) {
	s.update(func(snap *StoreSnapshot) { snap.Error = e })
}

func (s *ItemStore) setErrorUnless(ok bool) {
	if ok {
		s.setError(NoError)
	} else {
		s.setError(ErrDidNotMove)
	}
}

// RequestAccessibility shows macOS's prompt for Accessibility access.
func (s *ItemStore) RequestAccessibility() {
	trusted := s.p.requestTrust()
	s.update(func(snap *StoreSnapshot) { snap.Trusted = trusted })
}

// RequestLayoutAccess asks the user to pick the layout file; it runs a modal panel, so call it on
// the main thread.
func (s *ItemStore) RequestLayoutAccess() {
	if s.p.requestLayoutAccess() {
		s.setError(NoError)
	} else {
		s.setError(ErrNoLayoutAccess)
	}
	s.Refresh()
}

func find(items []Item, id string) (Item, bool) {
	if id == "" {
		return Item{}, false
	}
	for _, it := range items {
		if it.ID == id {
			return it, true
		}
	}
	return Item{}, false
}
