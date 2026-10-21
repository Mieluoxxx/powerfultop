import Darwin
import Foundation

@main
struct RegressionTests {
    private static var failures = 0

    static func main() {
        testSupplementalEstimateUsesBatteryPower()
        testDischargeEstimatorResetsAcrossPowerSources()
        testMenuBarAlwaysUsesSystemPower()
        testManufactureDateDecoding()
        testPowerFlowUsesIOKitDischargeSign()
        testGapAloneIsNotSupplementalDischarge()
        testEstimatorIgnoresRepeatedSnapshots()
        testTelemetryEnergyUnits()
        testMenuBarLabelStyles()
        testPowerStateClassification()

        guard failures == 0 else {
            fputs("\(failures) regression test(s) failed\n", stderr)
            exit(1)
        }
        print("All PowerTop regression tests passed")
    }

    private static func testSupplementalEstimateUsesBatteryPower() {
        let data = makeData(
            systemPowerW: 25,
            batteryPowerW: 5,
            acInputW: 20,
            isOnAC: true,
            connectionPhase: .onAC
        )

        expect(data.isSupplementalDischarge, "detect supplemental discharge")
        expectEqual(data.batterySupplementalW, 5, "report battery contribution")
        expectEqual(data.estimatedTimeRemainingMinutes, 600, "estimate 50 Wh at 5 W as 10 hours")
    }

    private static func testDischargeEstimatorResetsAcrossPowerSources() {
        var estimator = BatteryTimeEstimator()
        let supplemental = makeData(
            systemPowerW: 25,
            batteryPowerW: 5,
            acInputW: 20,
            isOnAC: true,
            connectionPhase: .onAC
        )
        let supplementalPower = estimator.smoothedPower(for: supplemental).dischargeW
        expectApproximately(supplementalPower, 5, "smooth battery contribution during supplemental discharge")

        let batteryOnly = makeData(
            systemPowerW: 25,
            batteryPowerW: 25,
            acInputW: 0,
            isOnAC: false,
            connectionPhase: .onBattery
        )
        let batteryPower = estimator.smoothedPower(for: batteryOnly).dischargeW
        expectApproximately(batteryPower, 25, "reset EMA when switching to battery-only discharge")
    }

    private static func testMenuBarAlwaysUsesSystemPower() {
        let batteryOnly = makeData(
            systemPowerW: 18,
            batteryPowerW: 18,
            acInputW: 0,
            isOnAC: false,
            connectionPhase: .onBattery
        )
        expectEqual(batteryOnly.menuBarPowerW, 18, "show system power on battery")

        let acPowered = makeData(
            systemPowerW: 15,
            batteryPowerW: 0,
            acInputW: 15,
            isOnAC: true,
            connectionPhase: .onAC
        )
        expectEqual(acPowered.menuBarPowerW, 15, "show system power while AC powered")

        let charging = makeData(
            systemPowerW: 30,
            batteryPowerW: -10,
            acInputW: 40,
            isOnAC: true,
            isCharging: true,
            connectionPhase: .onAC
        )
        expect(charging.isBatteryCharging, "detect active charging")
        expectEqual(charging.menuBarPowerW, 30, "show system power while charging")

        let supplemental = makeData(
            systemPowerW: 25,
            batteryPowerW: 5,
            acInputW: 20,
            isOnAC: true,
            connectionPhase: .onAC
        )
        expectEqual(supplemental.menuBarPowerW, 25, "show system power during supplemental discharge")

        let chargingBelowCap = makeData(
            systemPowerW: 80,
            batteryPowerW: -40,
            acInputW: 120,
            isOnAC: true,
            isCharging: true,
            connectionPhase: .onAC
        )
        expect(!chargingBelowCap.menuBarPowerExceedsCap, "do not warn for charger input above 99 W")

        let systemAboveCap = makeData(
            systemPowerW: 120,
            batteryPowerW: -20,
            acInputW: 140,
            isOnAC: true,
            isCharging: true,
            connectionPhase: .onAC
        )
        expect(systemAboveCap.menuBarPowerExceedsCap, "warn for system power above 99 W")
        expectEqual(systemAboveCap.menuBarPowerRoundedW, 99, "cap menu bar system power at 99 W")
    }

