package jit

import (
	"lunex/internal/ast"
	"math"
	goruntime "runtime"
	"testing"
)

func number(v string) *ast.Node { return &ast.Node{Type: ast.NumberLit, Value: v} }
func ident(slot int, name string) *ast.Node {
	return &ast.Node{Type: ast.Identifier, Name: name, ResolvedAddr: &ast.ResolvedAddr{Hops: 0, Slot: slot}}
}
func block(nodes ...*ast.Node) *ast.Node { return &ast.Node{Type: ast.Block, Body_: nodes} }
func expr(e *ast.Node) *ast.Node         { return &ast.Node{Type: ast.ExprStmt, Expr: e} }
func binary(op string, l, r *ast.Node) *ast.Node {
	return &ast.Node{Type: ast.BinaryExpr, Op: op, Left: l, Right: r}
}
func assign(op string, l, r *ast.Node) *ast.Node {
	return &ast.Node{Type: ast.AssignExpr, Op: op, Left: l, Right: r}
}
func unary(op string, arg *ast.Node, prefix bool) *ast.Node {
	return &ast.Node{Type: ast.UnaryExpr, Op: op, Arg: arg, Prefix: prefix}
}

func runLoopTest(t *testing.T, loop *ast.Node, slot int, initial float64, want float64, tier int) {
	t.Helper()
	p, err := CompileLoop(loop, tier)
	if err != nil {
		t.Fatal(err)
	}
	ctx := p.NewContext()
	ctx.Budget = -1
	ctx.Slots[0] = initial
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.Status != StatusOK {
		t.Fatalf("status=%d", ctx.Status)
	}
	if math.Float64bits(ctx.Slots[0]) != math.Float64bits(want) {
		t.Fatalf("got=%v want=%v", ctx.Slots[0], want)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}

func whileLoop(test *ast.Node, body *ast.Node) *ast.Node {
	return &ast.Node{Type: ast.WhileStmt, Test: test, Body: body}
}

func TestNativeLoopVariants(t *testing.T) {
	cases := []struct {
		name    string
		loop    *ast.Node
		initial float64
		want    float64
	}{
		{"while_increment", whileLoop(binary("<", ident(0, "i"), number("1000")), block(expr(unary("++", ident(0, "i"), true)))), 0, 1000},
		{"while_decrement", whileLoop(binary(">", ident(0, "i"), number("0")), block(expr(unary("--", ident(0, "i"), true)))), 1000, 0},
		{"while_add", whileLoop(binary("<", ident(0, "i"), number("100")), block(expr(assign("+=", ident(0, "i"), number("2"))))), 0, 100},
		{"while_sub", whileLoop(binary(">", ident(0, "i"), number("0")), block(expr(assign("-=", ident(0, "i"), number("3"))))), 100, -2},
		{"while_mul", whileLoop(binary("<", ident(0, "i"), number("100")), block(expr(assign("*=", ident(0, "i"), number("2"))))), 1, 128},
		{"while_div", whileLoop(binary(">", ident(0, "i"), number("0.5")), block(expr(assign("/=", ident(0, "i"), number("2"))))), 16, 0.5},
		{"if_true", whileLoop(binary("<", ident(0, "i"), number("20")), block(&ast.Node{Type: ast.IfStmt, Test: binary("<", ident(0, "i"), number("10")), Consequent: expr(assign("+=", ident(0, "i"), number("1"))), Alternate: expr(assign("+=", ident(0, "i"), number("2")))})), 0, 20},
		{"unless", whileLoop(binary("<", ident(0, "i"), number("20")), block(&ast.Node{Type: ast.UnlessStmt, Test: binary("<", ident(0, "i"), number("10")), Consequent: expr(assign("+=", ident(0, "i"), number("2"))), Alternate: expr(assign("+=", ident(0, "i"), number("1")))})), 0, 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runLoopTest(t, tc.loop, 0, tc.initial, tc.want, 1) })
	}
}

