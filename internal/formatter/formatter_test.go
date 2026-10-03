package formatter

import (
	"lunex/internal/ast"
	"strings"
	"testing"
)

func TestFormatPreservesStringLiterals(t *testing.T) {
	tree := &ast.Node{
		Type: ast.Program,
		Body_: []*ast.Node{
			{Type: ast.VarDecl, IsConst: true, Name: "empty", Init: &ast.Node{Type: ast.StringLit, Value: ""}},
			{Type: ast.VarDecl, IsConst: true, Name: "text", Init: &ast.Node{Type: ast.StringLit, Value: "a\"b\nc"}},
		},
	}
	out := Format(tree)
	if !strings.Contains(out, `val empty = ""`) {
		t.Fatalf("missing empty string: %q", out)
	}
	if !strings.Contains(out, "val text = \"a\\\"b\\nc\"") {
		t.Fatalf("string literal changed: %q", out)
	}
}

func TestFormatPreservesCompiledAstConstructs(t *testing.T) {
	mainBody := &ast.Node{Type: ast.Block}
	mainBody.Body_ = append(mainBody.Body_, &ast.Node{
		Type:    ast.VarDecl,
		IsConst: true,
		Name:    "items",
		Init: &ast.Node{Type: ast.ArrayLit, Elements: []*ast.Node{
			{Type: ast.NumberLit, Value: "1"},
			nil,
			{Type: ast.StringLit, Value: ""},
		}},
	})
	mainBody.Body_ = append(mainBody.Body_, &ast.Node{
		Type: ast.ExprStmt,
		Expr: &ast.Node{
			Type:   ast.CallExpr,
			Callee: &ast.Node{Type: ast.MemberExpr, Object: &ast.Node{Type: ast.Identifier, Name: "items"}, Prop: "push"},
			Args:   []*ast.Node{{Type: ast.StringLit, Value: ""}},
		},
	})
	mainBody.Body_ = append(mainBody.Body_, &ast.Node{
		Type: ast.ExprStmt,
		Expr: &ast.Node{Type: ast.RangeExpr, Args: []*ast.Node{
			{Type: ast.NumberLit, Value: "0"},
			{Type: ast.NumberLit, Value: "3"},
		}},
	})
	tree := &ast.Node{
		Type:  ast.Program,
		Body_: []*ast.Node{{Type: ast.FnDecl, Name: "main", Params: []*ast.Param{{Name: "value"}}, Body: mainBody}},
	}
	out := Format(tree)
	for _, want := range []string{`val items = [1, , ""]`, `items.push("")`, `range(0, 3)`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}
