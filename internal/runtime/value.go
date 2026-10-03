package runtime

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type ValueType int

const (
	TypeNull ValueType = iota
	TypeUndefined
	TypeBool
	TypeNumber
	TypeString
	TypeArray
	TypeObject
	TypeFunction
	TypeClass
	TypeInstance
	TypeRegex
	TypeChannel
	TypeError
)

type Value struct {
	Tag      ValueType
	BoolVal  bool
	NumVal   float64
	StrVal   string
	ArrVal   []*Value
	ObjVal   map[string]*Value
	FnVal    *Function
	ClsVal   *Class
	InstVal  *Instance
	RegexVal *regexp.Regexp
	ChanVal  *Channel
	ErrVal   error
}

var Null = &Value{Tag: TypeNull}
var Undefined = &Value{Tag: TypeUndefined}
var True = &Value{Tag: TypeBool, BoolVal: true}
var False = &Value{Tag: TypeBool, BoolVal: false}

const intPoolMin = -1024
const intPoolMax = 65535

var intPool [intPoolMax - intPoolMin + 1]*Value

func init() {
	for i := intPoolMin; i <= intPoolMax; i++ {
		intPool[i-intPoolMin] = &Value{Tag: TypeNumber, NumVal: float64(i)}
	}
	initNumberBuiltins()
}

func NumberVal(n float64) *Value {
	i := int(n)
	if float64(i) == n && i >= intPoolMin && i <= intPoolMax {
		return intPool[i-intPoolMin]
	}
	return &Value{Tag: TypeNumber, NumVal: n}
}

func NumberValInt(i int64) *Value {
	if i >= intPoolMin && i <= intPoolMax {
		return intPool[i-intPoolMin]
	}
	return &Value{Tag: TypeNumber, NumVal: float64(i)}
}
func StringVal(s string) *Value { return &Value{Tag: TypeString, StrVal: s} }
func BoolVal(b bool) *Value {
	if b {
		return True
	}
	return False
}
func ArrayVal(a []*Value) *Value { return &Value{Tag: TypeArray, ArrVal: a} }
func ObjectVal(m map[string]*Value) *Value {
	if m == nil {
		m = make(map[string]*Value)
	}
	return &Value{Tag: TypeObject, ObjVal: m}
}
func FuncVal(f *Function) *Value      { return &Value{Tag: TypeFunction, FnVal: f} }
func ClassVal(c *Class) *Value        { return &Value{Tag: TypeClass, ClsVal: c} }
func InstVal(i *Instance) *Value      { return &Value{Tag: TypeInstance, InstVal: i} }
func RegexV(re *regexp.Regexp) *Value { return &Value{Tag: TypeRegex, RegexVal: re} }
func ChanV(ch *Channel) *Value        { return &Value{Tag: TypeChannel, ChanVal: ch} }
func ErrorVal(err error) *Value       { return &Value{Tag: TypeError, ErrVal: err} }

func (v *Value) IsTruthy() bool {
	switch v.Tag {
	case TypeNull, TypeUndefined:
		return false
	case TypeBool:
		return v.BoolVal
	case TypeNumber:
		return v.NumVal != 0 && !math.IsNaN(v.NumVal)
	case TypeString:
		return v.StrVal != ""
	default:
		return true
	}
}

func (v *Value) IsNullish() bool {
	return v.Tag == TypeNull || v.Tag == TypeUndefined
}

func (v *Value) ToNumber() float64 {
	switch v.Tag {
	case TypeNumber:
		return v.NumVal
	case TypeBool:
		if v.BoolVal {
			return 1
		}
		return 0
	case TypeString:
		s := strings.TrimSpace(v.StrVal)
		if s == "" {
			return 0
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return math.NaN()
		}
		return f
	case TypeNull:
		return 0
	default:
		return math.NaN()
	}
}

func (v *Value) ToString() string {
	return v.toString(make(map[*Value]bool))
}

func (v *Value) toString(seen map[*Value]bool) string {
	if v == nil {
		return "undefined"
	}
	switch v.Tag {
	case TypeNull:
		return "null"
	case TypeUndefined:
		return "undefined"
	case TypeBool:
		if v.BoolVal {
			return "true"
		}
		return "false"
	case TypeNumber:
		if v.NumVal == math.Trunc(v.NumVal) && !math.IsInf(v.NumVal, 0) {
			return fmt.Sprintf("%.0f", v.NumVal)
		}
		return strconv.FormatFloat(v.NumVal, 'g', -1, 64)
	case TypeString:
		return v.StrVal
	case TypeArray:
		if seen[v] {
			return "[Circular]"
		}
		seen[v] = true
		parts := make([]string, len(v.ArrVal))
		for i, e := range v.ArrVal {
			if e == nil {
				parts[i] = ""
			} else {
				parts[i] = e.toString(seen)
			}
		}
		delete(seen, v)
		return strings.Join(parts, ",")
	case TypeObject:
		return "[object Object]"
	case TypeFunction:
		if v.FnVal != nil {
			return fmt.Sprintf("[Function: %s]", v.FnVal.Name)
		}
		return "[Function]"
	case TypeClass:
		if v.ClsVal != nil {
			return fmt.Sprintf("[class %s]", v.ClsVal.Name)
		}
		return "[class]"
	case TypeInstance:
		if v.InstVal != nil {
			return fmt.Sprintf("[%s]", v.InstVal.Class.Name)
		}
		return "[object]"
	case TypeRegex:
		if v.RegexVal != nil {
			return fmt.Sprintf("/%s/", v.RegexVal.String())
		}
		return "/(?:)/"
	case TypeError:
		if v.ErrVal != nil {
			return v.ErrVal.Error()
		}
		return "Error"
	default:
		return "undefined"
	}
}

func (v *Value) TypeName() string {
	switch v.Tag {
	case TypeNull:
		return "null"
	case TypeUndefined:
		return "undefined"
	case TypeBool:
		return "boolean"
	case TypeNumber:
		return "number"
	case TypeString:
		return "string"
	case TypeArray:
		return "array"
	case TypeObject:
		return "object"
	case TypeFunction:
		return "function"
	case TypeClass:
		return "class"
	case TypeInstance:
		if v.InstVal != nil {
			return v.InstVal.Class.Name
		}
		return "object"
	case TypeRegex:
		return "regexp"
	case TypeChannel:
		return "channel"
	case TypeError:
		return "error"
	default:
		return "unknown"
	}
}

