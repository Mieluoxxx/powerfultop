import Foundation
import Security

enum FanHelperStatus: Equatable {
    case notInstalled
    /// Installed helper or its recorded client requirement no longer matches this app build.
    case needsUpdate
    case ready
}

/// Installs and talks to the root fan helper. The app itself never writes to the SMC.
final class FanHelperClient {
    private var connection: NSXPCConnection?

    private var bundledHelperURL: URL? {
        Bundle.main.url(forAuxiliaryExecutable: "PowerfulTopFanHelper")
    }

    var status: FanHelperStatus {
        let fileManager = FileManager.default
        // A helper left by the upstream bundle ID would otherwise stay registered as a root daemon forever.
        if fileManager.fileExists(atPath: FanHelperConstants.Legacy.launchDaemonPath) {
            return .needsUpdate
        }
        guard fileManager.fileExists(atPath: FanHelperConstants.installedPath),
              fileManager.fileExists(atPath: FanHelperConstants.launchDaemonPath) else {
            return .notInstalled
        }
        guard let bundled = bundledHelperURL.flatMap({ try? Data(contentsOf: $0) }),
              let installed = fileManager.contents(atPath: FanHelperConstants.installedPath),
              bundled == installed,
              let recorded = try? String(contentsOfFile: FanHelperConstants.clientRequirementPath, encoding: .utf8),
              recorded.trimmingCharacters(in: .whitespacesAndNewlines) == Self.clientRequirement() else {
            return .needsUpdate
        }
        return .ready
    }

    /// Prompts for an administrator password once, then registers the helper as a launchd daemon.
    func install() throws {
        guard let helper = bundledHelperURL else { throw FanHelperError.missingHelper }
        guard let requirement = Self.clientRequirement() else { throw FanHelperError.unsignedApp }

        let label = FanHelperConstants.label
        let plist = """
        <?xml version="1.0" encoding="UTF-8"?>
        <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
        <plist version="1.0"><dict>
        <key>Label</key><string>\(label)</string>
        <key>Program</key><string>\(FanHelperConstants.installedPath)</string>
        <key>MachServices</key><dict><key>\(label)</key><true/></dict>
        </dict></plist>
        """
        let supportDir = FanHelperConstants.supportDirectory
        let script = """
        set -e
        launchctl bootout system/\(label) 2>/dev/null || true
        mkdir -p /Library/PrivilegedHelperTools \(shellQuote(supportDir))
        \(legacyCleanupScript)
        cp \(shellQuote(helper.path)) \(shellQuote(FanHelperConstants.installedPath))
        chown root:wheel \(shellQuote(FanHelperConstants.installedPath))
        chmod 755 \(shellQuote(FanHelperConstants.installedPath))
        printf '%s' \(shellQuote(requirement)) > \(shellQuote(FanHelperConstants.clientRequirementPath))
        chown root:wheel \(shellQuote(FanHelperConstants.clientRequirementPath))
        chmod 644 \(shellQuote(FanHelperConstants.clientRequirementPath))
        printf '%s' \(shellQuote(plist)) > \(shellQuote(FanHelperConstants.launchDaemonPath))
        chown root:wheel \(shellQuote(FanHelperConstants.launchDaemonPath))
        chmod 644 \(shellQuote(FanHelperConstants.launchDaemonPath))
        launchctl bootstrap system \(shellQuote(FanHelperConstants.launchDaemonPath))
        """
        try runAsAdministrator(script)
        invalidate()
    }

    func uninstall() throws {
        let label = FanHelperConstants.label
        let script = """
        launchctl bootout system/\(label) 2>/dev/null || true
        rm -f \(shellQuote(FanHelperConstants.launchDaemonPath)) \(shellQuote(FanHelperConstants.installedPath)) \(shellQuote(FanHelperConstants.clientRequirementPath))
        \(legacyCleanupScript)
        """
        try runAsAdministrator(script)
        invalidate()
    }

    /// The legacy helper has no SIGTERM handler, so if it held the fans when unloaded they stay manual.
    /// Moving its engaged marker makes the new helper restore automatic control as soon as it starts.
    private var legacyCleanupScript: String {
        let legacy = FanHelperConstants.Legacy.self
        return """
        if [ -e \(shellQuote(legacy.engagedMarkerPath)) ]; then mkdir -p \(shellQuote(FanHelperConstants.supportDirectory)); touch \(shellQuote(FanHelperConstants.engagedMarkerPath)); fi
        launchctl bootout system/\(legacy.label) 2>/dev/null || true
        rm -f \(shellQuote(legacy.launchDaemonPath)) \(shellQuote(legacy.installedPath))
        rm -rf \(shellQuote(legacy.supportDirectory))
        """
    }

