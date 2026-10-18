package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/checker"
	"lunex/internal/errfmt"
	"lunex/internal/lexer"
	"lunex/internal/parser"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

type Document struct {
	URI         string
	Path        string
	Text        string
	Version     int
	Open        bool
	Tokens      []lexer.Token
	AST         *ast.Node
	ParseErr    error
	Diagnostics []diagnostic
	Symbols     []*Symbol
	TokenScopes [][]int
	BracePairs  map[int]int
}

func (s *Server) didOpen(params map[string]json.RawMessage) {
	var event struct {
		TextDocument struct {
			URI        string `json:"uri"`
			Text       string `json:"text"`
			Version    int    `json:"version"`
			LanguageID string `json:"languageId"`
		} `json:"textDocument"`
	}
	decodeField(params, "textDocument", &event.TextDocument)
	if event.TextDocument.URI == "" {
		return
	}
	path := uriToPath(event.TextDocument.URI)
	if path == "" {
		return
	}
	doc := s.buildDocument(event.TextDocument.URI, path, event.TextDocument.Text, event.TextDocument.Version, true)
	s.docs[doc.URI] = doc
	s.reindexWorkspace()
	s.publish(doc)
}

func (s *Server) didChange(params map[string]json.RawMessage) {
	var event struct {
		TextDocument struct {
			URI     string `json:"uri"`
			Version int    `json:"version"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Range       *lspRange `json:"range"`
			RangeLength *int      `json:"rangeLength"`
			Text        string    `json:"text"`
		} `json:"contentChanges"`
	}
	decodeField(params, "textDocument", &event.TextDocument)
	decodeField(params, "contentChanges", &event.ContentChanges)
	doc := s.docs[event.TextDocument.URI]
	if doc == nil {
		path := uriToPath(event.TextDocument.URI)
		if path == "" {
			return
		}
		doc = s.buildDocument(event.TextDocument.URI, path, "", event.TextDocument.Version, true)
	}
	for _, change := range event.ContentChanges {
		if change.Range == nil {
			doc.Text = change.Text
			continue
		}
		start := offsetAt(doc.Text, change.Range.Start)
		end := offsetAt(doc.Text, change.Range.End)
		if end < start {
			end = start
		}
		doc.Text = doc.Text[:start] + change.Text + doc.Text[end:]
	}
	doc.Version = event.TextDocument.Version
	doc.Open = true
	s.refreshDocument(doc)
	s.docs[doc.URI] = doc
	s.reindexWorkspace()
	s.publish(doc)
}

func (s *Server) didSave(params map[string]json.RawMessage) {
	var event struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Text string `json:"text"`
	}
	decodeField(params, "textDocument", &event.TextDocument)
	decodeField(params, "text", &event.Text)
	if doc := s.docs[event.TextDocument.URI]; doc != nil {
		if event.Text != "" {
			doc.Text = event.Text
		}
		s.refreshDocument(doc)
		s.reindexWorkspace()
		s.publish(doc)
	}
}

func (s *Server) didClose(params map[string]json.RawMessage) {
	var event struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	decodeField(params, "textDocument", &event.TextDocument)
	doc := s.docs[event.TextDocument.URI]
	if doc == nil {
		return
	}
	doc.Open = false
	if content, err := os.ReadFile(doc.Path); err == nil {
		doc.Text = string(content)
		doc.Version = 0
	}
	s.refreshDocument(doc)
	s.publishDiagnostics(doc.URI, nil)
	s.reindexWorkspace()
}

func (s *Server) buildDocument(uri, path, text string, version int, open bool) *Document {
	doc := &Document{URI: uri, Path: path, Text: text, Version: version, Open: open}
	s.refreshDocument(doc)
	return doc
}

func (s *Server) refreshDocument(doc *Document) {
	tokens, lexErr := lexer.Tokenize(doc.Text, doc.Path)
	doc.Tokens = tokens
	doc.AST = nil
	doc.ParseErr = nil
	doc.Diagnostics = nil
	doc.Symbols = nil
	doc.BracePairs, doc.TokenScopes = buildTokenScopes(tokens)
	if lexErr != nil {
		doc.ParseErr = lexErr
		doc.Diagnostics = append(doc.Diagnostics, parseDiagnostic(lexErr, doc.Text))
		doc.AST = s.recoverDocumentAST(doc.Text, doc.Path, lexErr)
		if doc.AST != nil {
			doc.Symbols = s.collectSymbols(doc)
		}
		return
	}
	root, parseErr := parser.ParseWithLines(tokens, doc.Path, strings.Split(doc.Text, "\n"))
	doc.ParseErr = parseErr
	if parseErr != nil {
		doc.Diagnostics = append(doc.Diagnostics, parseDiagnostic(parseErr, doc.Text))
		doc.AST = s.recoverDocumentAST(doc.Text, doc.Path, parseErr)
		if doc.AST != nil {
			doc.Symbols = s.collectSymbols(doc)
		}
		return
	}
	doc.AST = root
	if doc.Open {
		ck := checker.New(checker.Options{Loader: s.moduleLoader, Warnings: true})
		result := ck.Check(doc.Text, doc.Path)
		for _, item := range result.Diagnostics {
			doc.Diagnostics = append(doc.Diagnostics, checkerDiagnostic(item, doc.Text))
		}
	}
	doc.Symbols = s.collectSymbols(doc)
}

func (s *Server) recoverDocumentAST(source, filename string, cause error) *ast.Node {
	lines := strings.Split(source, "\n")
	errorLine := len(lines)
	var le *errfmt.LunexError
	if errors.As(cause, &le) && le.Line > 0 {
		errorLine = le.Line
	}
	if errorLine > len(lines) {
		errorLine = len(lines)
	}
	if errorLine < 1 {
		errorLine = len(lines)
	}
	for count := errorLine - 1; count >= 0; count-- {
		prefix := strings.Join(lines[:count], "\n")
		if strings.TrimSpace(prefix) == "" {
			continue
		}
		prefixTokens, err := lexer.Tokenize(prefix, filename)
		if err != nil {
			continue
		}
		closures := closingDelimiters(prefixTokens)
		recovered := prefix
		if closures != "" {
			recovered += "\n" + closures
		}
		recoveredTokens, err := lexer.Tokenize(recovered, filename)
		if err != nil {
			continue
		}
		root, parseErr := parser.ParseWithLines(recoveredTokens, filename, strings.Split(recovered, "\n"))
		if parseErr == nil {
			return root
		}
	}
	return nil
}

func closingDelimiters(tokens []lexer.Token) string {
	stack := make([]string, 0)
	for _, token := range tokens {
		if token.Type != lexer.PUNCTUATION {
			continue
		}
		switch token.StrVal() {
		case "{", "(", "[":
			stack = append(stack, token.StrVal())
		case "}", ")", "]":
			if len(stack) == 0 {
				continue
			}
			open := stack[len(stack)-1]
			if open == "{" && token.StrVal() == "}" || open == "(" && token.StrVal() == ")" || open == "[" && token.StrVal() == "]" {
				stack = stack[:len(stack)-1]
			}
		}
	}
	var out strings.Builder
	for i := len(stack) - 1; i >= 0; i-- {
		switch stack[i] {
		case "{":
			out.WriteByte('}')
		case "(":
			out.WriteByte(')')
		case "[":
			out.WriteByte(']')
		}
	}
	return out.String()
}

func (s *Server) publish(doc *Document) {
	s.publishDiagnostics(doc.URI, doc.Diagnostics)
}

func (s *Server) publishDiagnostics(uri string, diagnostics []diagnostic) {
	s.notify("textDocument/publishDiagnostics", map[string]any{"uri": uri, "diagnostics": diagnostics})
}

func (s *Server) scanWorkspace() {
	count := 0
	const maxFiles = 25000
	for _, root := range s.roots {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if entry.IsDir() {
				if path != root && ignoredDirectory(entry.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.ToLower(filepath.Ext(path)) != ".lx" {
				return nil
			}
			count++
			if count > maxFiles {
				return filepath.SkipAll
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return nil
			}
			uri := pathToURI(abs)
			if current := s.docs[uri]; current != nil && current.Open {
				return nil
			}
			content, err := os.ReadFile(abs)
			if err != nil {
				return nil
			}
			s.docs[uri] = s.buildDocument(uri, abs, string(content), 0, false)
			return nil
		})
	}
	s.reindexWorkspace()
}

func ignoredDirectory(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".hg", ".svn", ".lunex", "node_modules", "vendor", "dist", "build", "target", "coverage", ".cache":
		return true
	default:
		return strings.HasPrefix(name, ".")
	}
}

func (s *Server) reindexWorkspace() {
	for _, doc := range s.docs {
		if doc.AST == nil {
			continue
		}
		for _, sym := range doc.Symbols {
			sym.URI = doc.URI
		}
	}
}

func parseDiagnostic(err error, source string) diagnostic {
	var le *errfmt.LunexError
	if errors.As(err, &le) {
		start := positionFromRuneColumn(source, le.Line, le.Col)
		end := position{Line: start.Line, Character: start.Character + 1}
		message := le.Message
		if le.Suggestion != "" {
			message += "\nSuggestion: " + le.Suggestion
		}
		return diagnostic{Range: lspRange{Start: start, End: end}, Severity: 1, Code: codeValue(le.Code), Source: "lunex", Message: message}
	}
	return diagnostic{Range: lspRange{Start: position{}, End: position{Line: 0, Character: 1}}, Severity: 1, Source: "lunex", Message: err.Error()}
}

func checkerDiagnostic(item checker.Diagnostic, source string) diagnostic {
	start := positionFromRuneColumn(source, item.Line, item.Col)
	end := position{Line: start.Line, Character: start.Character + 1}
	severity := 1

	message := item.Message
	if item.Suggestion != "" {
		message += "\nSuggestion: " + item.Suggestion
	}
	for _, note := range item.Notes {
		message += "\n" + note
	}
	return diagnostic{Range: lspRange{Start: start, End: end}, Severity: severity, Code: codeValue(item.Code), Source: "lunex", Message: message}
}

func codeValue(code string) any {
	if code == "" {
		return nil
	}
	return code
}

func (s *Server) moduleLoader(importPath, from string) (string, string, bool) {
	if importPath == "" || strings.HasPrefix(importPath, "std.") {
		return "", "", false
	}
	base := importPath
	if !filepath.IsAbs(base) {
		base = filepath.Join(filepath.Dir(from), filepath.FromSlash(importPath))
	}
	candidates := []string{base}
	if filepath.Ext(base) == "" {
		candidates = append(candidates, base+".lx", filepath.Join(base, "main.lx"))
	}
	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if info, err := os.Stat(abs); err == nil && !info.IsDir() {
			content, err := os.ReadFile(abs)
			if err == nil {
				return string(content), abs, true
			}
		}
	}
	return "", "", false
}

func uriToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		parsed, err := url.Parse(uri)
		if err == nil {
			path := parsed.Path
			if parsed.Host != "" && parsed.Host != "localhost" {
				path = "//" + parsed.Host + path
			}
			return filepath.FromSlash(path)
		}
	}
	return ""
}

func pathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func positionFromRuneColumn(source string, line, col int) position {
	if line < 1 {
		line = 1
	}
	lines := strings.Split(source, "\n")
	idx := line - 1
	if idx >= len(lines) {
		idx = len(lines) - 1
	}
	if idx < 0 {
		idx = 0
	}
	text := lines[idx]
	runes := []rune(text)
	cut := col - 1
	if cut < 0 {
		cut = 0
	}
	if cut > len(runes) {
		cut = len(runes)
	}
	character := len(utf16.Encode(runes[:cut]))
	return position{Line: idx, Character: character}
}

func offsetAt(text string, pos position) int {
	if pos.Line < 0 {
		return 0
	}
	line := 0
	byteOffset := 0
	for byteOffset < len(text) && line < pos.Line {
		if text[byteOffset] == '\n' {
			line++
		}
		byteOffset++
	}
	if line < pos.Line {
		return len(text)
	}
	remaining := text[byteOffset:]
	units := 0
	for i, r := range remaining {
		if r == '\n' {
			return byteOffset + i
		}
		u := len(utf16.Encode([]rune{r}))
		if units+u > pos.Character {
			return byteOffset + i
		}
		if units+u == pos.Character {
			return byteOffset + i + utf8.RuneLen(r)
		}
		units += u
	}
	return len(text)
}

func positionAt(text string, offset int) position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	prefix := text[:offset]
	line := strings.Count(prefix, "\n")
	last := strings.LastIndex(prefix, "\n")
	lineText := prefix[last+1:]
	character := len(utf16.Encode([]rune(lineText)))
	return position{Line: line, Character: character}
}

func buildTokenScopes(tokens []lexer.Token) (map[int]int, [][]int) {
	pairs := make(map[int]int)
	scopes := make([][]int, len(tokens))
	stack := make([]int, 0)
	for i, token := range tokens {
		scopes[i] = append([]int(nil), stack...)
		v := token.StrVal()
		if token.Type == lexer.PUNCTUATION && v == "{" {
			stack = append(stack, i)
		} else if token.Type == lexer.PUNCTUATION && v == "}" && len(stack) > 0 {
			open := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			pairs[open] = i
			pairs[i] = open
		}
	}
	return pairs, scopes
}

func prefixPath(prefix, path []int) bool {
	if len(prefix) > len(path) {
		return false
	}
	for i := range prefix {
		if prefix[i] != path[i] {
			return false
		}
	}
	return true
}

func clampRange(text string, r lspRange) lspRange {
	start := offsetAt(text, r.Start)
	end := offsetAt(text, r.End)
	if end < start {
		end = start
	}
	return lspRange{Start: positionAt(text, start), End: positionAt(text, end)}
}

func lineAt(text string, line int) string {
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	return strings.TrimRightFunc(lines[line], func(r rune) bool { return r == '\r' || unicode.IsControl(r) && r != '\t' })
}

func isIdentifierStart(r rune) bool {
	return r == '_' || r == '$' || unicode.IsLetter(r)
}

func isIdentifierPart(r rune) bool {
	return isIdentifierStart(r) || unicode.IsDigit(r)
}

func tokenRange(doc *Document, index int) lspRange {
	if index < 0 || index >= len(doc.Tokens) {
		p := position{}
		return lspRange{Start: p, End: p}
	}
	t := doc.Tokens[index]
	start := positionFromRuneColumn(doc.Text, t.Line, t.Col)
	length := len(utf16.Encode([]rune(t.StrVal())))
	if length == 0 {
		length = 1
	}
	return lspRange{Start: start, End: position{Line: start.Line, Character: start.Character + length}}
}

func (s *Server) docForURI(uri string) *Document {
	if doc := s.docs[uri]; doc != nil {
		return doc
	}
	path := uriToPath(uri)
	if path == "" {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	doc := s.buildDocument(uri, path, string(content), 0, false)
	s.docs[uri] = doc
	return doc
}

func (s *Server) ensureDoc(uri string) *Document {
	if doc := s.docForURI(uri); doc != nil {
		return doc
	}
	return nil
}

func (s *Server) reparseAffected() {
	for _, doc := range s.docs {
		if doc.Open {
			s.refreshDocument(doc)
			s.publish(doc)
		}
	}
}

func validFileURI(uri string) bool {
	return strings.HasPrefix(uri, "file://") && filepath.Ext(uriToPath(uri)) == ".lx"
}

func stableDocuments(docs map[string]*Document) []*Document {
	out := make([]*Document, 0, len(docs))
	for _, doc := range docs {
		if doc != nil {
			out = append(out, doc)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func (d *Document) String() string {
	return fmt.Sprintf("%s@%d", d.URI, d.Version)
}
