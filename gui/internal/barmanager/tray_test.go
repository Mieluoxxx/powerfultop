package barmanager

import (
	"bytes"
	"image/png"
	"reflect"
	"sync"
	"testing"
	"time"
)

type fakeTray struct {
	nullItems
	mu       sync.Mutex
	list     []any
	writes   int
	boxes    map[string]int
	flags    map[string]bool
	shown    map[string]Rect
	pos      map[string]float64
	apps     map[string]appRef
	reloads  int
	pressed  []axRef
	menus    map[axRef][]MenuEntry
	extras   map[string]axRef
	launched []string
	restarts []string
	ui       bool
}

func newFakeTray() *fakeTray {
	return &fakeTray{
		list: []any{
			allowKey("com.example.chat"), allowValue("com.example.chat", true),
			allowKey("com.example.vpn"), allowValue("com.example.vpn", true),
		},
		boxes:  map[string]int{"Sound": 2},
		flags:  map[string]bool{},
		shown:  map[string]Rect{"com.example.chat": {X: 900, W: 30, H: 24}, "com.example.vpn": {X: 940, W: 30, H: 24}},
		pos:    map[string]float64{"status:com.example.chat::Item-0": 300, "status:PowerfulTop::PowerfulTop.overflow": 200, "status:com.example.vpn::Item-0": 100},
		apps:   map[string]appRef{"com.example.chat": {PID: 10, Name: "Chat", BundleID: "com.example.chat", Path: "/Applications/Chat.app", Owners: []string{"com.example.chat"}}, "io.powerfultop": {PID: 1, BundleID: "io.powerfultop", Owners: []string{"io.powerfultop", "PowerfulTop"}}},
		menus:  map[axRef][]MenuEntry{},
		extras: map[string]axRef{},
	}
}

func (f *fakeTray) allowList() []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]any(nil), f.list...)
}
func (f *fakeTray) setAllowList(list []any) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.list = list
	f.writes++
	for i := 0; i+1 < len(list); i += 2 {
		if entry, _ := list[i+1].(map[string]any); entry["isAllowed"] == false {
			delete(f.shown, allowKeyBundle(list[i]))
		}
	}
	return true
}
func (f *fakeTray) allowed(id string) (bool, bool) {
	return allowedState(f.allowList(), allowOwner{bundleID: id})
}
func (f *fakeTray) checkbox(key string) (int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.boxes[key]
	return v, ok
}
func (f *fakeTray) setCheckbox(key string, w checkboxWrite) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if w.remove {
		delete(f.boxes, key)
	} else {
		f.boxes[key] = w.value
	}
}
func (f *fakeTray) flag(s flagSwitch) (bool, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.flags[s.key]
	return v, ok
}
func (f *fakeTray) setFlag(s flagSwitch, v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flags[s.key] = v
}
func (f *fakeTray) drawn() map[string]Rect {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]Rect{}
	for k, v := range f.shown {
		out[k] = v
	}
	return out
}
func (f *fakeTray) runningApp(id string) (appRef, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.apps[id]
	return a, ok
}
func (f *fakeTray) appPath(id string) string { return "/Applications/" + id + ".app" }
func (f *fakeTray) launch(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.launched = append(f.launched, path)
}
func (f *fakeTray) positions() map[string]float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]float64{}
	for k, v := range f.pos {
		out[k] = v
	}
	return out
}
func (f *fakeTray) setPositions(values map[string]float64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k, v := range values {
		f.pos[k] = v
	}
	return true
}
func (f *fakeTray) reloadMenuBar() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reloads++
}
func (f *fakeTray) extra(id string) axRef {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.extras[id]
}
func (f *fakeTray) onBar(id string) axRef {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.shown[id]; ok {
		return axRef(len(id))
	}
	return 0
}
func (f *fakeTray) menu(el axRef) []MenuEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.menus[el]
}
func (f *fakeTray) press(el axRef) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pressed = append(f.pressed, el)
}
func (f *fakeTray) showingUI(int32) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ui
}
func (f *fakeTray) ownBundleID() string       { return "io.powerfultop" }
func (f *fakeTray) onBarChange(func()) func() { return func() {} }
func (f *fakeTray) restart(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restarts = append(f.restarts, id)
}

