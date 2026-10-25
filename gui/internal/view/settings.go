package view

import (
	"github.com/egoist/mygo/ui"

	"powerfultop/internal/icons"
)

// HelperStatus is the fan helper's install state as the settings show it.
type HelperStatus int

const (
	HelperNotInstalled HelperStatus = iota
	HelperNeedsUpdate
	HelperReady
)

// SettingsData is what the settings page shows, gathered by the app from its monitors.
type SettingsData struct {
	LaunchAtLogin      bool
	LaunchAtLoginError string

	ChargeLimitText      string
	ChargeLimitByAlDente bool

	HasFans      bool
	HelperStatus HelperStatus
	HelperError  string

	Version         string
	ChipName        string
	ManufactureDate string
	Serial          string
}

// SettingsActions are what the page's controls do.
type SettingsActions struct {
	SetLaunchAtLogin        func(bool)
	OpenChargeLimitSettings func()
	InstallHelper           func()
	UninstallHelper         func()
	CopySerial              func(string)
}

// SettingsPage mirrors the Swift SettingsPage: general, charging, fan helper and about cards.
func SettingsPage(c *ui.Context, d SettingsData, a SettingsActions) {
	ui.Column(c).Gap(16).MaxWidth(560).Children(func() {
		Card(c, L("General", "通用"), "settings", Gray, func() {
			on := d.LaunchAtLogin
			if ui.Switch(c.Key("login"), &on).Label(L("Launch at Login", "开机启动")).Changed() && a.SetLaunchAtLogin != nil {
				a.SetLaunchAtLogin(on)
			}
			if d.LaunchAtLoginError != "" {
				Status(c, "octagon-x", Red, d.LaunchAtLoginError, "")
			}
		})

		Card(c, L("Charging", "充电"), "battery-charging", Green, func() {
			Row(c, L("Charge Limit", "充电上限"), d.ChargeLimitText, "", true)
			ui.Row(c).Children(func() {
				if ui.Button(c, L("Adjust in System Settings…", "在系统设置中调整…")).Clicked() && a.OpenChargeLimitSettings != nil {
					a.OpenChargeLimitSettings()
				}
			})
			if d.ChargeLimitByAlDente {
				Note(c, L("AlDente is currently managing the charge limit. Change it in AlDente, or quit AlDente to use the macOS setting.",
					"当前由 AlDente 管理充电上限。请在 AlDente 中调整，或退出 AlDente 后使用系统设置。"))
			} else {
				Note(c, L("macOS enforces the limit (80–100%) in firmware, so it keeps working while PowerfulTop is closed or the Mac sleeps.",
					"充电上限（80–100%）由 macOS 在固件层执行，PowerfulTop 关闭或 Mac 睡眠时依然有效。"))
			}
		})

		if d.HasFans {
			Card(c, L("Fan Helper", "风扇控制组件"), "shield-check", Teal, func() {
				Row(c, L("Status", "状态"), helperStatusText(d.HelperStatus), "", true)
				ui.Row(c).Gap(8).Children(func() {
					if d.HelperStatus != HelperReady {
						label := L("Install Fan Helper", "安装风扇控制组件")
						if d.HelperStatus == HelperNeedsUpdate {
							label = L("Update Fan Helper", "更新风扇控制组件")
						}
						if ui.Button(c, label).Clicked() && a.InstallHelper != nil {
							a.InstallHelper()
						}
					}
					if d.HelperStatus != HelperNotInstalled {
						if ui.Button(c, L("Uninstall", "卸载")).Clicked() && a.UninstallHelper != nil {
							a.UninstallHelper()
						}
					}
				})
				if d.HelperError != "" {
					Status(c, "octagon-x", Red, d.HelperError, "")
				}
				Note(c, L("The helper runs as root only to write fan speeds. It accepts requests from this exact PowerfulTop build and hands fans back to macOS whenever PowerfulTop stops checking in.",
					"该组件以 root 运行，仅用于写入风扇转速；只接受当前这个 PowerfulTop 版本的请求，PowerfulTop 停止响应时会自动把风扇交还系统。"))
			})
		}

		Card(c, L("About", "关于"), "info", Gray, func() {
			Row(c, "PowerfulTop", "v"+d.Version, "", false)
			if d.ChipName != "" {
				Row(c, L("Battery Management Chip", "电池管理芯片"), d.ChipName, "", false)
			}
			if d.ManufactureDate != "" {
				Row(c, L("Manufacture Date", "制造日期"), d.ManufactureDate, "", false)
			}
			if d.Serial != "" {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Row(c).Grow(1).Children(func() {
						Row(c, L("Battery Serial Number", "电池序列号"), d.Serial, "", false)
					})
					copy := ui.ButtonBase(c).Padding(4).Radius(5).Label(L("Copy", "复制")).
						Tooltip(L("Copy serial number. Device info is shown locally only and is not uploaded.",
							"复制序列号。设备信息仅在本地显示，不会上传。")).
						Children(func() {
							ui.Icon(c, icons.Get("copy")).Size(13, 13).TextColor(c.Theme().TextMuted)
						})
					if copy.Clicked() && a.CopySerial != nil {
						a.CopySerial(d.Serial)
					}
				})
			}
		})
	})
}

func helperStatusText(s HelperStatus) string {
	switch s {
	case HelperNeedsUpdate:
		return L("Needs authorization", "需要重新授权")
	case HelperReady:
		return L("Installed", "已安装")
	}
	return L("Not installed", "未安装")
}
