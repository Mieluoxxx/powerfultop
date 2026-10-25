//go:build darwin

package menubar

import (
	"slices"
	"testing"

	"powerfultop/internal/macos"
)

func testPrefs(t *testing.T) macos.Preferences {
	p := macos.DomainPreferences("io.github.mieluoxxx.PowerfulTop.gotest.menubar")
	clear := func() {
		for _, key := range []string{configurationKey, legacyDisplayStyleKey, legacyShowPowerInMenuBar} {
			_ = p.Set(key, nil)
		}
	}
	clear()
	t.Cleanup(clear)
	return p
}

func TestSettingsLegacyMigration(t *testing.T) {
	p := testPrefs(t)
	if got := NewSettings(p).Configuration(); !got.Equal(FromLegacy(LegacyBatteryPercent)) {
		t.Errorf("nothing stored: %+v", got)
	}
	_ = p.Set(legacyShowPowerInMenuBar, true)
	if got := NewSettings(p).Configuration(); !slices.Equal(got.Items, []Item{ItemBatteryPercent, ItemSystemPower}) {
		t.Errorf("power toggle migrates: %+v", got)
	}
	_ = p.Set(legacyDisplayStyleKey, "powerOnly")
	if got := NewSettings(p).Configuration(); got.IconStyle != IconHidden {
		t.Errorf("display style wins over the older toggle: %+v", got)
	}
	_ = p.Set(configurationKey, []byte(`{"iconStyle":"ring","items":["cpuUsage"],"spacing":2,"updateInterval":4}`))
	if got := NewSettings(p).Configuration(); got.IconStyle != IconRing || got.UpdateInterval != 4 {
		t.Errorf("the Swift app's saved configuration loads: %+v", got)
	}
}

func TestSettingsUpdateSavesAndNotifies(t *testing.T) {
	p := testPrefs(t)
	s := NewSettings(p)
	calls := 0
	s.OnChange(func() { calls++ })
	s.Update(func(c *Configuration) { c.Items = append(c.Items, ItemFanSpeed) })
	s.Update(func(c *Configuration) {})
	if calls != 1 {
		t.Errorf("listeners hear changes only: %d calls", calls)
	}
	if got := NewSettings(p).Configuration(); !got.Contains(ItemFanSpeed) {
		t.Errorf("the change was saved: %+v", got)
	}
	data, _ := p.Data(configurationKey)
	if _, err := DecodeConfiguration(data); err != nil {
		t.Errorf("saved as JSON data the Swift app reads: %s %v", data, err)
	}
}
