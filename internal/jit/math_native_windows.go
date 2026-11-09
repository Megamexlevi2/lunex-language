//go:build windows

package jit

import (
	"sync"

	"golang.org/x/sys/windows"
)

var (
	mathSymbolsOnce sync.Once
	mathSymbols     map[int]uintptr
)

func nativeMathSymbol(id int) uintptr {
	mathSymbolsOnce.Do(loadNativeMathSymbols)
	return mathSymbols[id]
}

func loadNativeMathSymbols() {
	mathSymbols = make(map[int]uintptr)

	var handle windows.Handle
	for _, name := range []string{"ucrtbase.dll", "msvcrt.dll"} {
		h, err := windows.LoadLibrary(name)
		if err == nil && h != 0 {
			handle = h
			break
		}
	}
	if handle == 0 {
		return
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
		address, err := windows.GetProcAddress(handle, symbol)
		if err == nil && address != 0 {
			mathSymbols[id] = address
		}
	}
}
