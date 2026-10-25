package barmanager

import (
	"math"
	"strings"
)

// Section is where an item sits relative to the separator.
type Section int

const (
	Shown Section = iota
	Hidden
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

// SectionOf sorts an expanded item around the separator.
func SectionOf(frame, separator, toggle Rect) Section {
	// macOS 27 parks items it isn't drawing at a fixed spot off the bar's row, e.g. (-1, 970).
	if math.Abs(frame.MidY()-toggle.MidY()) > toggle.H/2 {
		return Absent
	}
	// Neighbouring frames overlap by a point or two, so compare centres rather than edges.
	if frame.MidX() < separator.MidX() {
		return Hidden
	}
	return Shown
}

// DropPoint is where a ⌘-drag has to end for the item to land in section. A lifted item lets the
// items left of it slide right, so both targets stay valid whichever side it starts on.
func DropPoint(section Section, separator, toggle Rect) (Point, bool) {
	switch section {
	case Shown:
		return Point{toggle.MaxX() + 4, toggle.MidY()}, true
	case Hidden:
		return DropPointLeftOf(separator), true
	}
	return Point{}, false
}

// DropPointLeftOf lands an item immediately left of the item at frame.
func DropPointLeftOf(frame Rect) Point {
	return Point{frame.X - 8, frame.MidY()}
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

// SectionOfPosition: preferred positions count from the right edge, so a larger value sits
// further left, and everything left of the rightmost block item (separator or spacer) is pushed
// out when collapsed.
func SectionOfPosition(position, blockEdge float64) Section {
	if position > blockEdge {
		return Hidden
	}
	return Shown
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
