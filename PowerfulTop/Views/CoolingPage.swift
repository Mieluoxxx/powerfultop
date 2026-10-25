import SwiftUI

struct CoolingPage: View {
    let thermal: ThermalMonitor
    let history: HistoryRecorder

    private var columns: [GridItem] {
        [GridItem(.adaptive(minimum: 320), spacing: 16, alignment: .top)]
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            LazyVGrid(columns: columns, alignment: .leading, spacing: 16) {
                if !thermal.snapshot.temperatures.isEmpty {
                    TemperatureCard(snapshot: thermal.snapshot)
                }
                if thermal.snapshot.hasFans {
                    FanControlCard(thermal: thermal)
                }
            }
            LazyVGrid(columns: columns, alignment: .leading, spacing: 16) {
                if let cpu = thermal.snapshot.temperatures[.cpu] {
                    ChartPanel(
                        title: String(localized: "CPU Average"),
                        value: String(format: "%.1f°C", cpu.averageC),
                        icon: "cpu",
                        tint: .orange,
                        points: history.history.series { $0.cpuAverageC }
                    )
                }
                if thermal.snapshot.hasFans {
                    ChartPanel(
                        title: String(localized: "Fan Speed"),
                        value: String(format: "%.0f RPM", thermal.snapshot.fans.map(\.actualRPM).max() ?? 0),
                        icon: "fan.fill",
                        tint: .teal,
                        points: history.history.series { $0.fanRPM },
                        domain: 0...max(thermal.snapshot.fans.map(\.maxRPM).max() ?? 1, 1)
                    )
                }
            }
        }
    }
}

private struct TemperatureCard: View {
    let snapshot: ThermalSnapshot

    var body: some View {
        DashCard(title: String(localized: "Temperatures"), icon: "thermometer.medium", tint: .orange) {
            if let cpu = snapshot.temperatures[.cpu] {
                CardHero(value: celsius(cpu.averageC), caption: String(localized: "CPU Average"))
            }
            ForEach(SensorGroup.allCases) { group in
                if let summary = snapshot.temperatures[group] {
                    CardBarRow(
                        label: group.title,
                        value: String(format: String(localized: "%@ avg · %@ max"), celsius(summary.averageC), celsius(summary.maximumC)),
                        fraction: summary.maximumC / FanHelperConstants.failsafeCelsius,
                        tint: summary.maximumC >= 90 ? .red : (summary.maximumC >= 75 ? .orange : .green)
                    )
                }
            }
            CardNote(String(localized: "Readings are smoothed over about 10 seconds; individual cores spike briefly under load."))
        }
    }

    private func celsius(_ value: Double) -> String {
        String(format: "%.1f°C", value)
    }
}

/// Mirrors Macs Fan Control: Auto hands fans to macOS; Custom is a constant RPM or a temperature ramp.
struct FanControlCard: View {
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
        DashCard(title: String(localized: "Fans"), icon: "fan.fill", tint: .teal) {
            CardHero(
                value: String(format: "%.0f RPM", snapshot.fans.map(\.actualRPM).max() ?? 0),
                caption: thermal.isControlling ? String(localized: "Controlled by PowerfulTop") : String(localized: "Controlled by macOS")
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
            .padding(.top, 6)

            if preset == .custom {
                customControls
            }
            statusNotes
        }
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
            CardRow(label: String(localized: "Target Speed"), value: String(format: "%.0f RPM", draftRPM ?? rpm), emphasized: true)
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
            StatusNote(
                icon: "exclamationmark.triangle.fill",
                tint: .orange,
                text: String(localized: "Another fan tool is in control"),
                detail: String(localized: "Another fan tool (such as Macs Fan Control) is controlling the fans. Quit it to let PowerfulTop take over; until then PowerfulTop only monitors.")
            )
        } else if !thermal.mode.isAutomatic && thermal.helperStatus != .ready {
            FanHelperInstallButton(thermal: thermal)
        } else if thermal.mode.isAutomatic && snapshot.fans.allSatisfy({ $0.actualRPM == 0 }) {
            StatusNote(
                icon: "moon.zzz",
                tint: .secondary,
                text: String(localized: "Fans are resting"),
                detail: String(localized: "macOS stops the fans while temperatures are low; they spin up on their own when cooling is needed.")
            )
        } else if !thermal.mode.isAutomatic {
            StatusNote(
                icon: "checkmark.shield",
                tint: .green,
                text: String(localized: "Safety handback armed"),
                detail: String(format: String(localized: "Fans return to macOS control when PowerfulTop quits, the Mac sleeps, or the CPU or GPU reaches %.0f°C."), FanHelperConstants.failsafeCelsius)
            )
        }
        if let error = thermal.lastError {
            StatusNote(icon: "xmark.octagon.fill", tint: .red, text: error)
        }
    }
}

struct FanHelperInstallButton: View {
    let thermal: ThermalMonitor

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Button {
                thermal.installHelper()
            } label: {
                Label(
                    thermal.helperStatus == .needsUpdate ? String(localized: "Update Fan Helper") : String(localized: "Install Fan Helper"),
                    systemImage: "lock.shield"
                )
            }
            .buttonStyle(.borderedProminent)
            .controlSize(.regular)
            CardNote(thermal.helperStatus == .needsUpdate
                ? String(localized: "PowerfulTop was updated, so the fan helper must be authorized again.")
                : String(localized: "Controlling fans needs a small helper that runs as root. macOS asks for an administrator password once."))
        }
    }
}
