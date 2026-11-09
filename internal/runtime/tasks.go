package runtime

import (
	"fmt"
	"os"
	goruntime "runtime"
	"sync"

	"lunex/internal/ast"
	"lunex/internal/jit"
)

type Task struct {
	mu     sync.Mutex
	done   bool
	state  string
	result *Value
	err    error
}

type taskEventState struct {
	mu         sync.Mutex
	cond       *sync.Cond
	generation uint64
}

type taskJob struct {
	task *Task
	run  func()
}

type taskScheduler struct {
	mu      sync.Mutex
	cond    *sync.Cond
	queue   []taskJob
	head    int
	start   sync.Once
	workers int
}

var taskEvents = newTaskEventState()
var taskRuntime = newTaskScheduler()

func newTaskEventState() *taskEventState {
	events := &taskEventState{}
	events.cond = sync.NewCond(&events.mu)
	return events
}

func newTaskScheduler() *taskScheduler {
	scheduler := &taskScheduler{workers: goruntime.GOMAXPROCS(0)}
	if scheduler.workers < 1 {
		scheduler.workers = 1
	}
	scheduler.cond = sync.NewCond(&scheduler.mu)
	return scheduler
}

func (events *taskEventState) notify() {
	events.mu.Lock()
	events.generation++
	events.cond.Broadcast()
	events.mu.Unlock()
}

func (events *taskEventState) snapshot() uint64 {
	events.mu.Lock()
	generation := events.generation
	events.mu.Unlock()
	return generation
}

func (events *taskEventState) waitChange(generation uint64) {
	events.mu.Lock()
	for events.generation == generation {
		events.cond.Wait()
	}
	events.mu.Unlock()
}

func (events *taskEventState) waitAny(tasks []*Task) int {
	for {
		generation := events.snapshot()
		for i, task := range tasks {
			if task.Done() {
				return i
			}
		}
		if taskRuntime.runOne() {
			continue
		}
		events.waitChange(generation)
	}
}

func (scheduler *taskScheduler) ensureStarted() {
	scheduler.start.Do(func() {
		for i := 0; i < scheduler.workers; i++ {
			go scheduler.worker()
		}
	})
}

func (scheduler *taskScheduler) submit(task *Task, run func()) {
	scheduler.ensureStarted()
	scheduler.mu.Lock()
	scheduler.queue = append(scheduler.queue, taskJob{task: task, run: run})
	scheduler.cond.Signal()
	scheduler.mu.Unlock()
	taskEvents.notify()
}

func (scheduler *taskScheduler) take(block bool) (taskJob, bool) {
	scheduler.mu.Lock()
	for scheduler.head >= len(scheduler.queue) {
		if !block {
			scheduler.mu.Unlock()
			return taskJob{}, false
		}
		scheduler.cond.Wait()
	}
	job := scheduler.queue[scheduler.head]
	scheduler.queue[scheduler.head] = taskJob{}
	scheduler.head++
	if scheduler.head >= 64 && scheduler.head*2 >= len(scheduler.queue) {
		scheduler.queue = append([]taskJob(nil), scheduler.queue[scheduler.head:]...)
		scheduler.head = 0
	}
	scheduler.mu.Unlock()
	return job, true
}

func (scheduler *taskScheduler) worker() {
	for {
		job, ok := scheduler.take(true)
		if ok {
			job.run()
		}
	}
}

func (scheduler *taskScheduler) runOne() bool {
	job, ok := scheduler.take(false)
	if !ok {
		return false
	}
	job.run()
	return true
}

func newTaskHandle() *Task {
	return &Task{state: "pending"}
}

func (t *Task) start() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done || t.state != "pending" {
		return false
	}
	t.state = "running"
	return true
}

func (t *Task) complete(value *Value, err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if !t.done {
		if value == nil {
			value = Undefined
		}
		t.result = value
		t.err = err
		t.done = true
		if err != nil {
			t.state = "failed"
		} else {
			t.state = "completed"
		}
	}
	t.mu.Unlock()
	taskEvents.notify()
}

