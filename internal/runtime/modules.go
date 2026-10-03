package runtime

import (
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/errfmt"
	"lunex/internal/lexer"
	"lunex/internal/parser"
	"lunex/internal/resolver"
	"os"
	"path/filepath"
	"strings"
)

func (interp *Interpreter) evalAtImport(node *ast.Node, env *Environment) (*Value, error) {
	path := node.Source
	resolved := resolveModulePath(path)
	if resolved == "native" && interp.libLoadDepth == 0 {
		e := interp.runtimeError(errfmt.KindImport, "E0014",
			fmt.Sprintf("module %q is internal and cannot be imported by user code — use a standard lib module like @import(\"std.io\")", path), node, nil)
		return nil, e
	}
	if forceLocalImport(node) {

		if localPath, ok := interp.resolveLocalFile(path); ok {
			return interp.loadLocalFile(localPath, node)
		}

		if interp.ntlFileLoader != nil {
			if src, realPath, ok := interp.ntlFileLoader(path); ok && realPath != "" {
				abs := realPath
				if !filepath.IsAbs(abs) {
					abs, _ = filepath.Abs(realPath)
				}
				return interp.evalModuleSourceFile(src, abs, abs)
			}
		}
		if interp.astLoader != nil {
			if tree, realPath, ok := interp.astLoader(path); ok {
				if realPath == "" {
					realPath = path
				}
				return interp.evalModuleAST(tree, realPath)
			}
		}
		if interp.ntlLoader != nil {
			if src, ok := interp.ntlLoader(path); ok {
				return interp.evalModuleSourceFile(src, path, path)
			}
		}
		e := interp.runtimeError(errfmt.KindImport, "E0010F",
			fmt.Sprintf("local file %q not found", path), node, nil)
		return nil, e
	}
	return interp.loadModule(path)
}

func (interp *Interpreter) loadLocalFile(localPath string, node *ast.Node) (*Value, error) {
	abs, _ := filepath.Abs(localPath)

	interp.mu.RLock()
	if mod, ok := interp.modules[abs]; ok {
		interp.mu.RUnlock()
		return mod, nil
	}
	interp.mu.RUnlock()

	ext := strings.ToLower(filepath.Ext(localPath))
	switch ext {
	case ".nax":
		if interp.naxLoader == nil {
			e := interp.runtimeError(errfmt.KindImport, "E0015",
				fmt.Sprintf("cannot load %q: binary module loader is not available in this context", localPath), node, nil)
			return nil, e
		}
		mod, err := interp.naxLoader(abs)
		if err != nil {
			e := interp.runtimeError(errfmt.KindImport, "E0015",
				fmt.Sprintf("failed to load binary module %q: %v", localPath, err), node, nil)
			return nil, e
		}
		interp.mu.Lock()
		interp.modules[abs] = mod
		interp.mu.Unlock()
		return mod, nil

	default:
		data, err := os.ReadFile(localPath)
		if err != nil {
			e := interp.runtimeError(errfmt.KindImport, "E0015",
				fmt.Sprintf("cannot read file %q: %v", localPath, err), node, nil)
			return nil, e
		}
		return interp.evalModuleSourceFile(string(data), abs, localPath)
	}
}
func (interp *Interpreter) execImport(node *ast.Node, env *Environment) (*Value, error) {
	if node.TypeOnly {
		return Undefined, nil
	}
	modVal, err := interp.loadModule(node.Source)
	if err != nil {
		return nil, err
	}
	if node.Namespace != "" {
		env.Define(node.Namespace, modVal, true)
	} else if node.DefaultImport != "" && len(node.Specifiers) == 0 {
		env.Define(node.DefaultImport, modVal, true)
	} else {
		if node.DefaultImport != "" {
			def := modVal.Get("default")
			if def.IsNullish() {
				def = modVal
			}
			env.Define(node.DefaultImport, def, true)
		}
		for _, spec := range node.Specifiers {
			val := modVal.Get(spec.Imported)
			env.Define(spec.Local, val, true)
		}
	}
	return Undefined, nil
}

func resolveModulePath(path string) string {

	if strings.HasSuffix(path, ".lx") ||
		strings.HasPrefix(path, "./") ||
		strings.HasPrefix(path, "../") ||
		strings.HasPrefix(path, "/") {
		return path
	}

	slashPath := strings.ReplaceAll(path, ".", "/")
	for _, prefix := range []string{"std/", "core/", "internal/"} {
		if strings.HasPrefix(slashPath, prefix) {
			rest := slashPath[len(prefix):]
			if rest != "" {
				return rest
			}
		}
	}
	return slashPath
}

func forceLocalImport(node *ast.Node) bool {
	if node == nil {
		return false
	}
	if s, ok := node.Prop.(string); ok {
		return strings.EqualFold(s, "force-local") || strings.EqualFold(s, "fimport")
	}
	return false
}

