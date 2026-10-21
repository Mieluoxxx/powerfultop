import AppKit
import SwiftUI

struct DetailWindowView: View {
    static let preferredWidth: CGFloat = 920
    static let preferredHeight: CGFloat = 700
    static let minimumWidth: CGFloat = 620

    let monitor: PowerMonitor

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
    private var dashboardCards: [MasonryItem] {
        var cards = [
            MasonryItem(id: "level", weight: 5, view: AnyView(levelCard)),
            MasonryItem(id: "battery", weight: 7, view: AnyView(batteryCard)),
            MasonryItem(id: "health", weight: 8, view: AnyView(healthCard)),
        ]
        if data.hasSettledAC {
            cards.append(MasonryItem(id: "adapter", weight: 8, view: AnyView(adapterCard)))
            cards.append(MasonryItem(id: "charging", weight: 5, view: AnyView(chargingCard)))
        }
        if hasCellData {
            let rows = cellDisplayEntries.count * 2 + (data.batteryParallelCellCurrents?.count ?? 0) + 3
            cards.append(MasonryItem(id: "cells", weight: Double(rows), view: AnyView(cellsCard)))
        }
        if hasLifetimeData {
            cards.append(MasonryItem(id: "lifetime", weight: 9, view: AnyView(lifetimeCard)))
        }
        cards.append(MasonryItem(id: "device", weight: 5, view: AnyView(deviceCard)))
        return cards
    }

    // MARK: - Power Strip

