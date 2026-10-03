package runtime

import (
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/errfmt"
	"lunex/internal/lexer"
	"lunex/internal/parser"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

func (interp *Interpreter) evalStructLit(node *ast.Node, env *Environment) (*Value, error) {
	sEnv := NewEnvironment(env)

	for _, stmt := range node.Body_ {
		if stmt == nil {
			continue
		}
		if stmt.Type == ast.ExprStmt && stmt.Expr != nil && stmt.Expr.Type == ast.AssignExpr && stmt.Expr.Op == "=" {
			if left := stmt.Expr.Left; left != nil && left.Type == ast.Identifier && left.Name != "" {
				val, err := interp.evalExpr(stmt.Expr.Right, sEnv)
				if err != nil {
					return nil, err
				}
				sEnv.Define(left.Name, val, false)
				continue
			}
		}
		if _, err := interp.execNode(stmt, sEnv); err != nil {
			if _, ok := err.(*returnError); !ok {
				return nil, err
			}
		}
	}

	obj := make(map[string]*Value)
	hasFn := false
	for k, v := range sEnv.Snapshot() {
		if k == "self" || k == "this" || len(k) == 0 || k[0] == '_' {
			continue
		}
		if _, exists := obj[k]; exists {
			continue
		}
		obj[k] = v
		if v != nil && v.Tag == TypeFunction {
			hasFn = true
		}
	}

	structVal := ObjectVal(obj)
	sEnv.Define("self", structVal, false)
	sEnv.Define("this", structVal, false)

	if hasFn {
		MarkEscaped(sEnv)
	}
	return structVal, nil
}

func (interp *Interpreter) evalNumber(val interface{}) (*Value, error) {
	s, ok := val.(string)
	if !ok {
		if f, fok := val.(float64); fok {
			return NumberVal(f), nil
		}
		return NumberVal(0), nil
	}
	if cached, hit := interp.numCache.Load(s); hit {
		return NumberVal(cached.(float64)), nil
	}
	orig := s
	s = strings.ReplaceAll(s, "_", "")
	if strings.HasSuffix(s, "n") {
		s = s[:len(s)-1]
	}
	var f float64
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		n, err := strconv.ParseInt(s[2:], 16, 64)
		if err != nil {
			interp.numCache.Store(orig, float64(0))
			return NumberVal(0), nil
		}
		f = float64(n)
	} else if strings.HasPrefix(s, "0o") || strings.HasPrefix(s, "0O") {
		n, err := strconv.ParseInt(s[2:], 8, 64)
		if err != nil {
			interp.numCache.Store(orig, float64(0))
			return NumberVal(0), nil
		}
		f = float64(n)
	} else if strings.HasPrefix(s, "0b") || strings.HasPrefix(s, "0B") {
		n, err := strconv.ParseInt(s[2:], 2, 64)
		if err != nil {
			interp.numCache.Store(orig, float64(0))
			return NumberVal(0), nil
		}
		f = float64(n)
	} else {
		var err error
		f, err = strconv.ParseFloat(s, 64)
		if err != nil {
			f = math.NaN()
		}
	}
	interp.numCache.Store(orig, f)
	return NumberVal(f), nil
}

func (interp *Interpreter) evalNumberCached(node *ast.Node) (*Value, error) {
	val, err := interp.evalNumber(node.Value)
	if err != nil {
		return val, err
	}
	f := val.NumVal
	node.NumCache = &f
	return val, nil
}

var templateBuilderPool = sync.Pool{New: func() any { return new(strings.Builder) }}

