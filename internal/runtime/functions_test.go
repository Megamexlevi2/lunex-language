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

func TestTieredNativeRecursiveFibFunction(t *testing.T) {
	oldEnabled, oldHot, oldTrace := nativeEnabled, nativeHotLimit, nativeTrace
	loadNativeConfig()
	nativeEnabled = true
	nativeHotLimit = 1
	nativeTrace = false
	defer func() {
		nativeEnabled, nativeHotLimit, nativeTrace = oldEnabled, oldHot, oldTrace
	}()

	source := `fn fib(n) {
  if n < 2 {
    n
  } else {
    fib(n - 1) + fib(n - 2)
  }
}

fn fibLoop(n) {
  if n <= 1 {
    n
  } else {
    var a = 0
    var b = 1
    var i = 2
    while i <= n {
      val tmp = a + b
      a = b
      b = tmp
      i = i + 1
    }
    b
  }
}

fn fibTail(n, a, b) {
  if n <= 0 {
    a
  } else {
    fibTail(n - 1, b, a + b)
  }
}`
	tokens, err := lexer.Tokenize(source, "fib.lx")
	if err != nil {
		t.Fatal(err)
	}
	tree, err := parser.ParseWithLines(tokens, "fib.lx", strings.Split(source, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	resolver.Resolve(tree)
	interp := NewInterpreter()
	interp.SetFilename("fib.lx")
	interp.SetSourceLines(strings.Split(source, "\n"))
	if _, err := interp.Exec(tree); err != nil {
		t.Fatal(err)
	}
	value, ok := interp.topEnv.Get("fib")
	if !ok || value == nil || value.Tag != TypeFunction {
		t.Fatal("fib function was not defined")
	}
	for i := 0; i < 40; i++ {
		result, callErr := interp.callFunctionValue(value, []*Value{NumberVal(10)}, nil)
		if callErr != nil {
			t.Fatalf("fib call %d: %v", i, callErr)
		}
		if result == nil || result.Tag != TypeNumber || result.NumVal != 55 {
			t.Fatalf("fib call %d returned %#v, want 55", i, result)
		}
	}
	interp.jitMu.Lock()
	state := interp.jitFunctions[value.FnVal]
	var tier int
	if state != nil && state.current != nil {
		tier = state.current.Tier
	}
	interp.jitMu.Unlock()
	if tier != 2 {
		t.Fatalf("fib did not promote to tier 2: tier=%d", tier)
	}
	interp.jitMu.Lock()
	state = interp.jitFunctions[value.FnVal]
	if state == nil || state.tier1 == nil || len(state.tier1.Code) == 0 || state.current == nil || !state.current.FunctionMode || !state.current.Recursive || len(state.current.Code) == 0 {
		interp.jitMu.Unlock()
		t.Fatal("recursive Fibonacci did not compile to native machine code in both tiers")
	}
	interp.jitMu.Unlock()

	loopValue, ok := interp.topEnv.Get("fibLoop")
	if !ok || loopValue == nil || loopValue.Tag != TypeFunction {
		t.Fatal("fibLoop function was not defined")
	}
	cases := []struct {
		n    float64
		want float64
	}{
		{0, 0}, {1, 1}, {2, 1}, {3, 2}, {5, 5}, {10, 55}, {20, 6765}, {40, 102334155},
	}
	for i := 0; i < 40; i++ {
		for _, tc := range cases {
			result, callErr := interp.callFunctionValue(loopValue, []*Value{NumberVal(tc.n)}, nil)
			if callErr != nil {
				t.Fatalf("fibLoop(%v): %v", tc.n, callErr)
			}
			if result == nil || result.Tag != TypeNumber || result.NumVal != tc.want {
				t.Fatalf("fibLoop(%v) returned %#v, want %v", tc.n, result, tc.want)
			}
		}
	}
	interp.jitMu.Lock()
	loopState := interp.jitFunctions[loopValue.FnVal]
	loopTier := 0
	if loopState != nil && loopState.current != nil {
		loopTier = loopState.current.Tier
	}
	interp.jitMu.Unlock()
	if loopTier != 2 {
		t.Fatalf("iterative Fibonacci did not promote to tier 2: tier=%d", loopTier)
	}
	interp.jitMu.Lock()
	loopState = interp.jitFunctions[loopValue.FnVal]
	if loopState == nil || loopState.tier1 == nil || len(loopState.tier1.Code) == 0 || loopState.current == nil || !loopState.current.FunctionMode || loopState.current.Recursive || len(loopState.current.Code) == 0 {
		interp.jitMu.Unlock()
		t.Fatal("iterative Fibonacci did not compile to native machine code in both tiers")
	}
	interp.jitMu.Unlock()

	tailValue, ok := interp.topEnv.Get("fibTail")
	if !ok || tailValue == nil || tailValue.Tag != TypeFunction {
		t.Fatal("fibTail function was not defined")
	}
	tailCases := []struct {
		n    float64
		want float64
	}{
		{0, 0}, {1, 1}, {2, 1}, {10, 55}, {20, 6765}, {40, 102334155},
	}
	for i := 0; i < 40; i++ {
		for _, tc := range tailCases {
			result, callErr := interp.callFunctionValue(tailValue, []*Value{NumberVal(tc.n), NumberVal(0), NumberVal(1)}, nil)
			if callErr != nil {
				t.Fatalf("fibTail(%v): %v", tc.n, callErr)
			}
			if result == nil || result.Tag != TypeNumber || result.NumVal != tc.want {
				t.Fatalf("fibTail(%v) returned %#v, want %v", tc.n, result, tc.want)
			}
		}
	}
	interp.jitMu.Lock()
	tailState := interp.jitFunctions[tailValue.FnVal]
	tailTier := 0
	if tailState != nil && tailState.current != nil {
		tailTier = tailState.current.Tier
	}
	if tailState == nil || tailState.tier1 == nil || len(tailState.tier1.Code) == 0 || tailState.current == nil || !tailState.current.FunctionMode || !tailState.current.Recursive || len(tailState.current.Code) == 0 {
		interp.jitMu.Unlock()
		t.Fatal("tail-recursive Fibonacci did not compile to native machine code in both tiers")
	}
	interp.jitMu.Unlock()
	if tailTier != 2 {
		t.Fatalf("tail-recursive Fibonacci did not promote to tier 2: tier=%d", tailTier)
	}
}

func TestHigherOrderCallbacksAcceptCollectionArguments(t *testing.T) {
	err := runRuntimeSource(t, `val io = @import("std.io")
fn main() {
  val nums = [1, 2, 3, 4]
  val doubled = nums.map(fn(x) { x * 2 })
  val evens = nums.filter(fn(x) { x % 2 == 0 })
  val total = nums.reduce(fn(acc, value) { acc + value }, 0)
  if doubled.join(",") != "2,4,6,8" { throw "map callback failed" }
  if evens.join(",") != "2,4" { throw "filter callback failed" }
  if total != 10 { throw "reduce callback failed" }
  io.log("PASS higher_order_callback_arity")
}`)
	if err != nil {
		t.Fatalf("higher-order callbacks should accept the arguments provided by collection methods: %v", err)
	}
}
