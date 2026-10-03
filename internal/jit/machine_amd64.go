//go:build amd64

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
	regCtx   = 10
	regStack = 11
	xmm0     = 0
	xmm1     = 1
	xmm2     = 2
)

type x86Patch struct {
	pos   int
	label int
	kind  uint8
}

type x86Writer struct {
	b       []byte
	labels  map[int]int
	patches []x86Patch
}

const (
	patchJmp = 1
	patchJcc = 2
)

func (w *x86Writer) u8(v byte) { w.b = append(w.b, v) }
func (w *x86Writer) u32(v uint32) {
	w.b = append(w.b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}
func (w *x86Writer) u64(v uint64) {
	w.u32(uint32(v))
	w.u32(uint32(v >> 32))
}
func (w *x86Writer) mark(label int) { w.labels[label] = len(w.b) }
func (w *x86Writer) jmp(label int) {
	w.u8(0xE9)
	pos := len(w.b)
	w.u32(0)
	w.patches = append(w.patches, x86Patch{pos: pos, label: label, kind: patchJmp})
}
func (w *x86Writer) jcc(cc byte, label int) {
	w.u8(0x0F)
	w.u8(0x80 | cc)
	pos := len(w.b)
	w.u32(0)
	w.patches = append(w.patches, x86Patch{pos: pos, label: label, kind: patchJcc})
}
func (w *x86Writer) patchAll() error {
	for _, p := range w.patches {
		target, ok := w.labels[p.label]
		if !ok {
			return fmt.Errorf("unresolved native label %d", p.label)
		}
		base := p.pos + 4
		rel := int64(target) - int64(base)
		if rel < -2147483648 || rel > 2147483647 {
			return fmt.Errorf("amd64 relative branch out of range: %d", rel)
		}
		v := uint32(int32(rel))
		w.b[p.pos] = byte(v)
		w.b[p.pos+1] = byte(v >> 8)
		w.b[p.pos+2] = byte(v >> 16)
		w.b[p.pos+3] = byte(v >> 24)
	}
	return nil
}

func x86Rex(w, r, b bool) byte {
	v := byte(0x40)
	if w {
		v |= 0x08
	}
	if r {
		v |= 0x04
	}
	if b {
		v |= 0x01
	}
	return v
}

func (w *x86Writer) addRegImm8(reg byte, imm byte) {
	w.u8(x86Rex(true, false, reg >= 8))
	w.u8(0x83)
	w.u8(0xC0 | (reg & 7))
	w.u8(imm)
}

func (w *x86Writer) subRegImm8(reg byte, imm byte) {
	w.u8(x86Rex(true, false, reg >= 8))
	w.u8(0x83)
	w.u8(0xE8 | (reg & 7))
	w.u8(imm)
}

func (w *x86Writer) cmpRegImm8(reg byte, imm byte) {
	w.u8(x86Rex(true, false, reg >= 8))
	w.u8(0x83)
	w.u8(0xF8 | (reg & 7))
	w.u8(imm)
}

func (w *x86Writer) leaRegDisp32(dst, base byte, disp int) {
	w.u8(x86Rex(true, dst >= 8, base >= 8))
	w.u8(0x8D)
	w.u8(0x80 | ((dst & 7) << 3) | (base & 7))
	w.u32(uint32(int32(disp)))
}
func (w *x86Writer) movsdLoad(xmm byte, base byte, disp int) {
	w.u8(0xF2)
	w.u8(x86Rex(false, xmm >= 8, base >= 8))
	w.u8(0x0F)
	w.u8(0x10)
	w.u8(0x80 | ((xmm & 7) << 3) | (base & 7))
	w.u32(uint32(int32(disp)))
}

func (w *x86Writer) movsdStore(base byte, disp int, xmm byte) {
	w.u8(0xF2)
	w.u8(x86Rex(false, xmm >= 8, base >= 8))
	w.u8(0x0F)
	w.u8(0x11)
	w.u8(0x80 | ((xmm & 7) << 3) | (base & 7))
	w.u32(uint32(int32(disp)))
}

func (w *x86Writer) movqLoad(reg byte, base byte, disp int) {
	w.u8(x86Rex(true, reg >= 8, base >= 8))
	w.u8(0x8B)
	w.u8(0x80 | ((reg & 7) << 3) | (base & 7))
	w.u32(uint32(int32(disp)))
}

func (w *x86Writer) movqStore(base byte, disp int, reg byte) {
	w.u8(x86Rex(true, reg >= 8, base >= 8))
	w.u8(0x89)
	w.u8(0x80 | ((reg & 7) << 3) | (base & 7))
	w.u32(uint32(int32(disp)))
}

func (w *x86Writer) movqImmReg(reg byte, v uint64) {
	w.u8(x86Rex(true, false, reg >= 8))
	w.u8(0xB8 | (reg & 7))
	w.u64(v)
}

func (w *x86Writer) movqImmMem(base byte, disp int, v uint64) {
	w.movqImmReg(0, v)
	w.movqStore(base, disp, 0)
}

func (w *x86Writer) movsdRR(dst, src byte) {
	w.u8(0xF2)
	w.u8(0x0F)
	w.u8(0x10)
	w.u8(0xC0 | ((dst & 7) << 3) | (src & 7))
}

func (w *x86Writer) ucomisd(a, b byte) {
	w.u8(0x66)
	w.u8(0x0F)
	w.u8(0x2E)
	w.u8(0xC0 | ((a & 7) << 3) | (b & 7))
}

func (w *x86Writer) arithmetic(op byte) {
	w.u8(0xF2)
	w.u8(0x0F)
	w.u8(op)
	w.u8(0xC0 | (xmm1 & 7))
}

func (w *x86Writer) negTop() {
	w.movsdLoad(xmm0, regStack, -8)
	w.movqImmReg(0, 0x8000000000000000)
	w.u8(0x66)
	w.u8(0x48)
	w.u8(0x0F)
	w.u8(0x6E)
	w.u8(0xC8)
	w.u8(0x66)
	w.u8(0x0F)
	w.u8(0xEF)
	w.u8(0xC1)
	w.movsdStore(regStack, -8, xmm0)
}

func (w *x86Writer) setStatus(status int64, errorNode int64) {
	w.movqImmMem(regCtx, ctxStatusOff, uint64(status))
	w.movqImmMem(regCtx, ctxErrorNodeOff, uint64(errorNode))
}

func (w *x86Writer) addMemOne(base byte, disp int) {
	w.u8(x86Rex(true, false, base >= 8))
	w.u8(0x83)
	w.u8(0x80 | (base & 7))
	w.u32(uint32(int32(disp)))
	w.u8(1)
}

func (w *x86Writer) addRegImm32(reg byte, imm int32) {
	w.u8(x86Rex(true, false, reg >= 8))
	w.u8(0x81)
	w.u8(0xC0 | (reg & 7))
	w.u32(uint32(imm))
}

func (w *x86Writer) subRegImm32(reg byte, imm int32) {
	w.u8(x86Rex(true, false, reg >= 8))
	w.u8(0x81)
	w.u8(0xE8 | (reg & 7))
	w.u32(uint32(imm))
}

func (w *x86Writer) addMemImm32(base byte, disp int, imm int32) {
	w.u8(x86Rex(true, false, base >= 8))
	w.u8(0x81)
	w.u8(0x80 | (base & 7))
	w.u32(uint32(int32(disp)))
	w.u32(uint32(imm))
}

func (w *x86Writer) subMemOne(base byte, disp int) {
	w.u8(x86Rex(true, false, base >= 8))
	w.u8(0x83)
	w.u8(0xA8 | (base & 7))
	w.u32(uint32(int32(disp)))
	w.u8(1)
}

func (w *x86Writer) loadSlot(idx int) {
	w.movsdLoad(xmm0, regCtx, ctxSlotsOff+idx*8)
	w.movsdStore(regStack, 0, xmm0)
	w.addRegImm8(regStack, 8)
}

func (w *x86Writer) pushConst(idx int) {
	w.movsdLoad(xmm0, regCtx, ctxConstsOff+idx*8)
	w.movsdStore(regStack, 0, xmm0)
	w.addRegImm8(regStack, 8)
}

func (w *x86Writer) finishStackValue() {
	w.movsdStore(regStack, 0, xmm0)
	w.addRegImm8(regStack, 8)
}

func emitMachineLegacy(p *Program) ([]byte, error) {
	w := &x86Writer{labels: make(map[int]int)}
	returnLabel := 1 << 29
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
	fail := func(i int, err error) ([]byte, error) {
		return nil, fmt.Errorf("amd64 native instruction %d: %w", i, err)
	}
	w.leaRegDisp32(regStack, regCtx, ctxStackOff)
	for i, in := range p.Bytecode {
		for _, label := range labelAt[i] {
			w.mark(label)
		}
		switch in.Op {
		case OpPushConst:
			w.pushConst(in.A)
		case OpLoadSlot:
			w.loadSlot(in.A)
		case OpStoreSlot:
			w.subRegImm8(regStack, 8)
			w.movsdLoad(xmm0, regStack, 0)
			w.movsdStore(regCtx, ctxSlotsOff+in.A*8, xmm0)
		case OpStoreSlotKeep:
			w.movsdLoad(xmm0, regStack, -8)
			w.movsdStore(regCtx, ctxSlotsOff+in.A*8, xmm0)
		case OpDup:
			w.movsdLoad(xmm0, regStack, -8)
			w.movsdStore(regStack, 0, xmm0)
			w.addRegImm8(regStack, 8)
		case OpSwap:
			w.subRegImm8(regStack, 16)
			w.movsdLoad(xmm0, regStack, 0)
			w.movsdLoad(xmm1, regStack, 8)
			w.movsdStore(regStack, 0, xmm1)
			w.movsdStore(regStack, 8, xmm0)
			w.addRegImm8(regStack, 16)
		case OpPop:
			w.subRegImm8(regStack, 8)
		case OpAdd, OpSub, OpMul:
			w.subRegImm8(regStack, 8)
			w.movsdLoad(xmm1, regStack, 0)
			w.subRegImm8(regStack, 8)
			w.movsdLoad(xmm0, regStack, 0)
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
			w.u8(0xC1)
			w.finishStackValue()
		case OpDiv:
			safe := newLocal()
			errLabel := newLocal()
			done := newLocal()
			w.subRegImm8(regStack, 8)
			w.movsdLoad(xmm1, regStack, 0)
			w.movsdLoad(xmm0, regStack, -8)
			w.movsdLoad(xmm2, regCtx, ctxConstsOff)
			w.ucomisd(xmm1, xmm2)
			w.jcc(0x8A, safe)
			w.jcc(0x84, errLabel)
			w.mark(safe)
			w.u8(0xF2)
			w.u8(0x0F)
			w.u8(0x5E)
			w.u8(0xC1)
			w.movsdStore(regStack, -8, xmm0)
			w.jmp(done)
			w.mark(errLabel)
			w.setStatus(StatusDivisionByZero, int64(in.ErrorNode))
			w.jmp(returnLabel)
			w.mark(done)
		case OpMod:
			errLabel := newLocal()
			deopt := newLocal()
			done := newLocal()
			w.subRegImm8(regStack, 8)
			w.movsdLoad(xmm1, regStack, 0)
			w.movsdLoad(xmm2, regCtx, ctxConstsOff)
			w.ucomisd(xmm1, xmm2)
			w.jcc(0x8A, errLabel)
			w.jcc(0x84, errLabel)
			w.movsdLoad(xmm0, regStack, -8)
			w.ucomisd(xmm0, xmm2)
			w.jcc(0x8A, deopt)
			w.u8(0xF2)
			w.u8(0x0F)
			w.u8(0x5E)
			w.u8(0xC1)
			w.movsdLoad(xmm2, regCtx, ctxConstsOff+8)
			w.ucomisd(xmm0, xmm2)
			w.jcc(0x8A, deopt)
			w.jcc(0x83, deopt)
			w.movsdLoad(xmm2, regCtx, ctxConstsOff+16)
			w.ucomisd(xmm0, xmm2)
			w.jcc(0x8A, deopt)
			w.jcc(0x82, deopt)
			w.u8(0xF2)
			w.u8(0x48)
			w.u8(0x0F)
			w.u8(0x2C)
			w.u8(0xC0)
			w.u8(0xF2)
			w.u8(0x48)
			w.u8(0x0F)
			w.u8(0x2A)
			w.u8(0xD0)
			w.subRegImm8(regStack, 8)
			w.movsdLoad(xmm0, regStack, 0)
			w.u8(0xF2)
			w.u8(0x0F)
			w.u8(0x59)
			w.u8(0xD1)
			w.u8(0xF2)
			w.u8(0x0F)
			w.u8(0x5C)
			w.u8(0xC2)
			w.movsdStore(regStack, 0, xmm0)
			w.addRegImm8(regStack, 8)
			w.jmp(done)
			w.mark(errLabel)
			w.setStatus(StatusDivisionByZero, int64(in.ErrorNode))
			w.jmp(returnLabel)
			w.mark(deopt)
			w.setStatus(StatusDeopt, int64(in.ErrorNode))
			w.jmp(returnLabel)
			w.mark(done)

		case OpNeg:
			w.negTop()
		case OpCompare:
			w.subRegImm8(regStack, 8)
			w.movsdLoad(xmm1, regStack, 0)
			w.subRegImm8(regStack, 8)
			w.movsdLoad(xmm0, regStack, 0)
			localTrue := newLocal()
			localFalse := newLocal()
			localEnd := newLocal()
			w.ucomisd(xmm0, xmm1)
			if CompareOp(in.A) == CompareNE {
				w.jcc(0x8A, localTrue)
				w.jcc(0x85, localTrue)
			} else {
				w.jcc(0x8A, localFalse)
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
				w.jcc(cc, localTrue)
			}
			w.pushConst(0)
			w.jmp(localEnd)
			w.mark(localFalse)
			w.pushConst(0)
			w.jmp(localEnd)
			w.mark(localTrue)
			w.pushConst(3)
			w.mark(localEnd)
		case OpJump:
			w.jmp(in.Target)
		case OpBranchFalse, OpBranchTrue:
			w.subRegImm8(regStack, 8)
			w.movsdLoad(xmm0, regStack, 0)
			w.movsdLoad(xmm2, regCtx, ctxConstsOff)
			w.ucomisd(xmm0, xmm2)
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
			addUsed := newLocal()
			timeout := newLocal()
			done := newLocal()
			w.movqLoad(0, regCtx, ctxBudgetOff)
			w.u8(0x48)
			w.u8(0x85)
			w.u8(0xC0)
			w.jcc(0x88, addUsed)
			w.u8(0x48)
			w.u8(0x2D)
			w.u32(uint32(n))
			w.jcc(0x88, timeout)
			w.movqStore(regCtx, ctxBudgetOff, 0)
			w.mark(addUsed)
			w.addMemImm32(regCtx, ctxUsedOff, n)
			w.jmp(done)
			w.mark(timeout)
			w.addMemImm32(regCtx, ctxUsedOff, n)
			w.setStatus(StatusTimeout, int64(in.ErrorNode))
			w.jmp(returnLabel)
			w.mark(done)
		case OpSetRepeatCounter:
			deopt := newLocal()
			done := newLocal()
			w.movsdLoad(xmm0, regStack, -8)
			w.subRegImm8(regStack, 8)
			w.u8(0xF2)
			w.u8(0x48)
			w.u8(0x0F)
			w.u8(0x2C)
			w.u8(0xC0)
			w.u8(0xF2)
			w.u8(0x48)
			w.u8(0x0F)
			w.u8(0x2A)
			w.u8(0xC8)
			w.ucomisd(xmm0, xmm1)
			w.jcc(0x8A, deopt)
			w.jcc(0x85, deopt)
			w.movqStore(regCtx, ctxCountersOff+in.A*8, 0)
			w.jmp(done)
			w.mark(deopt)
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
	w.mark(returnLabel)
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