func (interp *Interpreter) evalTemplate(node *ast.Node, env *Environment) (*Value, error) {
	raw, _ := node.Parts.(string)
	result := templateBuilderPool.Get().(*strings.Builder)
	result.Reset()
	defer templateBuilderPool.Put(result)
	i := 0
	for i < len(raw) {
		if raw[i] == '$' && i+1 < len(raw) && raw[i+1] == '{' {
			i += 2
			depth := 1
			start := i
			for i < len(raw) && depth > 0 {
				if raw[i] == '{' {
					depth++
				} else if raw[i] == '}' {
					depth--
				}
				if depth > 0 {
					i++
				}
			}
			exprStr := raw[start:i]
			i++
			val, err := interp.evalTemplateExpr(exprStr, env)
			if err != nil {
				result.WriteString("${error}")
			} else {
				result.WriteString(val.ToString())
			}
		} else if raw[i] == '\\' && i+1 < len(raw) {
			i++
			switch raw[i] {
			case 'n':
				result.WriteByte('\n')
			case 't':
				result.WriteByte('\t')
			case 'r':
				result.WriteByte('\r')
			case '\\':
				result.WriteByte('\\')
			case '`':
				result.WriteByte('`')
			default:
				result.WriteByte(raw[i])
			}
			i++
		} else {
			r, size := utf8.DecodeRuneInString(raw[i:])
			result.WriteRune(r)
			i += size
		}
	}
	return StringVal(result.String()), nil
}

func (interp *Interpreter) evalTemplateExpr(src string, env *Environment) (*Value, error) {
	if cached, ok := interp.templateCache.Load(src); ok {
		return interp.evalExpr(cached.(*ast.Node), env)
	}
	toks, err := lexer.Tokenize(src, "<template>")
	if err != nil {
		return StringVal(src), nil
	}
	prog, err := parser.Parse(toks, "<template>")
	if err != nil {
		return StringVal(src), nil
	}
	if len(prog.Body_) == 0 {
		return Undefined, nil
	}
	stmt := prog.Body_[0]
	var exprNode *ast.Node
	if stmt.Type == ast.ExprStmt && stmt.Expr != nil {
		exprNode = stmt.Expr
	} else {
		exprNode = stmt
	}
	interp.templateCache.Store(src, exprNode)
	return interp.evalExpr(exprNode, env)
}

func (interp *Interpreter) evalArray(node *ast.Node, env *Environment) (*Value, error) {
	var elements []*Value
	for _, el := range node.Elements {
		if el == nil {
			elements = append(elements, Undefined)
			continue
		}
		if el.Type == ast.SpreadExpr {
			val, err := interp.evalExpr(el.Arg, env)
			if err != nil {
				return nil, err
			}
			if suspErr := interp.CheckArraySpread(val, el); suspErr != nil {
				errfmt.Print(suspErr.(*errfmt.LunexError))

				continue
			}
			if val.Tag == TypeArray {
				elements = append(elements, val.ArrVal...)
			} else {
				elements = append(elements, val)
			}
			continue
		}
		val, err := interp.evalExpr(el, env)
		if err != nil {
			return nil, err
		}
		elements = append(elements, val)
	}
	return ArrayVal(elements), nil
}

func (interp *Interpreter) evalObject(node *ast.Node, env *Environment) (*Value, error) {
	obj := make(map[string]*Value)
	for _, prop := range node.Properties {
		switch prop.Kind {
		case "spread":
			val, err := interp.evalExpr(prop.Arg, env)
			if err != nil {
				return nil, err
			}
			if suspErr := interp.CheckObjectSpread(val, prop.Arg); suspErr != nil {
				errfmt.Print(suspErr.(*errfmt.LunexError))

			} else if val.Tag == TypeObject {
				for k, v := range val.ObjVal {
					obj[k] = v
				}
			} else if val.Tag == TypeInstance {
				for k, v := range val.InstVal.Fields {
					obj[k] = v
				}
			}
		case "prop":
			var key string
			if prop.Computed {
				kv, err := interp.evalExpr(prop.Key.(*ast.Node), env)
				if err != nil {
					return nil, err
				}
				key = kv.ToString()
			} else {
				key, _ = prop.Key.(string)
			}
			val, err := interp.evalExpr(prop.Value, env)
			if err != nil {
				return nil, err
			}
			obj[key] = val
		case "shorthand":
			key, _ := prop.Key.(string)
			val, _ := env.Get(key)
			obj[key] = val
		case "method":
			var key string
			if prop.Computed {
				if keyNode, ok := prop.Key.(*ast.Node); ok {
					kv, err := interp.evalExpr(keyNode, env)
					if err != nil {
						return nil, err
					}
					key = kv.ToString()
				}
			} else {
				key, _ = prop.Key.(string)
			}
			fn := &Function{
				Name:   key,
				Params: paramsToFnParams(prop.Params),
				Body:   prop.Body,
				Env:    env,
			}
			obj[key] = FuncVal(fn)
		}
	}
	return ObjectVal(obj), nil
}

