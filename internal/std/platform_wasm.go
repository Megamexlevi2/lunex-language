//go:build js || wasip1

package std

func isWasmRuntime() bool {
	return true
}
