package barmanager

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"powerfultop/internal/notify"
)

// TrayError is why the last tray action failed; the page words it.
type TrayError int

const (
	TrayNoError TrayError = iota
	// ErrTrayNoAccess: the "Allow in the Menu Bar" list can't be read; its file wasn't picked.
	ErrTrayNoAccess
	// ErrTrayUntracked: MenuBarAgent hasn't listed the app yet, so it has no switch.
	ErrTrayUntracked
	// ErrTrayCannotOpen: the item neither took a press nor came back to the menu bar.
	ErrTrayCannotOpen
)

// TrayEntry is an item in the tray. Apps go in whole: macOS switches an app's items together.
type TrayEntry struct {
	// ID is the app's bundle identifier, or a SystemItem's ID.
	ID      string
	Name    string
	NameZh  string
	AppPath string
	// Icon is a lucide icon for Apple's items, which have no app logo.
	Icon    string
	Running bool
	// SwitchedOn: switched back on outside the tray, in System Settings, so it shows in the menu
	// bar. The tray doesn't guess whether the user meant it to leave; the page asks.
	SwitchedOn bool
}

// TraySnapshot is what the tray's flyout and page show; safe to read from any goroutine.
type TraySnapshot struct {
	Supported          bool
	Enabled            bool
	Entries            []TrayEntry
	HasAllowListAccess bool
	Error              TrayError
	// ErrorID is the entry or app Error is about.
	ErrorID string
}

// MenuEntry is an entry of a tray item's menu as Accessibility reads it; pressing it presses the
// real one, which runs it even while the menu is closed.
type MenuEntry struct {
	Title   string
	Enabled bool
	// Mark is the check mark character: "" for none, "-" for mixed.
	Mark string
	Key  string
	// Modifiers are AXMenuItemCmdModifiers: 1 Shift, 2 Option, 4 Control, 8 no Command.
	Modifiers int
	Submenu   []MenuEntry
	element   axRef
}

func (e MenuEntry) IsSeparator() bool { return e.Title == "" && e.Submenu == nil && e.Key == "" }

// axRef is an AXUIElement the platform retained; 0 for none.
type axRef uintptr

type trayPlatform interface {
	// allowList is nil when the list can't be read.
	allowList() []any
	setAllowList(list []any) bool
	requestAllowListAccess() bool
	checkbox(key string) (int, bool)
	setCheckbox(key string, w checkboxWrite)
	flag(f flagSwitch) (bool, bool)
	setFlag(f flagSwitch, value bool)
	setLoginItem(l loginItem, running bool)
	// restart quits the app with bundleID for launchd to start again: the processes behind
	// Apple's items read their switches as they start, not as they change.
	restart(bundleID string)
	// drawn is every item MenuBarAgent draws now, by tray ID: apps' bundle IDs and SystemItem IDs.
	drawn() map[string]Rect
	runningApp(bundleID string) (appRef, bool)
	appPath(bundleID string) string
	appIcon(path string) []byte
	launch(path string)
	positions() map[string]float64
	setPositions(values map[string]float64) bool
	reloadMenuBar()
	// extra is the item as its app exposes it, which stays while the item is switched off.
	extra(bundleID string) axRef
	// onBar is the pressable item MenuBarAgent draws for id right now.
	onBar(id string) axRef
	menu(el axRef) []MenuEntry
	press(el axRef)
	release(el axRef)
	click(at Point)
	// showingUI: a menu hangs from a menu bar, or pid shows a window above the normal level.
	showingUI(pid int32) bool
	ownBundleID() string
	// onBarChange calls fn, from any goroutine, a moment after MenuBarAgent lays a bar out again.
	onBarChange(fn func()) (stop func())
}

type prefStore interface {
	Value(key string) (any, bool)
	Bool(key string) (bool, bool)
	Set(key string, value any) error
}

