//go:build !(linux || android || darwin || freebsd || netbsd || windows)

package std

import "fmt"

func ffiOpenLibrary(string, bool, bool) (uintptr, error) {
	return 0, fmt.Errorf("ffi.load does not exist")
}

func ffiLookupSymbol(uintptr, string) (uintptr, error) {
	return 0, fmt.Errorf("ffi.symbol does not exist")
}

func ffiCloseLibrary(uintptr) error {
	return fmt.Errorf("ffi.close does not exist")
}

func ffiMalloc(uintptr) (uintptr, error) {
	return 0, fmt.Errorf("ffi.alloc does not exist")
}

func ffiCalloc(uintptr, uintptr) (uintptr, error) {
	return 0, fmt.Errorf("ffi.calloc does not exist")
}

func ffiRealloc(uintptr, uintptr) (uintptr, error) {
	return 0, fmt.Errorf("ffi.realloc does not exist")
}

func ffiFree(uintptr) error {
	return fmt.Errorf("ffi.free does not exist")
}

func ffiCopy(uintptr, uintptr, uintptr) error {
	return fmt.Errorf("ffi.copy does not exist")
}

func ffiFill(uintptr, uint8, uintptr) error {
	return fmt.Errorf("ffi.fill does not exist")
}

func ffiZero(uintptr, uintptr) error {
	return fmt.Errorf("ffi.zero does not exist")
}

func ffiRegisterFunc(any, uintptr) error {
	return fmt.Errorf("ffi.bind does not exist")
}

func ffiNewCallback(any) (uintptr, error) {
	return 0, fmt.Errorf("ffi.callback does not exist")
}