func (v *Value) Get(key string) *Value {
	switch v.Tag {
	case TypeObject:
		if v.ObjVal != nil {
			if val, ok := v.ObjVal[key]; ok {
				return val
			}
		}
		switch key {
		case "keys":
			return FuncVal(&Function{Name: "keys", Native: func(args []*Value, this *Value) (*Value, error) {
				if this == nil || this.Tag != TypeObject {
					return ArrayVal(nil), nil
				}
				var ks []*Value
				for k := range this.ObjVal {
					ks = append(ks, StringVal(k))
				}
				return ArrayVal(ks), nil
			}})
		case "values":
			return FuncVal(&Function{Name: "values", Native: func(args []*Value, this *Value) (*Value, error) {
				if this == nil || this.Tag != TypeObject {
					return ArrayVal(nil), nil
				}
				var vs []*Value
				for _, val2 := range this.ObjVal {
					vs = append(vs, val2)
				}
				return ArrayVal(vs), nil
			}})
		case "entries":
			return FuncVal(&Function{Name: "entries", Native: func(args []*Value, this *Value) (*Value, error) {
				if this == nil || this.Tag != TypeObject {
					return ArrayVal(nil), nil
				}
				var es []*Value
				for k, val2 := range this.ObjVal {
					es = append(es, ArrayVal([]*Value{StringVal(k), val2}))
				}
				return ArrayVal(es), nil
			}})
		}
		return Undefined
	case TypeInstance:
		if v.InstVal != nil {
			return v.InstVal.Get(key)
		}
		return Undefined
	case TypeArray:
		return arrayBuiltin(v, key)
	case TypeString:
		return stringBuiltin(v, key)
	case TypeNumber:
		return numberBuiltin(v, key)
	case TypeClass:
		if v.ClsVal != nil {
			if m, ok := v.ClsVal.StaticMethods[key]; ok {
				return FuncVal(m)
			}
		}
		return Undefined
	default:
		return Undefined
	}
}

func (v *Value) Set(key string, val *Value) {
	switch v.Tag {
	case TypeObject:
		if v.ObjVal == nil {
			v.ObjVal = make(map[string]*Value)
		}
		v.ObjVal[key] = val
	case TypeInstance:
		if v.InstVal != nil {
			v.InstVal.Set(key, val)
		}
	case TypeArray:
		if key == "length" {
			n := int(val.ToNumber())
			if n >= 0 && n < len(v.ArrVal) {
				v.ArrVal = v.ArrVal[:n]
			} else {
				for len(v.ArrVal) < n {
					v.ArrVal = append(v.ArrVal, Undefined)
				}
			}
		}
	}
}

func (v *Value) GetIndex(idx int) *Value {
	switch v.Tag {
	case TypeArray:
		if idx >= 0 && idx < len(v.ArrVal) {
			if v.ArrVal[idx] == nil {
				return Undefined
			}
			return v.ArrVal[idx]
		}
		return Undefined
	case TypeString:
		runes := []rune(v.StrVal)
		if idx >= 0 && idx < len(runes) {
			return StringVal(string(runes[idx]))
		}
		return Undefined
	default:
		return Undefined
	}
}

func (v *Value) Equals(other *Value) bool {
	if v.Tag != other.Tag {
		if v.Tag == TypeNull && other.Tag == TypeUndefined {
			return true
		}
		if v.Tag == TypeUndefined && other.Tag == TypeNull {
			return true
		}
		if v.Tag == TypeNumber && other.Tag == TypeString {
			return v.NumVal == other.ToNumber()
		}
		if v.Tag == TypeString && other.Tag == TypeNumber {
			return v.ToNumber() == other.NumVal
		}
		return false
	}
	switch v.Tag {
	case TypeNull, TypeUndefined:
		return true
	case TypeBool:
		return v.BoolVal == other.BoolVal
	case TypeNumber:
		return v.NumVal == other.NumVal
	case TypeString:
		return v.StrVal == other.StrVal
	default:
		return v == other
	}
}

func (v *Value) StrictEquals(other *Value) bool {
	if v.Tag != other.Tag {
		return false
	}
	return v.Equals(other)
}

func (v *Value) Inspect() string {
	return v.inspect(make(map[*Value]bool))
}

func (v *Value) inspect(seen map[*Value]bool) string {
	if v == nil {
		return "undefined"
	}
	switch v.Tag {
	case TypeString:
		return fmt.Sprintf("%q", v.StrVal)
	case TypeArray:
		if seen[v] {
			return "[Circular]"
		}
		seen[v] = true
		parts := make([]string, len(v.ArrVal))
		for i, e := range v.ArrVal {
			if e == nil {
				parts[i] = "undefined"
			} else {
				parts[i] = e.inspect(seen)
			}
		}
		delete(seen, v)
		return "[ " + strings.Join(parts, ", ") + " ]"
	case TypeObject:
		if seen[v] {
			return "{ Circular }"
		}
		seen[v] = true
		if len(v.ObjVal) == 0 {
			delete(seen, v)
			return "{}"
		}
		var sb strings.Builder
		sb.WriteString("{ ")
		first := true
		for k, val := range v.ObjVal {
			if !first {
				sb.WriteString(", ")
			}
			sb.WriteString(k)
			sb.WriteString(": ")
			if val == nil {
				sb.WriteString("undefined")
			} else {
				sb.WriteString(val.inspect(seen))
			}
			first = false
		}
		sb.WriteString(" }")
		delete(seen, v)
		return sb.String()
	case TypeInstance:
		if seen[v] {
			return "[Circular]"
		}
		seen[v] = true
		if v.InstVal != nil {
			var sb strings.Builder
			sb.WriteString(v.InstVal.Class.Name)
			sb.WriteString(" { ")
			first := true
			for k, val := range v.InstVal.Fields {
				if !first {
					sb.WriteString(", ")
				}
				sb.WriteString(k)
				sb.WriteString(": ")
				if val == nil {
					sb.WriteString("undefined")
				} else {
					sb.WriteString(val.inspect(seen))
				}
				first = false
			}
			sb.WriteString(" }")
			delete(seen, v)
			return sb.String()
		}
		delete(seen, v)
		return "[object]"
	default:
		return v.toString(seen)
	}
}

