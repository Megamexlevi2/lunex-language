package bytecode

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"lunex/internal/ast"
	"math"
)

var ErrNTZUnsupported = fmt.Errorf("lunex: feature not supported by NTZ compiler")

const (
	ntzConst       = 0
	ntzLoad        = 1
	ntzStore       = 2
	ntzAdd         = 3
	ntzSub         = 4
	ntzMul         = 5
	ntzDiv         = 6
	ntzMod         = 7
	ntzNeg         = 8
	ntzNot         = 9
	ntzBitAnd      = 10
	ntzBitOr       = 11
	ntzBitXor      = 12
	ntzBitNot      = 13
	ntzShl         = 14
	ntzShr         = 15
	ntzEq          = 16
	ntzNeq         = 17
	ntzLt          = 18
	ntzLte         = 19
	ntzGt          = 20
	ntzGte         = 21
	ntzAnd         = 22
	ntzOr          = 23
	ntzCall        = 24
	ntzCallRT      = 25
	ntzReturn      = 26
	ntzJump        = 27
	ntzJumpIf      = 28
	ntzJumpIfFalse = 29
	ntzGetField    = 30
	ntzSetField    = 31
	ntzGetIndex    = 32
	ntzSetIndex    = 33
	ntzMakeArray   = 34
	ntzMakeObject  = 35
	ntzSpread      = 36
	ntzIter        = 37
	ntzIterNext    = 38
	ntzThrow       = 39
	ntzTry         = 40
	ntzCatch       = 41
	ntzHalt        = 42
	ntzPop         = 43
	ntzFuncDef     = 44
)

const (
	ntzConstNull   = 0
	ntzConstBool   = 1
	ntzConstInt    = 2
	ntzConstFloat  = 3
	ntzConstString = 4
)

type patchSite struct{ offset int }

type loopCtx struct {
	continueTarget int
	breakSites     []int
}

type ntzC struct {
	buf      bytes.Buffer
	vars     map[string]uint16
	nextSlot uint16
	loops    []loopCtx
}

func CompileNTZ(tree *ast.Node) ([]byte, error) {
	if tree == nil {
		return []byte{ntzHalt}, nil
	}
	c := &ntzC{vars: make(map[string]uint16)}
	if err := c.stmt(tree); err != nil {
		return nil, err
	}

	if slot, hasMain := c.vars["main"]; hasMain {
		c.emitU8(ntzLoad)
		c.emitU16(slot)
		c.emitU8(ntzCall)
		c.emitU8(0)
		c.emitU8(ntzPop)
	}

	c.emit(ntzHalt)
	return c.buf.Bytes(), nil
}

func (c *ntzC) emit(b ...byte) { c.buf.Write(b) }

func (c *ntzC) emitU8(v uint8) { c.buf.WriteByte(v) }

func (c *ntzC) emitU16(v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	c.buf.Write(b[:])
}

func (c *ntzC) emitU32(v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	c.buf.Write(b[:])
}

func (c *ntzC) emitI64(v int64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(v))
	c.buf.Write(b[:])
}

func (c *ntzC) emitF64(v float64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(v))
	c.buf.Write(b[:])
}

func (c *ntzC) pos() int { return c.buf.Len() }

func (c *ntzC) emitJump(op byte) patchSite {
	c.emitU8(op)
	pt := patchSite{offset: c.buf.Len()}
	c.emitU32(0)
	return pt
}

func (c *ntzC) patch(pt patchSite, target int) {
	binary.LittleEndian.PutUint32(c.buf.Bytes()[pt.offset:], uint32(target))
}

func (c *ntzC) patchAt(offset int, target int) {
	binary.LittleEndian.PutUint32(c.buf.Bytes()[offset:], uint32(target))
}

func (c *ntzC) varSlot(name string) uint16 {
	if slot, ok := c.vars[name]; ok {
		return slot
	}
	slot := c.nextSlot
	c.vars[name] = slot
	c.nextSlot++
	return slot
}

func (c *ntzC) emitName(name string) {
	c.emitU8(uint8(len(name)))
	c.buf.WriteString(name)
}

