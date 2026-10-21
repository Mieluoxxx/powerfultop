import AppKit
import SwiftUI

struct DetailWindowView: View {
    static let preferredWidth: CGFloat = 920
    static let preferredHeight: CGFloat = 700
    static let minimumWidth: CGFloat = 620

    let monitor: PowerMonitor
    let thermal: ThermalMonitor

    private var data: PowerData { monitor.currentData }

    var body: some View {
        Group {
            if monitor.isDataAvailable {
                GeometryReader { proxy in
                    ScrollView {
                        VStack(alignment: .leading, spacing: 16) {
                            powerStrip
                            MasonryColumns(
                                items: dashboardCards,
                                columnCount: proxy.size.width >= 880 ? 3 : 2,
                                spacing: 16
                            )
                            footerTimestamp
                        }
                        .padding(20)
                    }
                }
            } else {
                unavailableState
            }
        }
        .frame(minWidth: Self.minimumWidth, maxWidth: .infinity, minHeight: 480, maxHeight: .infinity, alignment: .top)
        .background(.windowBackground)
        .focusEffectDisabled()
        .onAppear {
            DispatchQueue.main.async {
                NSApp.keyWindow?.makeFirstResponder(nil)
            }
        }
    }

    /// Weights approximate card heights in rows so the masonry columns stay balanced.
    /// Only depth the popover lacks earns a card; anything it already shows lives in the strip.
    private var dashboardCards: [MasonryItem] {
        var cards: [MasonryItem] = []
        if thermal.snapshot.hasFans || !thermal.snapshot.temperatures.isEmpty {
            let weight = 6.0 + Double(thermal.snapshot.fans.count) + (thermal.mode.isAutomatic ? 0 : 4)
            cards.append(MasonryItem(id: "cooling", weight: weight, view: AnyView(CoolingCard(thermal: thermal))))
        }
        cards.append(MasonryItem(id: "health", weight: 10, view: AnyView(healthCard)))
        if data.hasSettledAC {
            cards.append(MasonryItem(id: "charger", weight: 11, view: AnyView(chargerCard)))
        }
        if data.hasCellData || data.batteryVoltageMV != nil {
            let rows = data.cellDisplayEntries.count * 2 + (data.batteryParallelCellCurrents?.count ?? 0) + 6
            cards.append(MasonryItem(id: "cells", weight: Double(rows), view: AnyView(cellsCard)))
        }
        if data.hasLifetimeData {
            cards.append(MasonryItem(id: "lifetime", weight: 9, view: AnyView(lifetimeCard)))
        }
        return cards
    }

    // MARK: - Power Strip

