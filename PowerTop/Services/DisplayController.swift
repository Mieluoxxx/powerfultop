import AppKit
import CoreGraphics
import IOKit
import Observation

// Brightness paths and IOAVService discovery adapted from MonitorControl
// (https://github.com/MonitorControl/MonitorControl, MIT License, © MonitorControl contributors).

struct DisplayBrightness: Identifiable, Equatable {
    let id: CGDirectDisplayID
    let name: String
    let isBuiltIn: Bool
    let method: BrightnessMethod
    var level: Double
}

@MainActor
@Observable
final class DisplayController {
    private(set) var displays: [DisplayBrightness] = []

    /// Native brightness also moves with the keyboard keys, so poll it while the controls are on screen.
    var active = false {
        didSet {
            guard active != oldValue else { return }
            active ? startPolling() : stopPolling()
        }
    }

    @ObservationIgnored private var links: [CGDirectDisplayID: DDCLink] = [:]
    /// DDC/CI can be switched on in the monitor's own menu without any display change, so failed
    /// probes are retried, but rarely: each probe costs about half a second of I²C retries.
    @ObservationIgnored private var ddcFailedAt: [CGDirectDisplayID: Date] = [:]
    private static let ddcRetryInterval: TimeInterval = 60
    @ObservationIgnored private var probing: Set<CGDirectDisplayID> = []
    @ObservationIgnored private var baselineGamma: [CGDirectDisplayID: GammaTable] = [:]
    @ObservationIgnored private var pollTimer: Timer?
    @ObservationIgnored private var screenObserver: NSObjectProtocol?
    private let probeQueue = DispatchQueue(label: "PowerTop.ddc.probe", qos: .utility)

    func start() {
        guard screenObserver == nil else { return }
        screenObserver = NotificationCenter.default.addObserver(
            forName: NSApplication.didChangeScreenParametersNotification, object: nil, queue: .main
        ) { [weak self] _ in
            MainActor.assumeIsolated { self?.displaysChanged() }
        }
        refresh()
    }

    /// The system restores ColorSync when the process exits; restoring here covers the gap before that.
    func shutdown() {
        stopPolling()
        if let screenObserver { NotificationCenter.default.removeObserver(screenObserver) }
        screenObserver = nil
        if !baselineGamma.isEmpty { CGDisplayRestoreColorSyncSettings() }
        baselineGamma.removeAll()
    }

    func setLevel(_ level: Double, for id: CGDirectDisplayID) {
        guard let index = displays.firstIndex(where: { $0.id == id }) else { return }
        let level = min(max(level, 0), 1)
        displays[index].level = level
        switch displays[index].method {
        case .native:
            _ = PrivateDisplayAPI.setBrightness?(id, Float(level))
        case .ddc:
            links[id]?.send(level: level)
        case .software:
            applySoftware(level: level, to: id)
            UserDefaults.standard.set(level, forKey: Self.softwareKey(for: id))
        }
    }

    // MARK: - Discovery

    private func displaysChanged() {
        links.removeAll()
        ddcFailedAt.removeAll()
        baselineGamma.removeAll()
        refresh()
    }

    private func refresh() {
        displays = Self.onlineDisplays().map { id in
            let name = Self.name(for: id)
            let builtIn = CGDisplayIsBuiltin(id) != 0
            if let level = Self.nativeBrightness(id) {
                return DisplayBrightness(id: id, name: name, isBuiltIn: builtIn, method: .native, level: level)
            }
            if let link = links[id] {
                return DisplayBrightness(id: id, name: name, isBuiltIn: builtIn, method: .ddc, level: link.level)
            }
            let saved = UserDefaults.standard.object(forKey: Self.softwareKey(for: id)) as? Double ?? 1
            if saved < 1 { applySoftware(level: saved, to: id) }
            if ddcFailedAt[id].map({ Date().timeIntervalSince($0) >= Self.ddcRetryInterval }) ?? true { probeDDC(id) }
            return DisplayBrightness(id: id, name: name, isBuiltIn: builtIn, method: .software, level: saved)
        }
    }