const (
	trayItemsKey   = "menuBarTrayItems"
	trayEnabledKey = "menuBarTrayEnabled"
	// traySavedKey keeps what hiding a system item replaced, in the app's own preferences so it
	// outlasts a crash.
	traySavedKey = "menuBarTraySystemValues"

	reconcileInterval = 30 * time.Second
	reconcileSettle   = time.Second
	// MenuBarAgent takes well under a second to apply a switch.
	confirmDelay = 1500 * time.Millisecond
	// A restart that didn't help wouldn't help again right away, and each one blinks the bar.
	restartGap = 30 * time.Second

	barPollTries    = 50
	barPollInterval = 30 * time.Millisecond
	// Most items open on a press; a few only take a real click.
	pressSettle   = 350 * time.Millisecond
	watchInterval = 150 * time.Millisecond
	// An item that never showed anything is put away again rather than left in the bar.
	watchNoUI   = 4 * time.Second
	restoreWait = 2 * time.Second
)

// TrayStore keeps items out of the menu bar with macOS's own switches, the ones under
// System Settings › Menu Bar, and opens them from the tray's flyout. Unlike pushing items aside,
// which macOS 27 no longer allows, or its exam mode, which also hides Focus and the camera
// indicator, the switches take away only the item. Quitting puts every item back.
type TrayStore struct {
	p     trayPlatform
	prefs prefStore
	// overflowName is the autosave name of the tray's own item; items brought back to open
	// sit right of it, so what they open hangs near the flyout.
	overflowName string

	mu      sync.Mutex
	snap    TraySnapshot
	members []string
	changes notify.Listeners
	icons   map[string][]byte
	stop    chan struct{}
	// menuRefs are the elements of the menu last read, released when the next one is.
	menuRefs    []axRef
	opened      *openedItem
	openedGen   int
	lastRestart time.Time
	// lost are apps whose list entry went away; MenuBarAgent's new entry is allowed by default,
	// which is not the user switching it on.
	lost map[string]bool
	// conflicts are members switched on outside the tray, found by reconcile.
	conflicts map[string]bool
	// closed is set as the app quits; nothing switches after it.
	closed bool

	// work serializes switching, which reads and writes the same lists.
	work sync.Mutex

	sleep func(time.Duration)
	now   func() time.Time
	// logf records what the tray did and why, so an item leaving the tray can be explained.
	logf func(format string, args ...any)
}

// openedItem is a tray item brought back to the menu bar to open, because its app doesn't
// expose it while switched off.
type openedItem struct {
	id  string
	pid int32
}

func newTrayStore(p trayPlatform, prefs prefStore, supported bool, overflowName string) *TrayStore {
	s := &TrayStore{
		p: p, prefs: prefs, overflowName: overflowName,
		icons: map[string][]byte{}, lost: map[string]bool{}, conflicts: map[string]bool{},
		sleep: time.Sleep, now: time.Now, logf: func(string, ...any) {},
	}
	s.snap.Supported = supported
	s.snap.Enabled, _ = prefs.Bool(trayEnabledKey)
	if v, ok := prefs.Value(trayItemsKey); ok {
		list, _ := v.([]any)
		for _, id := range list {
			if id, ok := id.(string); ok && id != "" {
				s.members = append(s.members, id)
			}
		}
	}
	return s
}

func (s *TrayStore) Snapshot() TraySnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := s.snap
	snap.Entries = append([]TrayEntry(nil), s.snap.Entries...)
	return snap
}

// OnChange registers fn, called from the store's goroutines after the snapshot changes.
func (s *TrayStore) OnChange(fn func()) {
	s.changes.Add(fn)
}

func (s *TrayStore) update(change func(*TraySnapshot)) {
	s.mu.Lock()
	change(&s.snap)
	s.mu.Unlock()
	s.changes.Notify()
}

// Icon is the PNG of the entry's app, once fetched.
func (s *TrayStore) Icon(e TrayEntry) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.icons[e.AppPath]
}

// Contains reports whether id, a bundle identifier or SystemItem ID, is in the tray.
func (s *TrayStore) Contains(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return contains(s.members, id)
}

func (s *TrayStore) active() bool {
	snap := s.Snapshot()
	return snap.Supported && snap.Enabled && !s.isClosed()
}

