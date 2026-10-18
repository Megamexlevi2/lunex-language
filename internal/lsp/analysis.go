package lsp

import (
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/lexer"
	"reflect"
	"sort"
	"strings"
	"unicode/utf16"
)

type Symbol struct {
	Name       string
	Kind       string
	Detail     string
	URI        string
	Path       string
	Range      lspRange
	Selection  lspRange
	TokenIndex int
	ScopePath  []int
	Container  string
	Node       *ast.Node
	Type       string
	Exported   bool
	Deprecated bool
}

func (s *Server) collectSymbols(doc *Document) []*Symbol {
	if doc.AST == nil {
		return nil
	}
	var symbols []*Symbol
	seen := map[*ast.Node]bool{}
	var walk func(*ast.Node)
	walk = func(node *ast.Node) {
		if node == nil || seen[node] {
			return
		}
		seen[node] = true
		if name, kind := declarationInfo(node); name != "" {
			tokenIndex := s.findNameToken(doc, name, node.Line, node.Col, -1)
			if tokenIndex >= 0 {
				symbols = append(symbols, s.makeSymbol(doc, name, kind, tokenIndex, node, "", inferNodeType(node.Init), true))
			}
		}
		if node.Type == ast.ExportDecl && node.Declaration != nil {
			if name, _ := declarationInfo(node.Declaration); name != "" {
				for _, sym := range symbols {
					if sym.Name == name && sym.TokenIndex >= 0 {
						sym.Exported = true
					}
				}
			}
		}
		if node.Type == ast.FnDecl || node.Type == ast.FnExpr || node.Type == ast.ArrowFn || node.Type == ast.ComponentDecl {
			bodyOpen := s.findTokenAt(doc, node.Body)
			for _, param := range node.Params {
				if param == nil || param.Name == "" {
					continue
				}
				idx := s.findParameterToken(doc, param.Name, node.Line, node.Col, bodyOpen)
				if idx >= 0 {
					sym := s.makeSymbol(doc, param.Name, "Parameter", idx, nil, node.Name, "unknown", false)
					sym.ScopePath = append([]int(nil), doc.TokenScopes[idx]...)
					if bodyOpen >= 0 {
						sym.ScopePath = append(sym.ScopePath, bodyOpen)
					}
					sym.Detail = "parameter"
					symbols = append(symbols, sym)
				}
			}
		}
		if node.Type == ast.ClassDecl || node.Type == ast.ComponentDecl || node.Type == ast.StructLit {
			for _, member := range node.Methods {
				if member == nil || member.Name == "" {
					continue
				}
				idx := s.findNameToken(doc, member.Name, node.Line, node.Col, -1)
				if idx >= 0 {
					kind := "Method"
					if member.Kind == "field" {
						kind = "Field"
					}
					container := node.Name
					if container == "" {
						container = "object"
					}
					symbols = append(symbols, s.makeSymbol(doc, member.Name, kind, idx, nil, container, inferNodeType(member.Init), false))
				}
			}
		}
		for _, member := range node.Members {
			if member == nil || member.Name == "" {
				continue
			}
			idx := s.findNameToken(doc, member.Name, node.Line, node.Col, -1)
			if idx >= 0 {
				symbols = append(symbols, s.makeSymbol(doc, member.Name, "EnumMember", idx, nil, node.Name, "unknown", false))
			}
		}
		if node.Type == ast.ImportDecl {
			if node.DefaultImport != "" {
				idx := s.findNameToken(doc, node.DefaultImport, node.Line, node.Col, -1)
				if idx >= 0 {
					symbols = append(symbols, s.makeSymbol(doc, node.DefaultImport, "Import", idx, node, "", "module", true))
				}
			}
			if node.Namespace != "" {
				idx := s.findNameToken(doc, node.Namespace, node.Line, node.Col, -1)
				if idx >= 0 {
					symbols = append(symbols, s.makeSymbol(doc, node.Namespace, "Import", idx, node, "", "module", true))
				}
			}
			for _, spec := range node.Specifiers {
				if spec == nil || spec.Local == "" {
					continue
				}
				idx := s.findNameToken(doc, spec.Local, node.Line, node.Col, -1)
				if idx >= 0 {
					symbols = append(symbols, s.makeSymbol(doc, spec.Local, "Import", idx, node, "", "unknown", true))
				}
			}
		}
		for _, child := range childNodes(node) {
			walk(child)
		}
	}
	walk(doc.AST)
	applyExportRules(doc, symbols)
	sort.SliceStable(symbols, func(i, j int) bool {
		if symbols[i].TokenIndex == symbols[j].TokenIndex {
			return symbols[i].Name < symbols[j].Name
		}
		return symbols[i].TokenIndex < symbols[j].TokenIndex
	})
	return deduplicateSymbols(symbols)
}

