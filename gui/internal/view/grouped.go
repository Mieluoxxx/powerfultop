package view

import (
	"github.com/ELY-Open-Source/CrychicUI/kit"
	"github.com/ELY-Open-Source/CrychicUI/layout"
	"github.com/egoist/mygo/ui"
)

// Section is a System Settings–style group: a header above an inset box of rows and a footer
// under it, with hairlines between rows.
func Section(c *ui.Context, title, footer string, rows ...func()) {
	t := c.Theme()
	ui.Column(c).Gap(t.Space(2)).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(t.Rem(0.85)).FontWeight(600).PaddingX(t.Space(2))
		}
		box := kit.Card(c).Clip()
		box.Children(func() {
			for i, row := range rows {
				if i > 0 {
					layout.Divider(c, layout.DividerOptions{})
				}
				ui.Column(c.Key(i)).MinHeight(t.Space(10)).Justify(ui.Center).
					Padding(t.Space(2), t.Space(3)).Children(row)
			}
		})
		if footer != "" {
			kit.Muted(c, footer).FontSize(t.Rem(0.75)).PaddingX(t.Space(2))
		}
	})
}

// SectionLabel is a row's leading title; it takes the room the trailing control leaves.
func SectionLabel(c *ui.Context, title string) ui.Element {
	return ui.Text(c, title).FontSize(c.Theme().Rem(0.85)).Grow(1).Shrink(1)
}

// SectionValue is a row's trailing read-only value.
func SectionValue(c *ui.Context, value string) ui.Element {
	return kit.Numeric(c, value).FontSize(c.Theme().Rem(0.85)).TextColor(c.Theme().TextMuted).SingleLine()
}