// Start puts the tray's items away again, as a crash or the last quit left them, and keeps
// them away.
func (s *TrayStore) Start() {
	go func() {
		s.work.Lock()
		s.refreshEntries()
		if s.active() {
			s.switchMembers(false)
		}
		s.work.Unlock()
		if s.active() {
			s.startReconciling()
		}
	}()
}

func (s *TrayStore) SetEnabled(on bool) {
	if s.Snapshot().Enabled == on {
		return
	}
	s.logf("enabled %v", on)
	_ = s.prefs.Set(trayEnabledKey, on)
	s.update(func(snap *TraySnapshot) { snap.Enabled = on })
	if !on {
		s.stopReconciling()
		s.RestoreAll()
		return
	}
	go func() {
		s.work.Lock()
		s.switchMembers(false)
		s.work.Unlock()
		s.startReconciling()
	}()
}

// Shutdown puts every tray item back as the app quits and keeps the tray's list for the next
// launch. Nothing switches after it: a reconcile running while the app winds down would take the
// items put back for ones the user switched on, and drop them from the tray.
func (s *TrayStore) Shutdown() {
	s.logf("shutdown: putting back %v", s.membersCopy())
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.stopReconciling()
	s.RestoreAll()
}

func (s *TrayStore) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// RestoreAll puts every tray item back in the menu bar and keeps the tray's list for the next
// launch. It runs synchronously, on the main thread when the tray is switched off.
func (s *TrayStore) RestoreAll() {
	s.endOpened(false)
	// A switch in flight may be waiting for the main thread, which is where this runs at quit, so
	// it gets a moment to finish rather than the lock.
	for deadline := s.now().Add(restoreWait); !s.work.TryLock(); {
		if s.now().After(deadline) {
			s.switchMembers(true)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer s.work.Unlock()
	s.switchMembers(true)
}

// Add takes id, an app's bundle identifier or a SystemItem ID, out of the menu bar into the tray.
func (s *TrayStore) Add(id string) { go s.add(id) }

func (s *TrayStore) add(id string) {
	s.work.Lock()
	defer s.work.Unlock()
	if id == "" || id == s.p.ownBundleID() || s.Contains(id) || s.isClosed() {
		return
	}
	if s.active() {
		if e := s.switchItems([]string{id}, false); e != TrayNoError {
			s.logf("add %s failed: error %d", id, e)
			s.update(func(snap *TraySnapshot) { snap.Error, snap.ErrorID = e, id })
			return
		}
	}
	s.logf("add %s", id)
	s.mu.Lock()
	s.members = append(s.members, id)
	s.mu.Unlock()
	s.persist()
	s.update(func(snap *TraySnapshot) { snap.Error, snap.ErrorID = TrayNoError, "" })
	s.refreshEntries()
	if s.active() {
		go s.confirmGone([]string{id})
	}
}

// Remove puts id back in the menu bar for good: immediately left of the item with layout key
// leftOf, or where it was before it went into the tray when leftOf is "".
func (s *TrayStore) Remove(id, leftOf string) { go s.remove(id, leftOf) }

func (s *TrayStore) remove(id, leftOf string) {
	s.logf("remove %s (left of %q)", id, leftOf)
	s.endOpened(false)
	s.work.Lock()
	defer s.work.Unlock()
	s.mu.Lock()
	var kept []string
	for _, m := range s.members {
		if m != id {
			kept = append(kept, m)
		}
	}
	s.members = kept
	s.mu.Unlock()
	s.persist()
	if leftOf != "" {
		if positions := s.p.positions(); positions != nil {
			if anchor, ok := positions[leftOf]; ok {
				s.place(id, positions, func(existing []float64) float64 { return PositionLeftOf(anchor, existing) })
			}
		}
	}
	// A position written while the item is off is where it comes back.
	s.switchItems([]string{id}, true)
	s.update(func(snap *TraySnapshot) { snap.Error, snap.ErrorID = TrayNoError, "" })
	s.refreshEntries()
}

// RequestAllowListAccess asks the user to pick the list's file. The panel waits for the user, so
// it runs in the background.
func (s *TrayStore) RequestAllowListAccess() { go s.requestAllowListAccess() }

func (s *TrayStore) requestAllowListAccess() {
	ok := s.p.requestAllowListAccess()
	s.update(func(snap *TraySnapshot) {
		snap.HasAllowListAccess = ok
		if !ok {
			snap.Error, snap.ErrorID = ErrTrayNoAccess, ""
		} else if snap.Error == ErrTrayNoAccess {
			snap.Error = TrayNoError
		}
	})
	if ok && s.active() {
		s.work.Lock()
		defer s.work.Unlock()
		s.switchMembers(false)
	}
}

// RefreshMenuBar restarts MenuBarAgent, which lists an app it hadn't yet, then retries the add
// that failed for it. The menu bar blinks once.
func (s *TrayStore) RefreshMenuBar() {
	go func() {
		snap := s.Snapshot()
		s.mu.Lock()
		s.lastRestart = s.now()
		s.mu.Unlock()
		s.p.reloadMenuBar()
		if snap.Error == ErrTrayUntracked && snap.ErrorID != "" {
			s.add(snap.ErrorID)
		}
	}()
}

func (s *TrayStore) membersCopy() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.members...)
}

