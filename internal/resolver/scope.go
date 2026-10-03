package resolver

import "lunex/internal/ast"

type scope struct {
	parent  *scope
	node    *ast.Node
	names   []string
	index   map[string]int
	dynamic bool
}

func newScope(parent *scope, node *ast.Node) *scope {
	return &scope{
		parent: parent,
		node:   node,
		index:  make(map[string]int),
	}
}

func (s *scope) declare(name string) int {
	if name == "" {
		return -1
	}
	if slot, ok := s.index[name]; ok {
		return slot
	}
	slot := len(s.names)
	s.names = append(s.names, name)
	s.index[name] = slot
	return slot
}

func (s *scope) resolve(name string) (hops int, slot int, ok bool) {
	cur := s
	depth := 0
	for cur != nil {
		if cur.dynamic {

			return 0, 0, false
		}
		if slot, found := cur.index[name]; found {
			return depth, slot, true
		}
		cur = cur.parent
		depth++
	}
	return 0, 0, false
}

func (s *scope) finish() {
	if s.node == nil {
		return
	}
	s.node.ScopeInfo = &ast.ScopeInfo{
		Names:   append([]string(nil), s.names...),
		Dynamic: s.dynamic,
	}
}

func SlotCount(info *ast.ScopeInfo) int {
	if info == nil {
		return 0
	}
	return len(info.Names)
}

func SlotIndex(info *ast.ScopeInfo, name string) int {
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
