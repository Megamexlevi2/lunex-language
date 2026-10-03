//go:build linux || android || freebsd

package jit

import (
	"fmt"
	"golang.org/x/sys/unix"
)

func init() {
	allocateExecutable = unixAllocate
	protectExecutable = unixProtect
	releaseExecutable = unixRelease
}

func unixAllocate(size int) ([]byte, error) {
	if size <= 0 {
		return nil, fmt.Errorf("invalid executable memory size %d", size)
	}
	page := unix.Getpagesize()
	pages := (size + page - 1) / page
	total := pages * page
	code, err := unix.Mmap(-1, 0, total, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANON)
	if err != nil {
		return nil, err
	}
	return code, nil
}

func unixProtect(code []byte) error {
	if len(code) == 0 {
		return fmt.Errorf("empty executable buffer")
	}
	return unix.Mprotect(code, unix.PROT_READ|unix.PROT_EXEC)
}

func unixRelease(code []byte) error {
	if len(code) == 0 {
		return nil
	}
	return unix.Munmap(code)
}
