package checker

import (
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/errfmt"
	"lunex/internal/lexer"
	"lunex/internal/parser"
	"path/filepath"
	"strings"
)

type Loader func(path, from string) (source, realPath string, ok bool)

type Diagnostic = errfmt.LunexError

type Result struct {
	Diagnostics []Diagnostic
	Files       int
	Imports     int
	Symbols     int
}

type Options struct {
	Loader           Loader
	AllowMissingMain bool
	CheckStdMembers  bool
	Warnings         bool
	Debug            func(format string, args ...any)
	Verbose          func(format string, args ...any)
	KnownModules     map[string]map[string]struct{}
}

type Checker struct {
	opts       Options
	result     Result
	modules    map[string]*moduleInfo
	loading    map[string]bool
	cycleStack []string
}

type moduleInfo struct {
	path           string
	source         string
	lines          []string
	root           *ast.Node
	exports        map[string]symbol
	symbols        map[string]symbol
	analyzed       bool
	stdModule      bool
	rootExecutable bool
}

type symbolKind int

const (
	symUnknown symbolKind = iota
	symVar
	symConst
	symFunc
	symClass
	symModule
	symParam
	symBuiltin
	symType
)

type symbol struct {
	name     string
	kind     symbolKind
	line     int
	col      int
	arity    int
	minArity int
	variadic bool
	module   *moduleInfo
	exports  map[string]symbol
	mutable  bool
}

type scope struct {
	parent  *scope
	symbols map[string]symbol
	kind    string
}

type state struct {
	file          *moduleInfo
	scope         *scope
	functionDepth int
	loopDepth     int
	classDepth    int
	allowTopLevel bool
	knownModule   *moduleInfo
	reported      map[string]bool
}

var builtinNames = map[string]bool{
	"undefined": true, "null": true, "true": true, "false": true, "NaN": true, "Infinity": true,
	"lunex": true, "parseInt": true, "parseFloat": true, "isNaN": true, "isFinite": true,
	"String": true, "str": true, "Number": true, "num": true, "Boolean": true,
	"Array": true, "Object": true, "Math": true, "JSON": true, "Promise": true, "Error": true, "TypeError": true,
	"RangeError": true, "Map": true, "Set": true, "setTimeout": true, "setInterval": true, "clearTimeout": true,
	"clearInterval": true, "performance": true, "process": true, "encodeURIComponent": true, "decodeURIComponent": true,
	"encodeURI": true, "decodeURI": true, "btoa": true, "atob": true, "typeof": true, "this": true,
}

var stdModules = map[string]bool{
	"io": true, "fs": true, "http": true, "crypto": true, "db": true, "env": true, "testing": true, "ws": true,
	"utils": true, "json": true, "jwt": true, "math": true, "datetime": true, "os": true, "regex": true,
	"buffer": true, "ints": true, "runtime": true, "ffi": true, "internal.native": true,
	"http/router": true, "http/static": true,
}

func New(opts Options) *Checker {
	return &Checker{opts: opts, modules: make(map[string]*moduleInfo), loading: make(map[string]bool)}
}

func (c *Checker) Check(source, filename string) Result {
	c.result = Result{}
	abs, err := filepath.Abs(filename)
	if err == nil {
		filename = abs
	}
	m := c.loadSource(source, filename, false)
	if m != nil {
		m.rootExecutable = true
	}
	if m != nil {
		c.analyze(m)
	}
	return c.result
}

