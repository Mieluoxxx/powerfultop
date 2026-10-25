import Foundation

/// Pre-catalog menu bar styles, kept only to migrate saved preferences.
enum MenuBarDisplayStyle: String {
    case batteryPercent
    case batteryPercentPower
    case powerOnly
}

enum MenuBarIconStyle: String, CaseIterable, Codable, Identifiable {
    case hidden
    /// SF Symbols battery in five steps, or a bolt while on AC. Raw value predates the other styles.
    case battery
    /// Outline filled to the exact level, like the macOS battery item.
    case macOS
    /// Same outline with a green/red fill.
    case macOSColor
    /// Solid battery with the percentage printed inside, like iOS.
    case iOS
    /// Small ring that fills with the level.
    case ring
    case appIcon

    var id: String { rawValue }

    var title: String {
        switch self {
        case .hidden: return String(localized: "Hidden")
        case .battery: return String(localized: "Simple")
        case .macOS: return String(localized: "macOS Native")
        case .macOSColor: return String(localized: "macOS Color")
        case .iOS: return String(localized: "iOS Native")
        case .ring: return String(localized: "Ring")
        case .appIcon: return String(localized: "PowerfulTop Icon")
        }
    }

    var symbol: String {
        switch self {
        case .hidden: return "eye.slash"
        case .battery: return "battery.75percent"
        case .macOS, .macOSColor, .iOS: return "battery.100"
        case .ring: return "circle.dashed"
        case .appIcon: return "bolt.circle"
        }
    }
}

enum MenuBarIcon: Equatable {
    case symbol(String)
    /// Drawn per refresh because it reflects the exact level rather than five symbol steps.
    case battery(style: MenuBarIconStyle, percent: Int, charging: Bool)
}

enum MenuBarItemGroup: String, CaseIterable, Identifiable {
    case battery, adapter, health, system

    var id: String { rawValue }

    var title: String {
        switch self {
        case .battery: return String(localized: "Battery")
        case .adapter: return String(localized: "Power Adapter")
        case .health: return String(localized: "Battery Health")
        case .system: return String(localized: "System")
        }
    }
}

enum MenuBarItem: String, CaseIterable, Codable, Identifiable {
    case batteryPercent
    case timeRemaining
    case batteryTemperature
    case batteryCurrent
    case batteryVoltage
    case batteryPower
    case systemPower
    case adapterCurrent
    case adapterVoltage
    case adapterPower
    case health
    case cycleCount
    case cpuUsage
    case memoryUsage
    case cpuTemperature
    case fanSpeed

    var id: String { rawValue }

    var group: MenuBarItemGroup {
        switch self {
        case .batteryPercent, .timeRemaining, .batteryTemperature, .batteryCurrent, .batteryVoltage, .batteryPower, .systemPower:
            return .battery
        case .adapterCurrent, .adapterVoltage, .adapterPower:
            return .adapter
        case .health, .cycleCount:
            return .health
        case .cpuUsage, .memoryUsage, .cpuTemperature, .fanSpeed:
            return .system
        }
    }

    var title: String {
        switch self {
        case .batteryPercent: return String(localized: "Percentage")
        case .timeRemaining: return String(localized: "Time Remaining")
        case .batteryTemperature: return String(localized: "Temperature")
        case .batteryCurrent, .adapterCurrent: return String(localized: "Current")
        case .batteryVoltage, .adapterVoltage: return String(localized: "Voltage")
        case .batteryPower, .adapterPower: return String(localized: "Power")
        case .systemPower: return String(localized: "System Load")
        case .health: return String(localized: "Health")
        case .cycleCount: return String(localized: "Cycle Count")
        case .cpuUsage: return String(localized: "CPU Usage")
        case .memoryUsage: return String(localized: "Memory")
        case .cpuTemperature: return String(localized: "CPU Temperature")
        case .fanSpeed: return String(localized: "Fan Speed")
        }
    }

    /// Glyph shown before the value so several numbers in the menu bar stay distinguishable.
    var symbol: String? {
        switch self {
        case .batteryPercent: return nil
        case .timeRemaining: return "clock"
        case .batteryTemperature: return "thermometer.medium"
        case .batteryCurrent, .batteryVoltage, .batteryPower: return "battery.100"
        case .systemPower: return "laptopcomputer"
        case .adapterCurrent, .adapterVoltage, .adapterPower: return "powerplug"
        case .health: return "heart"
        case .cycleCount: return "arrow.triangle.2.circlepath"
        case .cpuUsage: return "cpu"
        case .memoryUsage: return "memorychip"
        case .cpuTemperature: return "thermometer.variable"
        case .fanSpeed: return "fan"
        }
    }
}

struct MenuBarConfiguration: Codable, Equatable {
    var iconStyle: MenuBarIconStyle = .battery
    var items: [MenuBarItem] = [.batteryPercent]
    /// Points between items.
    var spacing: Double = 6
    /// Seconds between menu bar refreshes; fewer redraws keep the item steady and cheaper.
    var updateInterval: Double = 2

