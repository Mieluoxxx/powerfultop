import AppKit
import Foundation

enum BatteryChargeLimitSource: Equatable {
    case none
    case macOSSetting
    case optimizedCharging
    case alDente
}

struct ResolvedBatteryChargeLimit: Equatable {
    let percent: Int
    let source: BatteryChargeLimitSource

    static let full = ResolvedBatteryChargeLimit(percent: 100, source: .none)
}

private let optimizedChargingPauseReason: Int = 0x0100_0000

/// Resolves the effective charge target for time-to-full estimates.
func resolveBatteryChargeLimit(
    dailyMaxSoc: Int?,
    notChargingReason: Int? = nil
) -> ResolvedBatteryChargeLimit {
    var activeLimits: [(percent: Int, source: BatteryChargeLimitSource)] = []

    if let limit = readMacOSChargeLimitPercent() {
        activeLimits.append((limit, .macOSSetting))
    }
    if let limit = readAlDenteChargeLimitPercent() {
        activeLimits.append((limit, .alDente))
    }

    if let strictest = activeLimits.min(by: { $0.percent < $1.percent }) {
        return ResolvedBatteryChargeLimit(percent: strictest.percent, source: strictest.source)
    }

    if isOptimizedChargingHoldActive(notChargingReason: notChargingReason),
       let maxSoc = dailyMaxSoc,
       (1..<100).contains(maxSoc) {
        return ResolvedBatteryChargeLimit(percent: maxSoc, source: .optimizedCharging)
    }

    return .full
}

func chargeLimitSourceLabel(_ source: BatteryChargeLimitSource) -> String? {
    switch source {
    case .none: return nil
    case .macOSSetting: return String(localized: "macOS setting")
    case .optimizedCharging: return String(localized: "Optimized Charging")
    case .alDente: return "AlDente"
    }
}

private func isOptimizedChargingHoldActive(notChargingReason: Int?) -> Bool {
    guard let reason = notChargingReason else { return false }
    return reason & optimizedChargingPauseReason != 0
}

/// System Settings › Battery › Charging, where macOS 26.4+ exposes the native 80–100% limit.
let macOSChargeLimitSettingsURL = URL(string: "x-apple.systempreferences:com.apple.Battery-Settings.extension")!

/// powerd enforces the native limit; `prior.limit` in the batteryui domain lingers after the limit
/// is switched off, so it cannot tell whether a limit is active.
private func readMacOSChargeLimitPercent() -> Int? {
    PowerdChargeLimitCache.shared.limit()
}

/// `pmset -g battlimit` lists the SOC limits powerd is enforcing; the strictest one applies.
func parsePowerdChargeLimit(_ output: String) -> Int? {
    let pattern = try! Regex(#"chargeSocLimitSoc\s*=\s*(\d+);"#)
    let limits = output.matches(of: pattern).compactMap { match in
        match.output[1].substring.flatMap { Int($0) }
    }
    return limits.compactMap(normalizedChargeLimitPercent).min()
}

/// Spawning pmset on every 2 s refresh is wasteful; the limit only changes when the user edits it.
private final class PowerdChargeLimitCache: @unchecked Sendable {
    static let shared = PowerdChargeLimitCache()
    private let lock = NSLock()
    private var cached: Int?
    private var readAt = Date.distantPast
    private let lifetime: TimeInterval = 30

    func limit() -> Int? {
        lock.lock()
        defer { lock.unlock() }
        if Date().timeIntervalSince(readAt) < lifetime { return cached }
        cached = Self.readPmset()
        readAt = Date()
        return cached
    }

    private static func readPmset() -> Int? {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/pmset")
        process.arguments = ["-g", "battlimit"]
        let pipe = Pipe()
        process.standardOutput = pipe
        process.standardError = FileHandle.nullDevice
        guard (try? process.run()) != nil else { return nil }
        let data = pipe.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()
        return parsePowerdChargeLimit(String(decoding: data, as: UTF8.self))
    }
}

/// AlDente keeps its last limit in preferences after it quits, so only trust it while it runs.
private func readAlDenteChargeLimitPercent() -> Int? {
    let running = ["com.apphousekitchen.aldente-pro", "com.davidwernhart.AlDente"].contains { bundleID in
        !NSRunningApplication.runningApplications(withBundleIdentifier: bundleID).isEmpty
    }
    guard running else { return nil }
    return readRunningAlDenteChargeLimitPercent()
}

private func readRunningAlDenteChargeLimitPercent() -> Int? {
    if let limit = readAlDenteProChargeLimitPercent() {
        return limit
    }
    return readThirdPartyChargeLimitPercent(bundleID: "com.davidwernhart.AlDente", key: "chargeVal")
}

private func readAlDenteProChargeLimitPercent() -> Int? {
    let bundleID = "com.apphousekitchen.aldente-pro"
    if readPreferenceBool(domain: bundleID, key: "useTahoeNativeLimit") == true {
        return nil
    }
    return readThirdPartyChargeLimitPercent(bundleID: bundleID, key: "chargeVal")
}

private func readThirdPartyChargeLimitPercent(bundleID: String, key: String) -> Int? {
    normalizedChargeLimitPercent(readPreferenceNumber(domain: bundleID, key: key))
}

/// Third-party apps may write plists directly without cfprefsd seeing them, so fall back to the file.
private func readPreferenceValue(domain: String, key: String) -> Any? {
    if let value = CFPreferencesCopyAppValue(key as CFString, domain as CFString) {
        return value
    }
    let plistName = domain.hasSuffix(".plist") ? domain : "\(domain).plist"
    let path = FileManager.default.homeDirectoryForCurrentUser
        .appendingPathComponent("Library/Preferences/\(plistName)").path
    return (NSDictionary(contentsOfFile: path) as? [String: Any])?[key]
}

private func readPreferenceNumber(domain: String, key: String) -> Int? {
    switch readPreferenceValue(domain: domain, key: key) {
    case let number as NSNumber: return Int(number.doubleValue.rounded())
    default: return nil
    }
}

private func readPreferenceBool(domain: String, key: String) -> Bool? {
    (readPreferenceValue(domain: domain, key: key) as? NSNumber)?.boolValue
}

private func normalizedChargeLimitPercent(_ value: Int?) -> Int? {
    guard let value, (1..<100).contains(value) else { return nil }
    return value
}