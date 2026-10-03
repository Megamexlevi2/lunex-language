package resolver

import "lunex/internal/ast"

const (
	pseudoThis       = "this"
	pseudoSuperClass = "__super_class__"
)

func (r *resolver) resolveFunctionBody(node *ast.Node, enclosing *scope) {
	r.resolveMethodLike(node.Body, node.Params, enclosing)
}

func (r *resolver) resolveMethodLike(body *ast.Node, params []*ast.Param, enclosing *scope) {
	bodyNode, ok := asBlock(body)
	if !ok {

		return
	}

	fnScope := newScope(enclosing, bodyNode)

	fnScope.declare(pseudoThis)
	fnScope.declare(pseudoSuperClass)

	paramNames := make(map[string]bool, len(params))
	for _, p := range params {
		if p == nil {
			continue
		}
		if p.Destructure != nil {
			p.ResolvedSlot = -1
			r.declareDestructure(fnScope, p.Destructure)
			continue
		}
		p.ResolvedSlot = fnScope.declare(p.Name)
		paramNames[p.Name] = true
		if p.DefaultVal != nil {

			r.resolveExpr(p.DefaultVal, fnScope)
		}
	}

	for _, stmt := range bodyNode.Body_ {
		if stmt != nil && stmt.Type == ast.FnDecl && stmt.Name != "" && !paramNames[stmt.Name] {
			fnScope.declare(stmt.Name)
		}
	}

	for _, stmt := range bodyNode.Body_ {
		if stmt != nil && stmt.Type == ast.FnDecl && stmt.Name != "" {

			r.resolveFunctionBody(stmt, fnScope)
			continue
		}
		r.resolveStmt(stmt, fnScope)
	}

	fnScope.finish()
}

func asBlock(node *ast.Node) (*ast.Node, bool) {
	if node == nil || node.Type != ast.Block {
		return nil, false
	}
	return node, true
}