func (c *ntzC) stmt(n *ast.Node) error {
	if n == nil {
		return nil
	}
	switch n.Type {
	case ast.Program:
		for _, s := range n.Body_ {
			if err := c.stmt(s); err != nil {
				return err
			}
		}

	case ast.Block:
		for _, s := range n.Body_ {
			if err := c.stmt(s); err != nil {
				return err
			}
		}

	case ast.VarDecl, ast.ImmutableDecl, ast.UsingDecl:
		if n.Init != nil {
			if err := c.expr(n.Init); err != nil {
				return err
			}
		} else {
			c.emitU8(ntzConst)
			c.emitU8(ntzConstNull)
		}
		slot := c.varSlot(n.Name)
		c.emitU8(ntzStore)
		c.emitU16(slot)
		c.emitU8(ntzPop)

	case ast.ExprStmt:
		if n.Expr != nil {
			if err := c.expr(n.Expr); err != nil {
				return err
			}
			c.emitU8(ntzPop)
		}

	case ast.LogStmt:
		for _, arg := range n.Args {
			if err := c.expr(arg); err != nil {
				return err
			}
		}
		c.emitU8(ntzCallRT)
		c.emitName("log")
		c.emitU8(uint8(len(n.Args)))
		c.emitU8(ntzPop)

	case ast.ReturnStmt:
		if n.Expr != nil {
			if err := c.expr(n.Expr); err != nil {
				return err
			}
		} else {
			c.emitU8(ntzConst)
			c.emitU8(ntzConstNull)
		}
		c.emitU8(ntzReturn)

	case ast.ThrowStmt, ast.RaiseStmt:
		if n.Expr != nil {
			if err := c.expr(n.Expr); err != nil {
				return err
			}
		} else {
			c.emitU8(ntzConst)
			c.emitU8(ntzConstNull)
		}
		c.emitU8(ntzThrow)

	case ast.IfStmt, ast.UnlessStmt:
		if err := c.compileIf(n); err != nil {
			return err
		}

	case ast.WhileStmt:
		if err := c.compileWhile(n); err != nil {
			return err
		}

	case ast.ForStmt:
		if err := c.compileFor(n); err != nil {
			return err
		}

	case ast.BreakStmt:
		if len(c.loops) == 0 {
			return fmt.Errorf("lunex/ntz: break outside loop")
		}
		jmp := c.emitJump(ntzJump)
		c.loops[len(c.loops)-1].breakSites = append(
			c.loops[len(c.loops)-1].breakSites, jmp.offset)

	case ast.ContinueStmt:
		if len(c.loops) == 0 {
			return fmt.Errorf("lunex/ntz: continue outside loop")
		}
		target := c.loops[len(c.loops)-1].continueTarget
		c.emitU8(ntzJump)
		c.emitU32(uint32(target))

	case ast.AssertStmt:

		if n.Expr != nil {
			if err := c.expr(n.Expr); err != nil {
				return err
			}
		} else {
			c.emitU8(ntzConst)
			c.emitU8(ntzConstBool)
			c.emitU8(1)
		}
		jmp := c.emitJump(ntzJumpIf)
		c.emitU8(ntzConst)
		c.emitU8(ntzConstString)
		msg := "assertion failed"
		c.emitU32(uint32(len(msg)))
		c.buf.WriteString(msg)
		c.emitU8(ntzThrow)
		c.patch(jmp, c.pos())

	case ast.FnDecl:
		if err := c.compileFn(n); err != nil {
			return err
		}
		if n.Name != "" {
			slot := c.varSlot(n.Name)
			c.emitU8(ntzStore)
			c.emitU16(slot)
			c.emitU8(ntzPop)
		} else {
			c.emitU8(ntzPop)
		}

	case ast.ForOfStmt, ast.EachInStmt:
		return c.compileForOf(n)

	case ast.ClassDecl, ast.EnumDecl, ast.NamespaceDecl,
		ast.ComponentDecl, ast.ImportDecl, ast.ExportDecl, ast.LunexRequire,
		ast.UseStmt, ast.TryStmt, ast.SpawnStmt, ast.SelectStmt,
		ast.WithStmt, ast.HaveStmt, ast.IfHaveStmt, ast.IfSetStmt,
		ast.MatchStmt, ast.GuardStmt, ast.DeferStmt, ast.DeleteStmt,
		ast.RepeatStmt, ast.LoopStmt:
		return ErrNTZUnsupported

	default:

		if err := c.expr(n); err != nil {
			return err
		}
		c.emitU8(ntzPop)
	}
	return nil
}