    func setFanTargets(_ rpms: [Double], completion: @escaping (String?) -> Void) {
        guard let proxy = proxy(onError: { completion($0.localizedDescription) }) else {
            completion(FanHelperError.connectionFailed.localizedDescription)
            return
        }
        proxy.setFanTargets(rpms.map { NSNumber(value: $0) }, reply: completion)
    }

    func restoreAutomatic(completion: @escaping (String?) -> Void) {
        guard let proxy = proxy(onError: { completion($0.localizedDescription) }) else {
            completion(FanHelperError.connectionFailed.localizedDescription)
            return
        }
        proxy.restoreAutomatic(reply: completion)
    }

    /// Blocks so fans are back under macOS control before the app exits.
    func restoreAutomaticSynchronously(timeout: TimeInterval = 3) {
        let done = DispatchSemaphore(value: 0)
        restoreAutomatic { _ in done.signal() }
        _ = done.wait(timeout: .now() + timeout)
    }

    func invalidate() {
        connection?.invalidate()
        connection = nil
    }

    private func proxy(onError: @escaping (Error) -> Void) -> FanHelperProtocol? {
        if connection == nil {
            let newConnection = NSXPCConnection(machServiceName: FanHelperConstants.label, options: .privileged)
            newConnection.remoteObjectInterface = NSXPCInterface(with: FanHelperProtocol.self)
            newConnection.invalidationHandler = { [weak self] in
                DispatchQueue.main.async { self?.connection = nil }
            }
            newConnection.resume()
            connection = newConnection
        }
        return connection?.remoteObjectProxyWithErrorHandler(onError) as? FanHelperProtocol
    }

    /// Pins the helper to this exact build: ad-hoc signatures have no team identity to check instead.
    static func clientRequirement() -> String? {
        var code: SecCode?
        guard SecCodeCopySelf([], &code) == errSecSuccess, let code else { return nil }
        var staticCode: SecStaticCode?
        guard SecCodeCopyStaticCode(code, [], &staticCode) == errSecSuccess, let staticCode else { return nil }
        var info: CFDictionary?
        guard SecCodeCopySigningInformation(staticCode, SecCSFlags(rawValue: kSecCSSigningInformation), &info) == errSecSuccess,
              let dictionary = info as? [String: Any],
              let cdhash = dictionary[kSecCodeInfoUnique as String] as? Data else { return nil }
        let hex = cdhash.map { String(format: "%02x", $0) }.joined()
        return "cdhash H\"\(hex)\""
    }

    private func runAsAdministrator(_ script: String) throws {
        let source = "do shell script \(appleScriptQuote(script)) with administrator privileges"
        var error: NSDictionary?
        guard let appleScript = NSAppleScript(source: source) else { throw FanHelperError.installFailed("AppleScript") }
        appleScript.executeAndReturnError(&error)
        if let error {
            let message = error[NSAppleScript.errorMessage] as? String ?? "\(error)"
            if (error[NSAppleScript.errorNumber] as? Int) == -128 { throw FanHelperError.cancelled }
            throw FanHelperError.installFailed(message)
        }
    }

    private func shellQuote(_ value: String) -> String {
        "'" + value.replacingOccurrences(of: "'", with: "'\\''") + "'"
    }

    private func appleScriptQuote(_ value: String) -> String {
        "\"" + value.replacingOccurrences(of: "\\", with: "\\\\").replacingOccurrences(of: "\"", with: "\\\"") + "\""
    }
}

enum FanHelperError: LocalizedError, Equatable {
    case missingHelper
    case unsignedApp
    case cancelled
    case connectionFailed
    case installFailed(String)

    var errorDescription: String? {
        switch self {
        case .missingHelper: return String(localized: "The fan helper is missing from the app bundle.")
        case .unsignedApp: return String(localized: "PowerfulTop must be code-signed to control fans.")
        case .cancelled: return String(localized: "Authorization was cancelled.")
        case .connectionFailed: return String(localized: "Could not connect to the fan helper.")
        case .installFailed(let message): return message
        }
    }
}
