package macos

import (
	"reflect"
	"testing"
)

func TestParsePlist(t *testing.T) {
	const doc = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>ExternalConnected</key><true/>
	<key>IsCharging</key><false/>
	<key>CurrentCapacity</key><integer>85</integer>
	<key>BatteryPower</key><integer>18446744073709536582</integer>
	<key>Temperature</key><real>30.5</real>
	<key>DeviceName</key><string>bq40z651</string>
	<key>CellVoltage</key><array><integer>4100</integer><integer>4098</integer></array>
	<key>AdapterDetails</key><dict><key>Watts</key><integer>96</integer></dict>
	<key>Raw</key><data>AAEC</data>
</dict>
</plist>`
	value, err := ParsePlist([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	dict := value.(map[string]any)

	if v, _ := Bool(dict, "ExternalConnected"); !v {
		t.Error("ExternalConnected should be true")
	}
	if v, ok := Bool(dict, "IsCharging"); !ok || v {
		t.Error("IsCharging should be false")
	}
	if v, _ := Int(dict, "CurrentCapacity"); v != 85 {
		t.Errorf("CurrentCapacity = %d", v)
	}
	if v, _ := Int(dict, "BatteryPower"); v != -15034 {
		t.Errorf("unsigned bit pattern should decode as signed: got %d", v)
	}
	if v, _ := Int(dict, "Temperature"); v != 30 {
		t.Errorf("reals truncate: got %d", v)
	}
	if v, _ := String(dict, "DeviceName"); v != "bq40z651" {
		t.Errorf("DeviceName = %q", v)
	}
	if v := dict["CellVoltage"]; !reflect.DeepEqual(v, []any{int64(4100), int64(4098)}) {
		t.Errorf("CellVoltage = %v", v)
	}
	adapter, _ := Dict(dict, "AdapterDetails")
	if v, _ := Int(adapter, "Watts"); v != 96 {
		t.Errorf("Watts = %d", v)
	}
	if v := dict["Raw"]; !reflect.DeepEqual(v, []byte{0, 1, 2}) {
		t.Errorf("Raw = %v", v)
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	in := map[string]any{
		"name": "a < b", "count": int64(-3), "ratio": 0.5, "on": true,
		"raw": []byte{9, 8}, "list": []any{"x", int64(1)}, "nested": map[string]any{"k": false},
	}
	xml, err := EncodePlist(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ParsePlist(xml)
	if err != nil {
		t.Fatalf("%v\n%s", err, xml)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip changed the value:\n in %v\nout %v", in, out)
	}
}
