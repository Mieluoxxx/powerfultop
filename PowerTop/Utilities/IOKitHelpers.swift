import Foundation
import IOKit

/// Properties of every registry entry matching an IOKit class, in registry order.
func ioServiceProperties(className: String) -> [[String: Any]] {
    var iterator: io_iterator_t = 0
    guard IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching(className), &iterator) == KERN_SUCCESS else {
        return []
    }
    defer { IOObjectRelease(iterator) }

    var results: [[String: Any]] = []
    while true {
        let service = IOIteratorNext(iterator)
        guard service != 0 else { break }
        defer { IOObjectRelease(service) }
        var properties: Unmanaged<CFMutableDictionary>?
        guard IORegistryEntryCreateCFProperties(service, &properties, kCFAllocatorDefault, 0) == KERN_SUCCESS,
              let props = properties?.takeRetainedValue() as? [String: Any] else { continue }
        results.append(props)
    }
    return results
}

func getIOServiceProperties(className: String) -> [String: Any]? {
    ioServiceProperties(className: className).first
}

/// `WallEnergyEstimate`, `AdapterEfficiencyLoss` and `SystemEnergyConsumed` share an energy unit
/// equal to mW / 3.6 (observed: SystemEnergyConsumed × 3.6 == SystemPowerIn, idle and charging).
func telemetryEnergyToWatts(_ value: Int) -> Double {
    Double(value) * 3.6 / 1000.0
}

func extractInt(from dict: [String: Any], key: String) -> Int? {
    guard let value = dict[key] else { return nil }
    if let intVal = value as? Int { return intVal }
    // Handle UInt64 overflow: IOKit stores some signed values (e.g. BatteryPower) as UInt64.
    // A negative value like -15034 gets stored as 18446744073709536582 (UInt64).
    // `as? Int` fails because it exceeds Int64.max, so we must use bitPattern conversion.
    if let uint64Val = value as? UInt64 {
        return Int(Int64(bitPattern: uint64Val))
    }
    if let numVal = value as? NSNumber { return numVal.intValue }
    return nil
}

func extractBool(from dict: [String: Any], key: String) -> Bool? {
    guard let value = dict[key] else { return nil }
    if let boolVal = value as? Bool { return boolVal }
    if let numVal = value as? NSNumber { return numVal.boolValue }
    return nil
}

func extractString(from dict: [String: Any], key: String) -> String? {
    return dict[key] as? String
}

func extractDict(from dict: [String: Any], key: String) -> [String: Any]? {
    return dict[key] as? [String: Any]
}

func extractIntArray(from dict: [String: Any], key: String) -> [Int]? {
    guard let value = dict[key] else { return nil }
    if let arr = value as? [Int] { return arr }
    if let arr = value as? [NSNumber] { return arr.map { $0.intValue } }
    return nil
}

/// Reads only the explicit `ManufactureDate` field; `ManufacturerData` is vendor-defined.
func readBatteryManufactureDate(
    from batteryProperties: [String: Any],
    relativeTo currentDate: Date = Date()
) -> String? {
    let rootGaugeName = extractString(from: batteryProperties, key: "DeviceName")
    if let rootGaugeName,
       let date = parseBatteryManufactureDate(
           from: batteryProperties,
           gaugeName: rootGaugeName,
           relativeTo: currentDate
       ) {
        return date
    }

    guard let packProperties = getIOServiceProperties(className: "AppleSmartBatteryPack"),
          let gaugeName = extractString(from: packProperties, key: "DeviceName") ?? rootGaugeName else {
        return nil
    }
    return parseBatteryManufactureDate(
        from: packProperties,
        gaugeName: gaugeName,
        relativeTo: currentDate
    )
}

func parseBatteryManufactureDate(
    from properties: [String: Any],
    gaugeName: String,
    relativeTo currentDate: Date = Date()
) -> String? {
    if let encodedDate = extractInt(from: properties, key: "ManufactureDate"),
       let date = formatBatteryManufactureDate(
           encodedDate,
           gaugeName: gaugeName,
           relativeTo: currentDate
       ) {
        return date
    }

    let batteryData = extractDict(from: properties, key: "BatteryData")
    guard let encodedDate = batteryData.flatMap({ extractInt(from: $0, key: "ManufactureDate") }) else {
        return nil
    }
    return formatBatteryManufactureDate(
        encodedDate,
        gaugeName: gaugeName,
        relativeTo: currentDate
    )
}

