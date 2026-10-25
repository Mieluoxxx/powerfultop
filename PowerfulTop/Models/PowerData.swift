import Foundation

enum PowerDataSource {
    case telemetry
    case legacy
}

/// How macOS exposes per-cell battery telemetry on this machine.
enum BatteryCellTelemetryLayout: Equatable {
    /// Pack-level `CellVoltage` / `Qmax` arrays — one entry per physical cell.
    case perCellArrays
    /// Bank/cell nodes — voltage and Qmax per series group, currents per parallel cell.
    /// `parallelCountKnown` is false when no cell nodes were found (topology shows series groups only).
    case seriesParallel(seriesCount: Int, parallelCount: Int, parallelCountKnown: Bool)
}

struct BatteryParallelCellCurrent: Equatable {
    let bankID: Int
    let cellID: Int
    let currentMA: Int
}

struct PowerData {
    // Core power metrics
    let systemPowerW: Double          // SystemLoad / 1000 — actual system consumption
    let batteryPowerW: Double         // BatteryPower / 1000 — positive=discharge, negative=charge
    let acInputW: Double              // SystemPowerIn / 1000 — total DC from charger
    let acAdapterWattage: Int
    /// Negotiated USB-PD contract from `AdapterDetails`.
    let adapterRatedVoltageMV: Int?
    let adapterRatedCurrentMA: Int?
    let batteryPercent: Int
    let isOnAC: Bool
    let isCharging: Bool
    let fullyCharged: Bool
    let wallPowerW: Double?
    let adapterEfficiencyLossW: Double?
    let systemVoltageMV: Int?
    let systemCurrentMA: Int?
    let batteryVoltageMV: Int?
    let batteryAmperageMA: Int?
    let batteryTemperatureC: Double?
    let cycleCount: Int?
    let adapterDescription: String?
    let dataSource: PowerDataSource
    let timestamp: Date
    private(set) var connectionPhase: PowerConnectionPhase

    // Battery health
    let batteryHealthPercent: Int?
    let designCapacityMAH: Int?
    let rawMaxCapacityMAH: Int?
    let nominalChargeCapacityMAH: Int?
    let designCycleCount: Int?

    // Charging details
    let chargingVoltageMV: Int?
    let chargingCurrentMA: Int?
    let notChargingReason: Int?
    let vacVoltageLimit: Int?

    // Cell-level data
    let cellVoltagesMV: [Int]?
    let stateOfCharge: Int?
    let qmaxMAH: [Int]?
    let batteryCellLayout: BatteryCellTelemetryLayout?
    let batteryParallelCellCurrents: [BatteryParallelCellCurrent]?
    let dailyMinSoc: Int?
    let dailyMaxSoc: Int?
    /// Effective charge target (1–100). 100 means physical full charge.
    let chargeLimitPercent: Int
    let chargeLimitSource: BatteryChargeLimitSource

    // Lifetime statistics
    let totalOperatingTimeMin: Int?
    let lifetimeMaxTempC: Int?
    let lifetimeMinTempC: Int?
    let lifetimeAvgTempC: Int?
    let lifetimeMaxPackVoltageMV: Int?
    let lifetimeMinPackVoltageMV: Int?
    let lifetimeMaxChargeCurrentMA: Int?
    let lifetimeMaxDischargeCurrentMA: Int?

    // Device info
    let batterySerial: String?
    let deviceName: String?
    let instantAmperageMA: Int?
    let atCriticalLevel: Bool?
    let permanentFailureStatus: Int?
    let batteryCellDisconnectCount: Int?

    // Battery timing and capacity (from IOKit; display via computed helpers below)
    let avgTimeToEmptyMinutes: Int?
    let avgTimeToFullMinutes: Int?
    let remainingCapacityMAH: Int?
    let fullChargeCapacityMAH: Int?
    let averageSystemPowerW: Double?
    let batteryManufactureDate: String?
    /// EMA-smoothed discharge/charge power for time estimates (set by PowerMonitor).
    private(set) var smoothedDischargePowerW: Double?
    private(set) var smoothedChargePowerW: Double?