type fakePrefs struct {
	mu     sync.Mutex
	values map[string]any
}

func (p *fakePrefs) Value(key string) (any, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.values[key]
	return v, ok
}
func (p *fakePrefs) Bool(key string) (bool, bool) {
	v, ok := p.Value(key)
	b, isBool := v.(bool)
	return b, ok && isBool
}
func (p *fakePrefs) Set(key string, value any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if value == nil {
		delete(p.values, key)
	} else {
		p.values[key] = value
	}
	return nil
}

func newTestTray(t *testing.T, f *fakeTray, members ...string) (*TrayStore, *fakePrefs) {
	t.Helper()
	list := make([]any, len(members))
	for i, m := range members {
		list[i] = m
	}
	prefs := &fakePrefs{values: map[string]any{trayEnabledKey: true, trayItemsKey: list}}
	s := newTrayStore(f, prefs, true, "PowerfulTop.overflow")
	s.sleep = func(time.Duration) {}
	t.Cleanup(s.stopReconciling)
	return s, prefs
}

func TestTrayAddSwitchesTheAppOff(t *testing.T) {
	f := newFakeTray()
	s, prefs := newTestTray(t, f)
	s.add("com.example.chat")
	if allowed, _ := f.allowed("com.example.chat"); allowed {
		t.Error("chat should be switched off")
	}
	if allowed, _ := f.allowed("com.example.vpn"); !allowed {
		t.Error("vpn must stay")
	}
	if v, _ := prefs.Value(trayItemsKey); !reflect.DeepEqual(v, []any{"com.example.chat"}) {
		t.Errorf("saved %v", v)
	}
	if e := s.Snapshot().Entries; len(e) != 1 || e[0].Name != "Chat" || !e[0].Running {
		t.Errorf("entries %+v", e)
	}
}

func TestTrayAddRefusesUntrackedAndOwnApps(t *testing.T) {
	f := newFakeTray()
	s, _ := newTestTray(t, f)
	s.add("com.example.new")
	if snap := s.Snapshot(); snap.Error != ErrTrayUntracked || snap.ErrorID != "com.example.new" || s.Contains("com.example.new") {
		t.Errorf("untracked: %+v", snap)
	}
	s.add("io.powerfultop")
	if s.Contains("io.powerfultop") {
		t.Error("PowerfulTop never goes in its own tray")
	}
}

func TestTraySystemItemPutsTheUsersChoiceBack(t *testing.T) {
	f := newFakeTray()
	f.boxes["Sound"] = 18
	s, _ := newTestTray(t, f)
	s.add("com.apple.menuextra.sound")
	if v, _ := f.checkbox("Sound"); v != controlCenterHidden {
		t.Fatalf("hidden: %d", v)
	}
	s.RestoreAll()
	if v, _ := f.checkbox("Sound"); v != 18 {
		t.Errorf("restored %d, want the user's 18", v)
	}
	if !s.Contains("com.apple.menuextra.sound") {
		t.Error("quitting keeps the tray for the next launch")
	}
}

func TestTrayFlagSwitch(t *testing.T) {
	f := newFakeTray()
	s, _ := newTestTray(t, f)
	s.add(textInputID)
	if v, ok := f.flag(flagSwitch{key: "visible"}); !ok || v {
		t.Error("Text Input's visible false hides it")
	}
	s.RestoreAll()
	if v, _ := f.flag(flagSwitch{key: "visible"}); !v {
		t.Error("shown again")
	}
	if want := []string{textInputID, textInputID}; !reflect.DeepEqual(f.restarts, want) {
		t.Errorf("its own process reads the flag as it starts: restarts %v", f.restarts)
	}
}

