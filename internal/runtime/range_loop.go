package runtime

import (
	"lunex/internal/ast"
	"lunex/internal/jit"
	"lunex/internal/resolver"
	"math"
)

const maxRangeCount = 9e18

func rangeElement(start, step float64, k int) float64 {
	return start + float64(float64(k)*step)
}

func (interp *Interpreter) rangeParams(node *ast.Node, env *Environment) (start, step float64, count int, err error) {
	args := node.Args
	if len(args) == 0 {
		return 0, 1, 0, nil
	}
	if len(args) == 1 {
		n, err := interp.evalExpr(args[0], env)
		if err != nil {
			return 0, 0, 0, err
		}
		c := n.ToNumber()
		if math.IsNaN(c) || c < 1 {
			return 0, 1, 0, nil
		}
		if c > maxRangeCount {
			c = maxRangeCount
		}
		return 0, 1, int(c), nil
	}
	startVal, err := interp.evalExpr(args[0], env)
	if err != nil {
		return 0, 0, 0, err
	}
	endVal, err := interp.evalExpr(args[1], env)
	if err != nil {
		return 0, 0, 0, err
	}
	step = 1.0
	if len(args) > 2 {
		stepVal, err := interp.evalExpr(args[2], env)
		if err != nil {
			return 0, 0, 0, err
		}
		step = stepVal.ToNumber()
	}
	start = startVal.ToNumber()
	end := endVal.ToNumber()
	if step == 0 {
		return start, step, 0, nil
	}
	c := math.Max(0, math.Ceil((end-start)/step))
	if math.IsNaN(c) {
		return start, step, 0, nil
	}
	if c > maxRangeCount {
		c = maxRangeCount
	}
	return start, step, int(c), nil
}

func (interp *Interpreter) runRangeJIT(node *ast.Node, env *Environment, start, step float64, done, remaining int) (bool, bool, error) {
	return interp.runJITSetup(node, env, int64(remaining), func(program *jit.Program, ctx *jit.Context) {
		if program.RangeRoot {
			ctx.Slots[program.RangeStart] = start
			ctx.Slots[program.RangeStep] = step
			ctx.Slots[program.RangeIndex] = float64(done)
		}
	})
}

func (interp *Interpreter) execRangeLoop(node *ast.Node, env *Environment) (*Value, error) {
	start, step, count, err := interp.rangeParams(node.Right, env)
	if err != nil {
		return nil, err
	}
	valueSlot := resolver.SlotIndex(node.ScopeInfo, node.Name)
	aliasSlot := -1
	if node.Alias != "" {
		aliasSlot = resolver.SlotIndex(node.ScopeInfo, node.Alias)
	}
	jitDisabled := false
	for k := 0; k < count; k++ {
		if !jitDisabled {
			handled, deopt, err := interp.runRangeJIT(node, env, start, step, k, count-k)
			if err != nil {
				return nil, err
			}
			if handled {
				return Undefined, nil
			}
			if deopt {
				jitDisabled = true
			}
		}
		iterEnv := NewResolvedEnvironment(env, resolver.SlotCount(node.ScopeInfo))
		iterEnv.DefineSlot(0, valueSlot, node.Name, NumberVal(rangeElement(start, step, k)), node.IsConst)
		if node.Alias != "" {
			iterEnv.DefineSlot(0, aliasSlot, node.Alias, NumberVal(float64(k)), node.IsConst)
		}
		if _, err := interp.execNode(node.Body, iterEnv); err != nil {
			if _, ok := err.(*breakError); ok {
				return Undefined, nil
			}
			if _, ok := err.(*continueError); ok {
				continue
			}
			return nil, err
		}
	}
	return Undefined, nil
}