func (s *TrayStore) lostCopy() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for id := range s.lost {
		out = append(out, id)
	}
	return out
}

func (s *TrayStore) persist() {
	s.mu.Lock()
	list := make([]any, len(s.members))
	for i, m := range s.members {
		list[i] = m
	}
	s.mu.Unlock()
	_ = s.prefs.Set(trayItemsKey, list)
}

// refreshEntries rebuilds the entries from the members, fetching icons it hasn't yet.
func (s *TrayStore) refreshEntries() {
	s.mu.Lock()
	members := append([]string(nil), s.members...)
	s.mu.Unlock()
	entries := make([]TrayEntry, 0, len(members))
	for _, id := range members {
		if sys, ok := systemItem(id); ok {
			entries = append(entries, TrayEntry{ID: id, Name: sys.Name, NameZh: sys.NameZh, Icon: sys.Icon, Running: true})
			continue
		}
		e := TrayEntry{ID: id}
		if app, ok := s.p.runningApp(id); ok {
			e.Name, e.AppPath, e.Running = app.Name, app.Path, true
		} else {
			e.AppPath = s.p.appPath(id)
			e.Name = strings.TrimSuffix(filepath.Base(e.AppPath), ".app")
		}
		if e.Name == "" || e.Name == "." {
			e.Name = id
		}
		entries = append(entries, e)
		s.mu.Lock()
		_, have := s.icons[e.AppPath]
		s.mu.Unlock()
		if !have && e.AppPath != "" {
			icon := s.p.appIcon(e.AppPath)
			s.mu.Lock()
			s.icons[e.AppPath] = icon
			s.mu.Unlock()
		}
	}
	s.mu.Lock()
	for i := range entries {
		entries[i].SwitchedOn = s.conflicts[entries[i].ID]
	}
	s.mu.Unlock()
	access := s.p.allowList() != nil
	s.update(func(snap *TraySnapshot) {
		snap.Entries = entries
		snap.HasAllowListAccess = access
	})
}

// switchMembers shows or hides every member at once; callers hold work.
func (s *TrayStore) switchMembers(shown bool) {
	s.mu.Lock()
	members := append([]string(nil), s.members...)
	s.mu.Unlock()
	if len(members) == 0 {
		return
	}
	e := s.switchItems(members, shown)
	if e == ErrTrayNoAccess {
		s.update(func(snap *TraySnapshot) { snap.HasAllowListAccess = false })
	}
	if !shown {
		go s.confirmGone(members)
	}
}

