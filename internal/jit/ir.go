package jit

import (
	"fmt"
	"lunex/internal/ast"
	"math"
	"strconv"
)

const (
	MaxSlots   = 256
	MaxStack   = 512
	MaxConsts  = 128
	MaxCounter = 32
)

const (
	StatusOK int64 = iota
	StatusBreak
	StatusTimeout
	StatusDivisionByZero
	StatusDeopt
)

type Context struct {
	Slots       [MaxSlots]float64
	Stack       [MaxStack]float64
	Consts      [MaxConsts]float64
	Budget      int64
	Used        int64
	Status      int64
	ErrorNode   int64
	RepeatLimit int64
	Counters    [MaxCounter]int64
}

type Reference struct {
	Hops int
	Slot int
}

type Write struct {
	Reference
	Name string
}

type Instruction struct {
	Op        OpCode
	A         int
	Target    int
	ErrorNode int
}

type OpCode uint8

const (
	OpPushConst OpCode = iota
	OpLoadSlot
	OpStoreSlot
	OpStoreSlotKeep
	OpDup
	OpSwap
	OpPop
	OpAdd
	OpSub
	OpMul
	OpDiv
	OpMod
	OpNeg
	OpCompare
	OpJump
	OpBranchFalse
	OpBranchTrue
	OpTick
	OpSetRepeatCounter
	OpRepeatCheck
	OpRepeatDecrement
	OpBreak
	OpReturn
)

type CompareOp uint8

const (
	CompareLT CompareOp = iota
	CompareLE
	CompareGT
	CompareGE
	CompareEQ
	CompareNE
)

type Program struct {
	Code            []byte
	FastCode        []byte
	Refs            []Reference
	Reads           []bool
	Writes          []Write
	Constants       []float64
	Nodes           []*ast.Node
	Bytecode        []Instruction
	Labels          map[int]int
	Tier            int
	ExpressionStack int
	CounterCount    int
	RootRepeat      bool
	NestedLoops     bool
	FastNote        string
	RangeRoot       bool
	RangeStart      int
	RangeStep       int
	RangeIndex      int
}

func (p *Program) NewContext() *Context {
	ctx := new(Context)
	for i, v := range p.Constants {
		ctx.Consts[i] = v
	}
	return ctx
}

func (p *Program) Run(ctx *Context) error {
	if ctx == nil {
		return fmt.Errorf("nil native context")
	}
	if len(p.Code) == 0 {
		return fmt.Errorf("native program has no machine code")
	}
	code := p.Code
	if len(p.FastCode) != 0 {
		code = p.FastCode
	}
	if !Available() {
		return fmt.Errorf("native execution is unavailable on this architecture")
	}
	return runNative(codeAddress(code), uintptrOfContext(ctx))
}

func CompileLoop(node *ast.Node, tier int) (*Program, error) {
	if tier < 1 || tier > 2 {
		return nil, fmt.Errorf("unsupported native tier %d", tier)
	}
	if node == nil {
		return nil, fmt.Errorf("nil loop")
	}
	c := compiler{
		program: &Program{Tier: tier, Labels: make(map[int]int)},
		tier:    tier,
		refs:    make(map[Reference]int),
		consts:  make(map[uint64]int),
		nodes:   make(map[*ast.Node]int),
		labels:  make(map[int]int),
		locals:  make(map[[2]int]int),
		constLocals: make(map[int]bool),
	}
	limits := []float64{0, math.Ldexp(1, 63), -math.Ldexp(1, 63), 1}
	for _, v := range limits {
		c.program.Constants = append(c.program.Constants, v)
		c.consts[math.Float64bits(v)] = len(c.program.Constants) - 1
	}
	if err := c.compileLoop(node, true); err != nil {
		return nil, err
	}
	c.emit(Instruction{Op: OpReturn})
	if c.stack != 0 {
		return nil, fmt.Errorf("native stack is not balanced at loop exit: %d", c.stack)
	}
	if c.maxStack > MaxStack {
		return nil, fmt.Errorf("loop expression stack depth %d exceeds native limit %d", c.maxStack, MaxStack)
	}
	if len(c.program.Refs) > MaxSlots {
		return nil, fmt.Errorf("loop references %d slots, native limit is %d", len(c.program.Refs), MaxSlots)
	}
	if len(c.layers) != 0 {
		return nil, fmt.Errorf("native scope stack is not balanced at loop exit: %d", len(c.layers))
	}
	if len(c.program.Constants) > MaxConsts {
		return nil, fmt.Errorf("loop uses %d numeric constants, native limit is %d", len(c.program.Constants), MaxConsts)
	}
	if c.program.CounterCount > MaxCounter {
		return nil, fmt.Errorf("loop nesting exceeds native counter limit %d", MaxCounter)
	}
	c.program.ExpressionStack = c.maxStack
	c.program.Bytecode = c.code
	c.program.Labels = c.labels
	c.program.Nodes = c.nodeList
	if tier >= 2 {
		coalesceTicks(c.program)
	}
	machine, err := emitMachine(c.program)
	if err != nil {
		return nil, err
	}
	c.program.Code = machine
	if fast, fastErr := emitMachineFast(c.program); fastErr == nil && len(fast) != 0 {
		c.program.FastCode = fast
	}
	if err := selfCheck(c.program); err != nil {
		_ = c.program.Close()
		return nil, err
	}
	return c.program, nil
}

