package lexer

import (
	"fmt"
	"strings"
	"unicode"
)

type TokenType string

const (
	KEYWORD     TokenType = "KEYWORD"
	IDENTIFIER  TokenType = "IDENTIFIER"
	NUMBER      TokenType = "NUMBER"
	STRING      TokenType = "STRING"
	TEMPLATE    TokenType = "TEMPLATE"
	OPERATOR    TokenType = "OPERATOR"
	PUNCTUATION TokenType = "PUNCTUATION"
	REGEX       TokenType = "REGEX"
	EOF         TokenType = "EOF"
)

var builtinKeywords = map[string]bool{
	"var": true, "val": true,
	"fn": true,
	"if": true, "else": true, "elif": true, "unless": true,
	"while": true, "for": true, "loop": true, "in": true, "of": true,
	"break": true, "continue": true, "do": true, "repeat": true,
	"each": true, "guard": true, "defer": true,
	"match": true, "case": true, "default": true, "when": true,
	"try": true, "catch": true, "finally": true, "raise": true, "throw": true,
	"extends": true, "new": true, "this": true, "super": true,
	"abstract": true, "override": true, "static": true,
	"get": true, "set": true, "private": true, "public": true, "protected": true,
	"readonly": true, "interface": true, "implements": true, "trait": true,
	"typeof": true, "instanceof": true, "keyof": true, "infer": true,
	"alias": true, "enum": true, "satisfies": true,
	"import": true, "export": true, "from": true, "as": true,
	"require": true, "module": true, "namespace": true,
	"true": true, "false": true, "null": true, "void": true, "undefined": true,
	"range": true, "sleep": true, "have": true,
	"ifhave": true, "ifset": true, "between": true, "matches": true,
	"startsWith": true, "endsWith": true, "is": true, "not": true,
	"nax": true, "lunex": true,
	"spawn": true, "select": true, "channel": true, "component": true,
	"macro": true, "immutable": true, "freeze": true,
	"with": true, "using": true, "assert": true, "delete": true, "watch": true,
	"use": true, "struct": true,
	"Not": true,
}

var multiCharOps = []string{
	"===", "!==", "<<=", ">>=", "**=", "&&=", "||=", "??=",
	">>>", "==", "!=", "<=", ">=", "&&", "||", "??", "|>",
	"=>", "->", "++", "--", "+=", "-=", "*=", "/=", "%=",
	"<<", ">>", "?.", "...", "::", "**", "..",
}

var singleOps = map[rune]bool{
	'+': true, '-': true, '*': true, '/': true, '%': true,
	'=': true, '<': true, '>': true, '!': true, '&': true,
	'|': true, '^': true, '~': true, '?': true, ':': true,
	'@': true, '#': true,
}

var punctuation = map[rune]bool{
	'{': true, '}': true, '(': true, ')': true,
	'[': true, ']': true, ',': true, '.': true, ';': true,
}

type RegexVal struct {
	Pattern string
	Flags   string
}

type Token struct {
	Type  TokenType
	Value interface{}
	Line  int
	Col   int
	Raw   string
}

func (t Token) StrVal() string {
	if s, ok := t.Value.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", t.Value)
}

type Lexer struct {
	source   []rune
	filename string
	pos      int
	line     int
	col      int
	tokens   []Token
}

func NewLexer(source, filename string) *Lexer {
	return &Lexer{
		source:   []rune(source),
		filename: filename,
		pos:      0,
		line:     1,
		col:      1,
	}
}

func (l *Lexer) peek(offset int) rune {
	i := l.pos + offset
	if i >= len(l.source) {
		return 0
	}
	return l.source[i]
}

func (l *Lexer) advance() rune {
	if l.pos >= len(l.source) {
		return 0
	}
	ch := l.source[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

func (l *Lexer) matchStr(s string) bool {
	r := []rune(s)
	if l.pos+len(r) > len(l.source) {
		return false
	}
	for i, c := range r {
		if l.source[l.pos+i] != c {
			return false
		}
	}
	l.pos += len(r)
	l.col += len(r)
	return true
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.source) {
		ch := l.peek(0)
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			l.advance()
			continue
		}
		if ch == '/' && l.peek(1) == '/' {
			for l.pos < len(l.source) && l.peek(0) != '\n' {
				l.advance()
			}
			continue
		}
		if ch == '/' && l.peek(1) == '*' {
			l.advance()
			l.advance()
			for l.pos < len(l.source) {
				if l.peek(0) == '*' && l.peek(1) == '/' {
					l.advance()
					l.advance()
					break
				}
				l.advance()
			}
			continue
		}
		if ch == '#' {
			for l.pos < len(l.source) && l.peek(0) != '\n' {
				l.advance()
			}
			continue
		}
		break
	}
}

