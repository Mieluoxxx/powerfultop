import AppKit
import SwiftUI

/// Depth the overview leaves out: capacity history, charger negotiation, cells and lifetime stats.
struct BatteryPage: View {
    let monitor: PowerMonitor
    let columnCount: Int

    private var data: PowerData { monitor.currentData }

    var body: some View {
        MasonryColumns(items: cards, columnCount: columnCount, spacing: 16)
    }

    private var cards: [MasonryItem] {
        var cards = [MasonryItem(id: "health", weight: 10, view: AnyView(healthCard))]
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

    private var healthColor: Color {
        guard let h = data.batteryHealthPercent else { return .secondary }
        if h >= 80 { return .green }
        if h >= 60 { return .orange }
        return .red
    }

    private func watts(_ value: Double) -> String {
        String(format: "%.1f W", value)
    }
}
