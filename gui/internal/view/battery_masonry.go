package view

import "github.com/egoist/mygo/ui"

// MasonryItem is one card for Masonry; Weight estimates its height so columns stay balanced.
type MasonryItem struct {
	ID     string
	Weight float64
	View   func()
}

// Masonry fills each card into the currently shortest column, keeping source order within a column.
func Masonry(c *ui.Context, items []MasonryItem, columns int, spacing float32) {
	columns = max(columns, 1)
	stacks := make([][]MasonryItem, columns)
	heights := make([]float64, columns)
	for _, item := range items {
		target := 0
		for i := range heights {
			if heights[i] < heights[target] {
				target = i
			}
		}
		stacks[target] = append(stacks[target], item)
		heights[target] += item.Weight
	}
	ui.Grid(c).Columns(columns).Gap(spacing).AlignItems(ui.Start).Children(func() {
		for i, stack := range stacks {
			ui.Column(c.Key(i)).Gap(spacing).Children(func() {
				for _, item := range stack {
					ui.Column(c.Key(item.ID)).Children(item.View)
				}
			})
		}
	})
}
