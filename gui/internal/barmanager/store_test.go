package barmanager

import (
	"sync"
	"testing"
	"time"
)

type fakeItems struct {
	mu       sync.Mutex
	bars     []agentBar
	pos      map[string]float64
	drags    [][2]Point
	setCalls int
	reloads  int
	// applyWrite moves the bar the way a layout-file write asked for; nil leaves the bar alone.
	applyWrite func(*fakeItems)
	// applyReload is what the bar does after MenuBarAgent restarts and rereads the file.
	applyReload func(*fakeItems)
	applyDrag   func(*fakeItems)
	isTrusted   bool
}

func (f *fakeItems) trusted() bool                  { return f.isTrusted }
func (f *fakeItems) requestTrust() bool             { return f.isTrusted }
func (f *fakeItems) runningApps() []appRef          { return testApps }
func (f *fakeItems) barAnchors([]appRef) barAnchors { return wideBar }
func (f *fakeItems) requestLayoutAccess() bool      { return false }
func (f *fakeItems) appIcon(string) []byte          { return nil }
func (f *fakeItems) ownPID() int32                  { return 1 }

func (f *fakeItems) agentBars() []agentBar {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]agentBar, len(f.bars))
	for i, b := range f.bars {
		out[i] = agentBar{Frame: b.Frame, Slots: append([]agentSlot(nil), b.Slots...)}
	}
	return out
}
func (f *fakeItems) displays() []Display {
	return []Display{{Name: "P275MV MAX", Frame: externalFrame}}
}
func (f *fakeItems) onBarChange(func()) func() { return func() {} }

// allowList has QQ switched off.
func (f *fakeItems) allowList() []any {
	return []any{allowKey("com.tencent.qq"), allowValue("com.tencent.qq", false)}
}
func (f *fakeItems) positions() map[string]float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pos == nil {
		return nil
	}
	out := map[string]float64{}
	for k, v := range f.pos {
		out[k] = v
	}
	return out
}
func (f *fakeItems) setPosition(key string, v float64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setCalls++
	f.pos[key] = v
	if f.applyWrite != nil {
		f.applyWrite(f)
	}
	return true
}
func (f *fakeItems) reloadMenuBar() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reloads++
	if f.applyReload != nil {
		f.applyReload(f)
	}
}
func (f *fakeItems) commandDrag(start, end Point) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.drags = append(f.drags, [2]Point{start, end})
	if f.applyDrag != nil {
		f.applyDrag(f)
	}
}

// slide moves the item at index to x on the bar; callers hold f.mu.
func slide(index int, x float64) func(*fakeItems) {
	return func(f *fakeItems) { f.bars[0].Slots[index].Frame.X = x }
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func visibleStore(t *testing.T, p *fakeItems) *ItemStore {
	t.Helper()
	s := newItemStore(p, &fakePrefs{values: map[string]any{}}, true, []string{"PowerfulTop.status"})
	s.SetVisible(true)
	t.Cleanup(func() { s.SetVisible(false) })
	waitFor(t, "items", func() bool { return len(s.Snapshot().Items) > 0 })
	return s
}

func TestStoreListsOnlyWhileVisibleAndTrusted(t *testing.T) {
	s := newItemStore(&fakeItems{bars: wideBars()}, &fakePrefs{values: map[string]any{}}, true, nil)
	s.SetVisible(true)
	time.Sleep(50 * time.Millisecond)
	if snap := s.Snapshot(); snap.Trusted || len(snap.Items) != 0 {
		t.Errorf("untrusted: %+v", snap)
	}
	s.SetVisible(false)

	trusted := newItemStore(&fakeItems{bars: wideBars(), isTrusted: true}, &fakePrefs{values: map[string]any{}}, true, nil)
	trusted.SetVisible(true)
	waitFor(t, "items", func() bool { return len(trusted.Snapshot().Items) == 6 })
	trusted.SetVisible(false)
	if len(trusted.Snapshot().Items) != 0 {
		t.Error("no items once the page is gone")
	}
}

func TestStoreMovesThroughLayoutFile(t *testing.T) {
	// RAM goes left of FineTune: between PowerfulTop's label and FineTune once the bar applies it.
	p := &fakeItems{bars: wideBars(), isTrusted: true, pos: filePositions(), applyReload: slide(3, 1100)}
	s := visibleStore(t, p)
	s.move("eu.exelban.Stats#RAM_mini", "com.finetuneapp.FineTune#Item-0")
	if p.setCalls != 1 || len(p.drags) != 0 {
		t.Fatalf("file moves don't drag: set %d drags %d", p.setCalls, len(p.drags))
	}
	if got := p.positions()["status:eu.exelban.Stats::RAM_mini"]; got <= 500 || got >= 700 {
		t.Errorf("the position lands between FineTune and the next item left of it: %v", got)
	}
	if p.reloads != 1 || s.Snapshot().Error != NoError {
		t.Errorf("a write the bar ignores reloads the bar once: reloads %d err %v", p.reloads, s.Snapshot().Error)
	}
}

func TestStoreSkipsReloadWhenTheBarApplies(t *testing.T) {
	p := &fakeItems{bars: wideBars(), isTrusted: true, pos: filePositions(), applyWrite: slide(3, 1100)}
	s := visibleStore(t, p)
	s.move("eu.exelban.Stats#RAM_mini", "com.finetuneapp.FineTune#Item-0")
	if p.reloads != 0 || s.Snapshot().Error != NoError {
		t.Errorf("reloads %d err %v", p.reloads, s.Snapshot().Error)
	}
}

func TestStoreMovesRightmostWithDrag(t *testing.T) {
	p := &fakeItems{bars: wideBars(), isTrusted: true, applyDrag: slide(1, 1540)}
	s := visibleStore(t, p)
	s.move("com.finetuneapp.FineTune#0", "")
	if len(p.drags) != 1 {
		t.Fatalf("drags: %d", len(p.drags))
	}
	if d := p.drags[0]; d[0] != (Point{1150, -1064.5}) || d[1] != (Point{1533, -1064.5}) {
		t.Errorf("drag from the item's centre to just right of the rightmost item: %v", d)
	}
	if s.Snapshot().Error != NoError {
		t.Errorf("landed: %v", s.Snapshot().Error)
	}
}

func TestStoreReportsMovesThatDidNotLand(t *testing.T) {
	p := &fakeItems{bars: wideBars(), isTrusted: true}
	s := visibleStore(t, p)
	s.move("eu.exelban.Stats#0", "")
	if s.Snapshot().Error != ErrDidNotMove {
		t.Errorf("a drag the bar ignored is reported: %v", s.Snapshot().Error)
	}
	s.move("com.tencent.qq#0", "")
	if s.Snapshot().Error != ErrNotInMenuBar {
		t.Error("an absent item can't be moved")
	}
}

func TestStoreUnsupportedNeverScans(t *testing.T) {
	s := newItemStore(&fakeItems{bars: wideBars(), isTrusted: true}, &fakePrefs{values: map[string]any{}}, false, nil)
	s.SetVisible(true)
	time.Sleep(50 * time.Millisecond)
	if len(s.Snapshot().Items) != 0 {
		t.Error("before macOS 27 the editor stays empty")
	}
}
