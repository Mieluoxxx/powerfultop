import AppKit
import SwiftUI

struct DetailWindowView: View {
    static let preferredWidth: CGFloat = 480

    enum Tab: String, CaseIterable, Identifiable {
        case power, battery, cells, device
        var id: String { rawValue }

        var title: String {
            switch self {
            case .power: return String(localized: "Power")
            case .battery: return String(localized: "Battery")
            case .cells: return String(localized: "Cells")
            case .device: return String(localized: "Device")
            }
        }
    }

    let monitor: PowerMonitor
    @AppStorage("detailWindowTab") private var selectedTab: Tab = .power

    private var data: PowerData { monitor.currentData }

    var body: some View {
        VStack(spacing: 0) {
            if monitor.isDataAvailable {
                overviewTiles
                    .padding(.horizontal, 20)
                    .padding(.top, 16)

                Picker("", selection: $selectedTab) {
                    ForEach(availableTabs) { tab in
                        Text(tab.title).tag(tab)
                    }
                }
                .pickerStyle(.segmented)
                .labelsHidden()
                .padding(.horizontal, 20)
                .padding(.vertical, 14)

                Divider()

                ScrollView {
                    VStack(alignment: .leading, spacing: 14) {
                        switch currentTab {
                        case .power: powerTab
                        case .battery: batteryTab
                        case .cells: cellsTab
                        case .device: deviceTab
                        }
                        footerTimestamp
                    }
                    .padding(20)
                }
            } else {
                unavailableState
            }
        }
        .frame(width: Self.preferredWidth)
        .frame(minHeight: 500, maxHeight: .infinity, alignment: .top)
        .background(.windowBackground)
        .focusEffectDisabled()
        .onAppear {
            DispatchQueue.main.async {
                NSApp.keyWindow?.makeFirstResponder(nil)
            }
        }
    }

    private var availableTabs: [Tab] {
        Tab.allCases.filter { $0 != .cells || hasCellData }
    }

    private var currentTab: Tab {
        availableTabs.contains(selectedTab) ? selectedTab : .power
    }

    // MARK: - Overview

    private var overviewTiles: some View {
        HStack(spacing: 8) {
            OverviewTile(
                value: "\(batteryLevelPercent)%",
                label: String(localized: "Battery Level"),
                tint: batteryLevelBarTint,
                fraction: Double(batteryLevelPercent) / 100
            )
            OverviewTile(
                value: data.batteryHealthPercent.map { "\($0)%" } ?? unavailableValue,
                label: String(localized: "Health"),
                tint: healthColor,
                fraction: data.batteryHealthPercent.map { Double($0) / 100 }
            )
            OverviewTile(
                value: data.cycleCount.map(String.init) ?? unavailableValue,
                caption: data.designCycleCount.map { "/ \($0)" },
                label: String(localized: "Cycle Count"),
                tint: .secondary,
                fraction: cycleCountFraction
            )
            if let temp = data.batteryTemperatureC {
                OverviewTile(
                    value: String(format: "%.1f°", temp),
                    label: String(localized: "Battery Temp"),
                    tint: temp > 40 ? .red : .secondary,
                    fraction: nil
                )
            }
        }
    }

    // MARK: - Power Tab

