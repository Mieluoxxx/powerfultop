//go:build darwin

package power

import (
	"errors"
	"testing"
)

// Reads this Mac's battery through purego, so a broken binding fails here rather than in the app.
func TestReadLive(t *testing.T) {
	s, err := Read()
	if errors.Is(err, ErrNoBattery) {
		t.Skip("no built-in battery")
	}
	if err != nil {
		t.Fatal(err)
	}
	if s.Percent <= 0 || s.Percent > 100 || s.SystemW <= 0 {
		t.Errorf("implausible reading: %+v", s)
	}
	t.Logf("%+v", s)
}