func (t *Task) Await() (*Value, error) {
	if t == nil {
		return Undefined, fmt.Errorf("cannot await an invalid task")
	}
	for {
		generation := taskEvents.snapshot()
		t.mu.Lock()
		if t.done {
			value, err := t.result, t.err
			t.mu.Unlock()
			if err != nil {
				return nil, err
			}
			if value == nil {
				return Undefined, nil
			}
			return value, nil
		}
		t.mu.Unlock()
		if taskRuntime.runOne() {
			continue
		}
		taskEvents.waitChange(generation)
	}
}

func (t *Task) Done() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	done := t.done
	t.mu.Unlock()
	return done
}

func (t *Task) State() string {
	if t == nil {
		return "invalid"
	}
	t.mu.Lock()
	state := t.state
	t.mu.Unlock()
	return state
}

func (t *Task) Cancel() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	if t.done || t.state != "pending" {
		t.mu.Unlock()
		return false
	}
	t.done = true
	t.state = "cancelled"
	t.result = Undefined
	t.err = fmt.Errorf("task cancelled")
	t.mu.Unlock()
	taskEvents.notify()
	return true
}

func (interp *Interpreter) createTask(args []*Value) (*Value, error) {
	if len(args) == 0 || args[0] == nil || args[0].Tag != TypeFunction || args[0].FnVal == nil {
		return nil, interp.runtimeError("TypeError", "E0003", "new.task expects a function as its first argument", nil, nil)
	}
	fn := args[0].FnVal
	fnArgs := append([]*Value(nil), args[1:]...)
	if fn.Native == nil {
		if err := interp.checkFunctionArity(fn, fnArgs); err != nil {
			return nil, err
		}
		MarkEscaped(fn.Env)
	}
	program := interp.compileTaskFunction(fn, fnArgs)
	task := newTaskHandle()
	taskRuntime.submit(task, func() {
		if !task.start() {
			return
		}
		defer func() {
			if recovered := recover(); recovered != nil {
				task.complete(Undefined, fmt.Errorf("task panicked: %v", recovered))
			}
		}()
		if program != nil {
			if value, err, ok := runNativeTask(interp, program, fn, fnArgs); ok {
				task.complete(value, err)
				return
			}
		}
		worker := interp.newTaskWorker(fn)
		value, err := worker.callFunctionValue(FuncVal(fn), fnArgs, Undefined)
		task.complete(value, err)
	})
	return TaskV(task), nil
}

func (interp *Interpreter) newTaskWorker(fn *Function) *Interpreter {
	profiler := NewProfiler()
	if interp.profiler != nil && interp.diagnosticTracing {
		profiler = interp.profiler
	}
	worker := &Interpreter{
		globals:           NewEnvironment(nil),
		profiler:          profiler,
		callStack:         append(interp.callStack[:0:0], interp.callStack...),
		diagnosticTracing: interp.diagnosticTracing,
		diagnosticJIT:     interp.diagnosticJIT,
		modules:           make(map[string]*Value),
		maxExecSteps:      interp.maxExecSteps,
		jitLoops:          make(map[*ast.Node]*jitLoopState),
		jitFunctions:      make(map[*Function]*jitFunctionState),
		watchers:          make([]*watcher, 0),
		moduleLoader:      interp.moduleLoader,
		ntlLoader:         interp.ntlLoader,
		astLoader:         interp.astLoader,
		ntlFileLoader:     interp.ntlFileLoader,
		naxLoader:         interp.naxLoader,
	}
	if fn != nil {
		worker.filename = fn.SourceFile
		worker.sourceLines = fn.SourceLines
	}
	worker.registerBuiltins()
	return worker
}