    private static func testManufactureDateDecoding() {
        let now = ISO8601DateFormatter().date(from: "2026-08-16T12:00:00Z")!

        expectEqual(
            formatBatteryManufactureDate(52_987_853_550_643, gaugeName: "bq40z651", relativeTo: now),
            "2026-02",
            "decode the M5 issue sample at month precision"
        )
        expectEqual(
            formatBatteryManufactureDate(56_286_354_946_354, gaugeName: "BQ40Z651", relativeTo: now),
            "2021-10",
            "decode a live battery sample case-insensitively"
        )
        expectEqual(
            formatBatteryManufactureDate(packed("108043"), gaugeName: "A1964", relativeTo: now),
            "2026-08",
            "accept a valid date in the current month"
        )
        expectNil(
            formatBatteryManufactureDate(52_987_853_550_643, gaugeName: "SN7038", relativeTo: now),
            "reject a gauge with a different encoding"
        )
        expectNil(
            formatBatteryManufactureDate(packed("032033"), gaugeName: "bq40z651", relativeTo: now),
            "reject an impossible calendar date"
        )
        expectNil(
            formatBatteryManufactureDate(packed("109043"), gaugeName: "bq40z651", relativeTo: now),
            "reject a future month"
        )
        expectNil(
            formatBatteryManufactureDate(packed("21A043"), gaugeName: "bq40z651", relativeTo: now),
            "reject non-digit bytes"
        )

        let nestedProperties: [String: Any] = [
            "BatteryData": ["ManufactureDate": NSNumber(value: 52_987_853_550_643)]
        ]
        expectEqual(
            parseBatteryManufactureDate(
                from: nestedProperties,
                gaugeName: "bq40z651",
                relativeTo: now
            ),
            "2026-02",
            "read ManufactureDate from BatteryData"
        )

        let manufacturerDataOnly: [String: Any] = [
            "ManufacturerData": Data([0x04, 0x31, 0x39, 0x31, 0x34])
        ]
        expectNil(
            parseBatteryManufactureDate(
                from: manufacturerDataOnly,
                gaugeName: "bq40z651",
                relativeTo: now
            ),
            "never infer a date from ManufacturerData"
        )
    }

    private static func testPowerFlowUsesIOKitDischargeSign() {
        // Live samples: on battery IOKit reports discharge as negative Amperage/BatteryPower.
        let battery = resolvePowerFlow(
            isOnAC: false, acInputW: 0, systemLoadW: 9.751,
            ioBatteryPowerW: -9.751, ioAmperagePowerW: -9.751, systemInputW: nil
        )
        expectEqual(battery, PowerFlowReading(systemPowerW: 9.751, batteryPowerW: 9.751), "battery discharge")

        let acIdle = resolvePowerFlow(
            isOnAC: true, acInputW: 11.501, systemLoadW: 11.501,
            ioBatteryPowerW: 0, ioAmperagePowerW: nil, systemInputW: 11.498
        )
        expectEqual(acIdle, PowerFlowReading(systemPowerW: 11.501, batteryPowerW: 0), "AC idle")

        let supplement = resolvePowerFlow(
            isOnAC: true, acInputW: 20, systemLoadW: 25,
            ioBatteryPowerW: -5, ioAmperagePowerW: -5, systemInputW: 20
        )
        expectEqual(supplement, PowerFlowReading(systemPowerW: 25, batteryPowerW: 5), "AC + battery supplement")

        let charging = resolvePowerFlow(
            isOnAC: true, acInputW: 40, systemLoadW: 30,
            ioBatteryPowerW: 10, ioAmperagePowerW: 10, systemInputW: 40
        )
        expectEqual(charging, PowerFlowReading(systemPowerW: 30, batteryPowerW: -10), "AC charging")

        let liveCharging = resolvePowerFlow(
            isOnAC: true, acInputW: 48.207, systemLoadW: 11.066,
            ioBatteryPowerW: 37.141, ioAmperagePowerW: 37.141, systemInputW: 48.207
        )
        expectEqual(liveCharging, PowerFlowReading(systemPowerW: 11.066, batteryPowerW: -37.141), "live charging sample")

        // Just plugged in: ExternalConnected flips before telemetry refreshes.
        let staleAfterPlug = resolvePowerFlow(
            isOnAC: true, acInputW: 0, systemLoadW: 5.818,
            ioBatteryPowerW: -5.818, ioAmperagePowerW: -5.152, systemInputW: nil
        )
        expectEqual(staleAfterPlug, PowerFlowReading(systemPowerW: 5.818, batteryPowerW: 5.818), "stale snapshot after plug-in")

        let legacyDischarge = resolvePowerFlow(
            isOnAC: false, acInputW: 0, systemLoadW: nil,
            ioBatteryPowerW: nil, ioAmperagePowerW: -8, systemInputW: nil
        )
        expectEqual(legacyDischarge, PowerFlowReading(systemPowerW: 8, batteryPowerW: 8), "legacy amperage discharge")
    }

