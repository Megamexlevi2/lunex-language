package formatter

import (
	"fmt"
	"lunex/internal/ast"
	"strings"
)

type Formatter struct {
	indent int
	buf    strings.Builder
}

func Format(root *ast.Node) string {
	f := &Formatter{}
	f.printNode(root, true)
	return strings.TrimRight(f.buf.String(), "\n") + "\n"
}

func (f *Formatter) writeLine(s string) {
	if s == "" {
		f.buf.WriteByte('\n')
		return
	}
	f.buf.WriteString(strings.Repeat("  ", f.indent))
	f.buf.WriteString(s)
	f.buf.WriteByte('\n')
}

func (f *Formatter) blankLine() {

	s := f.buf.String()
	if !strings.HasSuffix(s, "\n\n") {
		f.buf.WriteByte('\n')
	}
}

func (f *Formatter) printNode(n *ast.Node, topLevel bool) {
	if n == nil {
		return
	}
	switch n.Type {
	case ast.Program:
		f.printProgram(n)
	case ast.VarDecl, ast.ImmutableDecl:
		f.printVarDecl(n)
	case ast.FnDecl:
		f.printFnDecl(n)
	case ast.ClassDecl:
		f.printClassDecl(n)
	case ast.Block:
		f.printBlock(n)
	case ast.ExprStmt:
		f.printExprStmt(n)
	case ast.LogStmt:
		f.printLogStmt(n)
	case ast.ReturnStmt:
		f.printReturnStmt(n)
	case ast.IfStmt:
		f.printIfStmt(n)
	case ast.WhileStmt:
		f.printWhileStmt(n)
	case ast.WatchStmt:
		f.printWatchStmt(n)
	case ast.ForStmt:
		f.printForStmt(n)
	case ast.ForOfStmt, ast.EachInStmt:
		f.printForOfStmt(n)
	case ast.BreakStmt:
		f.writeLine("break")
	case ast.ContinueStmt:
		f.writeLine("continue")
	case ast.ThrowStmt, ast.RaiseStmt:
		f.writeLine("throw " + f.expr(n.Arg))
	case ast.TryStmt:
		f.printTryStmt(n)
	case ast.MatchStmt:
		f.printMatchStmt(n)
	case ast.ImportDecl:
		f.printImportDecl(n)
	case ast.ExportDecl:
		f.printExportDecl(n)
	case ast.SpawnStmt:
		f.writeLine("spawn " + f.expr(n.Expr))
	case ast.DeferStmt:
		f.writeLine("defer " + f.expr(n.Expr))
	case ast.AssertStmt:
		f.writeLine("assert " + f.expr(n.Expr))
	default:

		if n.Expr != nil {
			f.writeLine(f.expr(n.Expr))
		} else {

			f.writeLine(f.expr(n))
		}
	}
}

func (f *Formatter) printProgram(n *ast.Node) {
	stmts := n.Body_
	for i, stmt := range stmts {
		f.printNode(stmt, true)

		if i+1 < len(stmts) {
			next := stmts[i+1]
			if stmt.Type == ast.FnDecl || stmt.Type == ast.ClassDecl ||
				next.Type == ast.FnDecl || next.Type == ast.ClassDecl {
				f.blankLine()
			}
		}
	}
}

func (f *Formatter) printVarDecl(n *ast.Node) {
	kw := "val"
	if n.Type == ast.VarDecl && !n.IsConst {
		kw = "var"
	}
	if n.Init != nil {
		f.writeLine(fmt.Sprintf("%s %s = %s", kw, n.Name, f.expr(n.Init)))
	} else {
		f.writeLine(fmt.Sprintf("%s %s", kw, n.Name))
	}
}

func (f *Formatter) printFnDecl(n *ast.Node) {
	params := f.formatParams(n.Params)
	header := fmt.Sprintf("fn %s(%s) {", n.Name, params)
	f.writeLine(header)
	f.indent++
	f.printBlockBody(n.Body)
	f.indent--
	f.writeLine("}")
}

func (f *Formatter) printClassDecl(n *ast.Node) {
	header := fmt.Sprintf("class %s", n.Name)
	if n.Extends != nil {
		header += " extends " + f.expr(n.Extends)
	}
	header += " {"
	f.writeLine(header)
	f.indent++
	for _, m := range n.Methods {
		f.printClassMember(m)
	}
	f.indent--
	f.writeLine("}")
}

