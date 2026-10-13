//go:build arm64

package jit

import "unsafe"

//go:noescape
func icacheSyncRange(addr uintptr, length uintptr)

func syncInstructionCache(code []byte) {
	if len(code) == 0 {
		return
	}
	icacheSyncRange(uintptr(unsafe.Pointer(&code[0])), uintptr(len(code)))
}