type Function struct {
	Name         string
	Params       []FnParam
	Body         interface{}
	Env          *Environment
	Native       func(args []*Value, this *Value) (*Value, error)
	NativeTier      int
	CallCount    int64
	DefClass     *Class
	IsArrow      bool
	CapturedThis *Value

	SourceFile  string
	SourceLines []string
}

type FnParam struct {
	Name         string
	Default      interface{}
	Rest         bool
	Destructure  interface{}
	ResolvedSlot int
}

type Class struct {
	Name          string
	Super         *Class
	Methods       map[string]*Function
	StaticMethods map[string]*Function
	Env           *Environment
	InitNode      interface{}
}

type Instance struct {
	Class  *Class
	Fields map[string]*Value
	Proto  *Instance
}

func NewInstance(cls *Class) *Instance {
	return &Instance{
		Class:  cls,
		Fields: make(map[string]*Value),
	}
}

func (inst *Instance) Get(key string) *Value {
	if val, ok := inst.Fields[key]; ok {
		return val
	}
	if inst.Class != nil {
		if m, ok := inst.Class.Methods[key]; ok {
			return FuncVal(m)
		}
		if inst.Class.Super != nil {
			sup := &Instance{Class: inst.Class.Super, Fields: inst.Fields}
			return sup.Get(key)
		}
	}
	return Undefined
}

func (inst *Instance) Set(key string, val *Value) {
	inst.Fields[key] = val
}

type Channel struct {
	queue  chan *Value
	Closed bool
}

func NewChannel(bufSize int) *Channel {
	return &Channel{queue: make(chan *Value, bufSize)}
}

func (ch *Channel) Send(v *Value) {
	ch.queue <- v
}

func (ch *Channel) Receive() *Value {
	v, ok := <-ch.queue
	if !ok {
		return Null
	}
	return v
}

var (
	_arrPushVal     *Value
	_arrPopVal      *Value
	_arrShiftVal    *Value
	_arrUnshiftVal  *Value
	_arrSliceVal    *Value
	_arrSpliceVal   *Value
	_arrIndexOfVal  *Value
	_arrIncludesVal *Value
	_arrJoinVal     *Value
	_arrReverseVal  *Value
	_arrMapVal      *Value
	_arrFilterVal   *Value
	_arrReduceVal   *Value
	_arrForEachVal  *Value
	_arrFindVal     *Value
	_arrFindIdxVal  *Value
	_arrSomeVal     *Value
	_arrEveryVal    *Value
	_arrFlatVal     *Value
	_arrFlatMapVal  *Value
	_arrSortVal     *Value
	_arrConcatVal   *Value
	_arrToStrVal    *Value
	_arrFillVal     *Value
	_arrKeysVal     *Value
	_arrValuesVal   *Value
	_arrEntriesVal  *Value
)