func TestTraySystemItemsRestartControlCenterOnce(t *testing.T) {
	f := newFakeTray()
	s, _ := newTestTray(t, f, "com.apple.menuextra.sound", "com.apple.menuextra.wifi", "com.example.chat")
	s.switchMembers(false)
	if want := []string{controlCenterDomain}; !reflect.DeepEqual(f.restarts, want) {
		t.Errorf("restarts %v", f.restarts)
	}
}

func TestTrayRemovePlacesLeftOfTheAnchor(t *testing.T) {
	f := newFakeTray()
	s, _ := newTestTray(t, f, "com.example.chat")
	s.switchMembers(false)
	s.remove("com.example.chat", "status:com.example.vpn::Item-0")
	if allowed, _ := f.allowed("com.example.chat"); !allowed {
		t.Error("switched back on")
	}
	got := f.positions()["status:com.example.chat::Item-0"]
	if got <= 100 || got >= 200 {
		t.Errorf("position %v should sit just left of vpn (100), right of the next item (200)", got)
	}
	if s.Contains("com.example.chat") {
		t.Error("still a member")
	}
}

func TestTrayReconcileFlagsWhatWasSwitchedOnOutside(t *testing.T) {
	f := newFakeTray()
	s, prefs := newTestTray(t, f, "com.example.chat", "com.apple.menuextra.sound")
	s.switchMembers(false)
	// Switched back on in System Settings.
	out, _, _ := settingAllowed(f.allowList(), owners("com.example.chat"), true)
	f.setAllowList(out)
	f.setCheckbox("Sound", checkboxWrite{value: 2})
	s.reconcile()
	if !s.Contains("com.example.chat") || !s.Contains("com.apple.menuextra.sound") {
		t.Fatalf("the tray never drops members on a guess: %v", s.members)
	}
	if v, _ := prefs.Value(trayItemsKey); len(v.([]any)) != 2 {
		t.Errorf("saved %v", v)
	}
	for _, e := range s.Snapshot().Entries {
		if !e.SwitchedOn {
			t.Errorf("%s should be flagged", e.ID)
		}
	}
	s.Rehide("com.example.chat")
	waitFor(t, "rehidden", func() bool {
		allowed, _ := f.allowed("com.example.chat")
		return !allowed
	})
	waitFor(t, "flag cleared", func() bool {
		for _, e := range s.Snapshot().Entries {
			if e.ID == "com.example.chat" {
				return !e.SwitchedOn
			}
		}
		return false
	})
}

func TestTrayReconcileRelistsALostEntry(t *testing.T) {
	f := newFakeTray()
	s, _ := newTestTray(t, f, "com.example.chat")
	s.switchMembers(false)
	// System Settings dropped the entry, and MenuBarAgent draws the item again.
	f.mu.Lock()
	f.list = f.list[2:]
	f.shown["com.example.chat"] = Rect{X: 900, W: 30, H: 24}
	f.mu.Unlock()
	s.reconcile()
	if f.reloads != 1 {
		t.Fatalf("reloads %d", f.reloads)
	}
	// After the restart MenuBarAgent lists it again, allowed by default.
	f.mu.Lock()
	f.list = append(f.list, allowKey("com.example.chat"), allowValue("com.example.chat", true))
	f.mu.Unlock()
	s.reconcile()
	if allowed, _ := f.allowed("com.example.chat"); allowed {
		t.Error("the new entry is MenuBarAgent's default, not the user's choice: switch it off again")
	}
	if !s.Contains("com.example.chat") {
		t.Error("still in the tray")
	}
}

