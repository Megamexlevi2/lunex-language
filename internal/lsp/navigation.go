package lsp

import (
	"encoding/json"
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/lexer"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func (s *Server) hover(params map[string]json.RawMessage) any {
	var req struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position position `json:"position"`
	}
	decodeField(params, "textDocument", &req.TextDocument)
	decodeField(params, "position", &req.Position)
	doc := s.ensureDoc(req.TextDocument.URI)
	if doc == nil {
		return nil
	}
	symbol, index := s.symbolAt(doc, req.Position)
	if symbol != nil {
		markdown := "```lunex\n" + symbol.Detail + "\n```"
		if symbol.Type != "" && symbol.Type != "unknown" {
			markdown += "\n\nType: `" + symbol.Type + "`"
		}
		if symbol.Node != nil && symbol.Node.Type == ast.FnDecl {
			markdown += "\n\nLunex function declared in `" + filepath.Base(symbol.Path) + "`."
		}
		return map[string]any{"contents": map[string]any{"kind": "markdown", "value": markdown}, "range": symbol.Selection}
	}
	if index >= 0 && index < len(doc.Tokens) {
		token := doc.Tokens[index]
		value := token.StrVal()
		if token.Type == lexer.KEYWORD {
			return map[string]any{"contents": map[string]any{"kind": "markdown", "value": "Lunex keyword `" + value + "`."}, "range": tokenRange(doc, index)}
		}
		if token.Type == lexer.STRING || token.Type == lexer.NUMBER {
			return map[string]any{"contents": map[string]any{"kind": "markdown", "value": fmt.Sprintf("Lunex %s literal `%s`.", strings.ToLower(string(token.Type)), value)}, "range": tokenRange(doc, index)}
		}
	}
	return nil
}

func (s *Server) definition(params map[string]json.RawMessage) any {
	var req struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position position `json:"position"`
	}
	decodeField(params, "textDocument", &req.TextDocument)
	decodeField(params, "position", &req.Position)
	doc := s.ensureDoc(req.TextDocument.URI)
	if doc == nil {
		return []location{}
	}
	symbol, index := s.symbolAt(doc, req.Position)
	if symbol != nil && symbol.TokenIndex >= 0 {
		return location{URI: symbol.URI, Range: symbol.Selection}
	}
	if index < 0 || index >= len(doc.Tokens) {
		return []location{}
	}
	name := doc.Tokens[index].StrVal()
	if index > 0 && doc.Tokens[index-1].StrVal() == "." {
		return []location{}
	}
	for _, candidate := range s.allSymbols() {
		if candidate.Name == name && candidate.Exported && candidate.TokenIndex >= 0 {
			return location{URI: candidate.URI, Range: candidate.Selection}
		}
	}
	return []location{}
}

func (s *Server) targetAt(doc *Document, pos position) (*Symbol, int) {
	symbol, idx := s.symbolAt(doc, pos)
	if symbol != nil {
		return symbol, idx
	}
	if idx < 0 || idx >= len(doc.Tokens) {
		return nil, idx
	}
	name := doc.Tokens[idx].StrVal()
	if idx > 0 && doc.Tokens[idx-1].StrVal() == "." {
		return nil, idx
	}
	var best *Symbol
	for _, candidate := range doc.Symbols {
		if candidate.Name == name && candidate.Exported && candidate.ScopePath == nil {
			best = candidate
			break
		}
	}
	if best != nil {
		return best, idx
	}
	for _, candidate := range s.allSymbols() {
		if candidate.Name == name && candidate.Exported && candidate.ScopePath == nil {
			return candidate, idx
		}
	}
	return nil, idx
}

