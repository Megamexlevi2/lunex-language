package jit

import "math"

const (
	MathAbs = iota + 1
	MathCeil
	MathFloor
	MathRound
	MathSqrt
	MathCbrt
	MathSin
	MathCos
	MathTan
	MathAsin
	MathAcos
	MathAtan
	MathLog
	MathLog2
	MathLog10
	MathExp
	MathTrunc
	MathPow
	MathSign
	MathMax
	MathMin
	MathAtan2
	MathHypot
)

func mathFunction(name string) (int, bool) {
	switch name {
	case "abs":
		return MathAbs, true
	case "hypot":
		return MathHypot, true
	case "pow":
		return MathPow, true
	case "atan2":
		return MathAtan2, true
	case "ceil":
		return MathCeil, true
	case "floor":
		return MathFloor, true
	case "round":
		return MathRound, true
	case "sqrt":
		return MathSqrt, true
	case "cbrt":
		return MathCbrt, true
	case "sin":
		return MathSin, true
	case "cos":
		return MathCos, true
	case "tan":
		return MathTan, true
	case "asin":
		return MathAsin, true
	case "acos":
		return MathAcos, true
	case "atan":
		return MathAtan, true
	case "log":
		return MathLog, true
	case "log2":
		return MathLog2, true
	case "log10":
		return MathLog10, true
	case "exp":
		return MathExp, true
	case "trunc":
		return MathTrunc, true
	case "sign":
		return MathSign, true
	case "max":
		return MathMax, true
	case "min":
		return MathMin, true
	default:
		return 0, false
	}
}

func mathFunctionArity(id int) int {
	switch id {
	case MathPow, MathAtan2, MathMax, MathMin, MathHypot:
		return 2
	case MathAbs, MathCeil, MathFloor, MathRound, MathSqrt, MathCbrt, MathSin, MathCos, MathTan, MathAsin, MathAcos, MathAtan, MathLog, MathLog2, MathLog10, MathExp, MathTrunc, MathSign:
		return 1
	default:
		return 0
	}
}

func nativeMathConstant(name string) (float64, bool) {
	switch name {
	case "PI":
		return math.Pi, true
	case "E":
		return math.E, true
	case "LN2":
		return math.Ln2, true
	case "LN10":
		return math.Log(10), true
	case "LOG2E":
		return math.Log2E, true
	case "LOG10E":
		return math.Log10E, true
	case "SQRT2":
		return math.Sqrt2, true
	default:
		return 0, false
	}
}

func nativeMathValue(id int, a, b float64) float64 {
	switch id {
	case MathAbs:
		return math.Abs(a)
	case MathCeil:
		return math.Ceil(a)
	case MathFloor:
		return math.Floor(a)
	case MathRound:
		return math.Round(a)
	case MathSqrt:
		return math.Sqrt(a)
	case MathCbrt:
		return math.Cbrt(a)
	case MathSin:
		return math.Sin(a)
	case MathCos:
		return math.Cos(a)
	case MathTan:
		return math.Tan(a)
	case MathAsin:
		return math.Asin(a)
	case MathAcos:
		return math.Acos(a)
	case MathAtan:
		return math.Atan(a)
	case MathLog:
		return math.Log(a)
	case MathLog2:
		return math.Log2(a)
	case MathLog10:
		return math.Log10(a)
	case MathExp:
		return math.Exp(a)
	case MathTrunc:
		return math.Trunc(a)
	case MathPow:
		return math.Pow(a, b)
	case MathAtan2:
		return math.Atan2(a, b)
	case MathHypot:
		return math.Hypot(a, b)
	case MathSign:
		if a > 0 {
			return 1
		}
		if a < 0 {
			return -1
		}
		return 0
	case MathMax:
		if b > a {
			return b
		}
		return a
	case MathMin:
		if b < a {
			return b
		}
		return a
	default:
		return math.NaN()
	}
}

func nativeMathAddress(id int) uintptr {
	return nativeMathSymbol(id)
}