func TestTrayOpenShowsTheAppsMenu(t *testing.T) {
	f := newFakeTray()
	f.extras["com.example.chat"] = 7
	f.menus[7] = []MenuEntry{{Title: "New Chat", Enabled: true, element: 8}, {}, {Title: "Quit", Enabled: true, element: 9}}
	s, _ := newTestTray(t, f, "com.example.chat")
	entries, ok := s.Open("com.example.chat", nil)
	if !ok || len(entries) != 3 || !entries[1].IsSeparator() {
		t.Fatalf("entries %+v", entries)
	}
	s.p.press(entries[2].element)
	if !reflect.DeepEqual(f.pressed, []axRef{9}) {
		t.Errorf("pressed %v", f.pressed)
	}
}

func TestTrayOpenPressesAPopoverItem(t *testing.T) {
	f := newFakeTray()
	f.extras["com.example.chat"] = 7
	s, _ := newTestTray(t, f, "com.example.chat")
	if entries, ok := s.Open("com.example.chat", nil); !ok || entries != nil {
		t.Fatalf("got %v %v", entries, ok)
	}
	if !reflect.DeepEqual(f.pressed, []axRef{7}) {
		t.Errorf("pressed %v", f.pressed)
	}
}

func TestTrayOpenLaunchesAnAppThatIsntRunning(t *testing.T) {
	f := newFakeTray()
	s, _ := newTestTray(t, f, "com.example.vpn")
	if _, ok := s.Open("com.example.vpn", nil); !ok {
		t.Fatal("not opened")
	}
	if !reflect.DeepEqual(f.launched, []string{"/Applications/com.example.vpn.app"}) {
		t.Errorf("launched %v", f.launched)
	}
}

func TestTrayOpenBringsASystemItemBackAndPutsItAwayAfter(t *testing.T) {
	f := newFakeTray()
	f.pos["module:Sound"] = 500
	f.ui = true
	s, _ := newTestTray(t, f, "com.apple.menuextra.sound")
	var mu sync.Mutex
	steps, armed := 0, false
	s.sleep = func(time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		if !armed {
			return
		}
		steps++
		// MenuBarAgent draws it again as soon as its switch is on.
		if v, _ := f.checkbox("Sound"); v != controlCenterHidden {
			f.mu.Lock()
			f.shown["com.apple.menuextra.sound"] = Rect{X: 950, W: 30, H: 24}
			f.mu.Unlock()
		}
		// Its panel closes after a few polls.
		if steps > 4 {
			f.mu.Lock()
			f.ui = false
			f.mu.Unlock()
		}
	}
	s.switchMembers(false)
	mu.Lock()
	armed = true
	mu.Unlock()
	if _, ok := s.Open("com.apple.menuextra.sound", nil); !ok {
		t.Fatal("not opened")
	}
	if p := f.positions()["module:Sound"]; p >= 200 || p <= 100 {
		t.Errorf("position %v should sit right of the tray's item (200)", p)
	}
	if len(f.pressed) != 1 {
		t.Errorf("pressed %v", f.pressed)
	}
	waitFor(t, "put away", func() bool {
		v, _ := f.checkbox("Sound")
		return v == controlCenterHidden && !s.isOpened("com.apple.menuextra.sound")
	})
}

func TestTrayDisabledLeavesTheBarAlone(t *testing.T) {
	f := newFakeTray()
	s, prefs := newTestTray(t, f)
	prefs.Set(trayEnabledKey, false)
	s.snap.Enabled = false
	s.add("com.example.chat")
	if allowed, _ := f.allowed("com.example.chat"); !allowed {
		t.Error("a disabled tray keeps its list but switches nothing")
	}
	s.SetEnabled(true)
	waitFor(t, "switched off", func() bool {
		allowed, _ := f.allowed("com.example.chat")
		return !allowed
	})
	s.SetEnabled(false)
	if allowed, _ := f.allowed("com.example.chat"); !allowed {
		t.Error("disabling puts it back")
	}
}