func TestPrefixPostfixAndAssignment(t *testing.T) {
	for _, prefix := range []bool{true, false} {
		loop := whileLoop(binary("<", ident(0, "i"), number("20")), block(expr(unary("++", ident(0, "i"), prefix))))
		runLoopTest(t, loop, 0, 0, 20, 1)
	}

	left := ident(0, "i")
	right := unary("++", ident(0, "i"), false)
	assignNode := assign("=", ident(1, "j"), right)
	loop := whileLoop(binary("<", left, number("1")), block(expr(assignNode)))
	p, err := CompileLoop(loop, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx := p.NewContext()
	ctx.Budget = -1
	ctx.Slots[0] = 0
	ctx.Slots[1] = 0
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.Slots[0] != 1 || ctx.Slots[1] != 0 {
		t.Fatalf("postfix result wrong: i=%v j=%v", ctx.Slots[0], ctx.Slots[1])
	}
	_ = p.Close()
}

func TestNestedLoopsBreakContinue(t *testing.T) {
	inner := whileLoop(binary("<", ident(1, "j"), number("10")), block(
		expr(unary("++", ident(1, "j"), true)),
		&ast.Node{Type: ast.IfStmt, Test: binary("==", ident(1, "j"), number("5")), Consequent: &ast.Node{Type: ast.ContinueStmt}},
		&ast.Node{Type: ast.IfStmt, Test: binary("==", ident(1, "j"), number("8")), Consequent: &ast.Node{Type: ast.BreakStmt}},
	))
	outer := whileLoop(binary("<", ident(0, "i"), number("3")), block(inner, expr(unary("++", ident(0, "i"), true))))
	p, err := CompileLoop(outer, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx := p.NewContext()
	ctx.Budget = -1
	ctx.Slots[0] = 0
	ctx.Slots[1] = 0
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.Status != StatusOK || ctx.Slots[0] != 3 || ctx.Slots[1] != 10 {
		t.Fatalf("status=%d i=%v j=%v", ctx.Status, ctx.Slots[0], ctx.Slots[1])
	}
	_ = p.Close()
}

func TestRepeat(t *testing.T) {
	loop := &ast.Node{Type: ast.RepeatStmt, Body: block(expr(unary("++", ident(0, "i"), true)))}
	p, err := CompileLoop(loop, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx := p.NewContext()
	ctx.Budget = -1
	ctx.Counters[0] = 1000
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.Slots[0] != 1000 {
		t.Fatalf("got %v", ctx.Slots[0])
	}
	_ = p.Close()
}

func TestInfiniteBreak(t *testing.T) {
	loop := &ast.Node{Type: ast.LoopStmt, Body: block(
		expr(unary("++", ident(0, "i"), true)),
		&ast.Node{Type: ast.IfStmt, Test: binary(">=", ident(0, "i"), number("99")), Consequent: &ast.Node{Type: ast.BreakStmt}},
	)}
	p, err := CompileLoop(loop, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx := p.NewContext()
	ctx.Budget = -1
	ctx.Slots[0] = 0
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.Status != StatusBreak || ctx.Slots[0] != 99 {
		t.Fatalf("status=%d i=%v", ctx.Status, ctx.Slots[0])
	}
	_ = p.Close()
}

func TestModuloInteger(t *testing.T) {
	loop := whileLoop(binary("<", ident(0, "i"), number("10")), block(expr(assign("+=", ident(0, "i"), binary("%", number("17"), number("5"))))))
	runLoopTest(t, loop, 0, 0, 10, 1)
}

func TestModuloDeopt(t *testing.T) {
	loop := whileLoop(binary("<", ident(0, "i"), number("1")), block(expr(assign("=", ident(0, "i"), binary("%", number("1e308"), number("2"))))))
	p, err := CompileLoop(loop, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx := p.NewContext()
	ctx.Budget = -1
	ctx.Slots[0] = 0
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.Status != StatusDeopt {
		t.Fatalf("status=%d", ctx.Status)
	}
	_ = p.Close()
}

func TestDivisionByZero(t *testing.T) {
	loop := whileLoop(binary("<", ident(0, "i"), number("1")), block(expr(assign("=", ident(0, "i"), binary("/", number("1"), number("0"))))))
	p, err := CompileLoop(loop, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx := p.NewContext()
	ctx.Budget = -1
	ctx.Slots[0] = 0
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.Status != StatusDivisionByZero {
		t.Fatalf("status=%d", ctx.Status)
	}
	_ = p.Close()
}

func TestBudget(t *testing.T) {
	loop := whileLoop(binary("<", ident(0, "i"), number("100")), block(expr(unary("++", ident(0, "i"), true))))
	p, err := CompileLoop(loop, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx := p.NewContext()
	ctx.Budget = 8
	ctx.Slots[0] = 0
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.Status != StatusTimeout {
		t.Fatalf("status=%d used=%d", ctx.Status, ctx.Used)
	}
	if ctx.Used != 9 {
		t.Fatalf("used=%d", ctx.Used)
	}
	_ = p.Close()
}

func TestTier2ConstantFold(t *testing.T) {
	loop := whileLoop(binary("<", ident(0, "i"), number("10")), block(expr(assign("+=", ident(0, "i"), binary("+", number("1"), binary("*", number("2"), number("3")))))))
	p1, err := CompileLoop(loop, 1)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := CompileLoop(loop, 2)
	if err != nil {
		t.Fatal(err)
	}
	c1 := p1.NewContext()
	c2 := p2.NewContext()
	c1.Budget = -1
	c2.Budget = -1
	if err := p1.Run(c1); err != nil {
		t.Fatal(err)
	}
	if err := p2.Run(c2); err != nil {
		t.Fatal(err)
	}
	if c1.Slots[0] != c2.Slots[0] || c1.Status != c2.Status {
		t.Fatalf("tier mismatch %v/%v %d/%d", c1.Slots[0], c2.Slots[0], c1.Status, c2.Status)
	}
	_ = p1.Close()
	_ = p2.Close()
}

func TestNonSubsetRejected(t *testing.T) {
	loop := whileLoop(binary("<", ident(0, "i"), number("10")), block(expr(&ast.Node{Type: ast.CallExpr})))
	if _, err := CompileLoop(loop, 1); err == nil {
		t.Fatal("expected rejection")
	}
}

func TestComparisonMatrix(t *testing.T) {
	ops := []string{"<", "<=", ">", ">=", "==", "!="}
	vals := [][2]float64{{1, 2}, {2, 2}, {3, 2}}
	for _, op := range ops {
		for _, v := range vals {
			condition := binary(op, ident(0, "i"), number("2"))
			loop := whileLoop(condition, block(&ast.Node{Type: ast.BreakStmt}))
			p, err := CompileLoop(loop, 1)
			if err != nil {
				t.Fatal(err)
			}
			c := p.NewContext()
			c.Budget = -1
			c.Slots[0] = v[0]
			_ = v[1]
			if err := p.Run(c); err != nil {
				t.Fatal(err)
			}
			wantStatus := StatusOK
			v0 := v[0]
			truthy := false
			switch op {
			case "<":
				truthy = v0 < 2
			case "<=":
				truthy = v0 <= 2
			case ">":
				truthy = v0 > 2
			case ">=":
				truthy = v0 >= 2
			case "==":
				truthy = v0 == 2
			case "!=":
				truthy = v0 != 2
			}
			if truthy {
				wantStatus = StatusBreak
			}
			if c.Status != wantStatus {
				t.Fatalf("op=%s status=%d want=%d", op, c.Status, wantStatus)
			}
			_ = p.Close()
		}
	}
}

func TestForLoop(t *testing.T) {
	loop := &ast.Node{
		Type:  ast.ForStmt,
		Test:  binary("<", ident(0, "i"), number("10")),
		Right: unary("++", ident(0, "i"), true),
		Body:  block(),
	}
	runLoopTest(t, loop, 0, 0, 10, 1)
}

func TestNestedRepeat(t *testing.T) {
	inner := &ast.Node{Type: ast.RepeatStmt, Count: number("3"), Body: block(expr(unary("++", ident(0, "i"), true)))}
	outer := &ast.Node{Type: ast.RepeatStmt, Count: number("4"), Body: block(inner)}
	p, err := CompileLoop(outer, 1)
	if err != nil {
		t.Fatal(err)
	}
	c := p.NewContext()
	c.Budget = -1
	c.Counters[0] = 4
	if err := p.Run(c); err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusOK || c.Slots[0] != 12 {
		t.Fatalf("status=%d i=%v", c.Status, c.Slots[0])
	}
	_ = p.Close()
}

func TestFractionalModulo(t *testing.T) {
	loop := whileLoop(binary("<", ident(0, "i"), number("1")), block(expr(assign("=", ident(0, "i"), binary("%", number("5.5"), number("2"))))))
	runLoopTest(t, loop, 0, 0, 1.5, 1)
}

func TestNegativeModulo(t *testing.T) {
	loop := whileLoop(binary("<", ident(0, "i"), number("1")), block(
		expr(assign("=", ident(0, "i"), binary("%", number("-5.5"), number("2")))),
		&ast.Node{Type: ast.BreakStmt},
	))
	p, err := CompileLoop(loop, 1)
	if err != nil {
		t.Fatal(err)
	}
	c := p.NewContext()
	c.Budget = -1
	if err := p.Run(c); err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusBreak || math.Float64bits(c.Slots[0]) != math.Float64bits(-1.5) {
		t.Fatalf("status=%d i=%v", c.Status, c.Slots[0])
	}
	_ = p.Close()
}

func TestTier2BranchFold(t *testing.T) {
	loop := whileLoop(binary("<", ident(0, "i"), number("3")), block(&ast.Node{
		Type:       ast.IfStmt,
		Test:       binary("==", number("1"), number("1")),
		Consequent: expr(unary("++", ident(0, "i"), true)),
		Alternate:  expr(assign("+=", ident(0, "i"), number("100"))),
	}))
	p1, err := CompileLoop(loop, 1)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := CompileLoop(loop, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(p2.Bytecode) >= len(p1.Bytecode) {
		t.Fatalf("tier2 did not reduce bytecode: %d >= %d", len(p2.Bytecode), len(p1.Bytecode))
	}
	c1, c2 := p1.NewContext(), p2.NewContext()
	c1.Budget, c2.Budget = -1, -1
	if err := p1.Run(c1); err != nil {
		t.Fatal(err)
	}
	if err := p2.Run(c2); err != nil {
		t.Fatal(err)
	}
	if math.Float64bits(c1.Slots[0]) != math.Float64bits(c2.Slots[0]) || c1.Status != c2.Status {
		t.Fatalf("tier mismatch %v/%v %d/%d", c1.Slots[0], c2.Slots[0], c1.Status, c2.Status)
	}
	_ = p1.Close()
	_ = p2.Close()
}

func TestNaNCondition(t *testing.T) {
	loop := whileLoop(number("NaN"), block(&ast.Node{Type: ast.BreakStmt}))
	p, err := CompileLoop(loop, 2)
	if err != nil {
		t.Fatal(err)
	}
	c := p.NewContext()
	c.Budget = -1
	if err := p.Run(c); err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusOK {
		t.Fatalf("status=%d", c.Status)
	}
	_ = p.Close()
}

func TestOverflowModuloFallsBack(t *testing.T) {
	loop := whileLoop(binary("<", ident(0, "i"), number("1")), block(expr(assign("=", ident(0, "i"), binary("%", number("1e308"), number("2"))))))
	p, err := CompileLoop(loop, 1)
	if err != nil {
		t.Fatal(err)
	}
	c := p.NewContext()
	c.Budget = -1
	if err := p.Run(c); err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusDeopt {
		t.Fatalf("status=%d", c.Status)
	}
	_ = p.Close()
}

func TestBudgetVariants(t *testing.T) {
	loop := whileLoop(binary("<", ident(0, "i"), number("100")), block(expr(unary("++", ident(0, "i"), true))))
	for _, budget := range []int64{1, 2, 5, 20} {
		p, err := CompileLoop(loop, 1)
		if err != nil {
			t.Fatal(err)
		}
		c := p.NewContext()
		c.Budget = budget
		if err := p.Run(c); err != nil {
			t.Fatal(err)
		}
		if c.Status != StatusTimeout {
			t.Fatalf("budget=%d status=%d", budget, c.Status)
		}
		if c.Used != budget+1 {
			t.Fatalf("budget=%d used=%d", budget, c.Used)
		}
		_ = p.Close()
	}
}

func TestGCStress(t *testing.T) {
	loop := whileLoop(binary("<", ident(0, "i"), number("10000")), block(expr(unary("++", ident(0, "i"), true))))
	p, err := CompileLoop(loop, 2)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 64; n++ {
		c := p.NewContext()
		c.Budget = -1
		if err := p.Run(c); err != nil {
			t.Fatal(err)
		}
		if c.Status != StatusOK || c.Slots[0] != 10000 {
			t.Fatalf("run=%d status=%d i=%v", n, c.Status, c.Slots[0])
		}
		goruntime.GC()
	}
	_ = p.Close()
}

func TestTickCoalescingKeepsAccounting(t *testing.T) {
	loop := whileLoop(binary("<", ident(0, "i"), number("500")), block(
		expr(assign("+=", ident(1, "total"), ident(0, "i"))),
		expr(unary("++", ident(0, "i"), true)),
	))
	p1, err := CompileLoop(loop, 1)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := CompileLoop(loop, 2)
	if err != nil {
		t.Fatal(err)
	}
	c1 := p1.NewContext()
	c2 := p2.NewContext()
	c1.Budget = -1
	c2.Budget = -1
	if err := p1.Run(c1); err != nil {
		t.Fatal(err)
	}
	if err := p2.Run(c2); err != nil {
		t.Fatal(err)
	}
	if c1.Status != StatusOK || c2.Status != StatusOK {
		t.Fatalf("status %d/%d", c1.Status, c2.Status)
	}
	if c1.Used != c2.Used {
		t.Fatalf("tick accounting differs: tier1=%d tier2=%d", c1.Used, c2.Used)
	}
	if c1.Slots[1] != c2.Slots[1] || c2.Slots[1] != 124750 {
		t.Fatalf("total %v/%v", c1.Slots[1], c2.Slots[1])
	}
	_ = p1.Close()
	_ = p2.Close()
}

func TestTickCoalescingBudget(t *testing.T) {
	loop := whileLoop(binary("<", ident(0, "i"), number("100000")), block(expr(unary("++", ident(0, "i"), true))))
	for _, tier := range []int{1, 2} {
		p, err := CompileLoop(loop, tier)
		if err != nil {
			t.Fatal(err)
		}
		for _, budget := range []int64{0, 1, 7, 50, 333} {
			c := p.NewContext()
			c.Budget = budget
			if err := p.Run(c); err != nil {
				t.Fatal(err)
			}
			if c.Status != StatusTimeout {
				t.Fatalf("tier=%d budget=%d status=%d", tier, budget, c.Status)
			}
			if c.Used <= budget {
				t.Fatalf("tier=%d budget=%d used=%d", tier, budget, c.Used)
			}
		}
		_ = p.Close()
	}
}

func TestRepeatContinueConsumesIteration(t *testing.T) {
	for _, tier := range []int{1, 2} {
		loop := &ast.Node{Type: ast.RepeatStmt, Body: block(
			expr(unary("++", ident(0, "i"), true)),
			&ast.Node{Type: ast.IfStmt, Test: binary("<", ident(0, "i"), number("1000")), Consequent: &ast.Node{Type: ast.ContinueStmt}},
			expr(unary("++", ident(1, "j"), true)),
		)}
		p, err := CompileLoop(loop, tier)
		if err != nil {
			t.Fatal(err)
		}
		c := p.NewContext()
		c.Budget = 1000000
		c.Counters[0] = 5
		if err := p.Run(c); err != nil {
			t.Fatal(err)
		}
		if c.Status != StatusOK || c.Slots[0] != 5 || c.Slots[1] != 0 {
			t.Fatalf("tier=%d status=%d i=%v j=%v", tier, c.Status, c.Slots[0], c.Slots[1])
		}
		_ = p.Close()
	}
}

func TestWriteOnlySlotIsPreserved(t *testing.T) {
	for _, tier := range []int{1, 2} {
		loop := whileLoop(binary("<", ident(0, "i"), number("0")), block(expr(assign("=", ident(1, "x"), number("5")))))
		p, err := CompileLoop(loop, tier)
		if err != nil {
			t.Fatal(err)
		}
		c := p.NewContext()
		c.Budget = -1
		c.Slots[0] = 10
		c.Slots[1] = 42
		if err := p.Run(c); err != nil {
			t.Fatal(err)
		}
		if c.Status != StatusOK || c.Slots[1] != 42 {
			t.Fatalf("tier=%d status=%d x=%v", tier, c.Status, c.Slots[1])
		}
		_ = p.Close()
	}
}

func TestComparisonValueIsOne(t *testing.T) {
	for _, tier := range []int{1, 2} {
		loop := whileLoop(binary("<", ident(0, "i"), number("5")), block(
			expr(assign("=", ident(0, "i"), binary("+", ident(0, "i"), binary("<", ident(0, "i"), number("100"))))),
		))
		p, err := CompileLoop(loop, tier)
		if err != nil {
			t.Fatal(err)
		}
		c := p.NewContext()
		c.Budget = -1
		if err := p.Run(c); err != nil {
			t.Fatal(err)
		}
		if c.Status != StatusOK || c.Slots[0] != 5 {
			t.Fatalf("tier=%d status=%d i=%v", tier, c.Status, c.Slots[0])
		}
		_ = p.Close()
	}
}

func TestFastPathMatchesReference(t *testing.T) {
	loop := whileLoop(binary("<", ident(0, "i"), number("2000")), block(
		expr(assign("+=", ident(1, "total"), binary("%", ident(0, "i"), number("7")))),
		expr(unary("++", ident(0, "i"), true)),
	))
	p, err := CompileLoop(loop, 2)
	if err != nil {
		t.Fatal(err)
	}
	ref := p.NewContext()
	ref.Budget = -1
	if err := runMachine(p.Code, ref); err != nil {
		t.Fatal(err)
	}
	if len(p.FastCode) != 0 {
		fast := p.NewContext()
		fast.Budget = -1
		if err := runMachine(p.FastCode, fast); err != nil {
			t.Fatal(err)
		}
		if fast.Status != ref.Status || fast.Used != ref.Used || fast.Slots[0] != ref.Slots[0] || fast.Slots[1] != ref.Slots[1] {
			t.Fatalf("fast/reference mismatch: %v/%v", fast.Slots[:2], ref.Slots[:2])
		}
	}
	_ = p.Close()
}