func applyExportRules(doc *Document, symbols []*Symbol) {
	if doc == nil || doc.AST == nil {
		return
	}
	explicit := false
	exported := map[string]bool{}
	seen := map[*ast.Node]bool{}
	var walk func(*ast.Node)
	walk = func(node *ast.Node) {
		if node == nil || seen[node] {
			return
		}
		seen[node] = true
		if node.Type == ast.ExportDecl {
			explicit = true
			if node.Declaration != nil {
				if name, _ := declarationInfo(node.Declaration); name != "" {
					exported[name] = true
				}
			}
			for _, spec := range node.Specifiers {
				if spec != nil && spec.Imported != "" {
					exported[spec.Imported] = true
				}
			}
			if node.Value != nil {
				exported["default"] = true
			}
		}
		for _, child := range childNodes(node) {
			walk(child)
		}
	}
	walk(doc.AST)
	for _, symbol := range symbols {
		if symbol.Kind == "Import" {
			symbol.Exported = false
			continue
		}
		if len(symbol.ScopePath) != 0 {
			symbol.Exported = false
			continue
		}
		if explicit {
			symbol.Exported = exported[symbol.Name]
		}
	}
}

func declarationInfo(n *ast.Node) (string, string) {
	if n == nil {
		return "", ""
	}
	switch n.Type {
	case ast.VarDecl, ast.ImmutableDecl:
		if n.Name != "" {
			return n.Name, "Variable"
		}
	case ast.FnDecl:
		return n.Name, "Function"
	case ast.ComponentDecl:
		return n.Name, "Component"
	case ast.ClassDecl:
		return n.Name, "Class"
	case ast.EnumDecl:
		return n.Name, "Enum"
	case ast.NamespaceDecl:
		return n.Name, "Namespace"
	}
	return "", ""
}

func (s *Server) makeSymbol(doc *Document, name, kind string, tokenIndex int, node *ast.Node, container, typ string, exported bool) *Symbol {
	r := tokenRange(doc, tokenIndex)
	detail := kind
	if container != "" {
		detail = container + "." + name
	}
	if node != nil {
		switch node.Type {
		case ast.FnDecl, ast.ComponentDecl:
			params := make([]string, 0, len(node.Params))
			for _, p := range node.Params {
				if p != nil {
					params = append(params, p.Name)
				}
			}
			detail = fmt.Sprintf("%s(%s)", name, strings.Join(params, ", "))
		case ast.VarDecl:
			if typ != "unknown" && typ != "" {
				detail = kind + " · " + typ
			}
		}
	}
	var path []int
	if tokenIndex >= 0 && tokenIndex < len(doc.TokenScopes) {
		path = append([]int(nil), doc.TokenScopes[tokenIndex]...)
	}
	isExported := exported && !strings.HasPrefix(name, "_") && kind != "Import"
	return &Symbol{Name: name, Kind: kind, Detail: detail, URI: doc.URI, Path: doc.Path, Range: r, Selection: r, TokenIndex: tokenIndex, ScopePath: path, Container: container, Node: node, Type: typ, Exported: isExported}
}

func (s *Server) findNameToken(doc *Document, name string, line, col, stop int) int {
	if name == "" {
		return -1
	}
	startOffset := 0
	for i, token := range doc.Tokens {
		if token.Line > line || token.Line == line && token.Col >= col {
			startOffset = i
			break
		}
	}
	end := len(doc.Tokens)
	if stop >= 0 && stop < end {
		end = stop
	}
	best := -1
	bestDistance := int(^uint(0) >> 1)
	for i := startOffset; i < end; i++ {
		t := doc.Tokens[i]
		if t.StrVal() != name || (t.Type != lexer.IDENTIFIER && t.Type != lexer.KEYWORD) {
			continue
		}
		distance := (t.Line-line)*100000 + t.Col - col
		if distance >= 0 && distance < bestDistance {
			best = i
			bestDistance = distance
		}
	}
	return best
}

func (s *Server) findParameterToken(doc *Document, name string, line, col, bodyOpen int) int {
	if name == "" || bodyOpen < 0 {
		return -1
	}
	start := 0
	for i, token := range doc.Tokens {
		if token.Line > line || token.Line == line && token.Col >= col {
			start = i
			break
		}
	}
	open := -1
	for i := start; i < bodyOpen; i++ {
		if doc.Tokens[i].StrVal() == "(" {
			open = i
			break
		}
	}
	if open < 0 {
		return -1
	}
	for i := open + 1; i < bodyOpen; i++ {
		token := doc.Tokens[i]
		if token.StrVal() == name && (token.Type == lexer.IDENTIFIER || token.Type == lexer.KEYWORD) {
			return i
		}
	}
	return -1
}

