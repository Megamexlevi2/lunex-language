package runtime

import (
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/errfmt"
	"math"
)

func (interp *Interpreter) evalExpr(node *ast.Node, env *Environment) (*Value, error) {
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
	case ast.NumberLit:
		if node.NumCache != nil {
			return NumberVal(*node.NumCache), nil
		}
		return interp.evalNumberCached(node)
	case ast.StringLit:
		s, _ := node.Value.(string)
		return StringVal(s), nil
	case ast.BoolLit:
		b, _ := node.Value.(bool)
		return BoolVal(b), nil
	case ast.NullLit:
		return Null, nil
	case ast.UndefinedLit:
		return Undefined, nil
	case ast.TemplateLit:
		return interp.evalTemplate(node, env)
	case ast.ArrayLit:
		return interp.evalArray(node, env)
	case ast.ObjectLit:
		return interp.evalObject(node, env)
	case ast.RegexLit:
		return interp.evalRegex(node)
	case ast.Identifier:
		return interp.evalIdentifier(node, env)
	case ast.ThisExpr:
		val, _ := env.Get("this")
		return val, nil
	case ast.SuperExpr:
		val, _ := env.Get("__super__")
		return val, nil
	case ast.VoidExpr:
		interp.evalExpr(node.Arg, env)
		return Undefined, nil
	case ast.TypeofExpr:
		return interp.evalTypeof(node, env)
	case ast.DeleteExpr:
		return interp.evalDelete(node, env)
	case ast.FnExpr, ast.FnDecl:
		return interp.evalFnExpr(node, env)
	case ast.ArrowFn:
		return interp.evalArrowFn(node, env)
	case ast.CallExpr:
		return interp.evalCall(node, env)
	case ast.NewExpr:
		return interp.evalNew(node, env)
	case ast.MemberExpr:
		return interp.evalMember(node, env)
	case ast.BinaryExpr:
		return interp.evalBinary(node, env)
	case ast.UnaryExpr:
		return interp.evalUnary(node, env)
	case ast.AssignExpr:
		return interp.evalAssign(node, env)
	case ast.TernaryExpr:
		return interp.evalTernary(node, env)
	case ast.SpreadExpr:
		return interp.evalExpr(node.Arg, env)
	case ast.PipelineExpr:
		return interp.evalPipeline(node, env)
	case ast.SequenceExpr:
		return interp.evalSequence(node, env)
	case ast.NotExpr:
		val, err := interp.evalExpr(node.Arg, env)
		if err != nil {
			return nil, err
		}
		return BoolVal(!val.IsTruthy()), nil
	case ast.HaveExpr:
		return interp.evalHaveExpr(node, env)
	case ast.TrySafeExpr:
		return interp.evalTrySafe(node, env)
	case ast.RangeExpr:
		return interp.evalRange(node, env)
	case ast.SleepExpr:
		return interp.evalSleep(node, env)
	case ast.ChannelExpr:
		return ChanV(NewChannel(64)), nil
	case ast.NaxImportExpr:
		return Null, nil
	case ast.AtImportExpr:
		return interp.evalAtImport(node, env)
	case ast.StructLit:
		return interp.evalStructLit(node, env)
	case ast.MatchStmt:
		return interp.evalMatchExpr(node, env)
	case ast.SatisfiesExpr:
		return interp.evalExpr(node.Expr, env)
	case ast.DecoratedExpr:
		if node.Expr != nil {
			return interp.evalExpr(node.Expr, env)
		}
		return Undefined, nil
	case ast.ExprStmt:
		return interp.evalExpr(node.Expr, env)
	case ast.IfStmt:
		return interp.execIf(node, env)
	case ast.UnlessStmt:
		return interp.execUnless(node, env)
	default:
		return Undefined, nil
	}
}