// switchItems flips the switches of ids; callers hold work. Apps without a list entry are
// remembered as lost, for reconcile to bring back.
func (s *TrayStore) switchItems(ids []string, shown bool) TrayError {
	var apps, restarts []string
	for _, id := range ids {
		sys, ok := systemItem(id)
		if !ok {
			apps = append(apps, id)
			continue
		}
		s.switchSystem(sys, shown)
		owner := controlCenterDomain
		if sys.Flag != nil {
			owner = sys.ID
		}
		if !contains(restarts, owner) {
			restarts = append(restarts, owner)
		}
	}
	for _, owner := range restarts {
		s.p.restart(owner)
	}
	s.mu.Lock()
	for _, id := range ids {
		delete(s.conflicts, id)
	}
	s.mu.Unlock()
	if len(apps) == 0 {
		return TrayNoError
	}
	list := s.p.allowList()
	if list == nil {
		return ErrTrayNoAccess
	}
	out, changed, missing := settingAllowed(list, s.allowOwners(apps), shown)
	if changed && !s.p.setAllowList(out) {
		return ErrTrayNoAccess
	}
	if len(missing) > 0 {
		if len(ids) == 1 {
			return ErrTrayUntracked
		}
		if !shown {
			s.mu.Lock()
			for _, id := range missing {
				s.lost[id] = true
			}
			s.mu.Unlock()
		}
	}
	return TrayNoError
}

func (s *TrayStore) switchSystem(sys SystemItem, shown bool) {
	if sys.Flag != nil {
		s.p.setFlag(*sys.Flag, shown != sys.Flag.hides)
		return
	}
	if sys.Key != "" {
		current, has := s.p.checkbox(sys.Key)
		savedAll := map[string]any{}
		if v, ok := s.prefs.Value(traySavedKey); ok {
			if m, ok := v.(map[string]any); ok {
				savedAll = m
			}
		}
		saved, hasSaved := 0, false
		if v, ok := savedAll[sys.Key].(int64); ok {
			saved, hasSaved = int(v), true
		}
		w, keep, keepSaved := switchingCheckbox(shown, current, has, saved, hasSaved)
		if keepSaved {
			savedAll[sys.Key] = int64(keep)
		} else {
			delete(savedAll, sys.Key)
		}
		_ = s.prefs.Set(traySavedKey, savedAll)
		if !w.keep {
			s.p.setCheckbox(sys.Key, w)
		}
	}
	if sys.LoginItem != nil {
		s.p.setLoginItem(*sys.LoginItem, shown)
	}
}

// allowOwner names the app's process as the list may: by executable too, for an ad-hoc binary.
func (s *TrayStore) allowOwner(bundleID string) allowOwner {
	o := allowOwner{bundleID: bundleID}
	if app, ok := s.p.runningApp(bundleID); ok {
		o.executable = app.Executable
	}
	return o
}

func (s *TrayStore) allowOwners(bundleIDs []string) []allowOwner {
	owners := make([]allowOwner, len(bundleIDs))
	for i, id := range bundleIDs {
		owners[i] = s.allowOwner(id)
	}
	return owners
}

// systemShown reads whether a system item's own switch says shown.
func (s *TrayStore) systemShown(sys SystemItem) (bool, bool) {
	if sys.Flag != nil {
		v, ok := s.p.flag(*sys.Flag)
		return v != sys.Flag.hides, ok
	}
	v, has := s.p.checkbox(sys.Key)
	// A missing key is the item's default, which shows it.
	return !has || v != controlCenterHidden, true
}

// confirmGone checks that switched-off items left the bar: once, MenuBarAgent stopped applying
// changes until it was restarted.
func (s *TrayStore) confirmGone(ids []string) {
	s.sleep(confirmDelay)
	drawn := s.p.drawn()
	stuck := false
	for _, id := range ids {
		if _, ok := drawn[id]; ok && s.Contains(id) && !s.isOpened(id) {
			stuck = true
		}
	}
	if stuck && s.restartAgent() {
		s.logf("confirm: %v still drawn after being switched off; restarted MenuBarAgent", ids)
	}
}

func (s *TrayStore) restartAgent() bool {
	s.mu.Lock()
	if !s.lastRestart.IsZero() && s.now().Sub(s.lastRestart) < restartGap {
		s.mu.Unlock()
		return false
	}
	s.lastRestart = s.now()
	s.mu.Unlock()
	s.p.reloadMenuBar()
	return true
}