    private var powerStrip: some View {
        HStack(spacing: 0) {
            StripItem(
                icon: data.batteryLevelSymbol,
                tint: data.batteryLevelTint,
                label: "\(data.displayBatteryPercent)%",
                value: data.powerSourceDescription
            )
            StripDivider()
            if data.hasSettledAC {
                StripItem(icon: "powerplug.fill", tint: .green, label: String(localized: "Charger"), value: watts(data.effectiveACOutputW))
                StripDivider()
            }
            StripItem(icon: "laptopcomputer", tint: .primary, label: String(localized: "System"), value: watts(data.systemPowerW))
                .help(data.averageSystemPowerW.map {
                    String(format: String(localized: "Average System Power: %@"), watts($0))
                } ?? "")
            StripDivider()
            StripItem(icon: batteryStripIcon, tint: batteryStripTint, label: String(localized: "Battery"), value: data.signedBatteryPowerText)
            if let time = data.estimatedTimeRemainingText {
                StripDivider()
                StripItem(icon: "clock", tint: .secondary, label: data.estimatedTimeRemainingLabel, value: time)
                    .help(data.estimatedTimeFootnote ?? "")
            }
            if data.dataSource == .legacy {
                StripDivider()
                StripItem(icon: "exclamationmark.triangle.fill", tint: .yellow, label: String(localized: "Estimation Mode"), value: "")
                    .help(String(localized: "Estimation Mode — telemetry unavailable"))
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
        .background(Color.dashCard, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).strokeBorder(Color.dashCardBorder))
    }

    private var batteryStripIcon: String {
        data.isBatteryCharging ? "battery.100.bolt" : data.batteryLevelSymbol
    }

    private var batteryStripTint: Color {
        switch data.powerState {
        case .charging: return .blue
        case .onBattery, .supplementing: return .orange
        case .connectingAC, .acIdle: return .secondary
        }
    }

    // MARK: - Cards

    private var healthCard: some View {
        DashCard(title: String(localized: "Battery Health"), icon: "heart.fill", tint: healthColor) {
            if let health = data.batteryHealthPercent {
                CardHero(value: "\(health)%", caption: String(localized: "Health"))
            }
            capacityRow(String(localized: "Design Capacity"), data.designCapacityMAH, tint: .secondary)
            capacityRow(String(localized: "Current Full Charge"), data.fullChargeCapacityMAH ?? data.rawMaxCapacityMAH, tint: healthColor)
            capacityRow(String(localized: "Calibrated Full Charge"), data.nominalChargeCapacityMAH, tint: healthColor)
            capacityRow(String(localized: "Remaining Charge"), data.remainingCapacityMAH, tint: data.batteryLevelTint)
            HStack(spacing: 6) {
                CardRow(label: String(localized: "Charge Limit"), value: data.chargeLimitText)
                Button {
                    NSWorkspace.shared.open(macOSChargeLimitSettingsURL)
                } label: {
                    Image(systemName: "arrow.up.forward.app")
                }
                .buttonStyle(.borderless)
                .help(String(localized: "Adjust the charge limit (80–100%) in System Settings"))
            }
            if let minSoc = data.dailyMinSoc, let maxSoc = data.dailyMaxSoc {
                CardRow(label: String(localized: "Optimized Charging Range"), value: "\(minSoc)% – \(maxSoc)%")
            }
            if let cycles = data.cycleCount {
                CardRow(
                    label: String(localized: "Cycle Count"),
                    value: "\(cycles)",
                    rated: data.designCycleCount.map { "\($0)" }
                )
            }
            if let failure = data.permanentFailureStatus {
                CardRow(
                    label: String(localized: "Permanent Failure Status"),
                    value: failure == 0
                        ? String(localized: "Normal")
                        : String(format: String(localized: "Abnormal (%d)"), failure)
                )
            }
            if let critical = data.atCriticalLevel {
                CardRow(
                    label: String(localized: "Critical Low Battery"),
                    value: critical ? String(localized: "Yes") : String(localized: "No")
                )
            }
        }
    }

    private var chargerCard: some View {
        DashCard(title: String(localized: "Charger & Charging"), icon: "powerplug.fill", tint: .green) {
            CardHero(
                value: watts(data.effectiveACOutputW),
                rated: data.acAdapterWattage > 0 ? "\(data.acAdapterWattage) W" : nil,
                caption: String(localized: "AC Adapter Output")
            )
            if let sc = data.systemCurrentMA {
                CardRow(
                    label: String(localized: "Charger Output Current"),
                    value: String(format: "%.2f A", Double(sc) / 1000.0),
                    rated: data.adapterRatedCurrentMA.map { String(format: "%.1f A", Double($0) / 1000.0) }
                )
            }
            if let sv = data.systemVoltageMV {
                CardRow(
                    label: String(localized: "Charger Output Voltage"),
                    value: String(format: "%.2f V", Double(sv) / 1000.0),
                    rated: data.adapterRatedVoltageMV.map { String(format: "%.0f V", Double($0) / 1000.0) }
                )
            }
            if data.acAdapterWattage > 0 {
                let usage = data.effectiveACOutputW / Double(data.acAdapterWattage)
                CardBarRow(
                    label: String(localized: "Charger Load Rate"),
                    value: String(format: "%.0f%%", usage * 100),
                    fraction: usage,
                    tint: usage > 0.9 ? .orange : .green
                )
            }
            if let wall = data.wallPowerW {
                CardRow(label: String(localized: "Wall Outlet Power"), value: watts(wall))
            }
            if let loss = data.adapterEfficiencyLossW {
                CardRow(label: String(localized: "Adapter Loss"), value: watts(loss))
            }
            Divider().padding(.vertical, 2)
            CardRow(
                label: String(localized: "Status"),
                value: data.isBatteryCharging ? String(localized: "Charging") : String(localized: "Not Charging"),
                emphasized: true
            )
            if data.fullyCharged {
                CardRow(label: String(localized: "Fully Charged"), value: String(localized: "Yes"))
            }
            if let reason = data.notChargingReasonDescription {
                CardRow(label: String(localized: "Not Charging Reason"), value: reason)
            }
            if let cv = data.chargingVoltageMV {
                CardRow(label: String(localized: "Cell Charging Voltage"), value: String(format: "%.3f V", Double(cv) / 1000.0))
            }
            if let cc = data.chargingCurrentMA {
                CardRow(label: String(localized: "Charging Current"), value: String(format: "%d mA", cc))
            }
            if let vac = data.vacVoltageLimit {
                CardRow(label: String(localized: "Max Charging Voltage Limit"), value: String(format: "%.3f V", Double(vac) / 1000.0))
            }
            if let description = data.adapterDescription {
                CardNote(description)
            }
        }
    }

    private var cellsCard: some View {
        DashCard(title: String(localized: "Cells & Live Readings"), icon: "square.stack.3d.up", tint: .cyan) {
            if let voltage = data.batteryVoltageMV {
                CardRow(label: String(localized: "Battery Voltage"), value: String(format: "%.2f V", Double(voltage) / 1000.0))
            }
            if let amp = data.batteryAmperageMA {
                CardRow(label: String(localized: "Battery Current"), value: PowerData.batteryCurrentText(amp))
            }
            if let instant = data.instantAmperageMA {
                CardRow(label: String(localized: "Battery Instant Current"), value: PowerData.batteryCurrentText(instant))
            }
            if let topology = data.cellTopologyDescription {
                CardNote(topology)
            }
            if let cells = data.cellVoltagesMV, let balance = data.seriesVoltageBalanceSummary(cells) {
                CardRow(label: String(localized: "Series Voltage Balance"), value: balance)
            }
            ForEach(Array(data.cellDisplayEntries.enumerated()), id: \.offset) { idx, entry in
                CardBarRow(
                    label: data.usesSeriesParallelLayout
                        ? String(format: String(localized: "Series Group %d"), idx + 1)
                        : String(format: String(localized: "Cell %d"), idx + 1),
                    value: data.cellSummary(voltageMV: entry.voltageMV, qmaxMAH: entry.qmaxMAH),
                    fraction: entry.voltageMV.map(data.cellVoltageFraction),
                    tint: .cyan
                )
            }
            if data.usesSeriesParallelLayout, let currents = data.batteryParallelCellCurrents, !currents.isEmpty {
                if let balance = data.parallelCurrentBalanceSummary(currents) {
                    CardRow(label: String(localized: "Parallel Current Balance"), value: balance)
                }
                ForEach(Array(currents.enumerated()), id: \.offset) { _, cell in
                    CardRow(label: data.parallelCellLabel(bankID: cell.bankID, cellID: cell.cellID), value: "\(cell.currentMA) mA")
                }
                CardNote(String(localized: "Instantaneous current per parallel cell; unequal split within a group is normal."))
            }
        }
    }

    private var lifetimeCard: some View {
        DashCard(title: String(localized: "Lifetime Statistics"), icon: "clock.arrow.circlepath", tint: .purple) {
            if let total = data.totalOperatingTimeMin {
                CardRow(label: String(localized: "Total Operating Time"), value: "\(total / 60) \(String(localized: "hours"))")
            }
            if let maxT = data.lifetimeMaxTempC {
                CardRow(label: String(localized: "Battery Max Temperature"), value: "\(TemperatureUnits.lifetimeCelsius(fromWholeDegrees: maxT)) °C")
            }
            if let minT = data.lifetimeMinTempC {
                CardRow(label: String(localized: "Battery Min Temperature"), value: "\(TemperatureUnits.lifetimeCelsius(fromWholeDegrees: minT)) °C")
            }
            if let avgT = data.lifetimeAvgTempC {
                CardRow(
                    label: String(localized: "Battery Avg Temperature"),
                    value: String(format: "%.1f °C", TemperatureUnits.lifetimeAvgCelsius(fromDecidegrees: avgT))
                )
            }
            if let maxV = data.lifetimeMaxPackVoltageMV {
                CardRow(label: String(localized: "Max Pack Voltage"), value: String(format: "%.3f V", Double(maxV) / 1000.0))
            }
            if let minV = data.lifetimeMinPackVoltageMV {
                CardRow(label: String(localized: "Min Pack Voltage"), value: String(format: "%.3f V", Double(minV) / 1000.0))
            }
            if let maxCharge = data.lifetimeMaxChargeCurrentMA, maxCharge < 100_000 {
                CardRow(label: String(localized: "Max Charging Current"), value: "\(maxCharge) mA")
            }
            if let maxDischarge = data.lifetimeMaxDischargeCurrentMA, maxDischarge < 100_000 {
                CardRow(label: String(localized: "Max Discharging Current"), value: "\(maxDischarge) mA")
            }
            if let discCount = data.batteryCellDisconnectCount {
                CardRow(label: String(localized: "Protection Trigger Count"), value: data.protectionTriggerLabel(count: discCount))
            }
        }
    }

    private func celsius(_ value: Double) -> String {
        String(format: "%.1f°C", value)
    }

    /// Device facts are rarely needed, so they share the footer line instead of a card.
    private var footerTimestamp: some View {
        HStack(spacing: 6) {
            Text([data.deviceName, data.batteryManufactureDate.map { String(format: String(localized: "Manufactured %@"), $0) }]
                .compactMap { $0 }
                .joined(separator: " · "))
            if let serial = data.batterySerial {
                Text("·")
                Text(serial)
                    .font(.system(size: 10, design: .monospaced))
                    .textSelection(.enabled)
                Button {
                    NSPasteboard.general.clearContents()
                    NSPasteboard.general.setString(serial, forType: .string)
                } label: {
                    Image(systemName: "doc.on.doc")
                }
                .buttonStyle(.borderless)
                .help(String(localized: "Copy serial number. Device info is shown locally only and is not uploaded."))
            }
            Spacer()
            Text(String(format: String(localized: "Updated at %@"), data.timestamp.formatted(date: .omitted, time: .standard)))
                .monospacedDigit()
        }
        .font(.system(size: 10))
        .foregroundStyle(.tertiary)
    }

    private var unavailableState: some View {
        VStack(spacing: 12) {
            Image(systemName: "exclamationmark.triangle.fill")
                .font(.largeTitle)
                .foregroundStyle(.orange)
            Text(String(localized: "Built-in battery not detected. PowerTop requires a MacBook."))
                .font(.system(size: 13))
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 40)
    }

    // MARK: - Helpers

    private func watts(_ value: Double) -> String {
        String(format: "%.1f W", value)
    }

    @ViewBuilder
    private func capacityRow(_ label: String, _ value: Int?, tint: Color) -> some View {
        if let value {
            let fraction = data.capacityFractionOfDesign(value)
            CardBarRow(
                label: label,
                value: fraction.map { String(format: "%d mAh · %.0f%%", value, $0 * 100) } ?? "\(value) mAh",
                fraction: fraction,
                tint: tint
            )
        }
    }

    private var unavailableValue: String {
        String(localized: "—")
    }

    private var healthColor: Color {
        guard let h = data.batteryHealthPercent else { return .secondary }
        if h >= 80 { return .green }
        if h >= 60 { return .orange }
        return .red
    }

}

// MARK: - Fan control

/// Mirrors Macs Fan Control: Auto hands fans to macOS; Custom is a constant RPM or a temperature ramp.
private struct CoolingCard: View {
    let thermal: ThermalMonitor
    @State private var draftRPM: Double?

