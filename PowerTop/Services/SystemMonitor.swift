import Darwin
import Foundation
import IOKit
import Observation

@MainActor
@Observable
final class SystemMonitor {
    private(set) var cpu = CPULoad.idleLoad
    private(set) var memory: MemoryStats?
    private(set) var loadAverage: [Double] = []
    private(set) var bootDate: Date?
    private(set) var topCPU: [ProcessUsage] = []
    private(set) var topMemory: [ProcessUsage] = []
    let coreKinds: [CoreKind]

    /// Process lists cost a `ps`/`top` spawn (top uses ~0.6 s of CPU), so only sample while visible.
    var processSamplingActive = false {
        didSet { if processSamplingActive && !oldValue { sampleProcesses(force: true) } }
    }

    private var previousTicks: [CoreTicks] = []
    private var timer: Timer?
    private var lastCPUListAt = Date.distantPast
    private var lastMemoryListAt = Date.distantPast
    private let processQueue = DispatchQueue(label: "PowerTop.processes", qos: .utility)

    init() {
        coreKinds = Self.readCoreKinds()
    }

    func start() {
        guard timer == nil else { return }
        bootDate = Self.readBootDate()
        previousTicks = Self.readTicks()
        refresh()
        let newTimer = Timer(timeInterval: 2, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.refresh() }
        }
        RunLoop.main.add(newTimer, forMode: .common)
        timer = newTimer
    }

    func stop() {
        timer?.invalidate()
        timer = nil
    }

    private func refresh() {
        let ticks = Self.readTicks()
        if let load = cpuLoad(previous: previousTicks, current: ticks, kinds: coreKinds) {
            cpu = load
        }
        previousTicks = ticks
        memory = Self.readMemory()
        var load = [Double](repeating: 0, count: 3)
        if getloadavg(&load, 3) == 3 { loadAverage = load }
        if processSamplingActive { sampleProcesses(force: false) }
    }

    private func sampleProcesses(force: Bool) {
        let now = Date()
        if force || now.timeIntervalSince(lastCPUListAt) >= 5 {
            lastCPUListAt = now
            processQueue.async {
                // ps is setuid root, so it sees WindowServer and other system processes we cannot query.
                let list = parseProcessCPU(Self.run("/bin/ps", ["-Aceo", "pid=,pcpu=,comm=", "-r"]), limit: 8)
                Task { @MainActor in self.topCPU = list }
            }
        }
        if force || now.timeIntervalSince(lastMemoryListAt) >= 10 {
            lastMemoryListAt = now
            processQueue.async {
                let list = parseProcessMemory(Self.run("/usr/bin/top", ["-l", "1", "-o", "mem", "-n", "8", "-stats", "pid,command,mem"]), limit: 8)
                // top truncates COMMAND to 16 characters; ps reports the full executable name.
                let names = parseProcessNames(Self.run("/bin/ps", ["-Aceo", "pid=,comm="]))
                let named = list.map { ProcessUsage(pid: $0.pid, name: names[$0.pid] ?? $0.name, value: $0.value) }
                Task { @MainActor in self.topMemory = named }
            }
        }
    }

    // MARK: - System reads

    private nonisolated static func run(_ path: String, _ arguments: [String]) -> String {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: path)
        process.arguments = arguments
        let pipe = Pipe()
        process.standardOutput = pipe
        process.standardError = FileHandle.nullDevice
        guard (try? process.run()) != nil else { return "" }
        let data = pipe.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()
        return String(decoding: data, as: UTF8.self)
    }

    private static func readTicks() -> [CoreTicks] {
        var count: natural_t = 0
        var info: processor_info_array_t?
        var infoCount: mach_msg_type_number_t = 0
        guard host_processor_info(mach_host_self(), PROCESSOR_CPU_LOAD_INFO, &count, &info, &infoCount) == KERN_SUCCESS,
              let info else { return [] }
        defer {
            vm_deallocate(mach_task_self_, vm_address_t(bitPattern: info), vm_size_t(infoCount) * vm_size_t(MemoryLayout<integer_t>.stride))
        }
        let stride = Int(CPU_STATE_MAX)
        return (0..<Int(count)).map { core in
            func tick(_ state: Int32) -> UInt64 { UInt64(UInt32(bitPattern: info[core * stride + Int(state)])) }
            return CoreTicks(user: tick(CPU_STATE_USER), system: tick(CPU_STATE_SYSTEM), idle: tick(CPU_STATE_IDLE), nice: tick(CPU_STATE_NICE))
        }
    }

    /// Logical CPU order matches the device tree; each `cpuN` node carries `cluster-type` "E" or "P".
    private static func readCoreKinds() -> [CoreKind] {
        let cpus = IORegistryEntryFromPath(kIOMainPortDefault, "IODeviceTree:/cpus")
        guard cpus != 0 else { return [] }
        defer { IOObjectRelease(cpus) }
        var iterator: io_iterator_t = 0
        guard IORegistryEntryGetChildIterator(cpus, kIODeviceTreePlane, &iterator) == KERN_SUCCESS else { return [] }
        defer { IOObjectRelease(iterator) }
        var kinds: [(id: UInt32, kind: CoreKind)] = []
        while case let child = IOIteratorNext(iterator), child != 0 {
            defer { IOObjectRelease(child) }
            let type = (IORegistryEntryCreateCFProperty(child, "cluster-type" as CFString, kCFAllocatorDefault, 0)?
                .takeRetainedValue() as? Data).map { String(decoding: $0.filter { $0 != 0 }, as: UTF8.self) }
            let id = (IORegistryEntryCreateCFProperty(child, "cpu-id" as CFString, kCFAllocatorDefault, 0)?
                .takeRetainedValue() as? Data).map { $0.withUnsafeBytes { $0.loadUnaligned(as: UInt32.self) } }
            guard let type, let id else { continue }
            kinds.append((id, type == "E" ? .efficiency : .performance))
        }
        return kinds.sorted { $0.id < $1.id }.map(\.kind)
    }

    private static func readMemory() -> MemoryStats? {
        var stats = vm_statistics64()
        var count = mach_msg_type_number_t(MemoryLayout<vm_statistics64>.stride / MemoryLayout<integer_t>.stride)
        let result = withUnsafeMutablePointer(to: &stats) {
            $0.withMemoryRebound(to: integer_t.self, capacity: Int(count)) {
                host_statistics64(mach_host_self(), HOST_VM_INFO64, $0, &count)
            }
        }
        guard result == KERN_SUCCESS else { return nil }
        var swap = xsw_usage()
        var swapSize = MemoryLayout<xsw_usage>.size
        sysctlbyname("vm.swapusage", &swap, &swapSize, nil, 0)
        var level: Int32 = 1
        var levelSize = MemoryLayout<Int32>.size
        sysctlbyname("kern.memorystatus_vm_pressure_level", &level, &levelSize, nil, 0)
        return MemoryStats(
            totalBytes: ProcessInfo.processInfo.physicalMemory,
            internalPages: UInt64(stats.internal_page_count),
            purgeablePages: UInt64(stats.purgeable_count),
            wiredPages: UInt64(stats.wire_count),
            compressorPages: UInt64(stats.compressor_page_count),
            pageSize: UInt64(vm_kernel_page_size),
            swapUsedBytes: swap.xsu_used,
            pressure: MemoryPressure(rawValue: Int(level)) ?? .normal
        )
    }

    private static func readBootDate() -> Date? {
        var boot = timeval()
        var size = MemoryLayout<timeval>.size
        guard sysctlbyname("kern.boottime", &boot, &size, nil, 0) == 0 else { return nil }
        return Date(timeIntervalSince1970: TimeInterval(boot.tv_sec))
    }
}
