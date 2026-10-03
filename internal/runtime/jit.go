package runtime

import (
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/errfmt"
	"lunex/internal/jit"
	"os"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	nativeConfigOnce sync.Once
	nativeEnabled    bool
	nativeHotLimit   int64
	nativeTrace      bool
)

func loadNativeConfig() {
	nativeConfigOnce.Do(func() {
		nativeEnabled = resolveNativeBool("LUNEX_NATIVE", true)
		nativeHotLimit = resolveNativeInt("LUNEX_NATIVE_HOT", 32)
		nativeTrace = resolveNativeBool("LUNEX_NATIVE_TRACE", false)
		jit.SetVerify(resolveNativeBool("LUNEX_NATIVE_VERIFY", true))
		if nativeTrace {
			fmt.Fprintf(os.Stderr, "lunex native: enabled=%v available=%v hot=%d\n", nativeEnabled, jit.Available(), nativeHotLimit)
		}
	})
}

func SetNativeEnabled(enabled bool) {
	loadNativeConfig()
	nativeEnabled = enabled
}

type jitLoopState struct {
	hits    int64
	current *jit.Program
	never   bool
	notes   map[string]bool
}

func (interp *Interpreter) jitNote(node *ast.Node, reason string) {
	if !nativeTrace || interp == nil || node == nil {
		return
	}
	interp.jitMu.Lock()
	state := interp.jitLoops[node]
	if state == nil {
		interp.jitMu.Unlock()
		return
	}
	if state.notes == nil {
		state.notes = make(map[string]bool)
	}
	seen := state.notes[reason]
	state.notes[reason] = true
	interp.jitMu.Unlock()
	if !seen {
		fmt.Fprintf(os.Stderr, "lunex native: loop at line %d: %s\n", node.Line, reason)
	}
}

func resolveNativeBool(name string, def bool) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	switch strings.ToLower(raw) {
	case "0", "false", "off", "no":
		return false
	case "1", "true", "on", "yes":
		return true
	default:
		return def
	}
}

func resolveNativeInt(name string, def int64) int64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 1 {
		return def
	}
	return n
}

func (interp *Interpreter) ensureJITMap() {
	if interp.jitLoops == nil {
		interp.jitLoops = make(map[*ast.Node]*jitLoopState)
	}
}

func (interp *Interpreter) selectJIT(node *ast.Node) (*jit.Program, bool) {
	loadNativeConfig()
	if interp == nil || node == nil || !nativeEnabled || !jit.Available() {
		return nil, true
	}
	interp.jitMu.Lock()
	defer interp.jitMu.Unlock()
	interp.ensureJITMap()
	state := interp.jitLoops[node]
	if state == nil {
		state = &jitLoopState{}
		interp.jitLoops[node] = state
	}
	if state.never {
		return nil, true
	}
	if state.current != nil {
		return state.current, false
	}
	state.hits++
	if state.hits < nativeHotLimit {
		return nil, false
	}
	tier1, err := jit.CompileLoop(node, 1)
	if err != nil {
		state.never = true
		if nativeTrace {
			fmt.Fprintf(os.Stderr, "lunex native: loop at line %d rejected: %v\n", node.Line, err)
		}
		return nil, true
	}
	state.current = tier1
	tier2, tier2Err := jit.CompileLoop(node, 2)
	if tier2Err == nil {
		state.current = tier2
		_ = tier1.Close()
	} else if nativeTrace {
		fmt.Fprintf(os.Stderr, "lunex native: loop at line %d tier 2 rejected: %v\n", node.Line, tier2Err)
	}
	if nativeTrace {
		fmt.Fprintf(os.Stderr, "lunex native: loop at line %d compiled after %d iterations, tier=%d fast=%v\n", node.Line, state.hits, state.current.Tier, len(state.current.FastCode) != 0)
		if state.current.FastNote != "" {
			fmt.Fprintf(os.Stderr, "lunex native: loop at line %d fast path rejected: %s\n", node.Line, state.current.FastNote)
		}
	}
	return state.current, false
}

func (interp *Interpreter) jitContext(program *jit.Program, env *Environment, repeatRemaining int64) (*jit.Context, string) {
	ctx := program.NewContext()
	if program.RootRepeat {
		ctx.Counters[0] = repeatRemaining
	}
	if interp.maxExecSteps <= 0 {
		ctx.Budget = -1
	} else {
		left := interp.maxExecSteps - atomic.LoadInt64(&interp.execSteps)
		if left < 0 {
			left = 0
		}
		ctx.Budget = left
	}
	for i, ref := range program.Refs {
		if i >= len(program.Reads) || !program.Reads[i] {
			continue
		}
		value := env.GetSlotAddr(ref.Hops, ref.Slot)
		if value == nil || value.Tag != TypeNumber {
			return nil, fmt.Sprintf("slot %d (hops %d) does not hold a number", ref.Slot, ref.Hops)
		}
		ctx.Slots[i] = value.NumVal
	}
	for _, write := range program.Writes {
		if jitSlotConst(env, write.Reference, write.Name) {
			return nil, fmt.Sprintf("write target %q is constant", write.Name)
		}
	}
	return ctx, ""
}

func jitSlotConst(env *Environment, ref jit.Reference, name string) bool {
	cur := env
	for i := 0; i < ref.Hops && cur != nil; i++ {
		cur = cur.parent
	}
	if cur == nil {
		return false
	}
	return cur.isConstLocal(name)
}

