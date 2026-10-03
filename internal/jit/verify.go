package jit

import (
	"fmt"
	"math"
)

var verifyNative = true

func SetVerify(enabled bool) {
	verifyNative = enabled
}

const oracleStepLimit = 2000000

type oracleResult struct {
	slots     [MaxSlots]float64
	counters  [MaxCounter]int64
	used      int64
	status    int64
	errorNode int64
}

func oracleFalsy(v float64) bool {
	return v == 0 || math.IsNaN(v)
}

func oracleRun(p *Program, ctx *Context) (res oracleResult, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	var r oracleResult
	r.slots = ctx.Slots
	r.counters = ctx.Counters
	r.used = ctx.Used
	r.errorNode = ctx.ErrorNode
	budget := ctx.Budget
	stack := make([]float64, 0, 32)
	push := func(v float64) { stack = append(stack, v) }
	pop := func() float64 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return v
	}
	pc := 0
	for steps := 0; steps < oracleStepLimit; steps++ {
		if pc < 0 || pc >= len(p.Bytecode) {
			return r, true
		}
		in := p.Bytecode[pc]
		pc++
		switch in.Op {
		case OpPushConst:
			push(ctx.Consts[in.A])
		case OpLoadSlot:
			push(r.slots[in.A])
		case OpStoreSlot:
			r.slots[in.A] = pop()
		case OpStoreSlotKeep:
			r.slots[in.A] = stack[len(stack)-1]
		case OpDup:
			push(stack[len(stack)-1])
		case OpSwap:
			n := len(stack)
			stack[n-1], stack[n-2] = stack[n-2], stack[n-1]
		case OpPop:
			pop()
		case OpAdd, OpSub, OpMul:
			b := pop()
			a := pop()
			switch in.Op {
			case OpAdd:
				push(a + b)
			case OpSub:
				push(a - b)
			default:
				push(a * b)
			}
		case OpDiv:
			b := pop()
			a := pop()
			if b == 0 {
				r.status = StatusDivisionByZero
				r.errorNode = int64(in.ErrorNode)
				return r, true
			}
			push(a / b)
		case OpMod:
			b := pop()
			a := pop()
			if b == 0 || math.IsNaN(b) {
				r.status = StatusDivisionByZero
				r.errorNode = int64(in.ErrorNode)
				return r, true
			}
			q := a / b
			if math.IsNaN(a) || math.IsInf(a, 0) || math.IsNaN(q) || math.Abs(q) >= math.Ldexp(1, 63) {
				return r, false
			}
			push(math.Mod(a, b))
		case OpNeg:
			stack[len(stack)-1] = -stack[len(stack)-1]
		case OpCompare:
			b := pop()
			a := pop()
			var res bool
			if math.IsNaN(a) || math.IsNaN(b) {
				res = CompareOp(in.A) == CompareNE
			} else {
				switch CompareOp(in.A) {
				case CompareLT:
					res = a < b
				case CompareLE:
					res = a <= b
				case CompareGT:
					res = a > b
				case CompareGE:
					res = a >= b
				case CompareEQ:
					res = a == b
				default:
					res = a != b
				}
			}
			push(boolFloat(res))
		case OpJump:
			pc = p.Labels[in.Target]
		case OpBranchFalse, OpBranchTrue:
			v := pop()
			jump := oracleFalsy(v)
			if in.Op == OpBranchTrue {
				jump = !jump
			}
			if jump {
				pc = p.Labels[in.Target]
			}
		case OpTick:
			n := int64(in.A)
			if n <= 0 {
				continue
			}
			r.used += n
			if budget >= 0 {
				if budget < n {
					r.status = StatusTimeout
					r.errorNode = int64(in.ErrorNode)
					return r, true
				}
				budget -= n
			}
		case OpSetRepeatCounter:
			v := pop()
			if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) || v >= math.Ldexp(1, 63) || v < -math.Ldexp(1, 63) {
				return r, false
			}
			r.counters[in.A] = int64(v)
		case OpRepeatCheck:
			if r.counters[in.A] == 0 {
				pc = p.Labels[in.Target]
			}
		case OpRepeatDecrement:
			if r.counters[in.A] > 0 {
				r.counters[in.A]--
			}
		case OpBreak:
			r.status = StatusBreak
			pc = p.Labels[in.Target]
		case OpReturn:
			return r, true
		default:
			return r, false
		}
	}
	return r, false
}

