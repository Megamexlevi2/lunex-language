//go:build windows

package std

import (
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/windows"
)

func ffiOpenLibrary(path string, _, _ bool) (uintptr, error) {
	h, err := windows.LoadLibrary(path)
	if err != nil {
		return 0, err
	}
	return uintptr(h), nil
}

func ffiLookupSymbol(handle uintptr, name string) (uintptr, error) {
	addr, err := windows.GetProcAddress(windows.Handle(handle), name)
	if err != nil {
		return 0, err
	}
	return addr, nil
}

func ffiCloseLibrary(handle uintptr) error {
	return windows.FreeLibrary(windows.Handle(handle))
}

var ffiAllocator struct {
	sync.Mutex
	initialized bool
	malloc      func(uintptr) uintptr
	calloc      func(uintptr, uintptr) uintptr
	realloc     func(uintptr, uintptr) uintptr
	free        func(uintptr)
	memcpy      func(uintptr, uintptr, uintptr) uintptr
	memset      func(uintptr, uint8, uintptr) uintptr
}

func ffiInitAllocator() error {
	ffiAllocator.Lock()
	defer ffiAllocator.Unlock()
	if ffiAllocator.initialized {
		return nil
	}
	var h uintptr
	var err error
	for _, name := range []string{"ucrtbase.dll", "msvcrt.dll"} {
		var handle windows.Handle
		handle, err = windows.LoadLibrary(name)
		if err == nil {
			h = uintptr(handle)
			break
		}
	}
	if h == 0 {
		return err
	}
	register := func(dst any, name string) error {
		sym, lookupErr := windows.GetProcAddress(windows.Handle(h), name)
		if lookupErr != nil || sym == 0 {
			if lookupErr == nil {
				lookupErr = fmt.Errorf("symbol %q is unavailable", name)
			}
			return lookupErr
		}
		return ffiRegisterFunc(dst, sym)
	}
	for _, item := range []struct {
		dst  any
		name string
	}{
		{&ffiAllocator.malloc, "malloc"},
		{&ffiAllocator.calloc, "calloc"},
		{&ffiAllocator.realloc, "realloc"},
		{&ffiAllocator.free, "free"},
		{&ffiAllocator.memcpy, "memcpy"},
		{&ffiAllocator.memset, "memset"},
	} {
		if err := register(item.dst, item.name); err != nil {
			_ = windows.FreeLibrary(windows.Handle(h))
			return err
		}
	}
	ffiAllocator.initialized = true
	return nil
}

func ffiMalloc(size uintptr) (uintptr, error) {
	if err := ffiInitAllocator(); err != nil {
		return 0, err
	}
	return ffiAllocator.malloc(size), nil
}

func ffiCalloc(count, size uintptr) (uintptr, error) {
	if err := ffiInitAllocator(); err != nil {
		return 0, err
	}
	return ffiAllocator.calloc(count, size), nil
}

func ffiRealloc(ptr, size uintptr) (uintptr, error) {
	if err := ffiInitAllocator(); err != nil {
		return 0, err
	}
	return ffiAllocator.realloc(ptr, size), nil
}

func ffiFree(ptr uintptr) error {
	if ptr == 0 {
		return nil
	}
	if err := ffiInitAllocator(); err != nil {
		return err
	}
	ffiAllocator.free(ptr)
	return nil
}

func ffiCopy(dst, src, size uintptr) error {
	if size == 0 {
		return nil
	}
	if err := ffiInitAllocator(); err != nil {
		return err
	}
	ffiAllocator.memcpy(dst, src, size)
	return nil
}

func ffiFill(dst uintptr, value uint8, size uintptr) error {
	if size == 0 {
		return nil
	}
	if err := ffiInitAllocator(); err != nil {
		return err
	}
	ffiAllocator.memset(dst, value, size)
	return nil
}

func ffiZero(dst, size uintptr) error {
	return ffiFill(dst, 0, size)
}

func ffiRegisterFunc(dst any, symbol uintptr) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("native ABI backend rejected the function declaration: %v", recovered)
		}
	}()
	purego.RegisterFunc(dst, symbol)
	return nil
}

func ffiNewCallback(fn any) (address uintptr, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("native ABI backend rejected the callback declaration: %v", recovered)
		}
	}()
	address = purego.NewCallback(fn)
	if address == 0 {
		return 0, fmt.Errorf("native ABI backend returned a null callback address")
	}
	return address, nil
}
