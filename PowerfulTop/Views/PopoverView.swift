import SwiftUI

struct PopoverView: View {
    static let preferredWidth: CGFloat = 300

    let monitor: PowerMonitor
    let thermal: ThermalMonitor
    let system: SystemMonitor
    let displays: DisplayController
    let openDetails: () -> Void

    private var data: PowerData { monitor.currentData }

    var body: some View {
        contentSections
            .frame(width: Self.preferredWidth, alignment: .topLeading)
            .fixedSize(horizontal: false, vertical: true)
            .onAppear {
                DispatchQueue.main.async {
                    NSApp.keyWindow?.makeFirstResponder(nil)
                }
            }
    }

    private var contentSections: some View {
        VStack(spacing: 0) {
            if !monitor.isDataAvailable {
                unavailableBanner
            }
            headerSection
                .padding(.horizontal, 14)
                .padding(.top, 14)
                .padding(.bottom, 12)
            Divider().padding(.horizontal, 12)
            powerFlowSection
                .padding(.horizontal, 14)
                .padding(.vertical, 12)
            Divider().padding(.horizontal, 12)
            systemSection
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
            if !displays.displays.isEmpty {
                Divider().padding(.horizontal, 12)
                displaySection
                    .padding(.horizontal, 14)
                    .padding(.vertical, 10)
            }
            if thermal.snapshot.hasFans {
                Divider().padding(.horizontal, 12)
                fanSection
                    .padding(.horizontal, 14)
                    .padding(.vertical, 10)
            }
            if hasBatteryStats {
                Divider().padding(.horizontal, 12)
                batteryStatsSection
                    .padding(.horizontal, 14)
                    .padding(.top, 10)
                chargeLimitRow
                    .padding(.horizontal, 14)
                    .padding(.top, 8)
                    .padding(.bottom, 10)
            }
            Divider().padding(.horizontal, 12)
            footerSection
                .padding(.horizontal, 10)
                .padding(.vertical, 6)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var unavailableBanner: some View {
        HStack(spacing: 6) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(.orange)
            Text(String(localized: "Built-in battery not detected. PowerfulTop requires a MacBook."))
                .font(.system(size: 11))
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 8)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.orange.opacity(0.08))
    }

    // MARK: - Header

    private var statusSymbol: String {
        switch data.powerState {
        case .onBattery: return data.batteryLevelSymbol
        case .connectingAC: return "powerplug"
        case .acIdle: return "powerplug.fill"
        case .charging: return "bolt.fill"
        case .supplementing: return "exclamationmark.triangle.fill"
        }
    }

    private var statusTint: Color {
        switch data.powerState {
        case .onBattery: return data.displayBatteryPercent <= 20 ? .red : .secondary
        case .connectingAC: return .secondary
        case .acIdle, .charging: return .green
        case .supplementing: return .orange
        }
    }

    private var statusLine: String {
        guard let time = data.estimatedTimeRemainingText else { return data.powerSourceDescription }
        return "\(data.powerSourceDescription) · \(data.estimatedTimeRemainingLabel) \(time)"
    }

    private var headerSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("\(data.displayBatteryPercent)%")
                .font(.system(size: 30, weight: .bold, design: .rounded))
                .monospacedDigit()

            LevelBar(fraction: Double(data.displayBatteryPercent) / 100, tint: data.batteryLevelTint)