func (s *Server) references(params map[string]json.RawMessage) any {
	var req struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position position `json:"position"`
		Context  struct {
			IncludeDeclaration bool `json:"includeDeclaration"`
		} `json:"context"`
	}
	decodeField(params, "textDocument", &req.TextDocument)
	decodeField(params, "position", &req.Position)
	decodeField(params, "context", &req.Context)
	doc := s.ensureDoc(req.TextDocument.URI)
	if doc == nil {
		return []location{}
	}
	target, idx := s.targetAt(doc, req.Position)
	if idx < 0 || idx >= len(doc.Tokens) {
		return []location{}
	}
	if target == nil && idx > 0 && doc.Tokens[idx-1].StrVal() == "." {
		return []location{}
	}
	name := doc.Tokens[idx].StrVal()
	if target != nil {
		name = target.Name
	}
	var out []location
	for _, candidateDoc := range s.referenceDocuments(doc, target) {
		for tokenIndex, token := range candidateDoc.Tokens {
			if token.StrVal() != name || token.Type != lexer.IDENTIFIER && token.Type != lexer.KEYWORD {
				continue
			}
			if tokenIndex > 0 && candidateDoc.Tokens[tokenIndex-1].StrVal() == "." {
				continue
			}
			if !req.Context.IncludeDeclaration && target != nil && candidateDoc.URI == target.URI && tokenIndex == target.TokenIndex {
				continue
			}
			out = append(out, location{URI: candidateDoc.URI, Range: tokenRange(candidateDoc, tokenIndex)})
		}
	}
	return deduplicateLocations(out)
}

func (s *Server) referenceDocuments(current *Document, target *Symbol) []*Document {
	if target == nil || target.ScopePath != nil && len(target.ScopePath) > 0 {
		return []*Document{current}
	}
	out := map[string]*Document{current.URI: current}
	if target.URI != "" {
		if doc := s.docs[target.URI]; doc != nil {
			out[doc.URI] = doc
		}
	}
	for _, doc := range s.docs {
		if doc.URI == target.URI || doc.AST == nil {
			continue
		}
		if importsTarget(doc, target.Path) {
			out[doc.URI] = doc
		}
	}
	return stableDocuments(out)
}

func importsTarget(doc *Document, targetPath string) bool {
	if doc == nil || doc.AST == nil || targetPath == "" {
		return false
	}
	found := false
	var walk func(*ast.Node)
	seen := map[*ast.Node]bool{}
	walk = func(node *ast.Node) {
		if node == nil || seen[node] || found {
			return
		}
		seen[node] = true
		if node.Type == ast.ImportDecl || node.Type == ast.AtImportExpr || node.Type == ast.NaxImportExpr {
			for _, candidate := range possibleImportPaths(node.Source, doc.Path) {
				if candidate == targetPath {
					found = true
					return
				}
			}
		}
		for _, child := range childNodes(node) {
			walk(child)
		}
	}
	walk(doc.AST)
	return found
}

func possibleImportPaths(source, from string) []string {
	if source == "" || strings.HasPrefix(source, "std.") {
		return nil
	}
	base := source
	if !filepath.IsAbs(base) {
		base = filepath.Join(filepath.Dir(from), filepath.FromSlash(source))
	}
	candidates := []string{base}
	if filepath.Ext(base) == "" {
		candidates = append(candidates, base+".lx", filepath.Join(base, "main.lx"))
	}
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if abs, err := filepath.Abs(candidate); err == nil {
			out = append(out, abs)
		}
	}
	return out
}

func (s *Server) prepareRename(params map[string]json.RawMessage) any {
	var req struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position position `json:"position"`
	}
	decodeField(params, "textDocument", &req.TextDocument)
	decodeField(params, "position", &req.Position)
	doc := s.ensureDoc(req.TextDocument.URI)
	if doc == nil {
		return nil
	}
	symbol, index := s.targetAt(doc, req.Position)
	if index < 0 || index >= len(doc.Tokens) || symbol == nil || symbol.Name == "" {
		return nil
	}
	if strings.HasPrefix(symbol.Name, "_") && symbol.Kind == "Import" {
		return nil
	}
	return map[string]any{"range": tokenRange(doc, index), "placeholder": symbol.Name}
}