    private enum Preset: Hashable { case automatic, fullBlast, custom }

    private var snapshot: ThermalSnapshot { thermal.snapshot }
    private var minRPM: Double { snapshot.fans.map(\.minRPM).max() ?? 0 }
    private var maxRPM: Double { snapshot.fans.map(\.maxRPM).min() ?? 0 }

    private var preset: Preset {
        switch thermal.mode {
        case .automatic: return .automatic
        case .fullBlast: return .fullBlast
        case .constant, .sensor: return .custom
        }
    }

    var body: some View {
        DashCard(title: String(localized: "Cooling"), icon: "fan.fill", tint: .teal) {
            if let cpu = snapshot.temperatures[.cpu] {
                CardHero(value: String(format: "%.1f°C", cpu.averageC), caption: String(localized: "CPU Average"))
            }
            ForEach(SensorGroup.allCases) { group in
                if let summary = snapshot.temperatures[group] {
                    CardBarRow(
                        label: group.title,
                        value: String(
                            format: String(localized: "%@ avg · %@ max"),
                            String(format: "%.1f°C", summary.averageC),
                            String(format: "%.1f°C", summary.maximumC)
                        ),
                        fraction: summary.maximumC / FanHelperConstants.failsafeCelsius,
                        tint: summary.maximumC >= 90 ? .red : (summary.maximumC >= 75 ? .orange : .green)
                    )
                }
            }
            if snapshot.hasFans {
                fanSection
            }
        }
    }