    private func probeDDC(_ id: CGDirectDisplayID) {
        guard !probing.contains(id) else { return }
        probing.insert(id)
        let prefix = edidUUIDPrefix(vendor: CGDisplayVendorNumber(id), model: CGDisplayModelNumber(id))
        let name = Self.name(for: id)
        let claimed = Set(links.values.map(\.serviceLocation))
        probeQueue.async {
            let link = DDCLink.connect(edidPrefix: prefix, productName: name, excluding: claimed)
            Task { @MainActor [weak self] in
                guard let self else { return }
                probing.remove(id)
                guard let index = displays.firstIndex(where: { $0.id == id }) else { return }
                guard let link else {
                    ddcFailedAt[id] = Date()
                    return
                }
                // Hardware dimming replaces software dimming rather than stacking on it.
                if displays[index].level < 1 { restoreGamma(id) }
                links[id] = link
                displays[index] = DisplayBrightness(id: id, name: displays[index].name, isBuiltIn: displays[index].isBuiltIn,
                                                    method: .ddc, level: link.level)
            }
        }
    }

    private static func onlineDisplays() -> [CGDirectDisplayID] {
        var ids = [CGDirectDisplayID](repeating: 0, count: 16)
        var count: UInt32 = 0
        guard CGGetOnlineDisplayList(UInt32(ids.count), &ids, &count) == .success else { return [] }
        // Mirror targets share the source's picture; one slider per visible picture is enough.
        return ids.prefix(Int(count)).filter { CGDisplayMirrorsDisplay($0) == kCGNullDirectDisplay }
            .sorted { CGDisplayIsBuiltin($0) != 0 && CGDisplayIsBuiltin($1) == 0 }
    }

    private static func name(for id: CGDirectDisplayID) -> String {
        let screen = NSScreen.screens.first {
            ($0.deviceDescription[NSDeviceDescriptionKey("NSScreenNumber")] as? NSNumber)?.uint32Value == id
        }
        return screen?.localizedName ?? String(localized: "Display")
    }

    private static func nativeBrightness(_ id: CGDirectDisplayID) -> Double? {
        guard let get = PrivateDisplayAPI.getBrightness else { return nil }
        var value: Float = -1
        guard get(id, &value) == 0, value >= 0 else { return nil }
        return Double(value)
    }

    private static func softwareKey(for id: CGDirectDisplayID) -> String {
        "softwareBrightness.\(CGDisplayVendorNumber(id))-\(CGDisplayModelNumber(id))-\(CGDisplaySerialNumber(id))"
    }

    // MARK: - Polling

    private func startPolling() {
        refresh()
        let timer = Timer(timeInterval: 1, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.pollNative() }
        }
        RunLoop.main.add(timer, forMode: .common)
        pollTimer = timer
    }

    private func stopPolling() {
        pollTimer?.invalidate()
        pollTimer = nil
    }

    private func pollNative() {
        for index in displays.indices where displays[index].method == .native {
            if let level = Self.nativeBrightness(displays[index].id), abs(level - displays[index].level) > 0.001 {
                displays[index].level = level
            }
        }
    }

    // MARK: - Software dimming

    /// Scales the display's own calibrated table so colour calibration survives dimming.
    private func applySoftware(level: Double, to id: CGDirectDisplayID) {
        if baselineGamma[id] == nil { baselineGamma[id] = GammaTable.read(id) }
        guard let baseline = baselineGamma[id] else { return }
        baseline.scaled(by: SoftwareDimming.gammaCeiling(level: level)).apply(to: id)
    }

    private func restoreGamma(_ id: CGDirectDisplayID) {
        baselineGamma.removeValue(forKey: id)?.apply(to: id)
    }
}

private struct GammaTable {
    var red: [CGGammaValue]
    var green: [CGGammaValue]
    var blue: [CGGammaValue]

