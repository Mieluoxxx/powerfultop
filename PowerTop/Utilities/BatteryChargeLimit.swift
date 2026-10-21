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

private func readMacOSChargeLimitPercent() -> Int? {
    let domain = "com.apple.batteryui.charging.mac"
    let keys = [
        "com.apple.batteryui.charging.mac.limit",
        "com.apple.batteryui.charging.mac.prior.limit",
        "limit",
    ]
    for key in keys {
        if let limit = normalizedChargeLimitPercent(readPreferenceNumber(domain: domain, key: key)) {
            return limit
        }
    }
    return nil
}

private func readAlDenteChargeLimitPercent() -> Int? {
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