func (c *ntzC) compileIf(n *ast.Node) error {
	cond := n.Test
	if cond == nil {
		return ErrNTZUnsupported
	}
	if err := c.expr(cond); err != nil {
		return err
	}

	if n.Type == ast.UnlessStmt {
		c.emitU8(ntzNot)
	}
	jmpFalse := c.emitJump(ntzJumpIfFalse)
	if err := c.stmt(n.Consequent); err != nil {
		return err
	}
	if n.Alternate != nil {
		jmpEnd := c.emitJump(ntzJump)
		c.patch(jmpFalse, c.pos())
		if err := c.stmt(n.Alternate); err != nil {
			return err
		}
		c.patch(jmpEnd, c.pos())
	} else {
		c.patch(jmpFalse, c.pos())
	}
	return nil
}

func (c *ntzC) compileWhile(n *ast.Node) error {
	loopStart := c.pos()
	if err := c.expr(n.Test); err != nil {
		return err
	}
	jmpEnd := c.emitJump(ntzJumpIfFalse)
	c.loops = append(c.loops, loopCtx{continueTarget: loopStart})
	if err := c.stmt(n.Body); err != nil {
		return err
	}
	c.emitU8(ntzJump)
	c.emitU32(uint32(loopStart))
	c.patch(jmpEnd, c.pos())
	c.patchBreaks()
	return nil
}

func (c *ntzC) compileFor(n *ast.Node) error {

	if n.Init != nil {
		if err := c.stmt(n.Init); err != nil {
			return err
		}
	}
	loopStart := c.pos()
	var jmpEnd patchSite
	hasTest := n.Test != nil
	if hasTest {
		if err := c.expr(n.Test); err != nil {
			return err
		}
		jmpEnd = c.emitJump(ntzJumpIfFalse)
	}
	c.loops = append(c.loops, loopCtx{continueTarget: loopStart})
	if err := c.stmt(n.Body); err != nil {
		return err
	}

	if n.Right != nil {
		if err := c.expr(n.Right); err != nil {
			return err
		}
		c.emitU8(ntzPop)
	}
	c.emitU8(ntzJump)
	c.emitU32(uint32(loopStart))
	if hasTest {
		c.patch(jmpEnd, c.pos())
	}
	c.patchBreaks()
	return nil
}

func (c *ntzC) patchBreaks() {
	if len(c.loops) == 0 {
		return
	}
	end := c.pos()
	for _, site := range c.loops[len(c.loops)-1].breakSites {
		c.patchAt(site, end)
	}
	c.loops = c.loops[:len(c.loops)-1]
}

func (c *ntzC) expr(n *ast.Node) error {
	if n == nil {
		c.emitU8(ntzConst)
		c.emitU8(ntzConstNull)
		return nil
	}
	switch n.Type {
	case ast.NumberLit:
		return c.emitNumber(n.Value)

	case ast.StringLit:
		s := ""
		if n.Value != nil {
			s = fmt.Sprintf("%v", n.Value)
		}
		c.emitU8(ntzConst)
		c.emitU8(ntzConstString)
		c.emitU32(uint32(len(s)))
		c.buf.WriteString(s)

	case ast.BoolLit:
		c.emitU8(ntzConst)
		c.emitU8(ntzConstBool)
		if n.Value == true {
			c.emitU8(1)
		} else {
			c.emitU8(0)
		}

	case ast.NullLit, ast.UndefinedLit:
		c.emitU8(ntzConst)
		c.emitU8(ntzConstNull)

	case ast.Identifier:
		name := n.Name
		if slot, ok := c.vars[name]; ok {
			c.emitU8(ntzLoad)
			c.emitU16(slot)
		} else {

			slot = c.varSlot(name)
			c.emitU8(ntzLoad)
			c.emitU16(slot)
		}

	case ast.BinaryExpr:
		return c.compileBinary(n)

	case ast.UnaryExpr, ast.NotExpr:
		return c.compileUnary(n)

	case ast.LogicalExpr:
		return c.compileLogical(n)

	case ast.AssignExpr:
		return c.compileAssign(n)

	case ast.CallExpr:
		return c.compileCall(n)

	case ast.MemberExpr:
		return c.compileMember(n)

	case ast.ArrayLit:
		for _, el := range n.Elements {
			if err := c.expr(el); err != nil {
				return err
			}
		}
		c.emitU8(ntzMakeArray)
		c.emitU16(uint16(len(n.Elements)))

	case ast.ObjectLit:
		return c.compileObject(n)

	case ast.TernaryExpr:
		return c.compileTernary(n)

	case ast.SpreadExpr:
		if err := c.expr(n.Expr); err != nil {
			return err
		}
		c.emitU8(ntzSpread)

	case ast.VoidExpr:
		if n.Expr != nil {
			if err := c.expr(n.Expr); err != nil {
				return err
			}
			c.emitU8(ntzPop)
		}
		c.emitU8(ntzConst)
		c.emitU8(ntzConstNull)

	case ast.TypeofExpr:

		if err := c.expr(n.Expr); err != nil {
			return err
		}
		c.emitU8(ntzCallRT)
		c.emitName("typeof")
		c.emitU8(1)

	case ast.FnExpr, ast.ArrowFn:
		return c.compileFn(n)

	case ast.AtImportExpr:
		return c.compileAtImport(n)

	case ast.NewExpr, ast.TemplateLit,
		ast.PipelineExpr, ast.SequenceExpr, ast.HaveExpr,
		ast.TrySafeExpr, ast.RangeExpr, ast.SleepExpr,
		ast.ChannelExpr, ast.NaxImportExpr, ast.ThisExpr,
		ast.SuperExpr, ast.SatisfiesExpr, ast.DecoratedExpr,
		ast.RegexLit, ast.DeleteExpr:
		return ErrNTZUnsupported

	default:
		return ErrNTZUnsupported
	}
	return nil
}

