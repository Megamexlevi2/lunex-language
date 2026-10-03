package resolver

import "lunex/internal/ast"

func Resolve(program *ast.Node) {
	if program == nil {
		return
	}
	r := &resolver{}

	r.resolveStmts(program.Body_, nil)
}

type resolver struct{}

func (r *resolver) resolveStmts(stmts []*ast.Node, sc *scope) {
	for _, stmt := range stmts {
		r.resolveStmt(stmt, sc)
	}
}

func (r *resolver) declareIfLocal(sc *scope, name string) {
	if sc == nil || name == "" {
		return
	}
	sc.declare(name)
}

func (r *resolver) declareDestructure(sc *scope, pattern interface{}) {
	if sc == nil || pattern == nil {
		return
	}
	m, ok := pattern.(map[string]interface{})
	if !ok {
		return
	}
	switch m["kind"] {
	case "object":
		props, _ := m["props"].([]map[string]interface{})
		for _, prop := range props {
			if prop["kind"] == "rest" {
				name, _ := prop["name"].(string)
				sc.declare(name)
				continue
			}
			alias, _ := prop["alias"].(string)
			key, _ := prop["key"].(string)
			if alias == "" {
				alias = key
			}
			sc.declare(alias)
			if dn, ok := prop["default"].(*ast.Node); ok && dn != nil {

				r.resolveExpr(dn, sc)
			}
		}
	case "array":
		items, _ := m["items"].([]interface{})
		for _, item := range items {
			if item == nil {
				continue
			}
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if itemMap["kind"] == "rest" {
				name, _ := itemMap["name"].(string)
				sc.declare(name)
				continue
			}
			name, _ := itemMap["name"].(string)
			sc.declare(name)
			if dn, ok := itemMap["default"].(*ast.Node); ok && dn != nil {
				r.resolveExpr(dn, sc)
			}
		}
	}
}
