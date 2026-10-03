package jit

import (
	"fmt"
	"runtime"
	"unsafe"
)

var nativeAvailable bool
var archCallNative func(uintptr, uintptr)
var codeAddress = func(code []byte) uintptr {
	if len(code) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&code[0]))
}

func Available() bool {
	return nativeAvailable
}

func runNative(code, ctx uintptr) error {
	if !nativeAvailable || archCallNative == nil {
		return fmt.Errorf("native execution is unavailable")
	}
	archCallNative(code, ctx)
	runtime.KeepAlive(code)
	runtime.KeepAlive(ctx)
	return nil
}

func uintptrOfContext(ctx *Context) uintptr {
	if ctx == nil {
		return 0
	}
	return uintptr(unsafe.Pointer(ctx))
}
