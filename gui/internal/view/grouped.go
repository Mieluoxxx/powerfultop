package view

import "github.com/egoist/mygo/ui"

// Section is a System Settings–style group: a header above an inset box of rows and a footer
// under it, with hairlines between rows that stop short of the box's edges.
func Section(c *ui.Context, title, footer string, rows ...func()) {
	t := c.Theme()
	ui.Column(c).Gap(6).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(13).FontWeight(600).PaddingX(10)
		}
		Surface(c, ui.Column(c), 10).Children(func() {
			for i, row := range rows {
				if i > 0 {
					// Negative keys keep the hairlines apart from the rows' keys.
					ui.Box(c.Key(-i)).Height(1).MarginX(12).Background(CardBorder(c))
				}
				ui.Row(c.Key(i)).Gap(12).AlignItems(ui.Center).PaddingX(12).PaddingY(8).MinHeight(40).Children(row)
			}
		})
		if footer != "" {
			ui.Text(c, footer).FontSize(11).TextColor(t.TextMuted).PaddingX(10)
		}
	})
}

// SectionLabel is a row's leading title; it takes the room the trailing control leaves.
func SectionLabel(c *ui.Context, title string) ui.Element {
	return ui.Text(c, title).FontSize(13).Grow(1).Shrink(1)
}

// SectionValue is a row's trailing read-only value.
func SectionValue(c *ui.Context, value string) ui.Element {
	return ui.Text(c, value).FontSize(13).TextColor(c.Theme().TextMuted).FontFeatures("tnum").SingleLine()
}