func (interp *Interpreter) evalMember(node *ast.Node, env *Environment) (*Value, error) {
	obj, err := interp.evalExpr(node.Object, env)
	if err != nil {
		return nil, err
	}
	if node.Optional && obj.IsNullish() {
		return Undefined, nil
	}
	if node.Computed {
		propVal, err := interp.evalExpr(node.Prop.(*ast.Node), env)
		if err != nil {
			return nil, err
		}
		if obj.Tag == TypeArray {
			if propVal.Tag == TypeNumber {
				idx := int(propVal.NumVal)
				if v, suspErr := interp.CheckIndexBounds(obj, idx, node); suspErr != nil {
					errfmt.Print(suspErr.(*errfmt.LunexError))
					return v, nil
				}
				return obj.GetIndex(idx), nil
			}
			return obj.Get(propVal.ToString()), nil
		}
		if obj.Tag == TypeString && propVal.Tag == TypeNumber {
			return obj.GetIndex(int(propVal.NumVal)), nil
		}
		return obj.Get(propVal.ToString()), nil
	}
	key, _ := node.Prop.(string)

	if obj.Tag == TypeNull || obj.Tag == TypeUndefined {
		objName := ""
		if node.Object != nil && node.Object.Type == ast.Identifier {
			objName = node.Object.Name
		}
		msg := fmt.Sprintf("cannot read property `%s` of %s", key, obj.TypeName())
		if objName != "" {
			msg = fmt.Sprintf("cannot read property `%s` of `%s` (which is %s)", key, objName, obj.TypeName())
		}
		e := interp.runtimeError(errfmt.KindType, "E0004", msg, node, nil)
		e.Notes = append(e.Notes, "guard with: if "+func() string {
			if objName != "" {
				return objName
			}
			return "value"
		}()+" != null { ... }")
		return nil, e
	}
	return obj.Get(key), nil
}