            HStack(alignment: .firstTextBaseline, spacing: 5) {
                Image(systemName: statusSymbol)
                    .foregroundStyle(statusTint)
                Text(statusLine)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .font(.system(size: 12))

            if let reason = data.notChargingReasonDescription,
               data.powerState == .acIdle || data.isSupplementalDischarge {
                Text(reason)
                    .font(.system(size: 11))
                    .foregroundStyle(.tertiary)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    // MARK: - Power Flow

    private func watts(_ value: Double) -> String {
        String(format: "%.1f W", value)
    }

    private var chargerNode: FlowNode {
        FlowNode(
            icon: "powerplug.fill",
            title: String(localized: "Charger"),
            value: watts(data.effectiveACOutputW),
            capacity: data.acAdapterWattage > 0 ? "\(data.acAdapterWattage) W" : nil,
            tint: .green
        )
    }

    private var flowNodes: (sources: [FlowNode], destinations: [FlowNode]) {
        let system = FlowNode(icon: "laptopcomputer", title: String(localized: "System"), value: watts(data.systemPowerW), tint: .primary)
        let battery = String(localized: "Battery")
        switch data.powerState {
        case .onBattery:
            return ([FlowNode(icon: data.batteryLevelSymbol, title: battery, value: nil, tint: .orange)], [system])
        case .connectingAC:
            return ([FlowNode(icon: "powerplug", title: String(localized: "AC Connecting"), value: nil, tint: .secondary)], [system])
        case .acIdle:
            return ([chargerNode], [system])
        case .charging(let rate):
            return ([chargerNode], [system, FlowNode(icon: "battery.100.bolt", title: battery, value: watts(rate), tint: .blue)])
        case .supplementing(let rate):
            return ([chargerNode, FlowNode(icon: "battery.50", title: battery, value: watts(rate), tint: .orange)], [system])
        }
    }

    private var powerFlowSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 6) {
                VStack(spacing: 6) {
                    ForEach(flowNodes.sources) { FlowNodeView(node: $0) }
                }
                Image(systemName: "arrow.right")
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundStyle(.tertiary)
                VStack(spacing: 6) {
                    ForEach(flowNodes.destinations) { FlowNodeView(node: $0) }
                }
            }

            if data.dataSource == .legacy {
                Label(String(localized: "Estimation Mode"), systemImage: "exclamationmark.triangle.fill")
                    .font(.system(size: 11))
                    .foregroundStyle(.yellow)
            }
        }
    }

    // MARK: - Battery Stats

    private var hasBatteryStats: Bool {
        data.batteryHealthPercent != nil || packTemperatureC != nil || data.cycleCount != nil
    }

    private var packTemperatureC: Double? {
        batteryTemperatureC(data: data, thermal: thermal.snapshot)
    }

    private var batteryStatsSection: some View {
        HStack(spacing: 0) {
            if let health = data.batteryHealthPercent {
                StatCell(
                    value: "\(health)%",
                    label: String(localized: "Health"),
                    tint: health >= 80 ? .primary : (health >= 60 ? .orange : .red)
                )
            }
            if let temp = packTemperatureC {
                StatCell(
                    value: String(format: "%.1f°", temp),
                    label: String(localized: "Battery Temp"),
                    tint: temp > 40 ? .red : .primary
                )
            }
            if let cycles = data.cycleCount {
                StatCell(value: "\(cycles)", label: String(localized: "Cycle Count"), tint: .primary)
            }
        }
    }

    // MARK: - System

    private var systemSection: some View {
        HStack(spacing: 14) {
            UsageMeter(
                icon: "cpu",
                title: String(localized: "CPU"),
                fraction: system.cpu.usage,
                tint: .blue,
                detail: thermal.snapshot.temperatures[.cpu].map { String(format: "%.0f°C", $0.averageC) }
            )
            UsageMeter(
                icon: "memorychip",
                title: String(localized: "Memory"),
                fraction: system.memory?.usedFraction ?? 0,
                tint: system.memory?.pressure == .normal || system.memory == nil ? .green : .orange,
                detail: system.memory.map { "\(formatBytes(Double($0.usedBytes))) / \(formatBytes(Double($0.totalBytes)))" }
            )
        }
    }

    // MARK: - Displays

    private var displaySection: some View {
        VStack(alignment: .leading, spacing: 10) {
            ForEach(displays.displays) { display in
                BrightnessRow(display: display, powerW: display.isBuiltIn ? thermal.snapshot.displayPowerW : nil) {
                    displays.setLevel($0, for: display.id)
                }
            }
        }
    }

    // MARK: - Fans

    private enum FanPreset: Hashable { case automatic, fullBlast, custom }

    private var fanPreset: FanPreset {
        switch thermal.mode {
        case .automatic: return .automatic
        case .fullBlast: return .fullBlast
        case .constant, .sensor: return .custom
        }
    }

    private var fanSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 6) {
                Image(systemName: "fan.fill")
                    .foregroundStyle(.teal)
                Text(String(format: "%.0f RPM", thermal.snapshot.fans.map(\.actualRPM).max() ?? 0))
                    .font(.system(size: 13, weight: .semibold, design: .rounded))
                    .monospacedDigit()
                Text(thermal.isControlling ? String(localized: "Controlled by PowerfulTop") : String(localized: "Controlled by macOS"))
                    .font(.system(size: 11))
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                Spacer(minLength: 0)
            }
            .font(.system(size: 12))
            Picker("", selection: Binding(get: { fanPreset }, set: { preset in
                switch preset {
                case .automatic: thermal.setMode(.automatic)
                case .fullBlast: thermal.setMode(.fullBlast)
                case .custom: thermal.setMode(thermal.lastCustomMode)
                }
            })) {
                Text(String(localized: "Auto")).tag(FanPreset.automatic)
                Text(String(localized: "Full Blast")).tag(FanPreset.fullBlast)
                Text(String(localized: "Custom")).tag(FanPreset.custom)
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .controlSize(.small)
            fanStatus
        }
    }

    @ViewBuilder
    private var fanStatus: some View {
        if thermal.conflictDetected && !thermal.isControlling {
            StatusNote(icon: "exclamationmark.triangle.fill", tint: .orange, text: String(localized: "Another fan tool is in control"))
        } else if !thermal.mode.isAutomatic && thermal.helperStatus != .ready {
            Button {
                thermal.installHelper()
            } label: {
                Label(
                    thermal.helperStatus == .needsUpdate ? String(localized: "Update Fan Helper") : String(localized: "Install Fan Helper"),
                    systemImage: "lock.shield"
                )
                .font(.system(size: 11))
            }
            .controlSize(.small)
        } else if let error = thermal.lastError {
            StatusNote(icon: "xmark.octagon.fill", tint: .red, text: error)
        } else if case .constant(let rpm) = thermal.mode {
            StatusNote(icon: "gauge.with.dots.needle.33percent", tint: .secondary, text: String(format: String(localized: "Holding %.0f RPM"), rpm))
        } else if case .sensor(let group, let lowerC, let upperC) = thermal.mode {
            StatusNote(icon: "thermometer.medium", tint: .secondary,
                       text: String(format: String(localized: "%@ ramp %.0f–%.0f°C"), group.title, lowerC, upperC))
        }
    }

    // MARK: - Charge limit

    private var chargeLimitRow: some View {
        Button {
            NSWorkspace.shared.open(macOSChargeLimitSettingsURL)
        } label: {
            HStack(spacing: 6) {
                Image(systemName: "battery.100.bolt")
                    .foregroundStyle(.green)
                Text(String(localized: "Charge Limit"))
                    .foregroundStyle(.secondary)
                Spacer()
                Text(data.chargeLimitText)
                    .fontWeight(.medium)
                Image(systemName: "chevron.right")
                    .font(.system(size: 9, weight: .bold))
                    .foregroundStyle(.tertiary)
            }
            .font(.system(size: 12))
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .help(String(localized: "Adjust the charge limit (80–100%) in System Settings"))
    }

    // MARK: - Footer

    private var footerSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 2) {
                Button {
                    openDetails()
                } label: {
                    HStack(spacing: 4) {
                        Text(String(localized: "Details"))
                        Image(systemName: "chevron.right")
                            .font(.system(size: 9, weight: .bold))
                    }
                    .padding(.horizontal, 6)
                    .padding(.vertical, 4)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)

                Spacer()

                settingsMenu

                Button {
                    NSApplication.shared.terminate(nil)
                } label: {
                    Image(systemName: "power")
                        .padding(.horizontal, 6)
                        .padding(.vertical, 4)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .help(String(localized: "Quit"))
            }
            .font(.system(size: 12))
            .foregroundStyle(.secondary)
            .focusEffectDisabled()

            if let error = monitor.launchAtLoginError {
                Text(error)
                    .font(.system(size: 10))
                    .foregroundStyle(.red)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.horizontal, 6)
            }
        }
    }

    private var settingsMenu: some View {
        Menu {
            Toggle(String(localized: "Launch at Login"), isOn: Binding(
                get: { monitor.launchAtLogin },
                set: { monitor.launchAtLogin = $0 }
            ))
            Button(String(localized: "Customize Menu Bar…")) {
                UserDefaults.standard.set(DetailWindowView.Page.menuBar.rawValue, forKey: "dashboardPage")
                openDetails()
            }
            Divider()
            Text(AppInfo.versionLabel)
            Button(String(localized: "Quit")) {
                NSApplication.shared.terminate(nil)
            }
        } label: {
            Image(systemName: "gearshape")
        }
        .menuStyle(.borderlessButton)
        .menuIndicator(.hidden)
        .fixedSize()
        .padding(.horizontal, 4)
    }

}