func (c *Checker) loadSource(source, filename string, imported bool) *moduleInfo {
	key, err := filepath.Abs(filename)
	if err == nil {
		filename = key
	}
	if prev, ok := c.modules[filename]; ok {
		if imported {
			c.result.Imports++
		}
		return prev
	}
	if c.loading[filename] {
		e := errfmt.CircularImportError(filename, filename, 1)
		c.addNode(*e)
		return nil
	}
	lines := strings.Split(source, "\n")
	m := &moduleInfo{path: filename, source: source, lines: lines, exports: make(map[string]symbol), symbols: make(map[string]symbol)}
	c.modules[filename] = m
	c.loading[filename] = true
	c.result.Files++
	if c.opts.Debug != nil {
		c.opts.Debug("loading module %s", filename)
	}
	toks, err := lexer.Tokenize(source, filename)
	if err != nil {
		c.addFromLunex(errfmt.LunexError{Message: err.Error(), File: filename, Kind: errfmt.KindLex, Lines: lines})
		delete(c.loading, filename)
		return m
	}
	tree, err := parser.ParseWithLines(toks, filename, lines)
	if err != nil {
		if e, ok := err.(*errfmt.LunexError); ok {
			c.addFromLunex(*e)
		} else {
			c.addFromLunex(errfmt.LunexError{Message: err.Error(), File: filename, Kind: errfmt.KindParse, Lines: lines})
		}
		delete(c.loading, filename)
		return m
	}
	m.root = tree
	m.stdModule = isStdPath(filename)
	c.collectTopLevel(m)
	delete(c.loading, filename)
	if imported && c.opts.Debug != nil {
		c.opts.Debug("resolved import %s -> %s", filename, filename)
	}
	return m
}

func (c *Checker) resolveImport(path, from string) *moduleInfo {
	resolved := resolveModule(path)
	if isStdModule(resolved) {
		key := "<std:" + resolved + ">"
		if m, ok := c.modules[key]; ok {
			return m
		}
		m := &moduleInfo{path: key, exports: make(map[string]symbol), symbols: make(map[string]symbol), stdModule: true}
		if c.opts.KnownModules != nil {
			if members, ok := c.opts.KnownModules[resolved]; ok {
				for name := range members {
					m.exports[name] = symbol{name: name, kind: symUnknown}
				}
			}
		}
		c.modules[key] = m
		return m
	}
	if c.opts.Loader == nil {
		e := errfmt.ImportError(path, from, 1)
		c.addNode(*e)
		return nil
	}
	src, real, ok := c.opts.Loader(path, from)
	if !ok || strings.TrimSpace(real) == "" {
		e := errfmt.ImportError(path, from, 1)
		c.addNode(*e)
		return nil
	}
	return c.loadSource(src, real, true)
}

func (c *Checker) collectTopLevel(m *moduleInfo) {
	if m == nil || m.root == nil {
		return
	}
	for _, n := range m.root.Body_ {
		c.collectDecl(m, n)
	}
	for _, n := range m.root.Body_ {
		if n == nil {
			continue
		}
		if n.Type == ast.ExportDecl {
			if n.Declaration != nil {
				c.collectDecl(m, n.Declaration)
				if n.Declaration.Name != "" {
					if s, ok := m.symbols[n.Declaration.Name]; ok {
						m.exports[n.Declaration.Name] = s
					}
				}
			} else {
				if valueNode(n.Value) != nil {
					m.exports["default"] = symbol{name: "default", kind: symUnknown, line: n.Line, col: n.Col}
				}
				for _, spec := range n.Specifiers {
					if spec == nil {
						continue
					}
					if s, ok := m.symbols[spec.Imported]; ok {
						name := spec.Exported
						if name == "" {
							name = spec.Imported
						}
						m.exports[name] = s
					}
				}
			}
		}
	}
	for name, s := range m.symbols {
		if _, ok := m.exports[name]; !ok && m.isExportedByDefault() && !strings.HasPrefix(name, "_") {
			m.exports[name] = s
		}
	}
}

func (m *moduleInfo) isExportedByDefault() bool {
	return len(m.exports) == 0
}

func (c *Checker) collectDecl(m *moduleInfo, n *ast.Node) {
	if n == nil {
		return
	}
	name := n.Name
	if n.Type == ast.ExportDecl {
		if n.Declaration != nil {
			c.collectDecl(m, n.Declaration)
		}
		return
	}
	if name == "" {
		return
	}
	kind := symUnknown
	mutable := true
	arity := 0
	minArity := 0
	variadic := false
	switch n.Type {
	case ast.VarDecl:
		kind = symVar
		mutable = !n.IsConst
	case ast.FnDecl, ast.ComponentDecl:
		kind = symFunc
		arity = len(n.Params)
		for _, p := range n.Params {
			if p == nil {
				continue
			}
			if p.Optional || p.DefaultVal != nil {
				continue
			}
			minArity++
			if p.Rest {
				variadic = true
			}
		}
	case ast.ClassDecl, ast.EnumDecl, ast.NamespaceDecl:
		kind = symClass
	default:
		return
	}
	if _, exists := m.symbols[name]; exists {
		return
	}
	m.symbols[name] = symbol{name: name, kind: kind, line: n.Line, col: n.Col, arity: arity, minArity: minArity, variadic: variadic, mutable: mutable}
}

