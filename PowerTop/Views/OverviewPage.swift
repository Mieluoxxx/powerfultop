import SwiftUI

/// Glanceable dashboard: what the popover shows, plus specs and the last 30 minutes as charts.
struct OverviewPage: View {
    let monitor: PowerMonitor
    let thermal: ThermalMonitor
    let history: HistoryRecorder

    private var data: PowerData { monitor.currentData }
    private var samples: MetricHistory { history.history }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            PowerStrip(data: data)

            HStack(alignment: .top, spacing: 16) {
                batterySpecCard
                healthSpecCard
                adapterSpecCard
            }
            .fixedSize(horizontal: false, vertical: true)

            LazyVGrid(columns: [GridItem(.flexible(), spacing: 16), GridItem(.flexible(), spacing: 16)], spacing: 16) {
                ChartPanel(
                    title: String(localized: "Battery Level"),
                    value: "\(data.displayBatteryPercent)%",
                    detail: data.estimatedTimeRemainingText.map { "\(data.estimatedTimeRemainingLabel) \($0)" },
                    icon: data.batteryLevelSymbol,
                    tint: .green,
                    points: samples.series { $0.batteryPercent },
                    domain: 0...100
                )
                ChartPanel(
                    title: String(localized: "System Power"),
                    value: String(format: "%.1f W", data.systemPowerW),
                    detail: data.averageSystemPowerW.map { String(format: String(localized: "Average System Power: %@"), String(format: "%.1f W", $0)) },
                    icon: "bolt.fill",
                    tint: .purple,
                    points: samples.series { $0.systemPowerW }
                )
                if let cpu = thermal.snapshot.temperatures[.cpu] {
                    ChartPanel(
                        title: String(localized: "CPU Average"),
                        value: String(format: "%.1f°C", cpu.averageC),
                        detail: String(format: String(localized: "Hottest core %@"), String(format: "%.0f°C", cpu.maximumC)),
                        icon: "cpu",
                        tint: .orange,
                        points: samples.series { $0.cpuAverageC }
                    )
                }
                if thermal.snapshot.hasFans {
                    ChartPanel(
                        title: String(localized: "Fan Speed"),
                        value: String(format: "%.0f RPM", thermal.snapshot.fans.map(\.actualRPM).max() ?? 0),
                        detail: thermal.isControlling ? String(localized: "Controlled by PowerTop") : String(localized: "Controlled by macOS"),
                        icon: "fan.fill",
                        tint: .teal,
                        points: samples.series { $0.fanRPM },
                        domain: 0...max(thermal.snapshot.fans.map(\.maxRPM).max() ?? 1, 1)
                    )
                } else if let temperature = batteryTemperatureC(data: data, thermal: thermal.snapshot) {
                    ChartPanel(
                        title: String(localized: "Battery Temp"),
                        value: String(format: "%.1f°C", temperature),
                        icon: "thermometer.medium",
                        tint: .blue,
                        points: samples.series { $0.batteryTemperatureC }
                    )
                }
            }
        }
    }

    private var batterySpecCard: some View {
        DashCard(title: String(localized: "Battery"), icon: "bolt.fill", tint: .yellow) {
            if let amp = data.batteryAmperageMA {
                CardRow(label: String(localized: "Battery Current"), value: PowerData.batteryCurrentText(amp))
            }
            if let voltage = data.batteryVoltageMV {
                CardRow(label: String(localized: "Battery Voltage"), value: String(format: "%.2f V", Double(voltage) / 1000.0))
            }
            CardRow(label: String(localized: "Battery Power"), value: data.signedBatteryPowerText)
            if let remaining = data.remainingCapacityMAH {
                CardRow(label: String(localized: "Remaining Charge"), value: "\(remaining) mAh")
            }
            if let temperature = batteryTemperatureC(data: data, thermal: thermal.snapshot) {
                CardRow(label: String(localized: "Battery Temp"), value: String(format: "%.1f°C", temperature))
            }
        }
    }

    private var healthSpecCard: some View {
        DashCard(title: String(localized: "Battery Health"), icon: "heart.fill", tint: data.batteryHealthTint) {
            if let design = data.designCapacityMAH {
                CardRow(label: String(localized: "Design Capacity"), value: "\(design) mAh", rated: "100%")
            }
            if let full = data.fullChargeCapacityMAH ?? data.rawMaxCapacityMAH {
                CardRow(
                    label: String(localized: "Current Full Charge"),
                    value: "\(full) mAh",
                    rated: data.capacityFractionOfDesign(full).map { String(format: "%.0f%%", $0 * 100) }
                )
            }
            if let health = data.batteryHealthPercent {
                CardRow(label: String(localized: "Health"), value: "\(health)%", emphasized: true)
            }
            if let cycles = data.cycleCount {
                CardRow(label: String(localized: "Cycle Count"), value: "\(cycles)", rated: data.designCycleCount.map { "\($0)" })
            }
            CardRow(label: String(localized: "Charge Limit"), value: data.chargeLimitText)
        }
    }

    private var adapterSpecCard: some View {
        DashCard(title: String(localized: "Power Adapter"), icon: "powerplug.fill", tint: .green) {
            if data.hasSettledAC {
                if let sc = data.systemCurrentMA {
                    CardRow(
                        label: String(localized: "Current"),
                        value: String(format: "%.2f A", Double(sc) / 1000.0),
                        rated: data.adapterRatedCurrentMA.map { String(format: "%.1f A", Double($0) / 1000.0) }
                    )
                }
                if let sv = data.systemVoltageMV {
                    CardRow(
                        label: String(localized: "Voltage"),
                        value: String(format: "%.2f V", Double(sv) / 1000.0),
                        rated: data.adapterRatedVoltageMV.map { String(format: "%.0f V", Double($0) / 1000.0) }
                    )
                }
                CardRow(
                    label: String(localized: "Power"),
                    value: String(format: "%.1f W", data.effectiveACOutputW),
                    rated: data.acAdapterWattage > 0 ? "\(data.acAdapterWattage) W" : nil,
                    emphasized: true
                )
                if let wall = data.wallPowerW {
                    CardRow(label: String(localized: "Wall Outlet Power"), value: String(format: "%.1f W", wall))
                }
            } else {
                StatusNote(
                    icon: data.isConnectingAC ? "powerplug" : "powerplug.portrait",
                    tint: .secondary,
                    text: data.isConnectingAC ? String(localized: "AC Connecting") : String(localized: "Not connected to power")
                )
            }
        }
    }
}

