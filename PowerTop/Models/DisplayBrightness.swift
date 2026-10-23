import Foundation

enum BrightnessMethod: Equatable {
    /// DisplayServices: built-in and Apple displays.
    case native
    /// DDC/CI over I²C: external monitors that accept VCP 0x10.
    case ddc
    /// Gamma table scaling: darkens the picture, the backlight stays put.
    case software
}

/// DDC/CI framing for Apple silicon `IOAVService` I²C writes.
/// Packet layout follows MonitorControl (MIT License, © MonitorControl contributors).
enum DDC {
    static let chipAddress: UInt8 = 0x37
    static let dataAddress: UInt8 = 0x51
    static let luminance: UInt8 = 0x10

    static func getVCPPacket(_ code: UInt8) -> [UInt8] {
        packet([0x01, code])
    }

    static func setVCPPacket(_ code: UInt8, value: UInt16) -> [UInt8] {
        packet([0x03, code, UInt8(value >> 8), UInt8(value & 0xFF)])
    }

    /// The host address (0x6E) and the data address the transport sends as the register offset (0x51)
    /// are on the wire, so both seed the checksum.
    private static func packet(_ body: [UInt8]) -> [UInt8] {
        var bytes = [0x80 | UInt8(body.count)] + body
        bytes.append(bytes.reduce(chipAddress << 1 ^ dataAddress, ^))
        return bytes
    }

    /// Parses a Get VCP Feature reply. Monitors that ignore the request answer with a null message
    /// (`6e 80 be …`) whose checksum is valid, so the length and opcode must be checked too.
    static func parseVCPReply(_ reply: [UInt8], code: UInt8) -> (current: UInt16, max: UInt16)? {
        guard reply.count >= 11,
              reply[1] & 0x7F == 8,
              reply[2] == 0x02,
              reply[3] == 0x00,
              reply[4] == code,
              reply[..<10].reduce(0x50, ^) == reply[10]
        else { return nil }
        let max = UInt16(reply[6]) << 8 | UInt16(reply[7])
        let current = UInt16(reply[8]) << 8 | UInt16(reply[9])
        guard max > 0 else { return nil }
        return (min(current, max), max)
    }
}

enum SoftwareDimming {
    /// Keeps the darkest software level readable; a black screen can't be undone without seeing the slider.
    static let floor = 0.15

    static func gammaCeiling(level: Double) -> Double {
        floor + (1 - floor) * min(max(level, 0), 1)
    }
}

/// EDID UUIDs in the IORegistry begin with the vendor ID and the little-endian product code,
/// which is how an `IOAVService` is tied back to a `CGDirectDisplayID`.
func edidUUIDPrefix(vendor: UInt32, model: UInt32) -> String {
    let product = UInt16(truncatingIfNeeded: model)
    return String(format: "%04X%02X%02X", UInt16(truncatingIfNeeded: vendor), product & 0xFF, product >> 8)
}
