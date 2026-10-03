//go:build arm64

package jit

import (
	"fmt"
	"math"
)

const (
	a64FastSlotBase  = 16
	a64FastMaxSlots  = 16
	a64FastMaxStack  = 8
	a64FastBudgetReg = 6
	a64FastUsedReg   = 7
	a64FastTmpFP     = 8
	a64FastZeroFP    = 9
	a64FastTmp2FP    = 10
	a64FastLimitFP   = 11
	a64FastOneFP     = 12
)

type a64FastStatusSite struct {
	label     int
	status    int64
	errorNode int64
}

func fmovDX(rd, rn uint32) uint32 { return 0x9E670000 | (rn << 5) | rd }

func a64FastDivisor(p *Program, i int) (float64, error) {
	if i == 0 {
		return 0, fmt.Errorf("fast modulo has no divisor")
	}
	prev := p.Bytecode[i-1]
	if prev.Op != OpPushConst || prev.A < 0 || prev.A >= len(p.Constants) {
		return 0, fmt.Errorf("fast modulo requires a constant divisor")
	}
	v := p.Constants[prev.A]
	if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v || v < -9223372036854775808.0 || v >= 9223372036854775808.0 {
		return 0, fmt.Errorf("fast modulo requires a nonzero int64 constant divisor")
	}
	return v, nil
}

func a64FastEligible(p *Program) error {
	if len(p.Refs) > a64FastMaxSlots {
		return fmt.Errorf("fast register file requires more than %d values", a64FastMaxSlots)
	}
	if p.ExpressionStack > a64FastMaxStack {
		return fmt.Errorf("fast expression stack exceeds %d values", a64FastMaxStack)
	}
	for i, in := range p.Bytecode {
		if in.Op != OpMod {
			continue
		}
		if _, err := a64FastDivisor(p, i); err != nil {
			return err
		}
	}
	return nil
}