    @ViewBuilder
    private var fanSection: some View {
            Divider().padding(.vertical, 2)
            CardRow(
                label: thermal.isControlling ? String(localized: "Controlled by PowerTop") : String(localized: "Controlled by macOS"),
                value: String(format: "%.0f RPM", snapshot.fans.map(\.actualRPM).max() ?? 0),
                emphasized: true
            )
            ForEach(snapshot.fans) { fan in
                CardBarRow(
                    label: String(format: String(localized: "Fan %d"), fan.index + 1),
                    value: String(format: "%.0f / %.0f RPM", fan.actualRPM, fan.maxRPM),
                    fraction: fan.maxRPM > 0 ? fan.actualRPM / fan.maxRPM : nil,
                    tint: .teal
                )
            }

            Picker("", selection: Binding(get: { preset }, set: select)) {
                Text(String(localized: "Auto")).tag(Preset.automatic)
                Text(String(localized: "Full Blast")).tag(Preset.fullBlast)
                Text(String(localized: "Custom")).tag(Preset.custom)
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .padding(.top, 4)

            if preset == .custom {
                customControls
            }
            statusNotes
    }

    private func select(_ newPreset: Preset) {
        switch newPreset {
        case .automatic: thermal.setMode(.automatic)
        case .fullBlast: thermal.setMode(.fullBlast)
        case .custom: thermal.setMode(thermal.lastCustomMode)
        }
    }

    @ViewBuilder
    private var customControls: some View {
        let isSensor: Bool = { if case .sensor = thermal.mode { return true }; return false }()
        Picker("", selection: Binding(
            get: { isSensor },
            set: { thermal.setMode($0 ? sensorMode : constantMode) }
        )) {
            Text(String(localized: "Constant RPM")).tag(false)
            Text(String(localized: "Follow Temperature")).tag(true)
        }
        .pickerStyle(.segmented)
        .labelsHidden()

        switch thermal.mode {
        case .constant(let rpm):
            let shown = draftRPM ?? rpm
            CardRow(label: String(localized: "Target Speed"), value: String(format: "%.0f RPM", shown), emphasized: true)
            Slider(
                value: Binding(get: { draftRPM ?? rpm }, set: { draftRPM = $0 }),
                in: minRPM...max(maxRPM, minRPM + 100),
                step: 100
            ) { editing in
                if !editing, let draftRPM {
                    thermal.setMode(.constant(rpm: draftRPM))
                    self.draftRPM = nil
                }
            }
        case .sensor(let group, let lowerC, let upperC):
            Picker(String(localized: "Sensor"), selection: Binding(
                get: { group },
                set: { thermal.setMode(.sensor(group: $0, lowerC: lowerC, upperC: upperC)) }
            )) {
                ForEach([SensorGroup.cpu, .gpu]) { Text($0.title).tag($0) }
            }
            .font(.system(size: 12))
            Stepper(
                String(format: String(localized: "Start ramping at %.0f°C"), lowerC),
                value: Binding(get: { lowerC }, set: { thermal.setMode(.sensor(group: group, lowerC: $0, upperC: max(upperC, $0 + 5))) }),
                in: 30...90,
                step: 5
            )
            .font(.system(size: 12))
            Stepper(
                String(format: String(localized: "Full speed at %.0f°C"), upperC),
                value: Binding(get: { upperC }, set: { thermal.setMode(.sensor(group: group, lowerC: min(lowerC, $0 - 5), upperC: $0)) }),
                in: 40...100,
                step: 5
            )
            .font(.system(size: 12))
        default:
            EmptyView()
        }
    }

    private var constantMode: FanControlMode {
        if case .constant = thermal.lastCustomMode { return thermal.lastCustomMode }
        return .defaultConstant
    }

    private var sensorMode: FanControlMode {
        if case .sensor = thermal.lastCustomMode { return thermal.lastCustomMode }
        return .defaultSensor
    }

    @ViewBuilder
    private var statusNotes: some View {
        if thermal.conflictDetected && !thermal.isControlling {
            CardNote(String(localized: "Another fan tool (such as Macs Fan Control) is controlling the fans. Quit it to let PowerTop take over; until then PowerTop only monitors."))
        } else if !thermal.mode.isAutomatic && thermal.helperStatus != .ready {
            CardNote(thermal.helperStatus == .needsUpdate
                ? String(localized: "PowerTop was updated, so the fan helper must be authorized again.")
                : String(localized: "Controlling fans needs a small helper that runs as root. macOS asks for an administrator password once."))
            Button(thermal.helperStatus == .needsUpdate ? String(localized: "Update Fan Helper") : String(localized: "Install Fan Helper")) {
                thermal.installHelper()
            }
            .controlSize(.small)
        } else if thermal.mode.isAutomatic && snapshot.fans.allSatisfy({ $0.actualRPM == 0 }) {
            CardNote(String(localized: "macOS stops the fans while temperatures are low; they spin up on their own when cooling is needed."))
        }
        if let error = thermal.lastError {
            Text(error)
                .font(.system(size: 10))
                .foregroundStyle(.red)
                .fixedSize(horizontal: false, vertical: true)
        }
        if !thermal.mode.isAutomatic {
            CardNote(String(format: String(localized: "Fans return to macOS control when PowerTop quits, the Mac sleeps, or the CPU or GPU reaches %.0f°C."), FanHelperConstants.failsafeCelsius))
        }
    }
}

// MARK: - Components

private struct StripItem: View {
    let icon: String
    let tint: Color
    let label: String
    let value: String

