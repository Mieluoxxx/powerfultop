import Foundation

struct FanReading: Equatable, Identifiable {
    let index: Int
    let actualRPM: Double
    let minRPM: Double
    let maxRPM: Double
    let targetRPM: Double
    /// Raw `F%dMd`: 0 auto, 1 manual, 3 system-managed (Apple Silicon default).
    let mode: Int?

    var id: Int { index }
    var isManual: Bool { mode == 1 }
}

/// Sensor families whose keys were verified to track load on Apple Silicon (M4 Max probe).
enum SensorGroup: String, CaseIterable, Codable, Identifiable {
    case cpu
    case gpu
    case battery

    var id: String { rawValue }

    var title: String {
        switch self {
        case .cpu: return String(localized: "CPU")
        case .gpu: return String(localized: "GPU")
        case .battery: return String(localized: "Battery")
        }
    }

    /// `Tp*`/`Te*` are performance/efficiency cores, `Tg*` the GPU, `TB?T` the pack thermistors.
    /// `Tf*` stays excluded: it held 90.2 °C idle and under load, so it is a limit, not a reading.
    func matches(_ key: String) -> Bool {
        switch self {
        case .cpu: return key.hasPrefix("Tp") || key.hasPrefix("Te")
        case .gpu: return key.hasPrefix("Tg")
        case .battery: return key.count == 4 && key.hasPrefix("TB") && key.hasSuffix("T")
        }
    }
}

struct TemperatureSummary: Equatable {
    let averageC: Double
    let maximumC: Double
    let sensorCount: Int

    init(averageC: Double, maximumC: Double, sensorCount: Int) {
        self.averageC = averageC
        self.maximumC = maximumC
        self.sensorCount = sensorCount
    }

    init?(_ readings: [Double]) {
        guard !readings.isEmpty else { return nil }
        self.init(
            averageC: readings.reduce(0, +) / Double(readings.count),
            maximumC: readings.max() ?? 0,
            sensorCount: readings.count
        )
    }
}

/// Per-core hotspot sensors (`Tp1x`, `Tp1o`, …) jump 20–25 °C within half a second as single cores
/// burst, so the raw group maximum swings wildly. An exponential average over ~10 s keeps a brief
/// spike to a few degrees while sustained heating still comes through within tens of seconds.
func smoothTemperatures(
    previous: [SensorGroup: TemperatureSummary],
    current: [SensorGroup: TemperatureSummary],
    elapsed: TimeInterval,
    timeConstant: TimeInterval = 10
) -> [SensorGroup: TemperatureSummary] {
    let alpha = 1 - exp(-max(elapsed, 0) / timeConstant)
    var result: [SensorGroup: TemperatureSummary] = [:]
    for (group, sample) in current {
        guard let last = previous[group] else {
            result[group] = sample
            continue
        }
        result[group] = TemperatureSummary(
            averageC: last.averageC + alpha * (sample.averageC - last.averageC),
            maximumC: last.maximumC + alpha * (sample.maximumC - last.maximumC),
            sensorCount: sample.sensorCount
        )
    }
    return result
}

struct ThermalSnapshot: Equatable {
    var fans: [FanReading] = []
    var temperatures: [SensorGroup: TemperatureSummary] = [:]
    /// `Ftst` = 1 means something has put the SMC into diagnostic (manual fan) mode.
    var diagnosticUnlocked = false

    static let empty = ThermalSnapshot()

    var hasFans: Bool { !fans.isEmpty }
    var anyFanManual: Bool { fans.contains(where: \.isManual) }
    var hottestC: Double? { temperatures.values.map(\.maximumC).max() }
}

/// Plausible live temperatures only; discards sentinels and unused channels.
func isPlausibleTemperature(_ celsius: Double) -> Bool {
    celsius > 5 && celsius < 130
}

func groupSensorKeys(_ keys: [String]) -> [SensorGroup: [String]] {
    var groups: [SensorGroup: [String]] = [:]
    for key in keys {
        guard let group = SensorGroup.allCases.first(where: { $0.matches(key) }) else { continue }
        groups[group, default: []].append(key)
    }
    return groups
}