/// Decodes the observed six-byte ASCII date format used by known Mac battery gauges.
/// The least-significant bytes read as `YYMMDD`, where `YY` is years since 1992.
func formatBatteryManufactureDate(
    _ encodedDate: Int,
    gaugeName: String,
    relativeTo currentDate: Date = Date()
) -> String? {
    guard isKnownBatteryManufactureDateGauge(gaugeName), encodedDate > 0 else { return nil }
    let rawValue = UInt64(encodedDate)
    guard rawValue <= 0xFFFF_FFFF_FFFF else { return nil }

    func asciiDigit(at shift: Int) -> Int? {
        let byte = UInt8((rawValue >> shift) & 0xFF)
        guard (48...57).contains(byte) else { return nil }
        return Int(byte - 48)
    }

    guard let yearTens = asciiDigit(at: 0),
          let yearOnes = asciiDigit(at: 8),
          let monthTens = asciiDigit(at: 16),
          let monthOnes = asciiDigit(at: 24),
          let dayTens = asciiDigit(at: 32),
          let dayOnes = asciiDigit(at: 40) else {
        return nil
    }

    let year = 1992 + yearTens * 10 + yearOnes
    let month = monthTens * 10 + monthOnes
    let day = dayTens * 10 + dayOnes
    guard (2005...2091).contains(year) else { return nil }

    var calendar = Calendar(identifier: .gregorian)
    calendar.timeZone = TimeZone(identifier: "UTC") ?? .current
    var components = DateComponents()
    components.year = year
    components.month = month
    components.day = day
    guard components.isValidDate(in: calendar) else { return nil }

    let current = calendar.dateComponents([.year, .month], from: currentDate)
    guard let currentYear = current.year, let currentMonth = current.month,
          year * 12 + month <= currentYear * 12 + currentMonth else {
        return nil
    }

    // Day-level precision is not consistent across independent tools, so expose only the month.
    return String(format: "%04d-%02d", year, month)
}

private func isKnownBatteryManufactureDateGauge(_ gaugeName: String) -> Bool {
    let normalized = gaugeName.lowercased()
    if ["a1953", "a1964", "a1965"].contains(normalized) { return true }

    let bytes = Array(normalized.utf8)
    guard bytes.count >= 6,
          bytes[0] == Character("b").asciiValue,
          bytes[1] == Character("q").asciiValue,
          (48...57).contains(bytes[2]) else {
        return false
    }
    return bytes.dropFirst(2).allSatisfy { byte in
        (48...57).contains(byte) || (97...122).contains(byte)
    }
}

/// Capacity and health fields from `AppleSmartBattery`; many values live under `BatteryData` on Apple Silicon.
struct BatteryCapacitySnapshot: Equatable {
    let designCapacityMAH: Int?
    let fullChargeCapacityMAH: Int?
    let rawMaxCapacityMAH: Int?
    let nominalChargeCapacityMAH: Int?
    let remainingCapacityMAH: Int?
    let stateOfCharge: Int?
    let dailyMinSoc: Int?
    let dailyMaxSoc: Int?
    let temperatureCentidegrees: Int?

    var healthPercent: Int? {
        guard let design = designCapacityMAH, design > 0 else { return nil }
        let current = rawMaxCapacityMAH ?? fullChargeCapacityMAH ?? nominalChargeCapacityMAH
        guard let current, current > 0 else { return nil }
        return min(100, current * 100 / design)
    }
}

/// `banks` are the `AppleSmartBatteryBank` entries; some machines only report SoC and Qmax there.
func readBatteryCapacitySnapshot(from props: [String: Any], banks: [[String: Any]]) -> BatteryCapacitySnapshot {
    let batteryData = extractDict(from: props, key: "BatteryData")

    func packOrBatteryData(_ key: String) -> Int? {
        extractInt(from: props, key: key)
            ?? batteryData.flatMap { extractInt(from: $0, key: key) }
    }

    let bankBatteryData = banks.lazy.compactMap { extractDict(from: $0, key: "BatteryData") }.first
    return BatteryCapacitySnapshot(
        designCapacityMAH: packOrBatteryData("DesignCapacity"),
        fullChargeCapacityMAH: batteryData.flatMap { extractInt(from: $0, key: "FullChargeCapacity") }
            ?? extractInt(from: props, key: "FullChargeCapacity"),
        rawMaxCapacityMAH: packOrBatteryData("AppleRawMaxCapacity")
            ?? bankBatteryData.flatMap { extractInt(from: $0, key: "AppleRawMaxCapacity") },
        nominalChargeCapacityMAH: packOrBatteryData("NominalChargeCapacity"),
        remainingCapacityMAH: batteryData.flatMap { extractInt(from: $0, key: "RemainingCapacity") }
            ?? extractInt(from: props, key: "RemainingCapacity"),
        stateOfCharge: batteryData.flatMap { extractInt(from: $0, key: "StateOfCharge") }
            ?? extractInt(from: props, key: "StateOfCharge")
            ?? bankBatteryData.flatMap { extractInt(from: $0, key: "StateOfCharge") },
        dailyMinSoc: batteryData.flatMap { extractInt(from: $0, key: "DailyMinSoc") }
            ?? extractInt(from: props, key: "DailyMinSoc")
            ?? bankBatteryData.flatMap { extractInt(from: $0, key: "DailyMinSoc") },
        dailyMaxSoc: batteryData.flatMap { extractInt(from: $0, key: "DailyMaxSoc") }
            ?? extractInt(from: props, key: "DailyMaxSoc")
            ?? bankBatteryData.flatMap { extractInt(from: $0, key: "DailyMaxSoc") },
        temperatureCentidegrees: packOrBatteryData("Temperature")
            ?? bankBatteryData.flatMap { extractInt(from: $0, key: "Temperature") }
    )
}