func (c *ntzC) emitNumber(v interface{}) error {
	switch val := v.(type) {
	case int64:
		c.emitU8(ntzConst)
		c.emitU8(ntzConstInt)
		c.emitI64(val)
	case int:
		c.emitU8(ntzConst)
		c.emitU8(ntzConstInt)
		c.emitI64(int64(val))
	case float64:

		if val == math.Trunc(val) && val >= -9e15 && val <= 9e15 {
			c.emitU8(ntzConst)
			c.emitU8(ntzConstInt)
			c.emitI64(int64(val))
		} else {
			c.emitU8(ntzConst)
			c.emitU8(ntzConstFloat)
			c.emitF64(val)
		}
	default:
		c.emitU8(ntzConst)
		c.emitU8(ntzConstFloat)
		c.emitF64(0)
	}
	return nil
}

func (c *ntzC) compileBinary(n *ast.Node) error {
	if err := c.expr(n.Left); err != nil {
		return err
	}
	if err := c.expr(n.Right); err != nil {
		return err
	}
	switch n.Op {
	case "+":
		c.emitU8(ntzAdd)
	case "-":
		c.emitU8(ntzSub)
	case "*":
		c.emitU8(ntzMul)
	case "/":
		c.emitU8(ntzDiv)
	case "%":
		c.emitU8(ntzMod)
	case "==", "===":
		c.emitU8(ntzEq)
	case "!=", "!==":
		c.emitU8(ntzNeq)
	case "<":
		c.emitU8(ntzLt)
	case "<=":
		c.emitU8(ntzLte)
	case ">":
		c.emitU8(ntzGt)
	case ">=":
		c.emitU8(ntzGte)
	case "&":
		c.emitU8(ntzBitAnd)
	case "|":
		c.emitU8(ntzBitOr)
	case "^":
		c.emitU8(ntzBitXor)
	case "<<":
		c.emitU8(ntzShl)
	case ">>":
		c.emitU8(ntzShr)
	default:
		return fmt.Errorf("lunex/ntz: unsupported binary op %q", n.Op)
	}
	return nil
}

func (c *ntzC) compileUnary(n *ast.Node) error {
	arg := n.Arg
	if arg == nil {
		arg = n.Expr
	}
	if err := c.expr(arg); err != nil {
		return err
	}
	switch n.Op {
	case "-":
		c.emitU8(ntzNeg)
	case "!", "not":
		c.emitU8(ntzNot)
	case "~":
		c.emitU8(ntzBitNot)
	case "+":

	default:
		return fmt.Errorf("lunex/ntz: unsupported unary op %q", n.Op)
	}
	return nil
}

