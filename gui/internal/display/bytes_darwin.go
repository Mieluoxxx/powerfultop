//go:build darwin

package display

import "unsafe"

func nextByte(p *byte) *byte { return (*byte)(unsafe.Add(unsafe.Pointer(p), 1)) }
