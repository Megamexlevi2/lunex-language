//go:build windows

package jit

import (
	"fmt"
	"golang.org/x/sys/windows"
	"unsafe"
)

func init() {
	allocateExecutable = windowsAllocate
	protectExecutable = windowsProtect
	releaseExecutable = windowsRelease
}

func windowsAllocate(size int) ([]byte, error) {
	if size <= 0 {
		return nil, fmt.Errorf("invalid executable memory size %d", size)
	}
	pageSize := 4096
	aligned := ((size + pageSize - 1) / pageSize) * pageSize
	allocation := uintptr(aligned)
	ptr, err := windows.VirtualAlloc(0, allocation, windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil || ptr == 0 {
		if err == nil {
			err = fmt.Errorf("VirtualAlloc returned a null address")
		}
		return nil, err
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(ptr)), aligned), nil
}

func windowsProtect(code []byte) error {
	if len(code) == 0 {
		return fmt.Errorf("empty executable buffer")
	}
	var old uint32
	return windows.VirtualProtect(uintptr(unsafe.Pointer(&code[0])), uintptr(len(code)), windows.PAGE_EXECUTE_READ, &old)
}

func windowsRelease(code []byte) error {
	if len(code) == 0 {
		return nil
	}
	return windows.VirtualFree(uintptr(unsafe.Pointer(&code[0])), 0, windows.MEM_RELEASE)
}