func (s *Server) rename(params map[string]json.RawMessage) any {
	var req struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position position `json:"position"`
		NewName  string   `json:"newName"`
	}
	decodeField(params, "textDocument", &req.TextDocument)
	decodeField(params, "position", &req.Position)
	decodeField(params, "newName", &req.NewName)
	if !validIdentifier(req.NewName) {
		return nil
	}
	doc := s.ensureDoc(req.TextDocument.URI)
	if doc == nil {
		return nil
	}
	target, index := s.targetAt(doc, req.Position)
	if index < 0 || target == nil || index >= len(doc.Tokens) {
		return nil
	}
	if target.Name == req.NewName {
		return workspaceEdit{Changes: map[string][]textEdit{}}
	}
	changes := map[string][]textEdit{}
	for _, candidateDoc := range s.referenceDocuments(doc, target) {
		for i, token := range candidateDoc.Tokens {
			if token.StrVal() != target.Name || token.Type != lexer.IDENTIFIER && token.Type != lexer.KEYWORD {
				continue
			}
			if i > 0 && candidateDoc.Tokens[i-1].StrVal() == "." {
				continue
			}
			if !tokenInTargetScope(candidateDoc, i, target) {
				continue
			}
			changes[candidateDoc.URI] = append(changes[candidateDoc.URI], textEdit{Range: tokenRange(candidateDoc, i), NewText: req.NewName})
		}
	}
	return workspaceEdit{Changes: changes}
}

func tokenInTargetScope(doc *Document, tokenIndex int, target *Symbol) bool {
	if target.ScopePath == nil || len(target.ScopePath) == 0 {
		return true
	}
	if doc.URI != target.URI || tokenIndex >= len(doc.TokenScopes) {
		return false
	}
	return prefixPath(target.ScopePath, doc.TokenScopes[tokenIndex])
}

var reservedIdentifiers = map[string]bool{
	"undefined": true, "null": true, "true": true, "false": true, "val": true, "var": true, "const": true,
	"fn": true, "if": true, "else": true, "elif": true, "while": true, "for": true, "break": true,
	"continue": true, "do": true, "each": true, "match": true, "case": true, "try": true, "catch": true,
	"finally": true, "raise": true, "throw": true, "return": true, "new": true, "this": true, "super": true,
	"extends": true, "typeof": true, "instanceof": true, "import": true, "export": true, "require": true,
	"select": true, "macro": true, "immutable": true, "freeze": true, "with": true, "using": true,
	"assert": true, "delete": true, "use": true, "struct": true, "defer": true, "guard": true,
	"loop": true, "repeat": true, "when": true, "nax": true, "lunex": true, "sleep": true, "range": true,
	"ifhave": true, "ifset": true, "between": true, "matches": true, "startsWith": true, "endsWith": true,
	"unless": true, "watch": true,
}

func validIdentifier(name string) bool {
	if name == "" || reservedIdentifiers[name] {
		return false
	}
	tokens, err := lexer.Tokenize(name, "")
	if err != nil || len(tokens) < 2 {
		return false
	}
	return tokens[0].Type == lexer.IDENTIFIER && tokens[0].StrVal() == name && tokens[1].Type == lexer.EOF
}

func (s *Server) documentSymbols(params map[string]json.RawMessage) any {
	var req struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	decodeField(params, "textDocument", &req.TextDocument)
	doc := s.ensureDoc(req.TextDocument.URI)
	if doc == nil {
		return []documentSymbol{}
	}
	var out []documentSymbol
	classChildren := map[string][]documentSymbol{}
	for _, symbol := range doc.Symbols {
		if symbol.Container != "" && (symbol.Kind == "Method" || symbol.Kind == "Field" || symbol.Kind == "EnumMember") {
			classChildren[symbol.Container] = append(classChildren[symbol.Container], documentSymbol{Name: symbol.Name, Detail: symbol.Detail, Kind: documentSymbolKind(symbol.Kind), Range: fullSymbolRange(doc, symbol), SelectionRange: symbol.Selection})
		}
	}
	for _, symbol := range doc.Symbols {
		if len(symbol.ScopePath) != 0 || symbol.Kind == "Parameter" || symbol.Kind == "Method" || symbol.Kind == "Field" || symbol.Kind == "EnumMember" {
			continue
		}
		item := documentSymbol{Name: symbol.Name, Detail: symbol.Detail, Kind: documentSymbolKind(symbol.Kind), Range: fullSymbolRange(doc, symbol), SelectionRange: symbol.Selection, Children: classChildren[symbol.Name]}
		out = append(out, item)
	}
	return out
}

