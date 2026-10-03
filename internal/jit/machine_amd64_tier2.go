//go:build amd64

package jit

import "fmt"

const (
	t2BudgetReg = 8
	t2UsedReg   = 9
	t2ZeroXmm   = 7
	t2Tmp0Xmm   = 6
	t2Tmp1Xmm   = 6
	t2Tmp2Xmm   = 6
	t2MaxStack  = 6
)

func (w *x86Writer) xorpdXmm(dst, src byte) {
	w.u8(0x66)
	w.u8(0x0F)
	w.u8(0x57)
	w.u8(0xC0 | ((dst & 7) << 3) | (src & 7))
}

func (w *x86Writer) cvttsd2si64(dst byte, src byte) {
	w.u8(0xF2)
	w.u8(x86Rex(true, dst >= 8, false))
	w.u8(0x0F)
	w.u8(0x2C)
	w.u8(0xC0 | ((dst & 7) << 3) | (src & 7))
}

func (w *x86Writer) cvtsi2sd64(dst byte, src byte) {
	w.u8(0xF2)
	w.u8(x86Rex(true, dst >= 8, src >= 8))
	w.u8(0x0F)
	w.u8(0x2A)
	w.u8(0xC0 | ((dst & 7) << 3) | (src & 7))
}

func (w *x86Writer) syncTier2Counters() {
	w.movqStore(regCtx, ctxBudgetOff, t2BudgetReg)
	w.movqStore(regCtx, ctxUsedOff, t2UsedReg)
}