    @ViewBuilder
    private var powerTab: some View {
        DetailSection(title: String(localized: "Current Power"), icon: "bolt.fill", color: .green) {
            DetailRow(label: String(localized: "Power Source"), value: data.powerSourceDescription)
            DetailRow(
                label: String(localized: "System Power"),
                value: watts(data.systemPowerW),
                highlight: true
            )

            if data.effectiveIsOnAC && !data.isConnectingAC {
                DetailRow(label: String(localized: "AC Adapter Output"), value: watts(data.effectiveACOutputW))
            }

            if data.isBatteryCharging {
                DetailRow(label: String(localized: "Battery Charging"), value: watts(data.batteryChargeRateW))
            } else if data.isSupplementalDischarge {
                DetailRow(label: String(localized: "Battery Supplement"), value: watts(data.batterySupplementalW))
            } else if !data.effectiveIsOnAC, data.batteryPowerW > 0.3 {
                DetailRow(label: String(localized: "Battery Discharging Power"), value: watts(data.batteryPowerW))
            } else if data.effectiveIsOnAC, data.batteryPowerW < -0.3 {
                DetailRow(label: String(localized: "Battery Charging Power"), value: watts(abs(data.batteryPowerW)))
            }

            if let timeText = data.estimatedTimeRemainingText {
                DetailRow(label: data.estimatedTimeRemainingLabel, value: timeText)
                if let footnote = data.estimatedTimeFootnote {
                    DetailNote(footnote)
                }
            }

            if let avg = data.averageSystemPowerW, avg > 0.5 {
                DetailRow(label: String(localized: "Average System Power"), value: watts(avg))
            }

            if data.dataSource == .legacy {
                DetailRow(
                    label: String(localized: "Data Source"),
                    value: String(localized: "Estimation Mode — telemetry unavailable")
                )
            }
        }

        if data.effectiveIsOnAC && !data.isConnectingAC {
            DetailSection(title: String(localized: "AC Adapter"), icon: "powerplug.fill", color: .green) {
                if data.acAdapterWattage > 0 {
                    DetailRow(
                        label: String(localized: "Charger Spec"),
                        value: "\(data.acAdapterWattage) W" + (data.adapterDescription.map { " (\($0))" } ?? "")
                    )
                    let usage = data.effectiveACOutputW / Double(data.acAdapterWattage) * 100
                    BarRow(
                        label: String(localized: "Charger Load Rate"),
                        value: String(format: "%.0f%%", usage),
                        fraction: usage / 100,
                        tint: usage > 90 ? .orange : .green
                    )
                }
                if let wall = data.wallPowerW {
                    DetailRow(label: String(localized: "Wall Outlet Power"), value: watts(wall))
                }
                if let loss = data.adapterEfficiencyLossW {
                    DetailRow(label: String(localized: "Adapter Loss"), value: watts(loss))
                }
                if let sv = data.systemVoltageMV {
                    DetailRow(
                        label: String(localized: "Charger Output Voltage"),
                        value: String(format: "%.2f V", Double(sv) / 1000.0)
                    )
                }
                if let sc = data.systemCurrentMA {
                    DetailRow(
                        label: String(localized: "Charger Output Current"),
                        value: String(format: "%.3f A", Double(sc) / 1000.0)
                    )
                }
            }

            DetailSection(title: String(localized: "Charging Status"), icon: "battery.100.bolt", color: .blue) {
                DetailRow(
                    label: String(localized: "Status"),
                    value: data.isBatteryCharging ? String(localized: "Charging") : String(localized: "Not Charging")
                )
                if data.fullyCharged {
                    DetailRow(label: String(localized: "Fully Charged"), value: String(localized: "Yes"))
                }
                if let reason = data.notChargingReasonDescription {
                    DetailRow(label: String(localized: "Not Charging Reason"), value: reason)
                }
                if let cv = data.chargingVoltageMV {
                    DetailRow(
                        label: String(localized: "Cell Charging Voltage"),
                        value: String(format: "%.3f V", Double(cv) / 1000.0)
                    )
                }
                if let cc = data.chargingCurrentMA {
                    DetailRow(label: String(localized: "Charging Current"), value: String(format: "%d mA", cc))
                }
                if let vac = data.vacVoltageLimit {
                    DetailRow(
                        label: String(localized: "Max Charging Voltage Limit"),
                        value: String(format: "%.3f V", Double(vac) / 1000.0)
                    )
                }
            }
        }
    }

    // MARK: - Battery Tab

