//go:build darwin && cgo

package jit

/*
#include <pthread.h>
#include <libkern/OSCacheControl.h>

static void lunex_jit_write_enable(void) { pthread_jit_write_protect_np(0); }
static void lunex_jit_write_disable(void) { pthread_jit_write_protect_np(1); }
static void lunex_jit_sync_icache(void *addr, size_t len) { sys_icache_invalidate(addr, len); }
*/
import "C"

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

func init() {
	allocateExecutable = darwinAllocate
	protectExecutable = darwinProtect
	releaseExecutable = darwinRelease
}

func darwinAllocate(size int) ([]byte, error) {
	if size <= 0 {
		return nil, fmt.Errorf("invalid executable memory size %d", size)
	}
	page := unix.Getpagesize()
	pages := (size + page - 1) / page
	total := pages * page
	code, err := unix.Mmap(-1, 0, total, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANON|unix.MAP_JIT)
	if err != nil {
		return nil, err
	}
	C.lunex_jit_write_enable()
	return code, nil
}

func darwinProtect(code []byte) error {
	if len(code) == 0 {
		return fmt.Errorf("empty executable buffer")
	}
	C.lunex_jit_write_disable()
	C.lunex_jit_sync_icache(unsafe.Pointer(&code[0]), C.size_t(len(code)))
	return nil
}

func darwinRelease(code []byte) error {
	if len(code) == 0 {
		return nil
	}
	return unix.Munmap(code)
}
