package jit

import (
	"lunex/internal/ast"
	"math"
	"testing"
)

func identAt(hops, slot int, name string) *ast.Node {
	return &ast.Node{Type: ast.Identifier, Name: name, ResolvedAddr: &ast.ResolvedAddr{Hops: hops, Slot: slot}}
}

func rangeNode(args ...*ast.Node) *ast.Node {
	return &ast.Node{Type: ast.RangeExpr, Args: args}
}

func forRange(name string, rng *ast.Node, body *ast.Node) *ast.Node {
	return &ast.Node{
		Type:      ast.ForOfStmt,
		Name:      name,
		Right:     rng,
		Body:      body,
		ScopeInfo: &ast.ScopeInfo{Names: []string{name}},
	}
}

func varDecl(name string, slot int, init *ast.Node, isConst bool) *ast.Node {
	return &ast.Node{Type: ast.VarDecl, Name: name, Init: init, IsConst: isConst, ResolvedAddr: &ast.ResolvedAddr{Hops: 0, Slot: slot}}
}

func refIndex(p *Program, hops, slot int) int {
	for i, r := range p.Refs {
		if r.Hops == hops && r.Slot == slot {
			return i
		}
	}
	return -1
}

func referenceRange(start, end, step float64, single bool) (float64, int) {
	var count int
	if single {
		count = int(end)
		start, step = 0, 1
	} else if step == 0 {
		count = 0
	} else {
		count = int(math.Max(0, math.Ceil((end-start)/step)))
	}
	if count < 0 {
		count = 0
	}
	sum := 0.0
	for k := 0; k < count; k++ {
		sum += start + float64(float64(k)*step)
	}
	return sum, count
}

func TestRootRangeLoop(t *testing.T) {
	if !Available() {
		t.Skip("native execution unavailable")
	}
	cases := []struct {
		name               string
		start, end, step   float64
		single             bool
	}{
		{"single", 0, 10, 1, true},
		{"single_zero", 0, 0, 1, true},
		{"two_args", 3, 12, 1, false},
		{"step_three", 0, 10, 3, false},
		{"negative_step", 10, 0, -3, false},
		{"fractional_step", 0, 1, 0.25, false},
		{"empty", 5, 5, 1, false},
		{"zero_step", 0, 10, 0, false},
	}
	for _, tier := range []int{1, 2} {
		for _, tc := range cases {
			loop := forRange("i", rangeNode(number("0")), block(
				expr(assign("+=", identAt(1, 0, "total"), identAt(0, 0, "i"))),
			))
			p, err := CompileLoop(loop, tier)
			if err != nil {
				t.Fatalf("%s tier=%d: %v", tc.name, tier, err)
			}
			if !p.RangeRoot {
				t.Fatalf("%s tier=%d: root range metadata missing", tc.name, tier)
			}
			wantSum, count := referenceRange(tc.start, tc.end, tc.step, tc.single)
			start, step := tc.start, tc.step
			if tc.single {
				start, step = 0, 1
			}
			ctx := p.NewContext()
			ctx.Budget = -1
			ctx.Counters[0] = int64(count)
			ctx.Slots[p.RangeStart] = start
			ctx.Slots[p.RangeStep] = step
			ctx.Slots[p.RangeIndex] = 0
			total := refIndex(p, 0, 0)
			if total < 0 {
				t.Fatalf("%s tier=%d: total reference missing", tc.name, tier)
			}
			if err := p.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if ctx.Status != StatusOK {
				t.Fatalf("%s tier=%d: status %d", tc.name, tier, ctx.Status)
			}
			if math.Abs(ctx.Slots[total]-wantSum) > 1e-9 {
				t.Fatalf("%s tier=%d: got %v want %v", tc.name, tier, ctx.Slots[total], wantSum)
			}
			_ = p.Close()
		}
	}
}