func probeValue(seed, i int) float64 {
	switch seed {
	case 0:
		return 0
	case 1:
		return float64(i + 1)
	case 2:
		return -float64(i + 1)
	case 3:
		return 0.5 + float64(i)
	case 4:
		return 1e6 + float64(i)
	case 5:
		return -3.75
	case 6:
		return float64(7 * (i + 1))
	case 7:
		return 1e12 + float64(i)
	default:
		if i%2 == 0 {
			return math.NaN()
		}
		return 2
	}
}

const probeSeeds = 9

func probeContext(p *Program, seed int, budget int64) *Context {
	ctx := p.NewContext()
	ctx.Budget = budget
	for i := range p.Refs {
		ctx.Slots[i] = probeValue(seed, i)
	}
	if p.RootRepeat {
		ctx.Counters[0] = int64(3 + seed)
	}
	return ctx
}

func sameFloat(a, b float64, tolerant bool) bool {
	if math.IsNaN(a) && math.IsNaN(b) {
		return true
	}
	if math.Float64bits(a) == math.Float64bits(b) {
		return true
	}
	if a == b {
		return true
	}
	if !tolerant || math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		return false
	}
	scale := math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
	return math.Abs(a-b) <= 1e-9*scale
}

func hasOp(p *Program, op OpCode) bool {
	for _, in := range p.Bytecode {
		if in.Op == op {
			return true
		}
	}
	return false
}

func runMachine(code []byte, ctx *Context) error {
	if len(code) == 0 {
		return fmt.Errorf("empty machine code")
	}
	return runNative(codeAddress(code), uintptrOfContext(ctx))
}

func verifyCode(p *Program, code []byte, label string) error {
	tolerant := hasOp(p, OpMod)
	budgets := [2]int64{600, 41}
	for seed := 0; seed < probeSeeds; seed++ {
		for _, budget := range budgets {
			want, ok := oracleRun(p, probeContext(p, seed, budget))
			if !ok {
				continue
			}
			got := probeContext(p, seed, budget)
			if err := runMachine(code, got); err != nil {
				return err
			}
			if got.Status == StatusDeopt {
				continue
			}
			if got.Status != want.status {
				return fmt.Errorf("%s self-check: status %d, expected %d (seed %d budget %d)", label, got.Status, want.status, seed, budget)
			}
			if got.Used != want.used {
				return fmt.Errorf("%s self-check: used %d, expected %d (seed %d budget %d)", label, got.Used, want.used, seed, budget)
			}
			if (want.status == StatusTimeout || want.status == StatusDivisionByZero) && got.ErrorNode != want.errorNode {
				return fmt.Errorf("%s self-check: error node %d, expected %d (seed %d budget %d)", label, got.ErrorNode, want.errorNode, seed, budget)
			}
			for i := range p.Refs {
				if !sameFloat(got.Slots[i], want.slots[i], tolerant) {
					return fmt.Errorf("%s self-check: slot %d is %v, expected %v (seed %d budget %d)", label, i, got.Slots[i], want.slots[i], seed, budget)
				}
			}
			for i := 0; i < p.CounterCount && i < MaxCounter; i++ {
				if got.Counters[i] != want.counters[i] {
					return fmt.Errorf("%s self-check: counter %d is %d, expected %d (seed %d budget %d)", label, i, got.Counters[i], want.counters[i], seed, budget)
				}
			}
		}
	}
	return nil
}

func selfCheck(p *Program) error {
	if !verifyNative || !Available() {
		return nil
	}
	if err := verifyCode(p, p.Code, "native"); err != nil {
		return err
	}
	if len(p.FastCode) != 0 {
		if err := verifyCode(p, p.FastCode, "fast native"); err != nil {
			p.FastNote = err.Error()
			_ = releaseExecutable(p.FastCode)
			p.FastCode = nil
		}
	}
	return nil
}