    static func read(_ id: CGDirectDisplayID) -> GammaTable? {
        let capacity = Int(CGDisplayGammaTableCapacity(id))
        guard capacity > 0 else { return nil }
        var table = GammaTable(red: .init(repeating: 0, count: capacity), green: .init(repeating: 0, count: capacity),
                               blue: .init(repeating: 0, count: capacity))
        var count: UInt32 = 0
        guard CGGetDisplayTransferByTable(id, UInt32(capacity), &table.red, &table.green, &table.blue, &count) == .success,
              count > 0 else { return nil }
        table.red.removeSubrange(Int(count)...)
        table.green.removeSubrange(Int(count)...)
        table.blue.removeSubrange(Int(count)...)
        return table
    }

    func scaled(by factor: Double) -> GammaTable {
        let f = CGGammaValue(factor)
        return GammaTable(red: red.map { $0 * f }, green: green.map { $0 * f }, blue: blue.map { $0 * f })
    }

    func apply(to id: CGDirectDisplayID) {
        _ = CGSetDisplayTransferByTable(id, UInt32(red.count), red, green, blue)
    }
}

// MARK: - Private API

/// Resolved at runtime so the build needs no private framework linkage; missing symbols disable that path.
private enum PrivateDisplayAPI {
    typealias GetBrightness = @convention(c) (CGDirectDisplayID, UnsafeMutablePointer<Float>) -> Int32
    typealias SetBrightness = @convention(c) (CGDirectDisplayID, Float) -> Int32
    typealias AVServiceCreate = @convention(c) (CFAllocator?, io_service_t) -> Unmanaged<CFTypeRef>?
    typealias AVServiceI2C = @convention(c) (CFTypeRef, UInt32, UInt32, UnsafeMutableRawPointer, UInt32) -> IOReturn

    private static let displayServices = dlopen("/System/Library/PrivateFrameworks/DisplayServices.framework/DisplayServices", RTLD_LAZY)
    private static let iokit = dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", RTLD_LAZY)

    static let getBrightness = symbol(displayServices, "DisplayServicesGetBrightness", as: GetBrightness.self)
    static let setBrightness = symbol(displayServices, "DisplayServicesSetBrightness", as: SetBrightness.self)
    static let avServiceCreate = symbol(iokit, "IOAVServiceCreateWithService", as: AVServiceCreate.self)
    static let avServiceWrite = symbol(iokit, "IOAVServiceWriteI2C", as: AVServiceI2C.self)
    static let avServiceRead = symbol(iokit, "IOAVServiceReadI2C", as: AVServiceI2C.self)

    private static func symbol<T>(_ handle: UnsafeMutableRawPointer?, _ name: String, as type: T.Type) -> T? {
        guard let handle, let pointer = dlsym(handle, name) else { return nil }
        return unsafeBitCast(pointer, to: type)
    }
}

// MARK: - DDC

/// One external monitor's I²C channel. Writes are serialised and coalesced: a slider drag produces
/// far more values than DDC (tens of milliseconds per transaction) can deliver.
private final class DDCLink: @unchecked Sendable {
    let service: CFTypeRef
    let serviceLocation: Int
    let maxValue: UInt16
    private(set) var level: Double

    private let queue = DispatchQueue(label: "PowerTop.ddc.write", qos: .userInitiated)
    private let lock = NSLock()
    private var pending: UInt16?
    private var writing = false

    private init(service: CFTypeRef, serviceLocation: Int, current: UInt16, max: UInt16) {
        self.service = service
        self.serviceLocation = serviceLocation
        maxValue = max
        level = Double(current) / Double(max)
    }

    func send(level: Double) {
        self.level = level
        let value = UInt16((level * Double(maxValue)).rounded())
        lock.lock()
        pending = value
        let start = !writing
        writing = true
        lock.unlock()
        guard start else { return }
        queue.async { [self] in
            while true {
                lock.lock()
                guard let value = pending else {
                    writing = false
                    lock.unlock()
                    return
                }
                pending = nil
                lock.unlock()
                _ = Self.transact(service, DDC.setVCPPacket(DDC.luminance, value: value), replyLength: 0)
            }
        }
    }

