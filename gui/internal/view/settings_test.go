package view

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSettingsPage(t *testing.T) {
	var login *bool
	copied := ""
	d := SettingsData{Version: "1.4.0", ChipName: "bq40z651", Serial: "F5D123"}
	a := SettingsActions{
		SetLaunchAtLogin: func(on bool) { login = &on },
		CopySerial:       func(s string) { copied = s },
	}
	tt := ui.NewTester(func(c *ui.Context) { SettingsPage(c, d, a) }, 700, 900)
	for _, want := range []string{"General", "Launch at Login", "About", "PowerfulTop 1.4.0", "bq40z651", "F5D123"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	for _, moved := range []string{"Charge Limit", "Fan Helper"} {
		if tt.HasText(moved) {
			t.Errorf("%q belongs on its own page now", moved)
		}
	}
	// Values sit at the trailing edge of the grouped box, as in System Settings.
	if r, ok := tt.Find("bq40z651"); !ok || r.X+r.W < 600-20 {
		t.Errorf("chip value at %+v, want it trailing", r)
	}
	if err := tt.Click("Launch at Login"); err != nil {
		t.Fatal(err)
	}
	if login == nil || !*login {
		t.Error("toggling the switch should ask to launch at login")
	}
	if err := tt.Click("Copy"); err != nil {
		t.Fatal(err)
	}
	if copied != "F5D123" {
		t.Errorf("copied %q", copied)
	}
}
