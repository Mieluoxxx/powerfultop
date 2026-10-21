import Foundation

enum MenuBarDisplayStyle: String, CaseIterable, Identifiable {
    case batteryPercent
    case batteryPercentPower
    /// Wattage only, meant to sit next to the system battery icon.
    case powerOnly

    var id: String { rawValue }

    var title: String {
        switch self {
        case .batteryPercent: return String(localized: "Battery + Percent")
        case .batteryPercentPower: return String(localized: "Battery + Percent + Power")
        case .powerOnly: return String(localized: "Power Only")
        }
    }
}

/// Content for the status-item button; drawn by `StatusItemController` via title/image.
struct MenuBarLabel: Equatable {
    let symbolName: String?
    let title: String

    init(style: MenuBarDisplayStyle, isDataAvailable: Bool, data: PowerData) {
        guard isDataAvailable else {
            symbolName = "exclamationmark.triangle"
            title = ""
            return
        }
        let batteryPercent = min(100, max(0, data.batteryPercent))
        let percentage = "\(batteryPercent)%"
        // A figure space is exactly one digit wide in the monospaced-digit status font, so
        // single-digit wattage keeps the item width and neighbouring menu bar items don't shift.
        let power = data.menuBarPowerRoundedW < 10
            ? data.menuBarPowerText.replacingOccurrences(
                of: "\(data.menuBarPowerRoundedW)W",
                with: "\u{2007}\(data.menuBarPowerRoundedW)W"
            )
            : data.menuBarPowerText
        let batterySymbol = data.effectiveIsOnAC ? "bolt.fill" : Self.batteryIconName(for: batteryPercent)

        switch style {
        case .batteryPercent:
            symbolName = batterySymbol
            title = percentage
        case .batteryPercentPower:
            symbolName = batterySymbol
            title = "\(percentage) \(power)"
        case .powerOnly:
            symbolName = nil
            title = power
        }
    }

    static func batteryIconName(for batteryPercent: Int) -> String {
        switch batteryPercent {
        case ...10: return "battery.0"
        case ...35: return "battery.25"
        case ...60: return "battery.50"
        case ...85: return "battery.75"
        default: return "battery.100"
        }
    }
}
