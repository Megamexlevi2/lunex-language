package runtime

import (
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/errfmt"
	"math"
)

func (interp *Interpreter) suspectError(
	code, msg string,
	node *ast.Node,
	suggestion string,
	notes ...string,
) *errfmt.LunexError {
	line, col := interp.currentLine, interp.currentCol
	if node != nil {
		if node.Line != 0 {
			line = node.Line
		}
		if node.Col != 0 {
			col = node.Col
		}
	}
	e := &errfmt.LunexError{
		Message:    msg,
		File:       interp.filename,
		Line:       line,
		Col:        col,
		Kind:       errfmt.KindSuspect,
		Code:       code,
		Lines:      interp.sourceLines,
		Suggestion: suggestion,
	}
	for _, n := range notes {
		e.Notes = append(e.Notes, n)
	}
	return e
}

func (interp *Interpreter) CheckForOfIterable(val *Value, node *ast.Node) (*Value, error) {
	switch val.Tag {
	case TypeArray, TypeString, TypeObject:
		return nil, nil
	}

	typeName := val.TypeName()
	iterName := ""
	if node != nil && node.Name != "" {
		iterName = " `" + node.Name + "`"
	}

	note := fmt.Sprintf("value of type `%s` is not iterable", typeName)
	var hint string
	switch val.Tag {
	case TypeNull, TypeUndefined:
		hint = "guard the value before the loop:  if x != null { for el of x { ... } }"
	case TypeNumber:
		hint = "to iterate N times, use a range:  for i of 0..N { ... }"
	case TypeBool:
		hint = "booleans are not iterable — check the variable holding the loop target"
	case TypeFunction:
		hint = "a function was passed where an array was expected — did you forget to call it?"
	default:
		hint = "only arrays, strings, and objects are iterable in Lunex"
	}

	e := interp.suspectError(
		errfmt.ErrSuspectForOfNonIterable,
		fmt.Sprintf("cannot iterate over%s: value is `%s`, not an iterable", iterName, typeName),
		node,
		hint,
		note,
	)
	return Undefined, e
}

func (interp *Interpreter) CheckMatchResult(subject *Value, node *ast.Node) (*Value, error) {
	subjectStr := subject.ToString()
	if len(subjectStr) > 40 {
		subjectStr = subjectStr[:40] + "…"
	}
	e := interp.suspectError(
		errfmt.ErrSuspectMatchNoArm,
		fmt.Sprintf("no `match` arm matched the subject value `%s`", subjectStr),
		node,
		"add a default (catch-all) arm:  _ => {  }",
		fmt.Sprintf("subject type is `%s`", subject.TypeName()),
		"without a default arm, a missed match silently returns undefined",
	)
	e.ExBad = "match x {\n  1 => \"one\"\n  2 => \"two\"\n}"
	e.ExGood = "match x {\n  1 => \"one\"\n  2 => \"two\"\n  _ => \"other\"\n}"
	return Undefined, e
}

func (interp *Interpreter) CheckNaNResult(result float64, op string, left, right *Value, node *ast.Node) (*Value, error) {
	if !math.IsNaN(result) {
		return nil, nil
	}

	leftDesc := fmt.Sprintf("`%s` (%s)", left.ToString(), left.TypeName())
	rightDesc := fmt.Sprintf("`%s` (%s)", right.ToString(), right.TypeName())
	e := interp.suspectError(
		errfmt.ErrSuspectNaNResult,
		fmt.Sprintf("arithmetic operation `%s` produced NaN: left=%s, right=%s", op, leftDesc, rightDesc),
		node,
		"use explicit conversion before arithmetic:  Number(x)  or guard with:  if typeof(x) == \"number\" { ... }",
		"NaN silently propagates — all further arithmetic with this value will also be NaN",
		"common causes: undefined variable, null field access, non-numeric string in a math expression",
	)
	e.ExBad = "val x = undefined\nval y = x + 5"
	e.ExGood = "val x = 10\nval y = x + 5"
	return Undefined, e
}