func (c *ntzC) compileLogical(n *ast.Node) error {
	switch n.Op {
	case "&&":

		if err := c.expr(n.Left); err != nil {
			return err
		}

		temp := c.nextSlot
		c.nextSlot++
		c.emitU8(ntzStore)
		c.emitU16(temp)
		c.emitU8(ntzLoad)
		c.emitU16(temp)
		jmpFalse := c.emitJump(ntzJumpIfFalse)
		c.emitU8(ntzPop)
		if err := c.expr(n.Right); err != nil {
			return err
		}
		jmpEnd := c.emitJump(ntzJump)
		c.patch(jmpFalse, c.pos())

		c.patch(jmpEnd, c.pos())

	case "||":

		if err := c.expr(n.Left); err != nil {
			return err
		}
		temp := c.nextSlot
		c.nextSlot++
		c.emitU8(ntzStore)
		c.emitU16(temp)
		c.emitU8(ntzLoad)
		c.emitU16(temp)
		jmpTrue := c.emitJump(ntzJumpIf)
		c.emitU8(ntzPop)
		if err := c.expr(n.Right); err != nil {
			return err
		}
		jmpEnd := c.emitJump(ntzJump)
		c.patch(jmpTrue, c.pos())
		c.patch(jmpEnd, c.pos())

	default:
		return fmt.Errorf("lunex/ntz: unsupported logical op %q", n.Op)
	}
	return nil
}

func (c *ntzC) compileAssign(n *ast.Node) error {
	target := n.Left
	op := n.Op

	switch target.Type {
	case ast.Identifier:
		slot := c.varSlot(target.Name)
		if op == "=" {
			if err := c.expr(n.Right); err != nil {
				return err
			}
		} else {

			c.emitU8(ntzLoad)
			c.emitU16(slot)
			if err := c.expr(n.Right); err != nil {
				return err
			}
			if err := c.emitCompoundOp(op); err != nil {
				return err
			}
		}
		c.emitU8(ntzStore)
		c.emitU16(slot)

	case ast.MemberExpr:
		if target.Computed {

			if op != "=" {
				return ErrNTZUnsupported
			}
			if err := c.expr(target.Object); err != nil {
				return err
			}
			if err := c.compileIndex(target.Prop); err != nil {
				return err
			}
			if err := c.expr(n.Right); err != nil {
				return err
			}
			c.emitU8(ntzSetIndex)

		} else {

			if op != "=" {
				return ErrNTZUnsupported
			}
			if err := c.expr(target.Object); err != nil {
				return err
			}
			if err := c.expr(n.Right); err != nil {
				return err
			}
			name := c.fieldName(target)
			c.emitU8(ntzSetField)
			c.emitU8(uint8(len(name)))
			c.buf.WriteString(name)

		}

		c.emitU8(ntzConst)
		c.emitU8(ntzConstNull)

	default:
		return ErrNTZUnsupported
	}
	return nil
}

func (c *ntzC) emitCompoundOp(op string) error {
	switch op {
	case "+=":
		c.emitU8(ntzAdd)
	case "-=":
		c.emitU8(ntzSub)
	case "*=":
		c.emitU8(ntzMul)
	case "/=":
		c.emitU8(ntzDiv)
	case "%=":
		c.emitU8(ntzMod)
	case "&=":
		c.emitU8(ntzBitAnd)
	case "|=":
		c.emitU8(ntzBitOr)
	case "^=":
		c.emitU8(ntzBitXor)
	case "<<=":
		c.emitU8(ntzShl)
	case ">>=":
		c.emitU8(ntzShr)
	default:
		return fmt.Errorf("lunex/ntz: unsupported compound assignment %q", op)
	}
	return nil
}

func (c *ntzC) compileCall(n *ast.Node) error {
	if n.Callee == nil {
		return ErrNTZUnsupported
	}

	if n.Callee.Type == ast.Identifier {
		name := n.Callee.Name
		if _, isLocal := c.vars[name]; isLocal {

			slot := c.vars[name]
			c.emitU8(ntzLoad)
			c.emitU16(slot)
			for _, arg := range n.Args {
				if err := c.expr(arg); err != nil {
					return err
				}
			}
			c.emitU8(ntzCall)
			c.emitU8(uint8(len(n.Args)))
			return nil
		}

		for _, arg := range n.Args {
			if err := c.expr(arg); err != nil {
				return err
			}
		}
		c.emitU8(ntzCallRT)
		c.emitU8(uint8(len(name)))
		c.buf.WriteString(name)
		c.emitU8(uint8(len(n.Args)))
		return nil
	}

	if n.Callee.Type == ast.MemberExpr {
		callee := n.Callee
		method := c.fieldName(callee)
		if method == "" {
			return ErrNTZUnsupported
		}

		if err := c.expr(callee.Object); err != nil {
			return err
		}
		for _, arg := range n.Args {
			if err := c.expr(arg); err != nil {
				return err
			}
		}
		c.emitU8(ntzCallRT)
		c.emitU8(uint8(len(method)))
		c.buf.WriteString(method)
		c.emitU8(uint8(1 + len(n.Args)))
		return nil
	}

	return ErrNTZUnsupported
}