type compiler struct {
	program   *Program
	tier      int
	refs      map[Reference]int
	consts    map[uint64]int
	nodes     map[*ast.Node]int
	nodeList  []*ast.Node
	code      []Instruction
	labels    map[int]int
	loops     []loopTarget
	nextLabel int
	stack     int
	maxStack  int
	layers    []int
	nextLayer int
	scratch   int
	locals    map[[2]int]int
	constLocals   map[int]bool
}

type loopTarget struct {
	breakLabel    int
	continueLabel int
	root          bool
}

func (c *compiler) emit(in Instruction) int {
	idx := len(c.code)
	c.code = append(c.code, in)
	return idx
}

func (c *compiler) label() int {
	id := c.nextLabel
	c.nextLabel++
	c.labels[id] = len(c.code)
	return id
}

func (c *compiler) newLabel() int {
	id := c.nextLabel
	c.nextLabel++
	c.labels[id] = -1
	return id
}

func (c *compiler) branch(op OpCode, target int) {
	c.emit(Instruction{Op: op, Target: target})
}

func (c *compiler) nodeID(n *ast.Node) int {
	if n == nil {
		return -1
	}
	if id, ok := c.nodes[n]; ok {
		return id
	}
	id := len(c.nodeList)
	c.nodes[n] = id
	c.nodeList = append(c.nodeList, n)
	return id
}

func (c *compiler) addRef(node *ast.Node) (int, error) {
	if node == nil || node.ResolvedAddr == nil {
		return -1, fmt.Errorf("identifier is not statically resolved")
	}
	hops := node.ResolvedAddr.Hops
	slot := node.ResolvedAddr.Slot
	if hops < 0 || slot < 0 {
		return -1, fmt.Errorf("identifier has an invalid address")
	}
	depth := len(c.layers)
	if hops < depth {
		return c.localRef(c.layers[depth-1-hops], slot), nil
	}
	r := Reference{Hops: hops - depth, Slot: slot}
	if idx, ok := c.refs[r]; ok {
		return idx, nil
	}
	idx := len(c.program.Refs)
	c.refs[r] = idx
	c.program.Refs = append(c.program.Refs, r)
	c.program.Reads = append(c.program.Reads, false)
	return idx, nil
}

func (c *compiler) newScratch() int {
	idx := len(c.program.Refs)
	c.program.Refs = append(c.program.Refs, Reference{Hops: -1, Slot: c.scratch})
	c.program.Reads = append(c.program.Reads, false)
	c.scratch++
	return idx
}

func (c *compiler) localRef(layer, slot int) int {
	key := [2]int{layer, slot}
	if idx, ok := c.locals[key]; ok {
		return idx
	}
	idx := c.newScratch()
	c.locals[key] = idx
	return idx
}

func (c *compiler) isScratch(idx int) bool {
	return idx >= 0 && idx < len(c.program.Refs) && c.program.Refs[idx].Hops < 0
}

func (c *compiler) markRead(idx int) {
	if !c.isScratch(idx) {
		c.program.Reads[idx] = true
	}
}

func (c *compiler) enterLayer() int {
	id := c.nextLayer
	c.nextLayer++
	c.layers = append(c.layers, id)
	return id
}