    // Full memberwise init (package-internal; centralizes all assignments to avoid drift across call sites)
    init(
        systemPowerW: Double, batteryPowerW: Double, acInputW: Double,
        acAdapterWattage: Int = 0,
        adapterRatedVoltageMV: Int? = nil, adapterRatedCurrentMA: Int? = nil,
        batteryPercent: Int,
        isOnAC: Bool, isCharging: Bool = false, fullyCharged: Bool = false,
        wallPowerW: Double? = nil, adapterEfficiencyLossW: Double? = nil,
        systemVoltageMV: Int? = nil, systemCurrentMA: Int? = nil,
        batteryVoltageMV: Int? = nil, batteryAmperageMA: Int? = nil,
        batteryTemperatureC: Double? = nil, cycleCount: Int? = nil,
        adapterDescription: String? = nil, dataSource: PowerDataSource = .telemetry, timestamp: Date = Date(),
        connectionPhase: PowerConnectionPhase = .onBattery,
        batteryHealthPercent: Int? = nil, designCapacityMAH: Int? = nil, rawMaxCapacityMAH: Int? = nil,
        nominalChargeCapacityMAH: Int? = nil, designCycleCount: Int? = nil,
        chargingVoltageMV: Int? = nil, chargingCurrentMA: Int? = nil, notChargingReason: Int? = nil, vacVoltageLimit: Int? = nil,
        cellVoltagesMV: [Int]? = nil, stateOfCharge: Int? = nil, qmaxMAH: [Int]? = nil,
        batteryCellLayout: BatteryCellTelemetryLayout? = nil, batteryParallelCellCurrents: [BatteryParallelCellCurrent]? = nil,
        dailyMinSoc: Int? = nil, dailyMaxSoc: Int? = nil,
        chargeLimitPercent: Int = 100, chargeLimitSource: BatteryChargeLimitSource = .none,
        totalOperatingTimeMin: Int? = nil,
        lifetimeMaxTempC: Int? = nil, lifetimeMinTempC: Int? = nil, lifetimeAvgTempC: Int? = nil,
        lifetimeMaxPackVoltageMV: Int? = nil, lifetimeMinPackVoltageMV: Int? = nil,
        lifetimeMaxChargeCurrentMA: Int? = nil, lifetimeMaxDischargeCurrentMA: Int? = nil,
        batterySerial: String? = nil, deviceName: String? = nil,
        instantAmperageMA: Int? = nil, atCriticalLevel: Bool? = nil,
        permanentFailureStatus: Int? = nil, batteryCellDisconnectCount: Int? = nil,
        avgTimeToEmptyMinutes: Int? = nil, avgTimeToFullMinutes: Int? = nil,
        remainingCapacityMAH: Int? = nil, fullChargeCapacityMAH: Int? = nil,
        averageSystemPowerW: Double? = nil, batteryManufactureDate: String? = nil,
        smoothedDischargePowerW: Double? = nil, smoothedChargePowerW: Double? = nil
    ) {
        self.systemPowerW = systemPowerW
        self.batteryPowerW = batteryPowerW
        self.acInputW = acInputW
        self.acAdapterWattage = acAdapterWattage
        self.adapterRatedVoltageMV = adapterRatedVoltageMV
        self.adapterRatedCurrentMA = adapterRatedCurrentMA
        self.batteryPercent = batteryPercent
        self.isOnAC = isOnAC
        self.isCharging = isCharging
        self.fullyCharged = fullyCharged
        self.wallPowerW = wallPowerW
        self.adapterEfficiencyLossW = adapterEfficiencyLossW
        self.systemVoltageMV = systemVoltageMV
        self.systemCurrentMA = systemCurrentMA
        self.batteryVoltageMV = batteryVoltageMV
        self.batteryAmperageMA = batteryAmperageMA
        self.batteryTemperatureC = batteryTemperatureC
        self.cycleCount = cycleCount
        self.adapterDescription = adapterDescription
        self.dataSource = dataSource
        self.timestamp = timestamp
        self.connectionPhase = connectionPhase
        self.batteryHealthPercent = batteryHealthPercent
        self.designCapacityMAH = designCapacityMAH
        self.rawMaxCapacityMAH = rawMaxCapacityMAH
        self.nominalChargeCapacityMAH = nominalChargeCapacityMAH
        self.designCycleCount = designCycleCount
        self.chargingVoltageMV = chargingVoltageMV
        self.chargingCurrentMA = chargingCurrentMA
        self.notChargingReason = notChargingReason
        self.vacVoltageLimit = vacVoltageLimit
        self.cellVoltagesMV = cellVoltagesMV
        self.stateOfCharge = stateOfCharge
        self.qmaxMAH = qmaxMAH
        self.batteryCellLayout = batteryCellLayout
        self.batteryParallelCellCurrents = batteryParallelCellCurrents
        self.dailyMinSoc = dailyMinSoc
        self.dailyMaxSoc = dailyMaxSoc
        self.chargeLimitPercent = min(max(chargeLimitPercent, 1), 100)
        self.chargeLimitSource = chargeLimitSource
        self.totalOperatingTimeMin = totalOperatingTimeMin
        self.lifetimeMaxTempC = lifetimeMaxTempC
        self.lifetimeMinTempC = lifetimeMinTempC
        self.lifetimeAvgTempC = lifetimeAvgTempC
        self.lifetimeMaxPackVoltageMV = lifetimeMaxPackVoltageMV
        self.lifetimeMinPackVoltageMV = lifetimeMinPackVoltageMV
        self.lifetimeMaxChargeCurrentMA = lifetimeMaxChargeCurrentMA
        self.lifetimeMaxDischargeCurrentMA = lifetimeMaxDischargeCurrentMA
        self.batterySerial = batterySerial
        self.deviceName = deviceName
        self.instantAmperageMA = instantAmperageMA
        self.atCriticalLevel = atCriticalLevel
        self.permanentFailureStatus = permanentFailureStatus
        self.batteryCellDisconnectCount = batteryCellDisconnectCount
        self.avgTimeToEmptyMinutes = avgTimeToEmptyMinutes
        self.avgTimeToFullMinutes = avgTimeToFullMinutes
        self.remainingCapacityMAH = remainingCapacityMAH
        self.fullChargeCapacityMAH = fullChargeCapacityMAH
        self.averageSystemPowerW = averageSystemPowerW
        self.batteryManufactureDate = batteryManufactureDate
        self.smoothedDischargePowerW = smoothedDischargePowerW
        self.smoothedChargePowerW = smoothedChargePowerW
    }