func (c *ntzC) compileMember(n *ast.Node) error {
	if err := c.expr(n.Object); err != nil {
		return err
	}
	if n.Computed {
		if err := c.compileIndex(n.Prop); err != nil {
			return err
		}
		c.emitU8(ntzGetIndex)
		return nil
	}
	name := c.fieldName(n)
	c.emitU8(ntzGetField)
	c.emitU8(uint8(len(name)))
	c.buf.WriteString(name)
	return nil
}

func (c *ntzC) compileIndex(prop interface{}) error {
	switch p := prop.(type) {
	case *ast.Node:
		return c.expr(p)
	default:
		c.emitU8(ntzConst)
		c.emitU8(ntzConstNull)
		return nil
	}
}

func (c *ntzC) compileObject(n *ast.Node) error {
	for _, prop := range n.Properties {

		key := ""
		switch k := prop.Key.(type) {
		case string:
			key = k
		case *ast.Node:
			if k != nil {
				switch k.Type {
				case ast.Identifier:
					key = k.Name
				case ast.StringLit:
					if s, ok := k.Value.(string); ok {
						key = s
					}
				}
			}
		}
		c.emitU8(ntzConst)
		c.emitU8(ntzConstString)
		c.emitU32(uint32(len(key)))
		c.buf.WriteString(key)

		if prop.Value != nil {
			if err := c.expr(prop.Value); err != nil {
				return err
			}
		} else {
			c.emitU8(ntzConst)
			c.emitU8(ntzConstNull)
		}
	}
	c.emitU8(ntzMakeObject)
	c.emitU16(uint16(len(n.Properties)))
	return nil
}

func (c *ntzC) compileTernary(n *ast.Node) error {
	if err := c.expr(n.Test); err != nil {
		return err
	}
	jmpFalse := c.emitJump(ntzJumpIfFalse)
	if err := c.expr(n.Consequent); err != nil {
		return err
	}
	jmpEnd := c.emitJump(ntzJump)
	c.patch(jmpFalse, c.pos())
	if err := c.expr(n.Alternate); err != nil {
		return err
	}
	c.patch(jmpEnd, c.pos())
	return nil
}

func (c *ntzC) fieldName(n *ast.Node) string {
	if n.Prop == nil {
		return ""
	}
	switch p := n.Prop.(type) {
	case string:
		return p
	case *ast.Node:
		if p == nil {
			return ""
		}
		if p.Type == ast.Identifier {
			return p.Name
		}
		if p.Type == ast.StringLit {
			if s, ok := p.Value.(string); ok {
				return s
			}
		}
	}
	return ""
}

func (c *ntzC) compileFn(n *ast.Node) error {
	name := n.Name

	child := &ntzC{vars: make(map[string]uint16)}

	for _, p := range n.Params {
		if p != nil && p.Name != "" {
			child.varSlot(p.Name)
		}
	}

	body := n.Body
	if body == nil && len(n.Body_) > 0 {
		for _, s := range n.Body_ {
			if err := child.stmt(s); err != nil {
				return err
			}
		}
	} else if body != nil {
		if err := child.stmt(body); err != nil {
			return err
		}
	}

	child.emitU8(ntzConst)
	child.emitU8(ntzConstNull)
	child.emitU8(ntzReturn)

	bodyBytes := child.buf.Bytes()

	if len(name) > 255 {
		name = name[:255]
	}
	c.emitU8(ntzFuncDef)
	c.emitU8(uint8(len(n.Params)))
	c.emitU8(uint8(len(name)))
	c.buf.WriteString(name)
	c.emitU32(uint32(len(bodyBytes)))
	c.buf.Write(bodyBytes)
	return nil
}

