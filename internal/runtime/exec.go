package runtime

import (
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/errfmt"
	"lunex/internal/resolver"
)

func (interp *Interpreter) Exec(program *ast.Node) (*Value, error) {
	return interp.execProgram(program, true)
}

func (interp *Interpreter) ExecTrusted(program *ast.Node) (*Value, error) {
	return interp.execProgram(program, false)
}

func (interp *Interpreter) execProgram(program *ast.Node, validateTopLevel bool) (*Value, error) {
	interp.resetExecutionBudget()
	env := NewEnvironment(interp.globals)
	interp.topEnv = env
	for _, stmt := range program.Body_ {
		if stmt != nil && stmt.Type == ast.ClassDecl && stmt.Name != "" {
			if _, already := env.Get(stmt.Name); !already {
				stub := &Class{
					Name:          stmt.Name,
					Methods:       make(map[string]*Function),
					StaticMethods: make(map[string]*Function),
					Env:           env,
				}
				env.Define(stmt.Name, ClassVal(stub), false)
			}
		}
	}

	if validateTopLevel {
		for _, stmt := range program.Body_ {
			if stmt == nil {
				continue
			}
			if err := interp.checkTopLevelStatement(stmt); err != nil {
				return nil, err
			}
		}
	}

	return interp.execBlock(program.Body_, env)
}

func (interp *Interpreter) checkTopLevelStatement(stmt *ast.Node) *errfmt.LunexError {
	switch stmt.Type {
	case ast.FnDecl, ast.ClassDecl, ast.EnumDecl, ast.NamespaceDecl,
		ast.ComponentDecl, ast.ExportDecl, ast.ImportDecl,
		ast.LunexRequire, ast.UseStmt, ast.ImmutableDecl, ast.UsingDecl:
		return nil

	case ast.VarDecl:
		return nil

	case ast.ExprStmt:
		expr := stmt.Expr
		if expr == nil {
			return nil
		}
		return interp.checkTopLevelExpr(expr, stmt)

	default:
		e := errfmt.TopLevelStatementError(string(stmt.Type), interp.filename, stmt.Line, stmt.Col, interp.sourceLines)
		return e
	}
}

func (interp *Interpreter) checkTopLevelExpr(expr *ast.Node, stmt *ast.Node) *errfmt.LunexError {
	if expr == nil {
		return nil
	}
	switch expr.Type {
	case ast.CallExpr:
		callee := expr.Callee
		calleeName := ""
		if callee != nil && callee.Type == ast.Identifier {
			calleeName = callee.Name
		}

		if calleeName == "main" {
			return errfmt.ExplicitMainCallError(interp.filename, stmt.Line, stmt.Col, interp.sourceLines)
		}

		return errfmt.TopLevelCallError(calleeName, interp.filename, stmt.Line, stmt.Col, interp.sourceLines)

	case ast.AssignExpr:
		return errfmt.TopLevelAssignmentError(interp.filename, stmt.Line, stmt.Col, interp.sourceLines)
	}
	return errfmt.TopLevelStatementError(string(expr.Type), interp.filename, stmt.Line, stmt.Col, interp.sourceLines)
}

func (interp *Interpreter) ExecASTAsModule(tree *ast.Node, filename string) (*Value, error) {
	interp.resetExecutionBudget()
	return interp.evalModuleAST(tree, filename)
}

func (interp *Interpreter) ExecAsModule(source, filename string) (*Value, error) {
	interp.resetExecutionBudget()
	prevFilename := interp.filename
	interp.filename = filename
	defer func() { interp.filename = prevFilename }()
	return interp.evalModuleSource(source, filename)
}

func (interp *Interpreter) CallMain() error {
	if interp.topEnv == nil {
		return nil
	}
	mainVal, ok := interp.topEnv.Get("main")
	if !ok || mainVal == nil {
		return errfmt.MissingMainError(interp.filename, interp.sourceLines)
	}
	_, err := interp.callFunctionValue(mainVal, []*Value{}, nil)
	if err != nil {
		return err
	}
	return nil
}

func (interp *Interpreter) CallExport(name string, args ...interface{}) (interface{}, error) {
	interp.resetExecutionBudget()
	if interp.globals == nil {
		return nil, fmt.Errorf("interpreter not initialized")
	}
	fnVal, ok := interp.globals.Get(name)
	if !ok {
		return nil, fmt.Errorf("export %q not found", name)
	}
	lxArgs := make([]*Value, len(args))
	for i, a := range args {
		lxArgs[i] = goToValue(a)
	}
	result, err := interp.callFunctionValue(fnVal, lxArgs, nil)
	if err != nil {
		return nil, err
	}
	return valueToGo(result), nil
}

func goToValue(v interface{}) *Value {
	if v == nil {
		return Null
	}
	switch val := v.(type) {
	case bool:
		return BoolVal(val)
	case int:
		return NumberVal(float64(val))
	case int64:
		return NumberVal(float64(val))
	case float64:
		return NumberVal(val)
	case string:
		return StringVal(val)
	case []interface{}:
		arr := make([]*Value, len(val))
		for i, elem := range val {
			arr[i] = goToValue(elem)
		}
		return ArrayVal(arr)
	case map[string]interface{}:
		m := make(map[string]*Value, len(val))
		for k, elem := range val {
			m[k] = goToValue(elem)
		}
		return ObjectVal(m)
	default:
		return Null
	}
}