func init() {
	_arrPushVal = FuncVal(&Function{Name: "push", Native: func(args []*Value, this *Value) (*Value, error) {
		this.ArrVal = append(this.ArrVal, args...)
		return NumberVal(float64(len(this.ArrVal))), nil
	}})
	_arrPopVal = FuncVal(&Function{Name: "pop", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(this.ArrVal) == 0 {
			return Undefined, nil
		}
		last := this.ArrVal[len(this.ArrVal)-1]
		this.ArrVal = this.ArrVal[:len(this.ArrVal)-1]
		return last, nil
	}})
	_arrShiftVal = FuncVal(&Function{Name: "shift", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(this.ArrVal) == 0 {
			return Undefined, nil
		}
		first := this.ArrVal[0]
		this.ArrVal = this.ArrVal[1:]
		return first, nil
	}})
	_arrUnshiftVal = FuncVal(&Function{Name: "unshift", Native: func(args []*Value, this *Value) (*Value, error) {
		this.ArrVal = append(args, this.ArrVal...)
		return NumberVal(float64(len(this.ArrVal))), nil
	}})
	_arrSliceVal = FuncVal(&Function{Name: "slice", Native: func(args []*Value, this *Value) (*Value, error) {
		n := len(this.ArrVal)
		start, end := 0, n
		if len(args) > 0 {
			start = int(args[0].ToNumber())
			if start < 0 {
				start = n + start
			}
			if start < 0 {
				start = 0
			}
		}
		if len(args) > 1 {
			end = int(args[1].ToNumber())
			if end < 0 {
				end = n + end
			}
		}
		if start > n {
			start = n
		}
		if end > n {
			end = n
		}
		if end < start {
			end = start
		}
		result := make([]*Value, end-start)
		copy(result, this.ArrVal[start:end])
		return ArrayVal(result), nil
	}})
	_arrSpliceVal = FuncVal(&Function{Name: "splice", Native: func(args []*Value, this *Value) (*Value, error) {
		n := len(this.ArrVal)
		if len(args) == 0 {
			return ArrayVal(nil), nil
		}
		start := int(args[0].ToNumber())
		if start < 0 {
			start = n + start
		}
		if start < 0 {
			start = 0
		}
		if start > n {
			start = n
		}
		deleteCount := n - start
		if len(args) > 1 {
			deleteCount = int(args[1].ToNumber())
			if deleteCount < 0 {
				deleteCount = 0
			}
		}
		if start+deleteCount > n {
			deleteCount = n - start
		}
		removed := make([]*Value, deleteCount)
		copy(removed, this.ArrVal[start:start+deleteCount])
		var insertItems []*Value
		if len(args) > 2 {
			insertItems = args[2:]
		}
		newArr := make([]*Value, 0, n-deleteCount+len(insertItems))
		newArr = append(newArr, this.ArrVal[:start]...)
		newArr = append(newArr, insertItems...)
		newArr = append(newArr, this.ArrVal[start+deleteCount:]...)
		this.ArrVal = newArr
		return ArrayVal(removed), nil
	}})
	_arrIndexOfVal = FuncVal(&Function{Name: "indexOf", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return NumberVal(-1), nil
		}
		target := args[0]
		for i, e := range this.ArrVal {
			if e != nil && e.StrictEquals(target) {
				return NumberVal(float64(i)), nil
			}
		}
		return NumberVal(-1), nil
	}})
	_arrIncludesVal = FuncVal(&Function{Name: "includes", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return False, nil
		}
		target := args[0]
		for _, e := range this.ArrVal {
			if e != nil && e.StrictEquals(target) {
				return True, nil
			}
		}
		return False, nil
	}})
	_arrJoinVal = FuncVal(&Function{Name: "join", Native: func(args []*Value, this *Value) (*Value, error) {
		sep := ","
		if len(args) > 0 {
			sep = args[0].ToString()
		}
		parts := make([]string, len(this.ArrVal))
		for i, e := range this.ArrVal {
			if e == nil || e.Tag == TypeNull || e.Tag == TypeUndefined {
				parts[i] = ""
			} else {
				parts[i] = e.ToString()
			}
		}
		return StringVal(strings.Join(parts, sep)), nil
	}})
	_arrReverseVal = FuncVal(&Function{Name: "reverse", Native: func(args []*Value, this *Value) (*Value, error) {
		for i, j := 0, len(this.ArrVal)-1; i < j; i, j = i+1, j-1 {
			this.ArrVal[i], this.ArrVal[j] = this.ArrVal[j], this.ArrVal[i]
		}
		return this, nil
	}})
	_arrMapVal = FuncVal(&Function{Name: "map", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return this, nil
		}
		fn := args[0]
		if fn.Tag != TypeFunction {
			return this, nil
		}
		result := make([]*Value, len(this.ArrVal))
		for i, e := range this.ArrVal {
			if e == nil {
				e = Undefined
			}
			r, err := CallFunction(fn, []*Value{e, NumberVal(float64(i)), this}, nil)
			if err != nil {
				return Null, err
			}
			result[i] = r
		}
		return ArrayVal(result), nil
	}})
	_arrFilterVal = FuncVal(&Function{Name: "filter", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return this, nil
		}
		fn := args[0]
		if fn.Tag != TypeFunction {
			return this, nil
		}
		var result []*Value
		for i, e := range this.ArrVal {
			if e == nil {
				e = Undefined
			}
			r, err := CallFunction(fn, []*Value{e, NumberVal(float64(i)), this}, nil)
			if err != nil {
				return Null, err
			}
			if r.IsTruthy() {
				result = append(result, e)
			}
		}
		return ArrayVal(result), nil
	}})
	_arrReduceVal = FuncVal(&Function{Name: "reduce", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 || this == nil {
			return Undefined, nil
		}
		fn := args[0]
		if fn.Tag != TypeFunction {
			return Undefined, nil
		}
		var acc *Value
		startIdx := 0
		if len(args) > 1 {
			acc = args[1]
		} else if len(this.ArrVal) > 0 {
			acc = this.ArrVal[0]
			startIdx = 1
		} else {
			return Undefined, nil
		}
		var err error
		for i := startIdx; i < len(this.ArrVal); i++ {
			e := this.ArrVal[i]
			if e == nil {
				e = Undefined
			}
			acc, err = CallFunction(fn, []*Value{acc, e, NumberVal(float64(i)), this}, nil)
			if err != nil {
				return Null, err
			}
		}
		return acc, nil
	}})
	_arrForEachVal = FuncVal(&Function{Name: "forEach", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return Undefined, nil
		}
		fn := args[0]
		if fn.Tag != TypeFunction {
			return Undefined, nil
		}
		for i, e := range this.ArrVal {
			if e == nil {
				e = Undefined
			}
			_, err := CallFunction(fn, []*Value{e, NumberVal(float64(i)), this}, nil)
			if err != nil {
				return Null, err
			}
		}
		return Undefined, nil
	}})
	_arrFindVal = FuncVal(&Function{Name: "find", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return Undefined, nil
		}
		fn := args[0]
		for i, e := range this.ArrVal {
			if e == nil {
				e = Undefined
			}
			r, err := CallFunction(fn, []*Value{e, NumberVal(float64(i)), this}, nil)
			if err != nil {
				return Null, err
			}
			if r.IsTruthy() {
				return e, nil
			}
		}
		return Undefined, nil
	}})
	_arrFindIdxVal = FuncVal(&Function{Name: "findIndex", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return NumberVal(-1), nil
		}
		fn := args[0]
		for i, e := range this.ArrVal {
			if e == nil {
				e = Undefined
			}
			r, err := CallFunction(fn, []*Value{e, NumberVal(float64(i)), this}, nil)
			if err != nil {
				return NumberVal(-1), err
			}
			if r.IsTruthy() {
				return NumberVal(float64(i)), nil
			}
		}
		return NumberVal(-1), nil
	}})
	_arrSomeVal = FuncVal(&Function{Name: "some", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return False, nil
		}
		fn := args[0]
		for i, e := range this.ArrVal {
			if e == nil {
				e = Undefined
			}
			r, err := CallFunction(fn, []*Value{e, NumberVal(float64(i)), this}, nil)
			if err != nil {
				return False, err
			}
			if r.IsTruthy() {
				return True, nil
			}
		}
		return False, nil
	}})
	_arrEveryVal = FuncVal(&Function{Name: "every", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return True, nil
		}
		fn := args[0]
		for i, e := range this.ArrVal {
			if e == nil {
				e = Undefined
			}
			r, err := CallFunction(fn, []*Value{e, NumberVal(float64(i)), this}, nil)
			if err != nil {
				return False, err
			}
			if !r.IsTruthy() {
				return False, nil
			}
		}
		return True, nil
	}})
	_arrFlatVal = FuncVal(&Function{Name: "flat", Native: func(args []*Value, this *Value) (*Value, error) {
		depth := 1
		if len(args) > 0 {
			depth = int(args[0].ToNumber())
		}
		return flatArray(this, depth), nil
	}})
	_arrFlatMapVal = FuncVal(&Function{Name: "flatMap", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return this, nil
		}
		fn := args[0]
		var result []*Value
		for i, e := range this.ArrVal {
			if e == nil {
				e = Undefined
			}
			r, err := CallFunction(fn, []*Value{e, NumberVal(float64(i)), this}, nil)
			if err != nil {
				return Null, err
			}
			if r.Tag == TypeArray {
				result = append(result, r.ArrVal...)
			} else {
				result = append(result, r)
			}
		}
		return ArrayVal(result), nil
	}})
	_arrSortVal = FuncVal(&Function{Name: "sort", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(this.ArrVal) == 0 {
			return this, nil
		}
		var sortErr error
		if len(args) > 0 && args[0].Tag == TypeFunction {
			fn := args[0]
			stableSort(this.ArrVal, func(a, b *Value) bool {
				r, err := CallFunction(fn, []*Value{a, b}, nil)
				if err != nil {
					sortErr = err
					return false
				}
				return r.ToNumber() < 0
			})
		} else {
			stableSort(this.ArrVal, func(a, b *Value) bool {
				return a.ToString() < b.ToString()
			})
		}
		return this, sortErr
	}})
	_arrConcatVal = FuncVal(&Function{Name: "concat", Native: func(args []*Value, this *Value) (*Value, error) {
		result := make([]*Value, len(this.ArrVal))
		copy(result, this.ArrVal)
		for _, arg := range args {
			if arg.Tag == TypeArray {
				result = append(result, arg.ArrVal...)
			} else {
				result = append(result, arg)
			}
		}
		return ArrayVal(result), nil
	}})
	_arrToStrVal = FuncVal(&Function{Name: "toString", Native: func(args []*Value, this *Value) (*Value, error) {
		return StringVal(this.ToString()), nil
	}})
	_arrFillVal = FuncVal(&Function{Name: "fill", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return this, nil
		}
		fillVal := args[0]
		start := 0
		end := len(this.ArrVal)
		if len(args) > 1 {
			start = int(args[1].ToNumber())
		}
		if len(args) > 2 {
			end = int(args[2].ToNumber())
		}
		for i := start; i < end && i < len(this.ArrVal); i++ {
			this.ArrVal[i] = fillVal
		}
		return this, nil
	}})
	_arrKeysVal = FuncVal(&Function{Name: "keys", Native: func(args []*Value, this *Value) (*Value, error) {
		result := make([]*Value, len(this.ArrVal))
		for i := range this.ArrVal {
			result[i] = NumberVal(float64(i))
		}
		return ArrayVal(result), nil
	}})
	_arrValuesVal = FuncVal(&Function{Name: "values", Native: func(args []*Value, this *Value) (*Value, error) {
		result := make([]*Value, len(this.ArrVal))
		copy(result, this.ArrVal)
		return ArrayVal(result), nil
	}})
	_arrEntriesVal = FuncVal(&Function{Name: "entries", Native: func(args []*Value, this *Value) (*Value, error) {
		result := make([]*Value, len(this.ArrVal))
		for i, v := range this.ArrVal {
			if v == nil {
				v = Undefined
			}
			result[i] = ArrayVal([]*Value{NumberVal(float64(i)), v})
		}
		return ArrayVal(result), nil
	}})
}