func (c *compiler) leaveLayer() {
	c.layers = c.layers[:len(c.layers)-1]
}

func (c *compiler) addWrite(node *ast.Node) (int, error) {
	idx, err := c.addRef(node)
	if err != nil {
		return -1, err
	}
	if c.isScratch(idx) {
		if c.constLocals[idx] {
			return -1, fmt.Errorf("assignment to constant %q", node.Name)
		}
		return idx, nil
	}
	c.program.Reads[idx] = true
	ref := c.program.Refs[idx]
	for _, w := range c.program.Writes {
		if w.Reference == ref && w.Name == node.Name {
			return idx, nil
		}
	}
	c.program.Writes = append(c.program.Writes, Write{Reference: ref, Name: node.Name})
	return idx, nil
}

func (c *compiler) constant(v float64) int {
	bits := math.Float64bits(v)
	if idx, ok := c.consts[bits]; ok {
		return idx
	}
	idx := len(c.program.Constants)
	c.consts[bits] = idx
	c.program.Constants = append(c.program.Constants, v)
	return idx
}

func (c *compiler) push(n int) {
	c.stack += n
	if c.stack > c.maxStack {
		c.maxStack = c.stack
	}
	if c.stack < 0 {
		panic("negative native expression stack")
	}
}

func (c *compiler) emitTick(node *ast.Node) {
	c.emit(Instruction{Op: OpTick, A: 1, ErrorNode: c.nodeID(node)})
}

func (c *compiler) compileLoop(node *ast.Node, root bool) error {
	if node == nil {
		return nil
	}
	if !root {
		c.program.NestedLoops = true
	}
	switch node.Type {
	case ast.WhileStmt:
		return c.compileWhile(node, root)
	case ast.ForStmt:
		return c.compileFor(node, root)
	case ast.RepeatStmt:
		return c.compileRepeat(node, root)
	case ast.LoopStmt:
		return c.compileInfinite(node, root)
	case ast.ForOfStmt, ast.EachInStmt:
		return c.compileForRange(node, root)
	default:
		return fmt.Errorf("unsupported native loop type %s", node.Type)
	}
}

func (c *compiler) compileWhile(node *ast.Node, root bool) error {
	if node.Test == nil || node.Body == nil {
		return fmt.Errorf("while loop requires test and body")
	}
	if !root {
		c.emitTick(node)
	}
	header := c.label()
	exit := c.newLabel()
	if err := c.compileExpr(node.Test); err != nil {
		return err
	}
	c.branch(OpBranchFalse, exit)
	c.push(-1)
	c.loops = append(c.loops, loopTarget{breakLabel: exit, continueLabel: header, root: root})
	if err := c.compileStmt(node.Body); err != nil {
		return err
	}
	c.loops = c.loops[:len(c.loops)-1]
	c.branch(OpJump, header)
	c.labels[exit] = len(c.code)
	return nil
}

func (c *compiler) compileFor(node *ast.Node, root bool) error {
	if node.Body == nil {
		return fmt.Errorf("for loop requires body")
	}
	if node.Init != nil {
		return fmt.Errorf("for initializer is outside native numeric subset")
	}
	if !root {
		c.emitTick(node)
	}
	header := c.label()
	exit := c.newLabel()
	continueLabel := c.newLabel()
	if node.Test != nil {
		if err := c.compileExpr(node.Test); err != nil {
			return err
		}
		c.branch(OpBranchFalse, exit)
		c.push(-1)
	}
	c.loops = append(c.loops, loopTarget{breakLabel: exit, continueLabel: continueLabel, root: root})
	if err := c.compileStmt(node.Body); err != nil {
		return err
	}
	c.loops = c.loops[:len(c.loops)-1]
	c.labels[continueLabel] = len(c.code)
	if node.Right != nil {
		if err := c.compileExpr(node.Right); err != nil {
			return err
		}
		c.emit(Instruction{Op: OpPop})
		c.push(-1)
	}
	c.branch(OpJump, header)
	c.labels[exit] = len(c.code)
	return nil
}

