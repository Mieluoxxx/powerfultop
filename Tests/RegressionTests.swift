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
        testCellTelemetryParsing()
        testDetailDerivations()
        testSMCParamLayout()
        testFanTargets()
        testSensorGrouping()
        testTemperatureSmoothing()
        testPowerdChargeLimitParsing()
        testMetricHistoryWindow()
        testForeignFanControlDetection()
        testCPULoad()
        testMemoryAndProcessParsing()
        testDDCFraming()

        guard failures == 0 else {
            fputs("\(failures) regression test(s) failed\n", stderr)
            exit(1)
        }
        print("All PowerfulTop regression tests passed")
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

    private static func testSMCParamLayout() {
        // AppleSMC rejects every call unless the struct matches the kernel's 80-byte SMCKeyData_t.
        expectEqual(MemoryLayout<SMCParam>.size, 80, "SMCParam size")
        expectEqual(MemoryLayout<SMCParam>.offset(of: \SMCParam.keyInfo), 28, "keyInfo offset")
        expectEqual(MemoryLayout<SMCParam>.offset(of: \SMCParam.result), 40, "result offset")
        expectEqual(MemoryLayout<SMCParam>.offset(of: \SMCParam.command), 42, "command offset")
        expectEqual(MemoryLayout<SMCParam>.offset(of: \SMCParam.data32), 44, "data32 offset")
        expectEqual(MemoryLayout<SMCParam>.offset(of: \SMCParam.bytes), 48, "bytes offset")
        expectEqual(SMCParam.fourCC("F0Ac"), 0x4630_4163, "fourCC packs big-endian")
        expectEqual(SMCParam.string(0x4630_4163), "F0Ac", "fourCC round trip")

        let rpm = SMCValue(type: "flt ", bytes: SMCValue.floatBytes(2317))
        expectEqual(rpm.doubleValue, 2317, "flt round trip")
        expectEqual(SMCValue(type: "ui8 ", bytes: [3]).doubleValue, 3, "ui8 mode")
    }

    private static func testFanTargets() {
        let fan = FanReading(index: 0, actualRPM: 0, minRPM: 2317, maxRPM: 7826, targetRPM: 0, mode: 3)
        let hot = [SensorGroup.cpu: TemperatureSummary([60, 70])!]

        expectNil(fanTargetRPM(mode: .automatic, fan: fan, temperatures: hot), "automatic leaves fans to macOS")
        expectEqual(fanTargetRPM(mode: .fullBlast, fan: fan, temperatures: hot), 7826, "full blast uses the fan maximum")
        expectEqual(fanTargetRPM(mode: .constant(rpm: 1000), fan: fan, temperatures: hot), 2317, "clamp below minimum")
        expectEqual(fanTargetRPM(mode: .constant(rpm: 9000), fan: fan, temperatures: hot), 7826, "clamp above maximum")

        let ramp = FanControlMode.sensor(group: .cpu, lowerC: 50, upperC: 90)
        expectApproximately(fanTargetRPM(mode: ramp, fan: fan, temperatures: hot), 2317 + (7826 - 2317) * 0.5,
                            "70 °C is halfway between 50 and 90 °C")
        expectEqual(fanTargetRPM(mode: ramp, fan: fan, temperatures: [.cpu: TemperatureSummary([40])!]), 2317,
                    "below the ramp stays at minimum")
        expectEqual(fanTargetRPM(mode: ramp, fan: fan, temperatures: [:]), 7826,
                    "missing sensor fails towards cooling")

        let encoded = try? JSONEncoder().encode(ramp)
        expectEqual(encoded.flatMap { try? JSONDecoder().decode(FanControlMode.self, from: $0) }, ramp, "mode persists")
    }

    private static func testCPULoad() {
        let before = [
            CoreTicks(user: 100, system: 50, idle: 850, nice: 0),
            CoreTicks(user: 0, system: 0, idle: 1000, nice: 0),
        ]
        let after = [
            CoreTicks(user: 160, system: 70, idle: 870, nice: 0),   // 80 busy of 100
            CoreTicks(user: 10, system: 0, idle: 1090, nice: 0),    // 10 busy of 100
        ]
        let load = cpuLoad(previous: before, current: after, kinds: [.efficiency, .performance])
        expectApproximately(load?.usage, 0.45, "overall busy share across cores")
        expectApproximately(load?.user, 0.35, "user share")
        expectApproximately(load?.efficiency, 0.8, "efficiency cluster share")
        expectApproximately(load?.performance, 0.1, "performance cluster share")
        expectEqual(load?.perCore.count, 2, "per-core values")

        // Counters reset across sleep must not produce a negative or huge delta.
        let reset = [CoreTicks(user: 5, system: 0, idle: 5, nice: 0), CoreTicks(user: 20, system: 0, idle: 1180, nice: 0)]
        let afterReset = cpuLoad(previous: after, current: reset, kinds: [.efficiency, .performance])
        expectEqual(afterReset?.perCore.first, 0, "a core whose counters went backwards contributes nothing")
        expectNil(cpuLoad(previous: [], current: after, kinds: []), "no load without a previous sample")
    }

    private static func testDDCFraming() {
        // Reference bytes from the DDC/CI spec examples: 6E 51 82 01 10 AC and 6E 51 84 03 10 00 32 9A.
        expectEqual(DDC.getVCPPacket(0x10), [0x82, 0x01, 0x10, 0xAC], "get brightness packet")
        expectEqual(DDC.setVCPPacket(0x10, value: 50), [0x84, 0x03, 0x10, 0x00, 0x32, 0x9A], "set brightness packet")

        let reply: [UInt8] = [0x6E, 0x88, 0x02, 0x00, 0x10, 0x00, 0x00, 0x64, 0x00, 0x32, 0xF2]
        let parsed = DDC.parseVCPReply(reply, code: 0x10)
        expectEqual(parsed?.current, 50, "reply current")
        expectEqual(parsed?.max, 100, "reply max")
        // DELL U2520DR with DDC/CI off: valid checksum, zero length.
        expectNil(DDC.parseVCPReply([0x6E, 0x80, 0xBE, 0, 0, 0, 0, 0, 0, 0, 0], code: 0x10), "null message is not a reading")
        var corrupt = reply
        corrupt[9] = 0x33
        expectNil(DDC.parseVCPReply(corrupt, code: 0x10), "bad checksum")
        var unsupported = reply
        unsupported[3] = 0x01
        unsupported[10] ^= 0x01
        expectNil(DDC.parseVCPReply(unsupported, code: 0x10), "unsupported VCP code")
        expectNil(DDC.parseVCPReply(reply, code: 0x12), "reply for another VCP code")

        expectEqual(edidUUIDPrefix(vendor: 4268, model: 41299), "10AC53A1", "DELL U2520DR EDID UUID prefix")
        expectApproximately(SoftwareDimming.gammaCeiling(level: 0), SoftwareDimming.floor, "software floor")
        expectApproximately(SoftwareDimming.gammaCeiling(level: 1), 1, "software full")
        expectApproximately(SoftwareDimming.gammaCeiling(level: 2), 1, "software clamps")
    }

    private static func testMemoryAndProcessParsing() {
        let gib: UInt64 = 1 << 30
        let page: UInt64 = 16_384
        let memory = MemoryStats(
            totalBytes: 128 * gib,
            internalPages: 46 * gib / page, purgeablePages: 2 * gib / page,
            wiredPages: 7 * gib / page, compressorPages: 3 * gib / page,
            pageSize: page, swapUsedBytes: 0, pressure: .normal
        )
        expectEqual(memory.appBytes, 44 * gib, "app memory excludes purgeable")
        expectEqual(memory.usedBytes, 54 * gib, "used = app + wired + compressed")
        expectApproximately(memory.usedFraction, 54.0 / 128.0, "used fraction")

        // Live `ps -Aceo pid=,pcpu=,comm= -r` lines; names may contain spaces.
        let ps = """
          410  33.5 WindowServer
         5269  25.1 ego Helper (Renderer)
            1  20.6 launchd
        """
        let cpu = parseProcessCPU(ps, limit: 2)
        expectEqual(cpu.map(\.name), ["WindowServer", "ego Helper (Renderer)"], "top CPU processes keep spaced names")
        expectEqual(cpu.first?.value, 33.5, "CPU percent")

        // Live `top -l 1 -o mem -stats pid,command,mem` tail.
        let top = """
        Processes: 1076 total, 3 running
        PID    COMMAND         MEM
        1110   OrbStack Helper 1730M
        410    WindowServer    1461M+
        5892   WeChat          583K
        """
        let memoryList = parseProcessMemory(top, limit: 8)
        expectEqual(memoryList.map(\.name), ["OrbStack Helper", "WindowServer", "WeChat"], "top memory processes")
        expectEqual(memoryList.first?.value, 1730 * 1_048_576, "MEM megabytes")
        expectEqual(memoryList.last?.value, 583 * 1024, "MEM kilobytes")
        expectEqual(parseTopMemory("1.5G"), 1.5 * 1_073_741_824, "fractional gigabytes")
        let names = parseProcessNames("  5218 ego Helper (Renderer)\n  410 WindowServer\n")
        expectEqual(names[5218], "ego Helper (Renderer)", "full names replace top's 16-character truncation")
    }

    private static func testForeignFanControlDetection() {
        let manualFan = FanReading(index: 0, actualRPM: 5500, minRPM: 2317, maxRPM: 7826, targetRPM: 5517, mode: 1)
        let systemFan = FanReading(index: 0, actualRPM: 0, minRPM: 2317, maxRPM: 7826, targetRPM: 0, mode: 3)
        let ours = ThermalSnapshot(fans: [manualFan], temperatures: [:], diagnosticUnlocked: true)
        let idle = ThermalSnapshot(fans: [systemFan], temperatures: [:], diagnosticUnlocked: false)

        // Regression: a timed-out first request made PowerfulTop treat its own helper as a rival,
        // stop sending heartbeats, and the watchdog handed fans back every 30 s.
        expect(!isForeignFanControl(snapshot: ours, helperHoldsFans: true, otherToolRunning: false),
               "our helper's manual mode is not a conflict")
        expect(isForeignFanControl(snapshot: ours, helperHoldsFans: false, otherToolRunning: false),
               "manual mode without our marker belongs to another tool")
        expect(!isForeignFanControl(snapshot: idle, helperHoldsFans: false, otherToolRunning: false),
               "system-managed fans are free to take")
        expect(isForeignFanControl(snapshot: idle, helperHoldsFans: false, otherToolRunning: true),
               "a running fan app always counts as a conflict")
    }

    private static func testMetricHistoryWindow() {
        func sample(_ seconds: TimeInterval) -> MetricSample {
            MetricSample(date: Date(timeIntervalSince1970: seconds), batteryPercent: 80, systemPowerW: 10,
                         cpuAverageC: nil, fanRPM: nil, batteryTemperatureC: 30)
        }
        var history = MetricHistory()
        history.append(sample(0))
        history.append(sample(1))
        expectEqual(history.samples.count, 1, "bursts closer than half an interval are dropped")

        for second in stride(from: 5.0, through: 40 * 60, by: 5) {
            history.append(sample(second))
        }
        let span = (history.samples.last?.date.timeIntervalSince(history.samples.first!.date)) ?? 0
        expect(span <= MetricHistory.window, "history keeps at most 30 minutes")
        expectEqual(history.samples.count, Int(MetricHistory.window / MetricHistory.sampleInterval) + 1, "one sample per 5 s")
        expectEqual(history.series { $0.cpuAverageC }.count, 0, "missing values are skipped in a series")
        expectEqual(history.series { $0.batteryTemperatureC }.count, history.samples.count, "present values are kept")
    }

    private static func testPowerdChargeLimitParsing() {
        // Live `pmset -g battlimit` output with the native 85% limit on.
        let active = """
        Battery level limits:
        (
                {
                Terminated = 0;
                chargeSocLimitDrain = 1;
                chargeSocLimitOwner = 473;
                chargeSocLimitReason = manualChargeLimit;
                chargeSocLimitSoc = 85;
            },
                {
                chargeSocLimitReason = manualChargeLimit;
                chargeSocLimitSoc = 90;
            }
        )
        """
        expectEqual(parsePowerdChargeLimit(active), 85, "strictest powerd limit applies")
        expectNil(parsePowerdChargeLimit("No battery level limits set"), "no limit when powerd reports none")
        expectNil(parsePowerdChargeLimit("chargeSocLimitSoc = 100;"), "100% means no limit")
    }

    private static func testTemperatureSmoothing() {
        let calm = [SensorGroup.cpu: TemperatureSummary(averageC: 42, maximumC: 54, sensorCount: 112)]
        let spike = [SensorGroup.cpu: TemperatureSummary(averageC: 44, maximumC: 79, sensorCount: 112)]

        let first = smoothTemperatures(previous: [:], current: spike, elapsed: 0)
        expectEqual(first[.cpu]?.maximumC, 79, "first sample seeds the average")

        // Live trace: max jumped 54 -> 79 °C within one 2 s refresh while the average moved 2 °C.
        let damped = smoothTemperatures(previous: calm, current: spike, elapsed: 2)
        let alpha = 1 - exp(-2.0 / 10.0)
        expectApproximately(damped[.cpu]?.maximumC, 54 + 25 * alpha, "a one-sample spike moves the max by ~4.5 °C")
        expect((damped[.cpu]?.maximumC ?? 99) < 60, "spike stays damped")

        var sustained = calm
        for _ in 0..<30 {
            sustained = smoothTemperatures(previous: sustained, current: spike, elapsed: 2)
        }
        expect(abs((sustained[.cpu]?.maximumC ?? 0) - 79) < 0.1, "sustained heat converges within a minute")
    }

    private static func testSensorGrouping() {
        let groups = groupSensorKeys(["Tp00", "Te04", "Tg0K", "TB0T", "TB1T", "Tf04", "TBXT0", "TPD0", "TC0P"])
        expectEqual(groups[.cpu] ?? [], ["Tp00", "Te04"], "performance and efficiency cores")
        expectEqual(groups[.gpu] ?? [], ["Tg0K"], "GPU sensors")
        expectEqual(groups[.battery] ?? [], ["TB0T", "TB1T"], "pack thermistors only")
        expect(!isPlausibleTemperature(0) && !isPlausibleTemperature(200) && isPlausibleTemperature(45),
               "plausible temperature window")
    }

    private static func testDetailDerivations() {
        expectEqual(PowerData.batteryCurrentText(2_896), "2.90 A (Charging)", "positive IOKit amperage is charging")
        expectEqual(PowerData.batteryCurrentText(-791), "0.79 A (Discharging)", "negative IOKit amperage is discharging")
        expectEqual(PowerData.batteryCurrentText(0), "0.00 A (Idle)", "zero amperage is idle")

        let data = PowerData(
            systemPowerW: 10, batteryPowerW: 0, acInputW: 10, batteryPercent: 85, isOnAC: true,
            designCapacityMAH: 5_600,
            cellVoltagesMV: [4_190, 4_189, 4_250],
            qmaxMAH: [6_066, 6_085, 6_112],
            batteryCellLayout: .seriesParallel(seriesCount: 3, parallelCount: 1, parallelCountKnown: true),
            batteryParallelCellCurrents: [
                BatteryParallelCellCurrent(bankID: 0, cellID: 0, currentMA: 1_000),
                BatteryParallelCellCurrent(bankID: 0, cellID: 1, currentMA: 1_200),
            ],
            chargeLimitPercent: 85,
            chargeLimitSource: .macOSSetting
        )
        expectApproximately(data.capacityFractionOfDesign(5_560), 5_560.0 / 5_600.0, "capacity relative to design")
        expectNil(data.capacityFractionOfDesign(nil), "no fraction without a value")
        expectEqual(data.seriesVoltageBalanceSummary([4_190, 4_189, 4_191]), "2 mV spread — Normal", "tight series balance")
        expectEqual(data.seriesVoltageBalanceSummary([4_190, 4_189, 4_250]), "61 mV spread — Fair", "61 mV spread is fair")
        expectEqual(
            data.parallelCurrentBalanceSummary(data.batteryParallelCellCurrents ?? []),
            "200 mA max within-group spread — Worth Checking",
            "large within-group current spread"
        )
        expectEqual(data.chargeLimitText, "85% (macOS setting)", "charge limit names its source")
        let unlimited = PowerData(systemPowerW: 10, batteryPowerW: 0, acInputW: 10, batteryPercent: 85, isOnAC: true)
        expectEqual(unlimited.chargeLimitText, "Off", "no limit reads as off")
        expectEqual(data.cellTopologyDescription, "Battery topology: 3 series groups", "1P layout reads as series groups")
        expect(data.hasCellData, "cell data present")
        expect(!data.hasLifetimeData, "no lifetime data")
    }

    private static func testCellTelemetryParsing() {
        // Mirrors this machine's registry: three series banks, one parallel cell each.
        let banks: [[String: Any]] = [(0, 4190, 6066), (1, 4189, 6085), (2, 4191, 6112)].map { id, mv, qmax in
            ["BankID": id, "BatteryData": ["CellVoltage": mv, "Qmax": qmax, "StateOfCharge": 85]]
        }
        let cells: [[String: Any]] = [(2, 1417), (0, 1112), (1, 1271)].map { bank, ma in
            ["BankID": bank, "CellID": 0, "CellData": ["CellCurrent": ma]]
        }

        let telemetry = readBatteryCellTelemetry(packBatteryData: nil, banks: banks, cells: cells)
        expectEqual(telemetry?.voltagesMV ?? [], [4190, 4189, 4191], "series voltages in bank order")
        expectEqual(telemetry?.qmaxMAH ?? [], [6066, 6085, 6112], "series Qmax in bank order")
        expectEqual(telemetry?.parallelCellCurrents.map(\.currentMA) ?? [], [1112, 1271, 1417], "cell currents sorted by bank")
        expectEqual(
            telemetry?.layout,
            .seriesParallel(seriesCount: 3, parallelCount: 1, parallelCountKnown: true),
            "3S1P topology"
        )

        let snapshot = readBatteryCapacitySnapshot(from: [:], banks: banks)
        expectEqual(snapshot.stateOfCharge, 85, "fall back to bank SoC when the pack omits it")

        let packOnly = readBatteryCellTelemetry(
            packBatteryData: ["CellVoltage": [3900, 3910], "Qmax": [5000, 5010]],
            banks: [],
            cells: []
        )
        expectEqual(packOnly?.layout, .perCellArrays, "per-cell arrays without bank nodes")
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
        // Saved styles from earlier builds map onto catalog items.
        expectEqual(MenuBarConfiguration(legacy: .batteryPercent).items, [.batteryPercent], "percent style migrates")
        expectEqual(MenuBarConfiguration(legacy: .batteryPercentPower).items, [.batteryPercent, .systemPower], "percent + power migrates")
        let powerOnly = MenuBarConfiguration(legacy: .powerOnly)
        expectEqual(powerOnly.iconStyle, .hidden, "power-only drops the icon")
        expectEqual(powerOnly.items, [.systemPower], "power-only keeps wattage")

        var context = MenuBarContext()
        context.batteryPercent = 50
        context.systemPowerW = 7.4
        context.batteryPowerW = 37.1
        context.cpuUsage = 0.05
        context.memoryUsage = 0.43
        context.cpuTemperatureC = 42.6
        context.fanRPM = 0

        var configuration = MenuBarConfiguration()
        configuration.items = [.batteryPercent, .systemPower, .batteryPower, .cpuUsage, .memoryUsage, .cpuTemperature, .fanSpeed, .adapterPower]
        let label = MenuBarLabel(configuration: configuration, context: context)
        expectEqual(label.icon, .symbol("battery.50"), "battery icon follows level")
        expectEqual(label.segments.map(\.text), [
            "50%", "\u{2007}7W", "+37W", "\u{2007}5%", "43%", "43°", "\u{2007}\u{2007}\u{2007}0",
        ], "values are padded to a fixed width; adapter power is hidden on battery")
        expectEqual(label.segments[1].symbol, "laptopcomputer", "system load carries its glyph")

        var empty = MenuBarConfiguration()
        empty.iconStyle = .hidden
        empty.items = []
        expectEqual(MenuBarLabel(configuration: empty, context: context).icon, .symbol("bolt.circle"),
                    "an empty menu bar item keeps a clickable icon")

        var drawn = configuration
        drawn.iconStyle = .iOS
        expectEqual(MenuBarLabel(configuration: drawn, context: context).icon, .battery(style: .iOS, percent: 50, charging: false),
                    "drawn styles carry the exact level")
        let legacyJSON = #"{"iconStyle":"battery","items":["batteryPercent"],"spacing":6,"updateInterval":2}"#.data(using: .utf8)!
        expectEqual((try? JSONDecoder().decode(MenuBarConfiguration.self, from: legacyJSON))?.iconStyle, .battery,
                    "saved simple style still decodes")

        context.isDataAvailable = false
        expectEqual(MenuBarLabel(configuration: configuration, context: context).icon, .symbol("exclamationmark.triangle"),
                    "warn when data is unavailable")

        let encoded = try? JSONEncoder().encode(configuration)
        expectEqual(encoded.flatMap { try? JSONDecoder().decode(MenuBarConfiguration.self, from: $0) }, configuration,
                    "configuration persists")
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
            batteryVoltageMV: 10_000,
            timestamp: timestamp,
            connectionPhase: connectionPhase,
            stateOfCharge: 50,
            remainingCapacityMAH: 5_000,
            fullChargeCapacityMAH: 5_000
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