var (
	_strToUpperVal     *Value
	_strToLowerVal     *Value
	_strTrimVal        *Value
	_strTrimStartVal   *Value
	_strTrimEndVal     *Value
	_strSplitVal       *Value
	_strIncludesVal    *Value
	_strStartsWithVal  *Value
	_strEndsWithVal    *Value
	_strIndexOfVal     *Value
	_strLastIndexOfVal *Value
	_strSliceVal       *Value
	_strSubstringVal   *Value
	_strReplaceVal     *Value
	_strReplaceAllVal  *Value
	_strRepeatVal      *Value
	_strCharAtVal      *Value
	_strCharCodeAtVal  *Value
	_strPadStartVal    *Value
	_strPadEndVal      *Value
	_strMatchVal       *Value
	_strSearchVal      *Value
	_strToStringVal    *Value
)

func init() {
	_strToUpperVal = FuncVal(&Function{Name: "toUpperCase", Native: func(args []*Value, this *Value) (*Value, error) {
		return StringVal(strings.ToUpper(this.StrVal)), nil
	}})
	_strToLowerVal = FuncVal(&Function{Name: "toLowerCase", Native: func(args []*Value, this *Value) (*Value, error) {
		return StringVal(strings.ToLower(this.StrVal)), nil
	}})
	_strTrimVal = FuncVal(&Function{Name: "trim", Native: func(args []*Value, this *Value) (*Value, error) {
		return StringVal(strings.TrimSpace(this.StrVal)), nil
	}})
	_strTrimStartVal = FuncVal(&Function{Name: "trimStart", Native: func(args []*Value, this *Value) (*Value, error) {
		return StringVal(strings.TrimLeftFunc(this.StrVal, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '\n' || r == '\r'
		})), nil
	}})
	_strTrimEndVal = FuncVal(&Function{Name: "trimEnd", Native: func(args []*Value, this *Value) (*Value, error) {
		return StringVal(strings.TrimRightFunc(this.StrVal, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '\n' || r == '\r'
		})), nil
	}})
	_strSplitVal = FuncVal(&Function{Name: "split", Native: func(args []*Value, this *Value) (*Value, error) {
		sep := ""
		if len(args) > 0 {
			sep = args[0].ToString()
		}
		var parts []string
		if len(args) == 0 || args[0].Tag == TypeNull || args[0].Tag == TypeUndefined {
			parts = []string{this.StrVal}
		} else {
			parts = strings.Split(this.StrVal, sep)
		}
		result := make([]*Value, len(parts))
		for i, p := range parts {
			result[i] = StringVal(p)
		}
		return ArrayVal(result), nil
	}})
	_strIncludesVal = FuncVal(&Function{Name: "includes", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return False, nil
		}
		return BoolVal(strings.Contains(this.StrVal, args[0].ToString())), nil
	}})
	_strStartsWithVal = FuncVal(&Function{Name: "startsWith", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return False, nil
		}
		return BoolVal(strings.HasPrefix(this.StrVal, args[0].ToString())), nil
	}})
	_strEndsWithVal = FuncVal(&Function{Name: "endsWith", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return False, nil
		}
		return BoolVal(strings.HasSuffix(this.StrVal, args[0].ToString())), nil
	}})
	_strIndexOfVal = FuncVal(&Function{Name: "indexOf", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return NumberVal(-1), nil
		}
		idx := strings.Index(this.StrVal, args[0].ToString())
		return NumberVal(float64(idx)), nil
	}})
	_strLastIndexOfVal = FuncVal(&Function{Name: "lastIndexOf", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return NumberVal(-1), nil
		}
		idx := strings.LastIndex(this.StrVal, args[0].ToString())
		return NumberVal(float64(idx)), nil
	}})
	_strSliceVal = FuncVal(&Function{Name: "slice", Native: func(args []*Value, this *Value) (*Value, error) {
		runes := []rune(this.StrVal)
		n := len(runes)
		start, end := 0, n
		if len(args) > 0 {
			start = int(args[0].ToNumber())
			if start < 0 {
				start = n + start
			}
			if start < 0 {
				start = 0
			}
		}
		if len(args) > 1 {
			end = int(args[1].ToNumber())
			if end < 0 {
				end = n + end
			}
		}
		if start > n {
			start = n
		}
		if end > n {
			end = n
		}
		if end < start {
			end = start
		}
		return StringVal(string(runes[start:end])), nil
	}})
	_strSubstringVal = FuncVal(&Function{Name: "substring", Native: func(args []*Value, this *Value) (*Value, error) {
		runes := []rune(this.StrVal)
		n := len(runes)
		start, end := 0, n
		if len(args) > 0 {
			start = int(args[0].ToNumber())
		}
		if len(args) > 1 {
			end = int(args[1].ToNumber())
		}
		if start < 0 {
			start = 0
		}
		if end < 0 {
			end = 0
		}
		if start > n {
			start = n
		}
		if end > n {
			end = n
		}
		if start > end {
			start, end = end, start
		}
		return StringVal(string(runes[start:end])), nil
	}})

	_strReplaceVal = FuncVal(&Function{Name: "replace", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) < 2 {
			return this, nil
		}
		src := this.StrVal

		if args[0].Tag == TypeRegex {
			re := args[0].RegexVal
			return strReplaceRegex(src, re, args[1], false)
		}

		pattern := args[0].ToString()
		if pattern == "" {

			repl, err := strResolveReplacement(args[1], "", nil, 0, src)
			if err != nil {
				return this, err
			}
			return StringVal(repl + src), nil
		}

		nth := 1
		if len(args) >= 3 && args[2].Tag == TypeNumber {
			n := int(args[2].ToNumber())
			if n > 0 {
				nth = n
			}
		}

		return strReplaceNth(src, pattern, args[1], nth)
	}})

	_strReplaceAllVal = FuncVal(&Function{Name: "replaceAll", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) < 2 {
			return this, nil
		}
		src := this.StrVal

		if args[0].Tag == TypeRegex {
			re := args[0].RegexVal
			return strReplaceRegex(src, re, args[1], true)
		}

		pattern := args[0].ToString()
		if pattern == "" {

			runes := []rune(src)
			var sb strings.Builder
			for i, r := range runes {
				repl, err := strResolveReplacement(args[1], "", nil, i, src)
				if err != nil {
					return this, err
				}
				sb.WriteString(repl)
				sb.WriteRune(r)
			}

			repl, err := strResolveReplacement(args[1], "", nil, len(runes), src)
			if err != nil {
				return this, err
			}
			sb.WriteString(repl)
			return StringVal(sb.String()), nil
		}

		return strReplaceNth(src, pattern, args[1], -1)
	}})
	_strRepeatVal = FuncVal(&Function{Name: "repeat", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return this, nil
		}
		return StringVal(strings.Repeat(this.StrVal, int(args[0].ToNumber()))), nil
	}})
	_strCharAtVal = FuncVal(&Function{Name: "charAt", Native: func(args []*Value, this *Value) (*Value, error) {
		runes := []rune(this.StrVal)
		if len(args) == 0 {
			return StringVal(""), nil
		}
		idx := int(args[0].ToNumber())
		if idx < 0 || idx >= len(runes) {
			return StringVal(""), nil
		}
		return StringVal(string(runes[idx])), nil
	}})
	_strCharCodeAtVal = FuncVal(&Function{Name: "charCodeAt", Native: func(args []*Value, this *Value) (*Value, error) {
		runes := []rune(this.StrVal)
		if len(args) == 0 {
			return NumberVal(math.NaN()), nil
		}
		idx := int(args[0].ToNumber())
		if idx < 0 || idx >= len(runes) {
			return NumberVal(math.NaN()), nil
		}
		return NumberVal(float64(runes[idx])), nil
	}})
	_strPadStartVal = FuncVal(&Function{Name: "padStart", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return this, nil
		}
		targetLen := int(args[0].ToNumber())
		padStr := " "
		if len(args) > 1 {
			padStr = args[1].ToString()
		}
		s := this.StrVal
		for len([]rune(s)) < targetLen {
			s = padStr + s
		}
		runes := []rune(s)
		return StringVal(string(runes[len(runes)-targetLen:])), nil
	}})
	_strPadEndVal = FuncVal(&Function{Name: "padEnd", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return this, nil
		}
		targetLen := int(args[0].ToNumber())
		padStr := " "
		if len(args) > 1 {
			padStr = args[1].ToString()
		}
		s := this.StrVal
		for len([]rune(s)) < targetLen {
			s = s + padStr
		}
		runes := []rune(s)
		return StringVal(string(runes[:targetLen])), nil
	}})
	_strMatchVal = FuncVal(&Function{Name: "match", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return Null, nil
		}
		var re *regexp.Regexp
		var err error
		if args[0].Tag == TypeRegex {
			re = args[0].RegexVal
		} else {
			re, err = regexp.Compile(args[0].ToString())
			if err != nil {
				return Null, nil
			}
		}
		matches := re.FindAllString(this.StrVal, -1)
		if matches == nil {
			return Null, nil
		}
		result := make([]*Value, len(matches))
		for i, m := range matches {
			result[i] = StringVal(m)
		}
		return ArrayVal(result), nil
	}})
	_strSearchVal = FuncVal(&Function{Name: "search", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return NumberVal(-1), nil
		}
		var re *regexp.Regexp
		var err error
		if args[0].Tag == TypeRegex {
			re = args[0].RegexVal
		} else {
			re, err = regexp.Compile(args[0].ToString())
			if err != nil {
				return NumberVal(-1), nil
			}
		}
		loc := re.FindStringIndex(this.StrVal)
		if loc == nil {
			return NumberVal(-1), nil
		}
		return NumberVal(float64(loc[0])), nil
	}})
	_strToStringVal = FuncVal(&Function{Name: "toString", Native: func(args []*Value, this *Value) (*Value, error) {
		return this, nil
	}})
}

