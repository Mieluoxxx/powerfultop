//go:build darwin

package macos

import (
	"bytes"
	"testing"

	"github.com/ebitengine/purego/objc"
)

// Foundation objects show the marshalling works without touching AppKit's menu bar.
func TestObjCMarshalling(t *testing.T) {
	WithPool(func() {
		var withDouble func(objc.ID, objc.SEL, float64) objc.ID
		var withSize func(objc.ID, objc.SEL, struct{ W, H float64 }) objc.ID
		if err := BindMsgSend(&withDouble, &withSize); err != nil {
			t.Fatal(err)
		}
		n := withDouble(Class("NSNumber"), Sel("numberWithDouble:"), 12.5)
		if got := objc.Send[float64](n, Sel("doubleValue")); got != 12.5 {
			t.Errorf("float argument: %v", got)
		}
		size := struct{ W, H float64 }{8, 16}
		v := withSize(Class("NSValue"), Sel("valueWithSize:"), size)
		if got := objc.Send[struct{ W, H float64 }](v, Sel("sizeValue")); got != size {
			t.Errorf("struct round trip: %v", got)
		}
		if s := GoString(NSString("菜单栏 ⌘")); s != "菜单栏 ⌘" {
			t.Errorf("string round trip: %q", s)
		}
		raw := []byte{1, 2, 3, 0, 255}
		if got := DataBytes(NSData(raw)); !bytes.Equal(got, raw) {
			t.Errorf("data round trip: %v", got)
		}
	})
}

func TestAppRunning(t *testing.T) {
	if !AppRunning("com.apple.dock") {
		t.Error("the Dock should be running")
	}
	if AppRunning("io.invalid.not-installed") {
		t.Error("an app that doesn't exist runs")
	}
}
