import SwiftUI

struct PopoverView: View {
    static let preferredWidth: CGFloat = 280

    let monitor: PowerMonitor
    let thermal: ThermalMonitor
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
            if hasBatteryStats {
                Divider().padding(.horizontal, 12)
                batteryStatsSection
                    .padding(.horizontal, 14)
                    .padding(.vertical, 10)
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
            Text(String(localized: "Built-in battery not detected. PowerTop requires a MacBook."))
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
        data.batteryHealthPercent != nil || packTemperatureC != nil || data.cycleCount != nil || thermal.snapshot.hasFans
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
            if thermal.snapshot.hasFans {
                StatCell(
                    value: String(format: "%.0f", thermal.snapshot.fans.map(\.actualRPM).max() ?? 0),
                    label: String(localized: "Fan RPM"),
                    tint: thermal.isControlling ? .teal : .primary
                )
            }
        }
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
            if thermal.snapshot.hasFans {
                Picker(String(localized: "Fans"), selection: Binding(
                    get: { thermal.mode.isAutomatic ? 0 : (thermal.mode == .fullBlast ? 1 : 2) },
                    set: { choice in
                        switch choice {
                        case 0: thermal.setMode(.automatic)
                        case 1: thermal.setMode(.fullBlast)
                        default: thermal.setMode(thermal.lastCustomMode)
                        }
                    }
                )) {
                    Text(String(localized: "Auto")).tag(0)
                    Text(String(localized: "Full Blast")).tag(1)
                    Text(String(localized: "Custom")).tag(2)
                }
                .pickerStyle(.menu)
                Divider()
            }
            Button(String(format: String(localized: "Charge Limit: %@…"), data.chargeLimitText)) {
                NSWorkspace.shared.open(macOSChargeLimitSettingsURL)
            }
            Divider()
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
