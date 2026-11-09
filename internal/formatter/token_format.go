package formatter

import (
	"fmt"
	"strings"

	"lunex/internal/ast"
	"lunex/internal/lexer"
)

type formatMetadata struct {
	statementStarts map[string]ast.NodeType
	blockStarts     map[string]bool
	objectStarts    map[string]bool
	arrayStarts     map[string]bool
}

type tokenFrame struct {
	kind     string
	multi    bool
	inline   bool
	endIndex int
}

type tokenWriter struct {
	buf        strings.Builder
	indent     int
	pending    bool
	lineStart  bool
	lineHas    bool
	lineEnding string
}

func newTokenWriter(lineEnding string) *tokenWriter {
	return &tokenWriter{lineStart: true, lineEnding: lineEnding}
}

func (w *tokenWriter) space() {
	if w.lineHas {
		w.pending = true
	}
}

func (w *tokenWriter) newline() {
	w.pending = false
	if w.buf.Len() == 0 {
		w.lineStart = true
		w.lineHas = false
		return
	}
	last := w.lastByte()
	if last != '\n' {
		w.buf.WriteString(w.lineEnding)
	}
	w.lineStart = true
	w.lineHas = false
}

func (w *tokenWriter) blankline() {
	w.newline()
	if w.buf.Len() > 0 && !strings.HasSuffix(w.buf.String(), w.lineEnding+w.lineEnding) {
		w.buf.WriteString(w.lineEnding)
	}
	w.lineStart = true
	w.lineHas = false
}

func (w *tokenWriter) lastByte() byte {
	s := w.buf.String()
	if len(s) == 0 {
		return 0
	}
	return s[len(s)-1]
}

func (w *tokenWriter) write(value string) {
	if value == "" {
		return
	}
	if w.pending && !w.lineStart {
		w.buf.WriteByte(' ')
		w.pending = false
	}
	if w.lineStart && value[0] != '\n' && value[0] != '\r' {
		w.buf.WriteString(strings.Repeat("  ", maxInt(w.indent, 0)))
		w.lineStart = false
	}
	w.buf.WriteString(value)
	if offset := strings.LastIndexAny(value, "\n\r"); offset >= 0 {
		suffix := value[offset+1:]
		w.lineStart = len(suffix) == 0
		w.lineHas = strings.Trim(suffix, " \t\r") != ""
		w.pending = false
	} else if strings.Trim(value, " \t") != "" {
		w.lineHas = true
	}
}

func (w *tokenWriter) result() string {
	return strings.TrimRight(w.buf.String(), " \t\r\n") + w.lineEnding
}

