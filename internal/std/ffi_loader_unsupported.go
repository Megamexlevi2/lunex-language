//go:build !(linux || android || darwin || freebsd || netbsd || windows)

package std

import "fmt"

func ffiOpenLibrary(string, bool, bool) (uintptr, error) {
	return 0, fmt.Errorf("native shared-library loading is not supported on this platform")
}

func ffiLookupSymbol(uintptr, string) (uintptr, error) {
	return 0, fmt.Errorf("native symbol lookup is not supported on this platform")
}

func ffiCloseLibrary(uintptr) error {
	return fmt.Errorf("native shared-library closing is not supported on this platform")
}

func ffiMalloc(uintptr) (uintptr, error) {
	return 0, fmt.Errorf("native memory allocation is not supported on this platform")
}

func ffiCalloc(uintptr, uintptr) (uintptr, error) {
	return 0, fmt.Errorf("native memory allocation is not supported on this platform")
}

func ffiRealloc(uintptr, uintptr) (uintptr, error) {
	return 0, fmt.Errorf("native memory allocation is not supported on this platform")
}

func ffiFree(uintptr) error {
	return fmt.Errorf("native memory allocation is not supported on this platform")
}

func ffiCopy(uintptr, uintptr, uintptr) error {
	return fmt.Errorf("native memory access is not supported on this platform")
}

func ffiFill(uintptr, uint8, uintptr) error {
	return fmt.Errorf("native memory access is not supported on this platform")
}

func ffiZero(uintptr, uintptr) error {
	return fmt.Errorf("native memory access is not supported on this platform")
}

func ffiRegisterFunc(any, uintptr) error {
	return fmt.Errorf("native function registration is not supported on this platform")
}

func ffiNewCallback(any) (uintptr, error) {
	return 0, fmt.Errorf("native callbacks are not supported on this platform")
}
