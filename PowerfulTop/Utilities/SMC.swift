import Foundation
import IOKit

/// Minimal AppleSMC client. Reads work as any user; writes need root and only the fan helper issues them.
final class SMCConnection {
    private var connection: io_connect_t = 0

    init?() {
        let service = IOServiceGetMatchingService(kIOMainPortDefault, IOServiceMatching("AppleSMC"))
        guard service != 0 else { return nil }
        defer { IOObjectRelease(service) }
        guard IOServiceOpen(service, mach_task_self_, 0, &connection) == KERN_SUCCESS else { return nil }
    }

    deinit {
        IOServiceClose(connection)
    }

    func read(_ key: String) -> SMCValue? {
        guard let info = keyInfo(key) else { return nil }
        var input = SMCParam()
        input.key = SMCParam.fourCC(key)
        input.keyInfo.dataSize = info.dataSize
        input.command = SMCParam.readKey
        guard let output = call(&input), output.result == 0 else { return nil }
        let bytes = withUnsafeBytes(of: output.bytes) { Array($0.prefix(Int(info.dataSize))) }
        return SMCValue(type: SMCParam.string(info.dataType), bytes: bytes)
    }

    /// Returns the SMC result byte (0 = success) or nil when the call itself failed, e.g. not root.
    @discardableResult
    func write(_ key: String, bytes: [UInt8]) -> UInt8? {
        guard let info = keyInfo(key), Int(info.dataSize) == bytes.count else { return nil }
        var input = SMCParam()
        input.key = SMCParam.fourCC(key)
        input.keyInfo.dataSize = info.dataSize
        input.command = SMCParam.writeKey
        withUnsafeMutableBytes(of: &input.bytes) { raw in
            for (index, byte) in bytes.enumerated() { raw[index] = byte }
        }
        return call(&input)?.result
    }

    func keyCount() -> Int {
        guard let value = read("#KEY"), value.bytes.count >= 4 else { return 0 }
        return Int(value.bytes.prefix(4).reduce(UInt32(0)) { $0 << 8 | UInt32($1) })
    }

    func key(at index: Int) -> String? {
        var input = SMCParam()
        input.command = SMCParam.keyFromIndex
        input.data32 = UInt32(index)
        guard let output = call(&input), output.result == 0 else { return nil }
        return SMCParam.string(output.key)
    }

    private func keyInfo(_ key: String) -> SMCParam.KeyInfo? {
        var input = SMCParam()
        input.key = SMCParam.fourCC(key)
        input.command = SMCParam.getKeyInfo
        guard let output = call(&input), output.result == 0, output.keyInfo.dataSize > 0 else { return nil }
        return output.keyInfo
    }

    private func call(_ input: inout SMCParam) -> SMCParam? {
        var output = SMCParam()
        var outputSize = MemoryLayout<SMCParam>.stride
        let result = IOConnectCallStructMethod(
            connection,
            SMCParam.handleYPCEvent,
            &input,
            MemoryLayout<SMCParam>.stride,
            &output,
            &outputSize
        )
        return result == KERN_SUCCESS ? output : nil
    }
}

struct SMCValue: Equatable {
    let type: String
    let bytes: [UInt8]

    var doubleValue: Double? {
        switch type {
        case "flt " where bytes.count == 4:
            return Double(bytes.withUnsafeBytes { $0.loadUnaligned(as: Float.self) })
        case "ui8 " where bytes.count == 1:
            return Double(bytes[0])
        case "ui16" where bytes.count == 2:
            return Double(UInt16(bytes[0]) << 8 | UInt16(bytes[1]))
        case "ui32" where bytes.count == 4:
            return Double(bytes.reduce(UInt32(0)) { $0 << 8 | UInt32($1) })
        default:
            return nil
        }
    }

    /// Apple Silicon fan keys (`F%dTg`, `F%dAc`) are little-endian 32-bit floats.
    static func floatBytes(_ value: Double) -> [UInt8] {
        withUnsafeBytes(of: Float(value)) { Array($0) }
    }
}

/// Mirrors the kernel's `SMCKeyData_t` (80 bytes). Swift packs nested structs by size rather than
/// stride, so `KeyInfo` carries explicit tail padding to keep later fields at their C offsets.
struct SMCParam {
    struct Version { var major: UInt8 = 0, minor: UInt8 = 0, build: UInt8 = 0, reserved: UInt8 = 0, release: UInt16 = 0 }
    struct PLimit { var version: UInt16 = 0, length: UInt16 = 0, cpu: UInt32 = 0, gpu: UInt32 = 0, memory: UInt32 = 0 }
    struct KeyInfo { var dataSize: UInt32 = 0, dataType: UInt32 = 0, attributes: UInt8 = 0, padding: (UInt8, UInt8, UInt8) = (0, 0, 0) }

    static let handleYPCEvent: UInt32 = 2
    static let readKey: UInt8 = 5
    static let writeKey: UInt8 = 6
    static let keyFromIndex: UInt8 = 8
    static let getKeyInfo: UInt8 = 9

    var key: UInt32 = 0
    var version = Version()
    var pLimit = PLimit()
    var keyInfo = KeyInfo()
    var result: UInt8 = 0
    var status: UInt8 = 0
    var command: UInt8 = 0
    var data32: UInt32 = 0
    var bytes: (UInt64, UInt64, UInt64, UInt64) = (0, 0, 0, 0)

    static func fourCC(_ key: String) -> UInt32 {
        key.utf8.prefix(4).reduce(0) { $0 << 8 | UInt32($1) }
    }

    static func string(_ code: UInt32) -> String {
        String(decoding: [24, 16, 8, 0].map { UInt8((code >> $0) & 0xFF) }, as: UTF8.self)
    }
}