    @ViewBuilder
    private var batteryTab: some View {
        DetailSection(title: String(localized: "Capacity"), icon: "battery.100", color: healthColor) {
            capacityRow(String(localized: "Design Capacity"), data.designCapacityMAH, tint: .secondary)
            capacityRow(
                String(localized: "Current Full Charge"),
                data.fullChargeCapacityMAH ?? data.rawMaxCapacityMAH,
                tint: healthColor
            )
            capacityRow(String(localized: "Calibrated Full Charge"), data.nominalChargeCapacityMAH, tint: healthColor)
            capacityRow(String(localized: "Remaining Charge"), data.remainingCapacityMAH, tint: batteryLevelBarTint)
            if data.designCapacityMAH != nil {
                DetailNote(String(localized: "Bars and percentages are relative to design capacity."))
            }
        }

        if hasChargingCareDetails {
            DetailSection(title: String(localized: "Charging & Care"), icon: "leaf.fill", color: .green) {
                if data.chargeLimitPercent < 100 {
                    DetailRow(label: String(localized: "Charge Limit"), value: chargeLimitText)
                }
                if let minSoc = data.dailyMinSoc, let maxSoc = data.dailyMaxSoc {
                    DetailRow(label: String(localized: "Optimized Charging Range"), value: "\(minSoc)% – \(maxSoc)%")
                }
            }
        }

        if hasElectricalReadings {
            DetailSection(title: String(localized: "Live Readings"), icon: "waveform.path.ecg", color: .orange) {
                if let voltage = data.batteryVoltageMV {
                    DetailRow(
                        label: String(localized: "Battery Voltage"),
                        value: String(format: "%.2f V", Double(voltage) / 1000.0)
                    )
                }
                if let amp = data.batteryAmperageMA {
                    DetailRow(label: String(localized: "Battery Current"), value: currentText(amp))
                }
                if let instant = data.instantAmperageMA {
                    DetailRow(label: String(localized: "Battery Instant Current"), value: currentText(instant))
                }
            }
        }

        if hasLifetimeData {
            lifetimeSection
        }

        if hasStatusAlerts {
            DetailSection(title: String(localized: "Status & Alerts"), icon: "exclamationmark.shield", color: .secondary) {
                if let critical = data.atCriticalLevel {
                    DetailRow(
                        label: String(localized: "Critical Low Battery"),
                        value: critical ? String(localized: "Yes") : String(localized: "No")
                    )
                }
                if let failure = data.permanentFailureStatus {
                    DetailRow(
                        label: String(localized: "Permanent Failure Status"),
                        value: failure == 0
                            ? String(localized: "Normal")
                            : String(format: String(localized: "Abnormal (%d)"), failure)
                    )
                }
            }
        }
    }

    private func capacityRow(_ label: String, _ value: Int?, tint: Color) -> some View {
        let fraction = capacityFraction(value)
        let text: String = {
            guard let value else { return unavailableValue }
            guard let fraction else { return "\(value) mAh" }
            return String(format: "%d mAh · %.0f%%", value, fraction * 100)
        }()
        return BarRow(label: label, value: text, fraction: fraction, tint: tint)
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
        return String(format: "%d mA (%@)", abs(milliamps), direction)
    }

