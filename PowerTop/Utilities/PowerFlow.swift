import Foundation

enum PowerThresholds {
    /// Battery flow below this is measurement noise rather than charge or discharge.
    static let batteryFlowW = 0.3
}

/// System and battery power resolved from one AppleSmartBattery snapshot.
struct PowerFlowReading: Equatable {
    let systemPowerW: Double
    /// PowerData convention: positive = discharge, negative = charge.
    let batteryPowerW: Double
}

/// IOKit reports `Amperage` and `BatteryPower` as negative while discharging (observed on
/// battery: Amperage -791 mA, BatteryPower -9751 mW, SystemLoad 9751 mW), so flip them here.
/// Within a snapshot `SystemLoad == SystemPowerIn - BatteryPower`, which makes SystemLoad the
/// direct system consumption in every power state.
func resolvePowerFlow(
    isOnAC: Bool,
    acInputW: Double,
    systemLoadW: Double?,
    ioBatteryPowerW: Double?,
    ioAmperagePowerW: Double?,
    systemInputW: Double?
) -> PowerFlowReading {
    let threshold = PowerThresholds.batteryFlowW
    let ioBattery: Double = {
        if let telemetry = ioBatteryPowerW, abs(telemetry) > threshold { return telemetry }
        if let amperage = ioAmperagePowerW, abs(amperage) > threshold { return amperage }
        return 0
    }()

    // Without a charger the pack can only discharge; a positive reading here is a stale snapshot.
    let batteryPowerW = isOnAC ? -ioBattery : abs(ioBattery)

    var systemPowerW: Double
    if let load = systemLoadW, load > 0 {
        systemPowerW = load
    } else if isOnAC {
        systemPowerW = max(0, acInputW + batteryPowerW)
    } else {
        systemPowerW = batteryPowerW
    }

    if systemPowerW == 0, isOnAC, let input = systemInputW, input > 0 {
        systemPowerW = input
    }

    return PowerFlowReading(systemPowerW: systemPowerW, batteryPowerW: batteryPowerW)
}
