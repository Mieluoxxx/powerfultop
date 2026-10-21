import Foundation

/// Content for the status-item button; drawn by `StatusItemController` via title/image.
struct MenuBarLabel: Equatable {
    let symbolName: String
    let title: String

    @MainActor
    init(monitor: PowerMonitor) {
        guard monitor.isDataAvailable else {
            symbolName = "exclamationmark.triangle"
            title = ""
            return
        }
        let data = monitor.currentData
        let batteryPercent = min(100, max(0, data.batteryPercent))
        let percentage = "\(batteryPercent)%"
        symbolName = data.effectiveIsOnAC ? "bolt.fill" : Self.batteryIconName(for: batteryPercent)
        title = monitor.showPowerInMenuBar ? "\(percentage) \(data.menuBarPowerText)" : percentage
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