func (interp *Interpreter) evalRegex(node *ast.Node) (*Value, error) {
	flags := ""
	pattern := node.Pattern
	if strings.Contains(node.Flags, "i") {
		flags += "(?i)"
	}
	if strings.Contains(node.Flags, "m") {
		flags += "(?m)"
	}

	if strings.Contains(node.Flags, "s") {
		flags += "(?s)"
	}

	re, err := regexp.Compile(flags + pattern)
	if err != nil {
		return Null, nil
	}
	return RegexV(re), nil
}

func (interp *Interpreter) evalIdentifier(node *ast.Node, env *Environment) (*Value, error) {
	name := node.Name
	switch name {
	case "undefined":
		return Undefined, nil
	case "null":
		return Null, nil
	case "true":
		return True, nil
	case "false":
		return False, nil
	case "NaN":
		return NumberVal(math.NaN()), nil
	case "Infinity":
		return NumberVal(math.Inf(1)), nil
	}
	if addr := node.ResolvedAddr; addr != nil {
		return env.GetSlotAddr(addr.Hops, addr.Slot), nil
	}
	val, ok := env.Get(name)
	if !ok {
		allNames := visibleNames(env)
		similar := errfmt.FindSimilar(name, allNames)
		e := errfmt.ReferenceErrorWithSimilar(name, interp.filename, node.Line, node.Col, interp.sourceLines, similar)

		if len(similar) > 0 {
			e.Notes = append(e.Notes,
				fmt.Sprintf("did you mean `%s`? (closest match by name)", similar[0]))
		}

		noisy := map[string]bool{
			"Error": true, "Infinity": true, "NaN": true, "Math": true,
			"setInterval": true, "isFinite": true, "Number": true,
			"Map": true, "log": true, "str": true,
		}
		userNames := make([]string, 0, len(allNames))
		for _, n := range allNames {
			if !noisy[n] {
				userNames = append(userNames, n)
			}
		}
		if len(userNames) > 0 {
			visible := userNames
			if len(visible) > 6 {
				visible = visible[:6]
			}
			quoted := make([]string, len(visible))
			for i, n := range visible {
				quoted[i] = "`" + n + "`"
			}
			e.Notes = append(e.Notes, "names in scope: "+strings.Join(quoted, ", "))
		}
		return nil, e
	}
	return val, nil
}

func (interp *Interpreter) evalTypeof(node *ast.Node, env *Environment) (*Value, error) {
	val, _ := interp.evalExpr(node.Arg, env)
	if val == nil {
		return StringVal("undefined"), nil
	}
	return StringVal(val.TypeName()), nil
}

func (interp *Interpreter) evalDelete(node *ast.Node, env *Environment) (*Value, error) {
	if node.Arg.Type == ast.MemberExpr {
		obj, err := interp.evalExpr(node.Arg.Object, env)
		if err != nil {
			return nil, err
		}
		var key string
		if node.Arg.Computed {
			k, err := interp.evalExpr(node.Arg.Prop.(*ast.Node), env)
			if err != nil {
				return nil, err
			}
			key = k.ToString()
		} else {
			key, _ = node.Arg.Prop.(string)
		}
		if obj.Tag == TypeObject {
			delete(obj.ObjVal, key)
		}
	}
	return True, nil
}