    private static func testGapAloneIsNotSupplementalDischarge() {
        let data = makeData(
            systemPowerW: 25,
            batteryPowerW: 0,
            acInputW: 20,
            isOnAC: true,
            connectionPhase: .onAC
        )
        expect(!data.isSupplementalDischarge, "do not infer supplement from a load/input gap")
        expectEqual(data.menuBarPowerShowsWarning, false, "no supplement warning without measured discharge")
    }

    private static func testEstimatorIgnoresRepeatedSnapshots() {
        var estimator = BatteryTimeEstimator()
        func sample(_ watts: Double, at seconds: TimeInterval) -> Double? {
            let data = makeData(
                systemPowerW: watts,
                batteryPowerW: watts,
                acInputW: 0,
                isOnAC: false,
                connectionPhase: .onBattery,
                timestamp: Date(timeIntervalSince1970: seconds)
            )
            return estimator.smoothedPower(for: data).dischargeW
        }

        expectApproximately(sample(10, at: 0), 10, "seed EMA with first snapshot")
        for _ in 0..<30 {
            _ = sample(20, at: 0)
        }
        expectApproximately(sample(20, at: 0), 10, "repeated polls of one snapshot do not move EMA")
        expectApproximately(sample(20, at: 60), 10 + 10 * (1 - exp(-60.0 / 180.0)), "weight by snapshot age")
    }

    private static func testTelemetryEnergyUnits() {
        // Live samples: idle on AC (SystemPowerIn 11501 mW) and charging (SystemPowerIn 48207 mW).
        expect(abs(telemetryEnergyToWatts(3194) - 11.501) < 0.01, "idle SystemEnergyConsumed matches SystemPowerIn")
        expect(abs(telemetryEnergyToWatts(3321) - 11.956) < 0.01, "idle wall power includes adapter loss")
        expect(abs(telemetryEnergyToWatts(13390) - 48.207) < 0.01, "charging SystemEnergyConsumed matches SystemPowerIn")
        expect(abs(telemetryEnergyToWatts(14741) - 53.068) < 0.01, "charging wall power includes adapter loss")
    }

    private static func testPowerStateClassification() {
        func state(
            system: Double, battery: Double, input: Double, onAC: Bool,
            charging: Bool = false, phase: PowerConnectionPhase
        ) -> PowerState {
            makeData(systemPowerW: system, batteryPowerW: battery, acInputW: input,
                     isOnAC: onAC, isCharging: charging, connectionPhase: phase).powerState
        }

        expectEqual(state(system: 9.751, battery: 9.751, input: 0, onAC: false, phase: .onBattery),
                    .onBattery, "battery discharge")
        expectEqual(state(system: 11.501, battery: 0, input: 11.501, onAC: true, phase: .onAC),
                    .acIdle, "AC idle at charge limit")
        expectEqual(state(system: 11.066, battery: -37.141, input: 48.207, onAC: true, charging: true, phase: .onAC),
                    .charging(rateW: 37.141), "live charging sample")
        expectEqual(state(system: 25, battery: 5, input: 20, onAC: true, phase: .onAC),
                    .supplementing(rateW: 5), "battery supplements AC")
        expectEqual(state(system: 5.818, battery: 5.818, input: 0, onAC: true, phase: .connectingAC),
                    .connectingAC, "stale discharge snapshot while connecting")
        expectEqual(state(system: 5.818, battery: 5.818, input: 0, onAC: true, phase: .onAC),
                    .acIdle, "discharge without measured AC input is not a supplement")
        expectEqual(state(system: 12, battery: -0.1, input: 12.1, onAC: true, charging: true, phase: .onAC),
                    .charging(rateW: 0.1), "trickle charge reported by IsCharging")
        expectEqual(state(system: 12, battery: 0, input: 12, onAC: false, phase: .onAC),
                    .onBattery, "ExternalConnected=false overrides a lagging phase")

        let charging = makeData(systemPowerW: 30, batteryPowerW: -10, acInputW: 0,
                                isOnAC: true, isCharging: true, connectionPhase: .onAC)
        expectEqual(charging.effectiveACOutputW, 40, "estimate charger output without SystemPowerIn")
    }

