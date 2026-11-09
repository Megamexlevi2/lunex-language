//go:build linux || android || darwin || freebsd

package jit

import (
	goruntime "runtime"
	"sync"

	"github.com/ebitengine/purego"
)

var (
	mathSymbolsOnce sync.Once
	mathSymbols     map[int]uintptr
	mathSymbolLibs  []uintptr
)

func nativeMathSymbol(id int) uintptr {
	mathSymbolsOnce.Do(loadNativeMathSymbols)
	return mathSymbols[id]
}

func loadNativeMathSymbols() {
	mathSymbols = make(map[int]uintptr)
	var candidates []string
	switch goruntime.GOOS {
	case "darwin":
		candidates = []string{"/usr/lib/libSystem.B.dylib"}
	case "android":
		candidates = []string{"libm.so", "libc.so"}
	case "freebsd":
		candidates = []string{"libm.so.5", "libm.so", "libc.so.7"}
	default:
		candidates = []string{"libm.so.6", "libm.so", "libc.so.6"}
	}
	for _, candidate := range candidates {
		handle, err := purego.Dlopen(candidate, purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err == nil && handle != 0 {
			mathSymbolLibs = append(mathSymbolLibs, handle)
		}
	}
	for id, symbol := range map[int]string{
		MathAbs:   "fabs",
		MathCeil:  "ceil",
		MathFloor: "floor",
		MathRound: "round",
		MathSqrt:  "sqrt",
		MathCbrt:  "cbrt",
		MathSin:   "sin",
		MathCos:   "cos",
		MathTan:   "tan",
		MathAsin:  "asin",
		MathAcos:  "acos",
		MathAtan:  "atan",
		MathLog:   "log",
		MathLog2:  "log2",
		MathLog10: "log10",
		MathExp:   "exp",
		MathTrunc: "trunc",
		MathPow:   "pow",
		MathAtan2: "atan2",
		MathHypot: "hypot",
	} {
		for _, handle := range mathSymbolLibs {
			address, err := purego.Dlsym(handle, symbol)
			if err == nil && address != 0 {
				mathSymbols[id] = address
				break
			}
		}
	}
}