func strResolveReplacement(replacement *Value, match string, submatches []string, offset int, src string) (string, error) {

	if replacement.Tag == TypeFunction && CallFunction != nil {

		callArgs := make([]*Value, 0, 2+len(submatches))
		callArgs = append(callArgs, StringVal(match))
		for _, sub := range submatches {
			callArgs = append(callArgs, StringVal(sub))
		}
		callArgs = append(callArgs, NumberVal(float64(offset)), StringVal(src))
		result, err := CallFunction(replacement, callArgs, nil)
		if err != nil {
			return match, err
		}
		if result == nil || result.Tag == TypeUndefined || result.Tag == TypeNull {
			return match, nil
		}
		return result.ToString(), nil
	}

	tmpl := replacement.ToString()
	if !strings.ContainsRune(tmpl, '$') {
		return tmpl, nil
	}

	var sb strings.Builder
	i := 0
	for i < len(tmpl) {
		if tmpl[i] != '$' || i+1 >= len(tmpl) {
			sb.WriteByte(tmpl[i])
			i++
			continue
		}
		next := tmpl[i+1]
		switch {
		case next == '$':

			sb.WriteByte('$')
			i += 2
		case next == '&' || next == '0':

			sb.WriteString(match)
			i += 2
		case next == '`':

			if offset >= 0 && offset <= len(src) {
				sb.WriteString(src[:offset])
			}
			i += 2
		case next == '\'':

			end := offset + len(match)
			if end <= len(src) {
				sb.WriteString(src[end:])
			}
			i += 2
		case next >= '1' && next <= '9':

			idx := int(next - '0')

			if i+2 < len(tmpl) && tmpl[i+2] >= '0' && tmpl[i+2] <= '9' {
				idx2 := idx*10 + int(tmpl[i+2]-'0')
				if idx2 < len(submatches) {
					sb.WriteString(submatches[idx2-1])
					i += 3
					continue
				}
			}
			if idx <= len(submatches) {
				sb.WriteString(submatches[idx-1])
			}
			i += 2
		case next == '<':

			end := strings.IndexByte(tmpl[i+2:], '>')
			if end < 0 {
				sb.WriteByte('$')
				i++
				continue
			}

			i += 2 + end + 1
		default:
			sb.WriteByte('$')
			i++
		}
	}
	return sb.String(), nil
}

