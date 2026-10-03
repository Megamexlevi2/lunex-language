package resolver

import "lunex/internal/ast"

func (r *resolver) resolveMatch(node *ast.Node, sc *scope) {
	if node.Subject != nil {
		r.resolveExpr(node.Subject, sc)
	}
	for _, mc := range node.Cases {
		if mc.IsDefault {
			r.resolveStmt(mc.Body, sc)
			continue
		}

		caseScope := newScope(sc, nil)
		for _, pat := range mc.Patterns {
			collectPatternBindings(pat, caseScope)
		}
		if mc.Guard != nil {
			r.resolveExpr(mc.Guard, caseScope)
		}
		r.resolveStmt(mc.Body, caseScope)
	}
}

func collectPatternBindings(pat *ast.MatchPattern, sc *scope) {
	if pat == nil {
		return
	}
	switch pat.Kind {
	case "binding":
		sc.declare(pat.Name)
	case "array":
		for _, item := range pat.Items {
			if item == nil {
				continue
			}
			if item.Kind == "rest" {
				sc.declare(item.Name)
				continue
			}
			collectPatternBindings(item, sc)
		}
	case "object":

		for _, prop := range pat.Props {
			if prop == nil {
				continue
			}
			alias := prop.Alias
			if alias == "" {
				alias = prop.Key
			}
			sc.declare(alias)
		}
	}
}
