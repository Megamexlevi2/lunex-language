package lsp

import (
	"encoding/json"
	"lunex/internal/lexer"
	"sort"
	"strings"
	"unicode/utf16"
)

var semanticTokenTypes = []string{"namespace", "type", "class", "enum", "interface", "struct", "typeParameter", "parameter", "variable", "property", "enumMember", "event", "function", "method", "macro", "keyword", "modifier", "comment", "string", "number", "regexp", "operator", "decorator"}
var semanticTokenModifiers = []string{"declaration", "readonly", "static", "deprecated", "abstract", "async", "modification", "documentation", "defaultLibrary"}

func (s *Server) semanticTokens(params map[string]json.RawMessage) any {
	var req struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	decodeField(params, "textDocument", &req.TextDocument)
	doc := s.ensureDoc(req.TextDocument.URI)
	if doc == nil {
		return semanticTokens{Data: []uint32{}}
	}
	type entry struct {
		line   uint32
		char   uint32
		length uint32
		typeID uint32
		mods   uint32
	}
	entries := make([]entry, 0, len(doc.Tokens))
	symbolByIndex := map[int]*Symbol{}
	for _, sym := range doc.Symbols {
		if sym.TokenIndex >= 0 {
			symbolByIndex[sym.TokenIndex] = sym
		}
	}
	for i, token := range doc.Tokens {
		if token.Type == lexer.EOF || token.Type == lexer.PUNCTUATION {
			continue
		}
		r := tokenRange(doc, i)
		length := uint32(r.End.Character - r.Start.Character)
		if length == 0 {
			length = uint32(len(utf16.Encode([]rune(token.StrVal()))))
		}
		if length == 0 {
			continue
		}
		typeID, mods := semanticClassification(doc, token, i, symbolByIndex)
		if typeID < 0 {
			continue
		}
		entries = append(entries, entry{line: uint32(r.Start.Line), char: uint32(r.Start.Character), length: length, typeID: uint32(typeID), mods: mods})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].line == entries[j].line {
			return entries[i].char < entries[j].char
		}
		return entries[i].line < entries[j].line
	})
	data := make([]uint32, 0, len(entries)*5)
	var previousLine uint32
	var previousChar uint32
	for i, e := range entries {
		deltaLine := e.line
		deltaChar := e.char
		if i > 0 {
			deltaLine = e.line - previousLine
			if deltaLine == 0 {
				deltaChar = e.char - previousChar
			}
		}
		data = append(data, deltaLine, deltaChar, e.length, e.typeID, e.mods)
		previousLine = e.line
		previousChar = e.char
	}
	return semanticTokens{Data: data}
}

func semanticClassification(doc *Document, token lexer.Token, index int, symbols map[int]*Symbol) (int, uint32) {
	modifiers := uint32(0)
	if sym := symbols[index]; sym != nil {
		modifiers |= 1
		if sym.Kind == "Variable" && sym.Node != nil && sym.Node.IsConst {
			modifiers |= 1 << 1
		}
		switch sym.Kind {
		case "Namespace", "Import":
			return 0, modifiers
		case "Class":
			return 2, modifiers
		case "Enum":
			return 3, modifiers
		case "Function", "Component":
			return 12, modifiers
		case "Method":
			return 13, modifiers
		case "Field":
			return 9, modifiers
		case "EnumMember":
			return 10, modifiers
		case "Parameter":
			return 7, modifiers
		default:
			return 8, modifiers
		}
	}
	if index > 0 && doc.Tokens[index-1].StrVal() == "." && token.Type == lexer.IDENTIFIER {
		return 9, 0
	}
	switch token.Type {
	case lexer.KEYWORD:
		return 15, 0
	case lexer.STRING, lexer.TEMPLATE:
		return 18, 0
	case lexer.NUMBER:
		return 19, 0
	case lexer.REGEX:
		return 20, 0
	case lexer.OPERATOR:
		return 21, 0
	case lexer.IDENTIFIER:
		if isBuiltinSemanticName(token.StrVal()) {
			return 12, 1 << 8
		}
		if strings.HasPrefix(token.StrVal(), "_") {
			return 8, 0
		}
		return 8, 0
	}
	return -1, 0
}

func isBuiltinSemanticName(name string) bool {
	for _, item := range builtinCompletionNames {
		if item == name {
			return true
		}
	}
	return false
}