    private var lifetimeSection: some View {
        DetailSection(title: String(localized: "Lifetime Statistics"), icon: "clock.arrow.circlepath", color: .purple) {
            if let total = data.totalOperatingTimeMin {
                DetailRow(
                    label: String(localized: "Total Operating Time"),
                    value: "\(total / 60) \(String(localized: "hours"))"
                )
            }
            if let maxT = data.lifetimeMaxTempC {
                let c = TemperatureUnits.lifetimeCelsius(fromWholeDegrees: maxT)
                DetailRow(label: String(localized: "Battery Max Temperature"), value: "\(c) °C")
            }
            if let minT = data.lifetimeMinTempC {
                let c = TemperatureUnits.lifetimeCelsius(fromWholeDegrees: minT)
                DetailRow(label: String(localized: "Battery Min Temperature"), value: "\(c) °C")
            }
            if let avgT = data.lifetimeAvgTempC {
                DetailRow(
                    label: String(localized: "Battery Avg Temperature"),
                    value: String(format: "%.1f °C", TemperatureUnits.lifetimeAvgCelsius(fromDecidegrees: avgT))
                )
            }
            if let maxV = data.lifetimeMaxPackVoltageMV {
                DetailRow(label: String(localized: "Max Pack Voltage"), value: String(format: "%.3f V", Double(maxV) / 1000.0))
            }
            if let minV = data.lifetimeMinPackVoltageMV {
                DetailRow(label: String(localized: "Min Pack Voltage"), value: String(format: "%.3f V", Double(minV) / 1000.0))
            }
            if let maxCharge = data.lifetimeMaxChargeCurrentMA, maxCharge < 100_000 {
                DetailRow(label: String(localized: "Max Charging Current"), value: "\(maxCharge) mA")
            }
            if let maxDischarge = data.lifetimeMaxDischargeCurrentMA, maxDischarge < 100_000 {
                DetailRow(label: String(localized: "Max Discharging Current"), value: "\(maxDischarge) mA")
            }
            if let discCount = data.batteryCellDisconnectCount {
                DetailRow(label: String(localized: "Protection Trigger Count"), value: protectionTriggerLabel(count: discCount))
            }
        }
    }

    // MARK: - Cells Tab

    @ViewBuilder
    private var cellsTab: some View {
        DetailSection(title: String(localized: "Cell Data"), icon: "battery.100", color: .cyan) {
            if let topology = cellTopologyDescription {
                DetailNote(topology)
            }
            if let cells = data.cellVoltagesMV, let balance = seriesVoltageBalanceSummary(cells) {
                DetailRow(label: String(localized: "Series Voltage Balance"), value: balance)
            }
            if usesSeriesParallelLayout,
               let currents = data.batteryParallelCellCurrents,
               let balance = parallelCurrentBalanceSummary(currents) {
                DetailRow(label: String(localized: "Parallel Current Balance"), value: balance)
            }
        }

        DetailSection(
            title: usesSeriesParallelLayout ? String(localized: "Series Groups") : String(localized: "Cell Data"),
            icon: "square.stack.3d.up",
            color: .cyan
        ) {
            if usesSeriesParallelLayout {
                DetailNote(String(localized: "Series group Qmax is the parallel pair's estimated full-charge capacity (mAh), not derived from cell currents."))
            }
            ForEach(Array(cellDisplayEntries.enumerated()), id: \.offset) { idx, entry in
                BarRow(
                    label: usesSeriesParallelLayout
                        ? String(format: String(localized: "Series Group %d"), idx + 1)
                        : String(format: String(localized: "Cell %d"), idx + 1),
                    value: usesSeriesParallelLayout
                        ? seriesGroupSummary(voltageMV: entry.voltageMV, qmaxMAH: entry.qmaxMAH)
                        : cellSummary(voltageMV: entry.voltageMV, qmaxMAH: entry.qmaxMAH),
                    fraction: entry.voltageMV.map(cellVoltageFraction),
                    tint: .cyan
                )
            }
        }

        if usesSeriesParallelLayout, let currents = data.batteryParallelCellCurrents, !currents.isEmpty {
            DetailSection(title: String(localized: "Parallel Cell Currents"), icon: "arrow.left.arrow.right", color: .cyan) {
                DetailNote(String(localized: "Instantaneous current per parallel cell; unequal split within a group is normal."))
                ForEach(Array(currents.enumerated()), id: \.offset) { _, cell in
                    DetailRow(
                        label: parallelCellLabel(bankID: cell.bankID, cellID: cell.cellID),
                        value: "\(cell.currentMA) mA"
                    )
                }
            }
        }
    }

