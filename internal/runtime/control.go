package runtime

import (
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/errfmt"
	"lunex/internal/resolver"
)

func (interp *Interpreter) execIf(node *ast.Node, env *Environment) (*Value, error) {
	test, err := interp.evalExpr(node.Test, env)
	if err != nil {
		return nil, err
	}
	if test.IsTruthy() {
		return interp.execNode(node.Consequent, env)
	}
	if node.Alternate != nil {
		return interp.execNode(node.Alternate, env)
	}
	return Undefined, nil
}

func (interp *Interpreter) execUnless(node *ast.Node, env *Environment) (*Value, error) {
	test, err := interp.evalExpr(node.Test, env)
	if err != nil {
		return nil, err
	}
	if !test.IsTruthy() {
		return interp.execNode(node.Consequent, env)
	}
	if node.Alternate != nil {
		return interp.execNode(node.Alternate, env)
	}
	return Undefined, nil
}

func (interp *Interpreter) execWhile(node *ast.Node, env *Environment) (*Value, error) {
	jitDisabled := false
	for {
		if !jitDisabled {
			if handled, deopt, err := interp.runJIT(node, env, -1); err != nil {
				return nil, err
			} else if handled {
				return Undefined, nil
			} else if deopt {
				jitDisabled = true
			}
		}
		test, err := interp.evalExpr(node.Test, env)
		if err != nil {
			return nil, err
		}
		if !test.IsTruthy() {
			break
		}
		_, err = interp.execNode(node.Body, env)
		if err != nil {
			if _, ok := err.(*breakError); ok {
				break
			}
			if _, ok := err.(*continueError); ok {
				continue
			}
			return nil, err
		}
	}
	return Undefined, nil
}

func (interp *Interpreter) execForOf(node *ast.Node, env *Environment) (*Value, error) {
	if node.Right != nil && node.Right.Type == ast.RangeExpr && node.Destructure == nil {
		return interp.execRangeLoop(node, env)
	}
	iterVal, err := interp.evalExpr(node.Right, env)
	if err != nil {
		return nil, err
	}
	idx := 0
	iterLoop := func(val *Value) error {
		iterEnv := NewResolvedEnvironment(env, resolver.SlotCount(node.ScopeInfo))
		if node.Destructure != nil {
			if err := interp.bindDestructure(node.Destructure, val, iterEnv); err != nil {
				return err
			}
		} else {
			iterEnv.DefineSlot(0, resolver.SlotIndex(node.ScopeInfo, node.Name), node.Name, val, node.IsConst)
		}
		if node.Alias != "" {
			iterEnv.DefineSlot(0, resolver.SlotIndex(node.ScopeInfo, node.Alias), node.Alias, NumberVal(float64(idx)), node.IsConst)
		}
		idx++
		_, err := interp.execNode(node.Body, iterEnv)
		return err
	}
	if v, err := interp.CheckForOfIterable(iterVal, node); err != nil {
		errfmt.Print(err.(*errfmt.LunexError))
		return v, nil
	}
	switch iterVal.Tag {
	case TypeArray:
		for _, el := range iterVal.ArrVal {
			if el == nil {
				el = Undefined
			}
			if err := iterLoop(el); err != nil {
				if _, ok := err.(*breakError); ok {
					return Undefined, nil
				}
				if _, ok := err.(*continueError); ok {
					continue
				}
				return nil, err
			}
		}
	case TypeString:
		for _, r := range iterVal.StrVal {
			if err := iterLoop(StringVal(string(r))); err != nil {
				if _, ok := err.(*breakError); ok {
					return Undefined, nil
				}
				if _, ok := err.(*continueError); ok {
					continue
				}
				return nil, err
			}
		}
	case TypeObject:
		for k := range iterVal.ObjVal {
			if err := iterLoop(StringVal(k)); err != nil {
				if _, ok := err.(*breakError); ok {
					return Undefined, nil
				}
				if _, ok := err.(*continueError); ok {
					continue
				}
				return nil, err
			}
		}
	}
	return Undefined, nil
}

func (interp *Interpreter) execFor(node *ast.Node, env *Environment) (*Value, error) {
	if node.Body == nil && node.Init == nil {
		return Undefined, nil
	}

	forEnv := NewResolvedEnvironment(env, resolver.SlotCount(node.ScopeInfo))

	if node.Init != nil {
		if _, err := interp.execNode(node.Init, forEnv); err != nil {
			return nil, err
		}
	}

	jitDisabled := false
	for {
		if !jitDisabled {
			if handled, deopt, err := interp.runJIT(node, forEnv, -1); err != nil {
				return nil, err
			} else if handled {
				return Undefined, nil
			} else if deopt {
				jitDisabled = true
			}
		}
		if node.Test != nil {
			test, err := interp.evalExpr(node.Test, forEnv)
			if err != nil {
				return nil, err
			}
			if !test.IsTruthy() {
				break
			}
		}

		if node.Body != nil {
			if _, err := interp.execNode(node.Body, forEnv); err != nil {
				if _, ok := err.(*breakError); ok {
					break
				}
				if _, ok := err.(*continueError); ok {
				} else {
					return nil, err
				}
			}
		}

		if node.Right != nil {
			if _, err := interp.evalExpr(node.Right, forEnv); err != nil {
				return nil, err
			}
		}
	}
	return Undefined, nil
}

