package runtime

import (
	"lunex/internal/ast"
)

func (interp *Interpreter) evalNew(node *ast.Node, env *Environment) (*Value, error) {
	calleeVal, err := interp.evalExpr(node.Callee, env)
	if err != nil {
		return nil, err
	}
	args, err := interp.evalArgs(node.Args, env)
	if err != nil {
		return nil, err
	}
	if calleeVal.Tag == TypeClass {
		return interp.callClass(calleeVal.ClsVal, args, env)
	}
	if calleeVal.Tag == TypeFunction {
		inst := NewInstance(&Class{Name: calleeVal.FnVal.Name})
		instVal := InstVal(inst)
		_, err := interp.callFunctionValue(calleeVal, args, instVal)
		if err != nil {
			return nil, err
		}
		return instVal, nil
	}
	return Null, nil
}

func (interp *Interpreter) callClass(cls *Class, args []*Value, outerEnv *Environment) (*Value, error) {
	instVal := InstVal(NewInstance(cls))
	if err := interp.initializeClassFields(cls, instVal); err != nil {
		return nil, err
	}
	if initFn, ok := cls.Methods["constructor"]; ok {
		if _, err := interp.callFunctionValue(FuncVal(initFn), args, instVal); err != nil {
			return nil, err
		}
	} else if cls.Super != nil {
		if _, err := interp.runConstructorWithThis(cls.Super, args, instVal, outerEnv); err != nil {
			return nil, err
		}
	}
	return instVal, nil
}

func (interp *Interpreter) initializeClassFields(cls *Class, instVal *Value) error {
	if cls == nil || instVal == nil || instVal.Tag != TypeInstance {
		return nil
	}
	if cls.Super != nil {
		if err := interp.initializeClassFields(cls.Super, instVal); err != nil {
			return err
		}
	}
	for _, field := range cls.Fields {
		if field == nil || field.Name == "" {
			continue
		}
		value := Undefined
		if field.Init != nil {
			fieldEnv := NewEnvironment(field.Env)
			fieldEnv.Define("this", instVal, false)
			var err error
			value, err = interp.evalExpr(field.Init, fieldEnv)
			ReleaseEnvironment(fieldEnv)
			if err != nil {
				return err
			}
		}
		instVal.InstVal.Set(field.Name, value)
	}
	return nil
}

func (interp *Interpreter) runConstructorWithThis(cls *Class, args []*Value, thisVal *Value, outerEnv *Environment) (*Value, error) {
	if cls == nil {
		return thisVal, nil
	}
	if initFn, ok := cls.Methods["constructor"]; ok {
		if _, err := interp.callFunctionValue(FuncVal(initFn), args, thisVal); err != nil {
			return nil, err
		}
	} else if cls.Super != nil {
		return interp.runConstructorWithThis(cls.Super, args, thisVal, outerEnv)
	}
	return thisVal, nil
}