    /// Finds the external `DCPAVServiceProxy` whose framebuffer carries the display's EDID and asks
    /// it for VCP 0x10; a monitor with DDC/CI disabled answers with null messages and yields nil.
    static func connect(edidPrefix: String, productName: String, excluding claimed: Set<Int>) -> DDCLink? {
        guard PrivateDisplayAPI.avServiceCreate != nil else { return nil }
        let candidates = externalServices()
            .filter { $0.edidUUID.hasPrefix(edidPrefix) && !claimed.contains($0.location) }
            .sorted { ($0.productName == productName ? 0 : 1) < ($1.productName == productName ? 0 : 1) }
        for candidate in candidates {
            guard let reply = transact(candidate.service, DDC.getVCPPacket(DDC.luminance), replyLength: 11),
                  let value = DDC.parseVCPReply(reply, code: DDC.luminance) else { continue }
            return DDCLink(service: candidate.service, serviceLocation: candidate.location, current: value.current, max: value.max)
        }
        return nil
    }

    private struct ExternalService {
        let service: CFTypeRef
        let location: Int
        let edidUUID: String
        let productName: String
    }

    /// In the service plane each framebuffer precedes the AV service proxy that drives its port.
    private static func externalServices() -> [ExternalService] {
        guard let create = PrivateDisplayAPI.avServiceCreate else { return [] }
        var iterator = io_iterator_t()
        let root = IORegistryGetRootEntry(kIOMainPortDefault)
        defer { IOObjectRelease(root) }
        guard IORegistryEntryCreateIterator(root, kIOServicePlane, IOOptionBits(kIORegistryIterateRecursively), &iterator) == KERN_SUCCESS else {
            return []
        }
        defer { IOObjectRelease(iterator) }
        var services: [ExternalService] = []
        var edidUUID = ""
        var productName = ""
        var location = 0
        var name = [CChar](repeating: 0, count: 128)
        while case let entry = IOIteratorNext(iterator), entry != IO_OBJECT_NULL {
            defer { IOObjectRelease(entry) }
            guard IORegistryEntryGetName(entry, &name) == KERN_SUCCESS else { continue }
            switch String(cString: name) {
            case "AppleCLCD2", "IOMobileFramebufferShim":
                location += 1
                edidUUID = property(entry, "EDID UUID") as? String ?? ""
                let attributes = property(entry, "DisplayAttributes") as? NSDictionary
                productName = (attributes?["ProductAttributes"] as? NSDictionary)?["ProductName"] as? String ?? ""
            case "DCPAVServiceProxy":
                guard property(entry, "Location") as? String == "External",
                      let service = create(kCFAllocatorDefault, entry)?.takeRetainedValue() else { continue }
                services.append(ExternalService(service: service, location: location, edidUUID: edidUUID, productName: productName))
            default:
                continue
            }
        }
        return services
    }

    private static func property(_ entry: io_registry_entry_t, _ key: String) -> Any? {
        IORegistryEntryCreateCFProperty(entry, key as CFString, kCFAllocatorDefault, 0)?.takeRetainedValue()
    }

    /// Retry timings from MonitorControl: monitors drop transactions that arrive back to back.
    private static func transact(_ service: CFTypeRef, _ packet: [UInt8], replyLength: Int) -> [UInt8]? {
        guard let write = PrivateDisplayAPI.avServiceWrite, let read = PrivateDisplayAPI.avServiceRead else { return nil }
        var packet = packet
        for _ in 0..<5 {
            var written = false
            for _ in 0..<2 {
                usleep(10_000)
                written = write(service, UInt32(DDC.chipAddress), UInt32(DDC.dataAddress), &packet, UInt32(packet.count)) == kIOReturnSuccess
            }
            if replyLength == 0 {
                if written { return [] }
            } else {
                usleep(50_000)
                var reply = [UInt8](repeating: 0, count: replyLength)
                if read(service, UInt32(DDC.chipAddress), 0, &reply, UInt32(replyLength)) == kIOReturnSuccess,
                   DDC.parseVCPReply(reply, code: packet[2]) != nil {
                    return reply
                }
            }
            usleep(20_000)
        }
        return nil
    }
}
