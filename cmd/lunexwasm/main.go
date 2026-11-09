//go:build js && wasm

package main

import (
	"fmt"
	"path"
	"strings"
	"syscall/js"

	"lunex/internal/compiler"
	"lunex/internal/errfmt"
	"lunex/internal/runtime"
	"lunex/internal/std"
)

var runCallback js.Func
var readyCallback js.Func

const entryFilename = "<browser>"

func result(ok bool, message string) any {
	return map[string]any{"ok": ok, "error": message}
}

type projectFiles struct {
	filename string
	files    map[string]string
}

func normalizeKey(p string) string {
	cleaned := path.Clean(strings.ReplaceAll(p, "\\", "/"))
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = strings.TrimPrefix(cleaned, "./")
	return cleaned
}

func isLocalPath(name string) bool {
	return strings.HasPrefix(name, "./") ||
		strings.HasPrefix(name, "../") ||
		strings.HasPrefix(name, "/") ||
		strings.HasSuffix(name, ".lx") ||
		strings.HasSuffix(name, ".nax")
}

func (p *projectFiles) fileLoader(name string) (src, realPath string, ok bool) {
	if p == nil || len(p.files) == 0 {
		return "", "", false
	}
	if !isLocalPath(name) {
		return "", "", false
	}
	key := normalizeKey(name)
	if source, found := p.files[key]; found {
		return source, key, true
	}
	return "", "", false
}

func parseProject(arg js.Value) *projectFiles {
	if arg.IsUndefined() || arg.IsNull() || arg.Type() != js.TypeObject {
		return nil
	}

	p := &projectFiles{files: make(map[string]string)}

	if filenameVal := arg.Get("filename"); filenameVal.Type() == js.TypeString {
		p.filename = filenameVal.String()
	}

	filesVal := arg.Get("files")
	if filesVal.Type() == js.TypeObject {
		keys := js.Global().Get("Object").Call("keys", filesVal)
		for i := 0; i < keys.Length(); i++ {
			key := keys.Index(i).String()
			val := filesVal.Get(key)
			if val.Type() == js.TypeString {
				p.files[normalizeKey(key)] = val.String()
			}
		}
	}

	if len(p.files) == 0 && p.filename == "" {
		return nil
	}
	return p
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

	var project *projectFiles
	if len(args) > 1 {
		project = parseProject(args[1])
	}

	filename := entryFilename
	if project != nil && project.filename != "" {
		filename = project.filename
	}

	c := compiler.New(compiler.DefaultOptions)
	std.RegisterLazy(c)

	interp := c.Interpreter()
	if project != nil {
		interp.SetNTLFileLoader(runtime.NTLFileLoader(project.fileLoader))
	}

	compiled := c.CompileSource(source, filename)
	if !compiled.Success {
		messages := make([]string, 0, len(compiled.Errors))
		for _, diagnostic := range compiled.Errors {
			messages = append(messages, strings.TrimSpace(errfmt.Format(diagnostic)))
		}
		return result(false, strings.Join(messages, "\n"))
	}

	interp.SetFilename(filename)
	interp.SetSourceLines(strings.Split(source, "\n"))
	if _, err := interp.Exec(compiled.AST); err != nil {
		return result(false, err.Error())
	}
	if err := interp.CallMain(); err != nil {
		return result(false, err.Error())
	}
	return result(true, "")
}

func ready(_ js.Value, args []js.Value) any {
	promiseConstructor := js.Global().Get("Promise")
	return promiseConstructor.New(js.FuncOf(func(_ js.Value, resolveArgs []js.Value) any {
		resolve := resolveArgs[0]
		std.OnFsHydrated(func() {
			resolve.Invoke(js.Undefined())
		})
		return nil
	}))
}

func main() {
	api := js.Global().Get("Object").New()
	runCallback = js.FuncOf(run)
	readyCallback = js.FuncOf(ready)
	api.Set("run", runCallback)
	api.Set("ready", readyCallback)
	js.Global().Set("Lunex", api)
	select {}
}
