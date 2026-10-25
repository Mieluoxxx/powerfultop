package barmanager

import (
	"sync"
	"testing"
	"time"
)

type fakeItems struct {
	mu       sync.Mutex
	raw      []rawItem
	pos      map[string]float64
	drags    [][2]Point
	setCalls int
	reloads  int
	onReload func(*fakeItems)
	// applies makes layout-file writes reach the bar at once, moving FineTune left of the separator.
	applies   bool
	isTrusted bool
}

func (f *fakeItems) trusted() bool                 { return f.isTrusted }
func (f *fakeItems) requestTrust() bool            { return f.isTrusted }
func (f *fakeItems) runningApps() []appRef         { return testApps }
func (f *fakeItems) overflowButton([]appRef) *Rect { return nil }
func (f *fakeItems) requestLayoutAccess() bool     { return false }
func (f *fakeItems) ownPID() int32                 { return own }
func (f *fakeItems) appIcon(string) []byte         { return nil }
func (f *fakeItems) readItems([]int32) []rawItem {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]rawItem(nil), f.raw...)
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
	if f.applies {
		f.moveFineTuneLeft()
	}
	return true
}
func (f *fakeItems) reloadMenuBar() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reloads++
	if f.onReload != nil {
		f.onReload(f)
	}
}
func (f *fakeItems) commandDrag(start, end Point) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.drags = append(f.drags, [2]Point{start, end})
	f.moveFineTuneLeft()
}

// moveFineTuneLeft puts FineTune left of the separator; callers hold f.mu.
func (f *fakeItems) moveFineTuneLeft() {
	for i := range f.raw {
		if f.raw[i].PID == 2 {
			f.raw[i].Frame.X = 690
		}
	}
}

func newTestStore(p *fakeItems) (*ItemStore, *fakeBar) {
	m, b, _ := newTestManager(fakePrefs{enabledKey: true, autoHideDelayKey: 0.0})
	onMain(m, func() {
		m.Start()
		m.Collapse()
	})
	return newItemStore(m, p), b
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

func TestStoreHoldsBarWhileVisible(t *testing.T) {
	p := &fakeItems{raw: expandedRaw(), isTrusted: true}
	s, b := newTestStore(p)
	s.SetVisible(true)
	waitFor(t, "items", func() bool { return len(s.Snapshot().Items) > 0 })
	b.mu.Lock()
	expanded := !b.collapsed
	b.mu.Unlock()
	if !expanded {
		t.Error("the editor keeps the bar expanded")
	}
	s.SetVisible(false)
	waitFor(t, "collapse after editing", func() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.collapsed })
	if len(s.Snapshot().Items) != 0 {
		t.Error("no items once hidden")
	}
}

func TestStoreUntrustedListsNothing(t *testing.T) {
	s, _ := newTestStore(&fakeItems{raw: expandedRaw()})
	s.SetVisible(true)
	waitFor(t, "trust check", func() bool { return true })
	time.Sleep(50 * time.Millisecond)
	if snap := s.Snapshot(); snap.Trusted || len(snap.Items) != 0 {
		t.Errorf("untrusted: %+v", snap)
	}
	s.SetVisible(false)
}

func TestStoreMovesThroughLayoutFile(t *testing.T) {
	p := &fakeItems{raw: expandedRaw(), isTrusted: true, pos: map[string]float64{
		"status:PowerfulTop::PowerfulTop.MenuBarSection.separator": 928,
		"status:PowerfulTop::PowerfulTop.MenuBarSection.spacer0":   832,
		"status:PowerfulTop::PowerfulTop.MenuBarSection.toggle":    805,
		"status:com.finetuneapp.FineTune::Item-0":                  512,
		"status:com.tencent.qq::Item-0":                            588,
	}}
	s, _ := newTestStore(p)
	s.SetVisible(true)
	defer s.SetVisible(false)
	waitFor(t, "items", func() bool { return s.Snapshot().HasLayoutAccess && len(s.Snapshot().Items) > 0 })
	s.move("com.finetuneapp.FineTune#0", Hidden, "")
	if p.setCalls != 1 || len(p.drags) != 0 {
		t.Fatalf("file moves don't drag: set %d drags %d", p.setCalls, len(p.drags))
	}
	if p.reloads != 1 {
		t.Errorf("a write the bar ignores reloads the bar once, got %d", p.reloads)
	}
	if got := p.positions()["status:com.finetuneapp.FineTune::Item-0"]; got <= 928 {
		t.Errorf("hidden lands left of the separator: %v", got)
	}
	it, _ := find(s.Snapshot().Items, "com.finetuneapp.FineTune#0")
	if it.Section != Hidden || s.Snapshot().Error != NoError {
		t.Errorf("after move: %+v err %v", it, s.Snapshot().Error)
	}
	s.move("com.tencent.qq#0", Hidden, "")
	if s.Snapshot().Error != ErrNotInMenuBar {
		t.Error("an absent item can't be moved")
	}
}

func TestStoreFallsBackToDrag(t *testing.T) {
	p := &fakeItems{raw: expandedRaw(), isTrusted: true}
	s, _ := newTestStore(p)
	s.SetVisible(true)
	defer s.SetVisible(false)
	waitFor(t, "items", func() bool { return len(s.Snapshot().Items) > 0 })
	s.move("com.finetuneapp.FineTune#0", Hidden, "")
	if len(p.drags) != 1 {
		t.Fatalf("drags: %d", len(p.drags))
	}
	if d := p.drags[0]; d[0] != (Point{1150, -1064.5}) || d[1] != (Point{720, -1064.5}) {
		t.Errorf("drag from the item's centre to just left of the separator: %v", d)
	}
	if s.Snapshot().Error != NoError {
		t.Errorf("landed: %v", s.Snapshot().Error)
	}
}

func TestStoreSkipsReloadWhenTheBarApplies(t *testing.T) {
	p := &fakeItems{raw: expandedRaw(), isTrusted: true, applies: true, pos: map[string]float64{
		"status:PowerfulTop::PowerfulTop.MenuBarSection.separator": 928,
		"status:PowerfulTop::PowerfulTop.MenuBarSection.spacer0":   832,
		"status:PowerfulTop::PowerfulTop.MenuBarSection.toggle":    805,
		"status:com.finetuneapp.FineTune::Item-0":                  512,
	}}
	s, _ := newTestStore(p)
	s.SetVisible(true)
	defer s.SetVisible(false)
	waitFor(t, "items", func() bool { return s.Snapshot().HasLayoutAccess && len(s.Snapshot().Items) > 0 })
	s.move("com.finetuneapp.FineTune#0", Hidden, "")
	if p.reloads != 0 || s.Snapshot().Error != NoError {
		t.Errorf("reloads %d err %v", p.reloads, s.Snapshot().Error)
	}
}
