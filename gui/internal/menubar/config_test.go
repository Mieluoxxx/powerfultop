package menubar

import (
	"slices"
	"testing"
)

func TestDecodeSwiftJSON(t *testing.T) {
	legacy := `{"iconStyle":"battery","items":["batteryPercent"],"spacing":6,"updateInterval":2}`
	cfg, err := DecodeConfiguration([]byte(legacy))
	if err != nil || cfg.IconStyle != IconBattery {
		t.Fatalf("saved simple style still decodes: %+v %v", cfg, err)
	}
	full := `{"updateInterval":5,"items":["systemPower","cpuUsage","fanSpeed"],"iconStyle":"macOSColor","spacing":3.5}`
	cfg, err = DecodeConfiguration([]byte(full))
	if err != nil || cfg.IconStyle != IconMacOSColor || cfg.Spacing != 3.5 || cfg.UpdateInterval != 5 || len(cfg.Items) != 3 {
		t.Fatalf("Swift JSON decodes in any key order: %+v %v", cfg, err)
	}
	encoded, err := cfg.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeConfiguration(encoded)
	if err != nil || !back.Equal(cfg) {
		t.Errorf("configuration persists: %s -> %+v %v", encoded, back, err)
	}
	empty := DefaultConfiguration()
	empty.Items = nil
	if data, _ := empty.Encode(); string(data) != `{"iconStyle":"battery","items":[],"spacing":6,"updateInterval":2}` {
		t.Errorf("no items encode as an empty array Swift can read: %s", data)
	}
}

func TestDecodeRejectsWhatSwiftRejected(t *testing.T) {
	for _, bad := range []string{
		`{"iconStyle":"battery","items":["batteryPercent"],"spacing":6}`,
		`{"iconStyle":"sparkles","items":[],"spacing":6,"updateInterval":2}`,
		`{"iconStyle":"battery","items":["warpDrive"],"spacing":6,"updateInterval":2}`,
		`not json`,
	} {
		if _, err := DecodeConfiguration([]byte(bad)); err == nil {
			t.Errorf("should fail: %s", bad)
		}
	}
}

func TestMoveAndToggle(t *testing.T) {
	cfg := Configuration{Items: []Item{ItemBatteryPercent, ItemSystemPower, ItemCPUUsage}}
	cfg.Move(ItemCPUUsage, ItemBatteryPercent)
	if want := []Item{ItemCPUUsage, ItemBatteryPercent, ItemSystemPower}; !slices.Equal(cfg.Items, want) {
		t.Errorf("move before: %v", cfg.Items)
	}
	cfg.Move(ItemCPUUsage, "")
	if want := []Item{ItemBatteryPercent, ItemSystemPower, ItemCPUUsage}; !slices.Equal(cfg.Items, want) {
		t.Errorf("move to end: %v", cfg.Items)
	}
	cfg.Toggle(ItemSystemPower)
	cfg.Toggle(ItemFanSpeed)
	if want := []Item{ItemBatteryPercent, ItemCPUUsage, ItemFanSpeed}; !slices.Equal(cfg.Items, want) {
		t.Errorf("toggle: %v", cfg.Items)
	}
}