func (s *TrayStore) startReconciling() {
	s.mu.Lock()
	if s.stop != nil {
		s.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	s.stop = stop
	s.mu.Unlock()
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
			select {
			case <-stop:
				return
			case <-changed:
				// What the tray just switched lays the bar out too; let it settle first.
				s.sleep(reconcileSettle)
				select {
				case <-changed:
				default:
				}
				s.reconcile()
			case <-time.After(reconcileInterval):
				s.reconcile()
			}
		}
	}()
}

func (s *TrayStore) stopReconciling() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stop != nil {
		close(s.stop)
		s.stop = nil
	}
}

// Reconcile catches up with changes made behind the tray's back; the flyout runs it as it opens.
func (s *TrayStore) Reconcile() { go s.reconcile() }

func (s *TrayStore) reconcile() {
	if !s.active() {
		return
	}
	s.work.Lock()
	defer s.work.Unlock()
	// The tray may have been switched off, or the app begun quitting, while this waited.
	if !s.active() {
		return
	}
	s.mu.Lock()
	members := append([]string(nil), s.members...)
	s.mu.Unlock()

	var again []string
	conflicts := map[string]bool{}
	needRestart := false
	list := s.p.allowList()
	drawn := s.p.drawn()
	for _, id := range members {
		if s.isOpened(id) {
			continue
		}
		if sys, ok := systemItem(id); ok {
			if shown, ok := s.systemShown(sys); ok && shown {
				conflicts[id] = true
			}
			continue
		}
		if list == nil {
			continue
		}
		s.mu.Lock()
		lost := s.lost[id]
		s.mu.Unlock()
		on, listed := allowedState(list, s.allowOwner(id))
		switch {
		case !listed:
			// System Settings' Menu Bar pane drops entries of apps it can't find in a permanent
			// place, and MenuBarAgent then draws their items; a restart lists them again.
			if _, isDrawn := drawn[id]; isDrawn {
				s.mu.Lock()
				s.lost[id] = true
				s.mu.Unlock()
				needRestart = true
			}
		case on && lost:
			again = append(again, id)
		case on:
			conflicts[id] = true
		}
	}
	s.mu.Lock()
	for id := range conflicts {
		if !s.conflicts[id] {
			s.logf("reconcile: %s was switched on outside the tray", id)
		}
	}
	s.conflicts = conflicts
	s.mu.Unlock()
	if needRestart && s.restartAgent() {
		s.logf("reconcile: restarted MenuBarAgent to relist %v", s.lostCopy())
		s.mu.Lock()
		for id := range s.lost {
			again = append(again, id)
		}
		s.mu.Unlock()
	}
	if len(again) > 0 {
		list := s.p.allowList()
		if out, changed, missing := settingAllowed(list, s.allowOwners(again), false); list != nil {
			if changed {
				s.p.setAllowList(out)
			}
			s.mu.Lock()
			for _, id := range again {
				if !contains(missing, id) {
					delete(s.lost, id)
				}
			}
			s.mu.Unlock()
		}
	}
	s.refreshEntries()
}

// Rehide switches a member that was switched on outside the tray off again.
func (s *TrayStore) Rehide(id string) {
	go func() {
		s.work.Lock()
		defer s.work.Unlock()
		if !s.Contains(id) || !s.active() {
			return
		}
		s.logf("rehide %s", id)
		s.switchItems([]string{id}, false)
		s.refreshEntries()
	}()
}

// Open opens a tray item. An app's menu is returned for the flyout to show a copy of, whose
// entries Press presses; otherwise beforePress runs, for the flyout to close, and the item is
// pressed or brought back to the menu bar to open, and put away again once what it opened
// closes. Call it off the main thread: Accessibility blocks until the app answers.
func (s *TrayStore) Open(id string, beforePress func()) ([]MenuEntry, bool) {
	s.endOpened(true)
	s.releaseMenu()
	ready := func() {
		if beforePress != nil {
			beforePress()
		}
	}
	if _, isSystem := systemItem(id); !isSystem {
		app, running := s.p.runningApp(id)
		if !running {
			ready()
			if path := s.p.appPath(id); path != "" {
				s.p.launch(path)
				return nil, true
			}
			return nil, false
		}
		if el := s.p.extra(app.BundleID); el != 0 {
			if entries := s.p.menu(el); len(entries) > 0 {
				s.mu.Lock()
				s.menuRefs = append(collectRefs(entries, nil), el)
				s.mu.Unlock()
				return entries, true
			}
			ready()
			// A popover rather than a menu: it opens where the item was last drawn.
			s.p.press(el)
			s.p.release(el)
			return nil, true
		}
	}
	ready()
	return nil, s.openInMenuBar(id)
}

