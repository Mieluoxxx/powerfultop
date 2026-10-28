package barmanager

import (
	"sort"
	"strconv"
	"sync"
	"time"

	"powerfultop/internal/notify"
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
	// Supported is false before macOS 27, whose single menu bar and layout file the editor relies on.
	Supported bool
	// Items run left to right as in the bar.
	Items           []Item
	Trusted         bool
	HasLayoutAccess bool
	Moving          bool
	Error           StoreError
	// Displays have a menu bar each; Items are as the one named Display shows them.
	Displays []Display
	Display  string
}

type itemPlatform interface {
	trusted() bool
	requestTrust() bool
	runningApps() []appRef
	barAnchors(apps []appRef) barAnchors
	// agentBars are MenuBarAgent's windows, one per display's menu bar.
	agentBars() []agentBar
	displays() []Display
	// onBarChange calls fn, from any goroutine, a moment after MenuBarAgent lays a bar out again.
	onBarChange(fn func()) (stop func())
	// positions is nil without access to the layout file.
	positions() map[string]float64
	setPosition(key string, value float64) bool
	requestLayoutAccess() bool
	commandDrag(start, end Point)
	// reloadMenuBar restarts MenuBarAgent so it reads the layout file again.
	reloadMenuBar()
	ownPID() int32
	appIcon(path string) []byte
	// allowList is nil without access to the list's file.
	allowList() []any
}

const (
	// MenuBarAgent tells of every layout; the poll only catches what it doesn't, such as a
	// layout file write it ignores.
	pollInterval = 15 * time.Second
	// A layout comes as a burst of notifications, one per display and Space.
	changeDebounce = 250 * time.Millisecond
	// The bar needs a moment to lay out after a drop.
	settleDelay = 400 * time.Millisecond
)

// ItemStore lists every app's menu bar items for the layout editor, as the menu bar of a chosen
// display shows them (see model.go). Their order comes from the macOS 27 layout file when the user
// has granted it, which also moves them without the pointer; otherwise they are moved with the
// ⌘-drag a user would do, which only reaches items the bar shows.
type ItemStore struct {
	p        itemPlatform
	prefs    prefStore
	ownNames []string
	// chosen is the display the user picked; it may be disconnected, and comes back with it.
	chosen string

	mu      sync.Mutex
	snap    StoreSnapshot
	visible bool
	stop    chan struct{}
	changes notify.Listeners
	icons   map[string][]byte

	// work serializes scans and moves, which both read and drive the bar.
	work sync.Mutex
}

const editorDisplayKey = "menuBarEditorDisplay"

func newItemStore(p itemPlatform, prefs prefStore, supported bool, ownNames []string) *ItemStore {
	s := &ItemStore{p: p, prefs: prefs, ownNames: ownNames, snap: StoreSnapshot{Supported: supported}, icons: map[string][]byte{}}
	if v, ok := prefs.Value(editorDisplayKey); ok {
		s.chosen, _ = v.(string)
	}
	return s
}

// SelectDisplay shows the menu bar of the display named name, and keeps the choice.
func (s *ItemStore) SelectDisplay(name string) {
	_ = s.prefs.Set(editorDisplayKey, name)
	s.mu.Lock()
	s.chosen = name
	s.mu.Unlock()
	s.update(func(snap *StoreSnapshot) { snap.Display = name })
	s.Refresh()
}

// displaysOf names each bar after its display; frames come from MenuBarAgent and the display
// list alike, in global top-left points.
func (s *ItemStore) displaysOf(bars []agentBar) []Display {
	known := s.p.displays()
	var out []Display
	for i, b := range bars {
		d := Display{Name: "Display " + strconv.Itoa(i+1), Frame: b.Frame}
		for _, k := range known {
			if _, ok := barFor([]agentBar{b}, k.Frame); ok {
				d.Name, d.Internal = k.Name, k.Internal
			}
		}
		out = append(out, d)
	}
	// The built-in display first, as System Settings lists them.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Internal && !out[j].Internal })
	return out
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
	s.changes.Add(fn)
}