func valueToGo(v *Value) interface{} {
	if v == nil {
		return nil
	}
	switch v.Tag {
	case TypeNull, TypeUndefined:
		return nil
	case TypeBool:
		return v.BoolVal
	case TypeNumber:
		return v.NumVal
	case TypeString:
		return v.StrVal
	case TypeArray:
		out := make([]interface{}, len(v.ArrVal))
		for i, elem := range v.ArrVal {
			out[i] = valueToGo(elem)
		}
		return out
	case TypeObject:
		m := make(map[string]interface{}, len(v.ObjVal))
		for k, elem := range v.ObjVal {
			m[k] = valueToGo(elem)
		}
		return m
	default:
		return nil
	}
}

func blockNeedsOwnScope(node *ast.Node) bool {
	if node.NeedsScopeCache != nil {
		return *node.NeedsScopeCache
	}
	needs := false
	for _, stmt := range node.Body_ {
		if stmt == nil {
			continue
		}
		switch stmt.Type {
		case ast.VarDecl, ast.ImmutableDecl, ast.UsingDecl,
			ast.FnDecl, ast.ClassDecl, ast.EnumDecl, ast.NamespaceDecl,
			ast.ComponentDecl:
			needs = true
		}
		if needs {
			break
		}
	}
	node.NeedsScopeCache = &needs
	return needs
}

func (interp *Interpreter) execBlock(stmts []*ast.Node, env *Environment) (*Value, error) {
	var result *Value = Undefined
	for _, stmt := range stmts {
		val, err := interp.execNode(stmt, env)
		if err != nil {
			return nil, err
		}
		if val != nil {
			result = val
		}
	}
	return result, nil
}

func (interp *Interpreter) execNode(node *ast.Node, env *Environment) (*Value, error) {
	if err := interp.consumeExecutionBudget(node); err != nil {
		return nil, err
	}
	if node == nil {
		return Undefined, nil
	}
	if node.Line > 0 {
		interp.currentLine = node.Line
		interp.currentCol = node.Col
	}
	switch node.Type {
	case ast.Program:
		return interp.execBlock(node.Body_, env)
	case ast.Block:
		if !blockNeedsOwnScope(node) {

			return interp.execBlock(node.Body_, env)
		}
		childEnv := NewResolvedEnvironment(env, resolver.SlotCount(node.ScopeInfo))
		result, err := interp.execBlock(node.Body_, childEnv)
		ReleaseEnvironment(childEnv)
		return result, err
	case ast.VarDecl:
		return interp.execVarDecl(node, env)
	case ast.FnDecl:
		return interp.execFnDecl(node, env)
	case ast.ClassDecl:
		return interp.execClassDecl(node, env)
	case ast.EnumDecl:
		return interp.execEnumDecl(node, env)
	case ast.NamespaceDecl:
		return interp.execNamespace(node, env)
	case ast.ImportDecl:
		return interp.execImport(node, env)
	case ast.ExportDecl:
		return interp.execExport(node, env)
	case ast.LunexRequire:
		return interp.execLunexRequire(node, env)
	case ast.UseStmt:
		return interp.execUse(node, env)
	case ast.ImmutableDecl:
		return interp.execImmutable(node, env)
	case ast.UsingDecl:
		return interp.execUsing(node, env)
	case ast.ExprStmt:
		return interp.evalExpr(node.Expr, env)
	case ast.ReturnStmt:
		var val *Value = Undefined
		var err error
		if node.Value != nil {
			val, err = interp.evalExpr(node.Value.(*ast.Node), env)
			if err != nil {
				return nil, err
			}
		}
		return nil, &returnError{val: val}
	case ast.ThrowStmt, ast.RaiseStmt:
		val, err := interp.evalExpr(node.Value.(*ast.Node), env)
		if err != nil {
			return nil, err
		}
		return nil, &throwError{val: val}
	case ast.BreakStmt:
		return nil, &breakError{}
	case ast.ContinueStmt:
		return nil, &continueError{}
	case ast.IfStmt:
		return interp.execIf(node, env)
	case ast.UnlessStmt:
		return interp.execUnless(node, env)
	case ast.WhileStmt:
		return interp.execWhile(node, env)
	case ast.ForStmt:
		return interp.execFor(node, env)
	case ast.ForOfStmt, ast.EachInStmt:
		return interp.execForOf(node, env)
	case ast.RepeatStmt:
		return interp.execRepeat(node, env)
	case ast.LoopStmt:
		return interp.execLoop(node, env)
	case ast.MatchStmt:
		return interp.execMatch(node, env)
	case ast.TryStmt:
		return interp.execTry(node, env)
	case ast.GuardStmt:
		return interp.execGuard(node, env)
	case ast.DeferStmt:
		interp.defers = append(interp.defers, deferEntry{node: node, env: env})
		return Undefined, nil
	case ast.SpawnStmt:
		MarkEscaped(env)
		go func() {
			defer func() { recover() }()
			interp.evalExpr(node.Expr, env)
		}()
		return Undefined, nil
	case ast.AssertStmt:
		return interp.execAssert(node, env)
	case ast.HaveStmt:
		return interp.execHave(node, env)
	case ast.IfHaveStmt:
		return interp.execIfHave(node, env)
	case ast.IfSetStmt:
		return interp.execIfSet(node, env)
	case ast.DeleteStmt:
		return interp.execDelete(node, env)
	case ast.WithStmt:
		return interp.execWith(node, env)
	case ast.ComponentDecl:
		return interp.execComponent(node, env)
	case ast.SelectStmt:
		return interp.execSelect(node, env)
	case ast.DecoratedExpr:
		if node.Expr != nil {
			return interp.execNode(node.Expr, env)
		}
		return Undefined, nil
	default:
		return interp.evalExpr(node, env)
	}
}