func TestOverflowIconIsAMonochromeTemplate(t *testing.T) {
	for _, open := range []bool{false, true} {
		img, err := png.Decode(bytes.NewReader(OverflowIcon(open)))
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != 28 || b.Dy() != 32 {
			t.Errorf("size %v", b)
		}
		inked := 0
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				r, g, b, a := img.At(x, y).RGBA()
				if r|g|b != 0 {
					t.Fatal("template images are black on transparent")
				}
				if a > 0 {
					inked++
				}
			}
		}
		if inked < 40 {
			t.Errorf("open=%v: only %d pixels drawn", open, inked)
		}
	}
}

func TestTrayCanHold(t *testing.T) {
	s, _ := newTestTray(t, newFakeTray())
	for id, want := range map[string]bool{
		"com.example.chat": true, "com.apple.menuextra.wifi": true, textInputID: true,
		"io.powerfultop": false, "com.apple.menuextra.clock": false, menuBarAgentID: false, "": false,
	} {
		if got := s.CanHold(id); got != want {
			t.Errorf("%s: %v", id, got)
		}
	}
}

func TestTrayPlaceOverflowLeftOfTheStatusItem(t *testing.T) {
	f := newFakeTray()
	delete(f.pos, "status:PowerfulTop::PowerfulTop.overflow")
	f.pos["status:PowerfulTop::PowerfulTop.status"] = 250
	s, _ := newTestTray(t, f)
	s.PlaceOverflow("PowerfulTop.status")
	if v := f.positions()["status:PowerfulTop::PowerfulTop.overflow"]; v <= 250 || v >= 300 {
		t.Errorf("overflow at %v, want between the status item (250) and chat (300)", v)
	}
	f.pos["status:PowerfulTop::PowerfulTop.overflow"] = 120
	s.PlaceOverflow("PowerfulTop.status")
	if v := f.positions()["status:PowerfulTop::PowerfulTop.overflow"]; v != 120 {
		t.Errorf("a position the user gave it stays: %v", v)
	}
}

func TestTrayRestoreAllDoesntWaitForeverOnASwitchInFlight(t *testing.T) {
	f := newFakeTray()
	s, _ := newTestTray(t, f, "com.example.chat")
	s.switchMembers(false)
	// A switch in flight waiting for the main thread, where RestoreAll runs at quit.
	s.work.Lock()
	defer s.work.Unlock()
	done := make(chan struct{})
	go func() {
		s.RestoreAll()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(restoreWait + 2*time.Second):
		t.Fatal("RestoreAll waited for the lock forever")
	}
	if allowed, _ := f.allowed("com.example.chat"); !allowed {
		t.Error("quitting still puts the icon back")
	}
}

// Items put back as the app quits are on, as if the user had switched them on; a reconcile running
// while the app winds down must not drop them from the tray.
func TestTrayKeepsItsItemsAcrossQuit(t *testing.T) {
	f := newFakeTray()
	s, prefs := newTestTray(t, f, "com.example.chat", "com.apple.menuextra.sound")
	s.switchMembers(false)
	s.Shutdown()
	s.reconcile()
	if !s.Contains("com.example.chat") || !s.Contains("com.apple.menuextra.sound") {
		t.Errorf("members %v", s.members)
	}
	if v, _ := prefs.Value(trayItemsKey); len(v.([]any)) != 2 {
		t.Errorf("saved %v", v)
	}
	if allowed, _ := f.allowed("com.example.chat"); !allowed {
		t.Error("quitting puts it back")
	}
	s.add("com.example.vpn")
	if allowed, _ := f.allowed("com.example.vpn"); !allowed || s.Contains("com.example.vpn") {
		t.Error("nothing switches after Shutdown")
	}
}

// Turning the tray off puts its items back too, and a reconcile queued behind it must not take
// them for the user's.
func TestTrayKeepsItsItemsWhenSwitchedOff(t *testing.T) {
	f := newFakeTray()
	s, _ := newTestTray(t, f, "com.example.chat")
	s.switchMembers(false)
	s.RestoreAll()
	s.reconcile()
	if !s.Contains("com.example.chat") {
		t.Error("RestoreAll's items aren't the user's")
	}
}
