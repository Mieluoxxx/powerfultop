package barmanager

// SystemItem is one of Apple's own menu bar items with a switch under System Settings › Menu Bar.
// Most are drawn by MenuBarAgent, which names each by its menu extra's identifier; the rest
// belong to processes of their own and go by those processes' bundle identifiers.
type SystemItem struct {
	ID     string
	Name   string
	NameZh string
	// Icon is a lucide icon standing in for the item, which has no app logo.
	Icon string
	// Key is its checkbox in Control Center's current-host preferences, and names its position.
	Key string
	// Flag is the switch of an item outside Control Center's checkboxes; its own process, which
	// has the item's ID as bundle identifier, reads it as it starts.
	Flag *flagSwitch
	// LoginItem is set for Weather, whose item belongs to a login item launchd keeps running.
	LoginItem *loginItem
}

// PositionKey is the item's entry in the layout file, or "" when MenuBarAgent doesn't place it.
func (s SystemItem) PositionKey() string {
	if s.Key == "" {
		return ""
	}
	return "module:" + s.Key
}

type flagSwitch struct {
	domain      string
	key         string
	currentHost bool
	// hides is the value that switches the item off.
	hides bool
}

type loginItem struct {
	label string
	app   string
}

const (
	// controlCenterDomain is also the bundle identifier of the process that reads the checkboxes.
	controlCenterDomain = "com.apple.controlcenter"
	// A checkbox key of 8 hides its item; 2 shows it (or shows it when active) and 18 always does.
	controlCenterHidden = 8
	controlCenterShown  = 2
	textInputID         = "com.apple.TextInputMenuAgent"
)

func menuExtra(name, en, zh, icon, key string) SystemItem {
	return SystemItem{ID: "com.apple.menuextra." + name, Name: en, NameZh: zh, Icon: icon, Key: key}
}

// SystemItems are those PowerfulTop can put in the tray. The clock, Control Center and the
// camera and microphone indicator have no switch and stay. Spotlight's switch only takes when
// the Siri app that draws its item restarts, which is too much to do behind the user's back.
var SystemItems = []SystemItem{
	menuExtra("wifi", "Wi-Fi", "Wi-Fi", "wifi", "WiFi"),
	menuExtra("bluetooth", "Bluetooth", "蓝牙", "bluetooth", "Bluetooth"),
	menuExtra("airdrop", "AirDrop", "隔空投送", "radio", "AirDrop"),
	menuExtra("focusmode", "Focus", "专注模式", "moon", "FocusModes"),
	menuExtra("screen-mirroring", "Screen Mirroring", "屏幕镜像", "cast", "ScreenMirroring"),
	menuExtra("display", "Display", "显示器", "sun", "Display"),
	menuExtra("sound", "Sound", "声音", "volume-2", "Sound"),
	menuExtra("now-playing", "Now Playing", "正在播放", "play", "NowPlaying"),
	menuExtra("user", "Fast User Switching", "快速用户切换", "user", "UserSwitcher"),
	menuExtra("timer", "Timer", "计时器", "timer", "Timer"),
	menuExtra("battery", "Battery", "电池", "battery-full", "Battery"),
	{ID: textInputID, Name: "Text Input", NameZh: "输入法", Icon: "keyboard",
		Flag: &flagSwitch{domain: "com.apple.TextInputMenu", key: "visible", hides: false}},
	// Its key alone does nothing: System Settings also unregisters or registers the login item.
	{ID: "com.apple.weather.menu", Name: "Weather", NameZh: "天气", Icon: "cloud-sun", Key: "Weather",
		LoginItem: &loginItem{label: "com.apple.weather.menu", app: "/System/Applications/Weather.app/Contents/Library/LoginItems/WeatherMenu.app"}},
}

func systemItem(id string) (SystemItem, bool) {
	for _, s := range SystemItems {
		if s.ID == id {
			return s, true
		}
	}
	return SystemItem{}, false
}

func systemItemByPositionKey(key string) (SystemItem, bool) {
	for _, s := range SystemItems {
		if key != "" && s.PositionKey() == key {
			return s, true
		}
	}
	return SystemItem{}, false
}

// checkboxWrite is what to do to a checkbox key: leave it, set it to value, or remove it.
type checkboxWrite struct {
	keep, remove bool
	value        int
}

// missingValue stands for a key that was missing, among the values kept to put back.
const missingValue = -1

// switchingCheckbox decides how to hide (shown false) or show an item whose key holds current
// (has false when missing), with saved what hiding replaced, if PowerfulTop hid it. Showing puts
// the user's own choice back (shown when active, always, or the default), not one of ours.
func switchingCheckbox(shown bool, current int, has bool, saved int, hasSaved bool) (w checkboxWrite, keep int, keepSaved bool) {
	if !shown {
		if has && current == controlCenterHidden {
			return checkboxWrite{keep: true}, saved, hasSaved
		}
		if !has {
			current = missingValue
		}
		return checkboxWrite{value: controlCenterHidden}, current, true
	}
	if !has || current != controlCenterHidden {
		// Shown already, by the user from System Settings meanwhile.
		return checkboxWrite{keep: true}, 0, false
	}
	switch {
	case !hasSaved:
		return checkboxWrite{value: controlCenterShown}, 0, false
	case saved == missingValue:
		return checkboxWrite{remove: true}, 0, false
	}
	return checkboxWrite{value: saved}, 0, false
}