func (interp *Interpreter) execRepeat(node *ast.Node, env *Environment) (*Value, error) {
	count := -1
	if node.Count != nil {
		n, err := interp.evalExpr(node.Count, env)
		if err != nil {
			return nil, err
		}
		count = int(n.ToNumber())
	}
	jitDisabled := false
	for i := 0; count < 0 || i < count; i++ {
		if !jitDisabled {
			remaining := int64(-1)
			if count >= 0 {
				remaining = int64(count - i)
			}
			if handled, deopt, err := interp.runJIT(node, env, remaining); err != nil {
				return nil, err
			} else if handled {
				return Undefined, nil
			} else if deopt {
				jitDisabled = true
			}
		}
		_, err := interp.execNode(node.Body, env)
		if err != nil {
			if _, ok := err.(*breakError); ok {
				break
			}
			if _, ok := err.(*continueError); ok {
				continue
			}
			return nil, err
		}
	}
	return Undefined, nil
}

func (interp *Interpreter) execLoop(node *ast.Node, env *Environment) (*Value, error) {
	jitDisabled := false
	for {
		if !jitDisabled {
			if handled, deopt, err := interp.runJIT(node, env, -1); err != nil {
				return nil, err
			} else if handled {
				return Undefined, nil
			} else if deopt {
				jitDisabled = true
			}
		}
		_, err := interp.execNode(node.Body, env)
		if err != nil {
			if _, ok := err.(*breakError); ok {
				break
			}
			if _, ok := err.(*continueError); ok {
				continue
			}
			return nil, err
		}
	}
	return Undefined, nil
}

func (interp *Interpreter) execMatch(node *ast.Node, env *Environment) (*Value, error) {
	return interp.evalMatchExpr(node, env)
}

func (interp *Interpreter) execTry(node *ast.Node, env *Environment) (*Value, error) {
	tryResult, err := interp.execNode(node.Body, env)
	if tryResult == nil {
		tryResult = Undefined
	}
	result := tryResult
	if err != nil {
		if te, ok := err.(*throwError); ok {
			if node.CatchBlock != nil {
				catchEnv := NewResolvedEnvironment(env, resolver.SlotCount(node.CatchBlock.ScopeInfo))
				if node.CatchParam != "" {
					catchEnv.DefineSlot(0, resolver.SlotIndex(node.CatchBlock.ScopeInfo, node.CatchParam), node.CatchParam, te.val, false)
				}
				catchResult, catchErr := interp.execNode(node.CatchBlock, catchEnv)
				if catchErr != nil {

					if node.FinallyBlock != nil {
						interp.execNode(node.FinallyBlock, env)
					}
					return nil, catchErr
				}
				if catchResult != nil {
					result = catchResult
				}
			}
		} else if re, ok := err.(*returnError); ok {
			if node.FinallyBlock != nil {
				interp.execNode(node.FinallyBlock, env)
			}
			return nil, re
		} else {
			if node.CatchBlock != nil {
				catchEnv := NewResolvedEnvironment(env, resolver.SlotCount(node.CatchBlock.ScopeInfo))
				if node.CatchParam != "" {
					errMsg := err.Error()
					errObj := ObjectVal(map[string]*Value{
						"message": StringVal(errMsg),
						"name":    StringVal("Error"),
						"stack":   StringVal("Error: " + errMsg),
					})
					if le, ok := err.(*errfmt.LunexError); ok {
						errObj.ObjVal["code"] = StringVal(le.Code)
						errObj.ObjVal["kind"] = StringVal(string(le.Kind))
						if le.Phase != "" {
							errObj.ObjVal["phase"] = StringVal(le.Phase)
						}
					}
					catchEnv.DefineSlot(0, resolver.SlotIndex(node.CatchBlock.ScopeInfo, node.CatchParam), node.CatchParam, errObj, false)
				}
				catchResult, _ := interp.execNode(node.CatchBlock, catchEnv)
				if catchResult != nil {
					result = catchResult
				}
			}
		}
	}
	if node.FinallyBlock != nil {
		interp.execNode(node.FinallyBlock, env)
	}
	return result, nil
}

func (interp *Interpreter) execGuard(node *ast.Node, env *Environment) (*Value, error) {
	test, err := interp.evalExpr(node.Test, env)
	if err != nil {
		return nil, err
	}
	if !test.IsTruthy() {
		return interp.execNode(node.Alternate, env)
	}
	return Undefined, nil
}

func (interp *Interpreter) execAssert(node *ast.Node, env *Environment) (*Value, error) {
	test, err := interp.evalExpr(node.Test, env)
	if err != nil {
		return nil, err
	}
	if !test.IsTruthy() {
		msg := "Assertion failed"
		if node.Arg != nil {
			msgVal, err := interp.evalExpr(node.Arg, env)
			if err == nil {
				msg = msgVal.ToString()
			}
		}
		return nil, &throwError{val: ObjectVal(map[string]*Value{
			"message": StringVal(msg),
		})}
	}
	return Undefined, nil
}