func (interp *Interpreter) promiseAll(args []*Value) (*Value, error) {
	if len(args) == 0 || args[0] == nil || args[0].Tag != TypeArray {
		return nil, interp.runtimeError("TypeError", "E0003", "Promise.all expects an array", nil, nil)
	}
	input := args[0].ArrVal
	results := make([]*Value, len(input))
	type pendingTask struct {
		index int
		task  *Task
	}
	pending := make([]pendingTask, 0, len(input))
	for i, value := range input {
		if value == nil {
			results[i] = Undefined
			continue
		}
		if value.Tag == TypeTask {
			if value.TaskVal == nil {
				return nil, interp.runtimeError("TypeError", "E0003", "Promise.all received an invalid task", nil, nil)
			}
			pending = append(pending, pendingTask{index: i, task: value.TaskVal})
			continue
		}
		results[i] = value
	}
	for len(pending) > 0 {
		candidates := make([]*Task, len(pending))
		for i, item := range pending {
			candidates[i] = item.task
		}
		selected := taskEvents.waitAny(candidates)
		item := pending[selected]
		value, err := item.task.Await()
		if err != nil {
			return nil, err
		}
		results[item.index] = value
		pending = append(pending[:selected], pending[selected+1:]...)
	}
	return ArrayVal(results), nil
}

func (interp *Interpreter) taskGroup(args []*Value, waitAny bool) (*Value, error) {
	if len(args) != 1 || args[0] == nil || args[0].Tag != TypeArray {
		name := "tasks.all"
		if waitAny {
			name = "tasks.any"
		}
		return nil, interp.runtimeError("TypeError", "E0003", name+" expects an array of tasks", nil, nil)
	}
	tasks := make([]*Task, len(args[0].ArrVal))
	for i, value := range args[0].ArrVal {
		if value == nil || value.Tag != TypeTask || value.TaskVal == nil {
			return nil, interp.runtimeError("TypeError", "E0003", "task groups accept only task values", nil, nil)
		}
		tasks[i] = value.TaskVal
	}
	if waitAny {
		if len(tasks) == 0 {
			return ObjectVal(map[string]*Value{"index": NumberVal(-1), "value": Undefined}), nil
		}
		index := taskEvents.waitAny(tasks)
		value, err := tasks[index].Await()
		if err != nil {
			return nil, err
		}
		return ObjectVal(map[string]*Value{"index": NumberVal(float64(index)), "value": value}), nil
	}
	type pendingTask struct {
		index int
		task  *Task
	}
	results := make([]*Value, len(tasks))
	pending := make([]pendingTask, len(tasks))
	for i, task := range tasks {
		pending[i] = pendingTask{index: i, task: task}
	}
	for len(pending) > 0 {
		candidates := make([]*Task, len(pending))
		for i, item := range pending {
			candidates[i] = item.task
		}
		selected := taskEvents.waitAny(candidates)
		item := pending[selected]
		value, err := item.task.Await()
		if err != nil {
			return nil, err
		}
		results[item.index] = value
		pending = append(pending[:selected], pending[selected+1:]...)
	}
	return ArrayVal(results), nil
}

