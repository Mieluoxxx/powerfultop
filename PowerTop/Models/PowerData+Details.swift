import Foundation

/// Derived descriptions for the details window, kept out of SwiftUI so they can be tested.
extension PowerData {
    func capacityFractionOfDesign(_ value: Int?) -> Double? {
        guard let value, let design = designCapacityMAH, design > 0 else { return nil }
        return Double(value) / Double(design)
    }

    /// Raw IOKit amperage: positive = charging, negative = discharging.
    static func batteryCurrentText(_ milliamps: Int) -> String {
        let direction = milliamps > 0
            ? String(localized: "Charging")
            : (milliamps < 0 ? String(localized: "Discharging") : String(localized: "Idle"))
        return String(format: "%.2f A (%@)", Double(abs(milliamps)) / 1000.0, direction)
    }

    /// Li-ion working window; only used to draw a comparable bar per cell.
    func cellVoltageFraction(_ millivolts: Int) -> Double {
        (Double(millivolts) - 3000) / (4400 - 3000)
    }

    var hasCellData: Bool {
        !cellDisplayEntries.isEmpty || !(batteryParallelCellCurrents?.isEmpty ?? true)
    }

    var usesSeriesParallelLayout: Bool {
        if case .seriesParallel = batteryCellLayout { return true }
        return false
    }

    var cellTopologyDescription: String? {
        switch batteryCellLayout {
        case .seriesParallel(let seriesCount, let parallelCount, let parallelCountKnown):
            if parallelCountKnown, parallelCount > 1 {
                return String(
                    format: String(localized: "Battery topology: %dS%dP (%d cells)"),
                    seriesCount, parallelCount, seriesCount * parallelCount
                )
            }
            return String(format: String(localized: "Battery topology: %d series groups"), seriesCount)
        case .perCellArrays:
            if let count = cellVoltagesMV?.count, count > 0 {
                return String(format: String(localized: "Battery topology: %d cells"), count)
            }
            return nil
        case nil:
            return nil
        }
    }

    var cellDisplayEntries: [(voltageMV: Int?, qmaxMAH: Int?)] {
        let voltages = cellVoltagesMV ?? []
        let capacities = qmaxMAH ?? []
        let count = max(voltages.count, capacities.count)
        guard count > 0 else { return [] }
        return (0..<count).map { index in
            (
                voltageMV: index < voltages.count ? voltages[index] : nil,
                qmaxMAH: index < capacities.count ? capacities[index] : nil
            )
        }
    }

    func cellSummary(voltageMV: Int?, qmaxMAH: Int?) -> String {
        var parts: [String] = []
        if let voltageMV {
            parts.append(String(format: "%.3f V", Double(voltageMV) / 1000.0))
        }
        if let qmaxMAH {
            parts.append("\(qmaxMAH) mAh")
        }
        return parts.isEmpty ? String(localized: "—") : parts.joined(separator: " · ")
    }

    func parallelCellLabel(bankID: Int, cellID: Int) -> String {
        String(format: String(localized: "Series Group %d · Cell %d"), bankID + 1, cellID + 1)
    }

    var hasLifetimeData: Bool {
        totalOperatingTimeMin != nil
            || lifetimeMaxTempC != nil
            || lifetimeMinTempC != nil
            || lifetimeAvgTempC != nil
            || lifetimeMaxPackVoltageMV != nil
            || lifetimeMinPackVoltageMV != nil
            || lifetimeMaxChargeCurrentMA != nil
            || lifetimeMaxDischargeCurrentMA != nil
            || batteryCellDisconnectCount != nil
    }

    var chargeLimitText: String {
        if let source = chargeLimitSourceLabel(chargeLimitSource) {
            return "\(chargeLimitPercent)% (\(source))"
        }
        return "\(chargeLimitPercent)%"
    }

    func seriesVoltageBalanceSummary(_ voltagesMV: [Int]) -> String? {
        guard voltagesMV.count >= 2, let minMV = voltagesMV.min(), let maxMV = voltagesMV.max() else { return nil }
        let delta = maxMV - minMV
        return String(format: String(localized: "%d mV spread — %@"), delta, balanceStatus(forMillivoltSpread: delta))
    }

    func parallelCurrentBalanceSummary(_ currents: [BatteryParallelCellCurrent]) -> String? {
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

    fileprivate func balanceStatus(forMillivoltSpread delta: Int) -> String {
        if delta <= 50 { return String(localized: "Normal") }
        if delta <= 100 { return String(localized: "Fair") }
        return String(localized: "Worth Checking")
    }

    fileprivate func balanceStatus(forMilliampSpread delta: Int) -> String {
        if delta <= 80 { return String(localized: "Normal") }
        if delta <= 150 { return String(localized: "Fair") }
        return String(localized: "Worth Checking")
    }

    func protectionTriggerLabel(count: Int) -> String {
        count == 0 ? String(localized: "0 (normal)") : "\(count)"
    }
}