func (s *Server) findTokenAt(doc *Document, node *ast.Node) int {
	if node == nil {
		return -1
	}
	for i, token := range doc.Tokens {
		if token.Line == node.Line && token.Col == node.Col && token.StrVal() == "{" {
			return i
		}
	}
	return -1
}

func deduplicateSymbols(symbols []*Symbol) []*Symbol {
	seen := map[string]bool{}
	out := make([]*Symbol, 0, len(symbols))
	for _, symbol := range symbols {
		key := fmt.Sprintf("%s:%d:%d:%s", symbol.Name, symbol.Selection.Start.Line, symbol.Selection.Start.Character, symbol.Kind)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, symbol)
	}
	return out
}

func childNodes(n *ast.Node) []*ast.Node {
	value := reflect.ValueOf(n).Elem()
	var out []*ast.Node
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		if value.Type().Field(i).Name == "ScopeInfo" || value.Type().Field(i).Name == "ResolvedAddr" {
			continue
		}
		collectNodePointers(field, &out)
	}
	return out
}

func collectNodePointers(value reflect.Value, out *[]*ast.Node) {
	if !value.IsValid() {
		return
	}
	if value.Kind() == reflect.Interface {
		if !value.IsNil() {
			collectNodePointers(value.Elem(), out)
		}
		return
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return
		}
		if value.Type() == reflect.TypeOf((*ast.Node)(nil)) {
			*out = append(*out, value.Interface().(*ast.Node))
			return
		}
		collectNodePointers(value.Elem(), out)
		return
	}
	switch value.Kind() {
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if value.Field(i).CanInterface() {
				collectNodePointers(value.Field(i), out)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			collectNodePointers(value.Index(i), out)
		}
	case reflect.Map:
		iter := value.MapRange()
		for iter.Next() {
			collectNodePointers(iter.Value(), out)
		}
	}
}

func inferNodeType(n *ast.Node) string {
	if n == nil {
		return "unknown"
	}
	switch n.Type {
	case ast.StringLit, ast.TemplateLit:
		return "string"
	case ast.NumberLit:
		return "number"
	case ast.BoolLit:
		return "boolean"
	case ast.NullLit, ast.UndefinedLit:
		return "null"
	case ast.ArrayLit:
		return "array"
	case ast.ObjectLit, ast.StructLit:
		return "object"
	case ast.FnExpr, ast.ArrowFn:
		return "function"
	case ast.AtImportExpr:
		return "module:" + n.Source
	case ast.NewExpr:
		if n.Callee != nil && n.Callee.Name != "" {
			return "instance:" + n.Callee.Name
		}
		return "object"
	case ast.Identifier:
		return "identifier:" + n.Name
	case ast.CallExpr:
		return "call"
	}
	return "unknown"
}

func (s *Server) symbolAt(doc *Document, pos position) (*Symbol, int) {
	idx := tokenAtPosition(doc, pos)
	if idx < 0 {
		return nil, -1
	}
	name := doc.Tokens[idx].StrVal()
	if idx > 0 && doc.Tokens[idx-1].StrVal() == "." {
		return nil, idx
	}
	var best *Symbol
	bestDepth := -1
	for _, symbol := range doc.Symbols {
		if symbol.Name != name {
			continue
		}
		if symbol.TokenIndex == idx {
			return symbol, idx
		}
		if !prefixPath(symbol.ScopePath, doc.TokenScopes[idx]) {
			continue
		}
		if symbol.TokenIndex > idx && symbol.Kind != "Function" && symbol.Kind != "Class" && symbol.Kind != "Enum" && symbol.Kind != "Namespace" {
			continue
		}
		if len(symbol.ScopePath) > bestDepth {
			best = symbol
			bestDepth = len(symbol.ScopePath)
		}
	}
	return best, idx
}

func tokenAtPosition(doc *Document, pos position) int {
	best := -1
	for i := range doc.Tokens {
		r := tokenRange(doc, i)
		if pos.Line == r.Start.Line && pos.Character >= r.Start.Character && pos.Character <= r.End.Character {
			best = i
			break
		}
	}
	if best >= 0 {
		return best
	}
	return -1
}

func (s *Server) allSymbols() []*Symbol {
	var out []*Symbol
	for _, doc := range s.docs {
		out = append(out, doc.Symbols...)
	}
	return out
}

func runeLengthUTF16(text string) int {
	return len(utf16.Encode([]rune(text)))
}