func strReplaceNth(src, pattern string, replacement *Value, nth int) (*Value, error) {
	if nth == -1 {

		if replacement.Tag != TypeFunction {

			repl, err := strResolveReplacement(replacement, pattern, nil, 0, src)
			if err != nil {
				return StringVal(src), err
			}
			return StringVal(strings.ReplaceAll(src, pattern, repl)), nil
		}
		var sb strings.Builder
		remaining := src
		byteOffset := 0
		for {
			idx := strings.Index(remaining, pattern)
			if idx < 0 {
				sb.WriteString(remaining)
				break
			}
			sb.WriteString(remaining[:idx])
			repl, err := strResolveReplacement(replacement, pattern, nil, byteOffset+idx, src)
			if err != nil {
				sb.WriteString(pattern)
			} else {
				sb.WriteString(repl)
			}
			byteOffset += idx + len(pattern)
			remaining = remaining[idx+len(pattern):]
		}
		return StringVal(sb.String()), nil
	}

	count := 0
	var sb strings.Builder
	remaining := src
	byteOffset := 0
	found := false
	for {
		idx := strings.Index(remaining, pattern)
		if idx < 0 {
			sb.WriteString(remaining)
			break
		}
		count++
		sb.WriteString(remaining[:idx])
		if count == nth {
			repl, err := strResolveReplacement(replacement, pattern, nil, byteOffset+idx, src)
			if err != nil {
				sb.WriteString(pattern)
			} else {
				sb.WriteString(repl)
			}
			sb.WriteString(remaining[idx+len(pattern):])
			found = true
			break
		}
		sb.WriteString(pattern)
		byteOffset += idx + len(pattern)
		remaining = remaining[idx+len(pattern):]
	}
	_ = found
	return StringVal(sb.String()), nil
}

func strReplaceRegex(src string, re *regexp.Regexp, replacement *Value, all bool) (*Value, error) {
	names := re.SubexpNames()

	expand := func(loc []int, subLocs []int) (string, error) {
		match := src[loc[0]:loc[1]]
		captures := make([]string, 0, len(subLocs)/2)
		for i := 0; i < len(subLocs)-1; i += 2 {
			s, e := subLocs[i], subLocs[i+1]
			if s < 0 {
				captures = append(captures, "")
			} else {
				captures = append(captures, src[s:e])
			}
		}

		if replacement.Tag == TypeFunction && CallFunction != nil {
			callArgs := make([]*Value, 0, 2+len(captures))
			callArgs = append(callArgs, StringVal(match))
			for _, c := range captures {
				callArgs = append(callArgs, StringVal(c))
			}
			callArgs = append(callArgs, NumberVal(float64(loc[0])), StringVal(src))
			result, err := CallFunction(replacement, callArgs, nil)
			if err != nil {
				return match, err
			}
			if result == nil || result.Tag == TypeUndefined || result.Tag == TypeNull {
				return match, nil
			}
			return result.ToString(), nil
		}

		tmpl := replacement.ToString()
		if !strings.ContainsRune(tmpl, '$') {
			return tmpl, nil
		}

		if strings.Contains(tmpl, "$<") {
			var nb strings.Builder
			t := tmpl
			for {
				si := strings.Index(t, "$<")
				if si < 0 {
					nb.WriteString(t)
					break
				}
				nb.WriteString(t[:si])
				t = t[si+2:]
				ei := strings.IndexByte(t, '>')
				if ei < 0 {
					nb.WriteString("$<")
					nb.WriteString(t)
					break
				}
				name := t[:ei]
				t = t[ei+1:]

				resolved := false
				for gi, n := range names {
					if gi > 0 && n == name {
						s, e := subLocs[(gi-1)*2], subLocs[(gi-1)*2+1]
						if s >= 0 {
							nb.WriteString(src[s:e])
						}
						resolved = true
						break
					}
				}
				if !resolved {
					nb.WriteString("$<")
					nb.WriteString(name)
					nb.WriteByte('>')
				}
			}
			tmpl = nb.String()
		}

		return strResolveReplacement(StringVal(tmpl), match, captures, loc[0], src)
	}

	if all {

		allLocs := re.FindAllStringSubmatchIndex(src, -1)
		if len(allLocs) == 0 {
			return StringVal(src), nil
		}
		var sb strings.Builder
		prev := 0
		for _, locs := range allLocs {
			sb.WriteString(src[prev:locs[0]])
			repl, err := expand(locs[:2], locs[2:])
			if err != nil {
				sb.WriteString(src[locs[0]:locs[1]])
			} else {
				sb.WriteString(repl)
			}
			prev = locs[1]
		}
		sb.WriteString(src[prev:])
		return StringVal(sb.String()), nil
	}

	locs := re.FindStringSubmatchIndex(src)
	if locs == nil {
		return StringVal(src), nil
	}
	repl, err := expand(locs[:2], locs[2:])
	if err != nil {
		return StringVal(src), err
	}
	var sb strings.Builder
	sb.WriteString(src[:locs[0]])
	sb.WriteString(repl)
	sb.WriteString(src[locs[1]:])
	return StringVal(sb.String()), nil
}

