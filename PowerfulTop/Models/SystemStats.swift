import Foundation

enum CoreKind: Equatable {
    case efficiency
    case performance
}

/// Cumulative per-core tick counters from `host_processor_info(PROCESSOR_CPU_LOAD_INFO)`.
struct CoreTicks: Equatable {
    var user: UInt64
    var system: UInt64
    var idle: UInt64
    var nice: UInt64

    var busy: UInt64 { user + system + nice }
    var total: UInt64 { busy + idle }
}

struct CPULoad: Equatable {
    /// Fractions 0…1 of all core time over the sampling interval.
    let user: Double
    let system: Double
    let idle: Double
    let perCore: [Double]
    let efficiency: Double?
    let performance: Double?

    var usage: Double { user + system }

    static let idleLoad = CPULoad(user: 0, system: 0, idle: 1, perCore: [], efficiency: nil, performance: nil)
}

/// Busy share between two tick snapshots. Counters can wrap or reset across sleep, so a core whose
/// counters went backwards contributes nothing instead of a huge bogus delta.
func cpuLoad(previous: [CoreTicks], current: [CoreTicks], kinds: [CoreKind]) -> CPULoad? {
    guard !previous.isEmpty, previous.count == current.count else { return nil }
    var user = 0.0, system = 0.0, idle = 0.0
    var perCore: [Double] = []
    var clusterBusy: [CoreKind: Double] = [:]
    var clusterTotal: [CoreKind: Double] = [:]

    for (index, (old, new)) in zip(previous, current).enumerated() {
        guard new.user >= old.user, new.system >= old.system, new.idle >= old.idle, new.nice >= old.nice else {
            perCore.append(0)
            continue
        }
        let dUser = Double(new.user - old.user + new.nice - old.nice)
        let dSystem = Double(new.system - old.system)
        let dIdle = Double(new.idle - old.idle)
        let dTotal = dUser + dSystem + dIdle
        user += dUser
        system += dSystem
        idle += dIdle
        perCore.append(dTotal > 0 ? (dUser + dSystem) / dTotal : 0)
        if index < kinds.count {
            clusterBusy[kinds[index], default: 0] += dUser + dSystem
            clusterTotal[kinds[index], default: 0] += dTotal
        }
    }

    let total = user + system + idle
    guard total > 0 else { return nil }
    func share(_ kind: CoreKind) -> Double? {
        guard let total = clusterTotal[kind], total > 0 else { return nil }
        return (clusterBusy[kind] ?? 0) / total
    }
    return CPULoad(
        user: user / total,
        system: system / total,
        idle: idle / total,
        perCore: perCore,
        efficiency: share(.efficiency),
        performance: share(.performance)
    )
}

enum MemoryPressure: Int, Equatable {
    case normal = 1
    case warning = 2
    case critical = 4
}

/// Activity Monitor's breakdown: app = anonymous minus purgeable, wired, compressed; the rest is
/// cached or free and reclaimable, so it does not count as used.
struct MemoryStats: Equatable {
    let totalBytes: UInt64
    let appBytes: UInt64
    let wiredBytes: UInt64
    let compressedBytes: UInt64
    let swapUsedBytes: UInt64
    let pressure: MemoryPressure

    var usedBytes: UInt64 { appBytes + wiredBytes + compressedBytes }
    var availableBytes: UInt64 { totalBytes > usedBytes ? totalBytes - usedBytes : 0 }
    var usedFraction: Double { totalBytes > 0 ? Double(usedBytes) / Double(totalBytes) : 0 }

    init(totalBytes: UInt64, internalPages: UInt64, purgeablePages: UInt64, wiredPages: UInt64,
         compressorPages: UInt64, pageSize: UInt64, swapUsedBytes: UInt64, pressure: MemoryPressure) {
        self.totalBytes = totalBytes
        appBytes = (internalPages > purgeablePages ? internalPages - purgeablePages : 0) * pageSize
        wiredBytes = wiredPages * pageSize
        compressedBytes = compressorPages * pageSize
        self.swapUsedBytes = swapUsedBytes
        self.pressure = pressure
    }
}

struct ProcessUsage: Equatable, Identifiable {
    let pid: Int32
    let name: String
    /// Percent of one core for CPU lists, bytes for memory lists.
    let value: Double

    var id: Int32 { pid }
}

/// Parses `ps -Aceo pid=,pcpu=,comm= -r`; `comm` may contain spaces.
func parseProcessCPU(_ output: String, limit: Int) -> [ProcessUsage] {
    output.split(separator: "\n").compactMap { line -> ProcessUsage? in
        let parts = line.split(separator: " ", maxSplits: 2, omittingEmptySubsequences: true)
        guard parts.count == 3, let pid = Int32(parts[0]), let cpu = Double(parts[1]) else { return nil }
        return ProcessUsage(pid: pid, name: String(parts[2]), value: cpu)
    }
    .sorted { $0.value > $1.value }
    .prefix(limit)
    .map { $0 }
}

/// Parses `top -l 1 -o mem -stats pid,command,mem`; MEM is the physical footprint Activity Monitor
/// shows (e.g. "1461M", "583K", "1.2G", with an optional trailing "+"/"-" trend marker).
func parseProcessMemory(_ output: String, limit: Int) -> [ProcessUsage] {
    let rows = output.split(separator: "\n")
    guard let header = rows.firstIndex(where: { $0.hasPrefix("PID") }) else { return [] }
    return rows[(header + 1)...].compactMap { line -> ProcessUsage? in
        let trimmed = line.trimmingCharacters(in: .whitespaces)
        guard let firstSpace = trimmed.firstIndex(of: " "), let lastSpace = trimmed.lastIndex(of: " "),
              let pid = Int32(trimmed[..<firstSpace]) else { return nil }
        let name = trimmed[firstSpace..<lastSpace].trimmingCharacters(in: .whitespaces)
        guard let bytes = parseTopMemory(String(trimmed[trimmed.index(after: lastSpace)...])), !name.isEmpty else { return nil }
        return ProcessUsage(pid: pid, name: name, value: bytes)
    }
    .prefix(limit)
    .map { $0 }
}

/// Parses `ps -Aceo pid=,comm=` into a pid → full executable name map.
func parseProcessNames(_ output: String) -> [Int32: String] {
    var names: [Int32: String] = [:]
    for line in output.split(separator: "\n") {
        let parts = line.split(separator: " ", maxSplits: 1, omittingEmptySubsequences: true)
        guard parts.count == 2, let pid = Int32(parts[0]) else { continue }
        names[pid] = String(parts[1])
    }
    return names
}

func parseTopMemory(_ token: String) -> Double? {
    var text = token.trimmingCharacters(in: CharacterSet(charactersIn: "+- "))
    guard let unit = text.last else { return nil }
    let multipliers: [Character: Double] = ["B": 1, "K": 1024, "M": 1_048_576, "G": 1_073_741_824, "T": 1_099_511_627_776]
    guard let multiplier = multipliers[unit] else { return Double(text) }
    text.removeLast()
    return Double(text).map { $0 * multiplier }
}

func formatBytes(_ bytes: Double) -> String {
    ByteCountFormatter.string(fromByteCount: Int64(bytes), countStyle: .memory)
}
