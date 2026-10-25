package view

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSettingsPage(t *testing.T) {
	var login *bool
	installs, copied := 0, ""
	d := SettingsData{
		ChargeLimitText: "80%", HasFans: true, HelperStatus: HelperNeedsUpdate,
		Version: "1.4.0", ChipName: "bq40z651", Serial: "F5D123",
	}
	a := SettingsActions{
		SetLaunchAtLogin: func(on bool) { login = &on },
		InstallHelper:    func() { installs++ },
		CopySerial:       func(s string) { copied = s },
	}
	tt := ui.NewTester(func(c *ui.Context) { SettingsPage(c, d, a) }, 700, 900)
	for _, want := range []string{"General", "Charge Limit", "80%", "Needs authorization", "Update Fan Helper", "Uninstall", "v1.4.0", "bq40z651", "F5D123"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	if r, ok := tt.Find("Launch at Login"); !ok || r.W == 0 {
		t.Error("the switch needs a visible label")
	}
	if err := tt.Click("Launch at Login"); err != nil {
		t.Fatal(err)
	}
	if login == nil || !*login {
		t.Error("toggling the switch should ask to launch at login")
	}
	if err := tt.Click("Update Fan Helper"); err != nil {
		t.Fatal(err)
	}
	if installs != 1 {
		t.Errorf("installs = %d", installs)
	}
	if err := tt.Click("Copy"); err != nil {
		t.Fatal(err)
	}
	if copied != "F5D123" {
		t.Errorf("copied %q", copied)
	}

	d.HasFans = false
	tt = ui.NewTester(func(c *ui.Context) { SettingsPage(c, d, a) }, 700, 900)
	if tt.HasText("Fan Helper") {
		t.Error("no fan helper card without fans")
	}
}