func (c *compiler) compileRepeat(node *ast.Node, root bool) error {
	if node.Body == nil {
		return fmt.Errorf("repeat loop requires body")
	}
	if !root {
		c.emitTick(node)
	}
	counter := c.program.CounterCount
	c.program.CounterCount++
	if root {
		c.program.RootRepeat = true
	} else if node.Count != nil {
		if err := c.compileExpr(node.Count); err != nil {
			return err
		}
		c.emit(Instruction{Op: OpSetRepeatCounter, A: counter})
		c.push(-1)
	} else {
		c.emit(Instruction{Op: OpPushConst, A: c.constant(-1)})
		c.push(1)
		c.emit(Instruction{Op: OpSetRepeatCounter, A: counter})
		c.push(-1)
	}
	header := c.label()
	exit := c.newLabel()
	continueLabel := c.newLabel()
	c.emit(Instruction{Op: OpRepeatCheck, A: counter, Target: exit})
	c.loops = append(c.loops, loopTarget{breakLabel: exit, continueLabel: continueLabel, root: root})
	if err := c.compileStmt(node.Body); err != nil {
		return err
	}
	c.loops = c.loops[:len(c.loops)-1]
	c.labels[continueLabel] = len(c.code)
	c.emit(Instruction{Op: OpRepeatDecrement, A: counter})
	c.branch(OpJump, header)
	c.labels[exit] = len(c.code)
	return nil
}

func (c *compiler) compileInfinite(node *ast.Node, root bool) error {
	if node.Body == nil {
		return fmt.Errorf("loop requires body")
	}
	if !root {
		c.emitTick(node)
	}
	header := c.label()
	exit := c.newLabel()
	c.loops = append(c.loops, loopTarget{breakLabel: exit, continueLabel: header, root: root})
	if err := c.compileStmt(node.Body); err != nil {
		return err
	}
	c.loops = c.loops[:len(c.loops)-1]
	c.branch(OpJump, header)
	c.labels[exit] = len(c.code)
	return nil
}

func (c *compiler) compileStmt(node *ast.Node) error {
	if node == nil {
		return nil
	}
	switch node.Type {
	case ast.WhileStmt, ast.ForStmt, ast.RepeatStmt, ast.LoopStmt, ast.ForOfStmt, ast.EachInStmt:
		return c.compileLoop(node, false)
	}
	c.emitTick(node)
	switch node.Type {
	case ast.Block:
		own := blockOwnsScope(node)
		if own {
			c.enterLayer()
		}
		for _, stmt := range node.Body_ {
			if err := c.compileStmt(stmt); err != nil {
				return err
			}
		}
		if own {
			c.leaveLayer()
		}
	case ast.VarDecl:
		if err := c.compileVarDecl(node); err != nil {
			return err
		}
	case ast.ExprStmt:
		if err := c.compileExpr(node.Expr); err != nil {
			return err
		}
		c.emit(Instruction{Op: OpPop})
		c.push(-1)
	case ast.BreakStmt:
		if len(c.loops) == 0 {
			return fmt.Errorf("break outside native subset loop")
		}
		target := c.loops[len(c.loops)-1]
		if target.root {
			c.emit(Instruction{Op: OpBreak, Target: target.breakLabel, ErrorNode: -1})
		} else {
			c.branch(OpJump, target.breakLabel)
		}
	case ast.ContinueStmt:
		if len(c.loops) == 0 {
			return fmt.Errorf("continue outside native subset loop")
		}
		c.branch(OpJump, c.loops[len(c.loops)-1].continueLabel)
	case ast.IfStmt, ast.UnlessStmt:
		return c.compileIf(node)
	default:
		return fmt.Errorf("node %s is outside native subset", node.Type)
	}
	return nil
}

func (c *compiler) compileIf(node *ast.Node) error {
	if node.Test == nil || node.Consequent == nil {
		return fmt.Errorf("conditional requires test and consequent")
	}
	if c.tier >= 2 {
		if v, ok := foldConstant(node.Test); ok {
			c.emitFoldedTicks(node.Test)
			truthy := v != 0 && !math.IsNaN(v)
			if node.Type == ast.UnlessStmt {
				truthy = !truthy
			}
			if truthy {
				return c.compileStmt(node.Consequent)
			}
			if node.Alternate != nil {
				return c.compileStmt(node.Alternate)
			}
			return nil
		}
	}
	if err := c.compileExpr(node.Test); err != nil {
		return err
	}
	other := c.newLabel()
	end := c.newLabel()
	branch := OpBranchFalse
	if node.Type == ast.UnlessStmt {
		branch = OpBranchTrue
	}
	c.branch(branch, other)
	c.push(-1)
	if err := c.compileStmt(node.Consequent); err != nil {
		return err
	}
	c.branch(OpJump, end)
	c.labels[other] = len(c.code)
	if node.Alternate != nil {
		if err := c.compileStmt(node.Alternate); err != nil {
			return err
		}
	}
	c.labels[end] = len(c.code)
	return nil
}

