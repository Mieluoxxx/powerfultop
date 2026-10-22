import AppKit
import SwiftUI

struct SettingsPage: View {
    let monitor: PowerMonitor
    let thermal: ThermalMonitor

    private var data: PowerData { monitor.currentData }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            DashCard(title: String(localized: "General"), icon: "gearshape", tint: .gray) {
                Toggle(String(localized: "Launch at Login"), isOn: Binding(
                    get: { monitor.launchAtLogin },
                    set: { monitor.launchAtLogin = $0 }
                ))
                .font(.system(size: 12))
                if let error = monitor.launchAtLoginError {
                    StatusNote(icon: "xmark.octagon.fill", tint: .red, text: error)
                }
            }

            DashCard(title: String(localized: "Charging"), icon: "battery.100.bolt", tint: .green) {
                CardRow(label: String(localized: "Charge Limit"), value: data.chargeLimitText, emphasized: true)
                HStack {
                    Button(String(localized: "Adjust in System Settings…")) {
                        NSWorkspace.shared.open(macOSChargeLimitSettingsURL)
                    }
                    Spacer()
                }
                CardNote(data.chargeLimitSource == .alDente
                    ? String(localized: "AlDente is currently managing the charge limit. Change it in AlDente, or quit AlDente to use the macOS setting.")
                    : String(localized: "macOS enforces the limit (80–100%) in firmware, so it keeps working while PowerTop is closed or the Mac sleeps."))
            }

            if thermal.snapshot.hasFans {
                DashCard(title: String(localized: "Fan Helper"), icon: "lock.shield", tint: .teal) {
                    CardRow(label: String(localized: "Status"), value: helperStatusText, emphasized: true)
                    HStack(spacing: 8) {
                        if thermal.helperStatus != .ready {
                            Button(thermal.helperStatus == .needsUpdate ? String(localized: "Update Fan Helper") : String(localized: "Install Fan Helper")) {
                                thermal.installHelper()
                            }
                        }
                        if thermal.helperStatus != .notInstalled {
                            Button(String(localized: "Uninstall"), role: .destructive) {
                                thermal.uninstallHelper()
                            }
                        }
                        Spacer()
                    }
                    if let error = thermal.lastError {
                        StatusNote(icon: "xmark.octagon.fill", tint: .red, text: error)
                    }
                    CardNote(String(localized: "The helper runs as root only to write fan speeds. It accepts requests from this exact PowerTop build and hands fans back to macOS whenever PowerTop stops checking in."))
                }
            }

            DashCard(title: String(localized: "About"), icon: "info.circle", tint: .secondary) {
                CardRow(label: "PowerTop", value: AppInfo.versionLabel)
                if let chip = data.deviceName {
                    CardRow(label: String(localized: "Battery Management Chip"), value: chip)
                }
                if let date = data.batteryManufactureDate {
                    CardRow(label: String(localized: "Manufacture Date"), value: date)
                }
                if let serial = data.batterySerial {
                    HStack(spacing: 6) {
                        CardRow(label: String(localized: "Battery Serial Number"), value: serial)
                        Button {
                            NSPasteboard.general.clearContents()
                            NSPasteboard.general.setString(serial, forType: .string)
                        } label: {
                            Image(systemName: "doc.on.doc")
                        }
                        .buttonStyle(.borderless)
                        .help(String(localized: "Copy serial number. Device info is shown locally only and is not uploaded."))
                    }
                }
            }
        }
        .frame(maxWidth: 560, alignment: .leading)
    }

    private var helperStatusText: String {
        switch thermal.helperStatus {
        case .notInstalled: return String(localized: "Not installed")
        case .needsUpdate: return String(localized: "Needs authorization")
        case .ready: return String(localized: "Installed")
        }
    }
}