func (f *Formatter) printClassMember(m *ast.ClassMember) {
	prefix := ""
	if m.IsStatic {
		prefix += "static "
	}
	if m.IsPrivate {
		prefix += "private "
	}
	switch m.Kind {
	case "method", "constructor":
		params := f.formatParams(m.Params)
		f.writeLine(fmt.Sprintf("%sfn %s(%s) {", prefix, m.Name, params))
		f.indent++
		f.printBlockBody(m.Body)
		f.indent--
		f.writeLine("}")
		f.blankLine()
	case "field":
		if m.Init != nil {
			f.writeLine(fmt.Sprintf("%s%s = %s", prefix, m.Name, f.expr(m.Init)))
		} else {
			f.writeLine(fmt.Sprintf("%s%s", prefix, m.Name))
		}
	}
}

func (f *Formatter) printBlock(n *ast.Node) {
	f.writeLine("{")
	f.indent++
	f.printBlockBody(n)
	f.indent--
	f.writeLine("}")
}

func (f *Formatter) printBlockBody(n *ast.Node) {
	if n == nil {
		return
	}
	for _, stmt := range n.Body_ {
		f.printNode(stmt, false)
	}
}

func (f *Formatter) printExprStmt(n *ast.Node) {
	if n.Expr != nil {
		f.writeLine(f.expr(n.Expr))
	}
}

func (f *Formatter) printLogStmt(n *ast.Node) {

	if n.Expr != nil {
		f.writeLine("log " + f.expr(n.Expr))
	}
}

func (f *Formatter) printReturnStmt(n *ast.Node) {
	if n.Arg != nil {
		f.writeLine("return " + f.expr(n.Arg))
	} else {
		f.writeLine("return")
	}
}

func (f *Formatter) printIfStmt(n *ast.Node) {
	f.printIfChain(n)
}

func (f *Formatter) printIfChain(n *ast.Node) {

	f.writeLine(fmt.Sprintf("if %s {", f.expr(n.Test)))
	f.indent++
	f.printBlockBody(n.Consequent)
	f.indent--
	f.printElseChain(n.Alternate)
}

func (f *Formatter) printElseChain(alt *ast.Node) {
	if alt == nil {
		f.writeLine("}")
		return
	}
	if alt.Type == ast.IfStmt {
		f.writeLine(fmt.Sprintf("} else if %s {", f.expr(alt.Test)))
		f.indent++
		f.printBlockBody(alt.Consequent)
		f.indent--
		f.printElseChain(alt.Alternate)
	} else {
		f.writeLine("} else {")
		f.indent++
		f.printBlockBody(alt)
		f.indent--
		f.writeLine("}")
	}
}

func (f *Formatter) printWhileStmt(n *ast.Node) {
	f.writeLine(fmt.Sprintf("while %s {", f.expr(n.Test)))
	f.indent++
	f.printBlockBody(n.Body)
	f.indent--
	f.writeLine("}")
}

func (f *Formatter) printWatchStmt(n *ast.Node) {
	f.writeLine("watch " + f.expr(n.Subject) + " {")
	f.indent++
	f.printBlockBody(n.Body)
	f.indent--
	f.writeLine("}")
}

func (f *Formatter) printForStmt(n *ast.Node) {
	init := ""
	if n.Init != nil {
		init = f.expr(n.Init)
	}
	test := ""
	if n.Test != nil {
		test = f.expr(n.Test)
	}
	upd := ""
	if n.Right != nil {
		upd = f.expr(n.Right)
	}
	f.writeLine(fmt.Sprintf("for %s; %s; %s {", init, test, upd))
	f.indent++
	f.printBlockBody(n.Body)
	f.indent--
	f.writeLine("}")
}

func (f *Formatter) printForOfStmt(n *ast.Node) {
	kw := "for"
	if n.Type == ast.EachInStmt {
		kw = "each"
	}
	inKw := "of"
	if n.Type == ast.EachInStmt {
		inKw = "in"
	}
	f.writeLine(fmt.Sprintf("%s %s %s %s {", kw, n.Name, inKw, f.expr(n.Subject)))
	f.indent++
	f.printBlockBody(n.Body)
	f.indent--
	f.writeLine("}")
}

