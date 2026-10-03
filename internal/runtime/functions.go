package runtime

import (
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/errfmt"
	"lunex/internal/resolver"
	"strings"
	"time"
)

const (
	pseudoThisName       = "this"
	pseudoSuperClassName = "__super_class__"
)

func pseudoBindingSlot(bodyNode *ast.Node, name string) int {
	if bodyNode == nil || bodyNode.ScopeInfo == nil {
		return -1
	}
	for i, n := range bodyNode.ScopeInfo.Names {
		if n == name {
			return i
		}
	}
	return -1
}

func (interp *Interpreter) evalFnExpr(node *ast.Node, env *Environment) (*Value, error) {
	MarkEscaped(env)
	fn := &Function{
		Name:        node.Name,
		Params:      paramsToFnParams(node.Params),
		Body:        node.Body,
		Env:         env,
		SourceFile:  interp.filename,
		SourceLines: interp.sourceLines,
	}
	return FuncVal(fn), nil
}

func (interp *Interpreter) evalArrowFn(node *ast.Node, env *Environment) (*Value, error) {
	MarkEscaped(env)

	capturedThis, _ := env.Get("this")
	fn := &Function{
		Name:         "",
		Params:       paramsToFnParams(node.Params),
		Body:         node.Body,
		Env:          env,
		IsArrow:      true,
		CapturedThis: capturedThis,
		SourceFile:   interp.filename,
		SourceLines:  interp.sourceLines,
	}
	return FuncVal(fn), nil
}

func (interp *Interpreter) evalCall(node *ast.Node, env *Environment) (*Value, error) {
	var thisVal *Value = Undefined
	var fnVal *Value

	if node.Callee.Type == ast.MemberExpr {
		if node.Callee.Object != nil && node.Callee.Object.Type == ast.SuperExpr {
			superCls, _ := env.Get("__super_class__")
			if superCls != nil && superCls.Tag == TypeClass {
				key, _ := node.Callee.Prop.(string)
				if method, ok := superCls.ClsVal.Methods[key]; ok {
					thisVal, _ := env.Get("this")
					superArgs, err := interp.evalArgs(node.Args, env)
					if err != nil {
						return nil, err
					}
					return interp.callFunctionValue(FuncVal(method), superArgs, thisVal)
				}
			}
			return Undefined, nil
		}
		obj, err := interp.evalExpr(node.Callee.Object, env)
		if err != nil {
			return nil, err
		}
		if node.Optional && obj.IsNullish() {
			return Undefined, nil
		}
		thisVal = obj
		if node.Callee.Computed {
			k, err := interp.evalExpr(node.Callee.Prop.(*ast.Node), env)
			if err != nil {
				return nil, err
			}

			if obj.Tag == TypeArray && k.Tag == TypeNumber {
				fnVal = obj.GetIndex(int(k.NumVal))
			} else {
				fnVal = obj.Get(k.ToString())
			}
		} else {
			key, _ := node.Callee.Prop.(string)
			fnVal = obj.Get(key)

			if (fnVal == nil || fnVal.Tag == TypeUndefined) && obj.Tag == TypeChannel {
				ch := obj.ChanVal
				switch key {
				case "send":
					fnVal = FuncVal(&Function{Name: "send", Native: func(args []*Value, _ *Value) (*Value, error) {
						if len(args) > 0 {
							ch.Send(args[0])
						}
						return Undefined, nil
					}})
				case "recv":
					fnVal = FuncVal(&Function{Name: "recv", Native: func(args []*Value, _ *Value) (*Value, error) {
						return ch.Receive(), nil
					}})
				}
			}

			if fnVal == nil || fnVal.Tag == TypeUndefined {
				similar := errfmt.FindSimilar(key, objKeys(obj))
				objName := ""
				if node.Callee.Object != nil && node.Callee.Object.Type == ast.Identifier {
					objName = node.Callee.Object.Name
				}
				msg := fmt.Sprintf("method `%s` does not exist", key)
				if objName != "" {
					msg = fmt.Sprintf("method `%s` does not exist on `%s`", key, objName)
				}
				e := interp.runtimeError(errfmt.KindType, "E0002", msg, node, similar)
				avail := objKeys(obj)
				if len(avail) > 0 && len(avail) <= 12 {
					e.Notes = append(e.Notes, "available: "+strings.Join(avail, ", "))
				}
				return nil, e
			}
		}
	} else if node.Callee.Type == ast.SuperExpr {
		superCls, _ := env.Get("__super_class__")
		if superCls != nil && superCls.Tag == TypeClass {
			superArgs, err := interp.evalArgs(node.Args, env)
			if err != nil {
				return nil, err
			}
			childThis, hasThis := env.Get("this")
			if hasThis && childThis != nil && childThis.Tag == TypeInstance {
				return interp.runConstructorWithThis(superCls.ClsVal, superArgs, childThis, env)
			}
			return interp.callClass(superCls.ClsVal, superArgs, env)
		}
		return Undefined, nil
	} else {
		v, err := interp.evalExpr(node.Callee, env)
		if err != nil {
			return nil, err
		}
		fnVal = v
	}

	if node.Optional && fnVal.IsNullish() {
		return Undefined, nil
	}

	args, err := interp.evalArgs(node.Args, env)
	if err != nil {
		return nil, err
	}

	return interp.callFunctionValue(fnVal, args, thisVal)
}

