import SwiftUI

/// Display rules shared by the popover and details window so the two never disagree.
extension PowerData {
    var batteryLevelTint: Color {
        if isBatteryCharging { return .green }
        if displayBatteryPercent <= 20 { return .red }
        return displayBatteryPercent <= 50 ? .orange : .green
    }

    var batteryLevelSymbol: String {
        MenuBarLabel.batteryIconName(for: displayBatteryPercent)
    }

    /// Signed battery flow for display: "+" into the pack, "−" out of it.
    var signedBatteryPowerText: String {
        switch powerState {
        case .charging(let rate): return "+" + String(format: "%.1f W", rate)
        case .supplementing(let rate): return "−" + String(format: "%.1f W", rate)
        case .onBattery: return "−" + String(format: "%.1f W", systemPowerW)
        case .connectingAC, .acIdle: return String(format: "%.1f W", 0.0)
        }
    }
}