func (interp *Interpreter) compileTaskFunction(fn *Function, args []*Value) *jit.Program {
	loadNativeConfig()
	if interp == nil || fn == nil || fn.Native != nil || !nativeEnabled || !jit.Available() || len(args) != len(fn.Params) {
		return nil
	}
	for _, arg := range args {
		if arg == nil || arg.Tag != TypeNumber {
			return nil
		}
	}
	body, ok := fn.Body.(*ast.Node)
	if !ok || body == nil {
		return nil
	}
	paramSlots := make([]int, len(fn.Params))
	for i, param := range fn.Params {
		if param.Rest || fnParamHasDefault(param.Default) || param.Destructure != nil || param.ResolvedSlot < 0 {
			return nil
		}
		paramSlots[i] = param.ResolvedSlot
	}
	interp.jitMu.Lock()
	defer interp.jitMu.Unlock()
	if interp.jitFunctions == nil {
		interp.jitFunctions = make(map[*Function]*jitFunctionState)
	}
	state := interp.jitFunctions[fn]
	if state == nil {
		state = &jitFunctionState{}
		interp.jitFunctions[fn] = state
	}
	if state.current != nil {
		if taskProgramUsesExternalSlots(state.current) {
			state.taskNever = true
			return nil
		}
		return state.current
	}
	if state.never || state.taskNever {
		return nil
	}
	program, err := jit.CompileFunction(body, fn.Name, paramSlots, 2)
	if err == nil && taskProgramUsesExternalSlots(program) {
		_ = program.Close()
		program = nil
		err = fmt.Errorf("function captures values outside its native task context")
	}
	if err != nil {
		program, err = jit.CompileFunction(body, fn.Name, paramSlots, 1)
		if err == nil && taskProgramUsesExternalSlots(program) {
			_ = program.Close()
			program = nil
			err = fmt.Errorf("function captures values outside its native task context")
		}
	}
	if err != nil {
		state.taskNever = true
		if nativeTrace {
			fmt.Fprintf(os.Stderr, "lunex native: task function %s rejected: %v\n", fn.Name, err)
		}
		return nil
	}
	state.current = program
	state.tier1 = program
	state.hits = nativeHotLimit
	if program.Tier >= 2 {
		state.tier2Tried = true
	}
	if nativeTrace {
		fmt.Fprintf(os.Stderr, "lunex native: task function %s compiled, tier=%d\n", fn.Name, program.Tier)
	}
	return program
}

func taskProgramUsesExternalSlots(program *jit.Program) bool {
	if program == nil {
		return true
	}
	for _, ref := range program.Refs {
		if ref.Hops >= 0 {
			return true
		}
	}
	return false
}

func runNativeTask(interp *Interpreter, program *jit.Program, fn *Function, args []*Value) (*Value, error, bool) {
	if interp == nil || (interp.diagnosticTracing && !interp.diagnosticJIT) || program == nil || fn == nil || len(args) != len(fn.Params) || len(args) != len(program.ParamSlots) {
		return nil, nil, false
	}
	body, ok := fn.Body.(*ast.Node)
	if !ok || body == nil {
		return nil, nil, false
	}
	if program.Recursive {
		runner := &Interpreter{
			filename:     fn.SourceFile,
			sourceLines:  fn.SourceLines,
			maxExecSteps: interp.maxExecSteps,
			jitLoops:     make(map[*ast.Node]*jitLoopState),
			jitFunctions: map[*Function]*jitFunctionState{fn: {current: program, tier1: program, hits: nativeHotLimit, tier2Tried: program.Tier >= 2}},
		}
		value, handled, err := runner.runFunctionJIT(fn, body, args)
		return value, err, handled
	}
	ctx := program.NewContext()
	ctx.Budget = interp.maxExecSteps
	ctx.Status = jit.StatusOK
	ctx.ErrorNode = -1
	ctx.FrameDepth = 0
	ctx.FrameLimit = 0
	for i, arg := range args {
		if arg == nil || arg.Tag != TypeNumber {
			return nil, nil, false
		}
		slot := program.ParamSlots[i]
		if slot < 0 || slot >= len(ctx.Slots) {
			return nil, nil, false
		}
		ctx.Slots[slot] = arg.NumVal
	}
	if err := program.Run(ctx); err != nil || ctx.Status != jit.StatusOK {
		return nil, nil, false
	}
	return NumberVal(ctx.Result), nil, true
}

