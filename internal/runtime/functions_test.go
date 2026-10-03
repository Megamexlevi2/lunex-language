package runtime

import (
	"lunex/internal/ast"
	"lunex/internal/errfmt"
	"lunex/internal/lexer"
	"lunex/internal/parser"
	"lunex/internal/resolver"
	"strings"
	"testing"
)

func runRuntimeSource(t *testing.T, source string) error {
	t.Helper()
	tokens, err := lexer.Tokenize(source, "main.lx")
	if err != nil {
		return err
	}
	tree, err := parser.ParseWithLines(tokens, "main.lx", strings.Split(source, "\n"))
	if err != nil {
		return err
	}
	resolver.Resolve(tree)
	interp := NewInterpreter()
	interp.SetFilename("main.lx")
	interp.SetSourceLines(strings.Split(source, "\n"))
	if _, err := interp.Exec(tree); err != nil {
		return err
	}
	return interp.CallMain()
}

func TestRuntimeRejectsWrongArity(t *testing.T) {
	interp := NewInterpreter()
	fn := FuncVal(&Function{
		Name:   "add",
		Params: []FnParam{{Name: "a"}, {Name: "b"}},
		Body:   &ast.Node{Type: ast.NumberLit, Value: "0"},
		Env:    NewEnvironment(nil),
	})
	_, err := interp.callFunctionValue(fn, []*Value{NumberVal(1), NumberVal(2), NumberVal(3), NumberVal(4)}, nil)
	if err == nil {
		t.Fatal("expected runtime arity error")
	}
	le, ok := err.(*errfmt.LunexError)
	if !ok {
		t.Fatalf("expected LunexError, got %T", err)
	}
	if le.Code != "E0021" {
		t.Fatalf("expected E0021, got %s", le.Code)
	}
	if !strings.Contains(errfmt.Format(le), "error[E0021][type]") {
		t.Fatalf("unexpected formatted error: %s", errfmt.Format(le))
	}
}

func TestRuntimeArityThroughCallExpression(t *testing.T) {
	err := runRuntimeSource(t, `fn add(a, b) { a + b }
fn main() { add(1, 2, 3, 4) }`)
	if err == nil {
		t.Fatal("expected runtime arity error")
	}
	le, ok := err.(*errfmt.LunexError)
	if !ok {
		t.Fatalf("expected LunexError, got %T", err)
	}
	if le.Code != "E0021" {
		t.Fatalf("expected E0021, got %s", le.Code)
	}
	if le.Line == 0 || le.Col == 0 {
		t.Fatalf("expected call-site location, got %d:%d", le.Line, le.Col)
	}
}

func TestRuntimeTypeMismatchUsesErrfmtCode(t *testing.T) {
	err := runRuntimeSource(t, `fn main() { "text" + 5 }`)
	if err == nil {
		t.Fatal("expected runtime type error")
	}
	le, ok := err.(*errfmt.LunexError)
	if !ok {
		t.Fatalf("expected LunexError, got %T", err)
	}
	if le.Code != "E0020" || le.Kind != errfmt.KindType {
		t.Fatalf("expected E0020 TypeError, got code=%s kind=%s", le.Code, le.Kind)
	}
}

func TestRuntimeConstantDivisionByZeroUsesErrfmtCode(t *testing.T) {
	err := runRuntimeSource(t, `fn main() { 1 / 0 }`)
	if err == nil {
		t.Fatal("expected division by zero error")
	}
	le, ok := err.(*errfmt.LunexError)
	if !ok {
		t.Fatalf("expected LunexError, got %T", err)
	}
	if le.Code != "E0030" || le.Kind != errfmt.KindArithmetic {
		t.Fatalf("expected E0030 ArithmeticError, got code=%s kind=%s", le.Code, le.Kind)
	}
}
