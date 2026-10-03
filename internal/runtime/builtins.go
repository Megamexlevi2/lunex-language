package runtime

import (
	"fmt"
	"lunex/internal/meta"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (interp *Interpreter) registerBuiltins() {
	g := interp.globals

	g.Define("undefined", Undefined, false)
	g.Define("null", Null, false)
	g.Define("true", True, false)
	g.Define("false", False, false)
	g.Define("NaN", NumberVal(math.NaN()), false)
	g.Define("Infinity", NumberVal(math.Inf(1)), false)

	g.Define("lunex", ObjectVal(map[string]*Value{
		"getversion": FuncVal(&Function{Name: "getversion", Native: func(args []*Value, this *Value) (*Value, error) {
			return StringVal(meta.Version()), nil
		}}),
	}), false)

	g.Define("parseInt", FuncVal(&Function{Name: "parseInt", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return NumberVal(math.NaN()), nil
		}
		base := 10
		if len(args) > 1 {
			base = int(args[1].ToNumber())
		}
		s := strings.TrimSpace(args[0].ToString())
		n, err := strconv.ParseInt(s, base, 64)
		if err != nil {
			return NumberVal(math.NaN()), nil
		}
		return NumberVal(float64(n)), nil
	}}), false)

	g.Define("parseFloat", FuncVal(&Function{Name: "parseFloat", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return NumberVal(math.NaN()), nil
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(args[0].ToString()), 64)
		if err != nil {
			return NumberVal(math.NaN()), nil
		}
		return NumberVal(f), nil
	}}), false)

	g.Define("isNaN", FuncVal(&Function{Name: "isNaN", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return True, nil
		}
		return BoolVal(math.IsNaN(args[0].ToNumber())), nil
	}}), false)

	g.Define("isFinite", FuncVal(&Function{Name: "isFinite", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return False, nil
		}
		n := args[0].ToNumber()
		return BoolVal(!math.IsNaN(n) && !math.IsInf(n, 0)), nil
	}}), false)

	g.Define("String", FuncVal(&Function{Name: "String", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return StringVal(""), nil
		}
		return StringVal(args[0].ToString()), nil
	}}), false)

	g.Define("str", FuncVal(&Function{Name: "str", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return StringVal(""), nil
		}
		return StringVal(args[0].ToString()), nil
	}}), false)

	g.Define("Number", FuncVal(&Function{Name: "Number", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return NumberVal(0), nil
		}
		return NumberVal(args[0].ToNumber()), nil
	}}), false)

	g.Define("num", FuncVal(&Function{Name: "num", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return NumberVal(0), nil
		}
		return NumberVal(args[0].ToNumber()), nil
	}}), false)

	g.Define("Boolean", FuncVal(&Function{Name: "Boolean", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return False, nil
		}
		return BoolVal(args[0].IsTruthy()), nil
	}}), false)

	g.Define("Array", ObjectVal(map[string]*Value{
		"isArray": FuncVal(&Function{Name: "isArray", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return False, nil
			}
			return BoolVal(args[0].Tag == TypeArray), nil
		}}),
		"from": FuncVal(&Function{Name: "from", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return ArrayVal(nil), nil
			}
			src := args[0]
			if src.Tag == TypeArray {
				result := make([]*Value, len(src.ArrVal))
				copy(result, src.ArrVal)
				return ArrayVal(result), nil
			}
			if src.Tag == TypeString {
				runes := []rune(src.StrVal)
				result := make([]*Value, len(runes))
				for i, r := range runes {
					result[i] = StringVal(string(r))
				}
				return ArrayVal(result), nil
			}
			return ArrayVal(nil), nil
		}}),
		"of": FuncVal(&Function{Name: "of", Native: func(args []*Value, this *Value) (*Value, error) {
			return ArrayVal(args), nil
		}}),
	}), false)

	g.Define("Object", ObjectVal(map[string]*Value{
		"keys": FuncVal(&Function{Name: "keys", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return ArrayVal(nil), nil
			}
			obj := args[0]
			var keys []*Value
			if obj.Tag == TypeObject {
				for k := range obj.ObjVal {
					keys = append(keys, StringVal(k))
				}
			} else if obj.Tag == TypeInstance {
				for k := range obj.InstVal.Fields {
					keys = append(keys, StringVal(k))
				}
			}
			return ArrayVal(keys), nil
		}}),
		"values": FuncVal(&Function{Name: "values", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return ArrayVal(nil), nil
			}
			obj := args[0]
			var vals []*Value
			if obj.Tag == TypeObject {
				for _, v := range obj.ObjVal {
					vals = append(vals, v)
				}
			}
			return ArrayVal(vals), nil
		}}),
		"entries": FuncVal(&Function{Name: "entries", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return ArrayVal(nil), nil
			}
			obj := args[0]
			var entries []*Value
			if obj.Tag == TypeObject {
				for k, v := range obj.ObjVal {
					entries = append(entries, ArrayVal([]*Value{StringVal(k), v}))
				}
			}
			return ArrayVal(entries), nil
		}}),
		"assign": FuncVal(&Function{Name: "assign", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return ObjectVal(nil), nil
			}
			target := args[0]
			if target.Tag != TypeObject {
				return target, nil
			}
			for _, src := range args[1:] {
				if src.Tag == TypeObject {
					for k, v := range src.ObjVal {
						target.ObjVal[k] = v
					}
				}
			}
			return target, nil
		}}),
		"freeze": FuncVal(&Function{Name: "freeze", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return Undefined, nil
			}
			return args[0], nil
		}}),
		"create": FuncVal(&Function{Name: "create", Native: func(args []*Value, this *Value) (*Value, error) {
			obj := ObjectVal(nil)
			if len(args) > 0 && args[0].Tag == TypeObject {
				for k, v := range args[0].ObjVal {
					obj.ObjVal[k] = v
				}
			}
			return obj, nil
		}}),
		"fromEntries": FuncVal(&Function{Name: "fromEntries", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return ObjectVal(nil), nil
			}
			obj := make(map[string]*Value)
			if args[0].Tag == TypeArray {
				for _, entry := range args[0].ArrVal {
					if entry != nil && entry.Tag == TypeArray && len(entry.ArrVal) >= 2 {
						key := entry.ArrVal[0].ToString()
						obj[key] = entry.ArrVal[1]
					}
				}
			}
			return ObjectVal(obj), nil
		}}),
	}), false)

	g.Define("Math", ObjectVal(map[string]*Value{
		"PI":     NumberVal(math.Pi),
		"E":      NumberVal(math.E),
		"LN2":    NumberVal(math.Ln2),
		"LN10":   NumberVal(math.Log(10)),
		"LOG2E":  NumberVal(math.Log2E),
		"LOG10E": NumberVal(math.Log10E),
		"SQRT2":  NumberVal(math.Sqrt2),
		"abs":    mathFn1("abs", math.Abs),
		"ceil":   mathFn1("ceil", math.Ceil),
		"floor":  mathFn1("floor", math.Floor),
		"round":  mathFn1("round", math.Round),
		"sqrt":   mathFn1("sqrt", math.Sqrt),
		"cbrt":   mathFn1("cbrt", math.Cbrt),
		"sin":    mathFn1("sin", math.Sin),
		"cos":    mathFn1("cos", math.Cos),
		"tan":    mathFn1("tan", math.Tan),
		"asin":   mathFn1("asin", math.Asin),
		"acos":   mathFn1("acos", math.Acos),
		"atan":   mathFn1("atan", math.Atan),
		"log":    mathFn1("log", math.Log),
		"log2":   mathFn1("log2", math.Log2),
		"log10":  mathFn1("log10", math.Log10),
		"exp":    mathFn1("exp", math.Exp),
		"sign":   mathFn1("sign", mathSign),
		"trunc":  mathFn1("trunc", math.Trunc),
		"hypot":  mathFn1("hypot", math.Abs),
		"max": FuncVal(&Function{Name: "max", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return NumberVal(math.Inf(-1)), nil
			}
			max := args[0].ToNumber()
			for _, a := range args[1:] {
				if n := a.ToNumber(); n > max {
					max = n
				}
			}
			return NumberVal(max), nil
		}}),
		"min": FuncVal(&Function{Name: "min", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return NumberVal(math.Inf(1)), nil
			}
			min := args[0].ToNumber()
			for _, a := range args[1:] {
				if n := a.ToNumber(); n < min {
					min = n
				}
			}
			return NumberVal(min), nil
		}}),
		"pow": FuncVal(&Function{Name: "pow", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) < 2 {
				return NumberVal(math.NaN()), nil
			}
			return NumberVal(math.Pow(args[0].ToNumber(), args[1].ToNumber())), nil
		}}),
		"atan2": FuncVal(&Function{Name: "atan2", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) < 2 {
				return NumberVal(math.NaN()), nil
			}
			return NumberVal(math.Atan2(args[0].ToNumber(), args[1].ToNumber())), nil
		}}),
		"random": FuncVal(&Function{Name: "random", Native: func(args []*Value, this *Value) (*Value, error) {
			return NumberVal(pseudoRandom()), nil
		}}),
		"imul": FuncVal(&Function{Name: "imul", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) < 2 {
				return NumberVal(0), nil
			}
			return NumberVal(float64(int32(args[0].ToNumber()) * int32(args[1].ToNumber()))), nil
		}}),
	}), false)

	g.Define("JSON", ObjectVal(map[string]*Value{
		"stringify": FuncVal(&Function{Name: "stringify", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return Undefined, nil
			}
			indent := ""
			if len(args) > 2 {
				if args[2].Tag == TypeNumber {
					indent = strings.Repeat(" ", int(args[2].ToNumber()))
				} else if args[2].Tag == TypeString {
					indent = args[2].StrVal
				}
			}
			result := jsonStringify(args[0], indent, 0)
			return StringVal(result), nil
		}}),
		"parse": FuncVal(&Function{Name: "parse", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return Null, nil
			}
			val, err := jsonParse(args[0].ToString())
			if err != nil {
				return nil, &throwError{val: ObjectVal(map[string]*Value{"message": StringVal(err.Error())})}
			}
			return val, nil
		}}),
	}), false)

	g.Define("Promise", ObjectVal(map[string]*Value{
		"resolve": FuncVal(&Function{Name: "resolve", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return Undefined, nil
			}
			return args[0], nil
		}}),
		"reject": FuncVal(&Function{Name: "reject", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return Null, nil
			}
			return nil, &throwError{val: args[0]}
		}}),
		"all": FuncVal(&Function{Name: "all", Native: func(args []*Value, this *Value) (*Value, error) {
			if len(args) == 0 {
				return ArrayVal(nil), nil
			}
			arr := args[0]
			if arr.Tag != TypeArray {
				return ArrayVal(nil), nil
			}
			return arr, nil
		}}),
	}), false)

	g.Define("Error", FuncVal(&Function{Name: "Error", Native: func(args []*Value, this *Value) (*Value, error) {
		msg := ""
		if len(args) > 0 {
			msg = args[0].ToString()
		}
		return ObjectVal(map[string]*Value{
			"message": StringVal(msg),
			"name":    StringVal("Error"),
			"stack":   StringVal("Error: " + msg),
		}), nil
	}}), false)

	g.Define("TypeError", FuncVal(&Function{Name: "TypeError", Native: func(args []*Value, this *Value) (*Value, error) {
		msg := ""
		if len(args) > 0 {
			msg = args[0].ToString()
		}
		return ObjectVal(map[string]*Value{
			"message": StringVal(msg),
			"name":    StringVal("TypeError"),
		}), nil
	}}), false)

	g.Define("RangeError", FuncVal(&Function{Name: "RangeError", Native: func(args []*Value, this *Value) (*Value, error) {
		msg := ""
		if len(args) > 0 {
			msg = args[0].ToString()
		}
		return ObjectVal(map[string]*Value{
			"message": StringVal(msg),
			"name":    StringVal("RangeError"),
		}), nil
	}}), false)

	g.Define("Map", FuncVal(&Function{Name: "Map", Native: func(args []*Value, this *Value) (*Value, error) {
		m := &ntlMap{data: make(map[string]*Value), keyOrder: nil}
		if this != nil && this.Tag == TypeInstance {
			this.InstVal.Fields["__map__"] = ObjectVal(nil)
			this.InstVal.Fields["__map__"].ObjVal["_m"] = FuncVal(&Function{Native: func(a []*Value, t *Value) (*Value, error) {
				return ObjectVal(m.data), nil
			}})
		}
		return mapObject(m), nil
	}}), false)

	g.Define("Set", FuncVal(&Function{Name: "Set", Native: func(args []*Value, this *Value) (*Value, error) {
		s := &ntlSet{items: make(map[string]*Value)}
		if len(args) > 0 && args[0].Tag == TypeArray {
			for _, item := range args[0].ArrVal {
				if item != nil {
					s.items[item.ToString()] = item
				}
			}
		}
		return setObject(s), nil
	}}), false)

	g.Define("setTimeout", FuncVal(&Function{Name: "setTimeout", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) < 2 {
			return NumberVal(0), nil
		}
		fn := args[0]
		ms := int(args[1].ToNumber())
		if ms < 0 {
			ms = 0
		}
		go func() {
			time.Sleep(time.Duration(ms) * time.Millisecond)
			interp.callFunctionValue(fn, nil, nil)
		}()
		return NumberVal(0), nil
	}}), false)

	var intervalMu sync.Mutex
	intervalMap := make(map[float64]*time.Ticker)
	var intervalIDCounter float64

	g.Define("setInterval", FuncVal(&Function{Name: "setInterval", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) < 2 {
			return NumberVal(0), nil
		}
		fn := args[0]
		ms := int(args[1].ToNumber())
		if ms < 1 {
			ms = 1
		}
		ticker := time.NewTicker(time.Duration(ms) * time.Millisecond)
		intervalMu.Lock()
		intervalIDCounter++
		id := intervalIDCounter
		intervalMap[id] = ticker
		intervalMu.Unlock()
		go func() {
			for range ticker.C {
				interp.callFunctionValue(fn, nil, nil)
			}
		}()
		return NumberVal(id), nil
	}}), false)

	g.Define("clearTimeout", FuncVal(&Function{Name: "clearTimeout", Native: func(args []*Value, this *Value) (*Value, error) {
		return Undefined, nil
	}}), false)

	g.Define("clearInterval", FuncVal(&Function{Name: "clearInterval", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return Undefined, nil
		}
		id := args[0].ToNumber()
		intervalMu.Lock()
		if ticker, ok := intervalMap[id]; ok {
			ticker.Stop()
			delete(intervalMap, id)
		}
		intervalMu.Unlock()
		return Undefined, nil
	}}), false)

	g.Define("performance", ObjectVal(map[string]*Value{
		"now": FuncVal(&Function{Name: "now", Native: func(args []*Value, this *Value) (*Value, error) {
			return NumberVal(float64(time.Now().UnixNano()) / 1e6), nil
		}}),
	}), false)

	g.Define("process", ObjectVal(map[string]*Value{
		"env":  ObjectVal(nil),
		"argv": ArrayVal(nil),
		"exit": FuncVal(&Function{Name: "exit", Native: func(args []*Value, this *Value) (*Value, error) {
			code := 0
			if len(args) > 0 {
				code = int(args[0].ToNumber())
			}
			os.Exit(code)
			return Undefined, nil
		}}),
		"stdout": ObjectVal(map[string]*Value{
			"write": FuncVal(&Function{Name: "write", Native: func(args []*Value, this *Value) (*Value, error) {
				if len(args) > 0 {
					fmt.Print(args[0].ToString())
				}
				return Undefined, nil
			}}),
		}),
		"stderr": ObjectVal(map[string]*Value{
			"write": FuncVal(&Function{Name: "write", Native: func(args []*Value, this *Value) (*Value, error) {
				if len(args) > 0 {
					fmt.Fprint(os.Stderr, args[0].ToString())
				}
				return Undefined, nil
			}}),
		}),
	}), false)

	g.Define("encodeURIComponent", FuncVal(&Function{Name: "encodeURIComponent", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return StringVal("undefined"), nil
		}
		return StringVal(encodeURIComponent(args[0].ToString())), nil
	}}), false)

	g.Define("decodeURIComponent", FuncVal(&Function{Name: "decodeURIComponent", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return StringVal(""), nil
		}
		result, err := decodeURIComponent(args[0].ToString())
		if err != nil {
			return StringVal(args[0].ToString()), nil
		}
		return StringVal(result), nil
	}}), false)

	g.Define("encodeURI", FuncVal(&Function{Name: "encodeURI", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return StringVal("undefined"), nil
		}
		return StringVal(encodeURI(args[0].ToString())), nil
	}}), false)

	g.Define("decodeURI", FuncVal(&Function{Name: "decodeURI", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return StringVal(""), nil
		}
		result, _ := decodeURIComponent(args[0].ToString())
		return StringVal(result), nil
	}}), false)

	g.Define("btoa", FuncVal(&Function{Name: "btoa", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return StringVal(""), nil
		}
		encoded := base64Encode([]byte(args[0].ToString()))
		return StringVal(encoded), nil
	}}), false)

	g.Define("atob", FuncVal(&Function{Name: "atob", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return StringVal(""), nil
		}
		decoded, err := base64Decode(args[0].ToString())
		if err != nil {
			return StringVal(""), nil
		}
		return StringVal(string(decoded)), nil
	}}), false)
}

type returnError struct{ val *Value }
type throwError struct{ val *Value }
type breakError struct{}
type continueError struct{}