func fullSymbolRange(doc *Document, symbol *Symbol) lspRange {
	if symbol.Node == nil || symbol.TokenIndex < 0 {
		return symbol.Range
	}
	start := symbol.Range.Start
	end := symbol.Range.End
	if symbol.Node.Body != nil {
		open := -1
		for i, token := range doc.Tokens {
			if token.Line == symbol.Node.Body.Line && token.Col == symbol.Node.Body.Col && token.StrVal() == "{" {
				open = i
				break
			}
		}
		if open >= 0 {
			if close, ok := doc.BracePairs[open]; ok {
				end = tokenRange(doc, close).End
			}
		}
	}
	if end.Line < start.Line || end.Line == start.Line && end.Character < start.Character {
		end = start
	}
	return lspRange{Start: start, End: end}
}

func documentSymbolKind(kind string) int {
	switch kind {
	case "Function", "Component":
		return 12
	case "Variable":
		return 13
	case "Class":
		return 5
	case "Enum":
		return 10
	case "Namespace", "Import":
		return 3
	case "Method":
		return 6
	case "Field":
		return 8
	case "EnumMember":
		return 22
	case "Parameter":
		return 13
	default:
		return 13
	}
}

func (s *Server) workspaceSymbols(params map[string]json.RawMessage) any {
	var req struct {
		Query string `json:"query"`
	}
	decodeField(params, "query", &req.Query)
	query := strings.ToLower(req.Query)
	var out []map[string]any
	for _, doc := range stableDocuments(s.docs) {
		for _, symbol := range doc.Symbols {
			if len(symbol.ScopePath) != 0 || symbol.Kind == "Parameter" || symbol.Name == "" || !strings.Contains(strings.ToLower(symbol.Name), query) {
				continue
			}
			out = append(out, map[string]any{"name": symbol.Name, "kind": documentSymbolKind(symbol.Kind), "location": location{URI: doc.URI, Range: symbol.Selection}, "containerName": filepath.Base(doc.Path), "data": map[string]any{"detail": symbol.Detail}})
			if len(out) >= 5000 {
				return out
			}
		}
	}
	return out
}

func (s *Server) documentHighlights(params map[string]json.RawMessage) any {
	var req struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position position `json:"position"`
	}
	decodeField(params, "textDocument", &req.TextDocument)
	decodeField(params, "position", &req.Position)
	doc := s.ensureDoc(req.TextDocument.URI)
	if doc == nil {
		return []map[string]any{}
	}
	target, index := s.targetAt(doc, req.Position)
	if index < 0 || index >= len(doc.Tokens) {
		return []map[string]any{}
	}
	name := doc.Tokens[index].StrVal()
	if target != nil {
		name = target.Name
	}
	var out []map[string]any
	for i, token := range doc.Tokens {
		if token.StrVal() != name || token.Type != lexer.IDENTIFIER && token.Type != lexer.KEYWORD {
			continue
		}
		if i > 0 && doc.Tokens[i-1].StrVal() == "." {
			continue
		}
		if target != nil {
			resolved, _ := s.symbolAt(doc, tokenRange(doc, i).Start)
			if resolved == nil || resolved.URI != target.URI || resolved.TokenIndex != target.TokenIndex {
				continue
			}
		}
		kind := 2
		if target != nil && i == target.TokenIndex {
			kind = 3
		}
		out = append(out, map[string]any{"range": tokenRange(doc, i), "kind": kind})
	}
	return out
}

