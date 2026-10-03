package runtime

import (
	"lunex/internal/ast"
)

func (interp *Interpreter) execVarDecl(node *ast.Node, env *Environment) (*Value, error) {
	var val *Value = Undefined
	if node.Init != nil {
		var err error
		val, err = interp.evalExpr(node.Init, env)
		if err != nil {
			return nil, err
		}
	}
	if node.Destructure != nil {
		return Undefined, interp.bindDestructure(node.Destructure, val, env)
	}
	if addr := node.ResolvedAddr; addr != nil {

		env.DefineSlot(addr.Hops, addr.Slot, node.Name, val, node.IsConst)
		return Undefined, nil
	}
	env.Define(node.Name, val, node.IsConst)
	return Undefined, nil
}

func (interp *Interpreter) execFnDecl(node *ast.Node, env *Environment) (*Value, error) {
	MarkEscaped(env)
	fn := &Function{
		Name:        node.Name,
		Params:      paramsToFnParams(node.Params),
		Body:        node.Body,
		Env:         env,
		SourceFile:  interp.filename,
		SourceLines: interp.sourceLines,
	}
	fnVal := FuncVal(fn)
	if node.Name != "" {
		env.Define(node.Name, fnVal, false)
	}
	return fnVal, nil
}

func (interp *Interpreter) execClassDecl(node *ast.Node, env *Environment) (*Value, error) {
	MarkEscaped(env)
	cls := &Class{
		Name:          node.Name,
		Methods:       make(map[string]*Function),
		StaticMethods: make(map[string]*Function),
		Env:           env,
	}
	if node.SuperClass != nil {
		superVal, err := interp.evalExpr(node.SuperClass, env)
		if err != nil {
			return nil, err
		}
		if superVal.Tag == TypeClass {
			cls.Super = superVal.ClsVal
		}
	}
	for _, member := range node.Methods {
		fn := &Function{
			Name:        member.Name,
			Params:      paramsToFnParams(member.Params),
			Body:        member.Body,
			Env:         env,
			DefClass:    cls,
			SourceFile:  interp.filename,
			SourceLines: interp.sourceLines,
		}
		if member.Init != nil {
			fn.Body = member.Init
		}
		if member.IsStatic {
			cls.StaticMethods[member.Name] = fn
		} else {
			cls.Methods[member.Name] = fn
		}
	}
	clsVal := ClassVal(cls)
	if node.Name != "" {
		env.Define(node.Name, clsVal, false)
	}
	return clsVal, nil
}

func (interp *Interpreter) execEnumDecl(node *ast.Node, env *Environment) (*Value, error) {
	obj := make(map[string]*Value)
	for i, member := range node.Members {
		var val *Value
		if member.Init != nil {
			v, err := interp.evalExpr(member.Init, env)
			if err != nil {
				return nil, err
			}
			val = v
		} else {
			val = NumberVal(float64(i))
		}
		obj[member.Name] = val
	}
	enumVal := ObjectVal(obj)
	if node.Name != "" {
		env.Define(node.Name, enumVal, false)
	}
	return enumVal, nil
}

func (interp *Interpreter) execNamespace(node *ast.Node, env *Environment) (*Value, error) {
	nsEnv := NewEnvironment(env)
	for _, stmt := range node.Body_ {
		_, err := interp.execNode(stmt, nsEnv)
		if err != nil {
			return nil, err
		}
	}
	obj := nsEnv.Snapshot()
	nsVal := ObjectVal(obj)
	if node.Name != "" {
		env.Define(node.Name, nsVal, false)
	}
	return nsVal, nil
}