func (c *ntzC) compileForOf(n *ast.Node) error {

	varName := n.Binding
	if varName == "" {
		varName = n.BindingName
	}
	if varName == "" && n.Left != nil && n.Left.Type == ast.Identifier {
		varName = n.Left.Name
	}
	if varName == "" {
		return ErrNTZUnsupported
	}

	iterExpr := n.Right
	if iterExpr == nil {
		iterExpr = n.Init
	}
	if iterExpr == nil {
		return ErrNTZUnsupported
	}

	if err := c.expr(iterExpr); err != nil {
		return err
	}
	iterSlot := c.nextSlot
	c.nextSlot++
	c.emitU8(ntzStore)
	c.emitU16(iterSlot)
	c.emitU8(ntzPop)

	c.emitU8(ntzConst)
	c.emitU8(ntzConstInt)
	c.emitI64(0)
	idxSlot := c.nextSlot
	c.nextSlot++
	c.emitU8(ntzStore)
	c.emitU16(idxSlot)
	c.emitU8(ntzPop)

	elemSlot := c.varSlot(varName)

	loopStart := c.pos()
	c.emitU8(ntzLoad)
	c.emitU16(idxSlot)
	c.emitU8(ntzLoad)
	c.emitU16(iterSlot)
	c.emitU8(ntzGetField)
	c.emitU8(6)
	c.buf.WriteString("length")
	c.emitU8(ntzGte)
	jmpEnd := c.emitJump(ntzJumpIf)

	c.emitU8(ntzLoad)
	c.emitU16(iterSlot)
	c.emitU8(ntzLoad)
	c.emitU16(idxSlot)
	c.emitU8(ntzGetIndex)
	c.emitU8(ntzStore)
	c.emitU16(elemSlot)
	c.emitU8(ntzPop)

	c.loops = append(c.loops, loopCtx{continueTarget: loopStart})

	if n.Body != nil {
		if err := c.stmt(n.Body); err != nil {
			return err
		}
	}

	c.emitU8(ntzLoad)
	c.emitU16(idxSlot)
	c.emitU8(ntzConst)
	c.emitU8(ntzConstInt)
	c.emitI64(1)
	c.emitU8(ntzAdd)
	c.emitU8(ntzStore)
	c.emitU16(idxSlot)
	c.emitU8(ntzPop)

	c.emitU8(ntzJump)
	c.emitU32(uint32(loopStart))

	c.patch(jmpEnd, c.pos())
	c.patchBreaks()
	return nil
}

var stdlibModules = map[string][]struct {
	name   string
	params []string
}{
	"std.io": {
		{name: "log", params: []string{"__args"}},
		{name: "warn", params: []string{"__args"}},
		{name: "err", params: []string{"__args"}},
		{name: "read", params: []string{}},
		{name: "readLine", params: []string{}},
		{name: "write", params: []string{"__v"}},
	},
	"std.utils": {
		{name: "now", params: []string{}},
		{name: "sleep", params: []string{"ms"}},
		{name: "uuid", params: []string{}},
		{name: "hash", params: []string{"s"}},
		{name: "stringify", params: []string{"v"}},
		{name: "parse", params: []string{"s"}},
		{name: "keys", params: []string{"obj"}},
		{name: "values", params: []string{"obj"}},
		{name: "entries", params: []string{"obj"}},
		{name: "deepCopy", params: []string{"v"}},
		{name: "deepEqual", params: []string{"a", "b"}},
	},
	"std.math": {
		{name: "abs", params: []string{"x"}},
		{name: "ceil", params: []string{"x"}},
		{name: "floor", params: []string{"x"}},
		{name: "sqrt", params: []string{"x"}},
		{name: "pow", params: []string{"x", "y"}},
		{name: "min", params: []string{"a", "b"}},
		{name: "max", params: []string{"a", "b"}},
		{name: "round", params: []string{"x"}},
		{name: "log", params: []string{"x"}},
		{name: "log2", params: []string{"x"}},
		{name: "sin", params: []string{"x"}},
		{name: "cos", params: []string{"x"}},
		{name: "tan", params: []string{"x"}},
		{name: "PI", params: nil},
		{name: "E", params: nil},
	},
	"std.os": {
		{name: "exit", params: []string{"code"}},
		{name: "env", params: []string{"key"}},
		{name: "args", params: []string{}},
		{name: "cwd", params: []string{}},
		{name: "getenv", params: []string{"key"}},
		{name: "setenv", params: []string{"key", "val"}},
	},
	"std.fs": {
		{name: "readFile", params: []string{"path"}},
		{name: "writeFile", params: []string{"path", "data"}},
		{name: "exists", params: []string{"path"}},
		{name: "readDir", params: []string{"path"}},
		{name: "mkdir", params: []string{"path"}},
		{name: "remove", params: []string{"path"}},
		{name: "rename", params: []string{"src", "dst"}},
	},
	"std.env": {
		{name: "get", params: []string{"key"}},
		{name: "set", params: []string{"key", "val"}},
		{name: "all", params: []string{}},
		{name: "delete", params: []string{"key"}},
	},
	"std.json": {
		{name: "stringify", params: []string{"v"}},
		{name: "parse", params: []string{"s"}},
	},
	"std.regex": {
		{name: "test", params: []string{"pattern", "str"}},
		{name: "match", params: []string{"pattern", "str"}},
		{name: "replace", params: []string{"pattern", "str", "repl"}},
		{name: "split", params: []string{"pattern", "str"}},
	},
	"std.crypto": {
		{name: "md5", params: []string{"s"}},
		{name: "sha256", params: []string{"s"}},
		{name: "sha512", params: []string{"s"}},
		{name: "base64encode", params: []string{"s"}},
		{name: "base64decode", params: []string{"s"}},
		{name: "randomBytes", params: []string{"n"}},
	},
	"std.datetime": {
		{name: "now", params: []string{}},
		{name: "format", params: []string{"ts", "fmt"}},
		{name: "parse", params: []string{"s", "fmt"}},
	},
}