func (l *Lexer) readString(quote rune) string {
	l.advance()
	var buf strings.Builder
	for l.pos < len(l.source) && l.peek(0) != quote {
		if l.peek(0) == '\\' {
			l.advance()
			esc := l.advance()
			switch esc {
			case 'n':
				buf.WriteRune('\n')
			case 't':
				buf.WriteRune('\t')
			case 'r':
				buf.WriteRune('\r')
			case '\\':
				buf.WriteRune('\\')
			case '\'':
				buf.WriteRune('\'')
			case '"':
				buf.WriteRune('"')
			case '`':
				buf.WriteRune('`')
			case '0':
				buf.WriteRune(0)
			case 'b':
				buf.WriteRune('\b')
			case 'f':
				buf.WriteRune('\f')
			case 'u':
				if l.peek(0) == '{' {
					l.advance()
					var hex strings.Builder
					for l.peek(0) != '}' {
						hex.WriteRune(l.advance())
					}
					l.advance()
					var cp int32
					fmt.Sscanf(hex.String(), "%x", &cp)
					buf.WriteRune(rune(cp))
				} else {
					var hex strings.Builder
					for i := 0; i < 4; i++ {
						hex.WriteRune(l.advance())
					}
					var cp int32
					fmt.Sscanf(hex.String(), "%x", &cp)
					buf.WriteRune(rune(cp))
				}
			case 'x':
				var hex strings.Builder
				for i := 0; i < 2; i++ {
					hex.WriteRune(l.advance())
				}
				var cp int32
				fmt.Sscanf(hex.String(), "%x", &cp)
				buf.WriteRune(rune(cp))
			default:
				buf.WriteRune(esc)
			}
		} else {
			buf.WriteRune(l.advance())
		}
	}
	l.advance()
	return buf.String()
}

func (l *Lexer) readTemplate() string {
	l.advance()
	var buf strings.Builder
	for l.pos < len(l.source) && l.peek(0) != '`' {
		if l.peek(0) == '$' && l.peek(1) == '{' {
			buf.WriteString("${")
			l.advance()
			l.advance()
			depth := 1
			for l.pos < len(l.source) && depth > 0 {
				ch := l.peek(0)
				if ch == '{' {
					depth++
				} else if ch == '}' {
					depth--
				}
				if depth > 0 {
					buf.WriteRune(l.advance())
				} else {
					l.advance()
				}
			}
			buf.WriteRune('}')
		} else if l.peek(0) == '\\' {
			l.advance()
			esc := l.advance()
			switch esc {
			case 'n':
				buf.WriteString("\\n")
			case 't':
				buf.WriteString("\\t")
			case 'r':
				buf.WriteString("\\r")
			case '\\':
				buf.WriteString("\\\\")
			case '`':
				buf.WriteString("\\`")
			default:
				buf.WriteRune(esc)
			}
		} else {
			buf.WriteRune(l.advance())
		}
	}
	l.advance()
	return buf.String()
}

func (l *Lexer) readNumber() string {
	start := l.pos
	if l.peek(0) == '0' {
		next := l.peek(1)
		if next == 'x' || next == 'X' {
			l.advance()
			l.advance()
			for isHexDigit(l.peek(0)) || l.peek(0) == '_' {
				l.advance()
			}
			return string(l.source[start:l.pos])
		}
		if next == 'o' || next == 'O' {
			l.advance()
			l.advance()
			for (l.peek(0) >= '0' && l.peek(0) <= '7') || l.peek(0) == '_' {
				l.advance()
			}
			return string(l.source[start:l.pos])
		}
		if next == 'b' || next == 'B' {
			l.advance()
			l.advance()
			for l.peek(0) == '0' || l.peek(0) == '1' || l.peek(0) == '_' {
				l.advance()
			}
			return string(l.source[start:l.pos])
		}
	}
	for isDigit(l.peek(0)) || l.peek(0) == '_' {
		l.advance()
	}
	if l.peek(0) == '.' && isDigit(l.peek(1)) {
		l.advance()
		for isDigit(l.peek(0)) || l.peek(0) == '_' {
			l.advance()
		}
	}
	if l.peek(0) == 'e' || l.peek(0) == 'E' {
		l.advance()
		if l.peek(0) == '+' || l.peek(0) == '-' {
			l.advance()
		}
		for isDigit(l.peek(0)) {
			l.advance()
		}
	}
	if l.peek(0) == 'n' {
		l.advance()
	}
	return string(l.source[start:l.pos])
}

func (l *Lexer) readIdentifier() string {
	start := l.pos
	for isIdentPart(l.peek(0)) {
		l.advance()
	}
	return string(l.source[start:l.pos])
}

