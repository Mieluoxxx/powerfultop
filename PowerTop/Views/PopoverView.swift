import SwiftUI

struct PopoverView: View {
    static let preferredWidth: CGFloat = 280

    let monitor: PowerMonitor
    var openDetails: (() -> Void)? = nil
    @Environment(\.openWindow) private var openWindow

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

    private var batteryPercent: Int { min(100, max(0, data.batteryPercent)) }

    private var levelTint: Color {
        if data.isBatteryCharging { return .green }
        if batteryPercent <= 20 { return .red }
        return batteryPercent <= 50 ? .orange : .green
    }

    private var statusSymbol: String {
        if data.isConnectingAC { return "powerplug" }
        if data.isBatteryCharging { return "bolt.fill" }
        if data.isSupplementalDischarge { return "exclamationmark.triangle.fill" }
        return data.effectiveIsOnAC ? "powerplug.fill" : MenuBarLabel.batteryIconName(for: batteryPercent)
    }

    private var statusTint: Color {
        if data.isConnectingAC { return .secondary }
        if data.isSupplementalDischarge { return .orange }
        if data.effectiveIsOnAC { return .green }
        return batteryPercent <= 20 ? .red : .secondary
    }

    private var statusLine: String {
        guard let time = data.estimatedTimeRemainingText else { return data.powerSourceDescription }
        return "\(data.powerSourceDescription) · \(data.estimatedTimeRemainingLabel) \(time)"
    }

    private var headerSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("\(batteryPercent)%")
                .font(.system(size: 30, weight: .bold, design: .rounded))
                .monospacedDigit()

            LevelBar(fraction: Double(batteryPercent) / 100, tint: levelTint)

            HStack(alignment: .firstTextBaseline, spacing: 5) {
                Image(systemName: statusSymbol)
                    .foregroundStyle(statusTint)
                Text(statusLine)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .font(.system(size: 12))

            if let reason = data.notChargingReasonDescription,
               data.effectiveIsOnAC, !data.isConnectingAC, !data.isBatteryCharging {
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

    private var sourceNodes: [FlowNode] {
        if data.isConnectingAC {
            return [FlowNode(icon: "powerplug", title: String(localized: "AC Connecting"), value: nil, tint: .secondary)]
        }
        var nodes: [FlowNode] = []
        if data.effectiveIsOnAC {
            nodes.append(FlowNode(
                icon: "powerplug.fill",
                title: String(localized: "Charger"),
                value: watts(data.effectiveACOutputW),
                capacity: data.acAdapterWattage > 0 ? "\(data.acAdapterWattage) W" : nil,
                tint: .green
            ))
        }
        if data.isSupplementalDischarge {
            nodes.append(FlowNode(icon: "battery.50", title: String(localized: "Battery"), value: watts(data.batterySupplementalW), tint: .orange))
        } else if !data.effectiveIsOnAC {
            nodes.append(FlowNode(icon: MenuBarLabel.batteryIconName(for: batteryPercent), title: String(localized: "Battery"), value: nil, tint: .orange))
        }
        return nodes
    }

    private var destinationNodes: [FlowNode] {
        var nodes = [FlowNode(icon: "laptopcomputer", title: String(localized: "System"), value: watts(data.systemPowerW), tint: .primary)]
        if data.isBatteryCharging {
            nodes.append(FlowNode(icon: "battery.100.bolt", title: String(localized: "Battery"), value: watts(data.batteryChargeRateW), tint: .blue))
        }
        return nodes
    }

    private var powerFlowSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 6) {
                VStack(spacing: 6) {
                    ForEach(sourceNodes) { FlowNodeView(node: $0) }
                }
                Image(systemName: "arrow.right")
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundStyle(.tertiary)
                VStack(spacing: 6) {
                    ForEach(destinationNodes) { FlowNodeView(node: $0) }
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
        data.batteryHealthPercent != nil || data.batteryTemperatureC != nil || data.cycleCount != nil
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
            if let temp = data.batteryTemperatureC {
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

    // MARK: - Footer

    private var footerSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 2) {
                Button {
                    openDetailWindow()
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
            Toggle(String(localized: "Show Power in Menu Bar"), isOn: Binding(
                get: { monitor.showPowerInMenuBar },
                set: { monitor.showPowerInMenuBar = $0 }
            ))
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

    private func openDetailWindow() {
        if let openDetails {
            openDetails()
            return
        }
        let detailTitle = String(localized: "PowerTop Details")
        for window in NSApp.windows {
            if window.title == detailTitle {
                window.makeKeyAndOrderFront(nil)
                NSApp.activate(ignoringOtherApps: true)
                return
            }
        }
        openWindow(id: "detail")
        NSApp.activate(ignoringOtherApps: true)
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