func formatTokenStream(source string, tokens []lexer.Token, root *ast.Node) string {
	items := make([]lexer.Token, 0, len(tokens))
	for _, token := range tokens {
		if token.Type != lexer.EOF {
			items = append(items, token)
		}
	}
	if len(items) == 0 {
		if source == "" {
			return ""
		}
		return "\n"
	}
	metadata := collectFormatMetadata(root)
	pairs := matchingTokens(items)
	frames := make([]tokenFrame, 0, 16)
	lineEnding := "\n"
	if strings.Contains(source, "\r\n") {
		lineEnding = "\r\n"
	}
	writer := newTokenWriter(lineEnding)
	var previous *lexer.Token
	var previousTopKind ast.NodeType
	hasTopStatement := false
	for i := range items {
		token := items[i]
		raw := token.Raw
		key := locationKey(token.Line, token.Col)
		if token.Type == lexer.COMMENT {
			if previous != nil && token.Line > previous.Line && !writer.lineStart {
				writer.newline()
			} else if writer.lineHas {
				writer.space()
			}
			writer.write(raw)
			if strings.HasPrefix(raw, "/!") || strings.Contains(raw, "\n") || strings.Contains(raw, "\r") {
				writer.newline()
			} else {
				writer.space()
			}
			previous = &items[i]
			continue
		}
		if kind, ok := metadata.statementStarts[key]; ok {
			isClause := raw == "else" || raw == "catch" || raw == "finally" || (raw == "if" && previous != nil && previous.Raw == "else")
			if len(frames) == 0 {
				if hasTopStatement && (kind == ast.FnDecl || kind == ast.ClassDecl || previousTopKind == ast.FnDecl || previousTopKind == ast.ClassDecl) {
					writer.blankline()
				}
				previousTopKind = kind
				hasTopStatement = true
			} else if previous != nil && !writer.lineStart && !isClause && activeLayoutFrame(frames) {
				writer.newline()
			}
		}
		if previous != nil {
			if shouldBreakBefore(items, i, frames, metadata, writer) {
				writer.newline()
			} else if shouldAddSpace(items, i, frames, metadata, pairs) {
				writer.space()
			}
		}
		if token.Type == lexer.PUNCTUATION {
			switch raw {
			case "{":
				frame := makeTokenFrame(items, i, pairs, metadata)
				if frame.kind == "block" || frame.kind == "struct" {
					if i > 0 && !writer.lineStart {
						writer.space()
					}
					writer.write(raw)
					if frame.inline {
						frames = append(frames, frame)
					} else {
						frames = append(frames, frame)
						writer.indent++
						writer.newline()
					}
					previous = &items[i]
					continue
				}
				writer.write(raw)
				frames = append(frames, frame)
				if frame.multi && !frame.inline {
					writer.indent++
					writer.newline()
				} else if !frame.inline && pairs[i] > i+1 {
					writer.space()
				}
				previous = &items[i]
				continue
			case "[", "(":
				frame := makeTokenFrame(items, i, pairs, metadata)
				writer.write(raw)
				frames = append(frames, frame)
				if frame.multi && !frame.inline {
					writer.indent++
					writer.newline()
				}
				previous = &items[i]
				continue
			case "}":
				frame := takeFrame(frames)
				if len(frames) > 0 {
					frames = frames[:len(frames)-1]
				}
				if !frame.inline {
					if frame.kind == "block" || frame.kind == "struct" || frame.multi {
						if !writer.lineStart {
							writer.newline()
						}
						writer.indent--
						if writer.indent < 0 {
							writer.indent = 0
						}
					} else if frame.kind == "object" {
						writer.space()
					}
				}
				writer.write(raw)
				if i+1 < len(items) {
					next := items[i+1].Raw
					if next == "else" || next == "catch" || next == "finally" {
						writer.space()
					} else if next == ")" || next == "]" || next == "," || next == "." || next == "?." || next == "(" || next == "[" || next == "=" || next == "=>" {
						writer.space()
					} else if frame.kind == "block" || frame.kind == "struct" {
						writer.newline()
					}
				}
				previous = &items[i]
				continue
			case ")", "]":
				frame := takeFrame(frames)
				frames = frames[:len(frames)-1]
				if frame.multi && !frame.inline {
					if !writer.lineStart {
						writer.newline()
					}
					writer.indent--
					if writer.indent < 0 {
						writer.indent = 0
					}
				}
				writer.write(raw)
				previous = &items[i]
				continue
			case ",":
				writer.write(raw)
				if len(frames) > 0 && frames[len(frames)-1].multi && !frames[len(frames)-1].inline {
					writer.newline()
				} else {
					writer.space()
				}
				previous = &items[i]
				continue
			case ";":
				writer.write(raw)
				if !insideParentheses(frames) {
					writer.newline()
				} else {
					writer.space()
				}
				previous = &items[i]
				continue
			case ":":
				if !insideObject(frames) {
					writer.space()
				}
				writer.write(raw)
				writer.space()
				previous = &items[i]
				continue
			}
		}
		writer.write(raw)
		if raw == ":" || (isBinaryOperator(token) && !isUnaryOperator(items, i)) {
			writer.space()
		}
		previous = &items[i]
	}
	formatted := writer.result()
	return formatted
}

