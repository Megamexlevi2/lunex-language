package jit

import (
	"fmt"
	"lunex/internal/ast"
	"math"
	"sort"
)

func specializeStableSlots(program *Program, values map[string]float64) {
	if program == nil || len(values) == 0 || len(program.Refs) == 0 {
		return
	}
	writes := make(map[Reference]struct{}, len(program.Writes))
	for _, write := range program.Writes {
		writes[write.Reference] = struct{}{}
	}
	for index, ref := range program.Refs {
		if ref.Hops < 0 || index >= len(program.Reads) || !program.Reads[index] {
			continue
		}
		if _, written := writes[ref]; written {
			continue
		}
		if index >= len(program.RefNames) {
			continue
		}
		value, ok := values[program.RefNames[index]]
		if !ok || math.IsNaN(value) {
			continue
		}
		constant, ok := internOptimizedConstant(program, value)
		if !ok {
			continue
		}
		replaced := false
		for pc := range program.Bytecode {
			instruction := &program.Bytecode[pc]
			if instruction.Op == OpLoadSlot && instruction.A == index {
				instruction.Op = OpPushConst
				instruction.A = constant
				replaced = true
			}
		}
		if replaced {
			program.SpecializationGuards = append(program.SpecializationGuards, SpecializationGuard{Reference: ref, Value: value})
		}
	}
}

func (c *compiler) compileInlineCall(node *ast.Node) (bool, error) {
	if node == nil || node.Type != ast.CallExpr || node.Optional || node.Callee == nil || node.Callee.Type != ast.Identifier || node.Callee.Name == "" || node.Callee.ResolvedAddr == nil {
		return false, nil
	}
	depth := len(c.layers)
	address := node.Callee.ResolvedAddr
	if address.Hops < depth || address.Slot < 0 {
		return false, nil
	}
	inline, ok := c.inlineFunctions[node.Callee.Name]
	if !ok || inline.Identity == nil || len(inline.ParamSlots) != len(node.Args) {
		return false, nil
	}
	expression, wrapper, ok := inlineExpression(inline)
	if !ok || !safeInlineExpression(expression, inline.ParamSlots) {
		return false, nil
	}
	for _, slot := range inline.ParamSlots {
		if slot < 0 {
			return false, nil
		}
	}
	for _, argument := range node.Args {
		if err := c.compileExpr(argument); err != nil {
			return false, err
		}
	}
	layer := c.enterLayer()
	for index := len(inline.ParamSlots) - 1; index >= 0; index-- {
		slot := c.localRef(layer, inline.ParamSlots[index])
		c.emit(Instruction{Op: OpStoreSlot, A: slot})
		c.push(-1)
	}
	if wrapper != nil {
		c.emitTick(wrapper)
	}
	if err := c.compileExpr(expression); err != nil {
		c.leaveLayer()
		return true, err
	}
	c.leaveLayer()
	ref := Reference{Hops: address.Hops - depth, Slot: address.Slot}
	for _, guard := range c.program.InlineGuards {
		if guard.Reference == ref {
			if guard.Expected != inline.Identity {
				return true, nil
			}
			return true, nil
		}
	}
	c.program.InlineGuards = append(c.program.InlineGuards, InlineGuard{Reference: ref, Expected: inline.Identity})
	return true, nil
}

func inlineExpression(inline InlineFunction) (*ast.Node, *ast.Node, bool) {
	body := inline.Body
	if body == nil {
		return nil, nil, false
	}
	if body.Type == ast.Block {
		if len(body.Body_) != 1 || body.Body_[0] == nil {
			return nil, nil, false
		}
		statement := body.Body_[0]
		switch statement.Type {
		case ast.ExprStmt:
			return statement.Expr, statement, statement.Expr != nil
		case ast.ReturnStmt:
			expression, ok := statement.Value.(*ast.Node)
			return expression, statement, ok && expression != nil
		default:
			return nil, nil, false
		}
	}
	if body.Type == ast.ExprStmt {
		return body.Expr, body, body.Expr != nil
	}
	if body.Type == ast.ReturnStmt {
		expression, ok := body.Value.(*ast.Node)
		return expression, body, ok && expression != nil
	}
	return body, nil, true
}