func (l *Lexer) readRegex() RegexVal {
	l.advance()
	var pattern strings.Builder
	inClass := false
	for l.pos < len(l.source) {
		ch := l.peek(0)
		if ch == '\\' {
			pattern.WriteRune(ch)
			l.advance()
			pattern.WriteRune(l.advance())
			continue
		}
		if ch == '[' {
			inClass = true
			pattern.WriteRune(l.advance())
			continue
		}
		if ch == ']' {
			inClass = false
			pattern.WriteRune(l.advance())
			continue
		}
		if ch == '/' && !inClass {
			l.advance()
			break
		}
		if ch == '\n' {
			break
		}
		pattern.WriteRune(l.advance())
	}
	var flags strings.Builder
	for {
		ch := l.peek(0)
		if ch == 'g' || ch == 'i' || ch == 'm' || ch == 's' || ch == 'u' || ch == 'y' {
			flags.WriteRune(l.advance())
		} else {
			break
		}
	}
	return RegexVal{Pattern: pattern.String(), Flags: flags.String()}
}

func (l *Lexer) couldBeRegex() bool {
	if len(l.tokens) == 0 {
		return true
	}
	prev := l.tokens[len(l.tokens)-1]
	if prev.Type == NUMBER || prev.Type == STRING || prev.Type == TEMPLATE {
		return false
	}
	if prev.Type == IDENTIFIER {
		return false
	}
	if prev.Type == KEYWORD {
		v := prev.StrVal()
		if v == "this" || v == "null" || v == "true" || v == "false" {
			return false
		}
	}
	if prev.Type == PUNCTUATION {
		v := prev.StrVal()
		if v == ")" || v == "]" {
			return false
		}
	}
	return true
}

func (l *Lexer) Tokenize() ([]Token, error) {
	for l.pos < len(l.source) {
		l.skipWhitespace()
		if l.pos >= len(l.source) {
			break
		}
		line := l.line
		col := l.col
		ch := l.peek(0)

		if isDigit(ch) || (ch == '.' && isDigit(l.peek(1))) {
			val := l.readNumber()
			l.tokens = append(l.tokens, Token{Type: NUMBER, Value: val, Line: line, Col: col, Raw: val})
			continue
		}

		if ch == '"' || ch == '\'' {
			val := l.readString(ch)
			l.tokens = append(l.tokens, Token{Type: STRING, Value: val, Line: line, Col: col})
			continue
		}

		if ch == '`' {
			val := l.readTemplate()
			l.tokens = append(l.tokens, Token{Type: TEMPLATE, Value: val, Line: line, Col: col})
			continue
		}

		if isIdentStart(ch) {
			word := l.readIdentifier()
			tt := IDENTIFIER
			if builtinKeywords[word] {
				tt = KEYWORD
			}
			l.tokens = append(l.tokens, Token{Type: tt, Value: word, Line: line, Col: col})
			continue
		}

		if ch == '/' && l.couldBeRegex() && l.peek(1) != '/' && l.peek(1) != '*' {
			rv := l.readRegex()
			l.tokens = append(l.tokens, Token{Type: REGEX, Value: rv, Line: line, Col: col})
			continue
		}

		matched := false
		for _, op := range multiCharOps {
			runes := []rune(op)
			ok := true
			if l.pos+len(runes) > len(l.source) {
				continue
			}
			for i, r := range runes {
				if l.source[l.pos+i] != r {
					ok = false
					break
				}
			}
			if ok {
				l.pos += len(runes)
				l.col += len(runes)
				l.tokens = append(l.tokens, Token{Type: OPERATOR, Value: op, Line: line, Col: col})
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		if singleOps[ch] {
			l.advance()
			l.tokens = append(l.tokens, Token{Type: OPERATOR, Value: string(ch), Line: line, Col: col})
			continue
		}

		if punctuation[ch] {
			l.advance()
			l.tokens = append(l.tokens, Token{Type: PUNCTUATION, Value: string(ch), Line: line, Col: col})
			continue
		}

		if unicode.IsSpace(ch) {
			l.advance()
			continue
		}

		l.advance()
	}
	l.tokens = append(l.tokens, Token{Type: EOF, Value: "", Line: l.line, Col: l.col})
	return l.tokens, nil
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func isHexDigit(ch rune) bool {
	return isDigit(ch) || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func isIdentStart(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' || ch == '$'
}

func isIdentPart(ch rune) bool {
	return isIdentStart(ch) || isDigit(ch)
}

func Tokenize(source, filename string) ([]Token, error) {
	return NewLexer(source, filename).Tokenize()
}