func (interp *Interpreter) parallelMap(args []*Value) (*Value, error) {
	if len(args) < 2 || args[0] == nil || args[0].Tag != TypeFunction || args[0].FnVal == nil || args[1] == nil || args[1].Tag != TypeArray {
		return nil, interp.runtimeError("TypeError", "E0003", "parallel.map expects a function and an array", nil, nil)
	}
	fn := args[0].FnVal
	items := args[1].ArrVal
	if len(items) == 0 {
		return ArrayVal([]*Value{}), nil
	}
	if fn.Native == nil {
		if err := interp.checkFunctionArity(fn, []*Value{items[0]}); err != nil {
			return nil, err
		}
		MarkEscaped(fn.Env)
	}
	allNumbers := fn.Native == nil && len(fn.Params) == 1
	for _, item := range items {
		if item == nil || item.Tag != TypeNumber {
			allNumbers = false
			break
		}
	}
	var program *jit.Program
	if allNumbers && (!interp.diagnosticTracing || interp.diagnosticJIT) {
		program = interp.compileTaskFunction(fn, []*Value{items[0]})
	}
	workerCount := goruntime.GOMAXPROCS(0)
	if workerCount > len(items) {
		workerCount = len(items)
	}
	if workerCount < 1 {
		workerCount = 1
	}
	chunkSize := (len(items) + workerCount - 1) / workerCount
	results := make([]*Value, len(items))
	chunkTasks := make([]*Task, 0, workerCount)
	for start := 0; start < len(items); start += chunkSize {
		end := start + chunkSize
		if end > len(items) {
			end = len(items)
		}
		chunkStart, chunkEnd := start, end
		chunkTask := newTaskHandle()
		chunkTasks = append(chunkTasks, chunkTask)
		taskRuntime.submit(chunkTask, func() {
			if !chunkTask.start() {
				return
			}
			defer func() {
				if recovered := recover(); recovered != nil {
					chunkTask.complete(Undefined, fmt.Errorf("parallel.map worker panicked: %v", recovered))
				}
			}()
			var ctx *jit.Context
			var body *ast.Node
			if fn.Body != nil {
				body, _ = fn.Body.(*ast.Node)
			}
			if program != nil && !program.Recursive && len(program.ParamSlots) == 1 {
				ctx = program.NewContext()
				if slot := program.ParamSlots[0]; slot < 0 || slot >= len(ctx.Slots) {
					ctx = nil
				}
			}
			var worker *Interpreter
			var chunkErr error
			for i := chunkStart; i < chunkEnd; i++ {
				var value *Value
				var err error
				usedNative := false
				if program != nil && program.Recursive && body != nil {
					if worker == nil {
						worker = interp.newTaskWorker(fn)
						worker.jitMu.Lock()
						worker.jitFunctions[fn] = &jitFunctionState{current: program, tier1: program, hits: nativeHotLimit, tier2Tried: program.Tier >= 2}
						worker.jitMu.Unlock()
					}
					value, usedNative, err = worker.runFunctionJIT(fn, body, []*Value{items[i]})
					if usedNative && err != nil {
						if chunkErr == nil {
							chunkErr = err
						}
						continue
					}
				}
				if program != nil && ctx != nil && !usedNative {
					for j := range ctx.Slots {
						ctx.Slots[j] = 0
					}
					ctx.Status = jit.StatusOK
					ctx.ErrorNode = -1
					ctx.Result = 0
					ctx.Used = 0
					ctx.Budget = interp.maxExecSteps
					ctx.FrameDepth = 0
					ctx.FrameLimit = 0
					ctx.RepeatLimit = 0
					ctx.Counters = [jit.MaxCounter]int64{}
					slot := program.ParamSlots[0]
					ctx.Slots[slot] = items[i].NumVal
					if runErr := program.Run(ctx); runErr == nil && ctx.Status == jit.StatusOK {
						value = NumberVal(ctx.Result)
						usedNative = true
					}
				}
				if !usedNative {
					if worker == nil {
						worker = interp.newTaskWorker(fn)
					}
					value, err = worker.callFunctionValue(FuncVal(fn), []*Value{items[i]}, Undefined)
				}
				if err != nil {
					if chunkErr == nil {
						chunkErr = err
					}
				} else {
					results[i] = value
				}
			}
			chunkTask.complete(Undefined, chunkErr)
		})
	}
	var firstErr error
	for _, task := range chunkTasks {
		if _, err := task.Await(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return ArrayVal(results), nil
}