/// Manual fan state only counts as another tool's when our helper's marker is absent; otherwise a
/// single failed request would make PowerfulTop back off from its own control and let the watchdog fire.
func isForeignFanControl(snapshot: ThermalSnapshot, helperHoldsFans: Bool, otherToolRunning: Bool) -> Bool {
    if otherToolRunning { return true }
    guard snapshot.anyFanManual || snapshot.diagnosticUnlocked else { return false }
    return !helperHoldsFans
}

enum FanControlMode: Equatable, Codable {
    case automatic
    case fullBlast
    case constant(rpm: Double)
    /// Linear from the fan minimum at `lowerC` to its maximum at `upperC`, like Macs Fan Control.
    case sensor(group: SensorGroup, lowerC: Double, upperC: Double)

    static let defaultConstant = FanControlMode.constant(rpm: 3000)
    static let defaultSensor = FanControlMode.sensor(group: .cpu, lowerC: 55, upperC: 85)

    var isAutomatic: Bool { self == .automatic }
}

/// Target for one fan, or nil when the system should stay in charge.
func fanTargetRPM(mode: FanControlMode, fan: FanReading, temperatures: [SensorGroup: TemperatureSummary]) -> Double? {
    let target: Double
    switch mode {
    case .automatic:
        return nil
    case .fullBlast:
        target = fan.maxRPM
    case .constant(let rpm):
        target = rpm
    case .sensor(let group, let lowerC, let upperC):
        // Without a reading, fail towards cooling rather than silence.
        guard let celsius = temperatures[group]?.maximumC else { return fan.maxRPM }
        let span = max(upperC - lowerC, 1)
        let fraction = min(max((celsius - lowerC) / span, 0), 1)
        target = fan.minRPM + (fan.maxRPM - fan.minRPM) * fraction
    }
    return min(max(target, fan.minRPM), fan.maxRPM)
}

enum FanHelperConstants {
    static let label = "io.github.mieluoxxx.PowerfulTop.FanHelper"
    static let installedPath = "/Library/PrivilegedHelperTools/\(label)"
    static let launchDaemonPath = "/Library/LaunchDaemons/\(label).plist"
    static let supportDirectory = "/Library/Application Support/PowerfulTop"
    static let clientRequirementPath = "\(supportDirectory)/fan-helper-client.req"
    /// Present while our helper holds the fans; survives a helper crash so the next launch can restore.
    static let engagedMarkerPath = "\(supportDirectory)/fan-helper.engaged"

    /// The helper as installed by builds published under the upstream bundle ID.
    enum Legacy {
        static let label = "com.kdolphin.PowerTop.FanHelper"
        static let installedPath = "/Library/PrivilegedHelperTools/\(label)"
        static let launchDaemonPath = "/Library/LaunchDaemons/\(label).plist"
        static let supportDirectory = "/Library/Application Support/PowerTop"
        static let engagedMarkerPath = "\(supportDirectory)/fan-helper.engaged"
    }
    /// Ftst handoff took 10–12 s on an M4 Max under macOS 27, longer than the 3–6 s reported elsewhere.
    static let engageTimeout: TimeInterval = 20
    static let protocolVersion = 1
    /// Helper hands fans back to macOS if the app stops checking in for this long.
    static let heartbeatTimeout: TimeInterval = 15
    /// Helper hands fans back to macOS at this temperature regardless of the requested mode.
    static let failsafeCelsius: Double = 100
}

@objc protocol FanHelperProtocol {
    func protocolVersion(reply: @escaping (Int) -> Void)
    /// RPM per fan index; the helper clamps to each fan's reported range and unlocks manual mode if needed.
    func setFanTargets(_ rpms: [NSNumber], reply: @escaping (String?) -> Void)
    func restoreAutomatic(reply: @escaping (String?) -> Void)
}