func (interp *Interpreter) loadModule(path string) (*Value, error) {
	resolved := resolveModulePath(path)
	if resolved == "native" && interp.libLoadDepth == 0 {
		e := interp.runtimeError(errfmt.KindImport, "E0014",
			fmt.Sprintf("module %q is internal and cannot be imported by user code — use a standard lib module like @import(\"std.io\")", path), nil, nil)
		return nil, e
	}

	interp.mu.RLock()
	if mod, ok := interp.modules[resolved]; ok {
		interp.mu.RUnlock()
		return mod, nil
	}
	interp.mu.RUnlock()

	if interp.ntlFileLoader != nil {
		src, realPath, ok := interp.ntlFileLoader(resolved)
		if ok && realPath != "" {
			abs := realPath
			if !filepath.IsAbs(abs) {
				abs, _ = filepath.Abs(realPath)
			}

			interp.mu.RLock()
			if mod, ok2 := interp.modules[abs]; ok2 {
				interp.mu.RUnlock()
				return mod, nil
			}
			interp.mu.RUnlock()
			return interp.evalModuleSourceFile(src, abs, abs)
		}
	}

	if localPath, ok := interp.resolveLocalFile(path); ok {
		abs, _ := filepath.Abs(localPath)
		interp.mu.RLock()
		if mod, ok := interp.modules[abs]; ok {
			interp.mu.RUnlock()
			return mod, nil
		}
		interp.mu.RUnlock()
		return interp.loadLocalFile(localPath, nil)
	}

	if interp.moduleLoader != nil {
		if mod, ok := interp.moduleLoader(resolved); ok {
			if mod != nil {
				interp.mu.Lock()
				interp.modules[resolved] = mod
				interp.mu.Unlock()
			}
			return mod, nil
		}
	}

	if interp.astLoader != nil {
		if tree, realPath, ok := interp.astLoader(resolved); ok {
			if realPath == "" {
				realPath = resolved
			}
			return interp.evalModuleAST(tree, realPath)
		}
	}
	if interp.ntlLoader != nil {
		src, ok := interp.ntlLoader(resolved)
		if ok {
			return interp.evalModuleSourceFile(src, resolved, resolved)
		}
	}

	e := interp.runtimeError(errfmt.KindImport, "E0010",
		fmt.Sprintf("module %q not found", path), nil, nil)
	return nil, e
}

func (interp *Interpreter) resolveLocalFile(path string) (string, bool) {
	var candidates []string

	bases := []string{path}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".lx" && ext != ".nax" {
		bases = append(bases, path+".lx", path+".nax")
	}

	if interp.filename != "" {
		dir := filepath.Dir(interp.filename)
		for _, b := range bases {
			candidates = append(candidates,
				filepath.Join(dir, b),
				filepath.Join(dir, filepath.FromSlash(b)),
			)
		}
	}

	wd, _ := os.Getwd()
	for _, b := range bases {
		candidates = append(candidates, filepath.Join(wd, b))
	}

	candidates = append(candidates, bases...)

	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, true
		}
	}
	return "", false
}

func (interp *Interpreter) evalModuleAST(prog *ast.Node, name string) (*Value, error) {
	if prog == nil {
		return nil, fmt.Errorf("module %q has no compiled AST", name)
	}
	prevFilename := interp.filename
	prevLines := interp.sourceLines
	interp.filename = name
	interp.sourceLines = nil
	defer func() {
		interp.filename = prevFilename
		interp.sourceLines = prevLines
	}()
	interp.libLoadDepth++
	modEnv := NewEnvironment(interp.globals)
	_, execErr := interp.execBlock(prog.Body_, modEnv)
	interp.libLoadDepth--
	if execErr != nil {
		if _, ok := execErr.(*returnError); !ok {
			return nil, interp.runtimeError(errfmt.KindImport, "E0013", fmt.Sprintf("error while executing module '%s': %v", name, execErr), nil, nil)
		}
	}
	mod, ok := modEnv.GetLocal("__module__")
	if !ok {
		exports := make(map[string]*Value)
		for k, v := range modEnv.Snapshot() {
			if len(k) == 0 || k[0] == '_' {
				continue
			}
			exports[k] = v
		}
		mod = ObjectVal(exports)
	}
	interp.mu.Lock()
	interp.modules[name] = mod
	interp.mu.Unlock()
	return mod, nil
}

func (interp *Interpreter) evalModuleSourceFile(src, cacheKey, displayPath string) (*Value, error) {

	prevFilename := interp.filename
	prevLines := interp.sourceLines
	prevLine := interp.currentLine
	prevCol := interp.currentCol
	interp.filename = cacheKey
	defer func() {
		interp.filename = prevFilename
		interp.sourceLines = prevLines

		interp.currentLine = prevLine
		interp.currentCol = prevCol
	}()

	return interp.evalModuleSource(src, cacheKey)
}