    var body: some View {
        HStack(spacing: 6) {
            Image(systemName: icon)
                .foregroundStyle(tint)
            Text(label)
                .foregroundStyle(.secondary)
                .lineLimit(1)
            Text(value)
                .fontWeight(.semibold)
                .monospacedDigit()
                .lineLimit(1)
        }
        .font(.system(size: 12))
        .padding(.horizontal, 10)
        .fixedSize()
    }
}

private struct StripDivider: View {
    var body: some View {
        Divider().frame(height: 14)
    }
}

private extension Color {
    /// Raised card surface: white on the light window background, a faint lift in dark mode.
    static let dashCard = Color(nsColor: NSColor(name: nil) { appearance in
        appearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua
            ? NSColor(white: 1, alpha: 0.055)
            : NSColor(white: 1, alpha: 0.85)
    })

    static let dashCardBorder = Color.primary.opacity(0.07)
}

private struct DashCard<Content: View>: View {
    let title: String
    let icon: String
    let tint: Color
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 8) {
                Image(systemName: icon)
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundStyle(tint)
                    .frame(width: 24, height: 24)
                    .background(tint.opacity(0.16), in: RoundedRectangle(cornerRadius: 7, style: .continuous))
                Text(title)
                    .font(.system(size: 13, weight: .semibold))
            }
            .padding(.bottom, 2)