func (interp *Interpreter) execHave(node *ast.Node, env *Environment) (*Value, error) {
	val, err := interp.evalExpr(node.Expr, env)
	if err != nil {
		return nil, err
	}
	cond := interp.testHaveCondition(val, node, env)
	if node.IsGuard {
		if !cond {
			if node.Alternate != nil {
				return interp.execNode(node.Alternate, env)
			}
			return nil, &returnError{val: Undefined}
		}
		return Undefined, nil
	}
	haveEnv := NewResolvedEnvironment(env, resolver.SlotCount(node.ScopeInfo))
	if node.Alias != "" {
		haveEnv.DefineSlot(0, resolver.SlotIndex(node.ScopeInfo, node.Alias), node.Alias, val, false)
	}
	if cond {
		if node.Consequent != nil {
			return interp.execNode(node.Consequent, haveEnv)
		}
	} else {
		if node.Alternate != nil {
			return interp.execNode(node.Alternate, env)
		}
	}
	return Undefined, nil
}

func (interp *Interpreter) execIfHave(node *ast.Node, env *Environment) (*Value, error) {
	val, err := interp.evalExpr(node.Expr, env)
	if err != nil {
		return nil, err
	}
	cond := interp.testHaveCondition(val, node, env)
	ifEnv := NewResolvedEnvironment(env, resolver.SlotCount(node.ScopeInfo))
	if node.Alias != "" {
		ifEnv.DefineSlot(0, resolver.SlotIndex(node.ScopeInfo, node.Alias), node.Alias, val, false)
	}
	if cond {
		return interp.execNode(node.Consequent, ifEnv)
	}
	if node.Alternate != nil {
		return interp.execNode(node.Alternate, env)
	}
	return Undefined, nil
}

func (interp *Interpreter) execIfSet(node *ast.Node, env *Environment) (*Value, error) {
	val, err := interp.evalExpr(node.Expr, env)
	if err != nil {
		return nil, err
	}
	ifEnv := NewEnvironment(env)
	alias := node.Alias
	if alias == "" {
		alias = fmt.Sprintf("_ifset_%d", node.ID)
	}
	ifEnv.Define(alias, val, false)
	if !val.IsNullish() {
		return interp.execNode(node.Consequent, ifEnv)
	}
	if node.Alternate != nil {
		return interp.execNode(node.Alternate, env)
	}
	return Undefined, nil
}

func (interp *Interpreter) execDelete(node *ast.Node, env *Environment) (*Value, error) {
	if node.Expr.Type == ast.MemberExpr {
		obj, err := interp.evalExpr(node.Expr.Object, env)
		if err != nil {
			return nil, err
		}
		var key string
		if node.Expr.Computed {
			k, err := interp.evalExpr(node.Expr.Prop.(*ast.Node), env)
			if err != nil {
				return nil, err
			}
			key = k.ToString()
		} else {
			key, _ = node.Expr.Prop.(string)
		}
		if obj.Tag == TypeObject {
			delete(obj.ObjVal, key)
		}
	}
	return True, nil
}

func (interp *Interpreter) execWith(node *ast.Node, env *Environment) (*Value, error) {
	val, err := interp.evalExpr(node.Expr, env)
	if err != nil {
		return nil, err
	}
	withEnv := NewEnvironment(env)
	if val.Tag == TypeObject {
		for k, v := range val.ObjVal {
			withEnv.Define(k, v, false)
		}
	}
	return interp.execNode(node.Body, withEnv)
}

func (interp *Interpreter) execComponent(node *ast.Node, env *Environment) (*Value, error) {
	fn := &Function{
		Name:   node.Name,
		Params: paramsToFnParams(node.Params),
		Body:   node.Body,
		Env:    env,
	}
	fnVal := FuncVal(fn)
	if node.Name != "" {
		env.Define(node.Name, fnVal, false)
	}
	return fnVal, nil
}

func (interp *Interpreter) execSelect(node *ast.Node, env *Environment) (*Value, error) {
	type result struct {
		idx int
		val *Value
	}
	ch := make(chan result, len(node.SelectCases))
	for i, sc := range node.SelectCases {
		i, sc := i, sc
		go func() {
			chanVal, err := interp.evalExpr(sc.Channel, env)
			if err != nil {
				ch <- result{idx: i, val: Null}
				return
			}
			var val *Value
			if chanVal.Tag == TypeChannel {
				val = chanVal.ChanVal.Receive()
			} else {
				val = chanVal
			}
			ch <- result{idx: i, val: val}
		}()
	}
	r := <-ch
	if r.idx < len(node.SelectCases) {
		sc := node.SelectCases[r.idx]
		caseEnv := NewEnvironment(env)
		if sc.Binding != "" {
			caseEnv.Define(sc.Binding, r.val, false)
		}
		interp.execNode(sc.Body, caseEnv)
	}
	return Undefined, nil
}
