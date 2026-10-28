package view

import (
	"github.com/egoist/mygo/ui"

	"github.com/ELY-Open-Source/CrychicUI/button"
	"github.com/ELY-Open-Source/CrychicUI/choice"
)

// SettingsData is what the settings page shows, gathered by the app from its monitors.
type SettingsData struct {
	LaunchAtLogin      bool
	LaunchAtLoginError string

	Version         string
	ChipName        string
	ManufactureDate string
	Serial          string
}

// SettingsActions are what the page's controls do.
type SettingsActions struct {
	SetLaunchAtLogin func(bool)
	CopySerial       func(string)
}

// SettingsPage lays out app-wide settings as System Settings does; the charge limit lives on the
// Battery page and the fan helper on the Cooling page, next to what they control.
func SettingsPage(c *ui.Context, d SettingsData, a SettingsActions) {
	ui.Column(c).Gap(22).MaxWidth(600).Children(func() {
		ui.Column(c).Gap(6).Children(func() {
			Section(c, L("General", "通用"), "", func() {
				on := d.LaunchAtLogin
				if choice.ToggleRow(c, L("Launch at Login", "开机启动"), "", &on).Changed() && a.SetLaunchAtLogin != nil {
					a.SetLaunchAtLogin(on)
				}
			})
			if d.LaunchAtLoginError != "" {
				ui.Column(c).PaddingX(10).Children(func() {
					Status(c, "octagon-x", Red, d.LaunchAtLoginError, "")
				})
			}
		})

		rows := []func(){func() {
			SectionLabel(c, L("Version", "版本"))
			SectionValue(c, "PowerfulTop "+d.Version)
		}}
		if d.ChipName != "" {
			rows = append(rows, func() {
				SectionLabel(c, L("Battery Management Chip", "电池管理芯片"))
				SectionValue(c, d.ChipName)
			})
		}
		if d.ManufactureDate != "" {
			rows = append(rows, func() {
				SectionLabel(c, L("Manufacture Date", "制造日期"))
				SectionValue(c, d.ManufactureDate)
			})
		}
		if d.Serial != "" {
			rows = append(rows, func() {
				SectionLabel(c, L("Battery Serial Number", "电池序列号"))
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					SectionValue(c, d.Serial)
					if button.TextButton(c, L("Copy", "复制")).Tooltip(L("Copy serial number", "复制序列号")).Clicked() && a.CopySerial != nil {
						a.CopySerial(d.Serial)
					}
				})
			})
		}
		Section(c, L("About", "关于"), L("Device information is shown locally only and is never uploaded.", "设备信息仅在本地显示，不会上传。"), rows...)
	})
}