func (interp *Interpreter) evalBinary(node *ast.Node, env *Environment) (*Value, error) {
	op := node.Op
	if op == "&&" {
		left, err := interp.evalExpr(node.Left, env)
		if err != nil {
			return nil, err
		}
		if !left.IsTruthy() {
			return left, nil
		}
		return interp.evalExpr(node.Right, env)
	}
	if op == "||" {
		left, err := interp.evalExpr(node.Left, env)
		if err != nil {
			return nil, err
		}
		if left.IsTruthy() {
			return left, nil
		}
		return interp.evalExpr(node.Right, env)
	}
	if op == "??" {
		left, err := interp.evalExpr(node.Left, env)
		if err != nil {
			return nil, err
		}
		if !left.IsNullish() {
			return left, nil
		}
		return interp.evalExpr(node.Right, env)
	}

	left, err := interp.evalExpr(node.Left, env)
	if err != nil {
		return nil, err
	}
	right, err := interp.evalExpr(node.Right, env)
	if err != nil {
		return nil, err
	}

	switch op {
	case "+":
		if left.Tag == TypeNumber && right.Tag == TypeNumber {
			return NumberVal(left.NumVal + right.NumVal), nil
		}
		if left.Tag == TypeString && right.Tag == TypeString {
			return StringVal(left.StrVal + right.StrVal), nil
		}
		if left.Tag == TypeString || right.Tag == TypeString {

			var detail string
			if left.Tag == TypeString && (right.TypeName() == "undefined" || right.TypeName() == "null") {
				detail = fmt.Sprintf(
					"type error: '+' cannot concatenate string and %s — "+
						"the right-hand operand is %s. "+
						"Did you forget to initialize a variable? "+
						"Use str(%s) to convert, or guard with: if x != null { ... }",
					right.TypeName(), right.TypeName(), right.TypeName(),
				)
			} else if right.Tag == TypeString && (left.TypeName() == "undefined" || left.TypeName() == "null") {
				detail = fmt.Sprintf(
					"type error: '+' cannot concatenate %s and string — "+
						"the left-hand operand is %s. "+
						"Did you forget to initialize a variable? "+
						"Use str(%s) to convert, or guard with: if x != null { ... }",
					left.TypeName(), left.TypeName(), left.TypeName(),
				)
			} else {
				detail = fmt.Sprintf(
					"type error: '+' cannot combine %s and %s — "+
						"only string+string or number+number are allowed. "+
						"Use str(x) to convert to string, or Number(x) to convert to number.",
					left.TypeName(), right.TypeName(),
				)
			}
			return nil, interp.runtimeError(errfmt.KindType, errfmt.ErrTypeMismatch, detail, node, nil)
		}
		result := left.ToNumber() + right.ToNumber()
		if v, suspErr := interp.CheckNaNResult(result, "+", left, right, node); suspErr != nil {
			errfmt.Print(suspErr.(*errfmt.LunexError))
			return v, nil
		}
		return NumberVal(result), nil
	case "-":
		if left.Tag == TypeNumber && right.Tag == TypeNumber {
			return NumberVal(left.NumVal - right.NumVal), nil
		}
		if left.Tag == TypeString || right.Tag == TypeString {
			return nil, interp.runtimeError(errfmt.KindType, errfmt.ErrTypeMismatch,
				fmt.Sprintf("type error: '-' cannot be applied to %s and %s", left.TypeName(), right.TypeName()), node, nil)
		}
		result := left.ToNumber() - right.ToNumber()
		if v, suspErr := interp.CheckNaNResult(result, "-", left, right, node); suspErr != nil {
			errfmt.Print(suspErr.(*errfmt.LunexError))
			return v, nil
		}
		return NumberVal(result), nil
	case "*":
		if left.Tag == TypeNumber && right.Tag == TypeNumber {
			return NumberVal(left.NumVal * right.NumVal), nil
		}
		if left.Tag == TypeString || right.Tag == TypeString {
			return nil, interp.runtimeError(errfmt.KindType, errfmt.ErrTypeMismatch,
				fmt.Sprintf("type error: '*' cannot be applied to %s and %s", left.TypeName(), right.TypeName()), node, nil)
		}
		result := left.ToNumber() * right.ToNumber()
		if v, suspErr := interp.CheckNaNResult(result, "*", left, right, node); suspErr != nil {
			errfmt.Print(suspErr.(*errfmt.LunexError))
			return v, nil
		}
		return NumberVal(result), nil
	case "/":
		if left.Tag == TypeNumber && right.Tag == TypeNumber {
			if right.NumVal == 0 {
				return nil, interp.runtimeError(errfmt.KindArithmetic, errfmt.ErrDivisionByZero,
					"division by zero", node, nil)
			}
			return NumberVal(left.NumVal / right.NumVal), nil
		}
		if left.Tag == TypeString || right.Tag == TypeString {
			return nil, interp.runtimeError(errfmt.KindType, errfmt.ErrTypeMismatch,
				fmt.Sprintf("type error: '/' cannot be applied to %s and %s", left.TypeName(), right.TypeName()), node, nil)
		}
		r := right.ToNumber()
		if r == 0 {
			return nil, interp.runtimeError(errfmt.KindArithmetic, errfmt.ErrDivisionByZero,
				"division by zero", node, nil)
		}
		result := left.ToNumber() / r
		if v, suspErr := interp.CheckNaNResult(result, "/", left, right, node); suspErr != nil {
			errfmt.Print(suspErr.(*errfmt.LunexError))
			return v, nil
		}
		return NumberVal(result), nil
	case "%":
		if right.Tag == TypeNumber && right.NumVal == 0 {
			return nil, interp.runtimeError(errfmt.KindArithmetic, errfmt.ErrDivisionByZero,
				"modulo by zero", node, nil)
		}
		if left.Tag == TypeString || right.Tag == TypeString {
			return nil, interp.runtimeError(errfmt.KindType, errfmt.ErrTypeMismatch,
				fmt.Sprintf("type error: '%%' cannot be applied to %s and %s", left.TypeName(), right.TypeName()), node, nil)
		}
		r := right.ToNumber()
		if r == 0 {
			return nil, interp.runtimeError(errfmt.KindArithmetic, errfmt.ErrDivisionByZero,
				"modulo by zero", node, nil)
		}
		result := math.Mod(left.ToNumber(), r)
		if v, suspErr := interp.CheckNaNResult(result, "%", left, right, node); suspErr != nil {
			errfmt.Print(suspErr.(*errfmt.LunexError))
			return v, nil
		}
		return NumberVal(result), nil
	case "**":
		result := math.Pow(left.ToNumber(), right.ToNumber())
		if v, suspErr := interp.CheckNaNResult(result, "**", left, right, node); suspErr != nil {
			errfmt.Print(suspErr.(*errfmt.LunexError))
			return v, nil
		}
		return NumberVal(result), nil
	case "===":
		return BoolVal(!left.StrictEquals(right)), nil
	case "==":
		return BoolVal(left.Equals(right)), nil
	case "!=":
		return BoolVal(!left.Equals(right)), nil
	case "<":
		if left.Tag == TypeNumber && right.Tag == TypeNumber {
			return BoolVal(left.NumVal < right.NumVal), nil
		}
		if left.Tag == TypeString && right.Tag == TypeString {
			return BoolVal(left.StrVal < right.StrVal), nil
		}
		return BoolVal(left.ToNumber() < right.ToNumber()), nil
	case ">":
		if left.Tag == TypeNumber && right.Tag == TypeNumber {
			return BoolVal(left.NumVal > right.NumVal), nil
		}
		if left.Tag == TypeString && right.Tag == TypeString {
			return BoolVal(left.StrVal > right.StrVal), nil
		}
		return BoolVal(left.ToNumber() > right.ToNumber()), nil
	case "<=":
		if left.Tag == TypeNumber && right.Tag == TypeNumber {
			return BoolVal(left.NumVal <= right.NumVal), nil
		}
		if left.Tag == TypeString && right.Tag == TypeString {
			return BoolVal(left.StrVal <= right.StrVal), nil
		}
		return BoolVal(left.ToNumber() <= right.ToNumber()), nil
	case ">=":
		if left.Tag == TypeNumber && right.Tag == TypeNumber {
			return BoolVal(left.NumVal >= right.NumVal), nil
		}
		if left.Tag == TypeString && right.Tag == TypeString {
			return BoolVal(left.StrVal >= right.StrVal), nil
		}
		return BoolVal(left.ToNumber() >= right.ToNumber()), nil
	case "&":
		return NumberVal(float64(int64(left.ToNumber()) & int64(right.ToNumber()))), nil
	case "|":
		return NumberVal(float64(int64(left.ToNumber()) | int64(right.ToNumber()))), nil
	case "^":
		return NumberVal(float64(int64(left.ToNumber()) ^ int64(right.ToNumber()))), nil
	case "<<":
		return NumberVal(float64(int64(left.ToNumber()) << uint(right.ToNumber()))), nil
	case ">>":
		return NumberVal(float64(int64(left.ToNumber()) >> uint(right.ToNumber()))), nil
	case ">>>":
		return NumberVal(float64(uint64(left.ToNumber()) >> uint(right.ToNumber()))), nil
	case "instanceof":
		if right.Tag == TypeClass && left.Tag == TypeInstance {
			return BoolVal(isInstanceOf(left.InstVal, right.ClsVal)), nil
		}
		return False, nil
	case "in":
		key := left.ToString()
		switch right.Tag {
		case TypeObject:
			_, ok := right.ObjVal[key]
			return BoolVal(ok), nil
		case TypeArray:
			idx := int(left.ToNumber())
			if left.Tag == TypeNumber && idx >= 0 && idx < len(right.ArrVal) {
				return True, nil
			}
			return False, nil
		case TypeInstance:
			_, ok := right.InstVal.Fields[key]
			return BoolVal(ok), nil
		}
		return False, nil
	}
	return Undefined, nil
}

