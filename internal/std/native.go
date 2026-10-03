package std

import (
	"lunex/internal/runtime"
	shared "lunex/internal/std/shared"
)

func NativeModule(interp *runtime.Interpreter) *runtime.Value {
	return runtime.ObjectVal(map[string]*runtime.Value{
		"isString": runtime.FuncVal(&runtime.Function{Name: "isString", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			return runtime.BoolVal(len(args) > 0 && args[0].Tag == runtime.TypeString), nil
		}}),
		"isNumber": runtime.FuncVal(&runtime.Function{Name: "isNumber", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			return runtime.BoolVal(len(args) > 0 && args[0].Tag == runtime.TypeNumber), nil
		}}),
		"isBool": runtime.FuncVal(&runtime.Function{Name: "isBool", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			return runtime.BoolVal(len(args) > 0 && args[0].Tag == runtime.TypeBool), nil
		}}),
		"isArray": runtime.FuncVal(&runtime.Function{Name: "isArray", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			return runtime.BoolVal(len(args) > 0 && args[0].Tag == runtime.TypeArray), nil
		}}),
		"isObject": runtime.FuncVal(&runtime.Function{Name: "isObject", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			return runtime.BoolVal(len(args) > 0 && args[0].Tag == runtime.TypeObject), nil
		}}),
		"isNull": runtime.FuncVal(&runtime.Function{Name: "isNull", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			return runtime.BoolVal(len(args) > 0 && args[0].Tag == runtime.TypeNull), nil
		}}),
		"isUndefined": runtime.FuncVal(&runtime.Function{Name: "isUndefined", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			return runtime.BoolVal(len(args) == 0 || args[0].Tag == runtime.TypeUndefined), nil
		}}),
		"isFunction": runtime.FuncVal(&runtime.Function{Name: "isFunction", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			return runtime.BoolVal(len(args) > 0 && args[0].Tag == runtime.TypeFunction), nil
		}}),
		"typeOf": runtime.FuncVal(&runtime.Function{
			Name: "typeOf",
			Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
				if len(args) == 0 {
					return runtime.StringVal("undefined"), nil
				}
				return runtime.StringVal(shared.GetTypeName(args[0])), nil
			},
		}), "deepClone": runtime.FuncVal(&runtime.Function{Name: "deepClone", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.Undefined, nil
			}
			return shared.DeepCopy(args[0]), nil
		}}),
		"deepEqual": runtime.FuncVal(&runtime.Function{Name: "deepEqual", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.False, nil
			}
			return runtime.BoolVal(shared.DeepEqual(args[0], args[1])), nil
		}}),
		"ffi": ffiNativeModule(interp),
	})
}
