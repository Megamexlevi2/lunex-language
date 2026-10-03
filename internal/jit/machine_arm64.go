//go:build arm64

package jit

import (
	"fmt"
)

const (
	ctxSlotsOff     = 0
	ctxStackOff     = 2048
	ctxConstsOff    = 6144
	ctxBudgetOff    = 7168
	ctxUsedOff      = 7176
	ctxStatusOff    = 7184
	ctxErrorNodeOff = 7192
	ctxCountersOff  = 7208
)

const (
	regCtx   = 0
	regStack = 1
	regA     = 2
	regB     = 3
	regC     = 4
	regD     = 5
	fp0      = 0
	fp1      = 1
	fp2      = 2
)

type a64Patch struct {
	pos   int
	label int
	kind  uint8
}

type a64Writer struct {
	b       []byte
	labels  map[int]int
	patches []a64Patch
}

const (
	a64B     = 1
	a64BCond = 2
)

func (w *a64Writer) u32(v uint32) {
	w.b = append(w.b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func (w *a64Writer) mark(label int) {
	w.labels[label] = len(w.b)
}

func (w *a64Writer) branch(label int) {
	pos := len(w.b)
	w.u32(0)
	w.patches = append(w.patches, a64Patch{pos: pos, label: label, kind: a64B})
}

func (w *a64Writer) branchCond(cond uint32, label int) {
	pos := len(w.b)
	w.u32(0x54000000 | cond)
	w.patches = append(w.patches, a64Patch{pos: pos, label: label, kind: a64BCond})
}

func (w *a64Writer) patchAll() error {
	for _, p := range w.patches {
		target, ok := w.labels[p.label]
		if !ok {
			return fmt.Errorf("unresolved native label %d", p.label)
		}
		delta := int64(target) - int64(p.pos)
		if delta%4 != 0 {
			return fmt.Errorf("unaligned ARM64 branch target")
		}
		words := delta / 4
		if p.kind == a64B {
			if words < -(1<<25) || words >= (1<<25) {
				return fmt.Errorf("ARM64 branch out of range: %d", words)
			}
			v := 0x14000000 | uint32(int32(words)&0x03ffffff)
			w.put(p.pos, v)
		} else {
			if words < -(1<<18) || words >= (1<<18) {
				return fmt.Errorf("ARM64 conditional branch out of range: %d", words)
			}
			v := w.get(p.pos) | uint32(int32(words)&0x7ffff)<<5
			w.put(p.pos, v)
		}
	}
	return nil
}

func (w *a64Writer) put(pos int, v uint32) {
	w.b[pos] = byte(v)
	w.b[pos+1] = byte(v >> 8)
	w.b[pos+2] = byte(v >> 16)
	w.b[pos+3] = byte(v >> 24)
}

func (w *a64Writer) get(pos int) uint32 {
	return uint32(w.b[pos]) | uint32(w.b[pos+1])<<8 | uint32(w.b[pos+2])<<16 | uint32(w.b[pos+3])<<24
}

func addImm64(rd, rn uint32, imm uint32) uint32 {
	if imm >= 4096 {
		panic("ARM64 immediate out of range")
	}
	return 0x91000000 | (imm << 10) | (rn << 5) | rd
}

func subImm64(rd, rn uint32, imm uint32) uint32 {
	if imm >= 4096 {
		panic("ARM64 immediate out of range")
	}
	return 0xD1000000 | (imm << 10) | (rn << 5) | rd
}

func ldrX(rt, rn uint32, offset int) uint32 {
	if offset >= 0 && offset%8 == 0 && offset/8 < 4096 {
		return 0xF9400000 | (uint32(offset/8) << 10) | (rn << 5) | rt
	}
	if offset >= -256 && offset <= 255 {
		return 0xF8400000 | ((uint32(offset) & 0x1ff) << 12) | (rn << 5) | rt
	}
	panic("ARM64 LDR offset out of range")
}

func strX(rt, rn uint32, offset int) uint32 {
	if offset >= 0 && offset%8 == 0 && offset/8 < 4096 {
		return 0xF9000000 | (uint32(offset/8) << 10) | (rn << 5) | rt
	}
	if offset >= -256 && offset <= 255 {
		return 0xF8000000 | ((uint32(offset) & 0x1ff) << 12) | (rn << 5) | rt
	}
	panic("ARM64 STR offset out of range")
}

func ldrD(rt, rn uint32, offset int) uint32 {
	if offset >= 0 && offset%8 == 0 && offset/8 < 4096 {
		return 0xFD400000 | (uint32(offset/8) << 10) | (rn << 5) | rt
	}
	if offset >= -256 && offset <= 255 {
		return 0xFC400000 | ((uint32(offset) & 0x1ff) << 12) | (rn << 5) | rt
	}
	panic("ARM64 LDR D offset out of range")
}

func strD(rt, rn uint32, offset int) uint32 {
	if offset >= 0 && offset%8 == 0 && offset/8 < 4096 {
		return 0xFD000000 | (uint32(offset/8) << 10) | (rn << 5) | rt
	}
	if offset >= -256 && offset <= 255 {
		return 0xFC000000 | ((uint32(offset) & 0x1ff) << 12) | (rn << 5) | rt
	}
	panic("ARM64 STR D offset out of range")
}

func cmpX(rn, rm uint32) uint32 {
	return 0xEB00001F | (rm << 16) | (rn << 5)
}

func subsImm64(rd, rn uint32, imm uint32) uint32 {
	if imm >= 4096 {
		panic("ARM64 immediate out of range")
	}
	return 0xF1000000 | (imm << 10) | (rn << 5) | rd
}

func cmpXZero(rn uint32) uint32 {
	return 0xF100001F | (rn << 5)
}

func fcmpD(rn, rm uint32) uint32 {
	return 0x1E602000 | (rm << 16) | (rn << 5)
}

func faddD(rd, rn, rm uint32) uint32 { return 0x1E602800 | (rm << 16) | (rn << 5) | rd }
func fsubD(rd, rn, rm uint32) uint32 { return 0x1E603800 | (rm << 16) | (rn << 5) | rd }
func fmulD(rd, rn, rm uint32) uint32 { return 0x1E600800 | (rm << 16) | (rn << 5) | rd }
func fdivD(rd, rn, rm uint32) uint32 { return 0x1E601800 | (rm << 16) | (rn << 5) | rd }
func fnegD(rd, rn uint32) uint32     { return 0x1E614000 | (rn << 5) | rd }
func fcvtzs(rd, rn uint32) uint32    { return 0x9E780000 | (rn << 5) | rd }
func scvtf(rd, rn uint32) uint32     { return 0x9E620000 | (rn << 5) | rd }
func sdivX(rd, rn, rm uint32) uint32 { return 0x9AC00C00 | (rm << 16) | (rn << 5) | rd }
func msubX(rd, rn, rm, ra uint32) uint32 {
	return 0x9B008000 | (rm << 16) | (ra << 10) | (rn << 5) | rd
}

func (w *a64Writer) setStatus(status, node int64) {
	w.movImm64(regA, uint64(status))
	w.u32(strX(regA, regCtx, ctxStatusOff))
	w.movImm64(regA, uint64(node))
	w.u32(strX(regA, regCtx, ctxErrorNodeOff))
}

func (w *a64Writer) movImm64(rd uint32, v uint64) {
	w.u32(0xD2800000 | (uint32(v&0xffff) << 5) | rd)
	w.u32(0xF2800000 | (uint32((v>>16)&0xffff) << 5) | (1 << 21) | rd)
	w.u32(0xF2800000 | (uint32((v>>32)&0xffff) << 5) | (2 << 21) | rd)
	w.u32(0xF2800000 | (uint32((v>>48)&0xffff) << 5) | (3 << 21) | rd)
}

func emitMachineLegacy(p *Program) ([]byte, error) {
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
	fail := func(i int, err error) ([]byte, error) {
		return nil, fmt.Errorf("ARM64 native instruction %d: %w", i, err)
	}
	w.u32(addImm64(regStack, regCtx, ctxStackOff))
	for i, in := range p.Bytecode {
		for _, label := range labelAt[i] {
			w.mark(label)
		}
		switch in.Op {
		case OpPushConst:
			w.u32(ldrD(fp0, regCtx, ctxConstsOff+in.A*8))
			w.u32(strD(fp0, regStack, 0))
			w.u32(addImm64(regStack, regStack, 8))
		case OpLoadSlot:
			w.u32(ldrD(fp0, regCtx, ctxSlotsOff+in.A*8))
			w.u32(strD(fp0, regStack, 0))
			w.u32(addImm64(regStack, regStack, 8))
		case OpStoreSlot:
			w.u32(subImm64(regStack, regStack, 8))
			w.u32(ldrD(fp0, regStack, 0))
			w.u32(strD(fp0, regCtx, ctxSlotsOff+in.A*8))
		case OpStoreSlotKeep:
			w.u32(ldrD(fp0, regStack, -8))
			w.u32(strD(fp0, regCtx, ctxSlotsOff+in.A*8))
		case OpDup:
			w.u32(ldrD(fp0, regStack, -8))
			w.u32(strD(fp0, regStack, 0))
			w.u32(addImm64(regStack, regStack, 8))
		case OpSwap:
			w.u32(subImm64(regStack, regStack, 16))
			w.u32(ldrD(fp0, regStack, 0))
			w.u32(ldrD(fp1, regStack, 8))
			w.u32(strD(fp1, regStack, 0))
			w.u32(strD(fp0, regStack, 8))
			w.u32(addImm64(regStack, regStack, 16))
		case OpPop:
			w.u32(subImm64(regStack, regStack, 8))
		case OpAdd, OpSub, OpMul:
			w.u32(subImm64(regStack, regStack, 8))
			w.u32(ldrD(fp1, regStack, 0))
			w.u32(subImm64(regStack, regStack, 8))
			w.u32(ldrD(fp0, regStack, 0))
			switch in.Op {
			case OpAdd:
				w.u32(faddD(fp0, fp0, fp1))
			case OpSub:
				w.u32(fsubD(fp0, fp0, fp1))
			case OpMul:
				w.u32(fmulD(fp0, fp0, fp1))
			}
			w.u32(strD(fp0, regStack, 0))
			w.u32(addImm64(regStack, regStack, 8))
		case OpDiv:
			safe := newLocal()
			errLabel := newLocal()
			done := newLocal()
			w.u32(subImm64(regStack, regStack, 8))
			w.u32(ldrD(fp1, regStack, 0))
			w.u32(ldrD(fp0, regStack, -8))
			w.u32(ldrD(fp2, regCtx, ctxConstsOff))
			w.u32(fcmpD(fp1, fp2))
			w.branchCond(6, safe)
			w.branchCond(0, errLabel)
			w.mark(safe)
			w.u32(fdivD(fp0, fp0, fp1))
			w.u32(strD(fp0, regStack, -8))
			w.branch(done)
			w.mark(errLabel)
			w.setStatus(StatusDivisionByZero, int64(in.ErrorNode))
			w.branch(returnLabel)
			w.mark(done)
		case OpMod:
			errLabel := newLocal()
			deopt := newLocal()
			done := newLocal()
			w.u32(subImm64(regStack, regStack, 8))
			w.u32(ldrD(fp1, regStack, 0))
			w.u32(ldrD(fp0, regStack, -8))
			w.u32(ldrD(fp2, regCtx, ctxConstsOff))
			w.u32(fcmpD(fp1, fp2))
			w.branchCond(6, errLabel)
			w.branchCond(0, errLabel)
			w.u32(fcmpD(fp0, fp2))
			w.branchCond(6, deopt)
			w.u32(fdivD(fp0, fp0, fp1))
			w.u32(ldrD(fp2, regCtx, ctxConstsOff+8))
			w.u32(fcmpD(fp0, fp2))
			w.branchCond(6, deopt)
			w.branchCond(2, deopt)
			w.u32(ldrD(fp2, regCtx, ctxConstsOff+16))
			w.u32(fcmpD(fp0, fp2))
			w.branchCond(6, deopt)
			w.branchCond(3, deopt)
			w.u32(fcvtzs(regA, fp0))
			w.u32(scvtf(fp2, regA))
			w.u32(subImm64(regStack, regStack, 8))
			w.u32(ldrD(fp0, regStack, 0))
			w.u32(fmulD(fp2, fp2, fp1))
			w.u32(fsubD(fp0, fp0, fp2))
			w.u32(strD(fp0, regStack, 0))
			w.u32(addImm64(regStack, regStack, 8))
			w.branch(done)
			w.mark(errLabel)
			w.setStatus(StatusDivisionByZero, int64(in.ErrorNode))
			w.branch(returnLabel)
			w.mark(deopt)
			w.setStatus(StatusDeopt, int64(in.ErrorNode))
			w.branch(returnLabel)
			w.mark(done)

		case OpNeg:
			w.u32(ldrD(fp0, regStack, -8))
			w.u32(fnegD(fp0, fp0))
			w.u32(strD(fp0, regStack, -8))
		case OpCompare:
			w.u32(subImm64(regStack, regStack, 8))
			w.u32(ldrD(fp1, regStack, 0))
			w.u32(subImm64(regStack, regStack, 8))
			w.u32(ldrD(fp0, regStack, 0))
			trueLabel := newLocal()
			falseLabel := newLocal()
			endLabel := newLocal()
			w.u32(fcmpD(fp0, fp1))
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
			w.u32(ldrD(fp0, regCtx, ctxConstsOff))
			w.u32(strD(fp0, regStack, 0))
			w.u32(addImm64(regStack, regStack, 8))
			w.branch(endLabel)
			w.mark(falseLabel)
			w.u32(ldrD(fp0, regCtx, ctxConstsOff))
			w.u32(strD(fp0, regStack, 0))
			w.u32(addImm64(regStack, regStack, 8))
			w.branch(endLabel)
			w.mark(trueLabel)
			w.u32(ldrD(fp0, regCtx, ctxConstsOff+24))
			w.u32(strD(fp0, regStack, 0))
			w.u32(addImm64(regStack, regStack, 8))
			w.mark(endLabel)
		case OpJump:
			w.branch(in.Target)
		case OpBranchFalse, OpBranchTrue:
			w.u32(subImm64(regStack, regStack, 8))
			w.u32(ldrD(fp0, regStack, 0))
			w.u32(ldrD(fp2, regCtx, ctxConstsOff))
			w.u32(fcmpD(fp0, fp2))
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
			addUsed := newLocal()
			timeout := newLocal()
			done := newLocal()
			w.u32(ldrX(regA, regCtx, ctxBudgetOff))
			w.u32(cmpXZero(regA))
			w.branchCond(11, addUsed)
			w.u32(subsImm64(regA, regA, n))
			w.branchCond(4, timeout)
			w.u32(strX(regA, regCtx, ctxBudgetOff))
			w.mark(addUsed)
			w.u32(ldrX(regB, regCtx, ctxUsedOff))
			w.u32(addImm64(regB, regB, n))
			w.u32(strX(regB, regCtx, ctxUsedOff))
			w.branch(done)
			w.mark(timeout)
			w.u32(ldrX(regB, regCtx, ctxUsedOff))
			w.u32(addImm64(regB, regB, n))
			w.u32(strX(regB, regCtx, ctxUsedOff))
			w.setStatus(StatusTimeout, int64(in.ErrorNode))
			w.branch(returnLabel)
			w.mark(done)
		case OpSetRepeatCounter:
			deopt := newLocal()
			done := newLocal()
			w.u32(ldrD(fp0, regStack, -8))
			w.u32(subImm64(regStack, regStack, 8))
			w.u32(fcvtzs(regA, fp0))
			w.u32(scvtf(fp1, regA))
			w.u32(fcmpD(fp0, fp1))
			w.branchCond(6, deopt)
			w.branchCond(1, deopt)
			w.u32(strX(regA, regCtx, ctxCountersOff+in.A*8))
			w.branch(done)
			w.mark(deopt)
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
