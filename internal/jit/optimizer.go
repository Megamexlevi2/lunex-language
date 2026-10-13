package jit

import "math"

func optimizeBytecode(p *Program) {
	if p == nil || len(p.Bytecode) == 0 {
		return
	}
	maxPasses := len(p.Bytecode)
	for pass := 0; pass < maxPasses; pass++ {
		oldCode := p.Bytecode
		targeted := make(map[int]bool, len(p.Labels))
		for _, index := range p.Labels {
			targeted[index] = true
		}
		oldToNew := make([]int, len(oldCode)+1)
		optimized := make([]Instruction, 0, len(oldCode))
		changed := false
		for i := 0; i < len(oldCode); {
			oldToNew[i] = len(optimized)
			in := oldCode[i]
			width := 1
			var replacement Instruction
			replace := false
			remove := false
			if i+1 < len(oldCode) && !targeted[i+1] {
				next := oldCode[i+1]
				switch {
				case in.Op == OpPushConst && next.Op == OpNeg && validConstant(p, in.A):
					if index, ok := internOptimizedConstant(p, -p.Constants[in.A]); ok {
						replacement = Instruction{Op: OpPushConst, A: index}
						replace = true
						width = 2
					}
				case in.Op == OpPushConst && next.Op == OpPop:
					remove = true
					width = 2
				case in.Op == OpDup && next.Op == OpPop:
					remove = true
					width = 2
				case in.Op == OpLoadSlot && next.Op == OpPop:
					remove = true
					width = 2
				case in.Op == OpStoreSlotKeep && next.Op == OpPop:
					replacement = in
					replacement.Op = OpStoreSlot
					replace = true
					width = 2
				case in.Op == OpPushConst && (next.Op == OpBranchFalse || next.Op == OpBranchTrue) && validConstant(p, in.A):
					v := p.Constants[in.A]
					jump := v == 0 || math.IsNaN(v)
					if next.Op == OpBranchTrue {
						jump = !jump
					}
					if jump {
						replacement = Instruction{Op: OpJump, Target: next.Target, ErrorNode: next.ErrorNode}
						replace = true
					} else {
						remove = true
					}
					width = 2
				}
			}
			if !replace && !remove && i+2 < len(oldCode) && !targeted[i+1] && !targeted[i+2] && in.Op == OpPushConst && oldCode[i+1].Op == OpPushConst && validConstant(p, in.A) && validConstant(p, oldCode[i+1].A) {
				a := p.Constants[in.A]
				b := p.Constants[oldCode[i+1].A]
				op := oldCode[i+2]
				var value float64
				fold := true
				switch op.Op {
				case OpAdd:
					value = a + b
				case OpSub:
					value = a - b
				case OpMul:
					value = a * b
				case OpDiv:
					if b == 0 {
						fold = false
					} else {
						value = a / b
					}
				case OpCompare:
					value = boolFloat(compareNumbers(a, b, CompareOp(op.A)))
				default:
					fold = false
				}
				if fold {
					if index, ok := internOptimizedConstant(p, value); ok {
						replacement = Instruction{Op: OpPushConst, A: index}
						replace = true
						width = 3
					}
				}
			}
			for offset := 1; offset < width; offset++ {
				oldToNew[i+offset] = len(optimized)
			}
			switch {
			case replace:
				optimized = append(optimized, replacement)
			case remove:
			default:
				optimized = append(optimized, in)
				width = 1
			}
			if width > 1 {
				changed = true
			}
			i += width
		}
		oldToNew[len(oldCode)] = len(optimized)
		if !changed {
			break
		}
		labels := make(map[int]int, len(p.Labels))
		for label, index := range p.Labels {
			if index >= 0 && index < len(oldToNew) {
				labels[label] = oldToNew[index]
			}
		}
		p.Bytecode = optimized
		p.Labels = labels
	}
	if depths, err := bytecodeStackDepths(p); err == nil {
		maxDepth := 0
		for _, depth := range depths {
			if depth > maxDepth {
				maxDepth = depth
			}
		}
		p.ExpressionStack = maxDepth
	}
}

func validConstant(p *Program, index int) bool {
	return p != nil && index >= 0 && index < len(p.Constants)
}

func internOptimizedConstant(p *Program, value float64) (int, bool) {
	bits := math.Float64bits(value)
	for i, current := range p.Constants {
		if math.Float64bits(current) == bits {
			return i, true
		}
	}
	if len(p.Constants) >= MaxConsts {
		return 0, false
	}
	p.Constants = append(p.Constants, value)
	return len(p.Constants) - 1, true
}

func compareNumbers(a, b float64, op CompareOp) bool {
	switch op {
	case CompareLT:
		return a < b
	case CompareLE:
		return a <= b
	case CompareGT:
		return a > b
	case CompareGE:
		return a >= b
	case CompareEQ:
		return a == b
	default:
		return a != b
	}
}
