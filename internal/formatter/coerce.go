package formatter

import "lunex/internal/ast"

func FixCoercions(root *ast.Node) *ast.Node {
	walkFix(root)
	return root
}

func walkFix(n *ast.Node) {
	if n == nil {
		return
	}

	walkFix(n.Left)
	walkFix(n.Right)
	walkFix(n.Body)
	walkFix(n.Init)
	walkFix(n.Test)
	walkFix(n.Consequent)
	walkFix(n.Alternate)
	walkFix(n.Arg)
	walkFix(n.Expr)
	walkFix(n.Stmt)
	walkFix(n.Object)
	walkFix(n.Callee)
	walkFix(n.Subject)
	walkFix(n.Lo)
	walkFix(n.Hi)
	walkFix(n.Count)
	walkFix(n.Ms)
	walkFix(n.Guard)
	walkFix(n.Declaration)
	walkFix(n.Extends)
	walkFix(n.CatchBlock)
	walkFix(n.FinallyBlock)
	for _, c := range n.Args {
		walkFix(c)
	}
	for _, c := range n.Elements {
		walkFix(c)
	}
	for _, c := range n.Body_ {
		walkFix(c)
	}
	for _, c := range n.Decorators {
		walkFix(c)
	}
	for _, c := range n.Exprs {
		walkFix(c)
	}
	for _, p := range n.Properties {
		walkFix(p.Value)
		walkFix(p.Arg)
		walkFix(p.Body)
	}
	for _, m := range n.Methods {
		walkFix(m.Body)
		walkFix(m.Init)
	}
	for _, c := range n.Cases {
		walkFix(c.Body)
		walkFix(c.Guard)
	}
	for _, sc := range n.SelectCases {
		walkFix(sc.Body)
		walkFix(sc.Expr)
	}
	for _, pm := range n.Params {
		walkFix(pm.DefaultVal)
	}

	if n.Type == ast.BinaryExpr {
		fixBinaryCoercion(n)
	}
}

type staticType int

const (
	typeUnknown staticType = iota
	typeNumber
	typeString
	typeBool
)

func inferType(n *ast.Node) staticType {
	if n == nil {
		return typeUnknown
	}
	switch n.Type {
	case ast.NumberLit:
		return typeNumber
	case ast.StringLit, ast.TemplateLit:
		return typeString
	case ast.BoolLit:
		return typeBool
	case ast.CallExpr:

		if n.Callee != nil && n.Callee.Type == ast.Identifier {
			switch n.Callee.Name {
			case "str", "String":
				return typeString
			case "num", "Number", "parseInt", "parseFloat":
				return typeNumber
			case "bool", "Boolean":
				return typeBool
			}
		}
	case ast.BinaryExpr:

		l, r := inferType(n.Left), inferType(n.Right)
		if l == r && l != typeUnknown {
			return l
		}

		if n.Op == "+" && (l == typeString || r == typeString) {
			return typeString
		}
	}
	return typeUnknown
}

func wrapStr(n *ast.Node) *ast.Node {
	return &ast.Node{
		Type:   ast.CallExpr,
		Line:   n.Line,
		Col:    n.Col,
		Callee: &ast.Node{Type: ast.Identifier, Name: "str", Line: n.Line, Col: n.Col},
		Args:   []*ast.Node{n},
	}
}

func wrapNum(n *ast.Node) *ast.Node {
	return &ast.Node{
		Type:   ast.CallExpr,
		Line:   n.Line,
		Col:    n.Col,
		Callee: &ast.Node{Type: ast.Identifier, Name: "num", Line: n.Line, Col: n.Col},
		Args:   []*ast.Node{n},
	}
}

func isAlreadyWrapped(n *ast.Node, fn string) bool {
	if n == nil || n.Type != ast.CallExpr {
		return false
	}
	if n.Callee == nil || n.Callee.Type != ast.Identifier {
		return false
	}
	return n.Callee.Name == fn
}

func fixBinaryCoercion(n *ast.Node) {
	if n.Left == nil || n.Right == nil {
		return
	}
	lType := inferType(n.Left)
	rType := inferType(n.Right)

	if lType == typeUnknown && rType == typeUnknown {
		return
	}
	if lType == rType {
		return
	}

	op := n.Op
	switch op {
	case "+":

		if lType == typeString && rType != typeString && rType != typeUnknown {
			if !isAlreadyWrapped(n.Right, "str") {
				n.Right = wrapStr(n.Right)
			}
		} else if rType == typeString && lType != typeString && lType != typeUnknown {
			if !isAlreadyWrapped(n.Left, "str") {
				n.Left = wrapStr(n.Left)
			}
		}

	case "-", "*", "/", "%", "**":

		if lType == typeString && rType == typeNumber {
			if !isAlreadyWrapped(n.Left, "num") {
				n.Left = wrapNum(n.Left)
			}
		} else if rType == typeString && lType == typeNumber {
			if !isAlreadyWrapped(n.Right, "num") {
				n.Right = wrapNum(n.Right)
			}
		} else if lType == typeBool && rType == typeNumber {
			if !isAlreadyWrapped(n.Left, "num") {
				n.Left = wrapNum(n.Left)
			}
		} else if rType == typeBool && lType == typeNumber {
			if !isAlreadyWrapped(n.Right, "num") {
				n.Right = wrapNum(n.Right)
			}
		}
	}
}