    /// Percent as macOS shows it in the menu bar, clamped for display.
    var displayBatteryPercent: Int {
        min(100, max(0, batteryPercent))
    }

    // MARK: - Power state

    /// The one classification every view and estimate derives from.
    var powerState: PowerState {
        // ExternalConnected flips before telemetry refreshes; trust the phase machine first.
        if connectionPhase == .onBattery || !isOnAC { return .onBattery }
        if connectionPhase == .connectingAC { return .connectingAC }
        if batteryPowerW > PowerThresholds.batteryFlowW {
            // Battery outflow without measured AC input is a stale snapshot, not a supplement.
            return acInputW > 0 ? .supplementing(rateW: batteryPowerW) : .acIdle
        }
        if batteryPowerW < -PowerThresholds.batteryFlowW { return .charging(rateW: -batteryPowerW) }
        // Trickle charging near the charge limit reports IsCharging with negligible flow.
        if isCharging { return .charging(rateW: max(0, -batteryPowerW)) }
        return .acIdle
    }

    var isConnectingAC: Bool {
        powerState == .connectingAC
    }

    var effectiveIsOnAC: Bool {
        powerState != .onBattery
    }

    /// Plugged in and past the connecting window, so adapter readings are current.
    var hasSettledAC: Bool {
        switch powerState {
        case .acIdle, .charging, .supplementing: return true
        case .onBattery, .connectingAC: return false
        }
    }

    var isBatteryCharging: Bool {
        if case .charging = powerState { return true }
        return false
    }

    var isSupplementalDischarge: Bool {
        if case .supplementing = powerState { return true }
        return false
    }

    /// Battery charge rate in watts (positive value).
    var batteryChargeRateW: Double {
        if case .charging(let rate) = powerState { return rate }
        return 0
    }

    /// How much power the battery contributes during supplemental discharge.
    var batterySupplementalW: Double {
        if case .supplementing(let rate) = powerState { return rate }
        return 0
    }

    /// AC input once the connection has settled; zero while unplugged or connecting.
    var effectiveACInputW: Double {
        switch powerState {
        case .onBattery, .connectingAC: return 0
        case .acIdle, .charging, .supplementing: return acInputW
        }
    }

    /// Charger output, falling back to load + charge when SystemPowerIn is unavailable.
    var effectiveACOutputW: Double {
        switch powerState {
        case .onBattery, .connectingAC:
            return 0
        case .acIdle, .supplementing:
            return acInputW > 0 ? acInputW : systemPowerW
        case .charging(let rate):
            return acInputW > 0 ? acInputW : systemPowerW + rate
        }
    }

    var powerSourceDescription: String {
        switch powerState {
        case .onBattery: return String(localized: "Battery Discharging")
        case .connectingAC: return String(localized: "AC Connecting")
        case .acIdle: return String(localized: "AC Powered")
        case .charging: return String(localized: "AC Charging")
        case .supplementing: return String(localized: "AC + Battery Supplement")
        }
    }

    /// Raw system load shown in the menu bar across all power states.
    var menuBarPowerW: Double {
        systemPowerW
    }

    /// Rounded menu bar power, capped at 99 W.
    var menuBarPowerRoundedW: Int {
        min(99, max(0, Int(menuBarPowerW.rounded())))
    }