func deduplicateLocations(items []location) []location {
	seen := map[string]bool{}
	out := make([]location, 0, len(items))
	for _, item := range items {
		key := fmt.Sprintf("%s:%d:%d", item.URI, item.Range.Start.Line, item.Range.Start.Character)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].URI == out[j].URI {
			if out[i].Range.Start.Line == out[j].Range.Start.Line {
				return out[i].Range.Start.Character < out[j].Range.Start.Character
			}
			return out[i].Range.Start.Line < out[j].Range.Start.Line
		}
		return out[i].URI < out[j].URI
	})
	return out
}

var suggestionPattern = regexp.MustCompile("did you mean `([^`]+)`")

func (s *Server) codeActions(params map[string]json.RawMessage) any {
	var req struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Range   lspRange `json:"range"`
		Context struct {
			Diagnostics []diagnostic `json:"diagnostics"`
		} `json:"context"`
	}
	decodeField(params, "textDocument", &req.TextDocument)
	decodeField(params, "range", &req.Range)
	decodeField(params, "context", &req.Context)
	doc := s.ensureDoc(req.TextDocument.URI)
	if doc == nil {
		return []codeAction{}
	}
	var out []codeAction
	for _, diag := range req.Context.Diagnostics {
		match := suggestionPattern.FindStringSubmatch(diag.Message)
		if len(match) != 2 {
			continue
		}
		start := offsetAt(doc.Text, diag.Range.Start)
		end := offsetAt(doc.Text, diag.Range.End)
		if end < start {
			end = start
		}
		out = append(out, codeAction{Title: "Replace with " + match[1], Kind: "quickfix", Diagnostics: []diagnostic{diag}, IsPreferred: true, Edit: &workspaceEdit{Changes: map[string][]textEdit{doc.URI: {{Range: lspRange{Start: positionAt(doc.Text, start), End: positionAt(doc.Text, end)}, NewText: match[1]}}}}})
	}
	return out
}

func (s *Server) foldingRanges(params map[string]json.RawMessage) any {
	var req struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	decodeField(params, "textDocument", &req.TextDocument)
	doc := s.ensureDoc(req.TextDocument.URI)
	if doc == nil {
		return []map[string]any{}
	}
	var out []map[string]any
	for open, close := range doc.BracePairs {
		if close <= open {
			continue
		}
		start := doc.Tokens[open].Line - 1
		end := doc.Tokens[close].Line - 1
		if end > start {
			out = append(out, map[string]any{"startLine": start, "endLine": end, "kind": "region"})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a := out[i]["startLine"].(int)
		b := out[j]["startLine"].(int)
		if a == b {
			return out[i]["endLine"].(int) < out[j]["endLine"].(int)
		}
		return a < b
	})
	return out
}

func (s *Server) fileDefinitionPath(source, from string) string {
	for _, candidate := range possibleImportPaths(source, from) {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func (s *Server) typeDefinition(params map[string]json.RawMessage) any {
	var req struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position position `json:"position"`
	}
	decodeField(params, "textDocument", &req.TextDocument)
	decodeField(params, "position", &req.Position)
	doc := s.ensureDoc(req.TextDocument.URI)
	if doc == nil {
		return []location{}
	}
	symbol, _ := s.targetAt(doc, req.Position)
	if symbol == nil {
		return []location{}
	}
	typeName := strings.TrimPrefix(symbol.Type, "instance:")
	if typeName == symbol.Type && symbol.Node != nil && symbol.Node.TypeAnn != nil {
		typeName = fmt.Sprint(symbol.Node.TypeAnn)
	}
	if typeName == "" || typeName == "unknown" || typeName == symbol.Type {
		return []location{}
	}
	for _, candidate := range s.allSymbols() {
		if candidate.Name == typeName && (candidate.Kind == "Class" || candidate.Kind == "Enum" || candidate.Kind == "Namespace" || candidate.Kind == "Variable") {
			return location{URI: candidate.URI, Range: candidate.Selection}
		}
	}
	return []location{}
}
