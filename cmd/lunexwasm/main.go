//go:build js && wasm

package main

import (
	"fmt"
	"strings"
	"syscall/js"

	"lunex/internal/compiler"
	"lunex/internal/errfmt"
	"lunex/internal/std"
)

var runCallback js.Func

func result(ok bool, message string) any {
	return map[string]any{"ok": ok, "error": message}
}

func run(_ js.Value, args []js.Value) (response any) {
	defer func() {
		if recovered := recover(); recovered != nil {
			response = result(false, fmt.Sprint(recovered))
		}
	}()
	if len(args) == 0 || args[0].Type() != js.TypeString {
		return result(false, "source must be a string")
	}

	source := args[0].String()
	c := compiler.New(compiler.DefaultOptions)
	std.RegisterLazy(c)
	compiled := c.CompileSource(source, "<browser>")
	if !compiled.Success {
		messages := make([]string, 0, len(compiled.Errors))
		for _, diagnostic := range compiled.Errors {
			messages = append(messages, strings.TrimSpace(errfmt.Format(diagnostic)))
		}
		return result(false, strings.Join(messages, "\n"))
	}

	interp := c.Interpreter()
	interp.SetFilename("<browser>")
	interp.SetSourceLines(strings.Split(source, "\n"))
	if _, err := interp.Exec(compiled.AST); err != nil {
		return result(false, err.Error())
	}
	if err := interp.CallMain(); err != nil {
		return result(false, err.Error())
	}
	return result(true, "")
}

func main() {
	api := js.Global().Get("Object").New()
	runCallback = js.FuncOf(run)
	api.Set("run", runCallback)
	js.Global().Set("Lunex", api)
	select {}
}