/// macOS uses 65535 as an invalid sentinel for battery time estimates.
func isValidBatteryTimeMinutes(_ minutes: Int?) -> Int? {
    guard let minutes, minutes > 0, minutes < 65_535 else { return nil }
    return minutes
}

struct BatteryCellTelemetry: Equatable {
    let layout: BatteryCellTelemetryLayout
    let voltagesMV: [Int]
    let qmaxMAH: [Int]
    let parallelCellCurrents: [BatteryParallelCellCurrent]
}

private struct BatteryBankReading {
    let bankID: Int
    let voltageMV: Int?
    let qmaxMAH: Int?
}

private struct BatteryCellSlot {
    let bankID: Int
    let cellID: Int
}

/// Reads cell telemetry. Apple Silicon exposes series groups via bank nodes; pack-level arrays are a fallback when each entry is a distinct physical cell.
func readBatteryCellTelemetry(
    packBatteryData: [String: Any]?,
    banks bankProperties: [[String: Any]],
    cells cellProperties: [[String: Any]]
) -> BatteryCellTelemetry? {
    let banks = readBatteryBankReadings(bankProperties)
    if !banks.isEmpty {
        let sortedBanks = banks.sorted { $0.bankID < $1.bankID }
        let voltages = sortedBanks.compactMap(\.voltageMV)
        let qmaxValues = sortedBanks.compactMap(\.qmaxMAH)
        if !voltages.isEmpty, voltages.count == qmaxValues.count {
            let parallelCells = readBatteryParallelCellCurrents(cellProperties)
            let cellSlots = readBatteryCellSlots(cellProperties)
            let parallelCount = inferredParallelCount(parallelCells: parallelCells, cellSlots: cellSlots)
            let parallelCountKnown = parallelCount > 1
                || !parallelCells.isEmpty
                || !cellSlots.isEmpty

            return BatteryCellTelemetry(
                layout: .seriesParallel(
                    seriesCount: voltages.count,
                    parallelCount: parallelCount,
                    parallelCountKnown: parallelCountKnown
                ),
                voltagesMV: voltages,
                qmaxMAH: qmaxValues,
                parallelCellCurrents: parallelCells
            )
        }
    }

    if let pack = packBatteryData,
       let voltages = extractIntArray(from: pack, key: "CellVoltage"),
       let qmax = extractIntArray(from: pack, key: "Qmax"),
       !voltages.isEmpty,
       voltages.count == qmax.count {
        return BatteryCellTelemetry(
            layout: .perCellArrays,
            voltagesMV: voltages,
            qmaxMAH: qmax,
            parallelCellCurrents: []
        )
    }

    return nil
}

private func inferredParallelCount(
    parallelCells: [BatteryParallelCellCurrent],
    cellSlots: [BatteryCellSlot]
) -> Int {
    if !parallelCells.isEmpty {
        return Dictionary(grouping: parallelCells, by: \.bankID).values.map(\.count).max() ?? 1
    }
    if !cellSlots.isEmpty {
        return Dictionary(grouping: cellSlots, by: \.bankID).values.map(\.count).max() ?? 1
    }
    return 1
}

private func readBatteryBankReadings(_ banks: [[String: Any]]) -> [BatteryBankReading] {
    banks.enumerated().compactMap { index, props in
        let batteryData = extractDict(from: props, key: "BatteryData")
        let voltageMV = batteryData.flatMap { extractInt(from: $0, key: "CellVoltage") }
        let qmaxMAH = batteryData.flatMap { extractInt(from: $0, key: "Qmax") }
        guard voltageMV != nil || qmaxMAH != nil else { return nil }
        return BatteryBankReading(
            bankID: extractInt(from: props, key: "BankID") ?? index,
            voltageMV: voltageMV,
            qmaxMAH: qmaxMAH
        )
    }
}

private func readBatteryCellSlots(_ cells: [[String: Any]]) -> [BatteryCellSlot] {
    cells.compactMap { props -> BatteryCellSlot? in
        guard let bankID = extractInt(from: props, key: "BankID"),
              let cellID = extractInt(from: props, key: "CellID") else { return nil }
        return BatteryCellSlot(bankID: bankID, cellID: cellID)
    }
    .sorted { ($0.bankID, $0.cellID) < ($1.bankID, $1.cellID) }
}

private func readBatteryParallelCellCurrents(_ cells: [[String: Any]]) -> [BatteryParallelCellCurrent] {
    cells.compactMap { props -> BatteryParallelCellCurrent? in
        guard let bankID = extractInt(from: props, key: "BankID"),
              let cellID = extractInt(from: props, key: "CellID"),
              let currentMA = extractDict(from: props, key: "CellData").flatMap({ extractInt(from: $0, key: "CellCurrent") })
        else { return nil }
        return BatteryParallelCellCurrent(bankID: bankID, cellID: cellID, currentMA: currentMA)
    }
    .sorted { ($0.bankID, $0.cellID) < ($1.bankID, $1.cellID) }
}
