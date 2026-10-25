//go:build darwin

package display

import (
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
)

// Only reads: it lists the displays and waits for the DDC probe, never changing a level.
func TestControllerLive(t *testing.T) {
	if len(onlineDisplayList()) == 0 {
		t.Skip("no displays")
	}
	// NSScreen needs the WindowServer connection an app makes.
	bind()
	objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
	d := NewController()
	d.refresh()
	displays := d.Snapshot()
	if len(displays) == 0 {
		t.Fatal("no displays listed")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		probing := len(d.probing)
		d.mu.Unlock()
		if probing == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, b := range d.Snapshot() {
		t.Logf("%d %q builtIn=%v method=%d level=%.2f", b.ID, b.Name, b.BuiltIn, b.Method, b.Level)
		if b.Level < 0 || b.Level > 1 {
			t.Errorf("level out of range: %+v", b)
		}
		if b.Name == "" {
			t.Errorf("no name for display %d", b.ID)
		}
		if b.BuiltIn && b.Method != Native {
			t.Errorf("built-in display should use DisplayServices: %+v", b)
		}
	}
	d.mu.Lock()
	for _, link := range d.links {
		link.release()
	}
	d.mu.Unlock()
}