type jitSnapshot struct {
	env    *Environment
	name   string
	slot   int
	value  *Value
	varVal *Value
	hadVar bool
}

func jitEnvironmentSafe(program *jit.Program, env *Environment) bool {
	for _, ref := range program.Refs {
		if ref.Hops < 0 {
			continue
		}
		cur := env
		for i := 0; i < ref.Hops && cur != nil; i++ {
			cur = cur.parent
		}
		if cur == nil || cur.isEscaped() {
			return false
		}
	}
	return true
}

func snapshotJITWrites(program *jit.Program, env *Environment) []jitSnapshot {
	out := make([]jitSnapshot, 0, len(program.Writes))
	for _, write := range program.Writes {
		cur := env
		for i := 0; i < write.Hops && cur != nil; i++ {
			cur = cur.parent
		}
		if cur == nil {
			continue
		}
		var slotValue *Value
		if write.Slot >= 0 && write.Slot < len(cur.slots) {
			slotValue = cur.slots[write.Slot]
		}
		varValue, hadVar := cur.vars[write.Name]
		out = append(out, jitSnapshot{env: cur, name: write.Name, slot: write.Slot, value: slotValue, varVal: varValue, hadVar: hadVar})
	}
	return out
}

func restoreJITWrites(snapshot []jitSnapshot) {
	for _, item := range snapshot {
		if item.env == nil {
			continue
		}
		if item.slot >= 0 && item.slot < len(item.env.slots) {
			item.env.slots[item.slot] = item.value
		}
		if item.hadVar {
			item.env.vars[item.name] = item.varVal
		} else {
			delete(item.env.vars, item.name)
		}
	}
}

func (interp *Interpreter) commitJIT(program *jit.Program, ctx *jit.Context, env *Environment) error {
	committed := make(map[jit.Reference]bool, len(program.Writes))
	for _, write := range program.Writes {
		if committed[write.Reference] {
			continue
		}
		committed[write.Reference] = true
		if err := env.SetSlot(write.Hops, write.Slot, write.Name, NumberVal(ctx.Slots[findReferenceIndex(program.Refs, write.Reference)])); err != nil {
			return err
		}
	}
	return nil
}

func findReferenceIndex(refs []jit.Reference, target jit.Reference) int {
	for i, ref := range refs {
		if ref == target {
			return i
		}
	}
	return -1
}

func (interp *Interpreter) runJIT(node *ast.Node, env *Environment, repeatRemaining int64) (handled bool, deopt bool, err error) {
	return interp.runJITSetup(node, env, repeatRemaining, nil)
}

func (interp *Interpreter) runJITSetup(node *ast.Node, env *Environment, repeatRemaining int64, setup func(*jit.Program, *jit.Context)) (handled bool, deopt bool, err error) {
	program, disable := interp.selectJIT(node)
	if program == nil {
		return false, disable, nil
	}
	if !jitEnvironmentSafe(program, env) {
		interp.jitNote(node, "environment is captured by a closure or task")
		return false, true, nil
	}
	snapshot := snapshotJITWrites(program, env)
	ctx, reason := interp.jitContext(program, env, repeatRemaining)
	if ctx == nil {
		interp.jitNote(node, reason)
		return false, true, nil
	}
	if setup != nil {
		setup(program, ctx)
	}
	if err := program.Run(ctx); err != nil {
		interp.jitNote(node, "native run failed: "+err.Error())
		return false, true, err
	}
	goruntime.KeepAlive(ctx)
	atomic.AddInt64(&interp.execSteps, ctx.Used)
	switch ctx.Status {
	case jit.StatusDeopt:
		restoreJITWrites(snapshot)
		interp.jitNote(node, "native code requested deoptimization")
		return false, true, nil
	case jit.StatusTimeout:
		if err := interp.commitJIT(program, ctx, env); err != nil {
			return true, false, err
		}
		runtimeNode := jitErrorNode(program, ctx.ErrorNode)
		msg := fmt.Sprintf("execution budget exceeded after %d steps; possible infinite loop, runaway recursion, or unbounded spawn", interp.maxExecSteps)
		return true, false, interp.runtimeError(errfmt.KindTimeout, errfmt.ErrTimeout, msg, runtimeNode, []string{
			"add a terminating condition or reduce the amount of work per run",
			"limit recursive calls, loops, and background tasks",
		})
	case jit.StatusDivisionByZero:
		if err := interp.commitJIT(program, ctx, env); err != nil {
			return true, false, err
		}
		runtimeNode := jitErrorNode(program, ctx.ErrorNode)
		message := "division by zero"
		if runtimeNode != nil && runtimeNode.Op == "%" {
			message = "modulo by zero"
		}
		return true, false, interp.runtimeError(errfmt.KindArithmetic, errfmt.ErrDivisionByZero, message, runtimeNode, nil)
	case jit.StatusBreak:
		if err := interp.commitJIT(program, ctx, env); err != nil {
			return true, false, err
		}
		return true, false, nil
	case jit.StatusOK:
		if err := interp.commitJIT(program, ctx, env); err != nil {
			return true, false, err
		}
		interp.jitNote(node, "ran natively to completion")
		return true, false, nil
	default:
		interp.jitNote(node, fmt.Sprintf("unexpected native status %d", ctx.Status))
		return false, true, nil
	}
}

func jitErrorNode(program *jit.Program, id int64) *ast.Node {
	if id < 0 || id >= int64(len(program.Nodes)) {
		return nil
	}
	return program.Nodes[id]
}
