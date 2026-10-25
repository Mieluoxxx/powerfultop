import Foundation

/// Smooths instantaneous power for battery time estimates when macOS no longer reports AvgTimeToEmpty/Full.
struct BatteryTimeEstimator {
    private var dischargeEMA: Double?
    private var chargeEMA: Double?
    private var lastMode: Mode?
    private var lastSampleAt: Date?

    private enum Mode: Equatable {
        case batteryDischarging
        case supplementalDischarging
        case charging
    }

    /// AppleSmartBattery refreshes telemetry about once a minute while the UI polls every 2 s,
    /// so weight by elapsed sample time instead of per poll.
    private let timeConstant: TimeInterval = 180

    mutating func reset() {
        dischargeEMA = nil
        chargeEMA = nil
        lastMode = nil
        lastSampleAt = nil
    }

    /// Returns smoothed discharge/charge power (watts) for the current display snapshot.
    mutating func smoothedPower(for data: PowerData) -> (dischargeW: Double?, chargeW: Double?) {
        switch data.powerState {
        case .charging(let rate) where !data.fullyCharged:
            return (nil, updateChargeEMA(sample: rate, for: data))
        case .onBattery:
            return (updateDischargeEMA(.batteryDischarging, sample: data.systemPowerW, at: data.timestamp), nil)
        case .supplementing(let rate):
            return (updateDischargeEMA(.supplementalDischarging, sample: rate, at: data.timestamp), nil)
        case .charging, .connectingAC, .acIdle:
            reset()
            return (nil, nil)
        }
    }

    private mutating func updateDischargeEMA(_ mode: Mode, sample instant: Double, at sampleAt: Date) -> Double? {
        switchMode(mode)
        guard instant >= 0.2, let elapsed = consumeSample(at: sampleAt) else { return dischargeEMA }
        dischargeEMA = ema(previous: dischargeEMA, sample: instant, elapsed: elapsed)
        return dischargeEMA
    }

    private mutating func updateChargeEMA(sample instant: Double, for data: PowerData) -> Double? {
        switchMode(.charging)
        if instant >= 0.2, let elapsed = consumeSample(at: data.timestamp) {
            chargeEMA = ema(previous: chargeEMA, sample: instant, elapsed: elapsed)
        }
        guard let smoothed = chargeEMA else { return nil }
        return applyChargeTaper(
            rateW: smoothed,
            stateOfCharge: data.stateOfCharge ?? data.batteryPercent,
            targetSoc: data.chargeLimitPercent
        )
    }

    private mutating func switchMode(_ mode: Mode) {
        if lastMode != mode {
            dischargeEMA = nil
            chargeEMA = nil
            lastMode = mode
            lastSampleAt = nil
        }
    }

    /// Returns elapsed time since the previous sample, or nil when the snapshot was already consumed.
    private mutating func consumeSample(at sampleAt: Date) -> TimeInterval? {
        guard let last = lastSampleAt else {
            lastSampleAt = sampleAt
            return 0
        }
        let elapsed = sampleAt.timeIntervalSince(last)
        guard elapsed > 0 else { return nil }
        lastSampleAt = sampleAt
        return elapsed
    }

    private func ema(previous: Double?, sample: Double, elapsed: TimeInterval) -> Double {
        guard let previous else { return sample }
        let alpha = 1 - exp(-elapsed / timeConstant)
        return previous + alpha * (sample - previous)
    }

    /// Reduce effective charge rate near the charge target to approximate constant-voltage taper.
    private func applyChargeTaper(rateW: Double, stateOfCharge: Int, targetSoc: Int) -> Double {
        let clampedTarget = min(max(targetSoc, 1), 100)
        let taperStart = max(65, clampedTarget - 15)
        guard stateOfCharge > taperStart, clampedTarget > taperStart else { return rateW }
        let span = Double(clampedTarget - taperStart)
        let taper = max(0.25, 1.0 - Double(stateOfCharge - taperStart) / span * 0.75)
        return rateW * taper
    }
}