func collectFormatMetadata(root *ast.Node) formatMetadata {
	meta := formatMetadata{
		statementStarts: map[string]ast.NodeType{},
		blockStarts:     map[string]bool{},
		objectStarts:    map[string]bool{},
		arrayStarts:     map[string]bool{},
	}
	seen := map[*ast.Node]bool{}
	var visit func(*ast.Node)
	visit = func(n *ast.Node) {
		if n == nil || seen[n] {
			return
		}
		seen[n] = true
		key := locationKey(n.Line, n.Col)
		switch n.Type {
		case ast.Block:
			meta.blockStarts[key] = true
		case ast.ObjectLit:
			meta.objectStarts[key] = true
		case ast.ArrayLit:
			meta.arrayStarts[key] = true
		}
		if n.Type == ast.Program || n.Type == ast.Block || n.Type == ast.StructLit {
			for _, stmt := range n.Body_ {
				if stmt != nil {
					meta.statementStarts[locationKey(stmt.Line, stmt.Col)] = stmt.Type
				}
			}
		}
		children := []*ast.Node{
			n.Body, n.Init, n.Test, n.Alternate, n.Consequent, n.Left, n.Right, n.Object, n.Callee, n.Arg,
			n.Expr, n.Stmt, n.Subject, n.Count, n.Ms, n.Guard, n.Declaration, n.Extends, n.SuperClass,
			n.CatchBlock, n.FinallyBlock, n.Lo, n.Hi,
		}
		for _, child := range children {
			visit(child)
		}
		for _, list := range [][]*ast.Node{n.Args, n.Elements, n.Body_, n.Decorators, n.Exprs} {
			for _, child := range list {
				visit(child)
			}
		}
		for _, property := range n.Properties {
			if property == nil {
				continue
			}
			visit(property.Value)
			visit(property.Arg)
			visit(property.Body)
			if keyNode, ok := property.Key.(*ast.Node); ok {
				visit(keyNode)
			}
			for _, param := range property.Params {
				if param != nil {
					visit(param.DefaultVal)
				}
			}
		}
		for _, method := range n.Methods {
			if method != nil {
				visit(method.Body)
				visit(method.Init)
				for _, param := range method.Params {
					if param != nil {
						visit(param.DefaultVal)
					}
				}
			}
		}
		for _, item := range n.Cases {
			if item != nil {
				visit(item.Guard)
				visit(item.Body)
			}
		}
		for _, item := range n.SelectCases {
			if item != nil {
				visit(item.Body)
				visit(item.Expr)
			}
		}
		for _, param := range n.Params {
			if param != nil {
				visit(param.DefaultVal)
			}
		}
	}
	visit(root)
	return meta
}

func matchingTokens(tokens []lexer.Token) map[int]int {
	pairs := map[int]int{}
	type delimiter struct {
		raw   string
		index int
	}
	stack := make([]delimiter, 0, 16)
	for i, token := range tokens {
		if token.Type == lexer.COMMENT {
			continue
		}
		raw := token.Raw
		switch raw {
		case "{", "[", "(":
			stack = append(stack, delimiter{raw: raw, index: i})
		case "}", "]", ")":
			want := map[string]string{"}": "{", "]": "[", ")": "("}[raw]
			for j := len(stack) - 1; j >= 0; j-- {
				if stack[j].raw == want {
					pairs[stack[j].index] = i
					pairs[i] = stack[j].index
					stack = stack[:j]
					break
				}
			}
		}
	}
	return pairs
}

func makeTokenFrame(tokens []lexer.Token, index int, pairs map[int]int, meta formatMetadata) tokenFrame {
	token := tokens[index]
	frame := tokenFrame{endIndex: pairs[index]}
	if frame.endIndex <= index {
		frame.endIndex = index
	}
	key := locationKey(token.Line, token.Col)
	switch token.Raw {
	case "{":
		frame.kind = braceKind(tokens, index, meta)
	case "[":
		if meta.arrayStarts[key] {
			frame.kind = "array"
		} else {
			frame.kind = "index"
		}
	case "(":
		frame.kind = "group"
	}
	if frame.endIndex == index+1 {
		frame.inline = true
		return frame
	}
	frame.multi = shouldFormatMultiline(tokens, index, frame.endIndex, pairs, meta)
	if frame.kind == "block" || frame.kind == "struct" {
		frame.multi = true
		frame.inline = frame.endIndex == index+1
	}
	return frame
}

func shouldFormatMultiline(tokens []lexer.Token, start, end int, pairs map[int]int, meta formatMetadata) bool {
	if end <= start+1 || end >= len(tokens) {
		return false
	}
	width := 0
	previousLine := tokens[start].Line
	for i := start + 1; i < end; i++ {
		if tokens[i].Line > previousLine {
			return true
		}
		width += len([]rune(tokens[i].Raw)) + 1
		if width > 88 {
			return true
		}
		if isOpeningDelimiter(tokens[i].Raw) && pairs[i] > i {
			close := pairs[i]
			if tokens[i].Raw == "{" {
				kind := braceKind(tokens, i, meta)
				if kind == "block" || kind == "struct" {
					i = close
					previousLine = tokens[close].Line
					width += 2
					continue
				}
			}
			i = close
			previousLine = tokens[close].Line
			width += len([]rune(tokens[close].Raw))
			continue
		}
		previousLine = tokens[i].Line
	}
	return false
}

func isOpeningDelimiter(raw string) bool {
	return raw == "{" || raw == "[" || raw == "("
}