func (c *compiler) compileExpr(node *ast.Node) error {
	if node == nil {
		return fmt.Errorf("nil expression")
	}
	if c.tier >= 2 {
		if v, ok := foldConstant(node); ok {
			c.emitFoldedTicks(node)
			idx := c.constant(v)
			c.emit(Instruction{Op: OpPushConst, A: idx})
			c.push(1)
			return nil
		}
	}
	c.emitTick(node)
	switch node.Type {
	case ast.NumberLit:
		v, err := parseNumber(node)
		if err != nil {
			return err
		}
		idx := c.constant(v)
		c.emit(Instruction{Op: OpPushConst, A: idx})
		c.push(1)
	case ast.Identifier:
		idx, err := c.addRef(node)
		if err != nil {
			return err
		}
		c.markRead(idx)
		c.emit(Instruction{Op: OpLoadSlot, A: idx})
		c.push(1)
	case ast.BinaryExpr:
		if err := c.compileExpr(node.Left); err != nil {
			return err
		}
		if err := c.compileExpr(node.Right); err != nil {
			return err
		}
		op, ok := compareOp(node.Op)
		if ok {
			c.emit(Instruction{Op: OpCompare, A: int(op), ErrorNode: c.nodeID(node)})
		} else {
			switch node.Op {
			case "+":
				c.emit(Instruction{Op: OpAdd})
			case "-":
				c.emit(Instruction{Op: OpSub})
			case "*":
				c.emit(Instruction{Op: OpMul})
			case "/":
				c.emit(Instruction{Op: OpDiv, ErrorNode: c.nodeID(node)})
			case "%":
				c.emit(Instruction{Op: OpMod, ErrorNode: c.nodeID(node)})
			default:
				return fmt.Errorf("operator %q is outside native subset", node.Op)
			}
		}
		c.push(-1)
	case ast.UnaryExpr:
		switch node.Op {
		case "-":
			if err := c.compileExpr(node.Arg); err != nil {
				return err
			}
			c.emit(Instruction{Op: OpNeg})
		case "++", "--":
			if node.Arg == nil || node.Arg.Type != ast.Identifier || node.Arg.ResolvedAddr == nil {
				return fmt.Errorf("increment target must be a resolved identifier")
			}
			idx, err := c.addWrite(node.Arg)
			if err != nil {
				return err
			}
			if err := c.compileExpr(node.Arg); err != nil {
				return err
			}
			if !node.Prefix {
				c.emit(Instruction{Op: OpDup})
				c.push(1)
			}
			c.emit(Instruction{Op: OpPushConst, A: c.constant(1)})
			c.push(1)
			if node.Op == "++" {
				c.emit(Instruction{Op: OpAdd})
			} else {
				c.emit(Instruction{Op: OpSub})
			}
			c.push(-1)
			if node.Prefix {
				c.emit(Instruction{Op: OpDup})
				c.push(1)
				c.emit(Instruction{Op: OpStoreSlot, A: idx})
				c.push(-1)
			} else {
				c.emit(Instruction{Op: OpStoreSlotKeep, A: idx})
				c.emit(Instruction{Op: OpPop})
				c.push(-1)
			}
		default:
			return fmt.Errorf("unary operator %q is outside native subset", node.Op)
		}
	case ast.AssignExpr:
		if node.Left == nil || node.Left.Type != ast.Identifier || node.Left.ResolvedAddr == nil {
			return fmt.Errorf("assignment target must be a resolved identifier")
		}
		if isComparison(node.Right) {
			return fmt.Errorf("storing a comparison result is outside native subset")
		}
		idx, err := c.addWrite(node.Left)
		if err != nil {
			return err
		}
		if err := c.compileExpr(node.Right); err != nil {
			return err
		}
		if node.Op != "=" {
			if err := c.compileExpr(node.Left); err != nil {
				return err
			}
			c.emit(Instruction{Op: OpSwap})
			switch node.Op {
			case "+=":
				c.emit(Instruction{Op: OpAdd})
			case "-=":
				c.emit(Instruction{Op: OpSub})
			case "*=":
				c.emit(Instruction{Op: OpMul})
			case "/=":
				c.emit(Instruction{Op: OpDiv, ErrorNode: c.nodeID(node)})
			default:
				return fmt.Errorf("assignment operator %q is outside native subset", node.Op)
			}
			c.push(-1)
		}
		c.emit(Instruction{Op: OpDup})
		c.push(1)
		c.emit(Instruction{Op: OpStoreSlot, A: idx})
		c.push(-1)
	default:
		return fmt.Errorf("expression node %s is outside native subset", node.Type)
	}
	return nil
}