            content
        }
        .padding(18)
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .background(Color.dashCard, in: RoundedRectangle(cornerRadius: 18, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 18, style: .continuous).strokeBorder(Color.dashCardBorder))
    }
}

/// Lead figure of a card, e.g. "48.2 W / 90 W" with a caption underneath.
private struct CardHero: View {
    let value: String
    var rated: String? = nil
    var caption: String? = nil
    var size: CGFloat = 28

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(alignment: .firstTextBaseline, spacing: 4) {
                Text(value)
                    .font(.system(size: size, weight: .bold, design: .rounded))
                if let rated {
                    Text("/ \(rated)")
                        .font(.system(size: size * 0.5, weight: .medium, design: .rounded))
                        .foregroundStyle(.tertiary)
                }
            }
            .monospacedDigit()
            .lineLimit(1)
            .minimumScaleFactor(0.7)
            if let caption {
                Text(caption)
                    .font(.system(size: 11))
                    .foregroundStyle(.secondary)
            }
        }
        .padding(.bottom, 2)
    }
}

/// "Label    value / rated" — rated is the adapter or design limit the value is measured against.
private struct CardRow: View {
    let label: String
    let value: String
    var rated: String? = nil
    var emphasized: Bool = false

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Text(label)
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .layoutPriority(1)
            Spacer(minLength: 8)
            HStack(alignment: .firstTextBaseline, spacing: 3) {
                Text(value)
                    .fontWeight(emphasized ? .semibold : .medium)
                    .foregroundStyle(.primary)
                if let rated {
                    Text("/ \(rated)")
                        .foregroundStyle(.tertiary)
                }
            }
            .monospacedDigit()
            .lineLimit(1)
            .truncationMode(.middle)
            .textSelection(.enabled)
        }
        .font(.system(size: 12))
        .frame(maxWidth: .infinity)
    }
}

