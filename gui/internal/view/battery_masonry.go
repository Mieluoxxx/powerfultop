package view

import (
	"github.com/ELY-Open-Source/CrychicUI/layout"
	"github.com/egoist/mygo/ui"
)

// MasonryItem is one card for Masonry; Weight estimates its height so columns stay balanced.
type MasonryItem struct {
	ID     string
	Weight float64
	View   func()
}

// Masonry lays the cards out in the fewest empty columns, as the library's masonry does; Weight is
// kept for callers that estimate a card's height, and orders the cards as given.
func Masonry(c *ui.Context, items []MasonryItem, columns int, spacing float32) {
	views := make([]func(), len(items))
	for i, item := range items {
		views[i] = func() {
			ui.Column(c.Key(item.ID)).Children(item.View)
		}
	}
	_ = spacing
	layout.Masonry(c, max(columns, 1), views)
}