func (interp *Interpreter) evalUnary(node *ast.Node, env *Environment) (*Value, error) {
	if node.Op == "++" || node.Op == "--" {
		val, err := interp.evalExpr(node.Arg, env)
		if err != nil {
			return nil, err
		}
		num := val.ToNumber()
		var newNum float64
		if node.Op == "++" {
			newNum = num + 1
		} else {
			newNum = num - 1
		}
		newVal := NumberVal(newNum)
		err = interp.assignToNode(node.Arg, newVal, env)
		if err != nil {
			return nil, err
		}
		if err := interp.notifyWatchers(); err != nil {
			return nil, err
		}
		if node.Prefix {
			return newVal, nil
		}
		return val, nil
	}
	arg, err := interp.evalExpr(node.Arg, env)
	if err != nil {
		return nil, err
	}
	switch node.Op {
	case "!":
		return BoolVal(!arg.IsTruthy()), nil
	case "-":
		return NumberVal(-arg.ToNumber()), nil
	case "+":
		return NumberVal(arg.ToNumber()), nil
	case "~":
		return NumberVal(float64(^int64(arg.ToNumber()))), nil
	}
	return Undefined, nil
}

func (interp *Interpreter) evalAssign(node *ast.Node, env *Environment) (*Value, error) {
	right, err := interp.evalExpr(node.Right, env)
	if err != nil {
		return nil, err
	}
	if node.Op != "=" {
		left, err := interp.evalExpr(node.Left, env)
		if err != nil {
			return nil, err
		}
		op := node.Op[:len(node.Op)-1]
		right, err = interp.evalBinaryValues(left, right, op)
		if err != nil {
			return nil, err
		}
	}
	err = interp.assignToNode(node.Left, right, env)
	if err != nil {
		return nil, err
	}
	if err := interp.notifyWatchers(); err != nil {
		return nil, err
	}
	return right, nil
}