func braceKind(tokens []lexer.Token, index int, meta formatMetadata) string {
	token := tokens[index]
	key := locationKey(token.Line, token.Col)
	if meta.blockStarts[key] || inferredBlockStart(tokens, index) {
		return "block"
	}
	if meta.objectStarts[key] {
		return "object"
	}
	if previousNonComment(tokens, index) == "struct" {
		return "struct"
	}
	if inferredTypeBlock(tokens, index) {
		return "block"
	}
	return "object"
}

func inferredBlockStart(tokens []lexer.Token, index int) bool {
	limit := index - 1
	if limit < 0 {
		return false
	}
	keywords := map[string]bool{
		"if": true, "else": true, "elif": true, "unless": true, "while": true, "for": true, "each": true,
		"fn": true, "try": true, "catch": true, "finally": true, "match": true, "watch": true, "loop": true,
		"repeat": true, "defer": true, "select": true, "guard": true, "ifhave": true, "ifset": true,
		"with": true, "class": true, "enum": true, "namespace": true, "component": true, "trait": true,
		"interface": true, "abstract": true, "macro": true,
	}
	depth := 0
	for i := limit; i >= 0 && index-i < 40; i-- {
		t := tokens[i]
		if t.Type == lexer.COMMENT {
			continue
		}
		raw := t.Raw
		if raw == ")" || raw == "]" {
			depth++
			continue
		}
		if raw == "(" || raw == "[" {
			if depth > 0 {
				depth--
				continue
			}
		}
		if depth > 0 {
			continue
		}
		if raw == "=" || raw == "," || raw == ";" || raw == "=>" {
			return raw == "=>"
		}
		if raw == "{" || raw == "}" {
			break
		}
		if keywords[raw] {
			return true
		}
	}
	return false
}

func inferredTypeBlock(tokens []lexer.Token, index int) bool {
	for i := index - 1; i >= 0 && index-i < 12; i-- {
		raw := tokens[i].Raw
		if raw == "class" || raw == "enum" || raw == "namespace" || raw == "component" || raw == "trait" || raw == "interface" || raw == "macro" {
			return true
		}
		if raw == "=" || raw == ";" || raw == "{" || raw == "}" {
			return false
		}
	}
	return false
}

func previousNonComment(tokens []lexer.Token, index int) string {
	for i := index - 1; i >= 0; i-- {
		if tokens[i].Type != lexer.COMMENT {
			return tokens[i].Raw
		}
	}
	return ""
}

func shouldBreakBefore(tokens []lexer.Token, index int, frames []tokenFrame, meta formatMetadata, writer *tokenWriter) bool {
	if index <= 0 || writer.lineStart {
		return false
	}
	current := tokens[index]
	previous := tokens[index-1]
	if current.Type == lexer.COMMENT || previous.Type == lexer.COMMENT {
		return false
	}
	if previous.Raw == ";" {
		return true
	}
	if current.Raw == "}" {
		if len(frames) == 0 {
			return false
		}
		frame := frames[len(frames)-1]
		return !frame.inline && (frame.kind == "block" || frame.kind == "struct" || frame.multi)
	}
	if current.Raw == "else" || current.Raw == "catch" || current.Raw == "finally" || (current.Raw == "if" && previous.Raw == "else") {
		return false
	}
	if isBinaryOperator(previous) || isBinaryOperator(current) {
		return false
	}
	if kind, ok := meta.statementStarts[locationKey(current.Line, current.Col)]; ok {
		_ = kind
		return activeLayoutFrame(frames)
	}
	return false
}

