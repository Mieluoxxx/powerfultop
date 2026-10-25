package display

import (
	"bytes"
	"math"
	"testing"
)

// Mirrors testDDCFraming in the Swift regression tests.
func TestDDCFraming(t *testing.T) {
	// Reference bytes from the DDC/CI spec examples: 6E 51 82 01 10 AC and 6E 51 84 03 10 00 32 9A.
	if got := getVCPPacket(0x10); !bytes.Equal(got, []byte{0x82, 0x01, 0x10, 0xAC}) {
		t.Errorf("get packet % X", got)
	}
	if got := setVCPPacket(0x10, 50); !bytes.Equal(got, []byte{0x84, 0x03, 0x10, 0x00, 0x32, 0x9A}) {
		t.Errorf("set packet % X", got)
	}

	reply := []byte{0x6E, 0x88, 0x02, 0x00, 0x10, 0x00, 0x00, 0x64, 0x00, 0x32, 0xF2}
	if cur, max, ok := parseVCPReply(reply, 0x10); !ok || cur != 50 || max != 100 {
		t.Errorf("reply %d/%d %v", cur, max, ok)
	}
	// DELL U2520DR with DDC/CI off: valid checksum, zero length.
	if _, _, ok := parseVCPReply([]byte{0x6E, 0x80, 0xBE, 0, 0, 0, 0, 0, 0, 0, 0}, 0x10); ok {
		t.Error("null message is not a reading")
	}
	corrupt := append([]byte(nil), reply...)
	corrupt[9] = 0x33
	if _, _, ok := parseVCPReply(corrupt, 0x10); ok {
		t.Error("bad checksum")
	}
	unsupported := append([]byte(nil), reply...)
	unsupported[3] = 0x01
	unsupported[10] ^= 0x01
	if _, _, ok := parseVCPReply(unsupported, 0x10); ok {
		t.Error("unsupported VCP code")
	}
	if _, _, ok := parseVCPReply(reply, 0x12); ok {
		t.Error("reply for another VCP code")
	}

	if got := edidUUIDPrefix(4268, 41299); got != "10AC53A1" {
		t.Errorf("DELL U2520DR EDID UUID prefix %q", got)
	}
	if math.Abs(gammaCeiling(0)-softwareFloor) > 1e-9 || gammaCeiling(1) != 1 || gammaCeiling(2) != 1 {
		t.Error("software dimming range")
	}
}
