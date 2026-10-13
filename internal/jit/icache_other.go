//go:build !arm64

package jit

func syncInstructionCache(code []byte) {}
