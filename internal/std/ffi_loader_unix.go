//go:build linux || android || darwin || freebsd || netbsd

package std

import (
	"fmt"
	"os"
	goruntime "runtime"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
)

func ffiOpenLibrary(path string, global, lazy bool) (uintptr, error) {
	mode := purego.RTLD_NOW
	if lazy {
		mode = purego.RTLD_LAZY
	}
	if global {
		mode |= purego.RTLD_GLOBAL
	} else {
		mode |= purego.RTLD_LOCAL
	}

	var lastErr error
	for _, candidate := range ffiLibraryCandidates(path) {
		h, err := purego.Dlopen(candidate, mode)
		if err == nil {
			return h, nil
		}
		lastErr = err
	}
	return 0, lastErr
}

func ffiLookupSymbol(handle uintptr, name string) (uintptr, error) {
	return purego.Dlsym(handle, name)
}

func ffiCloseLibrary(handle uintptr) error {
	return purego.Dlclose(handle)
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
	h, err := purego.Dlopen(ffiSystemLibrary(), purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}
	register := func(dst any, name string) error {
		sym, lookupErr := purego.Dlsym(h, name)
		if lookupErr != nil || sym == 0 {
			if lookupErr == nil {
				lookupErr = fmt.Errorf("symbol %q is unavailable", name)
			}
			return lookupErr
		}
		return ffiRegisterFunc(dst, sym)
	}
	if err := register(&ffiAllocator.malloc, "malloc"); err != nil {
		_ = purego.Dlclose(h)
		return err
	}
	if err := register(&ffiAllocator.calloc, "calloc"); err != nil {
		_ = purego.Dlclose(h)
		return err
	}
	if err := register(&ffiAllocator.realloc, "realloc"); err != nil {
		_ = purego.Dlclose(h)
		return err
	}
	if err := register(&ffiAllocator.free, "free"); err != nil {
		_ = purego.Dlclose(h)
		return err
	}
	if err := register(&ffiAllocator.memcpy, "memcpy"); err != nil {
		_ = purego.Dlclose(h)
		return err
	}
	if err := register(&ffiAllocator.memset, "memset"); err != nil {
		_ = purego.Dlclose(h)
		return err
	}
	ffiAllocator.initialized = true
	return nil
}

func ffiSystemLibrary() string {
	if ffiAndroidRuntime() {
		return "libc.so"
	}
	switch goruntime.GOOS {
	case "darwin":
		return "/usr/lib/libSystem.B.dylib"
	case "freebsd":
		return "libc.so.7"
	case "netbsd":
		return "libc.so.12"
	default:
		return "libc.so.6"
	}
}

func ffiAndroidRuntime() bool {
	if goruntime.GOOS == "android" {
		return true
	}
	if goruntime.GOOS != "linux" {
		return false
	}
	if os.Getenv("ANDROID_ROOT") != "" && os.Getenv("ANDROID_DATA") != "" {
		return true
	}
	for _, path := range []string{
		"/system/bin/linker64",
		"/system/bin/linker",
		"/apex/com.android.runtime/bin/linker64",
		"/apex/com.android.runtime/bin/linker",
	} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func ffiLibraryCandidates(path string) []string {
	path = strings.TrimSpace(path)
	candidates := []string{path}
	if !ffiAndroidRuntime() {
		return candidates
	}
	switch strings.TrimSpace(path) {
	case "libc.so.6":
		candidates = append(candidates, "libc.so")
	case "libm.so.6":
		candidates = append(candidates, "libm.so")
	case "libdl.so.2":
		candidates = append(candidates, "libdl.so")
	case "libpthread.so.0":
		candidates = append(candidates, "libpthread.so")
	}
	return candidates
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
