import AppKit
import Foundation
import Observation

@MainActor
@Observable
final class ThermalMonitor {
    private static let modeKey = "fanControlMode"
    private static let lastCustomModeKey = "fanControlLastCustomMode"
    private static let conflictingBundleIDs = ["com.crystalidea.macsfancontrol"]

    private(set) var snapshot = ThermalSnapshot.empty
    private(set) var helperStatus: FanHelperStatus = .notInstalled
    private(set) var lastError: String?
    /// Another fan tool is running or already holds the fans; PowerTop then only monitors.
    private(set) var conflictDetected = false
    private(set) var mode: FanControlMode
    /// Custom setting restored when switching back from Auto or Full Blast, like Macs Fan Control.
    private(set) var lastCustomMode: FanControlMode

    private let helper = FanHelperClient()
    private let smc = SMCConnection()
    private var sensorKeys: [SensorGroup: [String]] = [:]
    private var timer: Timer?
    private var weEngaged = false
    private var lastRefreshAt: Date?
    private var installing = false
    private var workspaceObservers: [NSObjectProtocol] = []

    init() {
        mode = Self.loadMode(Self.modeKey) ?? .automatic
        lastCustomMode = Self.loadMode(Self.lastCustomModeKey) ?? .defaultConstant
    }

    var isControlling: Bool { !mode.isAutomatic && weEngaged }

    func start() {
        guard timer == nil, smc != nil else { return }
        helperStatus = helper.status
        Task.detached(priority: .utility) {
            let keys = Self.discoverSensorKeys()
            await MainActor.run {
                self.sensorKeys = keys
                self.refresh()
            }
        }
        refresh()
        let newTimer = Timer(timeInterval: 2, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.refresh() }
        }
        RunLoop.main.add(newTimer, forMode: .common)
        timer = newTimer

        let center = NSWorkspace.shared.notificationCenter
        workspaceObservers.append(center.addObserver(forName: NSWorkspace.willSleepNotification, object: nil, queue: .main) { [weak self] _ in
            // Firmware drops the manual unlock across sleep; hand control back and re-engage on wake.
            Task { @MainActor in self?.releaseFans() }
        })
    }

    /// Called on quit: fans must be back under macOS control before the process exits.
    func shutdown() {
        timer?.invalidate()
        timer = nil
        workspaceObservers.forEach { NSWorkspace.shared.notificationCenter.removeObserver($0) }
        workspaceObservers.removeAll()
        if weEngaged {
            helper.restoreAutomaticSynchronously()
            weEngaged = false
        }
    }

    func setMode(_ newMode: FanControlMode) {
        mode = newMode
        Self.saveMode(newMode, key: Self.modeKey)
        if case .constant = newMode { rememberCustom(newMode) }
        if case .sensor = newMode { rememberCustom(newMode) }
        lastError = nil
        if newMode.isAutomatic {
            releaseFans()
        } else {
            refresh()
        }
    }

    func installHelper() {
        guard !installing else { return }
        installing = true
        defer { installing = false }
        do {
            try helper.install()
            lastError = nil
        } catch FanHelperError.cancelled {
            lastError = nil
        } catch {
            lastError = error.localizedDescription
        }
        helperStatus = helper.status
        refresh()
    }

    private func rememberCustom(_ custom: FanControlMode) {
        lastCustomMode = custom
        Self.saveMode(custom, key: Self.lastCustomModeKey)
    }

    private func releaseFans() {
        guard weEngaged else { return }
        weEngaged = false
        helper.restoreAutomatic { [weak self] error in
            Task { @MainActor in self?.lastError = error }
        }
    }

    private func refresh() {
        guard let smc else { return }
        var next = Self.readSnapshot(smc: smc, sensorKeys: sensorKeys)
        let now = Date()
        next.temperatures = smoothTemperatures(
            previous: snapshot.temperatures,
            current: next.temperatures,
            elapsed: lastRefreshAt.map { now.timeIntervalSince($0) } ?? 0
        )
        lastRefreshAt = now
        snapshot = next
        conflictDetected = Self.isOtherFanToolRunning() || (!weEngaged && (snapshot.anyFanManual || snapshot.diagnosticUnlocked))
        applyControl()
    }

    private func applyControl() {
        guard !mode.isAutomatic, snapshot.hasFans, !conflictDetected || weEngaged, helperStatus == .ready else { return }
        let targets = snapshot.fans.map { fan in
            fanTargetRPM(mode: mode, fan: fan, temperatures: snapshot.temperatures) ?? fan.minRPM
        }
        weEngaged = true
        helper.setFanTargets(targets) { [weak self] error in
            Task { @MainActor in
                guard let self else { return }
                self.lastError = error
                if error != nil { self.weEngaged = false }
            }
        }
    }

    // MARK: - SMC reads (no privileges needed)

    private nonisolated static func discoverSensorKeys() -> [SensorGroup: [String]] {
        guard let smc = SMCConnection() else { return [:] }
        let keys = (0..<smc.keyCount()).compactMap { smc.key(at: $0) }
            .filter { key in
                key.hasPrefix("T") && smc.read(key).map { $0.type == "flt " } == true
            }
        return groupSensorKeys(keys)
    }

    private static func readSnapshot(smc: SMCConnection, sensorKeys: [SensorGroup: [String]]) -> ThermalSnapshot {
        let fanCount = Int(smc.read("FNum")?.doubleValue ?? 0)
        let fans: [FanReading] = (0..<fanCount).compactMap { index in
            guard let maxRPM = smc.read("F\(index)Mx")?.doubleValue, maxRPM > 0 else { return nil }
            let mode = smc.read("F\(index)Md") ?? smc.read("F\(index)md")
            return FanReading(
                index: index,
                actualRPM: smc.read("F\(index)Ac")?.doubleValue ?? 0,
                minRPM: smc.read("F\(index)Mn")?.doubleValue ?? 0,
                maxRPM: maxRPM,
                targetRPM: smc.read("F\(index)Tg")?.doubleValue ?? 0,
                mode: mode?.doubleValue.map(Int.init)
            )
        }
        var temperatures: [SensorGroup: TemperatureSummary] = [:]
        for (group, keys) in sensorKeys {
            let readings = keys.compactMap { smc.read($0)?.doubleValue }.filter(isPlausibleTemperature)
            temperatures[group] = TemperatureSummary(readings)
        }
        return ThermalSnapshot(
            fans: fans,
            temperatures: temperatures,
            diagnosticUnlocked: smc.read("Ftst")?.doubleValue == 1
        )
    }

    private static func isOtherFanToolRunning() -> Bool {
        NSWorkspace.shared.runningApplications.contains { app in
            conflictingBundleIDs.contains(app.bundleIdentifier ?? "")
        }
    }

    private static func loadMode(_ key: String) -> FanControlMode? {
        UserDefaults.standard.data(forKey: key).flatMap { try? JSONDecoder().decode(FanControlMode.self, from: $0) }
    }

    private static func saveMode(_ mode: FanControlMode, key: String) {
        UserDefaults.standard.set(try? JSONEncoder().encode(mode), forKey: key)
    }
}
