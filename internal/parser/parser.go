package parser

import (
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/errfmt"
	"lunex/internal/lexer"
	"strconv"
	"strings"
)

type Parser struct {
	tokens   []lexer.Token
	pos      int
	filename string
	ifsetID  int
	lines    []string
}

func New(tokens []lexer.Token, filename string) *Parser {
	return &Parser{tokens: tokens, filename: filename}
}

func NewWithLines(tokens []lexer.Token, filename string, lines []string) *Parser {
	return &Parser{tokens: tokens, filename: filename, lines: lines}
}

func isAllowedKeywordAsName(name string) bool {
	switch name {
	case "get", "set", "from", "as", "of", "in", "static",
		"module", "namespace", "is", "not", "have",
		"override", "abstract", "readonly", "private",
		"public", "protected", "implements", "trait",
		"interface", "satisfies", "infer", "keyof",
		"alias", "enum", "component", "default",
		"async", "await", "yield", "type":
		return true
	}
	return false
}

func isHardReservedKeyword(name string) bool {
	switch name {
	case "val", "var", "const", "fn", "if", "else", "elif",
		"while", "for", "break", "continue", "do", "each",
		"match", "case", "try", "catch", "finally", "raise", "throw",
		"return", "true", "false", "null", "void", "undefined",
		"new", "this", "super", "extends", "typeof", "instanceof",
		"import", "export", "require", "spawn", "select", "channel",
		"macro", "immutable", "freeze", "with", "using", "assert",
		"delete", "use", "struct", "defer", "guard", "loop",
		"repeat", "when", "nax", "lunex", "sleep", "range",
		"ifhave", "ifset", "between", "matches", "startsWith",
		"endsWith", "unless":
		return true
	}
	return false
}

func (p *Parser) peek(offset int) lexer.Token {
	i := p.pos + offset
	if i >= len(p.tokens) {
		return lexer.Token{Type: lexer.EOF, Value: ""}
	}
	return p.tokens[i]
}

func (p *Parser) current() lexer.Token {
	return p.peek(0)
}

func (p *Parser) advance() lexer.Token {
	t := p.current()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return t
}

func (p *Parser) check(tt lexer.TokenType, val string) bool {
	t := p.current()
	if t.Type != tt {
		return false
	}
	if val != "" && t.StrVal() != val {
		return false
	}
	return true
}

func (p *Parser) checkKw(kw string) bool {
	return p.check(lexer.KEYWORD, kw)
}

func (p *Parser) checkTok(tt lexer.TokenType) bool {
	return p.current().Type == tt
}

func (p *Parser) eat(tt lexer.TokenType, val string) (lexer.Token, error) {
	t := p.current()
	if t.Type != tt {
		return t, p.errorf(t, "expected %s %q but got %s %q", tt, val, t.Type, t.StrVal())
	}
	if val != "" && t.StrVal() != val {
		return t, p.errorf(t, "expected %q but got %q", val, t.StrVal())
	}
	p.advance()
	return t, nil
}

func (p *Parser) eatIf(tt lexer.TokenType, val string) bool {
	if p.check(tt, val) {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) eatSemi() {
	p.eatIf(lexer.PUNCTUATION, ";")
}

func (p *Parser) isLineEnd() bool {
	t := p.current()
	return t.Type == lexer.EOF || t.StrVal() == ";"
}

func (p *Parser) errorf(t lexer.Token, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	kind := errfmt.KindParse
	suggestion := p.contextualSuggestion(t, msg)
	code := p.contextualCode(t, msg)
	return &errfmt.LunexError{
		Message:    msg,
		File:       p.filename,
		Line:       t.Line,
		Col:        t.Col,
		Kind:       kind,
		Code:       code,
		Suggestion: suggestion,
		Lines:      p.lines,
	}
}

func (p *Parser) contextualCode(t lexer.Token, msg string) string {
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "reserved keyword") {
		if strings.Contains(lower, "parameter") {
			return errfmt.ErrKeywordAsArg
		}
		if strings.Contains(lower, "field") {
			return errfmt.ErrKeywordAsField
		}
		return errfmt.ErrReservedKeyword
	}
	if strings.Contains(lower, "unexpected token") {
		switch t.StrVal() {
		case ",":
			return "E1001"
		case ")":
			return "E1002"
		case "}":
			return "E1003"
		case "]":
			return "E1004"
		case "=":
			return "E1005"
		case ";":
			return "E1006"
		}
		return "E1000"
	}
	if strings.Contains(lower, "expected") {
		return "E1010"
	}
	return ""
}

func (p *Parser) contextualSuggestion(t lexer.Token, msg string) string {
	lower := strings.ToLower(msg)

	if !strings.Contains(lower, "unexpected token") && !strings.Contains(lower, "expected") {
		return ""
	}

	prev := lexer.Token{}
	if p.pos > 1 {
		prev = p.tokens[p.pos-2]
	}
	prevVal := prev.StrVal()

	switch t.StrVal() {
	case ",":

		if prev.Type == lexer.PUNCTUATION && (prevVal == "(" || prevVal == "[" || prevVal == "{") {
			return "a leading comma is not allowed — remove it or place it after the first element"
		}
		if prev.Type == lexer.PUNCTUATION && (prevVal == ",") {
			return "double comma detected — remove one of them"
		}

		return "a comma is not valid here — if you are listing values, wrap them in [ ] for an array or ( ) for a grouped expression"

	case ")":
		return "unexpected closing parenthesis — check that every '(' has a matching ')' and no extra closing bracket was added"

	case "}":
		return "unexpected closing brace — check that every '{' block is properly closed and there is no stray '}'"

	case "]":
		return "unexpected closing bracket — check that every '[' has a matching ']'"

	case "=":
		if prevVal == "=" {
			return "use '==' for equality comparison, not '='"
		}
		if prev.Type == lexer.KEYWORD && (prevVal == "val" || prevVal == "var") {
			return "declaration syntax is: val name = value  (no type annotation needed)"
		}
		return "assignment is not an expression in Lunex — use '==' to compare, or move the assignment to its own statement"

	case ";":
		return "Lunex uses newlines as statement separators — semicolons are optional and usually not needed here"

	case ":":
		if prev.Type == lexer.IDENTIFIER {
			return "type annotations use '::' in Lunex — e.g. val x :: int = 5 — or remove the colon if you don't need a type hint"
		}
		return "unexpected colon — if you meant to write an object key, wrap the object in { } and ensure the key is on the left of ':'"

	case "=>":
		return "arrow functions need parameters on the left — e.g. fn(x) => x * 2  or  (x) => x * 2"
	}

	if strings.Contains(lower, "expected") && strings.Contains(lower, "but got") {

		start := strings.Index(lower, "expected")
		end := strings.Index(lower[start:], "but got")
		if end > 0 {
			expected := strings.TrimSpace(msg[start+8 : start+end])
			return fmt.Sprintf("the parser expected %s at this position — check for a missing token before '%s'", expected, t.StrVal())
		}
	}

	return "check the syntax around this token — something may be missing or misplaced just before it"
}

func (p *Parser) Parse() (*ast.Node, error) {
	prog := &ast.Node{Type: ast.Program, Line: 1, Col: 1}
	for !p.check(lexer.EOF, "") {
		p.eatSemi()
		if p.check(lexer.EOF, "") {
			break
		}
		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			prog.Body_ = append(prog.Body_, stmt)
		}
	}
	return prog, nil
}

func (p *Parser) parseBlock() (*ast.Node, error) {
	openTok, err := p.eat(lexer.PUNCTUATION, "{")
	if err != nil {
		return nil, err
	}
	block := &ast.Node{Type: ast.Block, Line: openTok.Line, Col: openTok.Col}
	for !p.check(lexer.PUNCTUATION, "}") && !p.check(lexer.EOF, "") {
		p.eatSemi()
		if p.check(lexer.PUNCTUATION, "}") {
			break
		}
		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			block.Body_ = append(block.Body_, stmt)
		}
	}
	if p.check(lexer.EOF, "") {

		return nil, &errfmt.LunexError{
			Message:    fmt.Sprintf("unclosed block — '{' on line %d was never closed with '}'", openTok.Line),
			File:       p.filename,
			Line:       openTok.Line,
			Col:        openTok.Col,
			Kind:       errfmt.KindParse,
			Code:       "E0052",
			Suggestion: "add a closing '}' to match the '{' on this line",
			Lines:      p.lines,
		}
	}
	if _, err := p.eat(lexer.PUNCTUATION, "}"); err != nil {
		return nil, err
	}
	return block, nil
}

