//go:build amd64

package jit

import (
	"fmt"
	"math"
)

const (
	fastSlotBase          = 0
	fastStackBase         = 4
	fastMaxSlots          = 4
	fastMaxStack          = 2
	fastBudgetReg         = 8
	fastUsedReg           = 9
	fastOriginalBudgetReg = 11
)

type fastStatusSite struct {
	label     int
	status    int64
	errorNode int64
}

func (w *x86Writer) movsdRRFast(dst, src byte) {
	w.u8(0xF2)
	if dst >= 8 || src >= 8 {
		w.u8(x86Rex(false, dst >= 8, src >= 8))
	}
	w.u8(0x0F)
	w.u8(0x10)
	w.u8(0xC0 | ((dst & 7) << 3) | (src & 7))
}

func (w *x86Writer) arithmeticFast(op byte, dst, src byte) {
	w.u8(0xF2)
	if dst >= 8 || src >= 8 {
		w.u8(x86Rex(false, dst >= 8, src >= 8))
	}
	w.u8(0x0F)
	w.u8(op)
	w.u8(0xC0 | ((dst & 7) << 3) | (src & 7))
}

func (w *x86Writer) incReg64(reg byte) {
	w.u8(x86Rex(true, false, reg >= 8))
	w.u8(0xFF)
	w.u8(0xC0 | (reg & 7))
}

func (w *x86Writer) cmpRegReg64(a, b byte) {
	w.u8(x86Rex(true, b >= 8, a >= 8))
	w.u8(0x39)
	w.u8(0xC0 | ((b & 7) << 3) | (a & 7))
}

func (w *x86Writer) ucomisdMem(xmm, base byte, disp int) {
	w.u8(0x66)
	if xmm >= 8 || base >= 8 {
		w.u8(x86Rex(false, xmm >= 8, base >= 8))
	}
	w.u8(0x0F)
	w.u8(0x2E)
	w.u8(0x80 | ((xmm & 7) << 3) | (base & 7))
	w.u32(uint32(int32(disp)))
}

func (w *x86Writer) xorpdMem(xmm, base byte, disp int) {
	w.u8(0x66)
	if xmm >= 8 || base >= 8 {
		w.u8(x86Rex(false, xmm >= 8, base >= 8))
	}
	w.u8(0x0F)
	w.u8(0x57)
	w.u8(0x80 | ((xmm & 7) << 3) | (base & 7))
	w.u32(uint32(int32(disp)))
}

func (w *x86Writer) movqXmmToReg(dstGpr, srcXmm byte) {
	w.u8(0x66)
	w.u8(x86Rex(true, srcXmm >= 8, dstGpr >= 8))
	w.u8(0x0F)
	w.u8(0x7E)
	w.u8(0xC0 | ((srcXmm & 7) << 3) | (dstGpr & 7))
}

func (w *x86Writer) movqRegToXmm(dstXmm, srcGpr byte) {
	w.u8(0x66)
	w.u8(x86Rex(true, dstXmm >= 8, srcGpr >= 8))
	w.u8(0x0F)
	w.u8(0x6E)
	w.u8(0xC0 | ((dstXmm & 7) << 3) | (srcGpr & 7))
}

func fastSlotReg(index int) byte  { return byte(fastSlotBase + index) }
func fastStackReg(depth int) byte { return byte(fastStackBase + depth) }

func fastEligible(p *Program, refs int) error {
	if refs > fastMaxSlots {
		return fmt.Errorf("fast register file requires more than %d values", fastMaxSlots)
	}
	if p.ExpressionStack > fastMaxStack {
		return fmt.Errorf("fast expression stack exceeds %d values", fastMaxStack)
	}
	for i, in := range p.Bytecode {
		if in.Op != OpMod || i == 0 {
			continue
		}
		prev := p.Bytecode[i-1]
		if prev.Op != OpPushConst || prev.A < 0 || prev.A >= len(p.Constants) {
			return fmt.Errorf("fast modulo requires a constant divisor")
		}
		v := p.Constants[prev.A]
		if v == 0 || math.Trunc(v) != v || v < -9223372036854775808.0 || v >= 9223372036854775808.0 {
			return fmt.Errorf("fast modulo requires a nonzero int64 constant divisor")
		}
	}
	return nil
}

