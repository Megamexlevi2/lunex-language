package jit

import (
	"fmt"
	"math"
	goruntime "runtime"
)

func (p *Program) Resume(ctx *Context) error {
	if p == nil {
		return fmt.Errorf("nil native program")
	}
	if ctx == nil {
		return fmt.Errorf("nil native context")
	}
	p.runMu.RLock()
	defer p.runMu.RUnlock()
	if p.closed {
		return ErrProgramClosed
	}
	return p.resumeLocked(ctx)
}

func (p *Program) resumeLocked(ctx *Context) error {
	if ctx.Status != StatusSideExit {
		return fmt.Errorf("native resume requires side-exit status")
	}
	pc := int(ctx.ResumePC)
	depth := int(ctx.StackDepth)
	if pc < 0 || pc >= len(p.Bytecode) || depth < 0 || depth > MaxStack {
		return fmt.Errorf("invalid native resume state pc=%d depth=%d", pc, depth)
	}
	ctx.Status = StatusOK
	ctx.ErrorNode = -1
	stack := ctx.Stack[:depth]
	labelIndex := make(map[int]int, len(p.Labels))
	for label, index := range p.Labels {
		if index < 0 || index > len(p.Bytecode) {
			return fmt.Errorf("invalid native label %d", label)
		}
		labelIndex[label] = index
	}
	push := func(value float64) error {
		if len(stack) >= MaxStack {
			return fmt.Errorf("native resume stack overflow")
		}
		stack = append(stack, value)
		return nil
	}
	pop := func() (float64, error) {
		if len(stack) == 0 {
			return 0, fmt.Errorf("native resume stack underflow")
		}
		index := len(stack) - 1
		value := stack[index]
		stack = stack[:index]
		return value, nil
	}
	jump := func(label int) error {
		target, ok := labelIndex[label]
		if !ok {
			return fmt.Errorf("missing native label %d", label)
		}
		pc = target
		return nil
	}
	for steps := 0; steps < oracleStepLimit; steps++ {
		if pc < 0 || pc >= len(p.Bytecode) {
			ctx.StackDepth = int64(len(stack))
			return nil
		}
		instructionPC := pc
		instruction := p.Bytecode[pc]
		pc++
		switch instruction.Op {
		case OpPushConst:
			if instruction.A < 0 || instruction.A >= len(ctx.Consts) {
				return fmt.Errorf("invalid constant index %d", instruction.A)
			}
			if err := push(ctx.Consts[instruction.A]); err != nil {
				return err
			}
		case OpLoadSlot:
			if instruction.A < 0 || instruction.A >= MaxSlots {
				return fmt.Errorf("invalid slot index %d", instruction.A)
			}
			if err := push(ctx.Slots[instruction.A]); err != nil {
				return err
			}
		case OpStoreSlot:
			value, err := pop()
			if err != nil {
				return err
			}
			if instruction.A < 0 || instruction.A >= MaxSlots {
				return fmt.Errorf("invalid slot index %d", instruction.A)
			}
			ctx.Slots[instruction.A] = value
		case OpStoreSlotKeep:
			if len(stack) == 0 || instruction.A < 0 || instruction.A >= MaxSlots {
				return fmt.Errorf("invalid store-slot-keep state")
			}
			ctx.Slots[instruction.A] = stack[len(stack)-1]
		case OpDup:
			if len(stack) == 0 {
				return fmt.Errorf("native resume stack underflow")
			}
			if err := push(stack[len(stack)-1]); err != nil {
				return err
			}
		case OpSwap:
			if len(stack) < 2 {
				return fmt.Errorf("native resume stack underflow")
			}
			last := len(stack) - 1
			stack[last], stack[last-1] = stack[last-1], stack[last]
		case OpPop:
			if _, err := pop(); err != nil {
				return err
			}
		case OpAdd, OpSub, OpMul, OpDiv, OpMod, OpCompare:
			right, err := pop()
			if err != nil {
				return err
			}
			left, err := pop()
			if err != nil {
				return err
			}
			var result float64
			switch instruction.Op {
			case OpAdd:
				result = left + right
			case OpSub:
				result = left - right
			case OpMul:
				result = left * right
			case OpDiv:
				if right == 0 {
					ctx.Status = StatusDivisionByZero
					ctx.ErrorNode = int64(instruction.ErrorNode)
					ctx.StackDepth = int64(len(stack))
					return nil
				}
				result = left / right
			case OpMod:
				if right == 0 || math.IsNaN(right) {
					ctx.Status = StatusDivisionByZero
					ctx.ErrorNode = int64(instruction.ErrorNode)
					ctx.StackDepth = int64(len(stack))
					return nil
				}
				result = math.Mod(left, right)
			default:
				result = boolFloat(compareValues(CompareOp(instruction.A), left, right))
			}
			if err := push(result); err != nil {
				return err
			}
		case OpNeg:
			if len(stack) == 0 {
				return fmt.Errorf("native resume stack underflow")
			}
			stack[len(stack)-1] = -stack[len(stack)-1]
		case OpMath:
			arity := mathFunctionArity(instruction.A)
			if arity == 1 {
				value, err := pop()
				if err != nil {
					return err
				}
				if err := push(nativeMathValue(instruction.A, value, 0)); err != nil {
					return err
				}
			} else if arity == 2 {
				right, err := pop()
				if err != nil {
					return err
				}
				left, err := pop()
				if err != nil {
					return err
				}
				if err := push(nativeMathValue(instruction.A, left, right)); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("unsupported native math operation %d", instruction.A)
			}
		case OpJump:
			if err := jump(instruction.Target); err != nil {
				return err
			}
		case OpBranchFalse, OpBranchTrue:
			value, err := pop()
			if err != nil {
				return err
			}
			falsy := value == 0 || math.IsNaN(value)
			if (instruction.Op == OpBranchFalse && falsy) || (instruction.Op == OpBranchTrue && !falsy) {
				if err := jump(instruction.Target); err != nil {
					return err
				}
			}
		case OpTick:
			n := int64(instruction.A)
			if n <= 0 {
				continue
			}
			ctx.Used += n
			if ctx.Budget >= 0 {
				if ctx.Budget < n {
					ctx.Status = StatusTimeout
					ctx.ErrorNode = int64(instruction.ErrorNode)
					ctx.StackDepth = int64(len(stack))
					return nil
				}
				ctx.Budget -= n
			}
		case OpHotPoll:
			if p.Tier < 2 {
				ctx.HotCount++
				if ctx.HotCount >= HotLoopPollLimit {
					ctx.Status = StatusTierUp
					ctx.ErrorNode = int64(instruction.ErrorNode)
					ctx.StackDepth = int64(len(stack))
					return nil
				}
			}
		case OpSetRepeatCounter:
			value, err := pop()
			if err != nil {
				return err
			}
			if instruction.A < 0 || instruction.A >= MaxCounter {
				return fmt.Errorf("invalid repeat counter index %d", instruction.A)
			}
			ctx.Counters[instruction.A] = int64(value)
		case OpRepeatCheck:
			if instruction.A < 0 || instruction.A >= MaxCounter {
				return fmt.Errorf("invalid repeat counter index %d", instruction.A)
			}
			if ctx.Counters[instruction.A] == 0 {
				if err := jump(instruction.Target); err != nil {
					return err
				}
			}
		case OpRepeatDecrement:
			if instruction.A < 0 || instruction.A >= MaxCounter {
				return fmt.Errorf("invalid repeat counter index %d", instruction.A)
			}
			if ctx.Counters[instruction.A] > 0 {
				ctx.Counters[instruction.A]--
			}
		case OpBreak:
			ctx.Status = StatusBreak
			ctx.ErrorNode = int64(instruction.ErrorNode)
			if err := jump(instruction.Target); err != nil {
				return err
			}
		case OpCallSelf:
			if !p.FunctionMode || instruction.A < 0 || instruction.A != len(p.ParamSlots) || instruction.A > len(stack) {
				return fmt.Errorf("invalid native self-call at %d", instructionPC)
			}
			if ctx.FrameDepth >= ctx.FrameLimit {
				ctx.Status = StatusCallDepth
				ctx.ErrorNode = int64(instruction.ErrorNode)
				ctx.StackDepth = int64(len(stack))
				return nil
			}
			argumentStart := len(stack) - instruction.A
			child := ctx.frameAt(ctx.FrameIndex + 1)
			var ownedFrames []Context
			if child == nil {
				frameCount := int(ctx.FrameLimit - ctx.FrameDepth)
				if frameCount < 1 {
					ctx.Status = StatusCallDepth
					ctx.ErrorNode = int64(instruction.ErrorNode)
					ctx.StackDepth = int64(len(stack))
					return nil
				}
				ownedFrames = make([]Context, frameCount)
				base := &ownedFrames[0]
				for frameIndex := range ownedFrames {
					frame := &ownedFrames[frameIndex]
					for constantIndex, constant := range p.Constants {
						frame.Consts[constantIndex] = constant
					}
					frame.Budget = ctx.Budget
					frame.Used = ctx.Used
					frame.Status = StatusOK
					frame.ErrorNode = -1
					frame.FrameDepth = ctx.FrameDepth + 1 + int64(frameIndex)
					frame.FrameLimit = ctx.FrameLimit
					frame.FrameBase = base
					frame.FrameCount = int64(frameCount)
					frame.FrameIndex = int64(frameIndex)
				}
				child = &ownedFrames[0]
			}
			if child.Status != StatusSideExit {
				for constantIndex, constant := range p.Constants {
					child.Consts[constantIndex] = constant
				}
				child.Budget = ctx.Budget
				child.Used = ctx.Used
				child.Status = StatusOK
				child.ErrorNode = -1
				child.Result = 0
				child.Counters = [MaxCounter]int64{}
				child.FrameDepth = ctx.FrameDepth + 1
				child.FrameLimit = ctx.FrameLimit
				if ctx.FrameBase != nil {
					child.FrameBase = ctx.FrameBase
					child.FrameCount = ctx.FrameCount
					child.FrameIndex = ctx.FrameIndex + 1
				}
				for index, slot := range p.ParamSlots {
					child.Slots[slot] = stack[argumentStart+index]
				}
				if err := p.runLocked(child); err != nil {
					goruntime.KeepAlive(ownedFrames)
					return err
				}
			}
			if child.Status == StatusSideExit {
				if err := p.resumeLocked(child); err != nil {
					goruntime.KeepAlive(ownedFrames)
					return err
				}
			}
			goruntime.KeepAlive(ownedFrames)
			ctx.Budget = child.Budget
			ctx.Used = child.Used
			if child.Status != StatusOK {
				ctx.Status = child.Status
				ctx.ErrorNode = int64(instruction.ErrorNode)
				ctx.StackDepth = int64(len(stack))
				return nil
			}
			stack = stack[:argumentStart]
			if err := push(child.Result); err != nil {
				return err
			}

		case OpReturnValue:
			value, err := pop()
			if err != nil {
				return err
			}
			ctx.Result = value
			ctx.StackDepth = int64(len(stack))
			return nil
		case OpReturn:
			ctx.StackDepth = int64(len(stack))
			return nil
		default:
			return fmt.Errorf("unsupported native resume opcode %d", instruction.Op)
		}
	}
	ctx.Status = StatusDeopt
	ctx.StackDepth = int64(len(stack))
	return nil
}

func compareValues(operation CompareOp, left, right float64) bool {
	if math.IsNaN(left) || math.IsNaN(right) {
		return operation == CompareNE
	}
	switch operation {
	case CompareLT:
		return left < right
	case CompareLE:
		return left <= right
	case CompareGT:
		return left > right
	case CompareGE:
		return left >= right
	case CompareEQ:
		return left == right
	default:
		return left != right
	}
}