func shouldAddSpace(tokens []lexer.Token, index int, frames []tokenFrame, meta formatMetadata, pairs map[int]int) bool {
	if index <= 0 {
		return false
	}
	current, previous := tokens[index], tokens[index-1]
	if previous.Type == lexer.COMMENT || current.Type == lexer.COMMENT {
		return false
	}
	p, c := previous.Raw, current.Raw
	if c == "." || c == "?." || c == "::" || c == ")" || c == "]" || c == "," || c == ";" || c == "}" {
		return false
	}
	if p == "." || p == "?." || p == "::" || p == "(" || p == "[" {
		return false
	}
	if c == "(" {
		switch p {
		case "if", "while", "for", "catch", "switch", "match", "unless", "ifhave", "ifset":
			return true
		default:
			return false
		}
	}
	if c == "[" {
		return false
	}
	if c == "{" {
		frame := makeTokenFrame(tokens, index, pairs, meta)
		return frame.kind == "block" || frame.kind == "struct" || isBinaryOperator(previous)
	}
	if p == "{" {
		if len(frames) > 0 && (frames[len(frames)-1].kind == "object" || frames[len(frames)-1].kind == "struct") {
			return !frames[len(frames)-1].multi && !frames[len(frames)-1].inline
		}
		return false
	}
	if p == "," || p == ":" {
		return true
	}
	if c == ":" {
		return !insideObject(frames)
	}
	if c == "!" && isUnaryOperator(tokens, index) {
		switch p {
		case "if", "while", "unless", "return", "throw", "raise", "assert", "not":
			return true
		default:
			return false
		}
	}
	if c == "++" || c == "--" || c == "~" && isUnaryOperator(tokens, index) || c == "..." {
		return false
	}
	if isBinaryOperator(current) {
		return !isUnaryOperator(tokens, index)
	}
	if isBinaryOperator(previous) {
		return !isPostfixOperator(previous.Raw) && !isUnaryOperator(tokens, index-1)
	}
	if needsWordSpace(previous, current) {
		return true
	}
	if p == "}" && (c == "else" || c == "catch" || c == "finally") {
		return true
	}
	return false
}

func needsWordSpace(previous, current lexer.Token) bool {
	word := func(t lexer.Token) bool {
		switch t.Type {
		case lexer.KEYWORD, lexer.IDENTIFIER, lexer.NUMBER, lexer.STRING, lexer.TEMPLATE, lexer.REGEX:
			return true
		default:
			return false
		}
	}
	if word(previous) && word(current) {
		return true
	}
	if previous.Type == lexer.KEYWORD && (current.Type == lexer.IDENTIFIER || current.Type == lexer.STRING || current.Type == lexer.NUMBER) {
		return true
	}
	if previous.Raw == "return" || previous.Raw == "throw" || previous.Raw == "raise" || previous.Raw == "log" || previous.Raw == "defer" || previous.Raw == "assert" || previous.Raw == "new" || previous.Raw == "typeof" || previous.Raw == "void" || previous.Raw == "not" {
		return true
	}
	return false
}

func isBinaryOperator(token lexer.Token) bool {
	if token.Type != lexer.OPERATOR {
		return false
	}
	switch token.Raw {
	case ".", "?.", "::", "...", "++", "--", "!", "~", "@":
		return false
	default:
		return true
	}
}

func isUnaryOperator(tokens []lexer.Token, index int) bool {
	if index < 0 || index >= len(tokens) {
		return false
	}
	raw := tokens[index].Raw
	if raw != "!" && raw != "~" && raw != "+" && raw != "-" && raw != "++" && raw != "--" && raw != "..." {
		return false
	}
	if raw == "++" || raw == "--" || raw == "..." {
		return true
	}
	if index == 0 {
		return true
	}
	prev := tokens[index-1]
	if prev.Type == lexer.OPERATOR || prev.Type == lexer.PUNCTUATION {
		return prev.Raw != ")" && prev.Raw != "]" && prev.Raw != "}"
	}
	switch prev.Raw {
	case "return", "throw", "raise", "log", "case", "in", "of", "=>", "if", "while", "for", "unless", "else", "assert", "not":
		return true
	}
	return false
}

func isPostfixOperator(raw string) bool {
	return raw == "++" || raw == "--"
}

func activeLayoutFrame(frames []tokenFrame) bool {
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i].kind == "block" || frames[i].kind == "struct" {
			return true
		}
		if frames[i].kind == "object" || frames[i].kind == "array" || frames[i].kind == "group" || frames[i].kind == "index" {
			return false
		}
	}
	return true
}

func insideParentheses(frames []tokenFrame) bool {
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i].kind == "group" {
			return true
		}
		if frames[i].kind == "block" || frames[i].kind == "object" || frames[i].kind == "array" || frames[i].kind == "struct" {
			return false
		}
	}
	return false
}

func insideObject(frames []tokenFrame) bool {
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i].kind == "object" || frames[i].kind == "struct" {
			return true
		}
		if frames[i].kind == "block" {
			return false
		}
	}
	return false
}

func takeFrame(frames []tokenFrame) tokenFrame {
	if len(frames) == 0 {
		return tokenFrame{inline: true}
	}
	return frames[len(frames)-1]
}

func locationKey(line, col int) string {
	return fmt.Sprintf("%d:%d", line, col)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
