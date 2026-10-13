//go:build js || wasip1

package std

import (
	"fmt"
	"lunex/internal/runtime"
)

func DbModule() *runtime.Value {
	methods := make(map[string]*runtime.Value)
	for _, name := range []string{"create", "open", "connect", "drop", "list", "table", "collection"} {
		method := name
		methods[method] = runtime.FuncVal(&runtime.Function{
			Name: method,
			Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
				return runtime.Undefined, fmt.Errorf("db.%s does not exist", method)
			},
		})
	}
	return runtime.ObjectVal(methods)
}
