// Package barmanager hides menu bar icons Bartender-style on the macOS 27 menu bar and arranges
// every app's items for the layout editor.
package barmanager

import "math"

// Collapse geometry adapted from MenubarHide
// (https://github.com/junior-rj/menubar-hide, MIT License, © Sparrow Serviços e Soluções em Informática Ltda).
//
// macOS 27 draws every status item into one menu bar window and drops an item whose length
// reaches a per-display cliff instead of clamping it, so the classic 10 000 pt separator simply
// vanishes. Below the cliff the bar overflows from the left until the rest fits, so separator
// plus spacers must span the widest status area while each one stays under the narrowest cliff
// (one length applies to every display's copy of the bar). Cliffs measured on 27.0: half the
// width without a notch; on a notched display about 0.79 of the area right of the notch.

// Display is one screen as the collapse geometry sees it.
type Display struct {
	Width float64
	// StatusWidth is the room for status items: right of the notch when there is one.
	StatusWidth float64
}

// NewDisplay fills StatusWidth with the full width when the display has no notch (statusWidth 0).
func NewDisplay(width, statusWidth float64) Display {
	if statusWidth <= 0 {
		statusWidth = width
	}
	return Display{Width: width, StatusWidth: statusWidth}
}

const (
	// SpacerCount is fixed so every launch registers the same autosave names: a name first seen
	// on a later launch lands leftmost, outside the separator block.
	SpacerCount = 6
	MinimumUnit = 200.0
	// CliffMargin covers the measured cliffs not being exact.
	CliffMargin = 64.0
	// NotchedCliffFactor stays under the ~0.79 measured on notched displays.
	NotchedCliffFactor = 0.75
)

// Cliff is the length at which macOS 27 drops a status item on this display.
func Cliff(d Display) float64 {
	if d.StatusWidth < d.Width {
		return d.StatusWidth * NotchedCliffFactor
	}
	return d.Width / 2
}

// UnitLength is the length of the separator and each active spacer.
func UnitLength(displays []Display) float64 {
	if len(displays) == 0 {
		return MinimumUnit
	}
	lowest := math.Inf(1)
	for _, d := range displays {
		lowest = math.Min(lowest, Cliff(d))
	}
	return math.Max(MinimumUnit, math.Floor(lowest-CliffMargin))
}

// ActiveSpacers is the fewest spacers that cover the widest status area: extra ones would
// overflow themselves and show up as blank rows in the « menu.
func ActiveSpacers(unit float64, displays []Display) int {
	if unit <= 0 || len(displays) == 0 {
		return 0
	}
	widest := 0.0
	for _, d := range displays {
		widest = math.Max(widest, d.StatusWidth)
	}
	needed := int(math.Ceil(widest/unit)) - 1
	return min(max(needed, 0), SpacerCount)
}
