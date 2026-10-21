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
    /// Same label with the wattage padded to two digits, so the item keeps its width
    /// when power crosses 10 W instead of shifting neighbouring menu bar items.
    let widthTemplate: String

    init(style: MenuBarDisplayStyle, isDataAvailable: Bool, data: PowerData) {
        guard isDataAvailable else {
            symbolName = "exclamationmark.triangle"
            title = ""
            widthTemplate = ""
            return
        }
        let batteryPercent = min(100, max(0, data.batteryPercent))
        let percentage = "\(batteryPercent)%"
        let power = data.menuBarPowerText
        let paddedPower = data.menuBarPowerRoundedW < 10
            ? power.replacingOccurrences(of: "\(data.menuBarPowerRoundedW)W", with: "88W")
            : power
        let batterySymbol = data.effectiveIsOnAC ? "bolt.fill" : Self.batteryIconName(for: batteryPercent)

        switch style {
        case .batteryPercent:
            symbolName = batterySymbol
            title = percentage
            widthTemplate = percentage
        case .batteryPercentPower:
            symbolName = batterySymbol
            title = "\(percentage) \(power)"
            widthTemplate = "\(percentage) \(paddedPower)"
        case .powerOnly:
            symbolName = nil
            title = power
            widthTemplate = paddedPower
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
