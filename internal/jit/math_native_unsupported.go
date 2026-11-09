//go:build !linux && !android && !darwin && !freebsd && !windows

package jit

func nativeMathSymbol(int) uintptr {
	return 0
}
