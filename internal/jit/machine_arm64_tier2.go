//go:build arm64

package jit

import "fmt"

const (
	t2ArmBudgetReg = 6
	t2ArmUsedReg   = 7
	t2ArmZeroFP    = 7
	t2ArmTmpFP     = 6
	t2ArmMaxStack  = 6
)

func (w *a64Writer) syncTier2Counters() {
	w.u32(strX(t2ArmBudgetReg, regCtx, ctxBudgetOff))
	w.u32(strX(t2ArmUsedReg, regCtx, ctxUsedOff))
}

func fmovD(rd, rn uint32) uint32 { return 0x1E604000 | (rn << 5) | rd }

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
	if p.NestedLoops || maxDepth > t2ArmMaxStack {
		return emitMachineLegacy(p)
	}

	w := &a64Writer{labels: make(map[int]int)}
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
		return nil, fmt.Errorf("ARM64 native optimization instruction %d: %w", i, e)
	}

	w.u32(ldrX(t2ArmBudgetReg, regCtx, ctxBudgetOff))
	w.u32(ldrX(t2ArmUsedReg, regCtx, ctxUsedOff))

	for i, in := range p.Bytecode {
		for _, label := range labelAt[i] {
			w.mark(label)
		}
		d := depths[i]
		x := func(index int) uint32 { return uint32(index) }
		if d > t2ArmMaxStack {
			return emitMachineLegacy(p)
		}
		switch in.Op {
		case OpPushConst:
			w.u32(ldrD(x(d), regCtx, ctxConstsOff+in.A*8))
		case OpLoadSlot:
			w.u32(ldrD(x(d), regCtx, ctxSlotsOff+in.A*8))
		case OpStoreSlot:
			w.u32(strD(x(d-1), regCtx, ctxSlotsOff+in.A*8))
		case OpStoreSlotKeep:
			w.u32(strD(x(d-1), regCtx, ctxSlotsOff+in.A*8))
		case OpDup:
			w.u32(fmovD(x(d), x(d-1)))
		case OpSwap:
			w.u32(fmovD(t2ArmTmpFP, x(d-1)))
			w.u32(fmovD(x(d-1), x(d-2)))
			w.u32(fmovD(x(d-2), t2ArmTmpFP))
		case OpPop:
		case OpAdd:
			w.u32(faddD(x(d-2), x(d-2), x(d-1)))
		case OpSub:
			w.u32(fsubD(x(d-2), x(d-2), x(d-1)))
		case OpMul:
			w.u32(fmulD(x(d-2), x(d-2), x(d-1)))
		case OpDiv:
			safe := newLocal()
			errLabel := newLocal()
			done := newLocal()
			w.u32(ldrD(t2ArmZeroFP, regCtx, ctxConstsOff))
			w.u32(fcmpD(x(d-1), t2ArmZeroFP))
			w.branchCond(6, safe)
			w.branchCond(0, errLabel)
			w.mark(safe)
			w.u32(fdivD(x(d-2), x(d-2), x(d-1)))
			w.branch(done)
			w.mark(errLabel)
			w.syncTier2Counters()
			w.setStatus(StatusDivisionByZero, int64(in.ErrorNode))
			w.branch(returnLabel)
			w.mark(done)
		case OpMod:
			errLabel := newLocal()
			deopt := newLocal()
			done := newLocal()
			w.u32(ldrD(t2ArmZeroFP, regCtx, ctxConstsOff))
			w.u32(fcmpD(x(d-1), t2ArmZeroFP))
			w.branchCond(6, errLabel)
			w.branchCond(0, errLabel)
			w.u32(fcmpD(x(d-2), t2ArmZeroFP))
			w.branchCond(6, deopt)
			w.u32(fdivD(x(d-2), x(d-2), x(d-1)))
			w.u32(ldrD(t2ArmZeroFP, regCtx, ctxConstsOff+8))
			w.u32(fcmpD(x(d-2), t2ArmZeroFP))
			w.branchCond(6, deopt)
			w.branchCond(2, deopt)
			w.u32(ldrD(t2ArmZeroFP, regCtx, ctxConstsOff+16))
			w.u32(fcmpD(x(d-2), t2ArmZeroFP))
			w.branchCond(6, deopt)
			w.branchCond(3, deopt)
			w.u32(fcvtzs(regA, x(d-2)))
			w.u32(scvtf(t2ArmTmpFP, regA))
			w.u32(fmulD(t2ArmTmpFP, t2ArmTmpFP, x(d-1)))
			w.u32(fsubD(x(d-2), x(d-2), t2ArmTmpFP))
			w.branch(done)
			w.mark(errLabel)
			w.syncTier2Counters()
			w.setStatus(StatusDivisionByZero, int64(in.ErrorNode))
			w.branch(returnLabel)
			w.mark(deopt)
			w.syncTier2Counters()
			w.setStatus(StatusDeopt, int64(in.ErrorNode))
			w.branch(returnLabel)
			w.mark(done)
		case OpNeg:
			w.u32(fnegD(x(d-1), x(d-1)))
		case OpCompare:
			trueLabel := newLocal()
			falseLabel := newLocal()
			endLabel := newLocal()
			w.u32(fcmpD(x(d-2), x(d-1)))
			if CompareOp(in.A) == CompareNE {
				w.branchCond(6, trueLabel)
				w.branchCond(1, trueLabel)
			} else {
				w.branchCond(6, falseLabel)
				cc := uint32(0)
				switch CompareOp(in.A) {
				case CompareLT:
					cc = 11
				case CompareLE:
					cc = 13
				case CompareGT:
					cc = 12
				case CompareGE:
					cc = 10
				case CompareEQ:
					cc = 0
				}
				w.branchCond(cc, trueLabel)
			}
			w.mark(falseLabel)
			w.u32(ldrD(x(d-2), regCtx, ctxConstsOff))
			w.branch(endLabel)
			w.mark(trueLabel)
			w.u32(ldrD(x(d-2), regCtx, ctxConstsOff+24))
			w.mark(endLabel)
		case OpJump:
			w.branch(in.Target)
		case OpBranchFalse, OpBranchTrue:
			w.u32(ldrD(t2ArmZeroFP, regCtx, ctxConstsOff))
			w.u32(fcmpD(x(d-1), t2ArmZeroFP))
			if in.Op == OpBranchFalse {
				w.branchCond(6, in.Target)
				w.branchCond(0, in.Target)
			} else {
				skip := newLocal()
				w.branchCond(6, skip)
				w.branchCond(0, skip)
				w.branch(in.Target)
				w.mark(skip)
			}
		case OpTick:
			if in.A <= 0 {
				continue
			}
			n := uint32(in.A)
			done := newLocal()
			w.u32(addImm64(t2ArmUsedReg, t2ArmUsedReg, n))
			w.u32(cmpXZero(t2ArmBudgetReg))
			w.branchCond(11, done)
			w.u32(subsImm64(t2ArmBudgetReg, t2ArmBudgetReg, n))
			w.branchCond(5, done)
			w.syncTier2Counters()
			w.setStatus(StatusTimeout, int64(in.ErrorNode))
			w.branch(returnLabel)
			w.mark(done)
		case OpSetRepeatCounter:
			deopt := newLocal()
			done := newLocal()
			w.u32(fcvtzs(regA, x(d-1)))
			w.u32(scvtf(t2ArmTmpFP, regA))
			w.u32(fcmpD(x(d-1), t2ArmTmpFP))
			w.branchCond(6, deopt)
			w.branchCond(1, deopt)
			w.u32(strX(regA, regCtx, ctxCountersOff+in.A*8))
			w.branch(done)
			w.mark(deopt)
			w.syncTier2Counters()
			w.setStatus(StatusDeopt, int64(in.ErrorNode))
			w.branch(returnLabel)
			w.mark(done)
		case OpRepeatCheck:
			w.u32(ldrX(regA, regCtx, ctxCountersOff+in.A*8))
			w.u32(cmpXZero(regA))
			w.branchCond(0, in.Target)
		case OpRepeatDecrement:
			skip := newLocal()
			w.u32(ldrX(regA, regCtx, ctxCountersOff+in.A*8))
			w.u32(cmpXZero(regA))
			w.branchCond(13, skip)
			w.u32(subImm64(regA, regA, 1))
			w.u32(strX(regA, regCtx, ctxCountersOff+in.A*8))
			w.mark(skip)
		case OpBreak:
			w.syncTier2Counters()
			w.setStatus(StatusBreak, int64(in.ErrorNode))
			w.branch(in.Target)
		case OpReturn:
			w.branch(returnLabel)
		default:
			return fail(i, fmt.Errorf("unsupported opcode %d", in.Op))
		}
	}
	for _, label := range labelAt[len(p.Bytecode)] {
		w.mark(label)
	}
	w.mark(returnLabel)
	w.syncTier2Counters()
	w.u32(0xD65F03C0)
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
