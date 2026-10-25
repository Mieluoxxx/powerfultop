package view

import (
	"time"

	"github.com/egoist/mygo/ui"

	"powerfultop/internal/icons"
)

// PageID values match the Swift app's Page raw values, which "dashboardPage" stores.
type PageID string

const (
	PageOverview       PageID = "overview"
	PageSystem         PageID = "system"
	PageCooling        PageID = "cooling"
	PageBattery        PageID = "battery"
	PageMenuBar        PageID = "menuBar"
	PageMenuBarManager PageID = "menuBarManager"
	PageSettings       PageID = "settings"
)

// WindowPage is one entry of the details window's sidebar.
type WindowPage struct {
	ID    PageID
	Title string
	Icon  string
	// Attention flags the entry with a warning, with Hint as its tooltip.
	Attention bool
	Hint      string
	// Body draws the page at the content width it gets.
	Body func(c *ui.Context, width float32)
}

// DetailsState is the details window's own state.
type DetailsState struct {
	Selected PageID
	// Available is false on Macs without a built-in battery, where only a notice shows.
	Available bool
	UpdatedAt time.Time
	// OnSelect persists the choice; nil leaves it in memory.
	OnSelect func(PageID)

	contentWidth float32
}

const (
	sidebarWidth        = 190
	DetailsWidth        = 1000
	DetailsHeight       = 720
	DetailsMinWidth     = 760
	DetailsMinHeight    = 480
	TwoColumnMinWidth   = 760
	defaultContentWidth = DetailsWidth - sidebarWidth - 48
)

// Details is the details window: a sidebar of pages and the selected page under its title.
func Details(c *ui.Context, pages []WindowPage, s *DetailsState) {
	t := c.Theme()
	current := pages[0]
	for _, p := range pages {
		if p.ID == s.Selected {
			current = p
		}
	}
	ui.Row(c).Fill().Background(t.Background).Children(func() {
		sidebar(c, pages, current.ID, s)
		ui.Box(c).Width(1).Background(CardBorder(c))
		if !s.Available {
			ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
				ui.Icon(c, icons.Get("triangle-alert")).Size(32, 32).TextColor(Orange.Color(c))
				ui.Text(c, L("Built-in battery not detected. PowerfulTop requires a MacBook.", "未检测到内置电池。PowerfulTop 仅支持 MacBook。")).
					FontSize(13).TextColor(t.TextMuted).TextAlign(ui.Center)
			})
			return
		}
		scroll := ui.Scroll(c).Grow(1).Key(current.ID)
		if b := scroll.Bounds(); b.W > 0 {
			s.contentWidth = b.W - 48
		}
		width := s.contentWidth
		if width <= 0 {
			width = defaultContentWidth
		}
		scroll.Children(func() {
			ui.Column(c).Padding(24).Gap(16).Children(func() {
				ui.Text(c, current.Title).FontSize(22).Bold()
				current.Body(c, width)
			})
		})
	})
}

func sidebar(c *ui.Context, pages []WindowPage, selected PageID, s *DetailsState) {
	t := c.Theme()
	ui.Column(c).Width(sidebarWidth).Padding(12).PaddingY(20).Gap(4).Background(t.Text.Alpha(0.025)).Children(func() {
		for _, p := range pages {
			active := p.ID == selected
			color := t.Text
			if active {
				color = t.Accent
			}
			item := ui.Row(c.Key(p.ID)).Gap(8).AlignItems(ui.Center).PaddingX(12).PaddingY(8).Radius(9)
			if active {
				item.Background(t.Accent.Alpha(0.14))
			}
			item.OnClick(func() {
				s.Selected = p.ID
				if s.OnSelect != nil {
					s.OnSelect(p.ID)
				}
			})
			item.Children(func() {
				ui.Icon(c, icons.Get(p.Icon)).Size(15, 15).TextColor(color)
				label := ui.Text(c, p.Title).FontSize(13).TextColor(color).Grow(1).NoWrap()
				if active {
					label.Bold()
				}
				if p.Attention {
					ui.Icon(c, icons.Get("triangle-alert")).Size(13, 13).TextColor(Orange.Color(c)).Tooltip(p.Hint)
				}
			})
		}
		ui.Spacer(c)
		if !s.UpdatedAt.IsZero() {
			ui.Text(c, Lf("Updated at %s", "更新于 %s", s.UpdatedAt.Format("15:04:05"))).
				FontSize(10).TextColor(t.TextMuted.Alpha(0.75)).FontFeatures("tnum").PaddingX(12)
		}
	})
}
