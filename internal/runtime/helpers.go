package runtime

import (
	"lunex/internal/ast"
	"lunex/internal/errfmt"
	"math"
	"strings"
	"sync"
	"time"
)

func (e *returnError) Error() string { return "return" }
func (e *throwError) Error() string {
	if e.val != nil {
		if e.val.Tag == TypeObject {
			if msg, ok := e.val.ObjVal["message"]; ok {
				return msg.ToString()
			}
		}
		return e.val.ToString()
	}
	return "thrown"
}

func (e *throwError) DiagnosticError() *errfmt.LunexError {
	if e == nil || e.val == nil || e.val.Tag != TypeObject {
		return nil
	}
	codeVal, ok := e.val.ObjVal["code"]
	if !ok {
		return nil
	}
	code := codeVal.ToString()
	if _, ok := errfmt.LookupCode(code); !ok {
		return nil
	}
	message := e.Error()
	if message == "" {
		return nil
	}
	return errfmt.New(errfmt.KindRuntime, code, message, "", 0, 0, nil)
}
func (e *breakError) Error() string    { return "break" }
func (e *continueError) Error() string { return "continue" }

func paramsToFnParams(params []*ast.Param) []FnParam {
	result := make([]FnParam, len(params))
	for i, p := range params {
		result[i] = FnParam{
			Name:         p.Name,
			Default:      p.DefaultVal,
			Rest:         p.Rest,
			Destructure:  p.Destructure,
			ResolvedSlot: p.ResolvedSlot,
		}
	}
	return result
}

func isInstanceOf(inst *Instance, cls *Class) bool {
	if inst.Class == cls {
		return true
	}
	if inst.Class != nil && inst.Class.Super != nil {
		return isInstanceOf(&Instance{Class: inst.Class.Super}, cls)
	}
	return false
}

func buildArgSig(args []*Value) string {
	types := make([]string, len(args))
	for i, a := range args {
		types[i] = a.TypeName()
	}
	return strings.Join(types, ",")
}

func mathFn1(name string, fn func(float64) float64) *Value {
	return FuncVal(&Function{Name: name, Native: func(args []*Value, this *Value) (*Value, error) {
		if len(args) == 0 {
			return NumberVal(math.NaN()), nil
		}
		return NumberVal(fn(args[0].ToNumber())), nil
	}})
}

func mathSign(x float64) float64 {
	if x > 0 {
		return 1
	}
	if x < 0 {
		return -1
	}
	return 0
}

var randState = uint64(time.Now().UnixNano()) | 1

func pseudoRandom() float64 {
	randState ^= randState << 13
	randState ^= randState >> 7
	randState ^= randState << 17
	return float64(randState&0x7FFFFFFFFFFFFFFF) / float64(0x7FFFFFFFFFFFFFFF)
}

type ntlMap struct {
	data     map[string]*Value
	keyOrder []string
	mu       sync.RWMutex
}

type ntlSet struct {
	items map[string]*Value
	mu    sync.RWMutex
}