func (f *Formatter) printTryStmt(n *ast.Node) {
	f.writeLine("try {")
	f.indent++
	f.printBlockBody(n.Body)
	f.indent--
	if n.CatchBlock != nil {
		if n.CatchParam != "" {
			f.writeLine(fmt.Sprintf("} catch (%s) {", n.CatchParam))
		} else {
			f.writeLine("} catch {")
		}
		f.indent++
		f.printBlockBody(n.CatchBlock)
		f.indent--
	}
	if n.FinallyBlock != nil {
		f.writeLine("} finally {")
		f.indent++
		f.printBlockBody(n.FinallyBlock)
		f.indent--
	}
	f.writeLine("}")
}

func (f *Formatter) printMatchStmt(n *ast.Node) {
	f.writeLine(fmt.Sprintf("match %s {", f.expr(n.Subject)))
	f.indent++
	for _, c := range n.Cases {
		f.printMatchCase(c)
	}
	f.indent--
	f.writeLine("}")
}

func (f *Formatter) printMatchCase(c *ast.MatchCase) {
	if c.IsDefault {
		f.writeLine("_ => {")
	} else {
		pats := make([]string, len(c.Patterns))
		for i, p := range c.Patterns {
			pats[i] = f.formatMatchPattern(p)
		}
		line := strings.Join(pats, " | ") + " => {"
		if c.Guard != nil {
			line = strings.Join(pats, " | ") + " if " + f.expr(c.Guard) + " => {"
		}
		f.writeLine(line)
	}
	f.indent++
	f.printBlockBody(c.Body)
	f.indent--
	f.writeLine("}")
}

func (f *Formatter) formatMatchPattern(p *ast.MatchPattern) string {
	if p == nil {
		return "_"
	}
	switch p.Kind {
	case "literal":
		return fmt.Sprintf("%v", p.Value)
	case "identifier":
		return p.Name
	case "wildcard":
		return "_"
	default:
		return fmt.Sprintf("%v", p.Value)
	}
}

func (f *Formatter) printImportDecl(n *ast.Node) {
	if len(n.Specifiers) == 0 {
		f.writeLine(fmt.Sprintf("import \"%s\"", n.Source))
		return
	}
	names := make([]string, len(n.Specifiers))
	for i, s := range n.Specifiers {
		if s.Local != s.Imported && s.Local != "" {
			names[i] = s.Imported + " as " + s.Local
		} else {
			names[i] = s.Imported
		}
	}
	f.writeLine(fmt.Sprintf("import { %s } from \"%s\"", strings.Join(names, ", "), n.Source))
}

func (f *Formatter) printExportDecl(n *ast.Node) {
	if n.Declaration == nil {
		return
	}

	sub := &Formatter{indent: f.indent}
	sub.printNode(n.Declaration, false)
	line := strings.TrimRight(sub.buf.String(), "\n")

	indentStr := strings.Repeat("  ", f.indent)
	if strings.HasPrefix(line, indentStr) {
		line = line[len(indentStr):]
	}
	f.writeLine("export " + line)
}

