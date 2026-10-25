//go:build darwin

package barmanager

import (
	"bytes"
	"testing"

	"github.com/ebitengine/purego/objc"
)

// The status item code passes floats, bools and NSSize through these helpers; Foundation objects
// show the marshalling works without touching AppKit's menu bar.
func TestObjCMarshalling(t *testing.T) {
	withPool(func() {
		n := msgFloat(class("NSNumber"), sel("numberWithDouble:"), 12.5)
		if got := objc.Send[float64](n, sel("doubleValue")); got != 12.5 {
			t.Errorf("float argument: %v", got)
		}
		v := objc.ID(0)
		{
			// valueWithSize: takes a struct, so it needs the typed variant.
			var fn func(objc.ID, objc.SEL, nsSize) objc.ID
			registerMsg(&fn)
			v = fn(class("NSValue"), sel("valueWithSize:"), nsSize{8, 16})
		}
		if got := objc.Send[nsSize](v, sel("sizeValue")); got != (nsSize{8, 16}) {
			t.Errorf("struct round trip: %v", got)
		}
		if s := goString(nsString("菜单栏 ⌘")); s != "菜单栏 ⌘" {
			t.Errorf("string round trip: %q", s)
		}
		raw := []byte{1, 2, 3, 0, 255}
		if got := dataBytes(nsData(raw)); !bytes.Equal(got, raw) {
			t.Errorf("data round trip: %v", got)
		}
	})
}