func (c *Checker) analyze(m *moduleInfo) {
	if m == nil || m.analyzed || m.root == nil {
		return
	}
	m.analyzed = true
	rootScope := &scope{symbols: make(map[string]symbol), kind: "module"}
	for name := range builtinNames {
		rootScope.symbols[name] = symbol{name: name, kind: symBuiltin, mutable: true}
	}
	for name, s := range m.symbols {
		rootScope.symbols[name] = s
	}
	st := &state{file: m, scope: rootScope, allowTopLevel: false, reported: make(map[string]bool)}
	if c.opts.Debug != nil {
		c.opts.Debug("analyzing %s: %d top-level symbols", m.path, len(m.symbols))
	}
	for _, n := range m.root.Body_ {
		if m.rootExecutable {
			if e := c.topLevelErrorWithFile(n, m.path, m.lines); e != nil {
				c.addNode(*e)
				continue
			}
		}
		c.checkNode(n, st)
	}
	c.result.Symbols += len(m.symbols)
	if m.rootExecutable && !m.stdModule && !c.opts.AllowMissingMain {
		if _, ok := rootScope.symbols["main"]; !ok {
			e := errfmt.MissingMainError(m.path, m.lines)
			c.addNode(*e)
		}
	}
}

func (c *Checker) topLevelErrorWithFile(n *ast.Node, file string, lines []string) *errfmt.LunexError {
	if n == nil {
		return nil
	}
	switch n.Type {
	case ast.FnDecl, ast.ClassDecl, ast.EnumDecl, ast.NamespaceDecl,
		ast.ComponentDecl, ast.ExportDecl, ast.ImportDecl, ast.LunexRequire,
		ast.UseStmt, ast.ImmutableDecl, ast.UsingDecl, ast.VarDecl:
		return nil
	case ast.ExprStmt:
		expr := n.Expr
		if expr == nil {
			return nil
		}
		switch expr.Type {
		case ast.CallExpr:
			calleeName := ""
			if expr.Callee != nil && expr.Callee.Type == ast.Identifier {
				calleeName = expr.Callee.Name
			}
			if calleeName == "main" {
				return errfmt.ExplicitMainCallError(file, n.Line, n.Col, lines)
			}
			return errfmt.TopLevelCallError(calleeName, file, n.Line, n.Col, lines)
		case ast.AssignExpr:
			return errfmt.TopLevelAssignmentError(file, n.Line, n.Col, lines)
		default:
			return errfmt.TopLevelStatementError(string(expr.Type), file, n.Line, n.Col, lines)
		}
	default:
		return errfmt.TopLevelStatementError(string(n.Type), file, n.Line, n.Col, lines)
	}
}

