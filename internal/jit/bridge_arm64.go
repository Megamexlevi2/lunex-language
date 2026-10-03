//go:build arm64

package jit

import "unsafe"

func init() {
	nativeAvailable = true
	archCallNative = callNativeEntry
	codeAddress = func(code []byte) uintptr { return uintptr(unsafe.Pointer(&code[0])) }
}

func callNativeEntry(code, ctx uintptr)