func emitMachineFast(p *Program) ([]byte, error) {
	if err := a64FastEligible(p); err != nil {
		return nil, err
	}
	depths, err := bytecodeStackDepths(p)
	if err != nil {
		return nil, err
	}
	w := &a64Writer{labels: make(map[int]int)}
	labelAt := make(map[int][]int)
	for label, index := range p.Labels {
		if index >= 0 {
			labelAt[index] = append(labelAt[index], label)
		}
	}
	localSeq := 0
	newLocal := func() int {
		localSeq++
		return (1 << 30) + localSeq
	}
	returnLabel := 1 << 29
	sites := make([]a64FastStatusSite, 0, 8)
	site := func(status, node int64) int {
		label := newLocal()
		sites = append(sites, a64FastStatusSite{label: label, status: status, errorNode: node})
		return label
	}
	fail := func(i int, e error) ([]byte, error) {
		return nil, fmt.Errorf("ARM64 fast native instruction %d: %w", i, e)
	}
	sx := func(depth int) uint32 { return uint32(depth) }
	sr := func(index int) uint32 { return uint32(a64FastSlotBase + index) }

	w.u32(ldrX(a64FastBudgetReg, regCtx, ctxBudgetOff))
	w.u32(ldrX(a64FastUsedReg, regCtx, ctxUsedOff))
	ready := newLocal()
	w.u32(cmpXZero(a64FastBudgetReg))
	w.branchCond(10, ready)
	w.movImm64(a64FastBudgetReg, 0x7fffffffffffffff)
	w.mark(ready)
	w.u32(fmovDX(a64FastZeroFP, 31))
	w.u32(ldrD(a64FastOneFP, regCtx, ctxConstsOff+24))
	for i := range p.Refs {
		if i < len(p.Reads) && p.Reads[i] {
			w.u32(ldrD(sr(i), regCtx, ctxSlotsOff+i*8))
		} else {
			w.u32(fmovDX(sr(i), 31))
		}
	}

	for i, in := range p.Bytecode {
		for _, label := range labelAt[i] {
			w.mark(label)
		}
		d := depths[i]
		if d > a64FastMaxStack {
			return fail(i, fmt.Errorf("expression stack exceeds %d", a64FastMaxStack))
		}
		switch in.Op {
		case OpPushConst:
			if in.A < 0 || in.A >= len(p.Constants) {
				return fail(i, fmt.Errorf("invalid constant %d", in.A))
			}
			w.u32(ldrD(sx(d), regCtx, ctxConstsOff+in.A*8))
		case OpLoadSlot:
			if in.A < 0 || in.A >= len(p.Refs) {
				return fail(i, fmt.Errorf("invalid reference %d", in.A))
			}
			w.u32(fmovD(sx(d), sr(in.A)))
		case OpStoreSlot, OpStoreSlotKeep:
			if in.A < 0 || in.A >= len(p.Refs) || d < 1 {
				return fail(i, fmt.Errorf("invalid store %d", in.A))
			}
			w.u32(fmovD(sr(in.A), sx(d-1)))
		case OpDup:
			if d < 1 {
				return fail(i, fmt.Errorf("invalid dup"))
			}
			w.u32(fmovD(sx(d), sx(d-1)))
		case OpSwap:
			if d < 2 {
				return fail(i, fmt.Errorf("invalid swap"))
			}
			w.u32(fmovD(a64FastTmpFP, sx(d-1)))
			w.u32(fmovD(sx(d-1), sx(d-2)))
			w.u32(fmovD(sx(d-2), a64FastTmpFP))
		case OpPop:
		case OpAdd, OpSub, OpMul:
			if d < 2 {
				return fail(i, fmt.Errorf("invalid arithmetic depth"))
			}
			switch in.Op {
			case OpAdd:
				w.u32(faddD(sx(d-2), sx(d-2), sx(d-1)))
			case OpSub:
				w.u32(fsubD(sx(d-2), sx(d-2), sx(d-1)))
			default:
				w.u32(fmulD(sx(d-2), sx(d-2), sx(d-1)))
			}
		case OpDiv:
			if d < 2 {
				return fail(i, fmt.Errorf("invalid division depth"))
			}
			w.u32(fcmpD(sx(d-1), a64FastZeroFP))
			w.branchCond(0, site(StatusDivisionByZero, int64(in.ErrorNode)))
			w.u32(fdivD(sx(d-2), sx(d-2), sx(d-1)))
		case OpMod:
			if d < 2 {
				return fail(i, fmt.Errorf("invalid modulo depth"))
			}
			divisor, err := a64FastDivisor(p, i)
			if err != nil {
				return fail(i, err)
			}
			divisorInt := int64(divisor)
			x := sx(d - 2)
			y := sx(d - 1)
			floatPath := newLocal()
			done := newLocal()
			deopt := site(StatusDeopt, int64(in.ErrorNode))
			w.u32(fcvtzs(regA, x))
			w.u32(scvtf(a64FastTmpFP, regA))
			w.u32(fcmpD(x, a64FastTmpFP))
			w.branchCond(6, deopt)
			w.branchCond(1, floatPath)
			w.u32(ldrD(a64FastLimitFP, regCtx, ctxConstsOff+8))
			w.u32(fcmpD(x, a64FastLimitFP))
			w.branchCond(10, floatPath)
			if divisorInt == -1 {
				w.branch(floatPath)
			} else {
				w.movImm64(regB, uint64(divisorInt))
				w.u32(sdivX(regC, regA, regB))
				w.u32(msubX(regC, regC, regB, regA))
				w.u32(scvtf(x, regC))
				w.branch(done)
			}
			w.mark(floatPath)
			w.u32(ldrD(a64FastLimitFP, regCtx, ctxConstsOff+8))
			w.u32(fcmpD(x, a64FastLimitFP))
			w.branchCond(6, deopt)
			w.branchCond(10, deopt)
			w.u32(ldrD(a64FastLimitFP, regCtx, ctxConstsOff+16))
			w.u32(fcmpD(x, a64FastLimitFP))
			w.branchCond(11, deopt)
			w.u32(fdivD(a64FastTmp2FP, x, y))
			w.u32(fcvtzs(regA, a64FastTmp2FP))
			w.u32(scvtf(a64FastTmp2FP, regA))
			w.u32(fmulD(a64FastTmp2FP, a64FastTmp2FP, y))
			w.u32(fsubD(x, x, a64FastTmp2FP))
			w.mark(done)
		case OpNeg:
			if d < 1 {
				return fail(i, fmt.Errorf("invalid negation depth"))
			}
			w.u32(fnegD(sx(d-1), sx(d-1)))
		case OpCompare:
			if d < 2 {
				return fail(i, fmt.Errorf("invalid compare depth"))
			}
			trueLabel := newLocal()
			falseLabel := newLocal()
			end := newLocal()
			w.u32(fcmpD(sx(d-2), sx(d-1)))
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
			w.u32(fmovD(sx(d-2), a64FastZeroFP))
			w.branch(end)
			w.mark(trueLabel)
			w.u32(fmovD(sx(d-2), a64FastOneFP))
			w.mark(end)
		case OpJump:
			w.branch(in.Target)
		case OpBranchFalse, OpBranchTrue:
			if d < 1 {
				return fail(i, fmt.Errorf("invalid branch depth"))
			}
			w.u32(fcmpD(sx(d-1), a64FastZeroFP))
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
			w.u32(addImm64(a64FastUsedReg, a64FastUsedReg, uint32(in.A)))
			w.u32(cmpX(a64FastUsedReg, a64FastBudgetReg))
			w.branchCond(12, site(StatusTimeout, int64(in.ErrorNode)))
		case OpSetRepeatCounter:
			if d < 1 {
				return fail(i, fmt.Errorf("invalid repeat counter"))
			}
			deopt := site(StatusDeopt, int64(in.ErrorNode))
			w.u32(fcvtzs(regA, sx(d-1)))
			w.u32(scvtf(a64FastTmpFP, regA))
			w.u32(fcmpD(sx(d-1), a64FastTmpFP))
			w.branchCond(6, deopt)
			w.branchCond(1, deopt)
			w.u32(strX(regA, regCtx, ctxCountersOff+in.A*8))
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
	for _, s := range sites {
		w.mark(s.label)
		w.setStatus(s.status, s.errorNode)
		w.branch(returnLabel)
	}
	w.mark(returnLabel)
	w.u32(strX(a64FastUsedReg, regCtx, ctxUsedOff))
	for i, write := range p.Writes {
		ref := -1
		for j, candidate := range p.Refs {
			if candidate == write.Reference {
				ref = j
				break
			}
		}
		if ref < 0 || ref >= a64FastMaxSlots {
			return nil, fmt.Errorf("fast write reference %d is unavailable", i)
		}
		w.u32(strD(sr(ref), regCtx, ctxSlotsOff+ref*8))
	}
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