func (c *Checker) checkNode(n *ast.Node, st *state) {
	if n == nil {
		return
	}
	switch n.Type {
	case ast.Program, ast.Block:
		child := st
		if n.Type == ast.Block {
			child = &state{file: st.file, scope: &scope{parent: st.scope, symbols: make(map[string]symbol), kind: "block"}, functionDepth: st.functionDepth, loopDepth: st.loopDepth, classDepth: st.classDepth, allowTopLevel: st.allowTopLevel, knownModule: st.knownModule, reported: st.reported}
		}
		for _, x := range n.Body_ {
			c.checkNode(x, child)
		}
	case ast.VarDecl:
		module := c.inferModule(n.Init, st)
		c.checkNode(n.Init, st)
		if n.Name != "" {
			s := symbol{name: n.Name, kind: symVar, line: n.Line, col: n.Col, mutable: !n.IsConst}
			if module != nil {
				s.kind = symModule
				s.module = module
				s.exports = module.exports
			}
			st.scope.symbols[n.Name] = s
		}
	case ast.FnDecl, ast.ComponentDecl:
		if n.Name != "" && st.scope.kind != "module" {
			st.scope.symbols[n.Name] = symbol{name: n.Name, kind: symFunc, line: n.Line, col: n.Col, arity: len(n.Params), mutable: false}
		}
		fnScope := &scope{parent: st.scope, symbols: make(map[string]symbol), kind: "function"}
		for _, p := range n.Params {
			if p == nil || p.Name == "" {
				continue
			}
			fnScope.symbols[p.Name] = symbol{name: p.Name, kind: symParam, line: n.Line, col: n.Col, mutable: true}
			c.checkNode(p.DefaultVal, &state{file: st.file, scope: fnScope, functionDepth: st.functionDepth + 1, loopDepth: st.loopDepth, classDepth: st.classDepth, reported: st.reported})
		}
		if n.Body != nil {
			c.checkNode(n.Body, &state{file: st.file, scope: fnScope, functionDepth: st.functionDepth + 1, loopDepth: st.loopDepth, classDepth: st.classDepth, reported: st.reported})
		}
	case ast.ImportDecl:
		c.checkImportDecl(n, st)
	case ast.ExportDecl:
		if n.Declaration != nil {
			c.checkNode(n.Declaration, st)
		}
		c.checkNode(valueNode(n.Value), st)
		for _, sp := range n.Specifiers {
			if sp == nil {
				continue
			}
			if _, ok := c.lookup(st.scope, sp.Imported); !ok {
				e := errfmt.UnresolvedExportError(sp.Imported, st.file.path, n.Line, n.Col, st.file.lines)
				c.addNode(*e)
			}
		}
	case ast.LunexRequire:
		for _, mod := range n.Modules {
			c.resolveImport(mod, st.file.path)
		}
	case ast.ExprStmt:
		c.checkNode(n.Expr, st)
	case ast.Identifier:
		if !c.isLabelIdentifier(n.Name) {
			if _, ok := c.lookup(st.scope, n.Name); !ok && n.Name != "this" {
				e := errfmt.ReferenceErrorWithSimilar(n.Name, st.file.path, n.Line, n.Col, st.file.lines, c.similarNames(n.Name, st.scope))
				if len(e.Similar) > 0 {
					e.Notes = append(e.Notes, fmt.Sprintf("did you mean `%s`? (closest match by name)", e.Similar[0]))
				}
				c.addNode(*e)
			}
		}
	case ast.CallExpr:
		c.checkCall(n, st)
	case ast.MemberExpr:
		c.checkMember(n, st)
	case ast.AssignExpr:
		c.checkAssignment(n, st)
	case ast.ReturnStmt:
		c.checkNode(n.Expr, st)
		c.checkNode(valueNode(n.Value), st)
	case ast.BreakStmt, ast.ContinueStmt:
		return
	case ast.IfStmt, ast.UnlessStmt, ast.IfHaveStmt, ast.IfSetStmt:
		c.checkNode(n.Test, st)
		c.checkNode(n.Subject, st)
		c.checkNode(n.Consequent, st)
		c.checkNode(n.Alternate, st)
	case ast.WhileStmt, ast.ForStmt, ast.ForOfStmt, ast.EachInStmt, ast.RepeatStmt, ast.LoopStmt:
		c.checkNode(n.Test, st)
		c.checkNode(n.Init, st)
		c.checkNode(n.Left, st)
		c.checkNode(n.Right, st)
		c.checkNode(n.Expr, st)
		c.checkNode(n.Count, st)
		loopState := *st
		loopState.loopDepth++
		loopState.scope = &scope{parent: st.scope, symbols: make(map[string]symbol), kind: "loop"}
		if n.Name != "" {
			loopState.scope.symbols[n.Name] = symbol{name: n.Name, kind: symVar, line: n.Line, col: n.Col, mutable: !n.IsConst}
		}
		if n.Alias != "" {
			loopState.scope.symbols[n.Alias] = symbol{name: n.Alias, kind: symVar, line: n.Line, col: n.Col, mutable: true}
		}
		c.checkNode(n.Body, &loopState)
	case ast.MatchStmt:
		c.checkNode(n.Subject, st)
		for _, mc := range n.Cases {
			if mc == nil {
				continue
			}
			caseScope := &scope{parent: st.scope, symbols: make(map[string]symbol), kind: "match"}
			caseState := *st
			caseState.scope = caseScope
			for _, pattern := range mc.Patterns {
				c.bindMatchPattern(pattern, caseScope, st)
			}
			c.checkNode(mc.Guard, &caseState)
			c.checkNode(mc.Body, &caseState)
		}
	case ast.TryStmt:
		c.checkNode(n.Body, st)
		c.checkNode(n.CatchBlock, st)
		c.checkNode(n.FinallyBlock, st)
		if n.CatchParam != "" {
			if _, ok := c.lookup(st.scope, n.CatchParam); !ok {
				st.scope.symbols[n.CatchParam] = symbol{name: n.CatchParam, kind: symVar, line: n.Line, col: n.Col, mutable: true}
			}
		}
	case ast.SpawnStmt, ast.AssertStmt, ast.HaveStmt, ast.WithStmt, ast.SelectStmt, ast.GuardStmt, ast.DeferStmt, ast.ThrowStmt, ast.RaiseStmt, ast.DeleteStmt:
		c.checkNode(n.Expr, st)
		c.checkNode(valueNode(n.Value), st)
		c.checkNode(n.Body, st)
		c.checkNode(n.Guard, st)
		c.checkNode(n.CatchBlock, st)
		c.checkNode(n.FinallyBlock, st)
		for _, sc := range n.SelectCases {
			if sc != nil {
				c.checkNode(sc.Channel, st)
				c.checkNode(sc.Body, st)
			}
		}
	case ast.ClassDecl, ast.EnumDecl, ast.NamespaceDecl:
		c.checkDeclarationMembers(n, st)
	case ast.FnExpr, ast.ArrowFn:
		fnScope := &scope{parent: st.scope, symbols: make(map[string]symbol), kind: "function"}
		for _, p := range n.Params {
			if p != nil && p.Name != "" {
				fnScope.symbols[p.Name] = symbol{name: p.Name, kind: symParam, line: n.Line, col: n.Col, mutable: true}
			}
		}
		fnState := *st
		fnState.scope = fnScope
		fnState.functionDepth++
		for _, p := range n.Params {
			if p != nil {
				c.checkNode(p.DefaultVal, &fnState)
			}
		}
		c.checkNode(n.Body, &fnState)
	case ast.TypeofExpr, ast.DeleteExpr, ast.NotExpr, ast.UnaryExpr, ast.SleepExpr, ast.ChannelExpr, ast.SpreadExpr, ast.TernaryExpr, ast.LogicalExpr, ast.BinaryExpr, ast.PipelineExpr, ast.SequenceExpr, ast.RangeExpr, ast.StructLit, ast.ObjectLit, ast.ArrayLit, ast.NewExpr, ast.NaxImportExpr, ast.AtImportExpr, ast.DecoratedExpr, ast.SatisfiesExpr, ast.HaveExpr, ast.TrySafeExpr:
		c.checkExprChildren(n, st)
	default:
		c.checkExprChildren(n, st)
	}
}