func (c *compiler) emitFoldedTicks(node *ast.Node) {
	if node == nil {
		return
	}
	c.emitTick(node)
	switch node.Type {
	case ast.UnaryExpr:
		c.emitFoldedTicks(node.Arg)
	case ast.BinaryExpr:
		c.emitFoldedTicks(node.Left)
		c.emitFoldedTicks(node.Right)
	}
}

func compareOp(op string) (CompareOp, bool) {
	switch op {
	case "<":
		return CompareLT, true
	case "<=":
		return CompareLE, true
	case ">":
		return CompareGT, true
	case ">=":
		return CompareGE, true
	case "==":
		return CompareEQ, true
	case "!=":
		return CompareNE, true
	default:
		return 0, false
	}
}

func parseNumber(node *ast.Node) (float64, error) {
	s, ok := node.Value.(string)
	if !ok {
		return 0, fmt.Errorf("number literal has invalid representation")
	}
	orig := s
	if len(s) > 1 && s[len(s)-1] == '_' {
		s = s[:len(s)-1]
	}
	var f float64
	var err error
	switch {
	case len(s) > 2 && (s[:2] == "0x" || s[:2] == "0X"):
		var v int64
		v, err = strconv.ParseInt(s[2:], 16, 64)
		f = float64(v)
	case len(s) > 2 && (s[:2] == "0o" || s[:2] == "0O"):
		var v int64
		v, err = strconv.ParseInt(s[2:], 8, 64)
		f = float64(v)
	case len(s) > 2 && (s[:2] == "0b" || s[:2] == "0B"):
		var v int64
		v, err = strconv.ParseInt(s[2:], 2, 64)
		f = float64(v)
	default:
		f, err = strconv.ParseFloat(s, 64)
	}
	if err != nil {
		if orig == "NaN" {
			return math.NaN(), nil
		}
		return 0, err
	}
	return f, nil
}

func foldConstant(node *ast.Node) (float64, bool) {
	if node == nil {
		return 0, false
	}
	switch node.Type {
	case ast.NumberLit:
		v, err := parseNumber(node)
		return v, err == nil
	case ast.UnaryExpr:
		if node.Op != "-" {
			return 0, false
		}
		v, ok := foldConstant(node.Arg)
		if !ok {
			return 0, false
		}
		return -v, true
	case ast.BinaryExpr:
		lv, lok := foldConstant(node.Left)
		rv, rok := foldConstant(node.Right)
		if !lok || !rok {
			return 0, false
		}
		switch node.Op {
		case "+":
			return lv + rv, true
		case "-":
			return lv - rv, true
		case "*":
			return lv * rv, true
		case "/":
			if rv == 0 {
				return 0, false
			}
			return lv / rv, true
		case "%":
			return 0, false
		case "<":
			return boolFloat(lv < rv), true
		case "<=":
			return boolFloat(lv <= rv), true
		case ">":
			return boolFloat(lv > rv), true
		case ">=":
			return boolFloat(lv >= rv), true
		case "==":
			return boolFloat(lv == rv), true
		case "!=":
			return boolFloat(lv != rv), true
		}
	}
	return 0, false
}

func boolFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func (p *Program) Close() error {
	if p == nil {
		return nil
	}
	var first error
	if len(p.Code) != 0 {
		if err := releaseExecutable(p.Code); err != nil {
			first = err
		}
		p.Code = nil
	}
	if len(p.FastCode) != 0 {
		if err := releaseExecutable(p.FastCode); err != nil && first == nil {
			first = err
		}
		p.FastCode = nil
	}
	return first
}

