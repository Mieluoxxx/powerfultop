// Package display controls the brightness of every connected display.
//
// Brightness paths and IOAVService discovery adapted from MonitorControl
// (https://github.com/MonitorControl/MonitorControl, MIT License, © MonitorControl contributors).
package display

import (
	"fmt"
	"math"
)

type Method int

const (
	// Native goes through DisplayServices: built-in and Apple displays.
	Native Method = iota + 1
	// DDC is DDC/CI over I²C: external monitors that accept VCP 0x10.
	DDC
	// Software scales the gamma table: it darkens the picture, the backlight stays put.
	Software
)

// Brightness is one display's slider.
type Brightness struct {
	ID      uint32
	Name    string // "" when macOS reports none
	BuiltIn bool
	Method  Method
	Level   float64
}

// DDC/CI framing for Apple silicon IOAVService I²C writes.
// Packet layout follows MonitorControl (MIT License, © MonitorControl contributors).
const (
	ddcChipAddress = 0x37
	ddcDataAddress = 0x51
	vcpLuminance   = 0x10
)

func getVCPPacket(code byte) []byte { return ddcPacket([]byte{0x01, code}) }

func setVCPPacket(code byte, value uint16) []byte {
	return ddcPacket([]byte{0x03, code, byte(value >> 8), byte(value)})
}

// ddcPacket seeds the checksum with the host address (0x6E) and the data address the transport
// sends as the register offset (0x51), since both are on the wire.
func ddcPacket(body []byte) []byte {
	packet := append([]byte{0x80 | byte(len(body))}, body...)
	sum := byte(ddcChipAddress<<1) ^ ddcDataAddress
	for _, b := range packet {
		sum ^= b
	}
	return append(packet, sum)
}

// parseVCPReply parses a Get VCP Feature reply. Monitors that ignore the request answer with a null
// message (6e 80 be …) whose checksum is valid, so the length and opcode must be checked too.
func parseVCPReply(reply []byte, code byte) (current, max uint16, ok bool) {
	if len(reply) < 11 || reply[1]&0x7F != 8 || reply[2] != 0x02 || reply[3] != 0x00 || reply[4] != code {
		return 0, 0, false
	}
	sum := byte(0x50)
	for _, b := range reply[:10] {
		sum ^= b
	}
	if sum != reply[10] {
		return 0, 0, false
	}
	max = uint16(reply[6])<<8 | uint16(reply[7])
	current = uint16(reply[8])<<8 | uint16(reply[9])
	if max == 0 {
		return 0, 0, false
	}
	return min(current, max), max, true
}

// softwareFloor keeps the darkest software level readable; a black screen can't be undone without
// seeing the slider.
const softwareFloor = 0.15

func gammaCeiling(level float64) float64 {
	return softwareFloor + (1-softwareFloor)*math.Min(math.Max(level, 0), 1)
}

// edidUUIDPrefix is how an IOAVService is tied back to a display: EDID UUIDs in the IORegistry begin
// with the vendor ID and the little-endian product code.
func edidUUIDPrefix(vendor, model uint32) string {
	product := uint16(model)
	return fmt.Sprintf("%04X%02X%02X", uint16(vendor), product&0xFF, product>>8)
}