// CanHold reports whether id can go in the tray: not PowerfulTop itself, nor Apple's items
// without a switch, such as the clock.
func (s *TrayStore) CanHold(id string) bool {
	if id == "" || id == s.p.ownBundleID() || id == menuBarAgentID {
		return false
	}
	if _, ok := systemItem(id); ok {
		return true
	}
	return !strings.HasPrefix(id, "com.apple.menuextra.")
}

// PlaceOverflow gives the tray's own item a first position just left of the app's item named
// statusName, before the item first registers; a new item otherwise lands at the left end,
// where macOS tucks it behind « first.
func (s *TrayStore) PlaceOverflow(statusName string) {
	positions := s.p.positions()
	if positions == nil {
		return
	}
	if _, ok := s.overflowPosition(positions); ok {
		return
	}
	own := s.owners(s.p.ownBundleID())
	for key, v := range positions {
		owner, name, ok := ParseKey(key)
		if !ok || name != statusName || !contains(own, owner) {
			continue
		}
		var existing []float64
		for _, other := range positions {
			existing = append(existing, other)
		}
		s.p.setPositions(map[string]float64{"status:" + owner + "::" + s.overflowName: PositionLeftOf(v, existing)})
		return
	}
}

// Press runs an entry of the menu Open returned.
func (s *TrayStore) Press(e MenuEntry) {
	go s.p.press(e.element)
}

func collectRefs(entries []MenuEntry, refs []axRef) []axRef {
	for _, e := range entries {
		if e.element != 0 {
			refs = append(refs, e.element)
		}
		refs = collectRefs(e.Submenu, refs)
	}
	return refs
}

func (s *TrayStore) releaseMenu() {
	s.mu.Lock()
	refs := s.menuRefs
	s.menuRefs = nil
	s.mu.Unlock()
	for _, r := range refs {
		s.p.release(r)
	}
}

func (s *TrayStore) openInMenuBar(id string) bool {
	s.logf("open %s in the menu bar", id)
	s.work.Lock()
	if positions := s.p.positions(); positions != nil {
		if overflow, ok := s.overflowPosition(positions); ok {
			s.place(id, positions, func(existing []float64) float64 { return PositionRightOf(overflow, existing) })
		}
	}
	s.mu.Lock()
	s.openedGen++
	gen := s.openedGen
	s.opened = &openedItem{id: id}
	s.mu.Unlock()
	if e := s.switchItems([]string{id}, true); e != TrayNoError {
		s.work.Unlock()
		s.endOpened(true)
		s.update(func(snap *TraySnapshot) { snap.Error, snap.ErrorID = e, id })
		return false
	}
	s.work.Unlock()

	var el axRef
	for range barPollTries {
		if el = s.p.onBar(id); el != 0 {
			break
		}
		s.sleep(barPollInterval)
	}
	if el == 0 {
		s.endOpened(true)
		s.update(func(snap *TraySnapshot) { snap.Error, snap.ErrorID = ErrTrayCannotOpen, id })
		return false
	}
	pid := s.ownerPID(id)
	s.p.press(el)
	s.p.release(el)
	s.sleep(pressSettle)
	if !s.p.showingUI(pid) {
		if f, ok := s.p.drawn()[id]; ok {
			s.p.click(Point{f.MidX(), f.MidY()})
		}
	}
	go s.watchOpened(gen, pid)
	return true
}

