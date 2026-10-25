package thermal

import (
	"encoding/binary"
	"math"
)

// Param mirrors the kernel's SMCKeyData_t: AppleSMC rejects every call unless it is exactly 80 bytes
// with these field offsets.
type Param [ParamSize]byte

const (
	ParamSize = 80

	offsetKey           = 0
	offsetDataSize      = 28 // keyInfo
	offsetDataType      = 32
	offsetResult        = 40
	offsetStatus        = 41
	offsetCommand       = 42
	offsetData32        = 44
	offsetBytes         = 48
	maxValueBytes       = 32
	selectorYPCEvent    = 2
	commandReadKey      = 5
	commandWriteKey     = 6
	commandKeyFromIndex = 8
	commandGetKeyInfo   = 9
)

// FourCC packs an SMC key big-endian, as the kernel expects.
func FourCC(key string) uint32 {
	var code uint32
	for i := 0; i < 4 && i < len(key); i++ {
		code = code<<8 | uint32(key[i])
	}
	return code
}

// FourCCString unpacks a key.
func FourCCString(code uint32) string {
	return string([]byte{byte(code >> 24), byte(code >> 16), byte(code >> 8), byte(code)})
}

// The struct is native-endian (little on Apple Silicon and Intel) apart from the value bytes.
func (p *Param) key() uint32          { return binary.LittleEndian.Uint32(p[offsetKey:]) }
func (p *Param) setKey(k uint32)      { binary.LittleEndian.PutUint32(p[offsetKey:], k) }
func (p *Param) dataSize() uint32     { return binary.LittleEndian.Uint32(p[offsetDataSize:]) }
func (p *Param) setDataSize(n uint32) { binary.LittleEndian.PutUint32(p[offsetDataSize:], n) }
func (p *Param) dataType() uint32     { return binary.LittleEndian.Uint32(p[offsetDataType:]) }
func (p *Param) result() uint8        { return p[offsetResult] }
func (p *Param) setCommand(c uint8)   { p[offsetCommand] = c }
func (p *Param) setData32(v uint32)   { binary.LittleEndian.PutUint32(p[offsetData32:], v) }
func (p *Param) value(n int) []byte   { return append([]byte(nil), p[offsetBytes:offsetBytes+n]...) }
func (p *Param) setValue(b []byte)    { copy(p[offsetBytes:offsetBytes+maxValueBytes], b) }

// Value is one SMC key's typed bytes.
type Value struct {
	Type  string
	Bytes []byte
}

// Float decodes the numeric types PowerfulTop reads.
func (v Value) Float() (float64, bool) {
	switch {
	case v.Type == "flt " && len(v.Bytes) == 4:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(v.Bytes))), true
	case v.Type == "ui8 " && len(v.Bytes) == 1:
		return float64(v.Bytes[0]), true
	case v.Type == "ui16" && len(v.Bytes) == 2:
		return float64(binary.BigEndian.Uint16(v.Bytes)), true
	case v.Type == "ui32" && len(v.Bytes) == 4:
		return float64(binary.BigEndian.Uint32(v.Bytes)), true
	}
	return 0, false
}

// FloatBytes encodes Apple Silicon fan keys (F%dTg, F%dAc), which are little-endian 32-bit floats.
func FloatBytes(value float64) []byte {
	return binary.LittleEndian.AppendUint32(nil, math.Float32bits(float32(value)))
}

// SMC is the subset of AppleSMC the monitor and the helper use; tests substitute a fake.
type SMC interface {
	Read(key string) (Value, bool)
	// Write returns the SMC result byte (0 = success); ok is false when the call itself failed, e.g. not root.
	Write(key string, bytes []byte) (result uint8, ok bool)
	KeyCount() int
	KeyAt(index int) (string, bool)
}

func readFloat(smc SMC, key string) (float64, bool) {
	v, ok := smc.Read(key)
	if !ok {
		return 0, false
	}
	return v.Float()
}