func safeInlineExpression(node *ast.Node, paramSlots []int) bool {
	if node == nil {
		return false
	}
	switch node.Type {
	case ast.NumberLit:
		_, err := parseNumber(node)
		return err == nil
	case ast.Identifier:
		if node.ResolvedAddr == nil || node.ResolvedAddr.Hops != 0 {
			return false
		}
		for _, slot := range paramSlots {
			if slot == node.ResolvedAddr.Slot {
				return true
			}
		}
		return false
	case ast.UnaryExpr:
		return node.Op == "-" && safeInlineExpression(node.Arg, paramSlots)
	case ast.BinaryExpr:
		switch node.Op {
		case "+", "-", "*", "/", "%", "**", "<", "<=", ">", ">=", "==", "!=":
			return safeInlineExpression(node.Left, paramSlots) && safeInlineExpression(node.Right, paramSlots)
		default:
			return false
		}
	case ast.CallExpr:
		id, ok := nativeMathCall(node)
		if !ok || len(node.Args) != mathFunctionArity(id) {
			return false
		}
		for _, arg := range node.Args {
			if !safeInlineExpression(arg, paramSlots) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func constantUnrollCount(node *ast.Node) (int, bool) {
	if node == nil {
		return 0, false
	}
	value, ok := foldConstant(node)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 16 || math.Trunc(value) != value {
		return 0, false
	}
	return int(value), true
}

func chooseHotSlotRegisters(program *Program, registers []byte) map[int]byte {
	if program == nil || len(registers) == 0 {
		return nil
	}
	type slotUse struct {
		slot   int
		reads  int
		writes int
		score  int
	}
	uses := make(map[int]*slotUse)
	for _, instruction := range program.Bytecode {
		var slot int
		switch instruction.Op {
		case OpLoadSlot:
			slot = instruction.A
			use := uses[slot]
			if use == nil {
				use = &slotUse{slot: slot}
				uses[slot] = use
			}
			use.reads++
		case OpStoreSlot, OpStoreSlotKeep:
			slot = instruction.A
			use := uses[slot]
			if use == nil {
				use = &slotUse{slot: slot}
				uses[slot] = use
			}
			use.writes++
		}
	}
	candidates := make([]slotUse, 0, len(uses))
	for _, use := range uses {
		if use.slot < 0 || use.slot >= MaxSlots || use.reads == 0 || use.reads+use.writes < 3 {
			continue
		}
		use.score = use.reads*4 + use.writes*2
		candidates = append(candidates, *use)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].slot < candidates[j].slot
		}
		return candidates[i].score > candidates[j].score
	})
	if len(candidates) > len(registers) {
		candidates = candidates[:len(registers)]
	}
	if len(candidates) == 0 {
		return nil
	}
	mapping := make(map[int]byte, len(candidates))
	for index, use := range candidates {
		mapping[use.slot] = registers[index]
	}
	return mapping
}

func constantRange(node *ast.Node) (float64, float64, int, bool) {
	if node == nil || node.Type != ast.RangeExpr || len(node.Args) < 1 || len(node.Args) > 3 {
		return 0, 0, 0, false
	}
	values := make([]float64, len(node.Args))
	for index, argument := range node.Args {
		value, ok := foldConstant(argument)
		if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, 0, 0, false
		}
		values[index] = value
	}
	if len(values) == 1 {
		if values[0] < 1 {
			return 0, 1, 0, true
		}
		if values[0] > 16 {
			return 0, 1, 0, false
		}
		count := int(values[0])
		return 0, 1, count, count <= 16
	}
	start := values[0]
	end := values[1]
	step := 1.0
	if len(values) == 3 {
		step = values[2]
	}
	if step == 0 {
		return start, step, 0, true
	}
	countValue := math.Max(0, math.Ceil((end-start)/step))
	if math.IsNaN(countValue) || math.IsInf(countValue, 0) || countValue > 16 {
		return start, step, 0, false
	}
	return start, step, int(countValue), true
}

func (c *compiler) compileUnrolledRepeat(node *ast.Node, count int) error {
	if count == 0 {
		return nil
	}
	exit := c.newLabel()
	for iteration := 0; iteration < count; iteration++ {
		continueLabel := c.newLabel()
		c.loops = append(c.loops, loopTarget{breakLabel: exit, continueLabel: continueLabel})
		if err := c.compileStmt(node.Body); err != nil {
			c.loops = c.loops[:len(c.loops)-1]
			return err
		}
		c.loops = c.loops[:len(c.loops)-1]
		c.labels[continueLabel] = len(c.code)
	}
	c.labels[exit] = len(c.code)
	return nil
}

func (c *compiler) compileUnrolledRange(node *ast.Node, start, step float64, count int) error {
	valueSlot := scopeSlot(node.ScopeInfo, node.Name)
	if valueSlot < 0 {
		return fmt.Errorf("loop variable has no scope slot")
	}
	aliasSlot := -1
	if node.Alias != "" {
		aliasSlot = scopeSlot(node.ScopeInfo, node.Alias)
		if aliasSlot < 0 {
			return fmt.Errorf("loop index has no scope slot")
		}
	}
	layer := c.enterLayer()
	valueRef := c.localRef(layer, valueSlot)
	aliasRef := -1
	if aliasSlot >= 0 {
		aliasRef = c.localRef(layer, aliasSlot)
	}
	if node.IsConst {
		c.constLocals[valueRef] = true
		if aliasRef >= 0 {
			c.constLocals[aliasRef] = true
		}
	}
	exit := c.newLabel()
	for iteration := 0; iteration < count; iteration++ {
		c.emitTick(node)
		c.pushConst(start + float64(float64(iteration)*step))
		c.storeSlot(valueRef)
		if aliasRef >= 0 {
			c.pushConst(float64(iteration))
			c.storeSlot(aliasRef)
		}
		continueLabel := c.newLabel()
		c.loops = append(c.loops, loopTarget{breakLabel: exit, continueLabel: continueLabel})
		if err := c.compileStmt(node.Body); err != nil {
			c.loops = c.loops[:len(c.loops)-1]
			c.leaveLayer()
			return err
		}
		c.loops = c.loops[:len(c.loops)-1]
		c.labels[continueLabel] = len(c.code)
	}
	c.labels[exit] = len(c.code)
	c.leaveLayer()
	return nil
}