    /// Whether actual menu bar power exceeds the display cap.
    var menuBarPowerExceedsCap: Bool {
        menuBarPowerW > 99
    }

    /// Supplemental discharge while on AC — battery still discharging despite charger connected.
    var menuBarPowerShowsBatteryWarning: Bool {
        isSupplementalDischarge
    }

    /// Whether the menu bar should prefix a warning symbol (colors are ignored in the menu bar).
    var menuBarPowerShowsWarning: Bool {
        menuBarPowerShowsBatteryWarning || menuBarPowerExceedsCap
    }

    /// Compact menu bar label text, e.g. "19W" or "⚠ 33W".
    var menuBarPowerText: String {
        let power = "\(menuBarPowerRoundedW)W"
        if menuBarPowerShowsWarning { return "⚠ \(power)" }
        return power
    }

    var validAvgTimeToEmptyMinutes: Int? {
        isValidBatteryTimeMinutes(avgTimeToEmptyMinutes)
    }

    var validAvgTimeToFullMinutes: Int? {
        isValidBatteryTimeMinutes(avgTimeToFullMinutes)
    }

    /// Remaining stored energy (Wh) from coulomb count × pack voltage.
    var remainingEnergyWh: Double? {
        if let remaining = remainingCapacityMAH, let voltage = batteryVoltageMV, remaining > 0, voltage > 0 {
            return Double(remaining) * Double(voltage) / 1_000_000.0
        }
        guard let capacity = fullChargeCapacityMAH ?? rawMaxCapacityMAH ?? nominalChargeCapacityMAH,
              capacity > 0, let voltage = batteryVoltageMV, voltage > 0 else { return nil }
        let soc = stateOfCharge ?? batteryPercent
        guard soc > 0 else { return nil }
        return Double(capacity) * Double(soc) / 100.0 * Double(voltage) / 1_000_000.0
    }

    var isAtChargeLimit: Bool {
        guard isBatteryCharging else { return false }
        let currentSoc = stateOfCharge ?? batteryPercent
        guard currentSoc >= chargeLimitPercent else { return false }
        // SOC can read above the hold point while the pack is still accepting charge.
        return batteryChargeRateW < 1.0
    }

    /// Energy still needed to reach the effective charge target (Wh).
    var energyToFullWh: Double? {
        guard isBatteryCharging, !isAtChargeLimit else { return nil }
        guard let full = fullChargeCapacityMAH ?? rawMaxCapacityMAH ?? nominalChargeCapacityMAH,
              full > 0,
              let voltage = batteryVoltageMV, voltage > 0 else { return nil }

        let currentSoc = stateOfCharge ?? batteryPercent
        guard currentSoc < chargeLimitPercent else { return nil }

        let targetCapacityMAH = full * chargeLimitPercent / 100
        if let remaining = remainingCapacityMAH, remaining > 0 {
            let deltaMAH = targetCapacityMAH - remaining
            if deltaMAH > 0 {
                return Double(deltaMAH) * Double(voltage) / 1_000_000.0
            }
        }

        let socDelta = chargeLimitPercent - currentSoc
        if socDelta > 0 {
            return Double(full) * Double(socDelta) / 100.0 * Double(voltage) / 1_000_000.0
        }
        return nil
    }

    private var dischargePowerForEstimateW: Double? {
        if let smoothed = smoothedDischargePowerW, smoothed >= 0.2 { return smoothed }
        if isSupplementalDischarge {
            let power = batterySupplementalW
            return power >= 0.2 ? power : nil
        }
        if !effectiveIsOnAC {
            return systemPowerW >= 0.2 ? systemPowerW : nil
        }
        return nil
    }

    private var chargePowerForEstimateW: Double? {
        if let smoothed = smoothedChargePowerW, smoothed >= 0.2 { return smoothed }
        let rate = batteryChargeRateW
        return rate >= 0.2 ? rate : nil
    }

    private var computedTimeToEmptyMinutes: Int? {
        guard !isConnectingAC, !isBatteryCharging else { return nil }
        guard !effectiveIsOnAC || isSupplementalDischarge else { return nil }
        guard let energyWh = remainingEnergyWh, let powerW = dischargePowerForEstimateW else { return nil }
        return Self.minutesFromEnergy(energyWh: energyWh, powerW: powerW)
    }

    private var computedTimeToFullMinutes: Int? {
        guard let energyWh = energyToFullWh, let powerW = chargePowerForEstimateW else { return nil }
        return Self.minutesFromEnergy(energyWh: energyWh, powerW: powerW)
    }