private struct CardBarRow: View {
    let label: String
    let value: String
    let fraction: Double?
    let tint: Color

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            CardRow(label: label, value: value)
            LevelBar(fraction: fraction ?? 0, tint: tint, height: 5)
                .opacity(fraction == nil ? 0 : 1)
        }
    }
}

private struct CardNote: View {
    let text: String

    init(_ text: String) { self.text = text }

    var body: some View {
        Text(text)
            .font(.system(size: 10))
            .foregroundStyle(.tertiary)
            .fixedSize(horizontal: false, vertical: true)
    }
}

private struct MasonryItem: Identifiable {
    let id: String
    let weight: Double
    let view: AnyView
}

/// Fills each card into the currently shortest column, keeping source order within a column.
private struct MasonryColumns: View {
    let items: [MasonryItem]
    let columnCount: Int
    let spacing: CGFloat

    var body: some View {
        HStack(alignment: .top, spacing: spacing) {
            ForEach(Array(columns.enumerated()), id: \.offset) { _, column in
                VStack(spacing: spacing) {
                    ForEach(column) { $0.view }
                }
                .frame(maxWidth: .infinity, alignment: .top)
            }
        }
    }

    private var columns: [[MasonryItem]] {
        var result = Array(repeating: [MasonryItem](), count: max(columnCount, 1))
        var heights = Array(repeating: 0.0, count: result.count)
        for item in items {
            let target = heights.indices.min { heights[$0] < heights[$1] } ?? 0
            result[target].append(item)
            heights[target] += item.weight
        }
        return result
    }
}