    static let `default` = MenuBarConfiguration()
    static let spacingRange: ClosedRange<Double> = 0...12
    static let updateIntervalRange: ClosedRange<Double> = 2...20

    init() {}

    init(legacy style: MenuBarDisplayStyle) {
        switch style {
        case .batteryPercent:
            items = [.batteryPercent]
        case .batteryPercentPower:
            items = [.batteryPercent, .systemPower]
        case .powerOnly:
            iconStyle = .hidden
            items = [.systemPower]
        }
    }
}

/// Everything the menu bar can show, captured from the monitors so labels are pure and testable.
struct MenuBarContext: Equatable {
    var isDataAvailable = true
    var isOnAC = false
    var batteryPercent = 0
    var timeRemaining: String?
    var batteryTemperatureC: Double?
    var batteryCurrentMA: Int?
    var batteryVoltageMV: Int?
    /// Signed: positive into the pack.
    var batteryPowerW: Double?
    var systemPowerW = 0.0
    var systemPowerWarning = false
    var adapterCurrentA: Double?
    var adapterVoltageV: Double?
    var adapterPowerW: Double?
    var health: Int?
    var cycleCount: Int?
    var cpuUsage: Double?
    var memoryUsage: Double?
    var cpuTemperatureC: Double?
    var fanRPM: Double?
}

struct MenuBarSegment: Equatable {
    let symbol: String?
    let text: String
}

/// Content for the status-item button; drawn by `StatusItemController`.
struct MenuBarLabel: Equatable {
    let icon: MenuBarIcon?
    let segments: [MenuBarSegment]

    init(configuration: MenuBarConfiguration, context: MenuBarContext) {
        guard context.isDataAvailable else {
            icon = .symbol("exclamationmark.triangle")
            segments = []
            return
        }
        let segments = configuration.items.compactMap { Self.segment(for: $0, context: context) }
        self.segments = segments
        let percent = min(100, max(0, context.batteryPercent))
        switch configuration.iconStyle {
        case .hidden:
            // An empty status item cannot be clicked, which would lock users out of the settings.
            icon = segments.isEmpty ? .symbol("bolt.circle") : nil
        case .battery:
            icon = .symbol(context.isOnAC ? "bolt.fill" : Self.batteryIconName(for: percent))
        case .appIcon:
            icon = .symbol("bolt.circle")
        case .macOS, .macOSColor, .iOS, .ring:
            icon = .battery(style: configuration.iconStyle, percent: percent, charging: context.isOnAC)
        }
    }

    /// Nil when the reading does not apply right now (e.g. adapter values on battery).
    static func segment(for item: MenuBarItem, context: MenuBarContext) -> MenuBarSegment? {
        let text: String?
        switch item {
        case .batteryPercent:
            text = "\(min(100, max(0, context.batteryPercent)))%"
        case .timeRemaining:
            text = context.timeRemaining
        case .batteryTemperature:
            text = context.batteryTemperatureC.map { padded(Int($0.rounded()), digits: 2) + "°" }
        case .batteryCurrent:
            text = context.batteryCurrentMA.map { String(format: "%.1fA", Double($0) / 1000) }
        case .batteryVoltage:
            text = context.batteryVoltageMV.map { String(format: "%.1fV", Double($0) / 1000) }
        case .batteryPower:
            text = context.batteryPowerW.map { watts in
                (watts > 0.05 ? "+" : (watts < -0.05 ? "−" : "")) + padded(Int(abs(watts).rounded()), digits: 2) + "W"
            }
        case .systemPower:
            let rounded = min(99, max(0, Int(context.systemPowerW.rounded())))
            text = (context.systemPowerWarning ? "⚠ " : "") + padded(rounded, digits: 2) + "W"
        case .adapterCurrent:
            text = context.adapterCurrentA.map { String(format: "%.1fA", $0) }
        case .adapterVoltage:
            text = context.adapterVoltageV.map { String(format: "%.0fV", $0) }
        case .adapterPower:
            text = context.adapterPowerW.map { padded(Int($0.rounded()), digits: 2) + "W" }
        case .health:
            text = context.health.map { "\($0)%" }
        case .cycleCount:
            text = context.cycleCount.map(String.init)
        case .cpuUsage:
            text = context.cpuUsage.map { padded(Int(($0 * 100).rounded()), digits: 2) + "%" }
        case .memoryUsage:
            text = context.memoryUsage.map { padded(Int(($0 * 100).rounded()), digits: 2) + "%" }
        case .cpuTemperature:
            text = context.cpuTemperatureC.map { padded(Int($0.rounded()), digits: 2) + "°" }
        case .fanSpeed:
            text = context.fanRPM.map { padded(Int($0.rounded()), digits: 4) }
        }
        return text.map { MenuBarSegment(symbol: item.symbol, text: $0) }
    }

    /// A figure space is exactly one digit wide in the monospaced-digit status font, so values keep
    /// their width as they change and neighbouring menu bar items don't shift.
    static func padded(_ value: Int, digits: Int) -> String {
        let text = String(value)
        return String(repeating: "\u{2007}", count: max(0, digits - text.count)) + text
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