    private static func testMenuBarLabelStyles() {
        let battery = makeData(systemPowerW: 7.4, batteryPowerW: 7.4, acInputW: 0, isOnAC: false, connectionPhase: .onBattery)

        let percent = MenuBarLabel(style: .batteryPercent, isDataAvailable: true, data: battery)
        expectEqual(percent.title, "50%", "percent style hides power")
        expectEqual(percent.symbolName, "battery.50", "percent style shows battery level icon")

        let full = MenuBarLabel(style: .batteryPercentPower, isDataAvailable: true, data: battery)
        expectEqual(full.title, "50% \u{2007}7W", "pad single-digit power with a figure space")

        let powerOnly = MenuBarLabel(style: .powerOnly, isDataAvailable: true, data: battery)
        expectEqual(powerOnly.title, "\u{2007}7W", "power-only style shows padded wattage")
        expectNil(powerOnly.symbolName, "power-only style has no icon next to the system battery")

        let busy = makeData(systemPowerW: 23.6, batteryPowerW: 23.6, acInputW: 0, isOnAC: false, connectionPhase: .onBattery)
        let busyLabel = MenuBarLabel(style: .powerOnly, isDataAvailable: true, data: busy)
        expectEqual(busyLabel.title, "24W", "two-digit power needs no padding")

        let unavailable = MenuBarLabel(style: .powerOnly, isDataAvailable: false, data: battery)
        expectEqual(unavailable.symbolName, "exclamationmark.triangle", "warn when data is unavailable")
    }

    private static func packed(_ storedDigits: String) -> Int {
        storedDigits.utf8.reduce(0) { value, byte in
            value << 8 | Int(byte)
        }
    }

    private static func makeData(
        systemPowerW: Double,
        batteryPowerW: Double,
        acInputW: Double,
        isOnAC: Bool,
        isCharging: Bool = false,
        fullyCharged: Bool = false,
        connectionPhase: PowerConnectionPhase,
        timestamp: Date = Date(timeIntervalSince1970: 0)
    ) -> PowerData {
        PowerData(
            systemPowerW: systemPowerW,
            batteryPowerW: batteryPowerW,
            acInputW: acInputW,
            acAdapterWattage: 140,
            batteryPercent: 50,
            isOnAC: isOnAC,
            isCharging: isCharging,
            fullyCharged: fullyCharged,
            wallPowerW: nil,
            adapterEfficiencyLossW: nil,
            systemVoltageMV: nil,
            systemCurrentMA: nil,
            batteryVoltageMV: 10_000,
            batteryAmperageMA: nil,
            batteryTemperatureC: nil,
            cycleCount: nil,
            adapterDescription: nil,
            dataSource: .telemetry,
            timestamp: timestamp,
            connectionPhase: connectionPhase,
            batteryHealthPercent: nil,
            designCapacityMAH: nil,
            rawMaxCapacityMAH: nil,
            nominalChargeCapacityMAH: nil,
            designCycleCount: nil,
            chargingVoltageMV: nil,
            chargingCurrentMA: nil,
            notChargingReason: nil,
            vacVoltageLimit: nil,
            cellVoltagesMV: nil,
            stateOfCharge: 50,
            qmaxMAH: nil,
            batteryCellLayout: nil,
            batteryParallelCellCurrents: nil,
            dailyMinSoc: nil,
            dailyMaxSoc: nil,
            chargeLimitPercent: 100,
            chargeLimitSource: .none,
            totalOperatingTimeMin: nil,
            lifetimeMaxTempC: nil,
            lifetimeMinTempC: nil,
            lifetimeAvgTempC: nil,
            lifetimeMaxPackVoltageMV: nil,
            lifetimeMinPackVoltageMV: nil,
            lifetimeMaxChargeCurrentMA: nil,
            lifetimeMaxDischargeCurrentMA: nil,
            batterySerial: nil,
            deviceName: nil,
            instantAmperageMA: nil,
            atCriticalLevel: nil,
            permanentFailureStatus: nil,
            batteryCellDisconnectCount: nil,
            avgTimeToEmptyMinutes: nil,
            avgTimeToFullMinutes: nil,
            remainingCapacityMAH: 5_000,
            fullChargeCapacityMAH: 5_000,
            averageSystemPowerW: nil,
            batteryManufactureDate: nil,
            smoothedDischargePowerW: nil,
            smoothedChargePowerW: nil
        )
    }

    private static func expect(_ condition: @autoclosure () -> Bool, _ message: String) {
        guard condition() else {
            failures += 1
            fputs("FAIL: \(message)\n", stderr)
            return
        }
    }

    private static func expectEqual<T: Equatable>(_ actual: T, _ expected: T, _ message: String) {
        expect(actual == expected, "\(message): expected \(expected), got \(actual)")
    }

    private static func expectNil<T>(_ value: T?, _ message: String) {
        expect(value == nil, "\(message): got \(String(describing: value))")
    }

    private static func expectApproximately(_ actual: Double?, _ expected: Double, _ message: String) {
        guard let actual else {
            expect(false, "\(message): got nil")
            return
        }
        expect(abs(actual - expected) < 0.000_001, "\(message): expected \(expected), got \(actual)")
    }
}