    /// Li-ion working window; only used to draw a comparable bar per cell.
    private func cellVoltageFraction(_ millivolts: Int) -> Double {
        (Double(millivolts) - 3000) / (4400 - 3000)
    }

    // MARK: - Device Tab

    @ViewBuilder
    private var deviceTab: some View {
        DetailSection(title: String(localized: "Device Information"), icon: "info.circle", color: .secondary) {
            if let serial = data.batterySerial {
                HStack(spacing: DetailLayout.hSpacing) {
                    DetailRow(label: String(localized: "Battery Serial Number"), value: serial)
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
                DetailRow(label: String(localized: "Battery Management Chip"), value: name)
            }
            if let mfg = data.batteryManufactureDate {
                DetailRow(label: String(localized: "Manufacture Date"), value: mfg)
            }
            DetailRow(
                label: String(localized: "Data Source"),
                value: data.dataSource == .telemetry
                    ? String(localized: "Power Telemetry")
                    : String(localized: "Estimation Mode")
            )
            DetailNote(String(localized: "Device info is shown locally only and is not uploaded."))
        }
    }

    private var footerTimestamp: some View {
        HStack {
            Spacer()
            Text(
                String(
                    format: String(localized: "Updated at %@"),
                    data.timestamp.formatted(date: .omitted, time: .standard)
                )
            )
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

    private var hasCellData: Bool {
        !cellDisplayEntries.isEmpty
            || !(data.batteryParallelCellCurrents?.isEmpty ?? true)
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
                    seriesCount,
                    parallelCount,
                    seriesCount * parallelCount
                )
            }
            return String(
                format: String(localized: "Battery topology: %d series groups"),
                seriesCount
            )
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

    private func parallelCellLabel(bankID: Int, cellID: Int) -> String {
        String(
            format: String(localized: "Series Group %d · Cell %d"),
            bankID + 1,
            cellID + 1
        )
    }

    private func seriesGroupSummary(voltageMV: Int?, qmaxMAH: Int?) -> String {
        var parts: [String] = []
        if let voltageMV {
            parts.append(String(format: "%.3f V", Double(voltageMV) / 1000.0))
        }
        if let qmaxMAH {
            parts.append(String(format: String(localized: "Qmax %d mAh"), qmaxMAH))
        }
        return parts.isEmpty ? String(localized: "—") : parts.joined(separator: " · ")
    }

    private func cellSummary(voltageMV: Int?, qmaxMAH: Int?) -> String {
        var parts: [String] = []
        if let voltageMV {
            parts.append(String(format: "%.3f V", Double(voltageMV) / 1000.0))
        }
        if let qmaxMAH {
            parts.append("\(qmaxMAH) mAh")
        }
        return parts.isEmpty ? String(localized: "—") : parts.joined(separator: " · ")
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

    private var hasChargingCareDetails: Bool {
        data.chargeLimitPercent < 100 || (data.dailyMinSoc != nil && data.dailyMaxSoc != nil)
    }

    private var chargeLimitText: String {
        if let source = chargeLimitSourceLabel(data.chargeLimitSource) {
            return "\(data.chargeLimitPercent)% (\(source))"
        }
        return "\(data.chargeLimitPercent)%"
    }

    private var batteryLevelPercent: Int {
        data.stateOfCharge ?? data.batteryPercent
    }

    private var batteryLevelBarTint: Color {
        let level = batteryLevelPercent
        if level <= 20 { return .red }
        if level <= 50 { return .orange }
        return .green
    }

    private var cycleCountFraction: Double? {
        guard let cycles = data.cycleCount, let designCycles = data.designCycleCount, designCycles > 0 else {
            return nil
        }
        return Double(cycles) / Double(designCycles)
    }

    private var hasElectricalReadings: Bool {
        data.batteryVoltageMV != nil
            || data.batteryAmperageMA != nil
            || data.instantAmperageMA != nil
    }

    private var hasStatusAlerts: Bool {
        data.atCriticalLevel != nil || data.permanentFailureStatus != nil
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
        if count == 0 {
            return String(localized: "0 (normal)")
        }
        return "\(count)"
    }
}

// MARK: - Helper Views

/// Shared column geometry so EN/ZH label length does not shift bars or values.
private enum DetailLayout {
    static let labelWidth: CGFloat = 150
    static let barWidth: CGFloat = 96
    static let hSpacing: CGFloat = 10
}

private struct OverviewTile: View {
    let value: String
    var caption: String? = nil
    let label: String
    let tint: Color
    let fraction: Double?

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(alignment: .firstTextBaseline, spacing: 2) {
                Text(value)
                    .font(.system(size: 18, weight: .semibold, design: .rounded))
                    .monospacedDigit()
                if let caption {
                    Text(caption)
                        .font(.system(size: 10))
                        .foregroundStyle(.tertiary)
                        .monospacedDigit()
                }
            }
            .lineLimit(1)
            .minimumScaleFactor(0.7)
            Text(label)
                .font(.system(size: 10))
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .minimumScaleFactor(0.8)
            // Reserve bar height so tiles without a bar stay aligned.
            LevelBar(fraction: fraction ?? 0, tint: tint, height: 4)
                .opacity(fraction == nil ? 0 : 1)
        }
        .padding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.background.secondary, in: RoundedRectangle(cornerRadius: 8))
    }
}

