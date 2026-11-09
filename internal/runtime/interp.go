package runtime

import (
	"lunex/internal/ast"
	"lunex/internal/errfmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type continueSignal struct{}
type returnSignal struct{ value *Value }
type throwSignal struct{ value *Value }

type NTLLoader func(name string) (string, bool)
type NTLASTLoader func(name string) (*ast.Node, string, bool)

type NTLFileLoader func(name string) (src, realPath string, ok bool)

type NaxLoader func(path string) (*Value, error)

type deferEntry struct {
	node *ast.Node
	env  *Environment
}

type Interpreter struct {
	globals           *Environment
	topEnv            *Environment
	nativeMathObject  *Value
	nativeMathMembers map[string]*Value
	profiler          *Profiler
	modules           map[string]*Value
	mu                sync.RWMutex
	filename          string
	sourceLines       []string
	currentLine       int
	currentCol        int
	defers            []deferEntry
	ntlLoader         NTLLoader
	astLoader         NTLASTLoader
	ntlFileLoader     NTLFileLoader
	naxLoader         NaxLoader
	moduleLoader      func(string) (*Value, bool)
	libLoadDepth      int32
	callDepth         int
	diagnosticTracing bool
	diagnosticJIT     bool
	callStack         []errfmt.StackFrame
	lastPanicStack    []errfmt.StackFrame
	templateCache     sync.Map
	numCache          sync.Map
	scopeCache        sync.Map
	memberCache       sync.Map
	execSteps         int64
	maxExecSteps      int64
	jitMu             sync.Mutex
	feedbackMu        sync.RWMutex
	typeFeedback      map[*ast.Node]*NodeTypeFeedback
	feedbackTicker    atomic.Uint64
	jitLoops          map[*ast.Node]*jitLoopState
	jitFunctions      map[*Function]*jitFunctionState
	jitFunctionFrames sync.Pool
	watchMu           sync.Mutex
	watchers          []*watcher
}

const defaultMaxExecSteps = 200_000_000

func resolveMaxExecSteps() int64 {
	raw := strings.TrimSpace(os.Getenv("LUNEX_MAX_STEPS"))
	if raw == "" {
		return defaultMaxExecSteps
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return defaultMaxExecSteps
	}
	return n
}

func (interp *Interpreter) captureNativeMath() {
	value, ok := interp.globals.Get("Math")
	if !ok || value == nil || value.Tag != TypeObject || value.ObjVal == nil {
		return
	}
	interp.nativeMathObject = value
	interp.nativeMathMembers = make(map[string]*Value, len(value.ObjVal))
	for name, member := range value.ObjVal {
		interp.nativeMathMembers[name] = member
	}
}

func (interp *Interpreter) nativeMathUnmodified() bool {
	if interp == nil || interp.nativeMathObject == nil || interp.nativeMathMembers == nil {
		return false
	}
	value, ok := interp.globals.Get("Math")
	if !ok || value != interp.nativeMathObject || value == nil || value.Tag != TypeObject {
		return false
	}
	if len(value.ObjVal) != len(interp.nativeMathMembers) {
		return false
	}
	for name, original := range interp.nativeMathMembers {
		if value.ObjVal[name] != original {
			return false
		}
	}
	return true
}

func NewInterpreter() *Interpreter {
	interp := &Interpreter{
		globals:      NewEnvironment(nil),
		profiler:     NewProfiler(),
		modules:      make(map[string]*Value),
		maxExecSteps: resolveMaxExecSteps(),
		typeFeedback: make(map[*ast.Node]*NodeTypeFeedback),
		jitLoops:     make(map[*ast.Node]*jitLoopState),
		jitFunctions: make(map[*Function]*jitFunctionState),
		watchers:     make([]*watcher, 0),
	}
	interp.registerBuiltins()
	interp.captureNativeMath()
	CallFunction = func(fn *Value, args []*Value, this ...*Value) (*Value, error) {
		var t *Value
		if len(this) > 0 {
			t = this[0]
		}
		return interp.callFunctionValue(fn, normalizeCallbackArgs(fn, args), t)
	}
	return interp
}

func (interp *Interpreter) SetFilename(f string)          { interp.filename = f }
func (interp *Interpreter) SetSourceLines(lines []string) { interp.sourceLines = lines }
func (interp *Interpreter) SetDiagnosticTracing(enabled bool) {
	interp.diagnosticTracing = enabled
	interp.callStack = nil
	interp.lastPanicStack = nil
	if !enabled {
		interp.diagnosticJIT = false
	}
}

func (interp *Interpreter) DiagnosticStack() []errfmt.StackFrame {
	if interp == nil {
		return nil
	}
	frames := interp.callStack
	if len(interp.lastPanicStack) > 0 {
		frames = interp.lastPanicStack
	}
	return append([]errfmt.StackFrame(nil), frames...)
}

func (interp *Interpreter) SetDiagnosticJIT(enabled bool) {
	interp.diagnosticJIT = enabled && interp.diagnosticTracing
}
func (interp *Interpreter) getSourceLines() []string { return interp.sourceLines }

func (interp *Interpreter) updateNodeLocation(node *ast.Node) {
	if node == nil || node.Line <= 0 {
		return
	}
	interp.currentLine = node.Line
	interp.currentCol = node.Col
	if interp.diagnosticTracing && len(interp.callStack) > 0 {
		frame := &interp.callStack[len(interp.callStack)-1]
		frame.Line = node.Line
		frame.Col = node.Col
		if interp.filename != "" {
			frame.File = interp.filename
		}
	}
}

func (interp *Interpreter) pushCallFrame(fn *Function, body *ast.Node) {
	if !interp.diagnosticTracing {
		return
	}
	name := "<anonymous>"
	file := interp.filename
	if fn != nil {
		if fn.Name != "" {
			name = fn.Name
		}
		if fn.SourceFile != "" {
			file = fn.SourceFile
		}
	}
	frame := errfmt.StackFrame{FnName: name, File: file}
	if body != nil {
		frame.Line = body.Line
		frame.Col = body.Col
		if frame.Line == 0 && len(body.Body_) > 0 && body.Body_[0] != nil {
			frame.Line = body.Body_[0].Line
			frame.Col = body.Body_[0].Col
		}
	}
	interp.callStack = append(interp.callStack, frame)
}

func (interp *Interpreter) popCallFrame() {
	if !interp.diagnosticTracing || len(interp.callStack) == 0 {
		return
	}
	interp.callStack = interp.callStack[:len(interp.callStack)-1]
}

func (interp *Interpreter) runtimeError(kind errfmt.ErrorKind, code, msg string, node *ast.Node, similar []string) *errfmt.LunexError {
	line, col := 0, 0
	if node != nil {
		line = node.Line
		col = node.Col
		if line == 0 {
			line = interp.currentLine
			col = interp.currentCol
		}
	} else {
		line = interp.currentLine
		col = interp.currentCol
	}
	var stack []errfmt.StackFrame
	if interp.diagnosticTracing && len(interp.callStack) > 0 {
		stack = append([]errfmt.StackFrame(nil), interp.callStack...)
	}
	return &errfmt.LunexError{
		Message: msg,
		File:    interp.filename,
		Line:    line,
		Col:     col,
		Kind:    kind,
		Code:    code,
		Lines:   interp.sourceLines,
		Similar: similar,
		Stack:   stack,
	}
}

func visibleNames(env *Environment) []string {
	seen := make(map[string]bool)
	var names []string
	for e := env; e != nil; e = e.Parent() {
		for k := range e.Snapshot() {
			if !seen[k] && !strings.HasPrefix(k, "__") {
				seen[k] = true
				names = append(names, k)
			}
		}
	}
	return names
}

func objKeys(v *Value) []string {
	if v == nil || v.ObjVal == nil {
		return nil
	}
	keys := make([]string, 0, len(v.ObjVal))
	for k := range v.ObjVal {
		keys = append(keys, k)
	}
	return keys
}

func (interp *Interpreter) RegisterModule(name string, val *Value) {
	interp.mu.Lock()
	interp.modules[name] = val
	interp.mu.Unlock()
}

func (interp *Interpreter) RegisterNative(name string, val *Value) {
	interp.globals.Define("__native_"+name+"__", val, true)
}

func (interp *Interpreter) CallFunctionValue(fnVal *Value, args []*Value) (*Value, error) {
	return interp.callFunctionValue(fnVal, args, nil)
}

func (interp *Interpreter) GetModule(name string) (*Value, bool) {
	interp.mu.RLock()
	v, ok := interp.modules[name]
	interp.mu.RUnlock()
	return v, ok
}

func (interp *Interpreter) SetGlobal(name string, val *Value) {
	dot := strings.IndexByte(name, '.')
	if dot >= 0 {
		parent := name[:dot]
		child := name[dot+1:]
		if obj, ok := interp.globals.Get(parent); ok && obj != nil && obj.Tag == TypeObject {
			if obj.ObjVal == nil {
				obj.ObjVal = make(map[string]*Value)
			}
			obj.Set(child, val)
			return
		}
	}
	interp.globals.Define(name, val, false)
}

func (interp *Interpreter) GetGlobal(name string) (*Value, bool) {
	dot := strings.IndexByte(name, '.')
	if dot >= 0 {
		parent := name[:dot]
		child := name[dot+1:]
		if obj, ok := interp.globals.Get(parent); ok && obj != nil && obj.Tag == TypeObject {
			if v, exists := obj.ObjVal[child]; exists {
				return v, true
			}
			return Undefined, false
		}
		return Undefined, false
	}
	return interp.globals.Get(name)
}

func (interp *Interpreter) GetAllGlobalNames() []string {
	return interp.globals.AllNames()
}

func (interp *Interpreter) CallValue(fn *Value, args ...*Value) (*Value, error) {
	interp.resetExecutionBudget()
	return interp.callFunctionValue(fn, args, nil)
}

func (interp *Interpreter) SetASTLoader(loader NTLASTLoader) { interp.astLoader = loader }
func (interp *Interpreter) ASTLoader() NTLASTLoader          { return interp.astLoader }
func (interp *Interpreter) SetModuleLoader(loader func(string) (*Value, bool)) {
	interp.moduleLoader = loader
}

func (interp *Interpreter) SetNTLLoader(loader NTLLoader) {
	interp.ntlLoader = loader
}

func (interp *Interpreter) SetNTLFileLoader(loader NTLFileLoader) {
	interp.ntlFileLoader = loader
}

func (interp *Interpreter) NTLLoader() NTLLoader {
	return interp.ntlLoader
}

func (interp *Interpreter) SetNaxLoader(loader NaxLoader) {
	interp.naxLoader = loader
}

func (interp *Interpreter) SetPkgLoader(loader NTLLoader) {
	prev := interp.ntlLoader
	interp.ntlLoader = func(name string) (string, bool) {
		if prev != nil {
			if src, ok := prev(name); ok {
				return src, true
			}
		}
		return loader(name)
	}
}

func (interp *Interpreter) ProvideKeepAliveWait() {
	KeepAliveWait()
}

func (interp *Interpreter) provideKeepAliveWait() {
	interp.ProvideKeepAliveWait()
}