func emitMachineTier2(p *Program) ([]byte, error) {
	depths, err := bytecodeStackDepths(p)
	if err != nil {
		return nil, err
	}
	maxDepth := 0
	for _, d := range depths {
		if d > maxDepth {
			maxDepth = d
		}
	}
	for _, in := range p.Bytecode {
		if in.Op == OpMod {
			return emitMachineLegacy(p)
		}
	}
	if p.NestedLoops || maxDepth > t2MaxStack {
		return emitMachineLegacy(p)
	}

	w := &x86Writer{labels: make(map[int]int)}
	labelAt := make(map[int][]int)
	for label, idx := range p.Labels {
		if idx >= 0 {
			labelAt[idx] = append(labelAt[idx], label)
		}
	}
	localSeq := 0
	newLocal := func() int {
		localSeq++
		return (1 << 30) + localSeq
	}
	returnLabel := 1 << 29
	fail := func(i int, e error) ([]byte, error) {
		return nil, fmt.Errorf("amd64 native optimization instruction %d: %w", i, e)
	}

	w.movqLoad(t2BudgetReg, regCtx, ctxBudgetOff)
	w.movqLoad(t2UsedReg, regCtx, ctxUsedOff)

	for i, in := range p.Bytecode {
		for _, label := range labelAt[i] {
			w.mark(label)
		}
		d := depths[i]
		x := func(index int) byte { return byte(index) }
		if d > t2MaxStack {
			return emitMachineLegacy(p)
		}
		switch in.Op {
		case OpPushConst:
			w.movsdLoad(x(d), regCtx, ctxConstsOff+in.A*8)
		case OpLoadSlot:
			w.movsdLoad(x(d), regCtx, ctxSlotsOff+in.A*8)
		case OpStoreSlot:
			w.movsdStore(regCtx, ctxSlotsOff+in.A*8, x(d-1))
		case OpStoreSlotKeep:
			w.movsdStore(regCtx, ctxSlotsOff+in.A*8, x(d-1))
		case OpDup:
			w.movsdRR(x(d), x(d-1))
		case OpSwap:
			w.movsdRR(t2Tmp0Xmm, x(d-1))
			w.movsdRR(x(d-1), x(d-2))
			w.movsdRR(x(d-2), t2Tmp0Xmm)
		case OpPop:
		case OpAdd, OpSub, OpMul:
			op := byte(0x58)
			if in.Op == OpSub {
				op = 0x5C
			}
			if in.Op == OpMul {
				op = 0x59
			}
			w.u8(0xF2)
			w.u8(0x0F)
			w.u8(op)
			w.u8(0xC0 | ((x(d-2) & 7) << 3) | (x(d-1) & 7))
		case OpDiv:
			safe := newLocal()
			errLabel := newLocal()
			done := newLocal()
			w.movsdLoad(t2ZeroXmm, regCtx, ctxConstsOff)
			w.ucomisd(x(d-1), t2ZeroXmm)
			w.jcc(0x8A, safe)
			w.jcc(0x84, errLabel)
			w.mark(safe)
			w.u8(0xF2)
			w.u8(0x0F)
			w.u8(0x5E)
			w.u8(0xC0 | ((x(d-2) & 7) << 3) | (x(d-1) & 7))
			w.jmp(done)
			w.mark(errLabel)
			w.syncTier2Counters()
			w.setStatus(StatusDivisionByZero, int64(in.ErrorNode))
			w.jmp(returnLabel)
			w.mark(done)
		case OpMod:
			errLabel := newLocal()
			deopt := newLocal()
			done := newLocal()
			w.movsdLoad(t2ZeroXmm, regCtx, ctxConstsOff)
			w.ucomisd(x(d-1), t2ZeroXmm)
			w.jcc(0x8A, errLabel)
			w.jcc(0x84, errLabel)
			w.ucomisd(x(d-2), t2ZeroXmm)
			w.jcc(0x8A, deopt)
			w.u8(0xF2)
			w.u8(0x0F)
			w.u8(0x5E)
			w.u8(0xC0 | ((x(d-2) & 7) << 3) | (x(d-1) & 7))
			w.movsdLoad(t2ZeroXmm, regCtx, ctxConstsOff+8)
			w.ucomisd(x(d-2), t2ZeroXmm)
			w.jcc(0x8A, deopt)
			w.jcc(0x83, deopt)
			w.movsdLoad(t2ZeroXmm, regCtx, ctxConstsOff+16)
			w.ucomisd(x(d-2), t2ZeroXmm)
			w.jcc(0x8A, deopt)
			w.jcc(0x82, deopt)
			w.cvttsd2si64(0, x(d-2))
			w.cvtsi2sd64(t2Tmp0Xmm, 0)
			w.u8(0xF2)
			w.u8(0x0F)
			w.u8(0x59)
			w.u8(0xC0 | ((t2Tmp0Xmm & 7) << 3) | (x(d-1) & 7))
			w.u8(0xF2)
			w.u8(0x0F)
			w.u8(0x5C)
			w.u8(0xC0 | ((x(d-2) & 7) << 3) | (t2Tmp0Xmm & 7))
			w.jmp(done)
			w.mark(errLabel)
			w.syncTier2Counters()
			w.setStatus(StatusDivisionByZero, int64(in.ErrorNode))
			w.jmp(returnLabel)
			w.mark(deopt)
			w.syncTier2Counters()
			w.setStatus(StatusDeopt, int64(in.ErrorNode))
			w.jmp(returnLabel)
			w.mark(done)
		case OpNeg:
			w.movsdLoad(t2Tmp0Xmm, regCtx, ctxConstsOff+8)
			w.xorpdXmm(x(d-1), t2Tmp0Xmm)
		case OpCompare:
			trueLabel := newLocal()
			falseLabel := newLocal()
			endLabel := newLocal()
			w.ucomisd(x(d-2), x(d-1))
			if CompareOp(in.A) == CompareNE {
				w.jcc(0x8A, trueLabel)
				w.jcc(0x85, trueLabel)
			} else {
				w.jcc(0x8A, falseLabel)
				cc := byte(0x4)
				switch CompareOp(in.A) {
				case CompareLT:
					cc = 0x2
				case CompareLE:
					cc = 0x6
				case CompareGT:
					cc = 0x7
				case CompareGE:
					cc = 0x3
				case CompareEQ:
					cc = 0x4
				}
				w.jcc(cc, trueLabel)
			}
			w.mark(falseLabel)
			w.movsdLoad(x(d-2), regCtx, ctxConstsOff)
			w.jmp(endLabel)
			w.mark(trueLabel)
			w.movsdLoad(x(d-2), regCtx, ctxConstsOff+24)
			w.mark(endLabel)
		case OpJump:
			w.jmp(in.Target)
		case OpBranchFalse, OpBranchTrue:
			w.movsdLoad(t2ZeroXmm, regCtx, ctxConstsOff)
			w.ucomisd(x(d-1), t2ZeroXmm)
			if in.Op == OpBranchFalse {
				w.jcc(0x8A, in.Target)
				w.jcc(0x84, in.Target)
			} else {
				skip := newLocal()
				w.jcc(0x8A, skip)
				w.jcc(0x84, skip)
				w.jmp(in.Target)
				w.mark(skip)
			}
		case OpTick:
			if in.A <= 0 {
				continue
			}
			n := int32(in.A)
			done := newLocal()
			w.addRegImm32(t2UsedReg, n)
			w.u8(0x4D)
			w.u8(0x85)
			w.u8(0xC0)
			w.jcc(0x88, done)
			w.subRegImm32(t2BudgetReg, n)
			w.jcc(0x89, done)
			w.syncTier2Counters()
			w.setStatus(StatusTimeout, int64(in.ErrorNode))
			w.jmp(returnLabel)
			w.mark(done)
		case OpSetRepeatCounter:
			deopt := newLocal()
			done := newLocal()
			w.movsdRR(t2Tmp0Xmm, x(d-1))
			w.cvttsd2si64(0, t2Tmp0Xmm)
			w.cvtsi2sd64(t2Tmp1Xmm, 0)
			w.ucomisd(t2Tmp0Xmm, t2Tmp1Xmm)
			w.jcc(0x8A, deopt)
			w.jcc(0x85, deopt)
			w.movqStore(regCtx, ctxCountersOff+in.A*8, 0)
			w.jmp(done)
			w.mark(deopt)
			w.syncTier2Counters()
			w.setStatus(StatusDeopt, int64(in.ErrorNode))
			w.jmp(returnLabel)
			w.mark(done)
		case OpRepeatCheck:
			w.movqLoad(0, regCtx, ctxCountersOff+in.A*8)
			w.u8(0x48)
			w.u8(0x85)
			w.u8(0xC0)
			w.jcc(0x84, in.Target)
		case OpRepeatDecrement:
			skip := newLocal()
			w.movqLoad(0, regCtx, ctxCountersOff+in.A*8)
			w.cmpRegImm8(0, 0)
			w.jcc(0x8E, skip)
			w.u8(0x48)
			w.u8(0xFF)
			w.u8(0xC8)
			w.movqStore(regCtx, ctxCountersOff+in.A*8, 0)
			w.mark(skip)
		case OpBreak:
			w.syncTier2Counters()
			w.setStatus(StatusBreak, int64(in.ErrorNode))
			w.jmp(in.Target)
		case OpReturn:
			w.jmp(returnLabel)
		default:
			return fail(i, fmt.Errorf("unsupported opcode %d", in.Op))
		}
	}
	for _, label := range labelAt[len(p.Bytecode)] {
		w.mark(label)
	}
	w.mark(returnLabel)
	w.syncTier2Counters()
	w.u8(0xC3)
	if err := w.patchAll(); err != nil {
		return nil, err
	}
	code, err := allocateExecutable(len(w.b))
	if err != nil {
		return nil, err
	}
	copy(code, w.b)
	if err := protectExecutable(code); err != nil {
		_ = releaseExecutable(code)
		return nil, err
	}
	return code, nil
}
