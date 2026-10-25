// Package menubar models what the menu bar item shows, ported from the Swift app's MenuBarLabel,
// and draws it into the image MyGo's tray shows.
package menubar

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// IconStyle is the main icon of the label. Raw values match the Swift app's, which are persisted.
type IconStyle string

const (
	IconHidden IconStyle = "hidden"
	// IconBattery is a battery in five steps, or a bolt while on AC. Its raw value predates the other styles.
	IconBattery IconStyle = "battery"
	// IconMacOS is an outline filled to the exact level, like the macOS battery item.
	IconMacOS IconStyle = "macOS"
	// IconMacOSColor is the same outline with a green/red fill.
	IconMacOSColor IconStyle = "macOSColor"
	// IconIOS is a solid battery with the percentage printed inside, like iOS.
	IconIOS IconStyle = "iOS"
	// IconRing is a small ring that fills with the level.
	IconRing    IconStyle = "ring"
	IconAppIcon IconStyle = "appIcon"
)

// IconStyles lists the styles in the Swift app's order.
var IconStyles = []IconStyle{IconHidden, IconBattery, IconMacOS, IconMacOSColor, IconIOS, IconRing, IconAppIcon}

// Symbol is the SF Symbol name the Swift app showed for the style's catalog chip.
func (s IconStyle) Symbol() string {
	switch s {
	case IconHidden:
		return "eye.slash"
	case IconBattery:
		return "battery.75percent"
	case IconMacOS, IconMacOSColor, IconIOS:
		return "battery.100"
	case IconRing:
		return "circle.dashed"
	default:
		return "bolt.circle"
	}
}

// Group sorts the catalog of items.
type Group string

const (
	GroupBattery Group = "battery"
	GroupAdapter Group = "adapter"
	GroupHealth  Group = "health"
	GroupSystem  Group = "system"
)

var Groups = []Group{GroupBattery, GroupAdapter, GroupHealth, GroupSystem}

// Item is one reading the label can show. Raw values match the Swift app's, which are persisted.
type Item string

const (
	ItemBatteryPercent     Item = "batteryPercent"
	ItemTimeRemaining      Item = "timeRemaining"
	ItemBatteryTemperature Item = "batteryTemperature"
	ItemBatteryCurrent     Item = "batteryCurrent"
	ItemBatteryVoltage     Item = "batteryVoltage"
	ItemBatteryPower       Item = "batteryPower"
	ItemSystemPower        Item = "systemPower"
	ItemAdapterCurrent     Item = "adapterCurrent"
	ItemAdapterVoltage     Item = "adapterVoltage"
	ItemAdapterPower       Item = "adapterPower"
	ItemHealth             Item = "health"
	ItemCycleCount         Item = "cycleCount"
	ItemCPUUsage           Item = "cpuUsage"
	ItemMemoryUsage        Item = "memoryUsage"
	ItemCPUTemperature     Item = "cpuTemperature"
	ItemFanSpeed           Item = "fanSpeed"
)

// Items lists every item in the Swift app's order.
var Items = []Item{
	ItemBatteryPercent, ItemTimeRemaining, ItemBatteryTemperature, ItemBatteryCurrent, ItemBatteryVoltage,
	ItemBatteryPower, ItemSystemPower, ItemAdapterCurrent, ItemAdapterVoltage, ItemAdapterPower,
	ItemHealth, ItemCycleCount, ItemCPUUsage, ItemMemoryUsage, ItemCPUTemperature, ItemFanSpeed,
}

func (i Item) Group() Group {
	switch i {
	case ItemAdapterCurrent, ItemAdapterVoltage, ItemAdapterPower:
		return GroupAdapter
	case ItemHealth, ItemCycleCount:
		return GroupHealth
	case ItemCPUUsage, ItemMemoryUsage, ItemCPUTemperature, ItemFanSpeed:
		return GroupSystem
	default:
		return GroupBattery
	}
}

// Symbol is the SF Symbol name of the glyph shown before the value, so several numbers in the
// menu bar stay distinguishable; "" for none. Lucide maps it to the icon drawn.
func (i Item) Symbol() string {
	switch i {
	case ItemTimeRemaining:
		return "clock"
	case ItemBatteryTemperature:
		return "thermometer.medium"
	case ItemBatteryCurrent, ItemBatteryVoltage, ItemBatteryPower:
		return "battery.100"
	case ItemSystemPower:
		return "laptopcomputer"
	case ItemAdapterCurrent, ItemAdapterVoltage, ItemAdapterPower:
		return "powerplug"
	case ItemHealth:
		return "heart"
	case ItemCycleCount:
		return "arrow.triangle.2.circlepath"
	case ItemCPUUsage:
		return "cpu"
	case ItemMemoryUsage:
		return "memorychip"
	case ItemCPUTemperature:
		return "thermometer.variable"
	case ItemFanSpeed:
		return "fan"
	}
	return ""
}

