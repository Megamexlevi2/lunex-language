//go:build windows

package jit

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var flushInstructionCacheProc = windows.NewLazySystemDLL("kernel32.dll").NewProc("FlushInstructionCache")

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
	if err := windows.VirtualProtect(uintptr(unsafe.Pointer(&code[0])), uintptr(len(code)), windows.PAGE_EXECUTE_READ, &old); err != nil {
		return err
	}
	handle, err := windows.GetCurrentProcess()
	if err != nil {
		return err
	}
	if err := flushInstructionCacheProc.Find(); err != nil {
		return err
	}
	result, _, callErr := flushInstructionCacheProc.Call(uintptr(handle), uintptr(unsafe.Pointer(&code[0])), uintptr(len(code)))
	if result == 0 {
		if callErr != nil {
			return callErr
		}
		return fmt.Errorf("FlushInstructionCache failed")
	}
	return nil
}

func windowsRelease(code []byte) error {
	if len(code) == 0 {
		return nil
	}
	return windows.VirtualFree(uintptr(unsafe.Pointer(&code[0])), 0, windows.MEM_RELEASE)
}
