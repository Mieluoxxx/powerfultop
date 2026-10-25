//go:build darwin

package thermal

import (
	"errors"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

var (
	smcOnce sync.Once
	smcErr  error

	ioServiceMatching           func(name string) uintptr
	ioServiceGetMatchingService func(mainPort uint32, matching uintptr) uint32
	ioServiceOpen               func(service, owningTask, kind uint32, connect *uint32) int32
	ioServiceClose              func(connect uint32) int32
	ioObjectRelease             func(object uint32) int32
	ioConnectCallStructMethod   func(connect, selector uint32, input unsafe.Pointer, inputSize uintptr, output unsafe.Pointer, outputSize *uintptr) int32
	machTaskSelf                uint32
)

func loadIOKit() error {
	smcOnce.Do(func() {
		iokit, err := purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			smcErr = err
			return
		}
		system, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			smcErr = err
			return
		}
		purego.RegisterLibFunc(&ioServiceMatching, iokit, "IOServiceMatching")
		purego.RegisterLibFunc(&ioServiceGetMatchingService, iokit, "IOServiceGetMatchingService")
		purego.RegisterLibFunc(&ioServiceOpen, iokit, "IOServiceOpen")
		purego.RegisterLibFunc(&ioServiceClose, iokit, "IOServiceClose")
		purego.RegisterLibFunc(&ioObjectRelease, iokit, "IOObjectRelease")
		purego.RegisterLibFunc(&ioConnectCallStructMethod, iokit, "IOConnectCallStructMethod")
		// mach_task_self() is a macro over this global port name.
		addr, err := purego.Dlsym(system, "mach_task_self_")
		if err != nil {
			smcErr = err
			return
		}
		var port *uint32
		*(*uintptr)(unsafe.Pointer(&port)) = addr
		machTaskSelf = *port
	})
	return smcErr
}

// SMCConnection is a minimal AppleSMC client. Reads work as any user; writes need root and only the
// fan helper issues them. Calls on one connection are serialized.
type SMCConnection struct {
	mu   sync.Mutex
	conn uint32
}

// OpenSMC connects to AppleSMC.
func OpenSMC() (*SMCConnection, error) {
	if err := loadIOKit(); err != nil {
		return nil, err
	}
	service := ioServiceGetMatchingService(0, ioServiceMatching("AppleSMC"))
	if service == 0 {
		return nil, errors.New("thermal: no AppleSMC service")
	}
	defer ioObjectRelease(service)
	s := &SMCConnection{}
	if kr := ioServiceOpen(service, machTaskSelf, 0, &s.conn); kr != 0 {
		return nil, errors.New("thermal: IOServiceOpen(AppleSMC) failed")
	}
	return s, nil
}

func (s *SMCConnection) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != 0 {
		ioServiceClose(s.conn)
		s.conn = 0
	}
}

func (s *SMCConnection) call(in *Param) (*Param, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == 0 {
		return nil, false
	}
	var out Param
	size := uintptr(ParamSize)
	if ioConnectCallStructMethod(s.conn, selectorYPCEvent, unsafe.Pointer(&in[0]), ParamSize, unsafe.Pointer(&out[0]), &size) != 0 {
		return nil, false
	}
	return &out, true
}

func (s *SMCConnection) keyInfo(key string) (size, kind uint32, ok bool) {
	var in Param
	in.setKey(FourCC(key))
	in.setCommand(commandGetKeyInfo)
	out, ok := s.call(&in)
	if !ok || out.result() != 0 || out.dataSize() == 0 || out.dataSize() > maxValueBytes {
		return 0, 0, false
	}
	return out.dataSize(), out.dataType(), true
}

func (s *SMCConnection) Read(key string) (Value, bool) {
	size, kind, ok := s.keyInfo(key)
	if !ok {
		return Value{}, false
	}
	var in Param
	in.setKey(FourCC(key))
	in.setDataSize(size)
	in.setCommand(commandReadKey)
	out, ok := s.call(&in)
	if !ok || out.result() != 0 {
		return Value{}, false
	}
	return Value{Type: FourCCString(kind), Bytes: out.value(int(size))}, true
}

func (s *SMCConnection) Write(key string, bytes []byte) (uint8, bool) {
	size, _, ok := s.keyInfo(key)
	if !ok || int(size) != len(bytes) {
		return 0, false
	}
	var in Param
	in.setKey(FourCC(key))
	in.setDataSize(size)
	in.setCommand(commandWriteKey)
	in.setValue(bytes)
	out, ok := s.call(&in)
	if !ok {
		return 0, false
	}
	return out.result(), true
}

func (s *SMCConnection) KeyCount() int {
	v, ok := s.Read("#KEY")
	if !ok || len(v.Bytes) < 4 {
		return 0
	}
	return int(uint32(v.Bytes[0])<<24 | uint32(v.Bytes[1])<<16 | uint32(v.Bytes[2])<<8 | uint32(v.Bytes[3]))
}

func (s *SMCConnection) KeyAt(index int) (string, bool) {
	var in Param
	in.setCommand(commandKeyFromIndex)
	in.setData32(uint32(index))
	out, ok := s.call(&in)
	if !ok || out.result() != 0 {
		return "", false
	}
	return FourCCString(out.key()), true
}
