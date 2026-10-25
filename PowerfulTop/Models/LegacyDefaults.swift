import Foundation

enum LegacyDefaults {
    /// Builds before the rename stored their preferences under the upstream bundle ID.
    static let bundleID = "com.kdolphin.PowerTop"
    static let migratedKey = "migratedFromLegacyBundleID"

    /// Values the new domain doesn't already have; anything set since the rename wins.
    static func valuesToMigrate(legacy: [String: Any], current: [String: Any]) -> [String: Any] {
        legacy.filter { current[$0.key] == nil }
    }

    static func migrate(into defaults: UserDefaults = .standard) {
        guard !defaults.bool(forKey: migratedKey) else { return }
        if let legacy = defaults.persistentDomain(forName: bundleID) {
            let current = Bundle.main.bundleIdentifier.flatMap(defaults.persistentDomain(forName:)) ?? [:]
            for (key, value) in valuesToMigrate(legacy: legacy, current: current) {
                defaults.set(value, forKey: key)
            }
        }
        defaults.set(true, forKey: migratedKey)
    }
}