func (c *Checker) inferModule(n *ast.Node, st *state) *moduleInfo {
	if n == nil {
		return nil
	}
	if n.Type == ast.AtImportExpr || n.Type == ast.NaxImportExpr {
		return c.resolveImport(n.Source, st.file.path)
	}
	return nil
}

func (c *Checker) checkImportDecl(n *ast.Node, st *state) {
	mod := c.resolveImport(n.Source, st.file.path)
	if mod == nil {
		return
	}
	if n.Namespace != "" {
		st.scope.symbols[n.Namespace] = symbol{name: n.Namespace, kind: symModule, line: n.Line, col: n.Col, module: mod, exports: mod.exports, mutable: false}
	}
	if n.DefaultImport != "" {
		if len(n.Specifiers) == 0 {
			st.scope.symbols[n.DefaultImport] = symbol{name: n.DefaultImport, kind: symModule, line: n.Line, col: n.Col, module: mod, exports: mod.exports, mutable: false}
		} else {
			if s, ok := mod.exports["default"]; ok {
				st.scope.symbols[n.DefaultImport] = s
			} else {
				st.scope.symbols[n.DefaultImport] = symbol{name: n.DefaultImport, kind: symUnknown, line: n.Line, col: n.Col, mutable: false}
			}
		}
	}
	for _, sp := range n.Specifiers {
		if sp == nil {
			continue
		}
		s, ok := mod.exports[sp.Imported]
		if !ok && !mod.stdModule {
			s = symbol{name: sp.Local, kind: symUnknown, line: n.Line, col: n.Col}
		}
		if sp.Local == "" {
			sp.Local = sp.Imported
		}
		s.name = sp.Local
		st.scope.symbols[sp.Local] = s
	}
	c.analyze(mod)
}