func (interp *Interpreter) evalBinaryValues(left, right *Value, op string) (*Value, error) {
	switch op {
	case "+":
		if left.Tag == TypeNumber && right.Tag == TypeNumber {
			return NumberVal(left.NumVal + right.NumVal), nil
		}
		if left.Tag == TypeString && right.Tag == TypeString {
			return StringVal(left.StrVal + right.StrVal), nil
		}
		if left.Tag == TypeString || right.Tag == TypeString {
			var detail string
			if left.Tag == TypeString && (right.TypeName() == "undefined" || right.TypeName() == "null") {
				detail = fmt.Sprintf(
					"type error: '+=' cannot concatenate string and %s — "+
						"the right-hand operand is %s. "+
						"Did you forget to initialize this variable? "+
						"Use str(%s) to convert, or guard with: if x != null { x += y }",
					right.TypeName(), right.TypeName(), right.TypeName(),
				)
			} else if right.Tag == TypeString && (left.TypeName() == "undefined" || left.TypeName() == "null") {
				detail = fmt.Sprintf(
					"type error: '+=' cannot concatenate %s and string — "+
						"the left-hand operand is %s. "+
						"Ensure the variable was initialized before using '+='.",
					left.TypeName(), left.TypeName(),
				)
			} else {
				detail = fmt.Sprintf(
					"type error: '+=' cannot combine %s and %s — "+
						"only string+string or number+number are allowed. "+
						"Use str(x) to convert to string, or Number(x) to convert to number.",
					left.TypeName(), right.TypeName(),
				)
			}
			return nil, interp.runtimeError(errfmt.KindType, errfmt.ErrTypeMismatch, detail, nil, nil)
		}
		return NumberVal(left.ToNumber() + right.ToNumber()), nil
	case "-":
		if left.Tag == TypeNumber && right.Tag == TypeNumber {
			return NumberVal(left.NumVal - right.NumVal), nil
		}
		if left.Tag == TypeString || right.Tag == TypeString {
			return nil, interp.runtimeError(errfmt.KindType, errfmt.ErrTypeMismatch,
				fmt.Sprintf("type error: '-' cannot be applied to %s and %s", left.TypeName(), right.TypeName()), nil, nil)
		}
		return NumberVal(left.ToNumber() - right.ToNumber()), nil
	case "*":
		if left.Tag == TypeNumber && right.Tag == TypeNumber {
			return NumberVal(left.NumVal * right.NumVal), nil
		}
		if left.Tag == TypeString || right.Tag == TypeString {
			return nil, interp.runtimeError(errfmt.KindType, errfmt.ErrTypeMismatch,
				fmt.Sprintf("type error: '*' cannot be applied to %s and %s", left.TypeName(), right.TypeName()), nil, nil)
		}
		return NumberVal(left.ToNumber() * right.ToNumber()), nil
	case "/":
		if left.Tag == TypeNumber && right.Tag == TypeNumber {
			if right.NumVal == 0 {
				return nil, interp.runtimeError(errfmt.KindArithmetic, errfmt.ErrDivisionByZero,
					"division by zero", nil, nil)
			}
			return NumberVal(left.NumVal / right.NumVal), nil
		}
		if left.Tag == TypeString || right.Tag == TypeString {
			return nil, interp.runtimeError(errfmt.KindType, errfmt.ErrTypeMismatch,
				fmt.Sprintf("type error: '/' cannot be applied to %s and %s", left.TypeName(), right.TypeName()), nil, nil)
		}
		r2 := right.ToNumber()
		if r2 == 0 {
			return nil, interp.runtimeError(errfmt.KindArithmetic, errfmt.ErrDivisionByZero,
				"division by zero", nil, nil)
		}
		return NumberVal(left.ToNumber() / r2), nil
	case "%":
		if right.Tag == TypeNumber && right.NumVal == 0 {
			return nil, interp.runtimeError(errfmt.KindArithmetic, errfmt.ErrDivisionByZero,
				"modulo by zero", nil, nil)
		}
		if left.Tag == TypeString || right.Tag == TypeString {
			return nil, interp.runtimeError(errfmt.KindType, errfmt.ErrTypeMismatch,
				fmt.Sprintf("type error: '%%' cannot be applied to %s and %s", left.TypeName(), right.TypeName()), nil, nil)
		}
		r2 := right.ToNumber()
		if r2 == 0 {
			return nil, interp.runtimeError(errfmt.KindArithmetic, errfmt.ErrDivisionByZero,
				"modulo by zero", nil, nil)
		}
		return NumberVal(math.Mod(left.ToNumber(), r2)), nil
	case "**":
		return NumberVal(math.Pow(left.ToNumber(), right.ToNumber())), nil
	case "&&":
		if !left.IsTruthy() {
			return left, nil
		}
		return right, nil
	case "||":
		if left.IsTruthy() {
			return left, nil
		}
		return right, nil
	case "??":
		if !left.IsNullish() {
			return left, nil
		}
		return right, nil
	case "<<":
		return NumberVal(float64(int64(left.ToNumber()) << uint(right.ToNumber()))), nil
	case ">>":
		return NumberVal(float64(int64(left.ToNumber()) >> uint(right.ToNumber()))), nil
	}
	return right, nil
}

