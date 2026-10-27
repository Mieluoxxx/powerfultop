//go:build darwin

package barmanager

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// Switches the app TRAY_LIVE names off and on again in the real menu bar, through the same list
// the tray uses, and reads the bar to see each take. It needs the list's file and Accessibility.
func TestTrayLiveAllowList(t *testing.T) {
	id := os.Getenv("TRAY_LIVE")
	if id == "" {
		t.Skip()
	}
	d := &darwinItems{onMain: func(fn func()) { fn() }}
	if !d.trusted() {
		t.Fatal("needs Accessibility")
	}
	list := d.allowList()
	if list == nil {
		t.Fatal("can't read the allow list")
	}
	if _, ok := d.drawn()[id]; !ok {
		t.Fatalf("%s isn't drawn in the menu bar", id)
	}
	waitDrawn := func(want bool) time.Duration {
		start := time.Now()
		for time.Since(start) < 5*time.Second {
			if _, ok := d.drawn()[id]; ok == want {
				return time.Since(start)
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("%s drawn=%v never happened", id, !want)
		return 0
	}
	t.Cleanup(func() {
		if out, changed, _ := settingAllowed(d.allowList(), owners(id), true); changed {
			d.setAllowList(out)
		}
	})

	out, changed, missing := settingAllowed(list, owners(id), false)
	if !changed || len(missing) > 0 {
		t.Fatalf("changed %v missing %v", changed, missing)
	}
	if !d.setAllowList(out) {
		t.Fatal("write failed")
	}
	t.Logf("gone after %v", waitDrawn(false))
	if el := d.extra(id); el == 0 {
		t.Error("the app should still expose its item while switched off")
	} else {
		d.release(el)
	}

	out, _, _ = settingAllowed(d.allowList(), owners(id), true)
	d.setAllowList(out)
	t.Logf("back after %v", waitDrawn(true))
}

// Adds the app TRAY_LIVE_ADD names to a tray kept in memory, through the real platform, checks
// it left the bar, then puts it back.
func TestTrayLiveAdd(t *testing.T) {
	id := os.Getenv("TRAY_LIVE_ADD")
	if id == "" {
		t.Skip()
	}
	d := &darwinItems{onMain: func(fn func()) { fn() }}
	prefs := &livePrefs{values: map[string]any{trayEnabledKey: true}}
	// MyGo draws file icons on its run loop, which a test doesn't run.
	s := newTrayStore(noIcons{d}, prefs, true, "PowerfulTop.overflow")
	t.Cleanup(s.stopReconciling)
	t.Cleanup(s.RestoreAll)
	if !s.CanHold(id) {
		t.Fatalf("CanHold(%s) is false", id)
	}
	s.add(id)
	snap := s.Snapshot()
	t.Logf("error %v %q, entries %+v", snap.Error, snap.ErrorID, snap.Entries)
	if !s.Contains(id) {
		t.Fatal("not added")
	}
	start := time.Now()
	for time.Since(start) < 5*time.Second {
		if _, ok := d.drawn()[id]; !ok {
			t.Logf("gone after %v", time.Since(start))
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("still in the menu bar")
}

type livePrefs struct{ values map[string]any }

func (p *livePrefs) Value(k string) (any, bool) { v, ok := p.values[k]; return v, ok }
func (p *livePrefs) Bool(k string) (bool, bool) { v, ok := p.values[k].(bool); return v, ok }
func (p *livePrefs) Set(k string, v any) error {
	if v == nil {
		delete(p.values, k)
	} else {
		p.values[k] = v
	}
	return nil
}

type noIcons struct{ *darwinItems }

func (noIcons) appIcon(string) []byte { return nil }

// Scans the real menu bar as the editor does and prints each item's section.
func TestLiveScan(t *testing.T) {
	if os.Getenv("SCAN_LIVE") == "" {
		t.Skip()
	}
	s := newItemStore(&darwinItems{onMain: func(fn func()) { fn() }}, true, []string{"PowerfulTop.status", "PowerfulTop.overflow"})
	s.p = noIcons{s.p.(*darwinItems)}
	s.scan()
	for _, it := range s.Snapshot().Items {
		t.Logf("%-8v %-28s %v", map[Section]string{Shown: "shown", Overflow: "«", Absent: "absent"}[it.Section], it.BundleID, it.Frame)
	}
}

// Switches the app WATCH_LIVE names off and watches its switch and the bar for a minute.
func TestLiveWatchSwitch(t *testing.T) {
	id := os.Getenv("WATCH_LIVE")
	if id == "" {
		t.Skip()
	}
	d := &darwinItems{onMain: func(fn func()) { fn() }}
	o := allowOwner{bundleID: id}
	if app, ok := d.runningApp(id); ok {
		o.executable = app.Executable
	}
	t.Cleanup(func() {
		if out, changed, _ := settingAllowed(d.allowList(), []allowOwner{o}, true); changed {
			d.setAllowList(out)
		}
	})
	out, _, _ := settingAllowed(d.allowList(), []allowOwner{o}, false)
	d.setAllowList(out)
	start := time.Now()
	last := ""
	for time.Since(start) < 60*time.Second {
		allowed, listed := allowedState(d.allowList(), o)
		_, drawn := d.drawn()[id]
		now := fmt.Sprintf("allowed=%v listed=%v drawn=%v", allowed, listed, drawn)
		if now != last {
			t.Logf("%5.1fs %s", time.Since(start).Seconds(), now)
			last = now
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// Switches WATCH_RESTART's app off, restarts MenuBarAgent and watches what its switch does.
func TestLiveSwitchAcrossAgentRestart(t *testing.T) {
	id := os.Getenv("WATCH_RESTART")
	if id == "" {
		t.Skip()
	}
	d := &darwinItems{onMain: func(fn func()) { fn() }}
	o := allowOwner{bundleID: id}
	t.Cleanup(func() {
		if out, changed, _ := settingAllowed(d.allowList(), []allowOwner{o}, true); changed {
			d.setAllowList(out)
		}
	})
	out, _, _ := settingAllowed(d.allowList(), []allowOwner{o}, false)
	d.setAllowList(out)
	time.Sleep(2 * time.Second)
	allowed, listed := allowedState(d.allowList(), o)
	t.Logf("before restart: allowed=%v listed=%v", allowed, listed)
	d.reloadMenuBar()
	start := time.Now()
	last := ""
	for time.Since(start) < 15*time.Second {
		allowed, listed := allowedState(d.allowList(), o)
		_, drawn := d.drawn()[id]
		now := fmt.Sprintf("allowed=%v listed=%v drawn=%v", allowed, listed, drawn)
		if now != last {
			t.Logf("%5.1fs after restart %s", time.Since(start).Seconds(), now)
			last = now
		}
		time.Sleep(250 * time.Millisecond)
	}
}