func (interp *Interpreter) evalModuleSource(src, name string) (*Value, error) {

	displayName := name
	if !strings.HasSuffix(name, ".lx") {
		displayName = name + ".lx"
	}
	lines := strings.Split(src, "\n")

	prevLines := interp.sourceLines
	interp.sourceLines = lines
	defer func() { interp.sourceLines = prevLines }()

	toks, err := lexer.Tokenize(src, displayName)
	if err != nil {
		return nil, interp.runtimeError(errfmt.KindImport, "E0011",
			fmt.Sprintf("failed to tokenize module '%s': %v", name, err), nil, nil)
	}
	prog, err := parser.ParseWithLines(toks, displayName, lines)
	if err != nil {

		return nil, interp.runtimeError(errfmt.KindImport, "E0011",
			fmt.Sprintf("failed to parse module '%s': %v", name, err), nil, nil)
	}
	resolver.Resolve(prog)
	interp.libLoadDepth++
	modEnv := NewEnvironment(interp.globals)
	_, execErr := interp.execBlock(prog.Body_, modEnv)
	interp.libLoadDepth--
	if execErr != nil {
		if _, ok := execErr.(*returnError); !ok {
			return nil, interp.runtimeError(errfmt.KindImport, "E0013",
				fmt.Sprintf("error while executing module '%s': %v", name, execErr), nil, nil)
		}
	}
	mod, ok := modEnv.GetLocal("__module__")
	if !ok {
		exports := make(map[string]*Value)
		for k, v := range modEnv.Snapshot() {
			if len(k) == 0 || k[0] == '_' {
				continue
			}
			exports[k] = v
		}
		mod = ObjectVal(exports)
	}
	interp.mu.Lock()
	interp.modules[name] = mod
	interp.mu.Unlock()
	return mod, nil
}

func (interp *Interpreter) execExport(node *ast.Node, env *Environment) (*Value, error) {
	if node.Declaration != nil {
		return interp.execNode(node.Declaration, env)
	}
	if node.Value != nil {
		expr, ok := node.Value.(*ast.Node)
		if !ok || expr == nil {
			return nil, interp.runtimeError(errfmt.KindImport, "E0432", "invalid default export", node, nil)
		}
		value, err := interp.evalExpr(expr, env)
		if err != nil {
			return nil, err
		}
		if moduleValue, ok := env.GetLocal("__module__"); ok && moduleValue != nil && moduleValue.Tag == TypeObject {
			moduleValue.ObjVal["default"] = value
			return Undefined, nil
		}
		env.Define("__module__", ObjectVal(map[string]*Value{"default": value}), false)
		return Undefined, nil
	}
	if len(node.Specifiers) == 0 {
		return Undefined, nil
	}
	moduleValue, ok := env.GetLocal("__module__")
	if !ok || moduleValue == nil || moduleValue.Tag != TypeObject {
		moduleValue = ObjectVal(map[string]*Value{})
		env.Define("__module__", moduleValue, false)
	}
	for _, spec := range node.Specifiers {
		if spec == nil || spec.Imported == "" || spec.Exported == "" {
			continue
		}
		value, exists := env.Get(spec.Imported)
		if !exists {
			return nil, errfmt.UnresolvedExportError(spec.Imported, interp.filename, node.Line, node.Col, interp.sourceLines)
		}
		moduleValue.ObjVal[spec.Exported] = value
	}
	return Undefined, nil
}

func (interp *Interpreter) execUse(node *ast.Node, env *Environment) (*Value, error) {

	modName := ""
	if len(node.Modules) > 0 {
		modName = node.Modules[0]
	}
	suggestion := "std." + modName
	if modName == "native" {
		suggestion = "internal.native"
	}
	return nil, interp.runtimeError(errfmt.KindImport, "E0014",
		fmt.Sprintf("'use %s' is no longer valid — replace with: val %s = @import(%q)", modName, modName, suggestion), node, nil)
}

func (interp *Interpreter) execLunexRequire(node *ast.Node, env *Environment) (*Value, error) {
	for _, mod := range node.Modules {
		modVal, err := interp.loadModule(mod)
		if err != nil {
			return nil, err
		}
		env.Define(mod, modVal, true)
	}
	return Undefined, nil
}

func (interp *Interpreter) execImmutable(node *ast.Node, env *Environment) (*Value, error) {
	return interp.execNode(node.Body, env)
}

func (interp *Interpreter) execUsing(node *ast.Node, env *Environment) (*Value, error) {
	val, err := interp.evalExpr(node.Init, env)
	if err != nil {
		return nil, err
	}
	env.Define(node.Name, val, false)
	return Undefined, nil
}