func (c *Checker) checkCall(n *ast.Node, st *state) {
	if n.Callee != nil {
		c.checkNode(n.Callee, st)
	}
	for _, a := range n.Args {
		c.checkNode(a, st)
	}
}

func (c *Checker) checkMember(n *ast.Node, st *state) {
	c.checkNode(n.Object, st)
	if n.Computed {
		c.checkNode(valueNode(n.Prop), st)
	}
}

func (c *Checker) checkAssignment(n *ast.Node, st *state) {
	if n.Left == nil {
		return
	}
	if n.Left.Type == ast.Identifier {
		s, ok := c.lookup(st.scope, n.Left.Name)
		if !ok {
			e := errfmt.ReferenceError(n.Left.Name, st.file.path, n.Left.Line, n.Left.Col, st.file.lines)
			c.addNode(*e)
		} else if !s.mutable {
			e := errfmt.ConstReassignError(n.Left.Name, st.file.path, n.Left.Line, n.Left.Col, st.file.lines)
			c.addNode(*e)
		}
	} else {
		c.checkNode(n.Left, st)
	}
	c.checkNode(n.Right, st)
}

func (c *Checker) checkDeclarationMembers(n *ast.Node, st *state) {
	for _, m := range n.Methods {
		if m == nil {
			continue
		}
		fnScope := &scope{parent: st.scope, symbols: make(map[string]symbol), kind: "method"}
		fnScope.symbols["this"] = symbol{name: "this", kind: symParam, mutable: false}
		for _, p := range m.Params {
			if p != nil && p.Name != "" {
				fnScope.symbols[p.Name] = symbol{name: p.Name, kind: symParam, mutable: true}
			}
		}
		c.checkNode(m.Init, &state{file: st.file, scope: fnScope, functionDepth: st.functionDepth + 1, classDepth: st.classDepth + 1, reported: st.reported})
		c.checkNode(m.Body, &state{file: st.file, scope: fnScope, functionDepth: st.functionDepth + 1, classDepth: st.classDepth + 1, reported: st.reported})
	}
	c.checkNode(n.Body, st)
	c.checkNode(n.Extends, st)
}

func (c *Checker) checkExprChildren(n *ast.Node, st *state) {
	if n == nil {
		return
	}
	c.checkNode(n.Body, st)
	c.checkNode(n.Init, st)
	c.checkNode(n.Test, st)
	c.checkNode(n.Alternate, st)
	c.checkNode(n.Consequent, st)
	c.checkNode(n.Left, st)
	c.checkNode(n.Right, st)
	c.checkNode(n.Object, st)
	c.checkNode(n.Callee, st)
	c.checkNode(n.Arg, st)
	c.checkNode(n.Expr, st)
	c.checkNode(n.Stmt, st)
	c.checkNode(n.Subject, st)
	c.checkNode(n.Lo, st)
	c.checkNode(n.Hi, st)
	c.checkNode(n.Count, st)
	c.checkNode(n.Ms, st)
	c.checkNode(n.Channel, st)
	c.checkNode(n.Guard, st)
	c.checkNode(n.Declaration, st)
	c.checkNode(n.Extends, st)
	for _, a := range n.Args {
		c.checkNode(a, st)
	}
	for _, e := range n.Elements {
		c.checkNode(e, st)
	}
	for _, e := range n.Exprs {
		c.checkNode(e, st)
	}
	for _, p := range n.Properties {
		if p != nil {
			c.checkNode(p.Value, st)
			c.checkNode(p.Arg, st)
			c.checkNode(p.Body, st)
			for _, param := range p.Params {
				if param != nil {
					c.checkNode(param.DefaultVal, st)
				}
			}
		}
	}
	if n.Type == ast.AtImportExpr || n.Type == ast.NaxImportExpr {
		if strings.TrimSpace(n.Source) == "" {
			e := errfmt.ImportError(n.Source, st.file.path, n.Line)
			e.Col = n.Col
			e.Lines = st.file.lines
			c.addNode(*e)
		} else {
			mod := c.resolveImport(n.Source, st.file.path)
			c.analyze(mod)
		}
	}
}