func (c *ntzC) compileAtImport(n *ast.Node) error {
	modPath := n.Source
	if modPath == "std.ffi" {
		return ErrNTZUnsupported
	}
	methods, known := stdlibModules[modPath]
	if !known {

		c.emitU8(ntzMakeObject)
		c.emitU16(0)
		return nil
	}

	propCount := 0
	for _, m := range methods {
		if m.params == nil {

			key := m.name
			c.emitU8(ntzConst)
			c.emitU8(ntzConstString)
			c.emitU32(uint32(len(key)))
			c.buf.WriteString(key)

			switch key {
			case "PI":
				c.emitU8(ntzConst)
				c.emitU8(ntzConstFloat)
				c.emitF64(3.141592653589793)
			case "E":
				c.emitU8(ntzConst)
				c.emitU8(ntzConstFloat)
				c.emitF64(2.718281828459045)
			default:
				c.emitU8(ntzConst)
				c.emitU8(ntzConstNull)
			}
			propCount++
			continue
		}

		key := m.name
		c.emitU8(ntzConst)
		c.emitU8(ntzConstString)
		c.emitU32(uint32(len(key)))
		c.buf.WriteString(key)

		builtinName := moduleBuiltinName(modPath, m.name)
		child := &ntzC{vars: make(map[string]uint16)}

		for _, p := range m.params {
			child.varSlot(p)
		}

		for _, p := range m.params {
			slot := child.vars[p]
			child.emitU8(ntzLoad)
			child.emitU16(slot)
		}
		child.emitU8(ntzCallRT)
		child.emitU8(uint8(len(builtinName)))
		child.buf.WriteString(builtinName)
		child.emitU8(uint8(len(m.params)))
		child.emitU8(ntzReturn)

		bodyBytes := child.buf.Bytes()
		name := m.name
		if len(name) > 255 {
			name = name[:255]
		}
		c.emitU8(ntzFuncDef)
		c.emitU8(uint8(len(m.params)))
		c.emitU8(uint8(len(name)))
		c.buf.WriteString(name)
		c.emitU32(uint32(len(bodyBytes)))
		c.buf.Write(bodyBytes)

		propCount++
	}

	c.emitU8(ntzMakeObject)
	c.emitU16(uint16(propCount))
	return nil
}

func moduleBuiltinName(mod, method string) string {

	globals := map[string]bool{
		"log": true, "sleep": true,
		"keys": true, "values": true, "entries": true,
		"abs": true, "ceil": true, "floor": true,
		"sqrt": true, "min": true, "max": true,
		"includes": true, "indexOf": true, "replace": true,
		"startsWith": true, "endsWith": true,
		"push": true, "pop": true, "slice": true,
		"join": true, "split": true, "trim": true,
		"toUpperCase": true, "toLowerCase": true,
		"concat": true, "reverse": true, "sort": true,
		"range": true, "len": true, "typeof": true,
		"parseInt": true, "parseFloat": true,
		"str": true, "num": true,
	}
	if globals[method] {
		return method
	}

	prefix := ""
	switch mod {
	case "std.utils":
		prefix = "utils_"
	case "std.math":
		prefix = "math_"
	case "std.os":
		prefix = "os_"
	case "std.fs":
		prefix = "fs_"
	case "std.env":
		prefix = "env_"
	case "std.json":
		prefix = "json_"
	case "std.crypto":
		prefix = "crypto_"
	case "std.datetime":
		prefix = "datetime_"
	case "std.io":
		prefix = "io_"
	case "std.regex":
		prefix = "regex_"
	}
	return prefix + method
}