func blockOwnsScope(node *ast.Node) bool {
	for _, stmt := range node.Body_ {
		if stmt == nil {
			continue
		}
		switch stmt.Type {
		case ast.VarDecl, ast.ImmutableDecl, ast.UsingDecl,
			ast.FnDecl, ast.ClassDecl, ast.EnumDecl, ast.NamespaceDecl,
			ast.ComponentDecl:
			return true
		}
	}
	return false
}

func isComparison(node *ast.Node) bool {
	if node == nil || node.Type != ast.BinaryExpr {
		return false
	}
	_, ok := compareOp(node.Op)
	return ok
}

func scopeSlot(info *ast.ScopeInfo, name string) int {
	if info == nil || name == "" {
		return -1
	}
	for i, n := range info.Names {
		if n == name {
			return i
		}
	}
	return -1
}

func (c *compiler) pushConst(v float64) {
	c.emit(Instruction{Op: OpPushConst, A: c.constant(v)})
	c.push(1)
}

func (c *compiler) loadSlot(idx int) {
	c.emit(Instruction{Op: OpLoadSlot, A: idx})
	c.push(1)
}

func (c *compiler) storeSlot(idx int) {
	c.emit(Instruction{Op: OpStoreSlot, A: idx})
	c.push(-1)
}

func (c *compiler) arith(op OpCode, node *ast.Node) {
	in := Instruction{Op: op}
	if op == OpDiv || op == OpMod {
		in.ErrorNode = c.nodeID(node)
	}
	c.emit(in)
	c.push(-1)
}

func (c *compiler) compare(op CompareOp, node *ast.Node) {
	c.emit(Instruction{Op: OpCompare, A: int(op), ErrorNode: c.nodeID(node)})
	c.push(-1)
}

func (c *compiler) compileVarDecl(node *ast.Node) error {
	if node.Destructure != nil || node.Init == nil || node.ResolvedAddr == nil || node.Name == "" {
		return fmt.Errorf("declaration is outside native subset")
	}
	if len(c.layers) == 0 || node.ResolvedAddr.Hops != 0 || node.ResolvedAddr.Slot < 0 {
		return fmt.Errorf("declaration has no native scope")
	}
	if isComparison(node.Init) {
		return fmt.Errorf("storing a comparison result is outside native subset")
	}
	idx := c.localRef(c.layers[len(c.layers)-1], node.ResolvedAddr.Slot)
	if err := c.compileExpr(node.Init); err != nil {
		return err
	}
	c.storeSlot(idx)
	if node.IsConst {
		c.constLocals[idx] = true
	} else {
		delete(c.constLocals, idx)
	}
	return nil
}

func (c *compiler) compileForRange(node *ast.Node, root bool) error {
	if node.Body == nil || node.Right == nil || node.Right.Type != ast.RangeExpr {
		return fmt.Errorf("iteration is outside native subset: only range() is supported")
	}
	if node.Destructure != nil {
		return fmt.Errorf("destructuring loop variable is outside native subset")
	}
	args := node.Right.Args
	if len(args) < 1 || len(args) > 3 {
		return fmt.Errorf("range() with %d arguments is outside native subset", len(args))
	}
	valueSlot := scopeSlot(node.ScopeInfo, node.Name)
	if valueSlot < 0 {
		return fmt.Errorf("loop variable has no scope slot")
	}
	aliasSlot := -1
	if node.Alias != "" {
		aliasSlot = scopeSlot(node.ScopeInfo, node.Alias)
		if aliasSlot < 0 {
			return fmt.Errorf("loop index has no scope slot")
		}
	}
	if !root {
		c.emitTick(node)
	}
	startRef := c.newScratch()
	stepRef := c.newScratch()
	indexRef := c.newScratch()
	counter := c.program.CounterCount
	c.program.CounterCount++
	if root {
		c.program.RootRepeat = true
		c.program.RangeRoot = true
		c.program.RangeStart = startRef
		c.program.RangeStep = stepRef
		c.program.RangeIndex = indexRef
	} else {
		if err := c.compileRangeSetup(node, args, startRef, stepRef, counter); err != nil {
			return err
		}
		c.pushConst(0)
		c.storeSlot(indexRef)
	}
	layer := c.enterLayer()
	valueRef := c.localRef(layer, valueSlot)
	aliasRef := -1
	if aliasSlot >= 0 {
		aliasRef = c.localRef(layer, aliasSlot)
	}
	if node.IsConst {
		c.constLocals[valueRef] = true
		if aliasRef >= 0 {
			c.constLocals[aliasRef] = true
		}
	}
	header := c.label()
	exit := c.newLabel()
	continueLabel := c.newLabel()
	c.emit(Instruction{Op: OpRepeatCheck, A: counter, Target: exit})
	c.emitTick(node)
	c.loadSlot(indexRef)
	c.loadSlot(stepRef)
	c.arith(OpMul, node)
	c.loadSlot(startRef)
	c.arith(OpAdd, node)
	c.storeSlot(valueRef)
	if aliasRef >= 0 {
		c.loadSlot(indexRef)
		c.storeSlot(aliasRef)
	}
	c.loops = append(c.loops, loopTarget{breakLabel: exit, continueLabel: continueLabel, root: root})
	if err := c.compileStmt(node.Body); err != nil {
		return err
	}
	c.loops = c.loops[:len(c.loops)-1]
	c.labels[continueLabel] = len(c.code)
	c.loadSlot(indexRef)
	c.pushConst(1)
	c.arith(OpAdd, node)
	c.storeSlot(indexRef)
	c.emit(Instruction{Op: OpRepeatDecrement, A: counter})
	c.branch(OpJump, header)
	c.labels[exit] = len(c.code)
	c.leaveLayer()
	return nil
}

