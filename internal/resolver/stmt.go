package resolver

import "lunex/internal/ast"

func (r *resolver) resolveStmt(node *ast.Node, sc *scope) {
	if node == nil {
		return
	}
	switch node.Type {
	case ast.VarDecl, ast.ImmutableDecl, ast.UsingDecl:
		if node.Init != nil {
			r.resolveExpr(node.Init, sc)
		}
		if node.Destructure != nil {
			r.declareDestructure(sc, node.Destructure)
		} else if sc != nil && node.Name != "" {
			slot := sc.declare(node.Name)
			node.ResolvedAddr = &ast.ResolvedAddr{Hops: 0, Slot: slot}
		}

	case ast.FnDecl:

		r.declareIfLocal(sc, node.Name)
		r.resolveFunctionBody(node, sc)

	case ast.ClassDecl:
		r.declareIfLocal(sc, node.Name)
		if node.SuperClass != nil {
			r.resolveExpr(node.SuperClass, sc)
		}
		for _, member := range node.Methods {

			body := member.Body
			if member.Init != nil {
				body = member.Init
			}
			r.resolveMethodLike(body, member.Params, sc)
		}

	case ast.EnumDecl:
		r.declareIfLocal(sc, node.Name)
		for _, member := range node.Members {
			if member.Init != nil {
				r.resolveExpr(member.Init, sc)
			}
		}

	case ast.NamespaceDecl:
		r.declareIfLocal(sc, node.Name)

		nsScope := newScope(sc, nil)
		nsScope.dynamic = true
		r.resolveStmts(node.Body_, nsScope)

	case ast.ComponentDecl:
		r.declareIfLocal(sc, node.Name)
		r.resolveFunctionBody(node, sc)

	case ast.Block:
		r.resolveBlock(node, sc)

	case ast.ExprStmt:
		if node.Expr != nil {
			r.resolveExpr(node.Expr, sc)
		}

	case ast.LogStmt:
		for _, arg := range node.Args {
			r.resolveExpr(arg, sc)
		}

	case ast.ReturnStmt:
		if n, ok := node.Value.(*ast.Node); ok && n != nil {
			r.resolveExpr(n, sc)
		}

	case ast.ThrowStmt, ast.RaiseStmt:
		if n, ok := node.Value.(*ast.Node); ok && n != nil {
			r.resolveExpr(n, sc)
		}

	case ast.BreakStmt, ast.ContinueStmt:

	case ast.IfStmt, ast.UnlessStmt:
		r.resolveExpr(node.Test, sc)
		r.resolveStmt(node.Consequent, sc)
		if node.Alternate != nil {
			r.resolveStmt(node.Alternate, sc)
		}

	case ast.WhileStmt:
		r.resolveExpr(node.Test, sc)
		r.resolveStmt(node.Body, sc)

	case ast.ForStmt:

		forScope := newScope(sc, node)
		if node.Init != nil {
			r.resolveStmt(node.Init, forScope)
		}
		if node.Test != nil {
			r.resolveExpr(node.Test, forScope)
		}
		if node.Body != nil {
			r.resolveStmt(node.Body, forScope)
		}
		if node.Right != nil {
			r.resolveExpr(node.Right, forScope)
		}
		forScope.finish()

	case ast.ForOfStmt, ast.EachInStmt:
		r.resolveExpr(node.Right, sc)

		iterScope := newScope(sc, node)
		if node.Destructure != nil {
			r.declareDestructure(iterScope, node.Destructure)
		} else {
			iterScope.declare(node.Name)
		}
		if node.Alias != "" {
			iterScope.declare(node.Alias)
		}
		r.resolveStmt(node.Body, iterScope)
		iterScope.finish()

	case ast.RepeatStmt:
		if node.Count != nil {
			r.resolveExpr(node.Count, sc)
		}
		r.resolveStmt(node.Body, sc)

	case ast.LoopStmt:
		r.resolveStmt(node.Body, sc)

	case ast.MatchStmt:
		r.resolveMatch(node, sc)

	case ast.TryStmt:
		r.resolveStmt(node.Body, sc)
		if node.CatchBlock != nil {
			catchScope := newScope(sc, node.CatchBlock)
			if node.CatchParam != "" {
				catchScope.declare(node.CatchParam)
			}
			r.resolveStmt(node.CatchBlock, catchScope)
			catchScope.finish()
		}
		if node.FinallyBlock != nil {
			r.resolveStmt(node.FinallyBlock, sc)
		}

	case ast.SpawnStmt:

		if node.Expr != nil {
			r.resolveExpr(node.Expr, sc)
		}

	case ast.SelectStmt:
		for _, sel := range node.SelectCases {
			if sel.Channel != nil {
				r.resolveExpr(sel.Channel, sc)
			}
			caseScope := newScope(sc, nil)
			if sel.Binding != "" {
				caseScope.declare(sel.Binding)
			}
			r.resolveStmt(sel.Body, caseScope)
		}

	case ast.WithStmt:
		if node.Expr != nil {
			r.resolveExpr(node.Expr, sc)
		}

		withScope := newScope(sc, nil)
		withScope.dynamic = true
		r.resolveStmt(node.Body, withScope)

	case ast.GuardStmt:
		r.resolveExpr(node.Test, sc)
		if node.Alternate != nil {
			r.resolveStmt(node.Alternate, sc)
		}

	case ast.AssertStmt:
		r.resolveExpr(node.Test, sc)
		if node.Arg != nil {
			r.resolveExpr(node.Arg, sc)
		}

	case ast.HaveStmt, ast.IfHaveStmt:
		if node.Expr != nil {
			r.resolveExpr(node.Expr, sc)
		}
		haveScope := newScope(sc, node)
		if node.Alias != "" {
			haveScope.declare(node.Alias)
		}
		if node.Consequent != nil {
			r.resolveStmt(node.Consequent, haveScope)
		}
		haveScope.finish()
		if node.Alternate != nil {

			r.resolveStmt(node.Alternate, sc)
		}

	case ast.IfSetStmt:
		if node.Expr != nil {
			r.resolveExpr(node.Expr, sc)
		}
		ifSetScope := newScope(sc, node)

		alias := node.Alias
		if alias == "" {
			alias = syntheticIfSetName(node.ID)
		}
		ifSetScope.declare(alias)
		if node.Consequent != nil {
			r.resolveStmt(node.Consequent, ifSetScope)
		}
		ifSetScope.finish()
		if node.Alternate != nil {
			r.resolveStmt(node.Alternate, sc)
		}

	case ast.DeleteStmt:
		if node.Expr != nil {
			r.resolveExpr(node.Expr, sc)
		}

	case ast.ImportDecl, ast.ExportDecl, ast.LunexRequire, ast.UseStmt:

	case ast.DeferStmt:

		if node.Body != nil {
			r.resolveStmt(node.Body, sc)
		} else if node.Expr != nil {
			r.resolveExpr(node.Expr, sc)
		}

	default:

	}
}

func (r *resolver) resolveBlock(node *ast.Node, sc *scope) {
	if !blockDeclaresBindings(node) {
		r.resolveStmts(node.Body_, sc)
		return
	}
	blockScope := newScope(sc, node)
	r.resolveStmts(node.Body_, blockScope)
	blockScope.finish()
}

func blockDeclaresBindings(node *ast.Node) bool {
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

func syntheticIfSetName(id int) string {

	const prefix = "_ifset_"
	if id == 0 {
		return prefix + "0"
	}
	neg := id < 0
	if neg {
		id = -id
	}
	var buf [20]byte
	i := len(buf)
	for id > 0 {
		i--
		buf[i] = byte('0' + id%10)
		id /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return prefix + string(buf[i:])
}