// ownerPID is the process whose windows show what the item opened: Control Center's for
// MenuBarAgent's own items.
func (s *TrayStore) ownerPID(id string) int32 {
	owner := id
	if sys, ok := systemItem(id); ok && sys.Key != "" && sys.LoginItem == nil {
		owner = controlCenterDomain
	}
	if app, ok := s.p.runningApp(owner); ok {
		return app.PID
	}
	return 0
}

// watchOpened puts the item away once what it opened has closed; hiding while its menu is open
// would pull the item out from under the menu.
func (s *TrayStore) watchOpened(gen int, pid int32) {
	start := s.now()
	sawUI := false
	for {
		s.sleep(watchInterval)
		s.mu.Lock()
		current := s.openedGen == gen && s.opened != nil
		s.mu.Unlock()
		if !current {
			return
		}
		if s.p.showingUI(pid) {
			sawUI = true
			continue
		}
		if sawUI || s.now().Sub(start) > watchNoUI {
			s.endOpenedGen(gen, true)
			return
		}
	}
}

func (s *TrayStore) isOpened(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opened != nil && s.opened.id == id
}

func (s *TrayStore) endOpened(rehide bool) {
	s.mu.Lock()
	gen := s.openedGen
	s.mu.Unlock()
	s.endOpenedGen(gen, rehide)
}

func (s *TrayStore) endOpenedGen(gen int, rehide bool) {
	s.mu.Lock()
	if s.openedGen != gen || s.opened == nil {
		s.mu.Unlock()
		return
	}
	id := s.opened.id
	s.opened = nil
	s.mu.Unlock()
	s.logf("opened %s closed (rehide %v)", id, rehide)
	if rehide && s.Contains(id) && s.active() {
		s.work.Lock()
		s.switchItems([]string{id}, false)
		s.work.Unlock()
		go s.confirmGone([]string{id})
	}
}

// overflowPosition is the layout position of the tray's own item.
func (s *TrayStore) overflowPosition(positions map[string]float64) (float64, bool) {
	own := s.owners(s.p.ownBundleID())
	for key, v := range positions {
		if owner, name, ok := ParseKey(key); ok && name == s.overflowName && contains(own, owner) {
			return v, true
		}
	}
	return 0, false
}

// owners are every name macOS may file an app's layout keys under.
func (s *TrayStore) owners(bundleID string) []string {
	if app, ok := s.p.runningApp(bundleID); ok && len(app.Owners) > 0 {
		return app.Owners
	}
	return []string{bundleID}
}

// layoutKeys are id's entries in the layout file: every item of an app.
func (s *TrayStore) layoutKeys(id string, positions map[string]float64) []string {
	if sys, ok := systemItem(id); ok {
		if key := sys.PositionKey(); key != "" {
			if _, ok := positions[key]; ok {
				return []string{key}
			}
		}
		if sys.Flag != nil || sys.LoginItem != nil {
			return s.layoutKeysOf(s.owners(id), positions)
		}
		return nil
	}
	return s.layoutKeysOf(s.owners(id), positions)
}

func (s *TrayStore) layoutKeysOf(owners []string, positions map[string]float64) []string {
	var keys []string
	for key := range positions {
		if owner, _, ok := ParseKey(key); ok && contains(owners, owner) {
			keys = append(keys, key)
		}
	}
	return keys
}

// place writes id's layout position; target picks it among the other items' positions. An app's
// several items keep their order, a hair apart.
func (s *TrayStore) place(id string, positions map[string]float64, target func(existing []float64) float64) {
	keys := s.layoutKeys(id, positions)
	if len(keys) == 0 {
		return
	}
	var existing []float64
	for key, v := range positions {
		if !contains(keys, key) {
			existing = append(existing, v)
		}
	}
	at := target(existing)
	ordered := append([]string(nil), keys...)
	sortByPosition(ordered, positions)
	values := map[string]float64{}
	for i, key := range ordered {
		values[key] = at + float64(i)*0.01
	}
	s.p.setPositions(values)
}

// sortByPosition orders keys right to left: smaller positions sit further right.
func sortByPosition(keys []string, positions map[string]float64) {
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && positions[keys[j]] < positions[keys[j-1]]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
}