    private var powerStrip: some View {
        HStack(spacing: 0) {
            if data.hasSettledAC {
                StripItem(icon: "powerplug.fill", tint: .green, label: String(localized: "Charger"), value: watts(data.effectiveACOutputW))
                StripDivider()
            }
            StripItem(icon: "laptopcomputer", tint: .primary, label: String(localized: "System"), value: watts(data.systemPowerW))
            StripDivider()
            StripItem(icon: batteryStripIcon, tint: batteryStripTint, label: String(localized: "Battery"), value: data.signedBatteryPowerText)
            if let time = data.estimatedTimeRemainingText {
                StripDivider()
                StripItem(icon: "clock", tint: .secondary, label: data.estimatedTimeRemainingLabel, value: time)
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

    private var levelCard: some View {
        DashCard(title: String(localized: "Battery Level"), icon: data.batteryLevelSymbol, tint: data.batteryLevelTint) {
            CardHero(value: "\(data.displayBatteryPercent)%", size: 38)
            LevelBar(fraction: Double(data.displayBatteryPercent) / 100, tint: data.batteryLevelTint)
            Text(data.powerSourceDescription)
                .font(.system(size: 12))
                .foregroundStyle(.secondary)
            if let time = data.estimatedTimeRemainingText {
                CardRow(label: data.estimatedTimeRemainingLabel, value: time, emphasized: true)
            }
            if data.chargeLimitPercent < 100 {
                CardRow(label: String(localized: "Charge Limit"), value: chargeLimitText)
            }
            if let footnote = data.estimatedTimeFootnote {
                CardNote(footnote)
            }
            if data.dataSource == .legacy {
                CardNote(String(localized: "Estimation Mode — telemetry unavailable"))
            }
        }
    }

    private var batteryCard: some View {
        DashCard(title: String(localized: "Battery"), icon: "bolt.fill", tint: .yellow) {
            CardHero(value: data.signedBatteryPowerText, caption: String(localized: "Battery Power"))
            if let amp = data.batteryAmperageMA {
                CardRow(label: String(localized: "Battery Current"), value: currentText(amp))
            }
            if let voltage = data.batteryVoltageMV {
                CardRow(label: String(localized: "Battery Voltage"), value: String(format: "%.2f V", Double(voltage) / 1000.0))
            }
            CardRow(label: String(localized: "System Power"), value: watts(data.systemPowerW), emphasized: true)
            if let avg = data.averageSystemPowerW, avg > 0.5 {
                CardRow(label: String(localized: "Average System Power"), value: watts(avg))
            }
            if let remaining = data.remainingCapacityMAH {
                CardRow(label: String(localized: "Remaining Charge"), value: "\(remaining) mAh")
            }
            if let instant = data.instantAmperageMA {
                CardRow(label: String(localized: "Battery Instant Current"), value: currentText(instant))
            }
        }
    }

    private var healthCard: some View {
        DashCard(title: String(localized: "Battery Health"), icon: "heart.fill", tint: healthColor) {
            if let health = data.batteryHealthPercent {
                CardHero(value: "\(health)%", caption: String(localized: "Health"))
            }
            capacityRow(String(localized: "Design Capacity"), data.designCapacityMAH, tint: .secondary)
            capacityRow(String(localized: "Current Full Charge"), data.fullChargeCapacityMAH ?? data.rawMaxCapacityMAH, tint: healthColor)
            capacityRow(String(localized: "Calibrated Full Charge"), data.nominalChargeCapacityMAH, tint: healthColor)
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

    private var adapterCard: some View {
        DashCard(title: String(localized: "AC Adapter"), icon: "powerplug.fill", tint: .green) {
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
            if let description = data.adapterDescription {
                CardNote(description)
            }
        }
    }

    private var chargingCard: some View {
        DashCard(title: String(localized: "Charging Status"), icon: "battery.100.bolt", tint: .blue) {
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
            if data.chargeLimitPercent < 100 {
                CardRow(label: String(localized: "Charge Limit"), value: chargeLimitText)
            }
            if let minSoc = data.dailyMinSoc, let maxSoc = data.dailyMaxSoc {
                CardRow(label: String(localized: "Optimized Charging Range"), value: "\(minSoc)% – \(maxSoc)%")
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
        }
    }

    private var cellsCard: some View {
        DashCard(title: String(localized: "Cell Data"), icon: "square.stack.3d.up", tint: .cyan) {
            if let topology = cellTopologyDescription {
                CardNote(topology)
            }
            if let cells = data.cellVoltagesMV, let balance = seriesVoltageBalanceSummary(cells) {
                CardRow(label: String(localized: "Series Voltage Balance"), value: balance)
            }
            ForEach(Array(cellDisplayEntries.enumerated()), id: \.offset) { idx, entry in
                CardBarRow(
                    label: usesSeriesParallelLayout
                        ? String(format: String(localized: "Series Group %d"), idx + 1)
                        : String(format: String(localized: "Cell %d"), idx + 1),
                    value: cellSummary(voltageMV: entry.voltageMV, qmaxMAH: entry.qmaxMAH),
                    fraction: entry.voltageMV.map(cellVoltageFraction),
                    tint: .cyan
                )
            }
            if usesSeriesParallelLayout, let currents = data.batteryParallelCellCurrents, !currents.isEmpty {
                if let balance = parallelCurrentBalanceSummary(currents) {
                    CardRow(label: String(localized: "Parallel Current Balance"), value: balance)
                }
                ForEach(Array(currents.enumerated()), id: \.offset) { _, cell in
                    CardRow(label: parallelCellLabel(bankID: cell.bankID, cellID: cell.cellID), value: "\(cell.currentMA) mA")
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
                CardRow(label: String(localized: "Protection Trigger Count"), value: protectionTriggerLabel(count: discCount))
            }
        }
    }

    private var deviceCard: some View {
        DashCard(title: String(localized: "Device Information"), icon: "info.circle", tint: .secondary) {
            if let serial = data.batterySerial {
                HStack(spacing: 6) {
                    VStack(alignment: .leading, spacing: 3) {
                        Text(String(localized: "Battery Serial Number"))
                            .foregroundStyle(.secondary)
                        Text(serial)
                            .font(.system(size: 12, weight: .medium, design: .monospaced))
                            .textSelection(.enabled)
                    }
                    .font(.system(size: 12))
                    Spacer(minLength: 0)
                    Button {
                        NSPasteboard.general.clearContents()
                        NSPasteboard.general.setString(serial, forType: .string)
                    } label: {
                        Image(systemName: "doc.on.doc")
                    }
                    .buttonStyle(.borderless)
                    .help(String(localized: "Copy"))
                }
            }
            if let name = data.deviceName {
                CardRow(label: String(localized: "Battery Management Chip"), value: name)
            }
            if let mfg = data.batteryManufactureDate {
                CardRow(label: String(localized: "Manufacture Date"), value: mfg)
            }
            CardRow(
                label: String(localized: "Data Source"),
                value: data.dataSource == .telemetry
                    ? String(localized: "Power Telemetry")
                    : String(localized: "Estimation Mode")
            )
            CardNote(String(localized: "Device info is shown locally only and is not uploaded."))
        }
    }

    private var footerTimestamp: some View {
        HStack {
            Spacer()
            Text(String(format: String(localized: "Updated at %@"), data.timestamp.formatted(date: .omitted, time: .standard)))
                .font(.system(size: 10))
                .foregroundStyle(.tertiary)
                .monospacedDigit()
        }
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
            let fraction = capacityFraction(value)
            CardBarRow(
                label: label,
                value: fraction.map { String(format: "%d mAh · %.0f%%", value, $0 * 100) } ?? "\(value) mAh",
                fraction: fraction,
                tint: tint
            )
        }
    }

    private func capacityFraction(_ value: Int?) -> Double? {
        guard let value, let design = data.designCapacityMAH, design > 0 else { return nil }
        return Double(value) / Double(design)
    }

    /// Raw IOKit amperage: positive = charging, negative = discharging.
    private func currentText(_ milliamps: Int) -> String {
        let direction = milliamps > 0
            ? String(localized: "Charging")
            : (milliamps < 0 ? String(localized: "Discharging") : String(localized: "Idle"))
        return String(format: "%.2f A (%@)", Double(abs(milliamps)) / 1000.0, direction)
    }

    /// Li-ion working window; only used to draw a comparable bar per cell.
    private func cellVoltageFraction(_ millivolts: Int) -> Double {
        (Double(millivolts) - 3000) / (4400 - 3000)
    }

    private var hasCellData: Bool {
        !cellDisplayEntries.isEmpty || !(data.batteryParallelCellCurrents?.isEmpty ?? true)
    }

    private var usesSeriesParallelLayout: Bool {
        if case .seriesParallel = data.batteryCellLayout { return true }
        return false
    }

    private var cellTopologyDescription: String? {
        switch data.batteryCellLayout {
        case .seriesParallel(let seriesCount, let parallelCount, let parallelCountKnown):
            if parallelCountKnown, parallelCount > 1 {
                return String(
                    format: String(localized: "Battery topology: %dS%dP (%d cells)"),
                    seriesCount, parallelCount, seriesCount * parallelCount
                )
            }
            return String(format: String(localized: "Battery topology: %d series groups"), seriesCount)
        case .perCellArrays:
            if let count = data.cellVoltagesMV?.count, count > 0 {
                return String(format: String(localized: "Battery topology: %d cells"), count)
            }
            return nil
        case nil:
            return nil
        }
    }

    private var cellDisplayEntries: [(voltageMV: Int?, qmaxMAH: Int?)] {
        let voltages = data.cellVoltagesMV ?? []
        let capacities = data.qmaxMAH ?? []
        let count = max(voltages.count, capacities.count)
        guard count > 0 else { return [] }
        return (0..<count).map { index in
            (
                voltageMV: index < voltages.count ? voltages[index] : nil,
                qmaxMAH: index < capacities.count ? capacities[index] : nil
            )
        }
    }

    private func cellSummary(voltageMV: Int?, qmaxMAH: Int?) -> String {
        var parts: [String] = []
        if let voltageMV {
            parts.append(String(format: "%.3f V", Double(voltageMV) / 1000.0))
        }
        if let qmaxMAH {
            parts.append("\(qmaxMAH) mAh")
        }
        return parts.isEmpty ? unavailableValue : parts.joined(separator: " · ")
    }

    private func parallelCellLabel(bankID: Int, cellID: Int) -> String {
        String(format: String(localized: "Series Group %d · Cell %d"), bankID + 1, cellID + 1)
    }

    private var hasLifetimeData: Bool {
        data.totalOperatingTimeMin != nil
            || data.lifetimeMaxTempC != nil
            || data.lifetimeMinTempC != nil
            || data.lifetimeAvgTempC != nil
            || data.lifetimeMaxPackVoltageMV != nil
            || data.lifetimeMinPackVoltageMV != nil
            || data.lifetimeMaxChargeCurrentMA != nil
            || data.lifetimeMaxDischargeCurrentMA != nil
            || data.batteryCellDisconnectCount != nil
    }

    private var unavailableValue: String {
        String(localized: "—")
    }

    private var chargeLimitText: String {
        if let source = chargeLimitSourceLabel(data.chargeLimitSource) {
            return "\(data.chargeLimitPercent)% (\(source))"
        }
        return "\(data.chargeLimitPercent)%"
    }

    private var healthColor: Color {
        guard let h = data.batteryHealthPercent else { return .secondary }
        if h >= 80 { return .green }
        if h >= 60 { return .orange }
        return .red
    }

    private func seriesVoltageBalanceSummary(_ voltagesMV: [Int]) -> String? {
        guard voltagesMV.count >= 2, let minMV = voltagesMV.min(), let maxMV = voltagesMV.max() else { return nil }
        let delta = maxMV - minMV
        return String(format: String(localized: "%d mV spread — %@"), delta, balanceStatus(forMillivoltSpread: delta))
    }

    private func parallelCurrentBalanceSummary(_ currents: [BatteryParallelCellCurrent]) -> String? {
        let grouped = Dictionary(grouping: currents, by: \.bankID)
        let spreads = grouped.values.compactMap { cells -> Int? in
            guard cells.count >= 2 else { return nil }
            let values = cells.map(\.currentMA)
            guard let minMA = values.min(), let maxMA = values.max() else { return nil }
            return maxMA - minMA
        }
        guard let worstSpread = spreads.max() else { return nil }
        return String(
            format: String(localized: "%d mA max within-group spread — %@"),
            worstSpread,
            balanceStatus(forMilliampSpread: worstSpread)
        )
    }

    private func balanceStatus(forMillivoltSpread delta: Int) -> String {
        if delta <= 50 { return String(localized: "Normal") }
        if delta <= 100 { return String(localized: "Fair") }
        return String(localized: "Worth Checking")
    }

    private func balanceStatus(forMilliampSpread delta: Int) -> String {
        if delta <= 80 { return String(localized: "Normal") }
        if delta <= 150 { return String(localized: "Fair") }
        return String(localized: "Worth Checking")
    }

    private func protectionTriggerLabel(count: Int) -> String {
        count == 0 ? String(localized: "0 (normal)") : "\(count)"
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