func (interp *Interpreter) CheckIndexBounds(arr *Value, idx int, node *ast.Node) (*Value, error) {
	length := len(arr.ArrVal)
	if idx >= 0 && idx < length {
		return nil, nil
	}

	var detail string
	if idx < 0 {
		detail = fmt.Sprintf("index %d is negative — Lunex arrays start at 0", idx)
	} else {
		detail = fmt.Sprintf("index %d is out of range — array has %d element(s), valid range is 0..%d", idx, length, length-1)
	}

	e := interp.suspectError(
		errfmt.ErrSuspectIndexOutOfBounds,
		fmt.Sprintf("array index out of bounds: index=%d, length=%d", idx, length),
		node,
		fmt.Sprintf("guard the access:  if i >= 0 && i < arr.length { arr[i] }"),
		detail,
	)
	e.ExBad = "val arr = [1, 2, 3]\nval x = arr[5]"
	e.ExGood = "val arr = [1, 2, 3]\nif 5 < arr.length { val x = arr[5] }"
	return Undefined, e
}

func (interp *Interpreter) CheckArraySpread(val *Value, node *ast.Node) error {
	switch val.Tag {
	case TypeArray:
		return nil
	case TypeNull, TypeUndefined:
		e := interp.suspectError(
			errfmt.ErrSuspectNullSpread,
			fmt.Sprintf("spreading %s into array literal — nothing will be added", val.TypeName()),
			node,
			"guard the spread:  if src != null { ...src }",
			"spreading null or undefined is a no-op that silently produces an empty result",
		)
		return e
	default:
		e := interp.suspectError(
			errfmt.ErrSuspectSpreadNonIterable,
			fmt.Sprintf("cannot spread `%s` into array — only arrays can be spread here", val.TypeName()),
			node,
			"wrap in an array first:  [...[value]]  or convert to array before spreading",
			fmt.Sprintf("got: %s — expected: array", val.TypeName()),
		)
		return e
	}
}

func (interp *Interpreter) CheckObjectSpread(val *Value, node *ast.Node) error {
	switch val.Tag {
	case TypeObject, TypeInstance:
		return nil
	case TypeNull, TypeUndefined:
		e := interp.suspectError(
			errfmt.ErrSuspectNullSpread,
			fmt.Sprintf("spreading %s into object literal — no properties will be added", val.TypeName()),
			node,
			"guard the spread:  if src != null { ...src }",
			"spreading null or undefined is a no-op that silently produces an empty object",
		)
		return e
	default:
		e := interp.suspectError(
			errfmt.ErrSuspectSpreadNonIterable,
			fmt.Sprintf("cannot spread `%s` into object — only objects and instances can be spread here", val.TypeName()),
			node,
			"only objects and struct instances can be spread into object literals",
			fmt.Sprintf("got: %s — expected: object or instance", val.TypeName()),
		)
		return e
	}
}

func (interp *Interpreter) CheckCallResultUndefined(fnResult *Value, callerNode *ast.Node) (*Value, error) {
	if fnResult == nil || fnResult.Tag == TypeUndefined || fnResult.Tag == TypeNull {
		resultDesc := "undefined"
		if fnResult != nil {
			resultDesc = fnResult.TypeName()
		}
		calleeName := ""
		if callerNode != nil && callerNode.Callee != nil && callerNode.Callee.Type == ast.Identifier {
			calleeName = "`" + callerNode.Callee.Name + "` "
		}
		e := interp.suspectError(
			errfmt.ErrSuspectCallUndefined,
			fmt.Sprintf("calling %sresulted in %s — the function may not return a value", calleeName, resultDesc),
			callerNode,
			"make sure the function ends with an explicit `return <value>` statement",
			"a function without a `return` statement implicitly returns undefined",
			"check for early `return` branches that forget to return a value",
		)
		e.ExBad = "fn make() { val x = 42 }\nval result = make()()"
		e.ExGood = "fn make() { return 42 }\nval result = make()()"
		return Undefined, e
	}
	return nil, nil
}