func (s *ItemStore) update(change func(*StoreSnapshot)) {
	s.mu.Lock()
	change(&s.snap)
	s.mu.Unlock()
	s.changes.Notify()
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
	if visible == s.visible || !s.snap.Supported {
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
		s.update(func(snap *StoreSnapshot) { snap.Items = nil })
		return
	}
	changed := make(chan struct{}, 1)
	off := s.p.onBarChange(func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	go func() {
		defer off()
		for {
			s.refresh()
			select {
			case <-stop:
				return
			case <-changed:
				time.Sleep(changeDebounce)
				select {
				case <-changed:
				default:
				}
			case <-time.After(pollInterval):
			}
		}
	}()
}

// Refresh rescans now.
func (s *ItemStore) Refresh() { go s.refresh() }

func (s *ItemStore) refresh() {
	s.work.Lock()
	defer s.work.Unlock()
	trusted := s.p.trusted()
	s.update(func(snap *StoreSnapshot) { snap.Trusted = trusted })
	s.mu.Lock()
	visible := s.visible
	s.mu.Unlock()
	if !visible || !trusted {
		s.update(func(snap *StoreSnapshot) { snap.Items = nil })
		return
	}
	s.scan()
}

func (s *ItemStore) scan() {
	apps := s.p.runningApps()
	anchors := s.p.barAnchors(apps)
	bars := s.p.agentBars()
	displays := s.displaysOf(bars)
	positions := s.p.positions()
	off := map[string]bool{}
	if list := s.p.allowList(); list != nil {
		for _, a := range apps {
			if allowed, listed := allowedState(list, allowOwner{a.BundleID, a.Executable}); listed && !allowed {
				off[a.BundleID] = true
			}
		}
	}
	s.mu.Lock()
	chosen := s.chosen
	s.mu.Unlock()
	display, found := Display{}, false
	for _, d := range displays {
		if d.Name == chosen {
			display, found = d, true
		}
	}
	if !found && len(displays) > 0 {
		// Until the user picks one, the display in use, as the active bar tells.
		display = displays[0]
		for _, d := range displays {
			if anchors.row != nil && anchors.row.MidY() >= d.Frame.Y && anchors.row.MidY() <= d.Frame.Y+d.Frame.H &&
				anchors.row.MidX() >= d.Frame.X && anchors.row.MidX() <= d.Frame.MaxX() {
				display = d
			}
		}
	}
	items := buildItems(modelInput{
		apps: apps, bars: bars, positions: positions, off: off,
		own: ownItem{s.p.ownPID(), s.ownNames}, overflow: anchors.overflow,
	}, display.Frame)

	for _, item := range items {
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
	s.update(func(snap *StoreSnapshot) {
		snap.Items = items
		snap.HasLayoutAccess = positions != nil
		snap.Displays, snap.Display = displays, display.Name
	})
}

// Move puts an item immediately left of the item with ID before, or at the right end of the bar
// when before is "", where macOS keeps it on screen longest. It runs in the background.
func (s *ItemStore) Move(id, before string) {
	go s.move(id, before)
}

func (s *ItemStore) move(id, before string) {
	s.work.Lock()
	defer s.work.Unlock()
	if id == before || s.Snapshot().Moving {
		return
	}
	s.update(func(snap *StoreSnapshot) { snap.Moving = true })
	defer s.update(func(snap *StoreSnapshot) { snap.Moving = false })

	// Positions shift whenever any app's item changes width, so measure right before moving.
	s.scan()
	items := s.Snapshot().Items
	item, ok := find(items, id)
	if !ok {
		return
	}
	anchor, hasAnchor := find(items, before)
	if hasAnchor && anchor.Section == Absent {
		return
	}
	if !item.CanMove() {
		s.setError(ErrNotInMenuBar)
		return
	}
	if inPlace, known := placed(items, id, before); known && inPlace {
		return
	}

	if positions := s.p.positions(); item.LayoutKey != "" && positions != nil {
		if target, ok := targetPosition(items, item, anchor, hasAnchor, positions); ok && s.p.setPosition(item.LayoutKey, target) {
			if s.settledInPlace(id, before) {
				s.setError(NoError)
				return
			}
			// MenuBarAgent reads the file through its own app-group domain, so a write by path only
			// reaches it when its cache happens to miss; a restarted agent reads the file afresh.
			s.p.reloadMenuBar()
			s.scan()
			inPlace, known := placed(s.Snapshot().Items, id, before)
			s.setErrorUnless(inPlace || !known)
			return
		}
	}

	target, ok := dropTarget(items, item, anchor, hasAnchor)
	if !item.Reachable || !ok {
		s.setError(ErrDidNotMove)
		return
	}
	s.p.commandDrag(Point{item.Frame.MidX(), item.Frame.MidY()}, target)
	s.setErrorUnless(s.settledInPlace(id, before))
}

// settledInPlace rescans after the bar has had a moment and checks the bar itself: the scan orders
// by the layout file, which says nothing about whether MenuBarAgent applied a write.
func (s *ItemStore) settledInPlace(id, before string) bool {
	time.Sleep(settleDelay)
	s.scan()
	inPlace, known := placed(s.Snapshot().Items, id, before)
	return inPlace && known
}

// placed checks on-screen frames: id must sit right before the item before, or rightmost when
// before is "". known is false when the bar doesn't show the items needed to tell.
func placed(items []Item, id, before string) (inPlace, known bool) {
	item, ok := find(items, id)
	if !ok || !item.Reachable {
		return false, false
	}
	var anchor *Item
	if before != "" {
		a, ok := find(items, before)
		if !ok || !a.Reachable {
			return false, false
		}
		anchor = &a
		if item.Frame.MidX() >= a.Frame.MidX() {
			return false, true
		}
	}
	for _, other := range items {
		if !other.Reachable || other.ID == id || (anchor != nil && other.ID == anchor.ID) {
			continue
		}
		right := other.Frame.MidX() > item.Frame.MidX()
		if anchor == nil && right {
			return false, true
		}
		if anchor != nil && right && other.Frame.MidX() < anchor.Frame.MidX() {
			return false, true
		}
	}
	return true, true
}

// targetPosition is the layout-file value that puts item where Move asked.
func targetPosition(items []Item, item, anchor Item, hasAnchor bool, positions map[string]float64) (float64, bool) {
	var existing []float64
	for key, v := range positions {
		if key != item.LayoutKey {
			existing = append(existing, v)
		}
	}
	if hasAnchor {
		if !anchor.HasPosition {
			return 0, false
		}
		return PositionLeftOf(anchor.Position, existing), true
	}
	rightmost, found := 0.0, false
	for _, other := range items {
		if other.ID != item.ID && other.HasPosition && other.Section != Absent && (!found || other.Position < rightmost) {
			rightmost, found = other.Position, true
		}
	}
	if !found {
		return 0, false
	}
	return PositionRightOf(rightmost, existing), true
}

// dropTarget is where a ⌘-drag must end for Move's placement; only on-screen items can anchor it.
func dropTarget(items []Item, item, anchor Item, hasAnchor bool) (Point, bool) {
	if hasAnchor {
		return DropPointLeftOf(anchor.Frame), anchor.Reachable
	}
	var rightmost *Item
	for i := range items {
		other := items[i]
		if other.ID != item.ID && other.Reachable && (rightmost == nil || other.Frame.MidX() > rightmost.Frame.MidX()) {
			rightmost = &items[i]
		}
	}
	if rightmost == nil {
		return Point{}, false
	}
	return DropPointRightOf(rightmost.Frame), true
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

// RequestLayoutAccess asks the user to pick the layout file. The panel waits for the user, so it
// runs in the background.
func (s *ItemStore) RequestLayoutAccess() {
	go func() {
		if s.p.requestLayoutAccess() {
			s.setError(NoError)
		} else {
			s.setError(ErrNoLayoutAccess)
		}
		s.Refresh()
	}()
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