// MARK: - Components

private struct FlowNode: Identifiable {
    let icon: String
    let title: String
    let value: String?
    var capacity: String? = nil
    let tint: Color
    var id: String { icon + title }
}

private struct FlowNodeView: View {
    let node: FlowNode

    var body: some View {
        HStack(spacing: 6) {
            Image(systemName: node.icon)
                .font(.system(size: 13))
                .foregroundStyle(node.tint)
                .frame(width: 18)
            VStack(alignment: .leading, spacing: 1) {
                HStack(spacing: 4) {
                    Text(node.title)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .minimumScaleFactor(0.8)
                    if let capacity = node.capacity {
                        Spacer(minLength: 0)
                        Text(capacity)
                            .foregroundStyle(.tertiary)
                            .monospacedDigit()
                    }
                }
                .font(.system(size: 10))
                if let value = node.value {
                    Text(value)
                        .font(.system(size: 14, weight: .semibold, design: .rounded))
                        .monospacedDigit()
                        .lineLimit(1)
                }
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 7)
        // Value-less nodes (battery-only source) must line up with their valued counterpart.
        .frame(maxWidth: .infinity, minHeight: 46, alignment: .leading)
        .background(node.tint.opacity(0.1), in: RoundedRectangle(cornerRadius: 8))
    }
}

private struct StatCell: View {
    let value: String
    let label: String
    let tint: Color

