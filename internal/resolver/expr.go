package resolver

import "lunex/internal/ast"

func (r *resolver) resolveExpr(node *ast.Node, sc *scope) {
	if node == nil {
		return
	}
	switch node.Type {
	case ast.Identifier:
		r.resolveIdentifierRef(node, sc)

	case ast.NumberLit, ast.StringLit, ast.BoolLit, ast.NullLit,
		ast.UndefinedLit, ast.RegexLit, ast.ChannelExpr, ast.NaxImportExpr:

	case ast.TemplateLit:

	case ast.ArrayLit:
		for _, el := range node.Elements {
			r.resolveExpr(el, sc)
		}

	case ast.ObjectLit:
		for _, prop := range node.Properties {
			if prop == nil {
				continue
			}
			if prop.Computed {
				if kn, ok := prop.Key.(*ast.Node); ok {
					r.resolveExpr(kn, sc)
				}
			}
			if prop.Value != nil {
				r.resolveExpr(prop.Value, sc)
			}
			if prop.Arg != nil {
				r.resolveExpr(prop.Arg, sc)
			}
			if prop.Kind == "shorthand" {

				if sc != nil {
					if key, ok := prop.Key.(string); ok {
						if hops, slot, found := sc.resolve(key); found {
							prop.ShorthandAddr = &ast.ResolvedAddr{Hops: hops, Slot: slot}
						}
					}
				}
			}
			if prop.Kind == "method" && (prop.IsGet || prop.IsSet || prop.Body != nil) {

				r.resolveMethodLike(prop.Body, prop.Params, sc)
			}
		}

	case ast.ThisExpr, ast.SuperExpr:

	case ast.VoidExpr:
		r.resolveExpr(node.Arg, sc)

	case ast.TypeofExpr:
		if node.Arg != nil {
			r.resolveExpr(node.Arg, sc)
		} else if node.Expr != nil {
			r.resolveExpr(node.Expr, sc)
		}

	case ast.DeleteExpr:
		if node.Expr != nil {
			r.resolveExpr(node.Expr, sc)
		} else if node.Arg != nil {
			r.resolveExpr(node.Arg, sc)
		}

	case ast.FnExpr, ast.FnDecl:

		r.resolveFunctionBody(node, sc)

	case ast.ArrowFn:
		r.resolveFunctionBody(node, sc)

	case ast.CallExpr:
		if node.Callee != nil {
			r.resolveExpr(node.Callee, sc)
		}
		for _, arg := range node.Args {
			r.resolveExpr(arg, sc)
		}

	case ast.NewExpr:
		if node.Callee != nil {
			r.resolveExpr(node.Callee, sc)
		}
		for _, arg := range node.Args {
			r.resolveExpr(arg, sc)
		}

	case ast.MemberExpr:
		if node.Object != nil {
			r.resolveExpr(node.Object, sc)
		}
		if node.Computed {
			if pn, ok := node.Prop.(*ast.Node); ok {
				r.resolveExpr(pn, sc)
			}
		}

	case ast.BinaryExpr, ast.LogicalExpr:
		r.resolveExpr(node.Left, sc)
		r.resolveExpr(node.Right, sc)

	case ast.UnaryExpr:
		r.resolveExpr(node.Arg, sc)

	case ast.AssignExpr:
		r.resolveExpr(node.Right, sc)
		if node.Op != "=" {
			r.resolveExpr(node.Left, sc)
		}
		r.resolveAssignTarget(node.Left, sc)

	case ast.TernaryExpr:
		r.resolveExpr(node.Test, sc)
		r.resolveExpr(node.Consequent, sc)
		r.resolveExpr(node.Alternate, sc)

	case ast.SpreadExpr:
		r.resolveExpr(node.Arg, sc)

	case ast.PipelineExpr:
		r.resolveExpr(node.Left, sc)
		r.resolveExpr(node.Right, sc)

	case ast.SequenceExpr:
		for _, e := range node.Exprs {
			r.resolveExpr(e, sc)
		}

	case ast.NotExpr:
		r.resolveExpr(node.Arg, sc)

	case ast.HaveExpr:
		if node.Expr != nil {
			r.resolveExpr(node.Expr, sc)
		}
		if n, ok := node.InExpr.(*ast.Node); ok && n != nil {
			r.resolveExpr(n, sc)
		}

	case ast.TrySafeExpr:
		r.resolveExpr(node.Expr, sc)

	case ast.RangeExpr:
		for _, a := range node.Args {
			r.resolveExpr(a, sc)
		}
		if node.Lo != nil {
			r.resolveExpr(node.Lo, sc)
		}
		if node.Hi != nil {
			r.resolveExpr(node.Hi, sc)
		}

	case ast.SleepExpr:
		if node.Ms != nil {
			r.resolveExpr(node.Ms, sc)
		}

	case ast.AtImportExpr:

	case ast.StructLit:
		r.resolveStructLit(node, sc)

	case ast.MatchStmt:
		r.resolveMatch(node, sc)

	case ast.SatisfiesExpr:
		if node.Expr != nil {
			r.resolveExpr(node.Expr, sc)
		}

	case ast.DecoratedExpr:
		if node.Expr != nil {
			r.resolveExpr(node.Expr, sc)
		}
		for _, d := range node.Decorators {
			r.resolveExpr(d, sc)
		}

	case ast.ExprStmt:
		r.resolveExpr(node.Expr, sc)

	case ast.IfStmt, ast.UnlessStmt:

		r.resolveStmt(node, sc)

	default:

	}
}

func (r *resolver) resolveIdentifierRef(node *ast.Node, sc *scope) {
	if sc == nil {
		return
	}
	switch node.Name {
	case "undefined", "null", "true", "false", "NaN", "Infinity":

		return
	}
	if hops, slot, ok := sc.resolve(node.Name); ok {
		node.ResolvedAddr = &ast.ResolvedAddr{Hops: hops, Slot: slot}
	}
}

func (r *resolver) resolveAssignTarget(target *ast.Node, sc *scope) {
	if target == nil {
		return
	}
	switch target.Type {
	case ast.Identifier:
		r.resolveIdentifierRef(target, sc)
	case ast.MemberExpr:
		r.resolveExpr(target, sc)
	case ast.ArrayLit, ast.ObjectLit:

	}
}

func (r *resolver) resolveStructLit(node *ast.Node, sc *scope) {
	litScope := newScope(sc, node)
	for _, stmt := range node.Body_ {
		if stmt == nil {
			continue
		}
		if stmt.Type == ast.ExprStmt && stmt.Expr != nil &&
			stmt.Expr.Type == ast.AssignExpr && stmt.Expr.Op == "=" {
			if left := stmt.Expr.Left; left != nil && left.Type == ast.Identifier && left.Name != "" {
				r.resolveExpr(stmt.Expr.Right, litScope)
				litScope.declare(left.Name)
				continue
			}
		}
		r.resolveStmt(stmt, litScope)
	}
	litScope.finish()
}
