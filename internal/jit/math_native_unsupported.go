//go:build !linux && !android && !darwin

package jit

func nativeMathSymbol(int) uintptr {
	return 0
}
