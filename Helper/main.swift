import Foundation

/// Root-only SMC writer. Accepts fan keys exclusively and always fails towards macOS control.
final class FanController {
    private let smc: SMCConnection
    private let fanCount: Int
    private let modeKeys: [String]
    private let failsafeKeys: [String]
    private let hasDiagnosticKey: Bool
    private var engaged = false
    private var lastRequest = Date.distantPast
    private var cooldownUntil = Date.distantPast
    /// Consecutive 1 s ticks at or above the failsafe; a single core spike must not trip it.
    private var overheatTicks = 0

    /// Survives a helper crash so the next launch can hand fans back to macOS.
    private static let engagedMarkerPath = "/Library/Application Support/PowerTop/fan-helper.engaged"

    init(smc: SMCConnection) {
        self.smc = smc
        fanCount = Int(smc.read("FNum")?.doubleValue ?? 0)
        // M1–M4 use `F%dMd`; M5 renamed it to `F%dmd`.
        modeKeys = (0..<fanCount).map { index in
            smc.read("F\(index)Md") != nil ? "F\(index)Md" : "F\(index)md"
        }
        hasDiagnosticKey = smc.read("Ftst") != nil
        failsafeKeys = (0..<smc.keyCount()).compactMap { smc.key(at: $0) }
            .filter { SensorGroup.cpu.matches($0) || SensorGroup.gpu.matches($0) }

        if FileManager.default.fileExists(atPath: Self.engagedMarkerPath) {
            _ = restoreAutomatic()
        }
    }

    func apply(_ rpms: [Double]) -> String? {
        lastRequest = Date()
        guard Date() >= cooldownUntil else { return "Fan control paused after overheating" }
        guard fanCount > 0 else { return "No fans found" }
        if !engaged, let error = engage() {
            _ = restoreAutomatic()
            return error
        }
        for index in 0..<min(fanCount, rpms.count) {
            guard let minRPM = smc.read("F\(index)Mn")?.doubleValue,
                  let maxRPM = smc.read("F\(index)Mx")?.doubleValue,
                  maxRPM > minRPM else { continue }
            let rpm = min(max(rpms[index], minRPM), maxRPM)
            smc.write("F\(index)Tg", bytes: SMCValue.floatBytes(rpm))
        }
        return nil
    }

    func restoreAutomatic() -> String? {
        for key in modeKeys {
            smc.write(key, bytes: [0])
        }
        // Clearing Ftst returns fans to system mode even where the mode write was rejected.
        if hasDiagnosticKey {
            smc.write("Ftst", bytes: [0])
        }
        engaged = false
        try? FileManager.default.removeItem(atPath: Self.engagedMarkerPath)
        return nil
    }

    func tick() {
        guard engaged else { return }
        if Date().timeIntervalSince(lastRequest) > FanHelperConstants.heartbeatTimeout {
            _ = restoreAutomatic()
            return
        }
        let hottest = failsafeKeys.compactMap { smc.read($0)?.doubleValue }.filter(isPlausibleTemperature).max() ?? 0
        overheatTicks = hottest >= FanHelperConstants.failsafeCelsius ? overheatTicks + 1 : 0
        if overheatTicks >= 3 {
            overheatTicks = 0
            _ = restoreAutomatic()
            cooldownUntil = Date().addingTimeInterval(60)
        }
    }

    /// M3+ firmware rejects manual mode while thermalmonitord holds the fans (mode 3);
    /// Ftst = 1 makes it yield, after which the mode write lands within a few seconds.
    private func engage() -> String? {
        FileManager.default.createFile(atPath: Self.engagedMarkerPath, contents: nil)
        if hasDiagnosticKey, smc.write("Ftst", bytes: [1]) != 0 {
            return "SMC rejected the diagnostic unlock"
        }
        let deadline = Date().addingTimeInterval(10)
        var pending = Set(modeKeys)
        while !pending.isEmpty, Date() < deadline {
            for key in pending where smc.write(key, bytes: [1]) == 0 && smc.read(key)?.doubleValue == 1 {
                pending.remove(key)
            }
            if !pending.isEmpty { Thread.sleep(forTimeInterval: 0.2) }
        }
        guard pending.isEmpty else { return "Fans did not enter manual mode" }
        engaged = true
        return nil
    }
}

final class FanHelperService: NSObject, FanHelperProtocol {
    private let controller: FanController
    private let queue: DispatchQueue

    init(controller: FanController, queue: DispatchQueue) {
        self.controller = controller
        self.queue = queue
    }

    func protocolVersion(reply: @escaping (Int) -> Void) {
        reply(FanHelperConstants.protocolVersion)
    }

    func setFanTargets(_ rpms: [NSNumber], reply: @escaping (String?) -> Void) {
        let values = rpms.map(\.doubleValue)
        queue.async { reply(self.controller.apply(values)) }
    }

    func restoreAutomatic(reply: @escaping (String?) -> Void) {
        queue.async { reply(self.controller.restoreAutomatic()) }
    }
}

final class ListenerDelegate: NSObject, NSXPCListenerDelegate {
    private let service: FanHelperService

    init(service: FanHelperService) {
        self.service = service
    }

    func listener(_ listener: NSXPCListener, shouldAcceptNewConnection connection: NSXPCConnection) -> Bool {
        connection.exportedInterface = NSXPCInterface(with: FanHelperProtocol.self)
        connection.exportedObject = service
        connection.resume()
        return true
    }
}

guard getuid() == 0, let smc = SMCConnection() else {
    fputs("PowerTop fan helper must run as root\n", stderr)
    exit(1)
}

// Refuse every client unless the installer recorded which app build may connect.
guard let requirement = try? String(contentsOfFile: FanHelperConstants.clientRequirementPath, encoding: .utf8)
    .trimmingCharacters(in: .whitespacesAndNewlines), !requirement.isEmpty else {
    fputs("PowerTop fan helper has no client requirement\n", stderr)
    exit(1)
}

let controlQueue = DispatchQueue(label: "\(FanHelperConstants.label).control")
let controller = controlQueue.sync { FanController(smc: smc) }
let delegate = ListenerDelegate(service: FanHelperService(controller: controller, queue: controlQueue))
let listener = NSXPCListener(machServiceName: FanHelperConstants.label)
listener.setConnectionCodeSigningRequirement(requirement)
listener.delegate = delegate
listener.resume()

let watchdog = DispatchSource.makeTimerSource(queue: controlQueue)
watchdog.schedule(deadline: .now() + 1, repeating: 1)
watchdog.setEventHandler { controller.tick() }
watchdog.resume()

dispatchMain()