func (c *Checker) bindMatchPattern(p *ast.MatchPattern, scope *scope, st *state) {
	if p == nil {
		return
	}
	switch p.Kind {
	case "binding", "rest":
		if p.Name != "" {
			scope.symbols[p.Name] = symbol{name: p.Name, kind: symVar, mutable: true}
		}
	case "array", "variant":
		for _, child := range p.Items {
			c.bindMatchPattern(child, scope, st)
		}
		for _, child := range p.Fields {
			c.bindMatchPattern(child, scope, st)
		}
	case "object":
		for _, prop := range p.Props {
			if prop != nil && prop.Alias != "" {
				scope.symbols[prop.Alias] = symbol{name: prop.Alias, kind: symVar, mutable: true}
			}
		}
	}
}

func (c *Checker) lookup(s *scope, name string) (symbol, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if sym, ok := cur.symbols[name]; ok {
			return sym, true
		}
	}
	return symbol{}, false
}

func (c *Checker) similarNames(name string, s *scope) []string {
	var names []string
	seen := make(map[string]bool)
	for cur := s; cur != nil; cur = cur.parent {
		for n := range cur.symbols {
			if !seen[n] {
				names = append(names, n)
				seen[n] = true
			}
		}
	}
	return errfmt.FindSimilar(name, names)
}

func (c *Checker) isLabelIdentifier(name string) bool {
	return strings.HasPrefix(name, "__")
}

func (c *Checker) add(d Diagnostic) {
	key := fmt.Sprintf("%s:%d:%d:%s:%s", d.File, d.Line, d.Col, d.Code, d.Message)
	for _, existing := range c.result.Diagnostics {
		if fmt.Sprintf("%s:%d:%d:%s:%s", existing.File, existing.Line, existing.Col, existing.Code, existing.Message) == key {
			return
		}
	}
	c.result.Diagnostics = append(c.result.Diagnostics, d)
}

func (c *Checker) addNode(d Diagnostic) {
	if d.Line <= 0 {
		d.Line = 1
	}
	if d.Col <= 0 {
		d.Col = 1
	}
	if d.File == "" {
		d.File = "<unknown>"
	}
	for _, m := range c.modules {
		if m.path == d.File {
			d.Lines = m.lines
			break
		}
	}
	c.add(d)
}

func (c *Checker) addFromLunex(e errfmt.LunexError) {
	c.add(e)
}

func resolveModule(path string) string {
	if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") || strings.HasPrefix(path, "/") || strings.HasSuffix(path, ".lx") || strings.HasSuffix(path, ".nax") {
		return path
	}
	p := strings.ReplaceAll(path, ".", "/")
	for _, prefix := range []string{"std/", "core/"} {
		if strings.HasPrefix(p, prefix) {
			return p[len(prefix):]
		}
	}
	return p
}

func isStdModule(path string) bool {
	if stdModules[path] {
		return true
	}
	p := strings.TrimPrefix(path, "std/")
	p = strings.TrimSuffix(p, ".lx")
	p = strings.TrimSuffix(p, ".nax")
	return stdModules[p]
}

func isStdPath(path string) bool {
	return strings.HasPrefix(path, "<std:")
}

func valueNode(v interface{}) *ast.Node {
	n, _ := v.(*ast.Node)
	return n
}

func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
