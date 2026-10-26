package barmanager

import (
	"math"
	"strings"
)

// Section is where macOS 27 puts an item right now.
type Section int

const (
	// Shown items are drawn in the menu bar.
	Shown Section = iota
	// Overflow items didn't fit, so macOS tucked them behind its « button.
	Overflow
	// Absent items aren't drawn in any menu bar, typically switched off in System Settings › Menu Bar.
	Absent
)

// Rect is an accessibility frame: global points with a top-left origin.
type Rect struct{ X, Y, W, H float64 }

func (r Rect) MidX() float64 { return r.X + r.W/2 }
func (r Rect) MidY() float64 { return r.Y + r.H/2 }
func (r Rect) MaxX() float64 { return r.X + r.W }

// Point is a global point with a top-left origin.
type Point struct{ X, Y float64 }

// DropPointLeftOf lands an item immediately left of the item at frame.
func DropPointLeftOf(frame Rect) Point {
	return Point{frame.X - 8, frame.MidY()}
}

// DropPointRightOf lands an item immediately right of the item at frame.
func DropPointRightOf(frame Rect) Point {
	return Point{frame.MaxX() + 8, frame.MidY()}
}

// ParseKey splits a macOS 27 layout key, "status:<owner>::<autosave name>", where the owner is
// the bundle identifier or, for some apps, the process name.
func ParseKey(key string) (owner, name string, ok bool) {
	rest, found := strings.CutPrefix(key, "status:")
	if !found {
		return "", "", false
	}
	owner, name, found = strings.Cut(rest, "::")
	if !found || owner == "" {
		return "", "", false
	}
	return owner, name, true
}

// PositionLeftOf is immediately left of anchor, halfway to the next item so nothing else is crossed.
func PositionLeftOf(anchor float64, existing []float64) float64 {
	next, found := math.Inf(1), false
	for _, v := range existing {
		if v > anchor && v < next {
			next, found = v, true
		}
	}
	if !found {
		return anchor + 16
	}
	return (anchor + next) / 2
}

// PositionRightOf is immediately right of anchor; the clock sits at 0, so it never goes below that.
func PositionRightOf(anchor float64, existing []float64) float64 {
	next := 0.0
	for _, v := range existing {
		if v < anchor && v > next {
			next = v
		}
	}
	return (anchor + next) / 2
}