func emitMachineFast(p *Program) ([]byte, error) {
	if err := fastEligible(p, len(p.Refs)); err != nil {
		return nil, err
	}
	depths, err := bytecodeStackDepths(p)
	if err != nil {
		return nil, err
	}
	w := &x86Writer{labels: make(map[int]int)}
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
	statusSites := make([]fastStatusSite, 0, 8)
	fail := func(i int, e error) ([]byte, error) {
		return nil, fmt.Errorf("amd64 fast native instruction %d: %w", i, e)
	}

	w.movqLoad(fastBudgetReg, regCtx, ctxBudgetOff)
	w.movqLoad(fastOriginalBudgetReg, regCtx, ctxBudgetOff)
	w.movqLoad(fastUsedReg, regCtx, ctxUsedOff)
	w.cmpRegImm8(fastBudgetReg, 0)
	unlimited := newLocal()
	budgetReady := newLocal()
	w.jcc(0x88, unlimited)
	w.jmp(budgetReady)
	w.mark(unlimited)
	w.movqImmReg(fastBudgetReg, 0x7fffffffffffffff)
	w.mark(budgetReady)

	for i := range p.Refs {
		if i < len(p.Reads) && p.Reads[i] {
			w.movsdLoad(fastSlotReg(i), regCtx, ctxSlotsOff+i*8)
		} else {
			w.xorpdXmm(fastSlotReg(i), fastSlotReg(i))
		}
	}

	for i, in := range p.Bytecode {
		for _, label := range labelAt[i] {
			w.mark(label)
		}
		d := depths[i]
		if d > fastMaxStack {
			return nil, fmt.Errorf("fast expression stack exceeds %d at %d", fastMaxStack, i)
		}
		sx := func(depth int) byte { return fastStackReg(depth) }
		sr := func(index int) byte { return fastSlotReg(index) }
		switch in.Op {
		case OpPushConst:
			w.movsdLoad(sx(d), regCtx, ctxConstsOff+in.A*8)
		case OpLoadSlot:
			if in.A < 0 || in.A >= len(p.Refs) {
				return fail(i, fmt.Errorf("invalid fast reference %d", in.A))
			}
			w.movsdRRFast(sx(d), sr(in.A))
		case OpStoreSlot:
			if in.A < 0 || in.A >= len(p.Refs) || d < 1 {
				return fail(i, fmt.Errorf("invalid fast store %d", in.A))
			}
			w.movsdRRFast(sr(in.A), sx(d-1))
		case OpStoreSlotKeep:
			if in.A < 0 || in.A >= len(p.Refs) || d < 1 {
				return fail(i, fmt.Errorf("invalid fast store keep %d", in.A))
			}
			w.movsdRRFast(sr(in.A), sx(d-1))
		case OpDup:
			if d < 1 {
				return fail(i, fmt.Errorf("invalid fast dup"))
			}
			w.movsdRRFast(sx(d), sx(d-1))
		case OpSwap:
			if d < 2 {
				return fail(i, fmt.Errorf("invalid fast swap"))
			}
			w.movqXmmToReg(2, sx(d-1))
			w.movsdRRFast(sx(d-1), sx(d-2))
			w.movqRegToXmm(sx(d-2), 2)
		case OpPop:
		case OpAdd, OpSub, OpMul:
			if d < 2 {
				return fail(i, fmt.Errorf("invalid fast arithmetic depth"))
			}
			op := byte(0x58)
			if in.Op == OpSub {
				op = 0x5C
			}
			if in.Op == OpMul {
				op = 0x59
			}
			w.arithmeticFast(op, sx(d-2), sx(d-1))
		case OpDiv:
			if d < 2 {
				return fail(i, fmt.Errorf("invalid fast division depth"))
			}
			safe := newLocal()
			errLabel := newLocal()
			done := newLocal()
			w.ucomisdMem(sx(d-1), regCtx, ctxConstsOff)
			w.jcc(0x8A, safe)
			w.jcc(0x84, errLabel)
			w.mark(safe)
			w.u8(0xF2)
			w.u8(0x0F)
			w.u8(0x5E)
			w.u8(0xC0 | ((sx(d-2) & 7) << 3) | (sx(d-1) & 7))
			w.jmp(done)
			w.mark(errLabel)
			statusSites = append(statusSites, fastStatusSite{label: newLocal(), status: StatusDivisionByZero, errorNode: int64(in.ErrorNode)})
			w.jmp(statusSites[len(statusSites)-1].label)
			w.mark(done)
		case OpMod:
			if d < 2 || i == 0 {
				return fail(i, fmt.Errorf("invalid fast modulo depth"))
			}
			prev := p.Bytecode[i-1]
			if prev.Op != OpPushConst || prev.A < 0 || prev.A >= len(p.Constants) {
				return fail(i, fmt.Errorf("invalid fast modulo divisor"))
			}
			divisor := p.Constants[prev.A]
			if divisor == 0 || math.IsNaN(divisor) || math.IsInf(divisor, 0) || math.Trunc(divisor) != divisor || divisor < -9223372036854775808.0 || divisor >= 9223372036854775808.0 {
				return fail(i, fmt.Errorf("invalid fast modulo divisor"))
			}
			divisorInt := int64(divisor)
			floatPath := newLocal()
			errLabel := newLocal()
			deopt := newLocal()
			done := newLocal()
			w.ucomisdMem(sx(d-1), regCtx, ctxConstsOff)
			w.jcc(0x8A, errLabel)
			w.jcc(0x84, errLabel)
			w.cvttsd2si64(0, sx(d-2))
			w.cvtsi2sd64(5, 0)
			w.ucomisd(sx(d-2), 5)
			w.jcc(0x8A, deopt)
			w.jcc(0x85, floatPath)
			if divisorInt == -1 {
				w.jmp(floatPath)
			} else {
				w.movqImmReg(7, uint64(divisorInt))
				w.u8(0x48)
				w.u8(0x99)
				w.u8(0x48)
				w.u8(0xF7)
				w.u8(0xFF)
				w.cvtsi2sd64(sx(d-2), 2)
				w.jmp(done)
			}
			w.mark(floatPath)
			w.movsdLoad(5, regCtx, ctxConstsOff+prev.A*8)
			w.ucomisdMem(sx(d-2), regCtx, ctxConstsOff+8)
			w.jcc(0x8A, deopt)
			w.jcc(0x87, deopt)
			w.ucomisdMem(sx(d-2), regCtx, ctxConstsOff+16)
			w.jcc(0x8A, deopt)
			w.jcc(0x82, deopt)
			w.movqXmmToReg(2, sx(d-2))
			w.u8(0xF2)
			w.u8(0x0F)
			w.u8(0x5E)
			w.u8(0xC0 | ((sx(d-2) & 7) << 3) | (sx(d-1) & 7))
			w.cvttsd2si64(0, sx(d-2))
			w.cvtsi2sd64(sx(d-2), 0)
			w.u8(0xF2)
			w.u8(0x0F)
			w.u8(0x59)
			w.u8(0xC0 | ((sx(d-2) & 7) << 3) | ((sx(d - 1)) & 7))
			w.movqXmmToReg(0, sx(d-2))
			w.movqRegToXmm(sx(d-2), 2)
			w.movqRegToXmm(sx(d-1), 0)
			w.u8(0xF2)
			w.u8(0x0F)
			w.u8(0x5C)
			w.u8(0xC0 | ((sx(d-2) & 7) << 3) | ((sx(d - 1)) & 7))
			w.jmp(done)
			w.mark(errLabel)
			statusSites = append(statusSites, fastStatusSite{label: newLocal(), status: StatusDivisionByZero, errorNode: int64(in.ErrorNode)})
			w.jmp(statusSites[len(statusSites)-1].label)
			w.mark(deopt)
			statusSites = append(statusSites, fastStatusSite{label: newLocal(), status: StatusDeopt, errorNode: int64(in.ErrorNode)})
			w.jmp(statusSites[len(statusSites)-1].label)
			w.mark(done)
		case OpNeg:
			if d < 1 {
				return fail(i, fmt.Errorf("invalid fast negation depth"))
			}
			w.xorpdMem(sx(d-1), regCtx, ctxConstsOff+8)
		case OpCompare:
			if d < 2 {
				return fail(i, fmt.Errorf("invalid fast compare depth"))
			}
			trueLabel := newLocal()
			falseLabel := newLocal()
			end := newLocal()
			w.ucomisd(sx(d-2), sx(d-1))
			if CompareOp(in.A) == CompareNE {
				w.jcc(0x8A, trueLabel)
				w.jcc(0x85, trueLabel)
				w.jmp(falseLabel)
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
				w.jmp(falseLabel)
			}
			w.mark(falseLabel)
			w.movsdLoad(sx(d-2), regCtx, ctxConstsOff)
			w.jmp(end)
			w.mark(trueLabel)
			w.movsdLoad(sx(d-2), regCtx, ctxConstsOff+24)
			w.mark(end)
		case OpJump:
			w.jmp(in.Target)
		case OpBranchFalse, OpBranchTrue:
			if d < 1 {
				return fail(i, fmt.Errorf("invalid fast branch depth"))
			}
			w.ucomisdMem(sx(d-1), regCtx, ctxConstsOff)
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
			w.addRegImm32(fastUsedReg, int32(in.A))
			w.cmpRegReg64(fastUsedReg, fastBudgetReg)
			label := newLocal()
			w.jcc(0x8F, label)
			statusSites = append(statusSites, fastStatusSite{label: label, status: StatusTimeout, errorNode: int64(in.ErrorNode)})
		case OpSetRepeatCounter:
			if d < 1 {
				return fail(i, fmt.Errorf("invalid fast repeat counter"))
			}
			deopt := newLocal()
			done := newLocal()
			w.cvttsd2si64(0, sx(d-1))
			w.cvtsi2sd64(5, 0)
			w.ucomisd(sx(d-1), 5)
			w.jcc(0x8A, deopt)
			w.jcc(0x85, deopt)
			w.movqStore(regCtx, ctxCountersOff+in.A*8, 0)
			w.jmp(done)
			w.mark(deopt)
			statusSites = append(statusSites, fastStatusSite{label: newLocal(), status: StatusDeopt, errorNode: int64(in.ErrorNode)})
			w.jmp(statusSites[len(statusSites)-1].label)
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
			w.movqImmMem(regCtx, ctxStatusOff, uint64(StatusBreak))
			w.movqImmMem(regCtx, ctxErrorNodeOff, uint64(in.ErrorNode))
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
	for _, site := range statusSites {
		w.mark(site.label)
		w.setStatus(site.status, site.errorNode)
		w.jmp(returnLabel)
	}
	w.mark(returnLabel)
	w.movqStore(regCtx, ctxBudgetOff, fastOriginalBudgetReg)
	w.movqStore(regCtx, ctxUsedOff, fastUsedReg)
	for i, write := range p.Writes {
		ref := -1
		for j, candidate := range p.Refs {
			if candidate == write.Reference {
				ref = j
				break
			}
		}
		if ref < 0 || ref >= fastMaxSlots {
			return nil, fmt.Errorf("fast write reference %d is unavailable", i)
		}
		w.movsdStore(regCtx, ctxSlotsOff+ref*8, fastSlotReg(ref))
	}
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
