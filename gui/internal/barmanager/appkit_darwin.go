//go:build darwin

package barmanager

import (
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"

	"powerfultop/internal/macos"
)

// NewManager reads the saved settings; call Start once the app is ready.
func NewManager() *Manager {
	b := &appKitBar{}
	m := newManager(b, macos.AppPreferences(), mygo.RunOnMain, majorVersion() >= 27)
	b.manager = m
	return m
}

func majorVersion() int {
	version, err := syscall.Sysctl("kern.osproductversion")
	if err != nil {
		return 0
	}
	major, _ := strconv.Atoi(strings.SplitN(version, ".", 2)[0])
	return major
}

// tr picks the text for the language the app shows; the bar can't use view.L, which imports this package.
func tr(en, zh string) string {
	if strings.HasPrefix(i18n.Locale(), "zh") {
		return zh
	}
	return en
}

const (
	variableLength   = -1.0
	rightMouseUpType = 4
	// NSEventMaskLeftMouseUp | NSEventMaskRightMouseUp
	clickMask = uint64(1<<2 | 1<<4)
)

// appKitBar owns the toggle, separator and spacers as NSStatusItems, all touched on the main thread.
type appKitBar struct {
	manager   *Manager
	target    objc.ID
	toggle    objc.ID
	separator objc.ID
	spacers   []objc.ID
	images    map[string]objc.ID
}

var (
	targetOnce  sync.Once
	targetClass objc.Class
	// The one installed bar; the target's methods find it here, as Go values can't ride on ObjC objects.
	activeBar *appKitBar
)

func registerTarget() {
	targetOnce.Do(func() {
		var err error
		targetClass, err = objc.RegisterClass("PowerfulTopBarTarget", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{
			{Cmd: sel("toggleClicked:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) {
				if b := activeBar; b != nil {
					b.clicked()
				}
			}},
			{Cmd: sel("menuToggle:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) {
				if b := activeBar; b != nil {
					b.manager.Toggle()
				}
			}},
			{Cmd: sel("menuSettings:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) {
				if b := activeBar; b != nil && b.manager.OpenSettings != nil {
					b.manager.OpenSettings()
				}
			}},
			{Cmd: sel("menuTurnOff:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) {
				if b := activeBar; b != nil {
					// The toggle's menu is still being tracked here; removing the item under it would
					// free what performClick: is using, so turn off once the click has returned.
					m := b.manager
					go m.onMain(func() { m.SetEnabled(false) })
				}
			}},
			{Cmd: sel("screensChanged:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) {
				if b := activeBar; b != nil {
					b.manager.ScreensChanged()
				}
			}},
		})
		if err != nil {
			panic(err)
		}
	})
}

func statusBar() objc.ID { return send(class("NSStatusBar"), "systemStatusBar") }

func (b *appKitBar) newItem(length float64, name string) objc.ID {
	item := msgFloat(statusBar(), sel("statusItemWithLength:"), length)
	// The bar doesn't keep the item alive; releasing it would take it off the menu bar.
	send(item, "retain")
	send(item, "setAutosaveName:", nsString(name))
	return item
}

func (b *appKitBar) install() {
	registerTarget()
	withPool(func() {
		if b.target == 0 {
			b.target = send(objc.ID(targetClass), "new")
		}
		// macOS 27 keeps positions per autosave name and a name it has never seen lands leftmost,
		// so creating right to left puts the spacers between separator and toggle on first use.
		b.toggle = b.newItem(variableLength, ToggleIdentifier)
		b.spacers = nil
		for i := range SpacerCount {
			spacer := b.newItem(0, AutosavePrefix+"spacer"+strconv.Itoa(i))
			button := send(spacer, "button")
			msgBool(button, sel("setAccessibilityElement:"), false)
			send(button, "setAccessibilityIdentifier:", nsString(SpacerIdentifier))
			// Never hidden: on 27.0.1 an item made visible again re-enters leftmost, left of the
			// icons it should push out. Zero length keeps the slot without taking room.
			msgBool(spacer, sel("setVisible:"), true)
			b.spacers = append(b.spacers, spacer)
		}
		b.separator = b.newItem(variableLength, SeparatorIdentifier)
		separatorButton := send(b.separator, "button")
		send(separatorButton, "setAccessibilityLabel:", nsString(tr("Menu bar separator", "菜单栏分隔符")))
		send(separatorButton, "setAccessibilityIdentifier:", nsString(SeparatorIdentifier))

		toggleButton := send(b.toggle, "button")
		send(toggleButton, "setTarget:", b.target)
		send(toggleButton, "setAction:", sel("toggleClicked:"))
		send(toggleButton, "sendActionOn:", clickMask)
		send(toggleButton, "setAccessibilityIdentifier:", nsString(ToggleIdentifier))

		send(send(class("NSNotificationCenter"), "defaultCenter"), "addObserver:selector:name:object:",
			b.target, sel("screensChanged:"), nsString("NSApplicationDidChangeScreenParametersNotification"), objc.ID(0))
	})
	activeBar = b
}

func (b *appKitBar) uninstall() {
	withPool(func() {
		send(send(class("NSNotificationCenter"), "defaultCenter"), "removeObserver:", b.target)
		items := append([]objc.ID{b.toggle, b.separator}, b.spacers...)
		for _, item := range items {
			if item == 0 {
				continue
			}
			send(statusBar(), "removeStatusItem:", item)
			send(item, "release")
		}
	})
	b.toggle, b.separator, b.spacers = 0, 0, nil
	if activeBar == b {
		activeBar = nil
	}
}