func arrayBuiltin(v *Value, key string) *Value {
	switch key {
	case "length":
		return NumberVal(float64(len(v.ArrVal)))
	case "push":
		return _arrPushVal
	case "pop":
		return _arrPopVal
	case "shift":
		return _arrShiftVal
	case "unshift":
		return _arrUnshiftVal
	case "slice":
		return _arrSliceVal
	case "splice":
		return _arrSpliceVal
	case "indexOf":
		return _arrIndexOfVal
	case "includes":
		return _arrIncludesVal
	case "join":
		return _arrJoinVal
	case "reverse":
		return _arrReverseVal
	case "map":
		return _arrMapVal
	case "filter":
		return _arrFilterVal
	case "reduce":
		return _arrReduceVal
	case "forEach":
		return _arrForEachVal
	case "find":
		return _arrFindVal
	case "findIndex":
		return _arrFindIdxVal
	case "some":
		return _arrSomeVal
	case "every":
		return _arrEveryVal
	case "flat":
		return _arrFlatVal
	case "flatMap":
		return _arrFlatMapVal
	case "sort":
		return _arrSortVal
	case "concat":
		return _arrConcatVal
	case "toString":
		return _arrToStrVal
	case "fill":
		return _arrFillVal
	case "keys":
		return _arrKeysVal
	case "values":
		return _arrValuesVal
	case "entries":
		return _arrEntriesVal
	}
	return Undefined
}

func stringBuiltin(v *Value, key string) *Value {
	switch key {
	case "length":
		return NumberVal(float64(len([]rune(v.StrVal))))
	case "toUpperCase", "upper":
		return _strToUpperVal
	case "toLowerCase", "lower":
		return _strToLowerVal
	case "trim":
		return _strTrimVal
	case "trimStart", "trimLeft":
		return _strTrimStartVal
	case "trimEnd", "trimRight":
		return _strTrimEndVal
	case "split":
		return _strSplitVal
	case "includes":
		return _strIncludesVal
	case "startsWith":
		return _strStartsWithVal
	case "endsWith":
		return _strEndsWithVal
	case "indexOf":
		return _strIndexOfVal
	case "lastIndexOf":
		return _strLastIndexOfVal
	case "slice":
		return _strSliceVal
	case "substring":
		return _strSubstringVal
	case "replace":
		return _strReplaceVal
	case "replaceAll":
		return _strReplaceAllVal
	case "repeat":
		return _strRepeatVal
	case "charAt":
		return _strCharAtVal
	case "charCodeAt":
		return _strCharCodeAtVal
	case "padStart":
		return _strPadStartVal
	case "padEnd":
		return _strPadEndVal
	case "match":
		return _strMatchVal
	case "search":
		return _strSearchVal
	case "toString", "valueOf":
		return _strToStringVal
	}
	return Undefined
}

var (
	_numToStringVal    *Value
	_numToFixedVal     *Value
	_numToPrecisionVal *Value
	_numValueOfVal     *Value
)

func initNumberBuiltins() {
	_numToStringVal = FuncVal(&Function{Name: "toString", Native: func(args []*Value, this *Value) (*Value, error) {
		base := 10
		if len(args) > 0 {
			base = int(args[0].ToNumber())
		}
		if base == 10 {
			return StringVal(this.ToString()), nil
		}
		n := int64(this.NumVal)
		switch base {
		case 16:
			return StringVal(fmt.Sprintf("%x", n)), nil
		case 8:
			return StringVal(fmt.Sprintf("%o", n)), nil
		case 2:
			return StringVal(fmt.Sprintf("%b", n)), nil
		}
		return StringVal(this.ToString()), nil
	}})
	_numToFixedVal = FuncVal(&Function{Name: "toFixed", Native: func(args []*Value, this *Value) (*Value, error) {
		digits := 0
		if len(args) > 0 {
			digits = int(args[0].ToNumber())
		}
		return StringVal(fmt.Sprintf("%.*f", digits, this.NumVal)), nil
	}})
	_numToPrecisionVal = FuncVal(&Function{Name: "toPrecision", Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return StringVal(this.ToString()), nil
		}
		prec := int(args[0].ToNumber())
		return StringVal(fmt.Sprintf("%.*g", prec, this.NumVal)), nil
	}})
	_numValueOfVal = FuncVal(&Function{Name: "valueOf", Native: func(args []*Value, this *Value) (*Value, error) {
		return this, nil
	}})
}

func numberBuiltin(v *Value, key string) *Value {
	switch key {
	case "toString":
		return _numToStringVal
	case "toFixed":
		return _numToFixedVal
	case "toPrecision":
		return _numToPrecisionVal
	case "valueOf":
		return _numValueOfVal
	}
	return Undefined
}

var CallFunction func(fn *Value, args []*Value, this ...*Value) (*Value, error)

var keepAliveWG sync.WaitGroup

func KeepAliveAdd()  { keepAliveWG.Add(1) }
func KeepAliveDone() { keepAliveWG.Done() }
func KeepAliveWait() { keepAliveWG.Wait() }

func flatArray(v *Value, depth int) *Value {
	if depth == 0 || v.Tag != TypeArray {
		return v
	}
	var result []*Value
	for _, e := range v.ArrVal {
		if e != nil && e.Tag == TypeArray && depth > 0 {
			flattened := flatArray(e, depth-1)
			result = append(result, flattened.ArrVal...)
		} else {
			result = append(result, e)
		}
	}
	return ArrayVal(result)
}

func stableSort(arr []*Value, less func(a, b *Value) bool) {
	for i, v := range arr {
		if v == nil {
			arr[i] = Undefined
		}
	}
	sort.SliceStable(arr, func(i, j int) bool {
		return less(arr[i], arr[j])
	})
}