func (f *Formatter) expr(n *ast.Node) string {
	if n == nil {
		return ""
	}
	switch n.Type {
	case ast.NumberLit:
		return fmt.Sprintf("%v", n.Value)
	case ast.StringLit:
		s, _ := n.Value.(string)
		return fmt.Sprintf("%q", s)
	case ast.TemplateLit:
		s, _ := n.Value.(string)
		return "`" + s + "`"
	case ast.BoolLit:
		if b, ok := n.Value.(bool); ok {
			if b {
				return "true"
			}
			return "false"
		}
		return fmt.Sprintf("%v", n.Value)
	case ast.NullLit:
		return "null"
	case ast.UndefinedLit:
		return "undefined"
	case ast.Identifier:
		return n.Name
	case ast.ThisExpr:
		return "this"
	case ast.SuperExpr:
		return "super"
	case ast.VoidExpr:
		return "void " + f.expr(n.Arg)
	case ast.TypeofExpr:
		return "typeof " + f.expr(n.Arg)
	case ast.NotExpr, ast.UnaryExpr:
		if n.Op != "" {
			return n.Op + f.expr(n.Arg)
		}
		return "!" + f.expr(n.Arg)
	case ast.BinaryExpr, ast.LogicalExpr:
		return f.expr(n.Left) + " " + n.Op + " " + f.expr(n.Right)
	case ast.AssignExpr:
		op := "="
		if n.Op != "" {
			op = n.Op
		}
		return f.expr(n.Left) + " " + op + " " + f.expr(n.Right)
	case ast.TernaryExpr:
		return f.expr(n.Test) + " ? " + f.expr(n.Consequent) + " : " + f.expr(n.Alternate)
	case ast.MemberExpr:
		obj := f.expr(n.Object)
		if n.Computed {
			return obj + "[" + f.expr(n.Left) + "]"
		}
		prop := ""
		switch p := n.Prop.(type) {
		case string:
			prop = p
		case *ast.Node:
			prop = f.expr(p)
		default:
			prop = fmt.Sprintf("%v", p)
		}
		return obj + "." + prop
	case ast.CallExpr:
		callee := f.expr(n.Callee)
		args := f.formatArgs(n.Args)
		return callee + "(" + args + ")"
	case ast.NewExpr:
		callee := f.expr(n.Callee)
		args := f.formatArgs(n.Args)
		return "new " + callee + "(" + args + ")"
	case ast.ArrayLit:
		elems := f.formatArgs(n.Elements)
		return "[" + elems + "]"
	case ast.ObjectLit:
		return f.formatObjectLit(n)
	case ast.StructLit:
		return "struct " + f.formatObjectLit(n)
	case ast.FnExpr, ast.ArrowFn:
		return f.formatFnExpr(n)
	case ast.AtImportExpr, ast.LunexRequire:
		return "@import(\"" + n.Source + "\")"
	case ast.NaxImportExpr:
		return "@fimport(\"" + n.Source + "\")"
	case ast.SpreadExpr:
		return "..." + f.expr(n.Arg)
	case ast.PipelineExpr:
		return f.expr(n.Left) + " |> " + f.expr(n.Right)
	case ast.RangeExpr:
		return f.expr(n.Lo) + ".." + f.expr(n.Hi)
	case ast.TrySafeExpr:
		return "try? " + f.expr(n.Expr)
	case ast.HaveExpr:
		return "have " + f.expr(n.Expr)
	case ast.VarDecl, ast.ImmutableDecl:
		kw := "val"
		if n.Type == ast.VarDecl && !n.IsConst {
			kw = "var"
		}
		if n.Init != nil {
			return fmt.Sprintf("%s %s = %s", kw, n.Name, f.expr(n.Init))
		}
		return fmt.Sprintf("%s %s", kw, n.Name)
	default:

		if n.Value != nil {
			return fmt.Sprintf("%v", n.Value)
		}
		if n.Name != "" {
			return n.Name
		}
		return ""
	}
}

func (f *Formatter) formatArgs(args []*ast.Node) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = f.expr(a)
	}
	return strings.Join(parts, ", ")
}

func (f *Formatter) formatParams(params []*ast.Param) string {
	if len(params) == 0 {
		return ""
	}
	parts := make([]string, len(params))
	for i, p := range params {
		s := p.Name
		if p.Rest {
			s = "..." + s
		}
		if p.DefaultVal != nil {
			s += " = " + f.expr(p.DefaultVal)
		}
		parts[i] = s
	}
	return strings.Join(parts, ", ")
}

func (f *Formatter) formatObjectLit(n *ast.Node) string {
	if len(n.Properties) == 0 {
		return "{}"
	}
	parts := make([]string, len(n.Properties))
	for i, p := range n.Properties {
		key := ""
		switch k := p.Key.(type) {
		case string:
			key = k
		case *ast.Node:
			key = f.expr(k)
		default:
			key = fmt.Sprintf("%v", k)
		}
		if p.Value != nil {
			parts[i] = key + ": " + f.expr(p.Value)
		} else {
			parts[i] = key
		}
	}

	inline := "{ " + strings.Join(parts, ", ") + " }"
	if len(inline) <= 60 {
		return inline
	}
	return "{\n" + strings.Repeat("  ", f.indent+1) +
		strings.Join(parts, ",\n"+strings.Repeat("  ", f.indent+1)) +
		"\n" + strings.Repeat("  ", f.indent) + "}"
}

func (f *Formatter) formatFnExpr(n *ast.Node) string {
	params := f.formatParams(n.Params)
	if n.Type == ast.ArrowFn {
		if n.Body != nil && len(n.Body.Body_) == 1 {
			stmt := n.Body.Body_[0]
			if stmt.Type == ast.ReturnStmt && stmt.Arg != nil {
				return "(" + params + ") => " + f.expr(stmt.Arg)
			}
			if stmt.Type == ast.ExprStmt && stmt.Expr != nil {
				return "(" + params + ") => " + f.expr(stmt.Expr)
			}
		}
		return "(" + params + ") => { ... }"
	}
	return "fn(" + params + ") { ... }"
}
