package runtime

import (
	"fmt"
	"sync/atomic"

	"lunex/internal/ast"
	"lunex/internal/errfmt"
)

func (interp *Interpreter) resetExecutionBudget() {
	atomic.StoreInt64(&interp.execSteps, 0)
}

func (interp *Interpreter) consumeExecutionBudget(node *ast.Node) error {
	if interp == nil || interp.maxExecSteps <= 0 {
		return nil
	}
	steps := atomic.AddInt64(&interp.execSteps, 1)
	if steps <= interp.maxExecSteps {
		return nil
	}
	msg := fmt.Sprintf("execution budget exceeded after %d steps; possible infinite loop, runaway recursion, or unbounded task creation", interp.maxExecSteps)
	return interp.runtimeError(errfmt.KindTimeout, errfmt.ErrTimeout, msg, node, []string{
		"add a terminating condition or reduce the amount of work per run",
		"limit recursive calls, loops, and background tasks",
	})
}