/// Top summary shared by the overview: level, flow and time remaining in one line.
struct PowerStrip: View {
    let data: PowerData

    var body: some View {
        HStack(spacing: 0) {
            StripItem(
                icon: data.batteryLevelSymbol,
                tint: data.batteryLevelTint,
                label: "\(data.displayBatteryPercent)%",
                value: data.powerSourceDescription
            )
            StripDivider()
            if data.hasSettledAC {
                StripItem(icon: "powerplug.fill", tint: .green, label: String(localized: "Charger"), value: String(format: "%.1f W", data.effectiveACOutputW))
                StripDivider()
            }
            StripItem(icon: "laptopcomputer", tint: .primary, label: String(localized: "System"), value: String(format: "%.1f W", data.systemPowerW))
            StripDivider()
            StripItem(icon: batteryIcon, tint: batteryTint, label: String(localized: "Battery"), value: data.signedBatteryPowerText)
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
        .dashSurface(cornerRadius: 14)
    }

    private var batteryIcon: String {
        data.isBatteryCharging ? "battery.100.bolt" : data.batteryLevelSymbol
    }

    private var batteryTint: Color {
        switch data.powerState {
        case .charging: return .blue
        case .onBattery, .supplementing: return .orange
        case .connectingAC, .acIdle: return .secondary
        }
    }
}

extension PowerData {
    var batteryHealthTint: Color {
        guard let health = batteryHealthPercent else { return .secondary }
        if health >= 80 { return .green }
        return health >= 60 ? .orange : .red
    }
}