func (interp *Interpreter) evalArgs(argNodes []*ast.Node, env *Environment) ([]*Value, error) {
	var args []*Value
	for _, argNode := range argNodes {
		if argNode.Type == ast.SpreadExpr {
			val, err := interp.evalExpr(argNode.Arg, env)
			if err != nil {
				return nil, err
			}
			if val.Tag == TypeArray {
				args = append(args, val.ArrVal...)
			} else {
				args = append(args, val)
			}
		} else {
			val, err := interp.evalExpr(argNode, env)
			if err != nil {
				return nil, err
			}
			args = append(args, val)
		}
	}
	return args, nil
}

func (interp *Interpreter) callFunctionValue(fnVal *Value, args []*Value, thisVal *Value) (*Value, error) {
	if fnVal == nil || fnVal.Tag == TypeNull || fnVal.Tag == TypeUndefined {
		typeName := "undefined"
		if fnVal != nil {
			typeName = fnVal.TypeName()
		}
		e := interp.runtimeError(errfmt.KindType, "E0003",
			fmt.Sprintf("value of type `%s` is not callable", typeName), nil, nil)
		e.Notes = append(e.Notes, "only values declared with `fn` can be called")
		return nil, e
	}
	if fnVal.Tag == TypeClass {
		return interp.callClass(fnVal.ClsVal, args, nil)
	}
	if fnVal.Tag != TypeFunction {
		e := interp.runtimeError(errfmt.KindType, "E0003",
			fmt.Sprintf("value of type `%s` is not callable (expected a function)", fnVal.TypeName()), nil, nil)
		e.Notes = append(e.Notes, fmt.Sprintf("the value is: %s", fnVal.ToString()))
		return nil, e
	}
	fn := fnVal.FnVal
	if fn.Native != nil {
		result, err := fn.Native(args, thisVal)
		return result, err
	}
	return interp.callUserFunction(fn, args, thisVal)
}

func (interp *Interpreter) callUserFunction(fn *Function, args []*Value, thisVal *Value) (*Value, error) {
	const maxCallDepth = 1000
	interp.callDepth++
	if interp.callDepth > maxCallDepth {
		interp.callDepth--
		fnName := fn.Name
		if fnName == "" {
			fnName = "<anonymous>"
		}
		return nil, interp.runtimeError(errfmt.KindRecursion, errfmt.ErrStackOverflow,
			"maximum call depth exceeded (infinite recursion in '"+fnName+"')",
			nil, []string{"check for a function that calls itself without a base case"})
	}
	defer func() { interp.callDepth-- }()

	if fn.SourceFile != "" {
		prevFile := interp.filename
		prevLines := interp.sourceLines
		interp.filename = fn.SourceFile
		interp.sourceLines = fn.SourceLines
		defer func() {
			interp.filename = prevFile
			interp.sourceLines = prevLines
		}()
	}

	var fnProf *FnProfile
	var t0 int64
	if fn.Name != "" {
		fnProf = interp.profiler.GetOrCreate(fn.Name)
		if fnProf.ShouldSample() {
			t0 = time.Now().UnixNano()
		}
	}

	savedDefers := interp.defers
	interp.defers = nil

	bodyNode, _ := fn.Body.(*ast.Node)
	if bodyNode == nil {
		interp.defers = savedDefers
		return Undefined, nil
	}

	fnEnv := NewResolvedEnvironment(fn.Env, resolver.SlotCount(bodyNode.ScopeInfo))

	defer ReleaseEnvironment(fnEnv)

	effectiveThis := thisVal
	if fn.IsArrow && fn.CapturedThis != nil {
		effectiveThis = fn.CapturedThis
	}

	if effectiveThis != nil {
		fnEnv.DefineSlot(0, pseudoBindingSlot(bodyNode, pseudoThisName), "this", effectiveThis, false)
	}
	if fn.DefClass != nil && fn.DefClass.Super != nil {
		fnEnv.DefineSlot(0, pseudoBindingSlot(bodyNode, pseudoSuperClassName), "__super_class__", ClassVal(fn.DefClass.Super), false)
	}
	err := interp.bindParams(fn.Params, args, fnEnv)
	if err != nil {
		interp.defers = savedDefers
		return nil, err
	}
	var result *Value = Undefined
	var execErr error
	if bodyNode.Type == ast.Block {
		stmts := bodyNode.Body_

		for _, stmt := range stmts {
			if stmt != nil && stmt.Type == ast.FnDecl && stmt.Name != "" {
				if _, already := fnEnv.GetLocal(stmt.Name); !already {
					MarkEscaped(fnEnv)
					fn := &Function{
						Name:        stmt.Name,
						Params:      paramsToFnParams(stmt.Params),
						Body:        stmt.Body,
						Env:         fnEnv,
						SourceFile:  interp.filename,
						SourceLines: interp.sourceLines,
					}
					fnEnv.DefineSlot(0, pseudoBindingSlot(bodyNode, stmt.Name), stmt.Name, FuncVal(fn), false)
				}
			}
		}
		for i, stmt := range stmts {
			val, e := interp.execNode(stmt, fnEnv)
			if e != nil {
				if re, ok := e.(*returnError); ok {
					result = re.val
					break
				}
				execErr = e
				break
			}
			if i == len(stmts)-1 && (stmt.Type == ast.ExprStmt || stmt.Type == ast.MatchStmt ||
				stmt.Type == ast.IfStmt || stmt.Type == ast.UnlessStmt ||
				stmt.Type == ast.TryStmt || stmt.Type == ast.Block ||
				stmt.Type == ast.FnExpr || stmt.Type == ast.FnDecl) {
				result = val
			}
		}
	} else {
		result, execErr = interp.evalExpr(bodyNode, fnEnv)
	}

	localDefers := interp.defers
	interp.defers = savedDefers
	for i := len(localDefers) - 1; i >= 0; i-- {
		de := localDefers[i]
		if de.node.Body != nil {
			interp.execNode(de.node.Body, de.env)
		} else if de.node.Expr != nil {
			interp.evalExpr(de.node.Expr, de.env)
		}
	}

	if fnProf != nil && t0 != 0 {
		elapsed := time.Now().UnixNano() - t0
		fnProf.Record(elapsed)
	}

	if execErr != nil {
		if re, ok := execErr.(*returnError); ok {
			return re.val, nil
		}
		return nil, execErr
	}
	return result, nil
}

