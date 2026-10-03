package jit

import "fmt"

func bytecodeStackDepths(p *Program) ([]int, error) {
	depths := make([]int, len(p.Bytecode)+1)
	for i := range depths {
		depths[i] = -1
	}
	queue := []int{0}
	depths[0] = 0

	labelIndex := make(map[int]int, len(p.Labels))
	for label, index := range p.Labels {
		if index < 0 || index > len(p.Bytecode) {
			return nil, fmt.Errorf("invalid native label %d", label)
		}
		labelIndex[label] = index
	}

	delta := func(in Instruction, depth int) (int, error) {
		d := depth
		switch in.Op {
		case OpPushConst, OpLoadSlot, OpDup:
			d++
		case OpStoreSlot, OpPop, OpSetRepeatCounter:
			d--
		case OpStoreSlotKeep, OpSwap, OpNeg, OpJump, OpTick, OpRepeatCheck, OpRepeatDecrement, OpBreak, OpReturn:
		case OpAdd, OpSub, OpMul, OpDiv, OpMod, OpCompare:
			d--
		case OpBranchFalse, OpBranchTrue:
			d--
		default:
			return 0, fmt.Errorf("unsupported stack opcode %d", in.Op)
		}
		if d < 0 {
			return 0, fmt.Errorf("native stack underflow depth=%d op=%d", depth, in.Op)
		}
		return d, nil
	}

	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		if i == len(p.Bytecode) {
			continue
		}
		d := depths[i]
		out, err := delta(p.Bytecode[i], d)
		if err != nil {
			return nil, err
		}
		succ := func(index int) error {
			if index < 0 || index > len(p.Bytecode) {
				return fmt.Errorf("invalid native control-flow target %d", index)
			}
			if depths[index] == -1 {
				depths[index] = out
				queue = append(queue, index)
				return nil
			}
			if depths[index] != out {
				return fmt.Errorf("inconsistent native stack depth at %d: %d and %d", index, depths[index], out)
			}
			return nil
		}

		switch p.Bytecode[i].Op {
		case OpReturn:
			if err := succ(len(p.Bytecode)); err != nil {
				return nil, err
			}
		case OpJump, OpBreak:
			index, ok := labelIndex[p.Bytecode[i].Target]
			if !ok {
				return nil, fmt.Errorf("missing native label %d", p.Bytecode[i].Target)
			}
			if err := succ(index); err != nil {
				return nil, err
			}
		case OpBranchFalse, OpBranchTrue:
			index, ok := labelIndex[p.Bytecode[i].Target]
			if !ok {
				return nil, fmt.Errorf("missing native label %d", p.Bytecode[i].Target)
			}
			if err := succ(index); err != nil {
				return nil, err
			}
			if i+1 < len(p.Bytecode) {
				if err := succ(i + 1); err != nil {
					return nil, err
				}
			} else if err := succ(len(p.Bytecode)); err != nil {
				return nil, err
			}
		default:
			if i+1 < len(p.Bytecode) {
				if err := succ(i + 1); err != nil {
					return nil, err
				}
			} else if err := succ(len(p.Bytecode)); err != nil {
				return nil, err
			}
		}
	}

	for i := range depths {
		if depths[i] < 0 {
			depths[i] = 0
		}
	}
	return depths, nil
}