func TestRootRangeResumesMidway(t *testing.T) {
	if !Available() {
		t.Skip("native execution unavailable")
	}
	loop := forRange("i", rangeNode(number("100")), block(
		expr(assign("+=", identAt(1, 0, "total"), identAt(0, 0, "i"))),
	))
	p, err := CompileLoop(loop, 2)
	if err != nil {
		t.Fatal(err)
	}
	ctx := p.NewContext()
	ctx.Budget = -1
	ctx.Counters[0] = 70
	ctx.Slots[p.RangeStart] = 0
	ctx.Slots[p.RangeStep] = 1
	ctx.Slots[p.RangeIndex] = 30
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	want := 0.0
	for k := 30; k < 100; k++ {
		want += float64(k)
	}
	total := refIndex(p, 0, 0)
	if ctx.Status != StatusOK || ctx.Slots[total] != want {
		t.Fatalf("status %d total %v want %v", ctx.Status, ctx.Slots[total], want)
	}
	_ = p.Close()
}

func TestRangeIndexAlias(t *testing.T) {
	if !Available() {
		t.Skip("native execution unavailable")
	}
	loop := &ast.Node{
		Type:      ast.ForOfStmt,
		Name:      "v",
		Alias:     "n",
		Right:     rangeNode(number("5")),
		ScopeInfo: &ast.ScopeInfo{Names: []string{"v", "n"}},
		Body: block(
			expr(assign("+=", identAt(1, 0, "total"), binary("*", identAt(0, 0, "v"), identAt(0, 1, "n")))),
		),
	}
	p, err := CompileLoop(loop, 2)
	if err != nil {
		t.Fatal(err)
	}
	ctx := p.NewContext()
	ctx.Budget = -1
	ctx.Counters[0] = 5
	ctx.Slots[p.RangeStart] = 0
	ctx.Slots[p.RangeStep] = 1
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	total := refIndex(p, 0, 0)
	if ctx.Status != StatusOK || ctx.Slots[total] != 30 {
		t.Fatalf("status %d total %v", ctx.Status, ctx.Slots[total])
	}
	_ = p.Close()
}

func TestNestedRangeLoops(t *testing.T) {
	if !Available() {
		t.Skip("native execution unavailable")
	}
	type rangeCase struct {
		name   string
		args   []*ast.Node
		single bool
		start  float64
		end    float64
		step   float64
	}
	cases := []rangeCase{
		{"const_single", []*ast.Node{number("4")}, true, 0, 4, 1},
		{"variable_single", []*ast.Node{identAt(0, 2, "n")}, true, 0, 5, 1},
		{"const_pair", []*ast.Node{number("2"), number("9")}, false, 2, 9, 1},
		{"variable_pair", []*ast.Node{identAt(0, 2, "n"), number("12")}, false, 5, 12, 1},
		{"step", []*ast.Node{number("0"), number("10"), number("3")}, false, 0, 10, 3},
		{"negative_step", []*ast.Node{number("10"), number("0"), unary("-", number("3"), true)}, false, 10, 0, -3},
		{"fractional", []*ast.Node{number("0"), number("1"), number("0.25")}, false, 0, 1, 0.25},
		{"empty", []*ast.Node{number("5"), number("5")}, false, 5, 5, 1},
		{"zero_step", []*ast.Node{number("0"), number("10"), number("0")}, false, 0, 10, 0},
	}
	for _, tier := range []int{1, 2} {
		for _, tc := range cases {
			inner := forRange("j", rangeNode(tc.args...), block(
				expr(assign("+=", identAt(1, 1, "total"), identAt(0, 0, "j"))),
			))
			outer := whileLoop(binary("<", identAt(0, 0, "i"), number("3")), block(
				inner,
				expr(unary("++", identAt(0, 0, "i"), true)),
			))
			p, err := CompileLoop(outer, tier)
			if err != nil {
				t.Fatalf("%s tier=%d: %v", tc.name, tier, err)
			}
			ctx := p.NewContext()
			ctx.Budget = -1
			if i := refIndex(p, 0, 2); i >= 0 {
				ctx.Slots[i] = 5
			}
			if err := p.Run(ctx); err != nil {
				t.Fatal(err)
			}
			single, _ := referenceRange(tc.start, tc.end, tc.step, tc.single)
			want := 3 * single
			total := refIndex(p, 0, 1)
			if ctx.Status != StatusOK {
				t.Fatalf("%s tier=%d: status %d", tc.name, tier, ctx.Status)
			}
			if math.Abs(ctx.Slots[total]-want) > 1e-9 {
				t.Fatalf("%s tier=%d: got %v want %v", tc.name, tier, ctx.Slots[total], want)
			}
			_ = p.Close()
		}
	}
}