    /// Scale macOS time-to-full (100%) down when a lower charge target applies.
    private var scaledAvgTimeToFullForLimit: Int? {
        guard let avg = validAvgTimeToFullMinutes else { return nil }
        guard chargeLimitPercent < 100 else { return avg }
        let currentSoc = stateOfCharge ?? batteryPercent
        guard currentSoc < chargeLimitPercent else { return nil }
        let socToFull = 100 - currentSoc
        let socToTarget = chargeLimitPercent - currentSoc
        guard socToFull > 0, socToTarget > 0 else { return nil }
        return Self.cappedMinutes(avg * socToTarget / socToFull)
    }

    var estimatedTimeRemainingMinutes: Int? {
        if isBatteryCharging {
            if isAtChargeLimit { return nil }
            if chargeLimitPercent < 100 {
                return computedTimeToFullMinutes
                    ?? scaledAvgTimeToFullForLimit
                    ?? validAvgTimeToFullMinutes
            }
            return validAvgTimeToFullMinutes ?? computedTimeToFullMinutes
        }
        if !effectiveIsOnAC || isSupplementalDischarge {
            return validAvgTimeToEmptyMinutes ?? computedTimeToEmptyMinutes
        }
        return nil
    }

    /// True when macOS did not supply a valid AvgTimeTo* value and we used our own model.
    var estimatedTimeIsComputed: Bool {
        guard estimatedTimeRemainingMinutes != nil else { return false }
        if isBatteryCharging {
            if computedTimeToFullMinutes != nil { return true }
            if chargeLimitPercent < 100, scaledAvgTimeToFullForLimit != nil { return true }
            return validAvgTimeToFullMinutes == nil
        }
        return validAvgTimeToEmptyMinutes == nil
    }

    var estimatedTimeRemainingText: String? {
        guard let minutes = estimatedTimeRemainingMinutes else { return nil }
        return Self.formatDuration(minutes: minutes)
    }

    var estimatedTimeRemainingLabel: String {
        if isBatteryCharging {
            if chargeLimitPercent < 100 {
                return String(format: String(localized: "Est. Time to %d%%"), chargeLimitPercent)
            }
            return String(localized: "Est. Time to Full")
        }
        return String(localized: "Est. Time to Empty")
    }

    var estimatedTimeFootnote: String? {
        guard isBatteryCharging, estimatedTimeIsComputed else { return nil }
        if chargeLimitPercent < 100, let source = chargeLimitSourceLabel(chargeLimitSource) {
            return String(
                format: String(localized: "Estimated to %d%% charge limit (%@)."),
                chargeLimitPercent,
                source
            )
        }
        return String(localized: "Estimated from remaining energy and smoothed power (macOS no longer provides this).")
    }

    private static func formatDuration(minutes: Int) -> String {
        let hours = minutes / 60
        let mins = minutes % 60
        if hours > 0 { return String(format: String(localized: "Duration %dh %dm"), hours, mins) }
        return String(format: String(localized: "Duration %dm"), mins)
    }

    private static func minutesFromEnergy(energyWh: Double, powerW: Double) -> Int? {
        guard powerW >= 0.2, energyWh > 0 else { return nil }
        return cappedMinutes(Int((energyWh / powerW) * 60.0))
    }

    private static func cappedMinutes(_ minutes: Int) -> Int {
        min(max(0, minutes), 99 * 60 + 59)
    }

    var notChargingReasonDescription: String? {
        guard let reason = notChargingReason, reason != 0 else { return nil }
        // Check individual bits/flags
        if reason & 0x01000000 != 0 { return String(localized: "Optimized Battery Charging") }
        if reason & 0x00000002 != 0 { return String(localized: "Temperature Too High") }
        if reason & 0x00000001 != 0 { return String(localized: "Battery Fully Charged") }
        if reason & 0x00000004 != 0 { return String(localized: "Battery Abnormal") }
        if reason & 0x00000080 != 0 { return String(localized: "Charger Power Insufficient") }
        // Fallback for unknown codes
        return String(format: String(localized: "Reason Code %1$@ (0x%2$@)"), "\(reason)", String(format: "%X", reason))
    }

    static let empty = PowerData(systemPowerW: 0, batteryPowerW: 0, acInputW: 0, batteryPercent: 0, isOnAC: false)

    func withConnectionPhase(_ phase: PowerConnectionPhase) -> PowerData {
        var copy = self
        copy.connectionPhase = phase
        return copy
    }

    func withSmoothedPower(dischargeW: Double?, chargeW: Double?) -> PowerData {
        var copy = self
        copy.smoothedDischargePowerW = dischargeW
        copy.smoothedChargePowerW = chargeW
        return copy
    }
}