func (p *Parser) parseStmt() (*ast.Node, error) {
	t := p.current()

	if t.Type == lexer.IDENTIFIER {
		switch t.StrVal() {
		case "return":
			p.advance()
			return nil, p.errorf(t, "Lunex does not use 'return' — the last expression in a function is its result automatically\n  hint: remove 'return' and leave the value as the final expression")
		case "class":
			p.advance()
			return nil, p.errorf(t, "Lunex does not support 'class' — use 'val TypeName = struct { ... }' instead\n  hint: struct groups fields and functions into a named value")
		}
	}
	if t.Type == lexer.KEYWORD {
		switch t.StrVal() {
		case "var", "val", "let", "const":
			return p.parseVarDecl()
		case "fn":
			return p.parseFnDecl()
		case "enum":
			return p.parseEnumDecl()
		case "namespace":
			return p.parseNamespace()
		case "component":
			return p.parseComponent()
		case "import":
			return p.parseImport()
		case "export":
			return p.parseExport()
		case "require":
			if p.peek(1).StrVal() == "(" {
				return p.parseLunexRequire()
			}
		case "if":
			return p.parseIf()
		case "unless":
			return p.parseUnless()
		case "while":
			return p.parseWhile()
		case "for":
			return p.parseFor()
		case "each":
			return p.parseEachIn()
		case "repeat":
			return p.parseRepeat()
		case "loop":
			return p.parseLoop()
		case "match":
			return p.parseMatch()
		case "try":
			return p.parseTry()
		case "throw", "raise":
			return p.parseThrow()
		case "break":
			p.advance()
			p.eatSemi()
			return &ast.Node{Type: ast.BreakStmt, Line: t.Line, Col: t.Col}, nil
		case "continue":
			p.advance()
			p.eatSemi()
			return &ast.Node{Type: ast.ContinueStmt, Line: t.Line, Col: t.Col}, nil
		case "guard":
			return p.parseGuard()
		case "defer":
			return p.parseDefer()
		case "spawn":
			return p.parseSpawn()
		case "select":
			return p.parseSelect()
		case "immutable":
			return p.parseImmutable()
		case "assert":
			return p.parseAssert()
		case "have":
			return p.parseHaveStmt()
		case "ifhave":
			return p.parseIfHave()
		case "ifset":
			return p.parseIfSet()
		case "use":
			t2 := p.advance()
			return nil, p.errorf(t2, "'use' has been removed from Lunex — use 'val name = @import(\"std.module\")' instead\n  example: val io = @import(\"std.io\")")
		case "delete":
			return p.parseDelete()
		case "with":
			return p.parseWith()
		case "using":
			return p.parseUsing()
		case "type", "alias", "interface", "abstract", "trait":
			return p.parseTypeAlias()
		}
	}

	if t.Type == lexer.OPERATOR && t.StrVal() == "@" {
		if p.peek(1).StrVal() == "import" {

		} else {
			return p.parseDecorated()
		}
	}

	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	p.eatSemi()
	return &ast.Node{Type: ast.ExprStmt, Expr: expr, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseVarDecl() (*ast.Node, error) {
	t := p.advance()
	isConst := t.StrVal() == "val" || t.StrVal() == "const"

	node := &ast.Node{Type: ast.VarDecl, IsConst: isConst, Line: t.Line, Col: t.Col}

	cur := p.current()
	if cur.Type == lexer.PUNCTUATION && (cur.StrVal() == "{" || cur.StrVal() == "[") {
		destr, err := p.parseDestructurePattern()
		if err != nil {
			return nil, err
		}
		node.Destructure = destr
		if p.eatIf(lexer.OPERATOR, ":") {
			node.TypeAnn = p.skipTypeExpr()
		}
		if p.eatIf(lexer.OPERATOR, "=") {
			init, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			node.Init = init
		}
		p.eatSemi()
		return node, nil
	}

	nameTok, err := p.eat(lexer.IDENTIFIER, "")
	if err != nil {

		cur := p.current()
		if cur.Type == lexer.KEYWORD && isHardReservedKeyword(cur.StrVal()) {
			kw := cur.StrVal()
			p.advance()
			return nil, p.errorf(cur,
				"reserved keyword '%s' cannot be used as a variable name — "+
					"'%s' is reserved by Lunex and has special meaning. "+
					"Choose a different name (e.g. '%s_val', 'my_%s').",
				kw, kw, kw, kw,
			)
		}
		return nil, err
	}
	node.Name = nameTok.StrVal()

	if p.eatIf(lexer.OPERATOR, "?") {
	}
	if p.eatIf(lexer.OPERATOR, ":") {
		node.TypeAnn = p.skipTypeExpr()
	}
	if p.eatIf(lexer.OPERATOR, "=") {
		init, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		node.Init = init
	}
	p.eatSemi()
	return node, nil
}

func (p *Parser) parseDestructurePattern() (interface{}, error) {
	t := p.current()
	if t.StrVal() == "{" {
		p.advance()
		props := []map[string]interface{}{}
		for !p.check(lexer.PUNCTUATION, "}") && !p.check(lexer.EOF, "") {
			if p.check(lexer.OPERATOR, "...") {
				p.advance()
				rest, _ := p.eat(lexer.IDENTIFIER, "")
				props = append(props, map[string]interface{}{"kind": "rest", "name": rest.StrVal()})
				p.eatIf(lexer.PUNCTUATION, ",")
				continue
			}
			key := p.current()
			p.advance()
			alias := key.StrVal()
			if p.eatIf(lexer.OPERATOR, ":") {
				al, _ := p.eat(lexer.IDENTIFIER, "")
				alias = al.StrVal()
			}
			var defVal interface{}
			if p.eatIf(lexer.OPERATOR, "=") {
				dv, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				defVal = dv
			}
			props = append(props, map[string]interface{}{"key": key.StrVal(), "alias": alias, "default": defVal})
			p.eatIf(lexer.PUNCTUATION, ",")
		}
		p.eat(lexer.PUNCTUATION, "}")
		return map[string]interface{}{"kind": "object", "props": props}, nil
	}
	if t.StrVal() == "[" {
		p.advance()
		items := []interface{}{}
		for !p.check(lexer.PUNCTUATION, "]") && !p.check(lexer.EOF, "") {
			if p.check(lexer.PUNCTUATION, ",") {
				p.advance()
				items = append(items, nil)
				continue
			}
			if p.check(lexer.OPERATOR, "...") {
				p.advance()
				rest, _ := p.eat(lexer.IDENTIFIER, "")
				items = append(items, map[string]interface{}{"kind": "rest", "name": rest.StrVal()})
				p.eatIf(lexer.PUNCTUATION, ",")
				continue
			}
			name, _ := p.eat(lexer.IDENTIFIER, "")
			var defVal interface{}
			if p.eatIf(lexer.OPERATOR, "=") {
				dv, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				defVal = dv
			}
			items = append(items, map[string]interface{}{"name": name.StrVal(), "default": defVal})
			p.eatIf(lexer.PUNCTUATION, ",")
		}
		p.eat(lexer.PUNCTUATION, "]")
		return map[string]interface{}{"kind": "array", "items": items}, nil
	}
	return nil, nil
}

func (p *Parser) parseFnDecl() (*ast.Node, error) {
	t := p.current()
	_, err := p.eat(lexer.KEYWORD, "fn")
	if err != nil {
		return nil, err
	}
	node := &ast.Node{Type: ast.FnDecl, Line: t.Line, Col: t.Col}

	if p.checkTok(lexer.IDENTIFIER) || p.checkTok(lexer.KEYWORD) {
		nameTok := p.advance()
		name := nameTok.StrVal()

		if nameTok.Type == lexer.KEYWORD && !isAllowedKeywordAsName(name) {
			return nil, p.errorf(nameTok,
				"reserved keyword '%s' cannot be used as a function name — choose a different name (e.g. '%s_fn' or 'my_%s')",
				name, name, name,
			)
		}
		node.Name = name
	}

	if p.eatIf(lexer.OPERATOR, ":") {
		p.skipTypeExpr()
	}

	params, err := p.parseFnParams()
	if err != nil {
		return nil, err
	}
	node.Params = params

	if p.eatIf(lexer.OPERATOR, "->") || p.eatIf(lexer.OPERATOR, ":") {
		node.TypeAnn = p.skipTypeExpr()
	}

	if p.check(lexer.OPERATOR, "=>") {
		p.advance()
		body, err := p.parseExprAsBlock()
		if err != nil {
			return nil, err
		}
		node.Body = body
	} else {
		body, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		node.Body = body
	}
	return node, nil
}

func (p *Parser) parseFnParams() ([]*ast.Param, error) {
	if _, err := p.eat(lexer.PUNCTUATION, "("); err != nil {
		return nil, err
	}
	var params []*ast.Param
	for !p.check(lexer.PUNCTUATION, ")") && !p.check(lexer.EOF, "") {
		param := &ast.Param{ResolvedSlot: -1}
		if p.check(lexer.OPERATOR, "...") {
			p.advance()
			param.Rest = true
		}
		cur := p.current()
		if cur.StrVal() == "{" || cur.StrVal() == "[" {
			destr, err := p.parseDestructurePattern()
			if err != nil {
				return nil, err
			}
			param.Destructure = destr
			param.Name = "_"
		} else {
			var nameTok lexer.Token
			var err error
			if p.checkTok(lexer.KEYWORD) {
				nameTok = p.advance()

				if isHardReservedKeyword(nameTok.StrVal()) {
					return nil, p.errorf(nameTok,
						"reserved keyword '%s' cannot be used as a parameter name — "+
							"'%s' is a reserved keyword in Lunex. Use a descriptive name instead.",
						nameTok.StrVal(), nameTok.StrVal(),
					)
				}
			} else {
				nameTok, err = p.eat(lexer.IDENTIFIER, "")
				if err != nil {
					return nil, err
				}
			}
			param.Name = nameTok.StrVal()
		}
		p.eatIf(lexer.OPERATOR, "?")
		if p.eatIf(lexer.OPERATOR, ":") {
			param.TypeAnn = p.skipTypeExpr()
		}
		if !param.Rest && p.eatIf(lexer.OPERATOR, "=") {
			defVal, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			param.DefaultVal = defVal
		}
		params = append(params, param)
		if !p.check(lexer.PUNCTUATION, ")") {
			p.eatIf(lexer.PUNCTUATION, ",")
		}
	}
	if _, err := p.eat(lexer.PUNCTUATION, ")"); err != nil {
		return nil, err
	}
	return params, nil
}

func (p *Parser) parseClassDecl() (*ast.Node, error) {
	t := p.advance()
	node := &ast.Node{Type: ast.ClassDecl, Line: t.Line, Col: t.Col}
	if p.checkTok(lexer.IDENTIFIER) {
		node.Name = p.advance().StrVal()
	}
	if p.eatIf(lexer.KEYWORD, "extends") {
		super, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		node.SuperClass = super
	}
	if p.eatIf(lexer.KEYWORD, "implements") {
		for !p.check(lexer.PUNCTUATION, "{") && !p.check(lexer.EOF, "") {
			p.advance()
			p.eatIf(lexer.PUNCTUATION, ",")
		}
	}
	if _, err := p.eat(lexer.PUNCTUATION, "{"); err != nil {
		return nil, err
	}
	for !p.check(lexer.PUNCTUATION, "}") && !p.check(lexer.EOF, "") {
		p.eatSemi()
		if p.check(lexer.PUNCTUATION, "}") {
			break
		}
		member, err := p.parseClassMember()
		if err != nil {
			return nil, err
		}
		if member != nil {
			node.Methods = append(node.Methods, member)
		}
	}
	p.eat(lexer.PUNCTUATION, "}")
	return node, nil
}

func (p *Parser) parseClassMember() (*ast.ClassMember, error) {
	m := &ast.ClassMember{}
	if p.eatIf(lexer.OPERATOR, "@") {
		for !p.checkTok(lexer.IDENTIFIER) && !p.checkKw("fn") && !p.checkKw("static") {
			p.advance()
		}
	}
	if p.eatIf(lexer.KEYWORD, "static") {
		m.IsStatic = true
	}
	if p.eatIf(lexer.KEYWORD, "private") {
		m.IsPrivate = true
	}
	if p.eatIf(lexer.KEYWORD, "public") || p.eatIf(lexer.KEYWORD, "protected") || p.eatIf(lexer.KEYWORD, "readonly") {
	}
	isGet := p.checkKw("get") && p.peek(1).Type == lexer.IDENTIFIER
	isSet := p.checkKw("set") && p.peek(1).Type == lexer.IDENTIFIER
	if isGet || isSet {
		m.IsGet = isGet
		m.IsSet = isSet
		p.advance()
	}
	if p.eatIf(lexer.KEYWORD, "fn") {
	}
	p.eatIf(lexer.OPERATOR, "*")

	nameTok := p.current()
	m.Name = nameTok.StrVal()
	p.advance()

	if p.eatIf(lexer.OPERATOR, ":") {
		m.TypeAnn = p.skipTypeExpr()
	}
	if p.eatIf(lexer.OPERATOR, "=") {
		init, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		m.Init = init
		p.eatSemi()
		m.Kind = "field"
		return m, nil
	}

	if !p.check(lexer.PUNCTUATION, "(") {
		p.eatSemi()
		m.Kind = "field"
		return m, nil
	}

	params, err := p.parseFnParams()
	if err != nil {
		return nil, err
	}
	m.Params = params
	if p.eatIf(lexer.OPERATOR, "->") || p.eatIf(lexer.OPERATOR, ":") {
		m.TypeAnn = p.skipTypeExpr()
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	m.Body = body
	m.Kind = "method"
	return m, nil
}

func (p *Parser) parseEnumDecl() (*ast.Node, error) {
	t := p.advance()
	name, err := p.eat(lexer.IDENTIFIER, "")
	if err != nil {
		return nil, err
	}
	node := &ast.Node{Type: ast.EnumDecl, Name: name.StrVal(), Line: t.Line, Col: t.Col}
	if _, err := p.eat(lexer.PUNCTUATION, "{"); err != nil {
		return nil, err
	}
	for !p.check(lexer.PUNCTUATION, "}") && !p.check(lexer.EOF, "") {
		p.eatSemi()
		if p.check(lexer.PUNCTUATION, "}") {
			break
		}
		mem := &ast.EnumMember{}
		memName, err := p.eat(lexer.IDENTIFIER, "")
		if err != nil {
			return nil, err
		}
		mem.Name = memName.StrVal()
		if p.eatIf(lexer.OPERATOR, "=") {
			init, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			mem.Init = init
		}
		node.Members = append(node.Members, mem)
		p.eatIf(lexer.PUNCTUATION, ",")
	}
	p.eat(lexer.PUNCTUATION, "}")
	return node, nil
}

func (p *Parser) parseNamespace() (*ast.Node, error) {
	t := p.advance()
	name, err := p.eat(lexer.IDENTIFIER, "")
	if err != nil {
		return nil, err
	}
	node := &ast.Node{Type: ast.NamespaceDecl, Name: name.StrVal(), Line: t.Line, Col: t.Col}
	if _, err := p.eat(lexer.PUNCTUATION, "{"); err != nil {
		return nil, err
	}
	for !p.check(lexer.PUNCTUATION, "}") && !p.check(lexer.EOF, "") {
		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			node.Body_ = append(node.Body_, stmt)
		}
	}
	p.eat(lexer.PUNCTUATION, "}")
	return node, nil
}

func (p *Parser) parseComponent() (*ast.Node, error) {
	t := p.advance()
	name, err := p.eat(lexer.IDENTIFIER, "")
	if err != nil {
		return nil, err
	}
	params, err := p.parseFnParams()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.Node{Type: ast.ComponentDecl, Name: name.StrVal(), Params: params, Body: body, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseImport() (*ast.Node, error) {
	t := p.advance()
	node := &ast.Node{Type: ast.ImportDecl, Line: t.Line, Col: t.Col}
	if p.eatIf(lexer.KEYWORD, "type") {
		node.TypeOnly = true
	}
	if p.check(lexer.OPERATOR, "*") {
		p.advance()
		p.eat(lexer.KEYWORD, "as")
		ns, _ := p.eat(lexer.IDENTIFIER, "")
		node.Namespace = ns.StrVal()
	} else if p.checkTok(lexer.STRING) && !p.check(lexer.PUNCTUATION, "{") {
	} else if p.checkTok(lexer.IDENTIFIER) && p.peek(1).StrVal() != "," && p.peek(1).StrVal() != "{" {
		defTok := p.advance()
		node.DefaultImport = defTok.StrVal()
		if p.eatIf(lexer.PUNCTUATION, ",") {
			specs, err := p.parseImportSpecifiers()
			if err != nil {
				return nil, err
			}
			node.Specifiers = specs
		}
	} else if p.check(lexer.PUNCTUATION, "{") {
		specs, err := p.parseImportSpecifiers()
		if err != nil {
			return nil, err
		}
		node.Specifiers = specs
	} else if p.checkTok(lexer.IDENTIFIER) || p.checkKw("default") {
		defTok := p.advance()
		node.DefaultImport = defTok.StrVal()
		if p.eatIf(lexer.PUNCTUATION, ",") {
			specs, err := p.parseImportSpecifiers()
			if err != nil {
				return nil, err
			}
			node.Specifiers = specs
		}
	}
	p.eatIf(lexer.KEYWORD, "from")
	srcTok, err := p.eat(lexer.STRING, "")
	if err != nil {
		return nil, err
	}
	node.Source = srcTok.StrVal()
	p.eatSemi()
	return node, nil
}

func (p *Parser) parseImportSpecifiers() ([]*ast.ImportSpec, error) {
	if _, err := p.eat(lexer.PUNCTUATION, "{"); err != nil {
		return nil, err
	}
	var specs []*ast.ImportSpec
	for !p.check(lexer.PUNCTUATION, "}") && !p.check(lexer.EOF, "") {
		importedTok := p.current()
		if importedTok.Type != lexer.IDENTIFIER && importedTok.Type != lexer.KEYWORD {
			break
		}
		p.advance()
		spec := &ast.ImportSpec{Imported: importedTok.StrVal(), Local: importedTok.StrVal()}
		if p.eatIf(lexer.KEYWORD, "as") {
			localTok, err := p.eat(lexer.IDENTIFIER, "")
			if err != nil {
				return nil, err
			}
			spec.Local = localTok.StrVal()
		}
		specs = append(specs, spec)
		p.eatIf(lexer.PUNCTUATION, ",")
	}
	p.eat(lexer.PUNCTUATION, "}")
	return specs, nil
}

func (p *Parser) parseExport() (*ast.Node, error) {
	t := p.advance()
	node := &ast.Node{Type: ast.ExportDecl, Line: t.Line, Col: t.Col}
	if p.eatIf(lexer.KEYWORD, "default") {
		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		p.eatSemi()
		node.Value = val
		return node, nil
	}
	if p.check(lexer.PUNCTUATION, "{") {
		p.advance()
		for !p.check(lexer.PUNCTUATION, "}") && !p.check(lexer.EOF, "") {
			localTok := p.current()
			if localTok.Type != lexer.IDENTIFIER && localTok.Type != lexer.KEYWORD {
				break
			}
			p.advance()
			spec := &ast.ImportSpec{Imported: localTok.StrVal(), Exported: localTok.StrVal()}
			if p.eatIf(lexer.KEYWORD, "as") {
				expTok := p.advance()
				spec.Exported = expTok.StrVal()
			}
			node.Specifiers = append(node.Specifiers, spec)
			p.eatIf(lexer.PUNCTUATION, ",")
		}
		p.eat(lexer.PUNCTUATION, "}")
		p.eatSemi()
		return node, nil
	}
	decl, err := p.parseStmt()
	if err != nil {
		return nil, err
	}
	node.Declaration = decl
	return node, nil
}

func (p *Parser) parseUse() (*ast.Node, error) {
	t := p.advance()
	modTok := p.advance()
	modName := modTok.StrVal()

	for p.check(lexer.OPERATOR, "/") {
		p.advance()
		segTok := p.current()
		if segTok.Type != lexer.IDENTIFIER && segTok.Type != lexer.KEYWORD {
			break
		}
		p.advance()
		modName += "/" + segTok.StrVal()
	}

	alias := modName
	if idx := strings.LastIndex(modName, "/"); idx >= 0 {
		alias = modName[idx+1:]
	}

	if p.check(lexer.KEYWORD, "as") || p.check(lexer.IDENTIFIER, "as") {
		p.advance()
		aliasTok := p.advance()
		alias = aliasTok.StrVal()
	}
	p.eatSemi()
	return &ast.Node{Type: ast.UseStmt, Name: alias, Modules: []string{modName}, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseLunexRequire() (*ast.Node, error) {
	t := p.advance()
	p.eat(lexer.PUNCTUATION, "(")
	p.eat(lexer.KEYWORD, "lunex")
	p.eat(lexer.PUNCTUATION, ",")
	node := &ast.Node{Type: ast.LunexRequire, Line: t.Line, Col: t.Col}
	for !p.check(lexer.PUNCTUATION, ")") && !p.check(lexer.EOF, "") {
		mod := p.advance()
		node.Modules = append(node.Modules, mod.StrVal())
		p.eatIf(lexer.PUNCTUATION, ",")
	}
	p.eat(lexer.PUNCTUATION, ")")
	p.eatSemi()
	return node, nil
}

func (p *Parser) parseIf() (*ast.Node, error) {
	t := p.advance()
	test, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	node := &ast.Node{Type: ast.IfStmt, Test: test, Consequent: body, Line: t.Line, Col: t.Col}
	root := node
	for {
		if p.checkKw("elif") {
			p.advance()
		} else if p.checkKw("else") && p.peek(1).Type == lexer.KEYWORD && p.peek(1).StrVal() == "if" {
			p.advance()
			p.advance()
		} else {
			break
		}
		elifTest, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		elifBody, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		elif := &ast.Node{Type: ast.IfStmt, Test: elifTest, Consequent: elifBody, Line: t.Line, Col: t.Col}
		node.Alternate = elif
		node = elif
	}
	if p.eatIf(lexer.KEYWORD, "else") {
		alt, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		node.Alternate = alt
	}
	return root, nil
}

func (p *Parser) parseIfExpr() (*ast.Node, error) {
	t := p.advance()
	test, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	node := &ast.Node{Type: ast.IfStmt, Test: test, Consequent: body, Line: t.Line, Col: t.Col}
	root := node
	for {
		if p.checkKw("elif") {
			p.advance()
		} else if p.checkKw("else") && p.peek(1).Type == lexer.KEYWORD && p.peek(1).StrVal() == "if" {
			p.advance()
			p.advance()
		} else {
			break
		}
		elifTest, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		elifBody, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		elif := &ast.Node{Type: ast.IfStmt, Test: elifTest, Consequent: elifBody, Line: t.Line, Col: t.Col}
		node.Alternate = elif
		node = elif
	}
	if p.eatIf(lexer.KEYWORD, "else") {
		alt, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		node.Alternate = alt
	}
	return root, nil
}

func (p *Parser) parseUnless() (*ast.Node, error) {
	t := p.advance()
	test, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	node := &ast.Node{Type: ast.UnlessStmt, Test: test, Consequent: body, Line: t.Line, Col: t.Col}
	if p.eatIf(lexer.KEYWORD, "else") {
		alt, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		node.Alternate = alt
	}
	return node, nil
}

func (p *Parser) parseWhile() (*ast.Node, error) {
	t := p.advance()
	test, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.Node{Type: ast.WhileStmt, Test: test, Body: body, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseFor() (*ast.Node, error) {
	t := p.advance()
	if p.eatIf(lexer.KEYWORD, "each") {
		return p.parseForEachIn(t)
	}
	var id string
	isConst := false
	if p.checkKw("val") || p.checkKw("const") {
		isConst = true
		p.advance()
	} else if p.checkKw("var") || p.checkKw("let") {
		p.advance()
	}
	nameTok, err := p.eat(lexer.IDENTIFIER, "")
	if err != nil {
		return nil, err
	}
	id = nameTok.StrVal()

	var indexVar string
	if p.eatIf(lexer.PUNCTUATION, ",") {
		valTok, err2 := p.eat(lexer.IDENTIFIER, "")
		if err2 != nil {
			return nil, err2
		}
		indexVar = id
		id = valTok.StrVal()
	}
	if p.checkKw("in") || p.checkKw("of") {
		p.advance()
		iter, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		body, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		return &ast.Node{Type: ast.ForOfStmt, Name: id, Alias: indexVar, IsConst: isConst, Right: iter, Body: body, Line: t.Line, Col: t.Col}, nil
	}
	return &ast.Node{Type: ast.ForStmt, Name: id, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseForEachIn(t lexer.Token) (*ast.Node, error) {
	id, err := p.eat(lexer.IDENTIFIER, "")
	if err != nil {
		return nil, err
	}
	p.eat(lexer.KEYWORD, "in")
	iter, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.Node{Type: ast.EachInStmt, Name: id.StrVal(), Right: iter, Body: body, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseEachIn() (*ast.Node, error) {
	t := p.advance()
	id, err := p.eat(lexer.IDENTIFIER, "")
	if err != nil {
		return nil, err
	}
	p.eat(lexer.KEYWORD, "in")
	iter, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.Node{Type: ast.EachInStmt, Name: id.StrVal(), Right: iter, Body: body, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseRepeat() (*ast.Node, error) {
	t := p.advance()
	node := &ast.Node{Type: ast.RepeatStmt, Line: t.Line, Col: t.Col}
	if !p.check(lexer.PUNCTUATION, "{") {
		count, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		node.Count = count
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	node.Body = body
	return node, nil
}

func (p *Parser) parseLoop() (*ast.Node, error) {
	t := p.advance()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.Node{Type: ast.LoopStmt, Body: body, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseMatch() (*ast.Node, error) {
	t := p.advance()
	subject, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.eat(lexer.PUNCTUATION, "{"); err != nil {
		return nil, err
	}
	node := &ast.Node{Type: ast.MatchStmt, Subject: subject, Line: t.Line, Col: t.Col}
	for !p.check(lexer.PUNCTUATION, "}") && !p.check(lexer.EOF, "") {
		p.eatSemi()
		if p.check(lexer.PUNCTUATION, "}") {
			break
		}
		mc := &ast.MatchCase{}
		isDefault := p.checkKw("default") || p.checkKw("else")
		if isDefault {
			p.advance()
			mc.IsDefault = true
		} else {
			p.eat(lexer.KEYWORD, "case")
			for {
				pat, err := p.parseMatchPattern()
				if err != nil {
					return nil, err
				}
				mc.Patterns = append(mc.Patterns, pat)
				if !p.eatIf(lexer.OPERATOR, "|") {
					break
				}
			}
			if p.eatIf(lexer.KEYWORD, "when") {
				guard, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				mc.Guard = guard
			}
		}
		p.eat(lexer.OPERATOR, "=>")
		var body *ast.Node
		if p.check(lexer.PUNCTUATION, "{") {
			body, err = p.parseBlock()
		} else {
			body, err = p.parseExprAsBlock()
		}
		if err != nil {
			return nil, err
		}
		mc.Body = body
		node.Cases = append(node.Cases, mc)
	}
	p.eat(lexer.PUNCTUATION, "}")
	return node, nil
}

func (p *Parser) parseMatchPattern() (*ast.MatchPattern, error) {
	t := p.current()
	if t.Type == lexer.KEYWORD {
		switch t.StrVal() {
		case "null":
			p.advance()
			return &ast.MatchPattern{Kind: "literal", Value: nil}, nil
		case "true":
			p.advance()
			return &ast.MatchPattern{Kind: "literal", Value: true}, nil
		case "false":
			p.advance()
			return &ast.MatchPattern{Kind: "literal", Value: false}, nil
		case "undefined":
			p.advance()
			return &ast.MatchPattern{Kind: "literal", Value: "undefined"}, nil
		}
	}
	if t.Type == lexer.NUMBER {
		p.advance()
		return &ast.MatchPattern{Kind: "literal", Value: t.StrVal()}, nil
	}
	if t.Type == lexer.STRING {
		p.advance()
		return &ast.MatchPattern{Kind: "literal", Value: t.Value}, nil
	}
	if p.check(lexer.OPERATOR, "...") {
		p.advance()
		rest, _ := p.eat(lexer.IDENTIFIER, "")
		return &ast.MatchPattern{Kind: "rest", Name: rest.StrVal()}, nil
	}
	if p.check(lexer.PUNCTUATION, "[") {
		p.advance()
		var items []*ast.MatchPattern
		for !p.check(lexer.PUNCTUATION, "]") && !p.check(lexer.EOF, "") {
			item, err := p.parseMatchPattern()
			if err != nil {
				return nil, err
			}
			items = append(items, item)
			p.eatIf(lexer.PUNCTUATION, ",")
		}
		p.eat(lexer.PUNCTUATION, "]")
		return &ast.MatchPattern{Kind: "array", Items: items}, nil
	}
	if p.check(lexer.PUNCTUATION, "{") {
		p.advance()
		var props []*ast.MatchProp
		for !p.check(lexer.PUNCTUATION, "}") && !p.check(lexer.EOF, "") {
			keyTok := p.current()
			if keyTok.Type != lexer.IDENTIFIER && keyTok.Type != lexer.KEYWORD {
				break
			}
			p.advance()
			alias := keyTok.StrVal()
			if p.eatIf(lexer.OPERATOR, ":") {
				if p.checkTok(lexer.IDENTIFIER) {
					aliasTok := p.advance()
					alias = aliasTok.StrVal()
				}
			}
			props = append(props, &ast.MatchProp{Key: keyTok.StrVal(), Alias: alias})
			p.eatIf(lexer.PUNCTUATION, ",")
		}
		p.eat(lexer.PUNCTUATION, "}")
		return &ast.MatchPattern{Kind: "object", Props: props}, nil
	}
	if t.Type == lexer.IDENTIFIER {
		p.advance()
		if p.check(lexer.PUNCTUATION, "(") {
			p.advance()
			var fields []*ast.MatchPattern
			for !p.check(lexer.PUNCTUATION, ")") && !p.check(lexer.EOF, "") {
				f, err := p.parseMatchPattern()
				if err != nil {
					return nil, err
				}
				fields = append(fields, f)
				p.eatIf(lexer.PUNCTUATION, ",")
			}
			p.eat(lexer.PUNCTUATION, ")")
			return &ast.MatchPattern{Kind: "variant", Name: t.StrVal(), Fields: fields}, nil
		}
		r := []rune(t.StrVal())
		if len(r) > 0 && r[0] >= 'A' && r[0] <= 'Z' {
			return &ast.MatchPattern{Kind: "enumVal", Path: t.StrVal()}, nil
		}
		return &ast.MatchPattern{Kind: "binding", Name: t.StrVal()}, nil
	}
	return &ast.MatchPattern{Kind: "wildcard"}, nil
}

func (p *Parser) parseTry() (*ast.Node, error) {
	t := p.advance()
	if p.check(lexer.OPERATOR, "?") {
		p.advance()
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		p.eatSemi()
		safe := &ast.Node{Type: ast.TrySafeExpr, Expr: expr, Line: t.Line, Col: t.Col}
		return &ast.Node{Type: ast.ExprStmt, Expr: safe, Line: t.Line, Col: t.Col}, nil
	}
	block, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	node := &ast.Node{Type: ast.TryStmt, Body: block, Line: t.Line, Col: t.Col}
	if p.eatIf(lexer.KEYWORD, "catch") {
		if p.check(lexer.PUNCTUATION, "(") {
			p.advance()
			cp, _ := p.eat(lexer.IDENTIFIER, "")
			node.CatchParam = cp.StrVal()
			p.eat(lexer.PUNCTUATION, ")")
		} else if p.checkTok(lexer.IDENTIFIER) {
			cp := p.advance()
			node.CatchParam = cp.StrVal()
		}
		cb, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		node.CatchBlock = cb
	}
	if p.eatIf(lexer.KEYWORD, "finally") {
		fb, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		node.FinallyBlock = fb
	}
	return node, nil
}

func (p *Parser) parseReturn() (*ast.Node, error) {
	t := p.advance()
	node := &ast.Node{Type: ast.ReturnStmt, Line: t.Line, Col: t.Col}
	if !p.isLineEnd() && !p.check(lexer.PUNCTUATION, "}") && !p.check(lexer.EOF, "") {
		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		node.Value = val
	}
	p.eatSemi()
	return node, nil
}

func (p *Parser) parseThrow() (*ast.Node, error) {
	t := p.advance()
	val, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	p.eatSemi()
	return &ast.Node{Type: ast.ThrowStmt, Value: val, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseGuard() (*ast.Node, error) {
	t := p.advance()
	test, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	p.eat(lexer.KEYWORD, "else")
	alt, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.Node{Type: ast.GuardStmt, Test: test, Alternate: alt, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseDefer() (*ast.Node, error) {
	t := p.advance()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.Node{Type: ast.DeferStmt, Body: body, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseSpawn() (*ast.Node, error) {
	t := p.advance()
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	p.eatSemi()
	return &ast.Node{Type: ast.SpawnStmt, Expr: expr, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseSelect() (*ast.Node, error) {
	t := p.advance()
	p.eat(lexer.PUNCTUATION, "{")
	node := &ast.Node{Type: ast.SelectStmt, Line: t.Line, Col: t.Col}
	for !p.check(lexer.PUNCTUATION, "}") && !p.check(lexer.EOF, "") {
		p.eatSemi()
		if p.check(lexer.PUNCTUATION, "}") {
			break
		}
		p.eat(lexer.KEYWORD, "case")
		sc := &ast.SelectCase{}
		if p.checkTok(lexer.IDENTIFIER) && p.peek(1).StrVal() == "=" {
			sc.Binding = p.advance().StrVal()
			p.advance()
		}
		ch, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		sc.Channel = ch
		p.eat(lexer.OPERATOR, "=>")
		body, err := p.parseExprAsBlock()
		if err != nil {
			return nil, err
		}
		sc.Body = body
		node.SelectCases = append(node.SelectCases, sc)
	}
	p.eat(lexer.PUNCTUATION, "}")
	return node, nil
}

func (p *Parser) parseImmutable() (*ast.Node, error) {
	t := p.advance()
	decl, err := p.parseVarDecl()
	if err != nil {
		return nil, err
	}
	return &ast.Node{Type: ast.ImmutableDecl, Body: decl, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseAssert() (*ast.Node, error) {
	t := p.advance()
	test, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	node := &ast.Node{Type: ast.AssertStmt, Test: test, Line: t.Line, Col: t.Col}
	if p.eatIf(lexer.PUNCTUATION, ",") {
		msg, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		node.Arg = msg
	}
	p.eatSemi()
	return node, nil
}

func (p *Parser) parseHaveStmt() (*ast.Node, error) {
	t := p.advance()
	id := p.ifsetID
	p.ifsetID++
	expr, err := p.parsePostfix()
	if err != nil {
		return nil, err
	}
	node := &ast.Node{Type: ast.HaveStmt, Expr: expr, ID: id, Line: t.Line, Col: t.Col}
	p.parseHaveCondition(node)
	if p.checkTok(lexer.IDENTIFIER) || p.checkKw("as") {
		if p.eatIf(lexer.KEYWORD, "as") {
			al, _ := p.eat(lexer.IDENTIFIER, "")
			node.Alias = al.StrVal()
		}
	}
	isGuard := p.eatIf(lexer.KEYWORD, "else")
	node.IsGuard = isGuard
	if isGuard {
		alt, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		node.Alternate = alt
	} else {
		cons, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		node.Consequent = cons
		if p.eatIf(lexer.KEYWORD, "else") {
			alt, err := p.parseBlock()
			if err != nil {
				return nil, err
			}
			node.Alternate = alt
		}
	}
	return node, nil
}

func (p *Parser) parseIfHave() (*ast.Node, error) {
	t := p.advance()
	id := p.ifsetID
	p.ifsetID++
	expr, err := p.parsePostfix()
	if err != nil {
		return nil, err
	}
	node := &ast.Node{Type: ast.IfHaveStmt, Expr: expr, ID: id, Line: t.Line, Col: t.Col}
	p.parseHaveCondition(node)
	if p.eatIf(lexer.KEYWORD, "as") {
		al, _ := p.eat(lexer.IDENTIFIER, "")
		node.Alias = al.StrVal()
	}
	cons, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	node.Consequent = cons
	if p.eatIf(lexer.KEYWORD, "else") {
		alt, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		node.Alternate = alt
	}
	return node, nil
}

func (p *Parser) parseIfSet() (*ast.Node, error) {
	t := p.advance()
	id := p.ifsetID
	p.ifsetID++
	expr, err := p.parsePostfix()
	if err != nil {
		return nil, err
	}
	node := &ast.Node{Type: ast.IfSetStmt, Expr: expr, ID: id, Line: t.Line, Col: t.Col}
	if p.eatIf(lexer.KEYWORD, "as") {
		al, _ := p.eat(lexer.IDENTIFIER, "")
		node.Alias = al.StrVal()
	}
	cons, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	node.Consequent = cons
	if p.eatIf(lexer.KEYWORD, "else") {
		alt, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		node.Alternate = alt
	}
	return node, nil
}

func (p *Parser) parseHaveCondition(node *ast.Node) {
	if p.checkKw("in") {
		p.advance()
		inE, _ := p.parsePostfix()
		node.InExpr = inE
		node.MatchMode = "in"
	} else if p.checkKw("not") && p.peek(1).StrVal() == "in" {
		p.advance()
		p.advance()
		inE, _ := p.parsePostfix()
		node.InExpr = inE
		node.MatchMode = "not_in"
	} else if p.checkKw("matches") {
		p.advance()
		inE, _ := p.parsePostfix()
		node.InExpr = inE
		node.MatchMode = "matches"
	} else if p.checkKw("is") {
		p.advance()
		if p.checkKw("not") {
			p.advance()
			inE, _ := p.parsePostfix()
			node.InExpr = inE
			node.MatchMode = "is_not"
		} else {
			inE, _ := p.parsePostfix()
			node.InExpr = inE
			node.MatchMode = "is"
		}
	} else if p.checkKw("between") {
		p.advance()
		lo, _ := p.parsePostfix()
		hi, _ := p.parsePostfix()
		node.Lo = lo
		node.Hi = hi
		node.MatchMode = "between"
	} else if p.checkKw("startsWith") {
		p.advance()
		inE, _ := p.parsePostfix()
		node.InExpr = inE
		node.MatchMode = "startsWith"
	} else if p.checkKw("endsWith") {
		p.advance()
		inE, _ := p.parsePostfix()
		node.InExpr = inE
		node.MatchMode = "endsWith"
	}
}

func (p *Parser) parseDelete() (*ast.Node, error) {
	t := p.advance()
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	p.eatSemi()
	return &ast.Node{Type: ast.DeleteStmt, Expr: expr, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseWith() (*ast.Node, error) {
	t := p.advance()
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.Node{Type: ast.WithStmt, Expr: expr, Body: body, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseUsing() (*ast.Node, error) {
	t := p.advance()
	name, _ := p.eat(lexer.IDENTIFIER, "")
	p.eat(lexer.OPERATOR, "=")
	init, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	p.eatSemi()
	return &ast.Node{Type: ast.UsingDecl, Name: name.StrVal(), Init: init, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseTypeAlias() (*ast.Node, error) {
	p.advance()
	for !p.isLineEnd() && !p.check(lexer.PUNCTUATION, "{") && !p.check(lexer.EOF, "") {
		if p.check(lexer.PUNCTUATION, "{") {
			p.skipBlock()
			break
		}
		p.advance()
	}
	p.eatSemi()
	return nil, nil
}

func (p *Parser) skipBlock() {
	if !p.check(lexer.PUNCTUATION, "{") {
		return
	}
	p.advance()
	depth := 1
	for depth > 0 && !p.check(lexer.EOF, "") {
		if p.check(lexer.PUNCTUATION, "{") {
			depth++
		} else if p.check(lexer.PUNCTUATION, "}") {
			depth--
		}
		p.advance()
	}
}

func (p *Parser) parseDecorated() (*ast.Node, error) {
	t := p.current()
	var decorators []*ast.Node
	for p.check(lexer.OPERATOR, "@") {
		p.advance()
		dec, err := p.parsePostfix()
		if err != nil {
			return nil, err
		}
		decorators = append(decorators, dec)
	}
	stmt, err := p.parseStmt()
	if err != nil {
		return nil, err
	}
	if stmt == nil {
		return nil, nil
	}
	stmt.Decorators = decorators
	return &ast.Node{Type: ast.DecoratedExpr, Decorators: decorators, Expr: stmt, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseExprAsBlock() (*ast.Node, error) {
	t := p.current()
	stmtKws := map[string]bool{"assert": true, "throw": true, "raise": true, "break": true, "continue": true}
	if stmtKws[t.StrVal()] && t.Type == lexer.KEYWORD {
		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		block := &ast.Node{Type: ast.Block, Line: t.Line, Col: t.Col}
		if stmt != nil {
			block.Body_ = []*ast.Node{stmt}
		}
		return block, nil
	}
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	p.eatSemi()
	block := &ast.Node{Type: ast.Block, Line: t.Line, Col: t.Col}
	block.Body_ = []*ast.Node{{Type: ast.ExprStmt, Expr: expr, Line: t.Line, Col: t.Col}}
	return block, nil
}

func (p *Parser) skipTypeExpr() interface{} {
	depth := 0
	for {
		t := p.current()
		if t.Type == lexer.EOF {
			break
		}
		if t.StrVal() == "<" || t.StrVal() == "[" || t.StrVal() == "(" {
			depth++
		}
		if t.StrVal() == ">" || t.StrVal() == "]" || t.StrVal() == ")" {
			if depth == 0 {
				break
			}
			depth--
		}
		if depth == 0 {
			v := t.StrVal()
			if v == "=" || v == "," || v == "{" || v == "}" || v == ";" || v == ")" {
				break
			}
			if v == "=>" {
				break
			}
		}
		p.advance()
	}
	return nil
}

func (p *Parser) parseExpr() (*ast.Node, error) {
	return p.parseAssignment()
}

func (p *Parser) parseAssignment() (*ast.Node, error) {
	left, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	t := p.current()
	assignOps := map[string]bool{
		"=": true, "+=": true, "-=": true, "*=": true, "/=": true, "%=": true,
		"**=": true, "&&=": true, "||=": true, "??=": true, "<<=": true, ">>=": true,
	}
	if t.Type == lexer.OPERATOR && assignOps[t.StrVal()] {
		op := p.advance().StrVal()
		right, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		return &ast.Node{Type: ast.AssignExpr, Left: left, Op: op, Right: right, Line: t.Line, Col: t.Col}, nil
	}
	return left, nil
}

func (p *Parser) parseTernary() (*ast.Node, error) {
	cond, err := p.parsePipeline()
	if err != nil {
		return nil, err
	}
	if p.check(lexer.OPERATOR, "?") && p.peek(1).StrVal() != "." && p.peek(1).StrVal() != "?" {
		t := p.advance()
		cons, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		p.eat(lexer.OPERATOR, ":")
		alt, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		return &ast.Node{Type: ast.TernaryExpr, Test: cond, Consequent: cons, Alternate: alt, Line: t.Line, Col: t.Col}, nil
	}
	return cond, nil
}

func (p *Parser) parsePipeline() (*ast.Node, error) {
	left, err := p.parseNullCoalesce()
	if err != nil {
		return nil, err
	}
	for p.check(lexer.OPERATOR, "|>") {
		t := p.advance()
		right, err := p.parseNullCoalesce()
		if err != nil {
			return nil, err
		}
		left = &ast.Node{Type: ast.PipelineExpr, Left: left, Right: right, Line: t.Line, Col: t.Col}
	}
	return left, nil
}

func (p *Parser) parseNullCoalesce() (*ast.Node, error) {
	left, err := p.parseLogicalOr()
	if err != nil {
		return nil, err
	}
	for p.check(lexer.OPERATOR, "??") {
		t := p.advance()
		right, err := p.parseLogicalOr()
		if err != nil {
			return nil, err
		}
		left = &ast.Node{Type: ast.BinaryExpr, Left: left, Op: "??", Right: right, Line: t.Line, Col: t.Col}
	}
	return left, nil
}

func (p *Parser) parseLogicalOr() (*ast.Node, error) {
	left, err := p.parseLogicalAnd()
	if err != nil {
		return nil, err
	}
	for p.check(lexer.OPERATOR, "||") || p.check(lexer.IDENTIFIER, "or") {
		t := p.advance()
		right, err := p.parseLogicalAnd()
		if err != nil {
			return nil, err
		}
		left = &ast.Node{Type: ast.BinaryExpr, Left: left, Op: "||", Right: right, Line: t.Line, Col: t.Col}
	}
	return left, nil
}

func (p *Parser) parseLogicalAnd() (*ast.Node, error) {
	left, err := p.parseBitOr()
	if err != nil {
		return nil, err
	}
	for p.check(lexer.OPERATOR, "&&") || p.check(lexer.IDENTIFIER, "and") {
		t := p.advance()
		right, err := p.parseBitOr()
		if err != nil {
			return nil, err
		}
		left = &ast.Node{Type: ast.BinaryExpr, Left: left, Op: "&&", Right: right, Line: t.Line, Col: t.Col}
	}
	return left, nil
}

func (p *Parser) parseBitOr() (*ast.Node, error) {
	left, err := p.parseBitXor()
	if err != nil {
		return nil, err
	}
	for p.check(lexer.OPERATOR, "|") && !p.check(lexer.OPERATOR, "|>") && !p.check(lexer.OPERATOR, "||") {
		t := p.advance()
		right, err := p.parseBitXor()
		if err != nil {
			return nil, err
		}
		left = &ast.Node{Type: ast.BinaryExpr, Left: left, Op: "|", Right: right, Line: t.Line, Col: t.Col}
	}
	return left, nil
}

func (p *Parser) parseBitXor() (*ast.Node, error) {
	left, err := p.parseBitAnd()
	if err != nil {
		return nil, err
	}
	for p.check(lexer.OPERATOR, "^") {
		t := p.advance()
		right, err := p.parseBitAnd()
		if err != nil {
			return nil, err
		}
		left = &ast.Node{Type: ast.BinaryExpr, Left: left, Op: "^", Right: right, Line: t.Line, Col: t.Col}
	}
	return left, nil
}

func (p *Parser) parseBitAnd() (*ast.Node, error) {
	left, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for p.check(lexer.OPERATOR, "&") && !p.check(lexer.OPERATOR, "&&") {
		t := p.advance()
		right, err := p.parseEquality()
		if err != nil {
			return nil, err
		}
		left = &ast.Node{Type: ast.BinaryExpr, Left: left, Op: "&", Right: right, Line: t.Line, Col: t.Col}
	}
	return left, nil
}

func (p *Parser) parseEquality() (*ast.Node, error) {
	left, err := p.parseRelational()
	if err != nil {
		return nil, err
	}
	for {
		t := p.current()
		if t.Type != lexer.OPERATOR {
			break
		}
		op := t.StrVal()
		if op != "===" && op != "!==" && op != "==" && op != "!=" {
			break
		}
		p.advance()
		right, err := p.parseRelational()
		if err != nil {
			return nil, err
		}
		left = &ast.Node{Type: ast.BinaryExpr, Left: left, Op: op, Right: right, Line: t.Line, Col: t.Col}
	}
	return left, nil
}

func (p *Parser) parseRelational() (*ast.Node, error) {
	left, err := p.parseShift()
	if err != nil {
		return nil, err
	}
	for {
		t := p.current()
		if t.Type != lexer.OPERATOR && t.Type != lexer.KEYWORD {
			break
		}
		op := t.StrVal()
		if op == "<" || op == ">" || op == "<=" || op == ">=" {
			p.advance()
			right, err := p.parseShift()
			if err != nil {
				return nil, err
			}
			left = &ast.Node{Type: ast.BinaryExpr, Left: left, Op: op, Right: right, Line: t.Line, Col: t.Col}
		} else if op == "instanceof" || op == "in" {
			p.advance()
			right, err := p.parseShift()
			if err != nil {
				return nil, err
			}
			left = &ast.Node{Type: ast.BinaryExpr, Left: left, Op: op, Right: right, Line: t.Line, Col: t.Col}
		} else {
			break
		}
	}
	return left, nil
}

func (p *Parser) parseShift() (*ast.Node, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	for {
		t := p.current()
		if t.Type != lexer.OPERATOR {
			break
		}
		op := t.StrVal()
		if op != "<<" && op != ">>" && op != ">>>" {
			break
		}
		p.advance()
		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		left = &ast.Node{Type: ast.BinaryExpr, Left: left, Op: op, Right: right, Line: t.Line, Col: t.Col}
	}
	return left, nil
}

func (p *Parser) parseAdditive() (*ast.Node, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for {
		t := p.current()
		if t.Type != lexer.OPERATOR {
			break
		}
		op := t.StrVal()
		if op != "+" && op != "-" {
			break
		}
		p.advance()
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = &ast.Node{Type: ast.BinaryExpr, Left: left, Op: op, Right: right, Line: t.Line, Col: t.Col}
	}
	return left, nil
}

func (p *Parser) parseMultiplicative() (*ast.Node, error) {
	left, err := p.parseExponentiation()
	if err != nil {
		return nil, err
	}
	for {
		t := p.current()
		if t.Type != lexer.OPERATOR {
			break
		}
		op := t.StrVal()
		if op != "*" && op != "/" && op != "%" {
			break
		}
		p.advance()
		right, err := p.parseExponentiation()
		if err != nil {
			return nil, err
		}
		left = &ast.Node{Type: ast.BinaryExpr, Left: left, Op: op, Right: right, Line: t.Line, Col: t.Col}
	}
	return left, nil
}

func (p *Parser) parseExponentiation() (*ast.Node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	if p.check(lexer.OPERATOR, "**") {
		t := p.advance()
		right, err := p.parseExponentiation()
		if err != nil {
			return nil, err
		}
		return &ast.Node{Type: ast.BinaryExpr, Left: left, Op: "**", Right: right, Line: t.Line, Col: t.Col}, nil
	}
	return left, nil
}

func (p *Parser) parseUnary() (*ast.Node, error) {
	t := p.current()
	if t.Type == lexer.OPERATOR {
		op := t.StrVal()
		if op == "!" || op == "-" || op == "+" || op == "~" {
			p.advance()
			arg, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return &ast.Node{Type: ast.UnaryExpr, Op: op, Arg: arg, Prefix: true, Line: t.Line, Col: t.Col}, nil
		}
		if op == "++" || op == "--" {
			p.advance()
			arg, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return &ast.Node{Type: ast.UnaryExpr, Op: op, Arg: arg, Prefix: true, Line: t.Line, Col: t.Col}, nil
		}
		if op == "..." {
			p.advance()
			arg, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return &ast.Node{Type: ast.SpreadExpr, Arg: arg, Line: t.Line, Col: t.Col}, nil
		}
	}
	if t.Type == lexer.KEYWORD {
		switch t.StrVal() {
		case "typeof":
			p.advance()
			arg, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return &ast.Node{Type: ast.TypeofExpr, Arg: arg, Line: t.Line, Col: t.Col}, nil
		case "void":
			p.advance()
			arg, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return &ast.Node{Type: ast.VoidExpr, Arg: arg, Line: t.Line, Col: t.Col}, nil
		case "delete":
			p.advance()
			arg, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return &ast.Node{Type: ast.DeleteExpr, Arg: arg, Line: t.Line, Col: t.Col}, nil
		case "not":
			p.advance()
			arg, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return &ast.Node{Type: ast.NotExpr, Arg: arg, Line: t.Line, Col: t.Col}, nil
		}
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() (*ast.Node, error) {
	base, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	return p.parsePostfixChain(base)
}

func (p *Parser) parsePostfixChain(expr *ast.Node) (*ast.Node, error) {
	for {
		t := p.current()
		if t.Type == lexer.PUNCTUATION && t.StrVal() == "(" {
			p.advance()
			args, err := p.parseArgList()
			if err != nil {
				return nil, err
			}
			expr = &ast.Node{Type: ast.CallExpr, Callee: expr, Args: args, Line: t.Line, Col: t.Col}
			continue
		}
		if t.Type == lexer.OPERATOR && t.StrVal() == "?." && p.peek(1).StrVal() == "(" {
			p.advance()
			p.advance()
			args, err := p.parseArgList()
			if err != nil {
				return nil, err
			}
			expr = &ast.Node{Type: ast.CallExpr, Callee: expr, Args: args, Optional: true, Line: t.Line, Col: t.Col}
			continue
		}
		if t.Type == lexer.PUNCTUATION && t.StrVal() == "[" {
			p.advance()
			prop, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			p.eat(lexer.PUNCTUATION, "]")
			expr = &ast.Node{Type: ast.MemberExpr, Object: expr, Prop: prop, Computed: true, Line: t.Line, Col: t.Col}
			continue
		}
		if t.Type == lexer.OPERATOR && t.StrVal() == "?." && p.peek(1).StrVal() == "[" {
			p.advance()
			p.advance()
			prop, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			p.eat(lexer.PUNCTUATION, "]")
			expr = &ast.Node{Type: ast.MemberExpr, Object: expr, Prop: prop, Computed: true, Optional: true, Line: t.Line, Col: t.Col}
			continue
		}
		if t.Type == lexer.PUNCTUATION && t.StrVal() == "." && p.peek(1).StrVal() != "." {
			p.advance()
			propTok := p.current()
			if propTok.Type != lexer.IDENTIFIER && propTok.Type != lexer.KEYWORD {
				break
			}
			p.advance()
			expr = &ast.Node{Type: ast.MemberExpr, Object: expr, Prop: propTok.StrVal(), Computed: false, Line: t.Line, Col: t.Col}
			continue
		}
		if t.Type == lexer.OPERATOR && t.StrVal() == "?." {
			p.advance()
			propTok := p.current()
			if propTok.Type != lexer.IDENTIFIER && propTok.Type != lexer.KEYWORD {
				break
			}
			p.advance()
			expr = &ast.Node{Type: ast.MemberExpr, Object: expr, Prop: propTok.StrVal(), Computed: false, Optional: true, Line: t.Line, Col: t.Col}
			continue
		}
		if t.Type == lexer.OPERATOR && (t.StrVal() == "++" || t.StrVal() == "--") {
			op := p.advance().StrVal()
			expr = &ast.Node{Type: ast.UnaryExpr, Op: op, Arg: expr, Prefix: false, Line: t.Line, Col: t.Col}
			continue
		}
		break
	}
	return expr, nil
}

func (p *Parser) parseArgList() ([]*ast.Node, error) {
	var args []*ast.Node
	for !p.check(lexer.PUNCTUATION, ")") && !p.check(lexer.EOF, "") {
		arg, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		if !p.check(lexer.PUNCTUATION, ")") {
			p.eatIf(lexer.PUNCTUATION, ",")
		}
	}
	p.eat(lexer.PUNCTUATION, ")")
	return args, nil
}

func (p *Parser) parsePrimary() (*ast.Node, error) {
	t := p.current()

	if t.Type == lexer.KEYWORD {
		switch t.StrVal() {
		case "fn":
			return p.parseFnExpr()
		case "if":
			return p.parseIfExpr()
		case "new":
			return p.parseNew()
		case "this":
			p.advance()
			return &ast.Node{Type: ast.ThisExpr, Line: t.Line, Col: t.Col}, nil
		case "super":
			p.advance()
			return &ast.Node{Type: ast.SuperExpr, Line: t.Line, Col: t.Col}, nil
		case "true":
			p.advance()
			return &ast.Node{Type: ast.BoolLit, Value: true, Line: t.Line, Col: t.Col}, nil
		case "false":
			p.advance()
			return &ast.Node{Type: ast.BoolLit, Value: false, Line: t.Line, Col: t.Col}, nil
		case "null":
			p.advance()
			return &ast.Node{Type: ast.NullLit, Line: t.Line, Col: t.Col}, nil
		case "undefined":
			p.advance()
			return &ast.Node{Type: ast.UndefinedLit, Line: t.Line, Col: t.Col}, nil
		case "void":
			p.advance()
			arg, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return &ast.Node{Type: ast.VoidExpr, Arg: arg, Line: t.Line, Col: t.Col}, nil
		case "assert":
			p.advance()
			return &ast.Node{Type: ast.Identifier, Name: "assert", Line: t.Line, Col: t.Col}, nil
		case "Not":
			p.advance()
			val, err := p.parsePrimary()
			if err != nil {
				return nil, err
			}
			return &ast.Node{Type: ast.NotExpr, Arg: val, Line: t.Line, Col: t.Col}, nil
		case "range":
			p.advance()
			p.eat(lexer.PUNCTUATION, "(")
			args, err := p.parseArgList()
			if err != nil {
				return nil, err
			}
			return &ast.Node{Type: ast.RangeExpr, Args: args, Line: t.Line, Col: t.Col}, nil
		case "sleep":
			p.advance()
			p.eat(lexer.PUNCTUATION, "(")
			ms, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			p.eat(lexer.PUNCTUATION, ")")
			return &ast.Node{Type: ast.SleepExpr, Ms: ms, Line: t.Line, Col: t.Col}, nil
		case "try":
			p.advance()
			if p.check(lexer.OPERATOR, "?") {
				p.advance()
				expr, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				return &ast.Node{Type: ast.TrySafeExpr, Expr: expr, Line: t.Line, Col: t.Col}, nil
			}
			return nil, p.errorf(t, "unexpected 'try' in expression — use 'try? expr' or a 'try { }' block as a statement")
		case "typeof":
			p.advance()
			arg, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return &ast.Node{Type: ast.TypeofExpr, Arg: arg, Line: t.Line, Col: t.Col}, nil
		case "channel":
			p.advance()
			p.eatIf(lexer.PUNCTUATION, "(")
			p.eatIf(lexer.PUNCTUATION, ")")
			return &ast.Node{Type: ast.ChannelExpr, Line: t.Line, Col: t.Col}, nil
		case "match":
			return p.parseMatch()
		case "nax":
			p.advance()
			p.eat(lexer.PUNCTUATION, "(")
			urlTok, err := p.eat(lexer.STRING, "")
			if err != nil {
				return nil, err
			}
			p.eat(lexer.PUNCTUATION, ")")
			return &ast.Node{Type: ast.NaxImportExpr, URL: urlTok.StrVal(), Line: t.Line, Col: t.Col}, nil

		case "repeat", "val", "from", "state", "have", "freeze",
			"immutable", "module", "namespace", "component",
			"macro", "not", "is", "in", "of", "as",
			"get", "set", "static", "abstract", "override",
			"readonly", "interface", "implements", "trait",
			"satisfies", "alias", "enum",
			"require", "export", "import",
			"spawn", "select", "do", "using", "with",
			"delete", "guard", "defer":
			p.advance()
			return &ast.Node{Type: ast.Identifier, Name: t.StrVal(), Line: t.Line, Col: t.Col}, nil
		}
	}

	if t.Type == lexer.REGEX {
		p.advance()
		rv := t.Value.(lexer.RegexVal)
		return &ast.Node{Type: ast.RegexLit, Pattern: rv.Pattern, Flags: rv.Flags, Line: t.Line, Col: t.Col}, nil
	}

	if t.Type == lexer.IDENTIFIER {
		p.advance()
		if p.check(lexer.OPERATOR, "=>") {
			p.advance()
			var body *ast.Node
			var err error
			if p.check(lexer.PUNCTUATION, "{") {
				body, err = p.parseBlock()
			} else {
				body, err = p.parseArrowBody()
			}
			if err != nil {
				return nil, err
			}
			return &ast.Node{Type: ast.ArrowFn, Params: []*ast.Param{{Name: t.StrVal()}}, Body: body, Line: t.Line, Col: t.Col}, nil
		}
		return &ast.Node{Type: ast.Identifier, Name: t.StrVal(), Line: t.Line, Col: t.Col}, nil
	}

	if t.Type == lexer.NUMBER {
		p.advance()
		return &ast.Node{Type: ast.NumberLit, Value: t.StrVal(), Line: t.Line, Col: t.Col}, nil
	}
	if t.Type == lexer.STRING {
		p.advance()
		return &ast.Node{Type: ast.StringLit, Value: t.Value.(string), Line: t.Line, Col: t.Col}, nil
	}
	if t.Type == lexer.TEMPLATE {
		p.advance()
		return &ast.Node{Type: ast.TemplateLit, Parts: t.Value.(string), Line: t.Line, Col: t.Col}, nil
	}

	if t.Type == lexer.PUNCTUATION {
		switch t.StrVal() {
		case "(":
			p.advance()
			if p.check(lexer.PUNCTUATION, ")") {
				p.advance()
				if p.check(lexer.OPERATOR, "=>") {
					p.advance()
					body, err := p.parseArrowBodyOrBlock()
					if err != nil {
						return nil, err
					}
					return &ast.Node{Type: ast.ArrowFn, Params: nil, Body: body, Line: t.Line, Col: t.Col}, nil
				}
				return &ast.Node{Type: ast.ArrayLit, Line: t.Line, Col: t.Col}, nil
			}
			if p.lookAheadArrow() {
				params, err := p.parseArrowParams()
				if err != nil {
					return nil, err
				}
				p.eat(lexer.OPERATOR, "=>")
				body, err := p.parseArrowBodyOrBlock()
				if err != nil {
					return nil, err
				}
				return &ast.Node{Type: ast.ArrowFn, Params: params, Body: body, Line: t.Line, Col: t.Col}, nil
			}
			expr, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if p.check(lexer.PUNCTUATION, ",") {
				exprs := []*ast.Node{expr}
				for p.eatIf(lexer.PUNCTUATION, ",") {
					e, err := p.parseExpr()
					if err != nil {
						return nil, err
					}
					exprs = append(exprs, e)
				}
				p.eat(lexer.PUNCTUATION, ")")
				return &ast.Node{Type: ast.SequenceExpr, Exprs: exprs, Line: t.Line, Col: t.Col}, nil
			}
			p.eat(lexer.PUNCTUATION, ")")
			return expr, nil
		case "[":
			return p.parseArrayLit()
		case "{":
			return p.parseObjectLit()
		}
	}

	if t.Type == lexer.OPERATOR && t.StrVal() == "@" {
		next := p.peek(1).StrVal()
		if next == "import" || next == "fimport" {
			return p.parseAtImport()
		}
		return p.parseDecorated()
	}

	if t.Type == lexer.KEYWORD && t.StrVal() == "struct" {
		return p.parseStructLit()
	}

	return nil, p.errorf(t, "unexpected token '%s' (%s)", t.StrVal(), t.Type)
}

func (p *Parser) parseAtImport() (*ast.Node, error) {
	t := p.current()
	p.advance()
	forceLocal := false

	if p.check(lexer.KEYWORD, "import") || p.check(lexer.IDENTIFIER, "import") {
		p.advance()
	} else if p.check(lexer.KEYWORD, "fimport") || p.check(lexer.IDENTIFIER, "fimport") {
		forceLocal = true
		p.advance()
	} else {
		return nil, p.errorf(p.current(), "expected 'import' after '@'")
	}
	if _, err := p.eat(lexer.PUNCTUATION, "("); err != nil {
		return nil, p.errorf(p.current(), "@import requires parentheses — e.g. @import(\"std.io\")")
	}
	pathTok, err := p.eat(lexer.STRING, "")
	if err != nil {
		return nil, p.errorf(p.current(), "@import requires a string module path — e.g. @import(\"std.io\")")
	}
	if _, err := p.eat(lexer.PUNCTUATION, ")"); err != nil {
		return nil, err
	}
	node := &ast.Node{Type: ast.AtImportExpr, Source: pathTok.Value.(string), Line: t.Line, Col: t.Col}
	if forceLocal {
		node.Prop = "force-local"
	}
	return node, nil
}
func (p *Parser) parseStructLit() (*ast.Node, error) {
	t := p.advance()
	node := &ast.Node{Type: ast.StructLit, Line: t.Line, Col: t.Col}
	if _, err := p.eat(lexer.PUNCTUATION, "{"); err != nil {
		return nil, err
	}
	for !p.check(lexer.PUNCTUATION, "}") && !p.check(lexer.EOF, "") {
		p.eatSemi()
		if p.check(lexer.PUNCTUATION, "}") {
			break
		}
		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			node.Body_ = append(node.Body_, stmt)
		}
	}
	if _, err := p.eat(lexer.PUNCTUATION, "}"); err != nil {
		return nil, err
	}
	return node, nil
}

func (p *Parser) parseFnExpr() (*ast.Node, error) {
	t := p.current()
	p.advance()
	node := &ast.Node{Type: ast.FnExpr, Line: t.Line, Col: t.Col}
	if p.checkTok(lexer.IDENTIFIER) && !p.check(lexer.PUNCTUATION, "(") {
		node.Name = p.advance().StrVal()
	}
	params, err := p.parseFnParams()
	if err != nil {
		return nil, err
	}
	node.Params = params
	if p.eatIf(lexer.OPERATOR, "->") || p.eatIf(lexer.OPERATOR, ":") {
		p.skipTypeExpr()
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	node.Body = body
	return node, nil
}

func (p *Parser) parseNew() (*ast.Node, error) {
	t := p.advance()
	callee, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.check(lexer.PUNCTUATION, ".") && p.peek(1).StrVal() != "." {
		p.advance()
		propTok := p.current()
		p.advance()
		callee = &ast.Node{Type: ast.MemberExpr, Object: callee, Prop: propTok.StrVal(), Line: t.Line, Col: t.Col}
	}
	var args []*ast.Node
	if p.check(lexer.PUNCTUATION, "(") {
		p.advance()
		var err error
		args, err = p.parseArgList()
		if err != nil {
			return nil, err
		}
	}
	return &ast.Node{Type: ast.NewExpr, Callee: callee, Args: args, Line: t.Line, Col: t.Col}, nil
}

func (p *Parser) parseArrayLit() (*ast.Node, error) {
	t := p.advance()
	node := &ast.Node{Type: ast.ArrayLit, Line: t.Line, Col: t.Col}
	for !p.check(lexer.PUNCTUATION, "]") && !p.check(lexer.EOF, "") {
		if p.check(lexer.PUNCTUATION, ",") {
			p.advance()
			node.Elements = append(node.Elements, nil)
			continue
		}
		el, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		node.Elements = append(node.Elements, el)
		if !p.check(lexer.PUNCTUATION, "]") {
			p.eatIf(lexer.PUNCTUATION, ",")
		}
	}
	p.eat(lexer.PUNCTUATION, "]")
	return node, nil
}

func (p *Parser) parseObjectLit() (*ast.Node, error) {
	t := p.advance()
	node := &ast.Node{Type: ast.ObjectLit, Line: t.Line, Col: t.Col}
	for !p.check(lexer.PUNCTUATION, "}") && !p.check(lexer.EOF, "") {
		if p.check(lexer.OPERATOR, "...") {
			p.advance()
			arg, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			node.Properties = append(node.Properties, &ast.ObjProp{Kind: "spread", Arg: arg})
			p.eatIf(lexer.PUNCTUATION, ",")
			continue
		}
		var key interface{}
		computed := false
		if p.check(lexer.PUNCTUATION, "[") {
			p.advance()
			keyExpr, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			key = keyExpr
			computed = true
			p.eat(lexer.PUNCTUATION, "]")
		} else {
			keyTok := p.current()
			if keyTok.Type != lexer.IDENTIFIER && keyTok.Type != lexer.KEYWORD && keyTok.Type != lexer.STRING && keyTok.Type != lexer.NUMBER {
				break
			}
			key = keyTok.StrVal()
			p.advance()
		}
		keyStr, isStr := key.(string)
		if isStr && (keyStr == "get" || keyStr == "set") && p.checkTok(lexer.IDENTIFIER) {
			isGet := keyStr == "get"
			isSet := keyStr == "set"
			mname := p.advance().StrVal()
			params, err := p.parseFnParams()
			if err != nil {
				return nil, err
			}
			body, err := p.parseBlock()
			if err != nil {
				return nil, err
			}
			node.Properties = append(node.Properties, &ast.ObjProp{Kind: "method", Key: mname, Params: params, Body: body, IsGet: isGet, IsSet: isSet})
			p.eatIf(lexer.PUNCTUATION, ",")
			continue
		}
		if p.check(lexer.PUNCTUATION, "(") {
			params, err := p.parseFnParams()
			if err != nil {
				return nil, err
			}
			body, err := p.parseBlock()
			if err != nil {
				return nil, err
			}
			node.Properties = append(node.Properties, &ast.ObjProp{Kind: "method", Key: key, Params: params, Body: body, Computed: computed})
			p.eatIf(lexer.PUNCTUATION, ",")
			continue
		}
		if p.eatIf(lexer.OPERATOR, ":") {
			val, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			node.Properties = append(node.Properties, &ast.ObjProp{Kind: "prop", Key: key, Value: val, Computed: computed})
		} else {
			node.Properties = append(node.Properties, &ast.ObjProp{Kind: "shorthand", Key: key})
		}
		p.eatIf(lexer.PUNCTUATION, ",")
	}
	if _, err := p.eat(lexer.PUNCTUATION, "}"); err != nil {
		return nil, err
	}
	return node, nil
}

func (p *Parser) parseArrowBody() (*ast.Node, error) {
	t := p.current()
	if t.Type == lexer.KEYWORD {
		switch t.StrVal() {
		case "if", "unless", "throw", "raise", "var", "val", "let", "const":
			return p.parseStmt()
		}
	}
	return p.parseExpr()
}

func (p *Parser) parseArrowBodyOrBlock() (*ast.Node, error) {
	if p.check(lexer.PUNCTUATION, "{") {
		return p.parseBlock()
	}
	return p.parseArrowBody()
}

func (p *Parser) lookAheadArrow() bool {
	depth := 0
	for i := p.pos; i < len(p.tokens); i++ {
		tok := p.tokens[i]
		v := tok.StrVal()
		if v == "(" || v == "[" {
			depth++
		}
		if v == ")" || v == "]" {
			if depth == 0 {
				if i+1 < len(p.tokens) {
					next := p.tokens[i+1]
					return next.Type == lexer.OPERATOR && next.StrVal() == "=>"
				}
				return false
			}
			depth--
		}
	}
	return false
}

func (p *Parser) parseArrowParams() ([]*ast.Param, error) {
	var params []*ast.Param
	for !p.check(lexer.PUNCTUATION, ")") && !p.check(lexer.EOF, "") {
		param := &ast.Param{ResolvedSlot: -1}
		if p.check(lexer.OPERATOR, "...") {
			p.advance()
			param.Rest = true
		}
		if p.checkTok(lexer.IDENTIFIER) {
			param.Name = p.advance().StrVal()
		} else {
			param.Name = "_"
		}
		p.eatIf(lexer.OPERATOR, "?")
		if p.eatIf(lexer.OPERATOR, ":") {
			p.skipTypeExpr()
		}
		if !param.Rest && p.eatIf(lexer.OPERATOR, "=") {
			defVal, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			param.DefaultVal = defVal
		}
		params = append(params, param)
		if !p.check(lexer.PUNCTUATION, ")") {
			p.eatIf(lexer.PUNCTUATION, ",")
		}
	}
	p.eat(lexer.PUNCTUATION, ")")
	return params, nil
}

func Parse(tokens []lexer.Token, filename string) (*ast.Node, error) {
	return New(tokens, filename).Parse()
}

func ParseWithLines(tokens []lexer.Token, filename string, lines []string) (*ast.Node, error) {
	return NewWithLines(tokens, filename, lines).Parse()
}

var _ = strconv.Itoa