func (b *appKitBar) apply(collapsed bool, unit float64, active int) {
	if b.separator == 0 {
		return
	}
	withPool(func() {
		if collapsed {
			// The span stays on screen while the bar overflows, so the glyph would float mid-bar.
			send(send(b.separator, "button"), "setImage:", objc.ID(0))
			msgFloat(b.separator, sel("setLength:"), unit)
			for i, spacer := range b.spacers {
				length := 0.0
				if i < active {
					length = unit
				}
				msgFloat(spacer, sel("setLength:"), length)
			}
			return
		}
		for _, spacer := range b.spacers {
			msgFloat(spacer, sel("setLength:"), 0)
		}
		msgFloat(b.separator, sel("setLength:"), variableLength)
		send(send(b.separator, "button"), "setImage:", b.image("separator", separatorPNG, separatorSize))
	})
}

func (b *appKitBar) setToggle(collapsed bool) {
	if b.toggle == 0 {
		return
	}
	withPool(func() {
		button := send(b.toggle, "button")
		if collapsed {
			send(button, "setImage:", b.image("chevron.left", func() []byte { return chevronPNG(true) }, chevronSize))
			// The chevron is the only cue, so VoiceOver needs the state spelled out.
			send(button, "setAccessibilityLabel:", nsString(tr("Show Hidden Items", "显示隐藏的图标")))
		} else {
			send(button, "setImage:", b.image("chevron.right", func() []byte { return chevronPNG(false) }, chevronSize))
			send(button, "setAccessibilityLabel:", nsString(tr("Hide Items", "隐藏图标")))
		}
	})
}

// image keeps one template NSImage per glyph for the app's lifetime.
func (b *appKitBar) image(name string, draw func() []byte, size nsSize) objc.ID {
	if img, ok := b.images[name]; ok {
		return img
	}
	img := send(send(class("NSImage"), "alloc"), "initWithData:", nsData(draw()))
	msgSize(img, sel("setSize:"), size)
	msgBool(img, sel("setTemplate:"), true)
	if b.images == nil {
		b.images = map[string]objc.ID{}
	}
	b.images[name] = img
	return img
}

func (b *appKitBar) clicked() {
	event := send(send(class("NSApplication"), "sharedApplication"), "currentEvent")
	if event != 0 && objc.Send[uint](event, sel("type")) == rightMouseUpType {
		b.showMenu()
		return
	}
	b.manager.Toggle()
}

func (b *appKitBar) showMenu() {
	withPool(func() {
		menu := send(send(class("NSMenu"), "alloc"), "init")
		add := func(title, action string) {
			item := send(send(class("NSMenuItem"), "alloc"), "initWithTitle:action:keyEquivalent:", nsString(title), sel(action), nsString(""))
			send(item, "setTarget:", b.target)
			send(menu, "addItem:", item)
			send(item, "release")
		}
		if b.manager.Snapshot().Collapsed {
			add(tr("Show Hidden Items", "显示隐藏的图标"), "menuToggle:")
		} else {
			add(tr("Hide Items", "隐藏图标"), "menuToggle:")
		}
		send(menu, "addItem:", send(class("NSMenuItem"), "separatorItem"))
		add(tr("Menu Bar Manager Settings…", "菜单栏管理设置…"), "menuSettings:")
		add(tr("Turn Off Menu Bar Manager", "关闭菜单栏管理"), "menuTurnOff:")
		// Assigning the menu only for this click keeps left clicks toggling.
		toggle := send(b.toggle, "retain")
		send(toggle, "setMenu:", menu)
		send(send(toggle, "button"), "performClick:", objc.ID(0))
		send(toggle, "setMenu:", objc.ID(0))
		send(toggle, "release")
		send(menu, "release")
	})
}

func screens() []objc.ID {
	list := send(class("NSScreen"), "screens")
	n := objc.Send[uint](list, sel("count"))
	out := make([]objc.ID, 0, n)
	for i := range n {
		out = append(out, send(list, "objectAtIndex:", i))
	}
	return out
}

func (b *appKitBar) displays() []Display {
	var displays []Display
	withPool(func() {
		for _, screen := range screens() {
			frame := objc.Send[nsRect](screen, sel("frame"))
			// Zero on displays without a notch.
			right := objc.Send[nsRect](screen, sel("auxiliaryTopRightArea"))
			displays = append(displays, NewDisplay(frame.Size.W, right.Size.W))
		}
	})
	return displays
}

func (b *appKitBar) menuBarInUse() bool {
	inBar := false
	withPool(func() {
		mouse := objc.Send[nsPoint](class("NSEvent"), sel("mouseLocation"))
		thickness := objc.Send[float64](statusBar(), sel("thickness"))
		for _, screen := range screens() {
			frame := objc.Send[nsRect](screen, sel("frame"))
			visible := objc.Send[nsRect](screen, sel("visibleFrame"))
			top := frame.Origin.Y + frame.Size.H
			barHeight := max(top-(visible.Origin.Y+visible.Size.H), thickness)
			inside := mouse.X >= frame.Origin.X && mouse.X < frame.Origin.X+frame.Size.W &&
				mouse.Y >= frame.Origin.Y && mouse.Y <= top
			if inside && mouse.Y >= top-barHeight {
				inBar = true
			}
		}
	})
	return inBar || popUpMenuOpen()
}