private struct DetailSection<Content: View>: View {
    let title: String
    let icon: String
    let color: Color
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 7) {
            HStack(spacing: 6) {
                Image(systemName: icon).foregroundStyle(color).font(.system(size: 12))
                Text(title).font(.system(size: 13, weight: .semibold))
            }
            .padding(.bottom, 2)

            content
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.background.secondary, in: RoundedRectangle(cornerRadius: 8))
    }
}

private struct DetailNote: View {
    let text: String

    init(_ text: String) { self.text = text }

    var body: some View {
        Text(text)
            .font(.system(size: 10))
            .foregroundStyle(.tertiary)
            .fixedSize(horizontal: false, vertical: true)
    }
}

/// Label | fixed bar column | trailing value.
private struct BarRow: View {
    let label: String
    let value: String
    let fraction: Double?
    var tint: Color = .accentColor

    var body: some View {
        HStack(alignment: .center, spacing: DetailLayout.hSpacing) {
            Text(label)
                .font(.system(size: 12))
                .foregroundStyle(.secondary)
                .frame(width: DetailLayout.labelWidth, alignment: .leading)
                .lineLimit(2)
                .minimumScaleFactor(0.85)

            LevelBar(fraction: fraction ?? 0, tint: tint)
                .frame(width: DetailLayout.barWidth)
                .opacity(fraction == nil ? 0 : 1)

            Spacer(minLength: 8)

            Text(value)
                .font(.system(size: 12, design: .rounded))
                .monospacedDigit()
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.trailing)
                .lineLimit(1)
                .minimumScaleFactor(0.8)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// Key–value row with the same fixed label column as bar rows.
private struct DetailRow: View {
    let label: String
    let value: String
    var highlight: Bool = false

    var body: some View {
        HStack(alignment: .top, spacing: DetailLayout.hSpacing) {
            Text(label)
                .font(.system(size: 12))
                .foregroundStyle(.secondary)
                .frame(width: DetailLayout.labelWidth, alignment: .leading)
                .lineLimit(2)
                .minimumScaleFactor(0.85)
                .fixedSize(horizontal: false, vertical: true)

            Spacer(minLength: 8)

            Text(value)
                .font(.system(size: 12, weight: highlight ? .bold : .regular, design: .rounded))
                .monospacedDigit()
                .foregroundStyle(highlight ? .primary : .secondary)
                .multilineTextAlignment(.trailing)
                .lineLimit(3)
                .minimumScaleFactor(0.85)
                .fixedSize(horizontal: false, vertical: true)
                .textSelection(.enabled)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}