    var body: some View {
        VStack(spacing: 2) {
            Text(value)
                .font(.system(size: 13, weight: .semibold, design: .rounded))
                .monospacedDigit()
                .foregroundStyle(tint)
            Text(label)
                .font(.system(size: 10))
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .minimumScaleFactor(0.8)
        }
        .frame(maxWidth: .infinity)
    }
}

/// Compact usage bar for the popover: icon, title, percent and a secondary reading.
private struct UsageMeter: View {
    let icon: String
    let title: String
    let fraction: Double
    let tint: Color
    let detail: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack(spacing: 5) {
                Image(systemName: icon)
                    .foregroundStyle(tint)
                Text(title)
                    .foregroundStyle(.secondary)
                Spacer(minLength: 2)
                Text(String(format: "%.0f%%", fraction * 100))
                    .fontWeight(.semibold)
                    .monospacedDigit()
            }
            .font(.system(size: 12))
            LevelBar(fraction: fraction, tint: tint, height: 5)
            if let detail {
                Text(detail)
                    .font(.system(size: 10))
                    .foregroundStyle(.tertiary)
                    .monospacedDigit()
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

private struct BrightnessRow: View {
    let display: DisplayBrightness
    /// Only the built-in panel's draw is measurable; external monitors have their own supply.
    let powerW: Double?
    let onChange: (Double) -> Void

    private var methodLabel: String {
        switch display.method {
        case .native: String(localized: "Native")
        case .ddc: String(localized: "DDC")
        case .software: String(localized: "Software")
        }
    }

    private var methodHelp: String {
        switch display.method {
        case .native: String(localized: "Backlight controlled by macOS")
        case .ddc: String(localized: "Backlight controlled over DDC/CI")
        case .software: String(localized: "This monitor doesn't answer DDC/CI, so the picture is darkened instead of the backlight. Turn on DDC/CI in the monitor's menu or connect over USB-C or DisplayPort.")
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 6) {
                Image(systemName: display.isBuiltIn ? "laptopcomputer" : "display")
                    .foregroundStyle(.yellow)
                Text(display.name)
                    .lineLimit(1)
                    .truncationMode(.middle)
                Text(methodLabel)
                    .font(.system(size: 9, weight: .medium))
                    .foregroundStyle(display.method == .software ? .orange : .secondary)
                    .padding(.horizontal, 5)
                    .padding(.vertical, 1)
                    .background(.quaternary, in: Capsule())
                    .help(methodHelp)
                Spacer(minLength: 4)
                if let powerW {
                    Text(String(format: "%.1f W", powerW))
                        .foregroundStyle(.secondary)
                        .monospacedDigit()
                        .help(String(localized: "Built-in display, included in system power"))
                }
                Text(String(format: "%.0f%%", display.level * 100))
                    .fontWeight(.semibold)
                    .monospacedDigit()
            }
            .font(.system(size: 12))
            HStack(spacing: 6) {
                Image(systemName: "sun.min")
                    .font(.system(size: 10))
                    .foregroundStyle(.secondary)
                Slider(value: Binding(get: { display.level }, set: onChange), in: 0...1)
                    .controlSize(.small)
                Image(systemName: "sun.max")
                    .font(.system(size: 11))
                    .foregroundStyle(.secondary)
            }
        }
    }
}