func (c *compiler) compileRangeSetup(node *ast.Node, args []*ast.Node, startRef, stepRef, counter int) error {
	if len(args) == 1 {
		c.pushConst(0)
		c.storeSlot(startRef)
		c.pushConst(1)
		c.storeSlot(stepRef)
		limit := c.newScratch()
		if err := c.compileExpr(args[0]); err != nil {
			return err
		}
		c.storeSlot(limit)
		c.emitRangeCount(node, limit, false, counter, -1)
		return nil
	}
	endRef := c.newScratch()
	if err := c.compileExpr(args[0]); err != nil {
		return err
	}
	c.storeSlot(startRef)
	if err := c.compileExpr(args[1]); err != nil {
		return err
	}
	c.storeSlot(endRef)
	if len(args) == 3 {
		if err := c.compileExpr(args[2]); err != nil {
			return err
		}
	} else {
		c.pushConst(1)
	}
	c.storeSlot(stepRef)
	noStep := c.newLabel()
	c.loadSlot(stepRef)
	c.pushConst(0)
	c.compare(CompareEQ, node)
	c.branch(OpBranchTrue, noStep)
	c.push(-1)
	span := c.newScratch()
	c.loadSlot(endRef)
	c.loadSlot(startRef)
	c.arith(OpSub, node)
	c.loadSlot(stepRef)
	c.arith(OpDiv, node)
	c.storeSlot(span)
	c.emitRangeCount(node, span, true, counter, noStep)
	return nil
}

func (c *compiler) emitRangeCount(node *ast.Node, value int, ceil bool, counter int, extraZero int) {
	zero := c.newLabel()
	done := c.newLabel()
	frac := c.newScratch()
	c.loadSlot(value)
	c.pushConst(0)
	c.compare(CompareGT, node)
	c.branch(OpBranchFalse, zero)
	c.push(-1)
	c.loadSlot(value)
	c.pushConst(1)
	c.arith(OpMod, node)
	c.storeSlot(frac)
	bump := -1
	if ceil {
		bump = c.newScratch()
		c.loadSlot(frac)
		c.pushConst(0)
		c.compare(CompareGT, node)
		c.storeSlot(bump)
	}
	c.loadSlot(value)
	c.loadSlot(frac)
	c.arith(OpSub, node)
	if ceil {
		c.loadSlot(bump)
		c.arith(OpAdd, node)
	}
	c.emit(Instruction{Op: OpSetRepeatCounter, A: counter})
	c.push(-1)
	c.branch(OpJump, done)
	c.labels[zero] = len(c.code)
	if extraZero >= 0 {
		c.labels[extraZero] = len(c.code)
	}
	c.pushConst(0)
	c.emit(Instruction{Op: OpSetRepeatCounter, A: counter})
	c.push(-1)
	c.labels[done] = len(c.code)
}