func (interp *Interpreter) bindParams(params []FnParam, args []*Value, env *Environment) error {
	for i, param := range params {
		if param.Rest {
			var rest []*Value
			if i < len(args) {
				rest = args[i:]
			}
			if param.ResolvedSlot >= 0 {
				env.DefineSlot(0, param.ResolvedSlot, param.Name, ArrayVal(rest), false)
			} else {
				env.Define(param.Name, ArrayVal(rest), false)
			}
			break
		}
		var val *Value
		if i < len(args) {
			val = args[i]
		} else {
			if param.Default != nil {
				defNode, ok := param.Default.(*ast.Node)
				if ok {
					var err error
					val, err = interp.evalExpr(defNode, env)
					if err != nil {
						return err
					}
				}
			}
			if val == nil {
				val = Undefined
			}
		}
		if param.Destructure != nil {
			if err := interp.bindDestructure(param.Destructure, val, env); err != nil {
				return err
			}
		} else if param.ResolvedSlot >= 0 {
			env.DefineSlot(0, param.ResolvedSlot, param.Name, val, false)
		} else {
			env.Define(param.Name, val, false)
		}
	}
	return nil
}

func (interp *Interpreter) bindDestructure(pattern interface{}, val *Value, env *Environment) error {
	m, ok := pattern.(map[string]interface{})
	if !ok {
		return nil
	}
	kind, _ := m["kind"].(string)
	switch kind {
	case "object":
		props, _ := m["props"].([]map[string]interface{})
		for _, prop := range props {
			key, _ := prop["key"].(string)
			alias, _ := prop["alias"].(string)
			if alias == "" {
				alias = key
			}
			fieldVal := val.Get(key)
			if fieldVal.IsNullish() {
				if defNode, ok := prop["default"]; ok && defNode != nil {
					if dn, ok := defNode.(*ast.Node); ok {
						v, err := interp.evalExpr(dn, env)
						if err != nil {
							return err
						}
						fieldVal = v
					}
				}
			}
			env.Define(alias, fieldVal, false)
		}
	case "array":
		items, _ := m["items"].([]interface{})
		for i, item := range items {
			if item == nil {
				continue
			}
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if itemMap["kind"] == "rest" {
				name, _ := itemMap["name"].(string)
				var rest []*Value
				if val.Tag == TypeArray && i < len(val.ArrVal) {
					rest = val.ArrVal[i:]
				}
				env.Define(name, ArrayVal(rest), false)
				break
			}
			name, _ := itemMap["name"].(string)
			var fieldVal *Value
			if val.Tag == TypeArray && i < len(val.ArrVal) {
				fieldVal = val.ArrVal[i]
			}
			if fieldVal == nil || fieldVal.IsNullish() {
				if defNode, ok := itemMap["default"]; ok && defNode != nil {
					if dn, ok := defNode.(*ast.Node); ok {
						v, err := interp.evalExpr(dn, env)
						if err != nil {
							return err
						}
						fieldVal = v
					}
				}
			}
			if fieldVal == nil {
				fieldVal = Undefined
			}
			env.Define(name, fieldVal, false)
		}
	}
	return nil
}
