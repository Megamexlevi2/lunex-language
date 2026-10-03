package std

import (
	"lunex/internal/runtime"
	"math"
	"math/rand"
)

func MathModule() *runtime.Value {
	return runtime.ObjectVal(map[string]*runtime.Value{
		"PI":       runtime.NumberVal(math.Pi),
		"E":        runtime.NumberVal(math.E),
		"LN2":      runtime.NumberVal(math.Ln2),
		"LN10":     runtime.NumberVal(math.Log(10)),
		"LOG2E":    runtime.NumberVal(math.Log2E),
		"LOG10E":   runtime.NumberVal(math.Log10E),
		"SQRT2":    runtime.NumberVal(math.Sqrt2),
		"SQRT1_2":  runtime.NumberVal(1.0 / math.Sqrt2),
		"PHI":      runtime.NumberVal(math.Phi),
		"Infinity": runtime.NumberVal(math.Inf(1)),
		"NaN":      runtime.NumberVal(math.NaN()),

		"sin": runtime.FuncVal(&runtime.Function{Name: "sin", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Sin(args[0].ToNumber())), nil
		}}),

		"cos": runtime.FuncVal(&runtime.Function{Name: "cos", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Cos(args[0].ToNumber())), nil
		}}),

		"tan": runtime.FuncVal(&runtime.Function{Name: "tan", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Tan(args[0].ToNumber())), nil
		}}),

		"asin": runtime.FuncVal(&runtime.Function{Name: "asin", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Asin(args[0].ToNumber())), nil
		}}),

		"acos": runtime.FuncVal(&runtime.Function{Name: "acos", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Acos(args[0].ToNumber())), nil
		}}),

		"atan": runtime.FuncVal(&runtime.Function{Name: "atan", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Atan(args[0].ToNumber())), nil
		}}),

		"atan2": runtime.FuncVal(&runtime.Function{Name: "atan2", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Atan2(args[0].ToNumber(), args[1].ToNumber())), nil
		}}),

		"sinh": runtime.FuncVal(&runtime.Function{Name: "sinh", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Sinh(args[0].ToNumber())), nil
		}}),

		"cosh": runtime.FuncVal(&runtime.Function{Name: "cosh", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Cosh(args[0].ToNumber())), nil
		}}),

		"tanh": runtime.FuncVal(&runtime.Function{Name: "tanh", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Tanh(args[0].ToNumber())), nil
		}}),

		"asinh": runtime.FuncVal(&runtime.Function{Name: "asinh", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Asinh(args[0].ToNumber())), nil
		}}),

		"acosh": runtime.FuncVal(&runtime.Function{Name: "acosh", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Acosh(args[0].ToNumber())), nil
		}}),

		"atanh": runtime.FuncVal(&runtime.Function{Name: "atanh", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Atanh(args[0].ToNumber())), nil
		}}),

		"log": runtime.FuncVal(&runtime.Function{Name: "log", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			if len(args) == 2 {
				base := args[1].ToNumber()
				return runtime.NumberVal(math.Log(args[0].ToNumber()) / math.Log(base)), nil
			}
			return runtime.NumberVal(math.Log(args[0].ToNumber())), nil
		}}),

		"log2": runtime.FuncVal(&runtime.Function{Name: "log2", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Log2(args[0].ToNumber())), nil
		}}),

		"log10": runtime.FuncVal(&runtime.Function{Name: "log10", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Log10(args[0].ToNumber())), nil
		}}),

		"exp": runtime.FuncVal(&runtime.Function{Name: "exp", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Exp(args[0].ToNumber())), nil
		}}),

		"exp2": runtime.FuncVal(&runtime.Function{Name: "exp2", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Exp2(args[0].ToNumber())), nil
		}}),

		"expm1": runtime.FuncVal(&runtime.Function{Name: "expm1", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Expm1(args[0].ToNumber())), nil
		}}),

		"log1p": runtime.FuncVal(&runtime.Function{Name: "log1p", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Log1p(args[0].ToNumber())), nil
		}}),

		"sqrt": runtime.FuncVal(&runtime.Function{Name: "sqrt", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Sqrt(args[0].ToNumber())), nil
		}}),

		"cbrt": runtime.FuncVal(&runtime.Function{Name: "cbrt", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Cbrt(args[0].ToNumber())), nil
		}}),

		"pow": runtime.FuncVal(&runtime.Function{Name: "pow", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Pow(args[0].ToNumber(), args[1].ToNumber())), nil
		}}),

		"hypot": runtime.FuncVal(&runtime.Function{Name: "hypot", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(0), nil
			}
			if len(args) == 2 {
				return runtime.NumberVal(math.Hypot(args[0].ToNumber(), args[1].ToNumber())), nil
			}
			sum := 0.0
			for _, a := range args {
				v := a.ToNumber()
				sum += v * v
			}
			return runtime.NumberVal(math.Sqrt(sum)), nil
		}}),

		"abs": runtime.FuncVal(&runtime.Function{Name: "abs", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Abs(args[0].ToNumber())), nil
		}}),

		"ceil": runtime.FuncVal(&runtime.Function{Name: "ceil", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Ceil(args[0].ToNumber())), nil
		}}),

		"floor": runtime.FuncVal(&runtime.Function{Name: "floor", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Floor(args[0].ToNumber())), nil
		}}),

		"round": runtime.FuncVal(&runtime.Function{Name: "round", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			if len(args) > 1 {
				decimals := int(args[1].ToNumber())
				factor := math.Pow(10, float64(decimals))
				return runtime.NumberVal(math.Round(args[0].ToNumber()*factor) / factor), nil
			}
			return runtime.NumberVal(math.Round(args[0].ToNumber())), nil
		}}),

		"trunc": runtime.FuncVal(&runtime.Function{Name: "trunc", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			return runtime.NumberVal(math.Trunc(args[0].ToNumber())), nil
		}}),

		"sign": runtime.FuncVal(&runtime.Function{Name: "sign", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			v := args[0].ToNumber()
			if v > 0 {
				return runtime.NumberVal(1), nil
			}
			if v < 0 {
				return runtime.NumberVal(-1), nil
			}
			return runtime.NumberVal(0), nil
		}}),

		"max": runtime.FuncVal(&runtime.Function{Name: "max", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.Inf(-1)), nil
			}
			if len(args) == 1 && args[0].Tag == runtime.TypeArray {
				m := math.Inf(-1)
				for _, v := range args[0].ArrVal {
					if v != nil && v.ToNumber() > m {
						m = v.ToNumber()
					}
				}
				return runtime.NumberVal(m), nil
			}
			m := args[0].ToNumber()
			for _, a := range args[1:] {
				if a.ToNumber() > m {
					m = a.ToNumber()
				}
			}
			return runtime.NumberVal(m), nil
		}}),

		"min": runtime.FuncVal(&runtime.Function{Name: "min", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.Inf(1)), nil
			}
			if len(args) == 1 && args[0].Tag == runtime.TypeArray {
				m := math.Inf(1)
				for _, v := range args[0].ArrVal {
					if v != nil && v.ToNumber() < m {
						m = v.ToNumber()
					}
				}
				return runtime.NumberVal(m), nil
			}
			m := args[0].ToNumber()
			for _, a := range args[1:] {
				if a.ToNumber() < m {
					m = a.ToNumber()
				}
			}
			return runtime.NumberVal(m), nil
		}}),

		"clamp": runtime.FuncVal(&runtime.Function{Name: "clamp", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 3 {
				return runtime.NumberVal(0), nil
			}
			v, lo, hi := args[0].ToNumber(), args[1].ToNumber(), args[2].ToNumber()
			return runtime.NumberVal(math.Min(math.Max(v, lo), hi)), nil
		}}),

		"lerp": runtime.FuncVal(&runtime.Function{Name: "lerp", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 3 {
				return runtime.NumberVal(0), nil
			}
			a, b, t := args[0].ToNumber(), args[1].ToNumber(), args[2].ToNumber()
			return runtime.NumberVal(a + (b-a)*t), nil
		}}),

		"random": runtime.FuncVal(&runtime.Function{Name: "random", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(rand.Float64()), nil
			}
			if len(args) == 1 {
				return runtime.NumberVal(rand.Float64() * args[0].ToNumber()), nil
			}
			lo, hi := args[0].ToNumber(), args[1].ToNumber()
			return runtime.NumberVal(lo + rand.Float64()*(hi-lo)), nil
		}}),

		"randomInt": runtime.FuncVal(&runtime.Function{Name: "randomInt", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(float64(rand.Int63())), nil
			}
			if len(args) == 1 {
				n := int64(args[0].ToNumber())
				if n <= 0 {
					return runtime.NumberVal(0), nil
				}
				return runtime.NumberVal(float64(rand.Int63n(n))), nil
			}
			lo, hi := int64(args[0].ToNumber()), int64(args[1].ToNumber())
			if hi <= lo {
				return runtime.NumberVal(float64(lo)), nil
			}
			return runtime.NumberVal(float64(lo + rand.Int63n(hi-lo))), nil
		}}),

		"seed": runtime.FuncVal(&runtime.Function{Name: "seed", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) > 0 {
				rand.Seed(int64(args[0].ToNumber()))
			}
			return runtime.Undefined, nil
		}}),

		"gcd": runtime.FuncVal(&runtime.Function{Name: "gcd", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.NumberVal(0), nil
			}
			a, b := int64(math.Abs(args[0].ToNumber())), int64(math.Abs(args[1].ToNumber()))
			for b != 0 {
				a, b = b, a%b
			}
			return runtime.NumberVal(float64(a)), nil
		}}),

		"lcm": runtime.FuncVal(&runtime.Function{Name: "lcm", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.NumberVal(0), nil
			}
			a, b := int64(math.Abs(args[0].ToNumber())), int64(math.Abs(args[1].ToNumber()))
			if a == 0 || b == 0 {
				return runtime.NumberVal(0), nil
			}
			g := a
			bb := b
			for bb != 0 {
				g, bb = bb, g%bb
			}
			return runtime.NumberVal(float64(a / g * b)), nil
		}}),

		"isPrime": runtime.FuncVal(&runtime.Function{Name: "isPrime", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			n := int64(args[0].ToNumber())
			if n < 2 {
				return runtime.False, nil
			}
			if n == 2 {
				return runtime.True, nil
			}
			if n%2 == 0 {
				return runtime.False, nil
			}
			for i := int64(3); i*i <= n; i += 2 {
				if n%i == 0 {
					return runtime.False, nil
				}
			}
			return runtime.True, nil
		}}),

		"factorial": runtime.FuncVal(&runtime.Function{Name: "factorial", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(1), nil
			}
			n := int64(args[0].ToNumber())
			if n < 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			result := int64(1)
			for i := int64(2); i <= n; i++ {
				result *= i
			}
			return runtime.NumberVal(float64(result)), nil
		}}),

		"combinations": runtime.FuncVal(&runtime.Function{Name: "combinations", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.NumberVal(0), nil
			}
			n := int64(args[0].ToNumber())
			k := int64(args[1].ToNumber())
			if k > n {
				return runtime.NumberVal(0), nil
			}
			if k == 0 || k == n {
				return runtime.NumberVal(1), nil
			}
			if k > n-k {
				k = n - k
			}
			result := int64(1)
			for i := int64(0); i < k; i++ {
				result = result * (n - i) / (i + 1)
			}
			return runtime.NumberVal(float64(result)), nil
		}}),

		"permutations": runtime.FuncVal(&runtime.Function{Name: "permutations", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.NumberVal(0), nil
			}
			n := int64(args[0].ToNumber())
			k := int64(args[1].ToNumber())
			if k > n {
				return runtime.NumberVal(0), nil
			}
			result := int64(1)
			for i := n; i > n-k; i-- {
				result *= i
			}
			return runtime.NumberVal(float64(result)), nil
		}}),

		"degToRad": runtime.FuncVal(&runtime.Function{Name: "degToRad", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(0), nil
			}
			return runtime.NumberVal(args[0].ToNumber() * math.Pi / 180), nil
		}}),

		"radToDeg": runtime.FuncVal(&runtime.Function{Name: "radToDeg", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(0), nil
			}
			return runtime.NumberVal(args[0].ToNumber() * 180 / math.Pi), nil
		}}),

		"isNaN": runtime.FuncVal(&runtime.Function{Name: "isNaN", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.True, nil
			}
			return runtime.BoolVal(math.IsNaN(args[0].ToNumber())), nil
		}}),

		"isFinite": runtime.FuncVal(&runtime.Function{Name: "isFinite", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			return runtime.BoolVal(!math.IsInf(args[0].ToNumber(), 0) && !math.IsNaN(args[0].ToNumber())), nil
		}}),

		"isInfinite": runtime.FuncVal(&runtime.Function{Name: "isInfinite", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			return runtime.BoolVal(math.IsInf(args[0].ToNumber(), 0)), nil
		}}),

		"sum": runtime.FuncVal(&runtime.Function{Name: "sum", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(0), nil
			}
			if args[0].Tag == runtime.TypeArray {
				s := 0.0
				for _, v := range args[0].ArrVal {
					if v != nil {
						s += v.ToNumber()
					}
				}
				return runtime.NumberVal(s), nil
			}
			s := 0.0
			for _, a := range args {
				s += a.ToNumber()
			}
			return runtime.NumberVal(s), nil
		}}),

		"product": runtime.FuncVal(&runtime.Function{Name: "product", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(1), nil
			}
			if args[0].Tag == runtime.TypeArray {
				p := 1.0
				for _, v := range args[0].ArrVal {
					if v != nil {
						p *= v.ToNumber()
					}
				}
				return runtime.NumberVal(p), nil
			}
			p := 1.0
			for _, a := range args {
				p *= a.ToNumber()
			}
			return runtime.NumberVal(p), nil
		}}),

		"mean": runtime.FuncVal(&runtime.Function{Name: "mean", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			if args[0].Tag == runtime.TypeArray {
				if len(args[0].ArrVal) == 0 {
					return runtime.NumberVal(math.NaN()), nil
				}
				s := 0.0
				for _, v := range args[0].ArrVal {
					if v != nil {
						s += v.ToNumber()
					}
				}
				return runtime.NumberVal(s / float64(len(args[0].ArrVal))), nil
			}
			s := 0.0
			for _, a := range args {
				s += a.ToNumber()
			}
			return runtime.NumberVal(s / float64(len(args))), nil
		}}),

		"variance": runtime.FuncVal(&runtime.Function{Name: "variance", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 || args[0].Tag != runtime.TypeArray || len(args[0].ArrVal) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			arr := args[0].ArrVal
			n := float64(len(arr))
			mean := 0.0
			for _, v := range arr {
				if v != nil {
					mean += v.ToNumber()
				}
			}
			mean /= n
			variance := 0.0
			for _, v := range arr {
				if v != nil {
					d := v.ToNumber() - mean
					variance += d * d
				}
			}
			return runtime.NumberVal(variance / n), nil
		}}),

		"stdDev": runtime.FuncVal(&runtime.Function{Name: "stdDev", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 || args[0].Tag != runtime.TypeArray || len(args[0].ArrVal) == 0 {
				return runtime.NumberVal(math.NaN()), nil
			}
			arr := args[0].ArrVal
			n := float64(len(arr))
			mean := 0.0
			for _, v := range arr {
				if v != nil {
					mean += v.ToNumber()
				}
			}
			mean /= n
			variance := 0.0
			for _, v := range arr {
				if v != nil {
					d := v.ToNumber() - mean
					variance += d * d
				}
			}
			return runtime.NumberVal(math.Sqrt(variance / n)), nil
		}}),

		"fib": runtime.FuncVal(&runtime.Function{Name: "fib", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(0), nil
			}
			n := int64(args[0].ToNumber())
			if n <= 0 {
				return runtime.NumberVal(0), nil
			}
			if n == 1 {
				return runtime.NumberVal(1), nil
			}
			a, b := int64(0), int64(1)
			for i := int64(2); i <= n; i++ {
				a, b = b, a+b
			}
			return runtime.NumberVal(float64(b)), nil
		}}),

		"primes": runtime.FuncVal(&runtime.Function{Name: "primes", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.ArrayVal(nil), nil
			}
			n := int(args[0].ToNumber())
			if n < 2 {
				return runtime.ArrayVal(nil), nil
			}
			sieve := make([]bool, n+1)
			for i := range sieve {
				sieve[i] = true
			}
			sieve[0], sieve[1] = false, false
			for i := 2; i*i <= n; i++ {
				if sieve[i] {
					for j := i * i; j <= n; j += i {
						sieve[j] = false
					}
				}
			}
			var out []*runtime.Value
			for i, ok := range sieve {
				if ok {
					out = append(out, runtime.NumberVal(float64(i)))
				}
			}
			return runtime.ArrayVal(out), nil
		}}),

		"toBinary": runtime.FuncVal(&runtime.Function{Name: "toBinary", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.StringVal("0"), nil
			}
			n := int64(args[0].ToNumber())
			if n == 0 {
				return runtime.StringVal("0"), nil
			}
			result := ""
			negative := n < 0
			if negative {
				n = -n
			}
			for n > 0 {
				result = string(rune('0'+n%2)) + result
				n >>= 1
			}
			if negative {
				result = "-" + result
			}
			return runtime.StringVal(result), nil
		}}),

		"toHex": runtime.FuncVal(&runtime.Function{Name: "toHex", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.StringVal("0"), nil
			}
			n := int64(args[0].ToNumber())
			prefix := ""
			if len(args) > 1 && args[1].Tag == runtime.TypeBool && args[1].BoolVal {
				prefix = "0x"
			}
			return runtime.StringVal(prefix + formatHex(n)), nil
		}}),

		"toOctal": runtime.FuncVal(&runtime.Function{Name: "toOctal", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.StringVal("0"), nil
			}
			n := int64(args[0].ToNumber())
			return runtime.StringVal(formatOctal(n)), nil
		}}),
	})
}

func formatHex(n int64) string {
	if n == 0 {
		return "0"
	}
	const hexDigits = "0123456789abcdef"
	result := ""
	negative := n < 0
	if negative {
		n = -n
	}
	for n > 0 {
		result = string(hexDigits[n&0xf]) + result
		n >>= 4
	}
	if negative {
		result = "-" + result
	}
	return result
}

func formatOctal(n int64) string {
	if n == 0 {
		return "0"
	}
	result := ""
	negative := n < 0
	if negative {
		n = -n
	}
	for n > 0 {
		result = string(rune('0'+n%8)) + result
		n >>= 3
	}
	if negative {
		result = "-" + result
	}
	return result
}