func (interp *Interpreter) assignToNode(target *ast.Node, val *Value, env *Environment) error {
	switch target.Type {
	case ast.Identifier:
		var err error
		if addr := target.ResolvedAddr; addr != nil {
			err = env.SetSlot(addr.Hops, addr.Slot, target.Name, val)
		} else {
			err = env.Set(target.Name, val)
		}
		if err != nil {
			if le, ok := err.(*errfmt.LunexError); ok {
				if le.Line == 0 {
					le.File = interp.filename
					le.Lines = interp.sourceLines
					le.Line = target.Line
					le.Col = target.Col
					if le.Line == 0 {
						le.Line = interp.currentLine
						le.Col = interp.currentCol
					}
				}
			}
			return err
		}
		return nil
	case ast.MemberExpr:
		obj, err := interp.evalExpr(target.Object, env)
		if err != nil {
			return err
		}
		if obj == nil || obj.IsNullish() {
			return interp.runtimeError(errfmt.KindType, "E0005", "cannot assign a member of null or undefined", target, nil)
		}
		if target.Computed {
			keyVal, err := interp.evalExpr(target.Prop.(*ast.Node), env)
			if err != nil {
				return err
			}
			key := keyVal.ToString()
			if obj.Tag == TypeArray {
				idx := int(keyVal.ToNumber())
				for len(obj.ArrVal) <= idx {
					obj.ArrVal = append(obj.ArrVal, Undefined)
				}
				obj.ArrVal[idx] = val
			} else {
				obj.Set(key, val)
			}
			return nil
		}
		key, _ := target.Prop.(string)
		obj.Set(key, val)
		return nil
	}
	return nil
}

func (interp *Interpreter) evalTernary(node *ast.Node, env *Environment) (*Value, error) {
	cond, err := interp.evalExpr(node.Test, env)
	if err != nil {
		return nil, err
	}
	if cond.IsTruthy() {
		return interp.evalExpr(node.Consequent, env)
	}
	return interp.evalExpr(node.Alternate, env)
}

func (interp *Interpreter) evalPipeline(node *ast.Node, env *Environment) (*Value, error) {
	left, err := interp.evalExpr(node.Left, env)
	if err != nil {
		return nil, err
	}
	fn, err := interp.evalExpr(node.Right, env)
	if err != nil {
		return nil, err
	}
	return interp.callFunctionValue(fn, []*Value{left}, nil)
}

func (interp *Interpreter) evalSequence(node *ast.Node, env *Environment) (*Value, error) {
	var result *Value = Undefined
	for _, e := range node.Exprs {
		val, err := interp.evalExpr(e, env)
		if err != nil {
			return nil, err
		}
		result = val
	}
	return result, nil
}
