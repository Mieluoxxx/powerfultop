package view

import (
	"time"

	lucide "github.com/ELY-Open-Source/CrychicUI/icon"
	"github.com/ELY-Open-Source/CrychicUI/kit"
	"github.com/ELY-Open-Source/CrychicUI/layout"
	"github.com/ELY-Open-Source/CrychicUI/navigation"
	"github.com/egoist/mygo/ui"
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
	sidebarWidth        = 224
	DetailsWidth        = 1040
	DetailsHeight       = 720
	DetailsMinWidth     = 820
	DetailsMinHeight    = 480
	TwoColumnMinWidth   = 760
	defaultContentWidth = DetailsWidth - sidebarWidth - 48
)

// Details is the details window: the sidebar of pages and the selected page under its title.
func Details(c *ui.Context, pages []WindowPage, s *DetailsState) {
	t := c.Theme()
	current, index := pages[0], 0
	for i, p := range pages {
		if p.ID == s.Selected {
			current, index = p, i
		}
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(t.Background).Children(func() {
		sidebar(c, pages, index, s)
		ui.Box(c).Width(1).Background(t.Border)
		if !s.Available {
			ui.Column(c).Grow(1).Center().Gap(t.Space(3)).Children(func() {
				ui.Icon(c, lucide.Must("triangle-alert")).Size(32, 32).TextColor(Orange.Color(c))
				ui.Text(c, L("Built-in battery not detected. PowerfulTop requires a MacBook.", "未检测到内置电池。PowerfulTop 仅支持 MacBook。")).
					FontSize(t.Rem(0.9)).TextColor(t.TextMuted).TextAlign(ui.Center)
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
			ui.Column(c).Padding(24).Gap(t.Space(4)).Children(func() {
				layout.PageHeader(c, current.Title, layout.PageHeaderOptions{})
				current.Body(c, width)
			})
		})
	})
}

// sidebar is the library's navigation, with the readings' age under it.
func sidebar(c *ui.Context, pages []WindowPage, selected int, s *DetailsState) {
	t := c.Theme()
	active := selected
	ui.Column(c).Width(sidebarWidth).PaddingY(t.Space(3)).Background(t.Text.Alpha(0.02)).Children(func() {
		items := make([]navigation.NavItem, len(pages))
		for i, p := range pages {
			items[i] = navigation.NavItem{Label: p.Title, Icon: p.Icon}
		}
		navigation.SideNav(c, L("Pages", "页面"), items, &active)
		if active != selected && active >= 0 && active < len(pages) {
			s.Selected = pages[active].ID
			if s.OnSelect != nil {
				s.OnSelect(pages[active].ID)
			}
		}
		ui.Spacer(c)
		if !s.UpdatedAt.IsZero() {
			kit.Muted(c, Lf("Updated at %s", "更新于 %s", s.UpdatedAt.Format("15:04:05"))).
				FontSize(t.Rem(0.7)).TextColor(t.TextMuted.Alpha(0.75)).PaddingX(t.Space(3))
		}
	})
}
