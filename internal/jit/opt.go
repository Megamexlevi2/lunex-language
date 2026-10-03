package jit

const maxTickGroup = 4000

func coalesceTicks(p *Program) {
	targets := make(map[int]bool, len(p.Labels))
	for _, index := range p.Labels {
		if index >= 0 {
			targets[index] = true
		}
	}
	open := -1
	for i := range p.Bytecode {
		if targets[i] {
			open = -1
		}
		in := &p.Bytecode[i]
		switch in.Op {
		case OpTick:
			if in.A <= 0 {
				continue
			}
			if open >= 0 && p.Bytecode[open].A+in.A <= maxTickGroup {
				p.Bytecode[open].A += in.A
				in.A = 0
			} else {
				open = i
			}
		case OpJump, OpBranchFalse, OpBranchTrue, OpBreak, OpReturn, OpRepeatCheck, OpDiv, OpMod, OpSetRepeatCounter:
			open = -1
		}
	}
}