func TestBlockLocalBindings(t *testing.T) {
	if !Available() {
		t.Skip("native execution unavailable")
	}
	for _, tier := range []int{1, 2} {
		loop := whileLoop(binary("<", identAt(0, 0, "i"), number("10")), &ast.Node{
			Type:      ast.Block,
			ScopeInfo: &ast.ScopeInfo{Names: []string{"t"}},
			Body_: []*ast.Node{
				varDecl("t", 0, binary("*", identAt(1, 0, "i"), number("2")), false),
				expr(assign("+=", identAt(1, 1, "total"), identAt(0, 0, "t"))),
				expr(unary("++", identAt(1, 0, "i"), true)),
			},
		})
		p, err := CompileLoop(loop, tier)
		if err != nil {
			t.Fatalf("tier=%d: %v", tier, err)
		}
		ctx := p.NewContext()
		ctx.Budget = -1
		if err := p.Run(ctx); err != nil {
			t.Fatal(err)
		}
		i := refIndex(p, 0, 0)
		total := refIndex(p, 0, 1)
		if ctx.Status != StatusOK || ctx.Slots[i] != 10 || ctx.Slots[total] != 90 {
			t.Fatalf("tier=%d: status %d i=%v total=%v", tier, ctx.Status, ctx.Slots[i], ctx.Slots[total])
		}
		if len(p.Writes) != 2 {
			t.Fatalf("tier=%d: local binding leaked into writes: %d", tier, len(p.Writes))
		}
		_ = p.Close()
	}
}

func TestConstantLocalIsRejected(t *testing.T) {
	loop := whileLoop(binary("<", identAt(0, 0, "i"), number("10")), &ast.Node{
		Type:      ast.Block,
		ScopeInfo: &ast.ScopeInfo{Names: []string{"t"}},
		Body_: []*ast.Node{
			varDecl("t", 0, number("1"), true),
			expr(assign("=", identAt(0, 0, "t"), number("2"))),
			expr(unary("++", identAt(1, 0, "i"), true)),
		},
	})
	if p, err := CompileLoop(loop, 1); err == nil {
		_ = p.Close()
		t.Fatal("assignment to a constant local must not compile")
	}
}

func TestComparisonStoreIsRejected(t *testing.T) {
	loop := whileLoop(binary("<", identAt(0, 0, "i"), number("10")), block(
		expr(assign("=", identAt(0, 1, "flag"), binary("<", identAt(0, 0, "i"), number("5")))),
		expr(unary("++", identAt(0, 0, "i"), true)),
	))
	if p, err := CompileLoop(loop, 1); err == nil {
		_ = p.Close()
		t.Fatal("storing a comparison result must not compile")
	}
}

func TestUnsupportedShapesStayRejected(t *testing.T) {
	call := &ast.Node{Type: ast.CallExpr, Callee: identAt(0, 1, "f")}
	loop := whileLoop(binary("<", identAt(0, 0, "i"), number("10")), block(
		expr(assign("=", identAt(0, 0, "i"), call)),
	))
	if p, err := CompileLoop(loop, 1); err == nil {
		_ = p.Close()
		t.Fatal("calls must not compile")
	}
	bits := whileLoop(binary("<", identAt(0, 0, "i"), number("10")), block(
		expr(assign("=", identAt(0, 0, "i"), binary("&", identAt(0, 0, "i"), number("3")))),
	))
	if p, err := CompileLoop(bits, 1); err == nil {
		_ = p.Close()
		t.Fatal("bitwise operators must not compile")
	}
}