// lucide maps the SF Symbol names the model keeps (so labels compare as the Swift app's did) to
// the Lucide icons MyGo draws.
var lucide = map[string]string{
	"clock":                       "clock",
	"thermometer.medium":          "thermometer",
	"thermometer.variable":        "thermometer",
	"battery.100":                 "battery-full",
	"battery.75percent":           "battery-medium",
	"laptopcomputer":              "laptop",
	"powerplug":                   "plug",
	"heart":                       "heart",
	"arrow.triangle.2.circlepath": "refresh-cw",
	"cpu":                         "cpu",
	"memorychip":                  "memory-stick",
	"fan":                         "fan",
	"eye.slash":                   "eye-off",
	"circle.dashed":               "circle-dashed",
	"bolt.circle":                 "zap",
	"bolt.fill":                   "zap",
	"exclamationmark.triangle":    "triangle-alert",
	"percent":                     "percent",
}

// LucideName returns the Lucide icon standing in for an SF Symbol name.
func LucideName(sfSymbol string) string {
	if name, ok := lucide[sfSymbol]; ok {
		return name
	}
	return "battery-medium"
}

// Configuration is what the user picked for the label. Its JSON matches the Swift app's Codable form.
type Configuration struct {
	IconStyle IconStyle `json:"iconStyle"`
	Items     []Item    `json:"items"`
	// Spacing is in points between items.
	Spacing float64 `json:"spacing"`
	// UpdateInterval is the seconds between menu bar refreshes; fewer redraws keep the item steady and cheaper.
	UpdateInterval float64 `json:"updateInterval"`
}

var (
	SpacingRange        = [2]float64{0, 12}
	UpdateIntervalRange = [2]float64{2, 20}
)

// DefaultConfiguration is the Swift app's MenuBarConfiguration().
func DefaultConfiguration() Configuration {
	return Configuration{IconStyle: IconBattery, Items: []Item{ItemBatteryPercent}, Spacing: 6, UpdateInterval: 2}
}

// LegacyStyle is the pre-catalog setting stored under "menuBarDisplayStyle".
type LegacyStyle string

const (
	LegacyBatteryPercent      LegacyStyle = "batteryPercent"
	LegacyBatteryPercentPower LegacyStyle = "batteryPercentPower"
	LegacyPowerOnly           LegacyStyle = "powerOnly"
)

// FromLegacy maps a pre-catalog style onto catalog items.
func FromLegacy(style LegacyStyle) Configuration {
	cfg := DefaultConfiguration()
	switch style {
	case LegacyBatteryPercentPower:
		cfg.Items = []Item{ItemBatteryPercent, ItemSystemPower}
	case LegacyPowerOnly:
		cfg.IconStyle = IconHidden
		cfg.Items = []Item{ItemSystemPower}
	}
	return cfg
}

// Equal compares configurations by value.
func (c Configuration) Equal(o Configuration) bool {
	return c.IconStyle == o.IconStyle && slices.Equal(c.Items, o.Items) && c.Spacing == o.Spacing && c.UpdateInterval == o.UpdateInterval
}

// Contains reports whether the item is selected.
func (c Configuration) Contains(item Item) bool { return slices.Contains(c.Items, item) }

// DecodeConfiguration reads the Swift app's JSON. Like Swift's synthesized Codable it fails on a
// missing key or an unknown value, so a damaged setting falls back as it did there.
func DecodeConfiguration(data []byte) (Configuration, error) {
	var raw struct {
		IconStyle      *IconStyle `json:"iconStyle"`
		Items          *[]Item    `json:"items"`
		Spacing        *float64   `json:"spacing"`
		UpdateInterval *float64   `json:"updateInterval"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Configuration{}, err
	}
	if raw.IconStyle == nil || raw.Items == nil || raw.Spacing == nil || raw.UpdateInterval == nil {
		return Configuration{}, errors.New("menubar: configuration is missing a key")
	}
	if !slices.Contains(IconStyles, *raw.IconStyle) {
		return Configuration{}, fmt.Errorf("menubar: unknown icon style %q", *raw.IconStyle)
	}
	for _, item := range *raw.Items {
		if !slices.Contains(Items, item) {
			return Configuration{}, fmt.Errorf("menubar: unknown item %q", item)
		}
	}
	return Configuration{IconStyle: *raw.IconStyle, Items: *raw.Items, Spacing: *raw.Spacing, UpdateInterval: *raw.UpdateInterval}, nil
}

// Encode writes the JSON the Swift app reads.
func (c Configuration) Encode() ([]byte, error) {
	if c.Items == nil {
		c.Items = []Item{}
	}
	return json.Marshal(c)
}

// Move puts item just before the item before, or at the end when before is "" or not selected.
func (c *Configuration) Move(item, before Item) {
	if item == before || !c.Contains(item) {
		return
	}
	items := slices.DeleteFunc(slices.Clone(c.Items), func(i Item) bool { return i == item })
	at := slices.Index(items, before)
	if at < 0 {
		at = len(items)
	}
	c.Items = slices.Insert(items, at, item)
}

// Toggle selects an unselected item at the end, or removes a selected one.
func (c *Configuration) Toggle(item Item) {
	if c.Contains(item) {
		c.Items = slices.DeleteFunc(slices.Clone(c.Items), func(i Item) bool { return i == item })
	} else {
		c.Items = append(slices.Clone(c.Items), item)
	}
}
