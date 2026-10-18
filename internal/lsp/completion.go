package lsp

import (
	"encoding/json"
	"fmt"
	"lunex/internal/ast"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	completionText        = 1
	completionMethod      = 2
	completionFunction    = 3
	completionConstructor = 4
	completionField       = 5
	completionVariable    = 6
	completionClass       = 7
	completionEnum        = 13
	completionInterface   = 8
	completionModule      = 9
	completionProperty    = 10
	completionKeyword     = 14
	completionSnippet     = 15
)

var stdModuleMembers = map[string][]string{
	"std.io":       {"log", "err", "warn", "info", "success", "read", "readLine", "readInt", "clear", "table", "progress", "spinner", "tick", "stop", "format", "write", "red", "green", "yellow", "blue", "magenta", "cyan", "white", "gray", "bold", "dim", "italic", "color", "strip", "newline", "hr", "banner", "json", "isTerminal"},
	"std.fs":       {"readFile", "writeFile", "appendFile", "delete", "deleteAll", "exists", "mkdir", "rmdir", "rename", "moveFile", "copy", "copyFile", "list", "isDir", "isFile", "size", "stat", "readDir", "readLines", "readJSON", "writeJSON", "readCSV", "writeCSV", "tempFile", "tempDir", "abs", "join", "dirname", "basename", "extname", "glob", "cwd", "home", "ensureDir"},
	"std.math":     {"PI", "E", "LN2", "LN10", "LOG2E", "LOG10E", "SQRT2", "SQRT1_2", "PHI", "Infinity", "NaN", "sin", "cos", "tan", "asin", "acos", "atan", "atan2", "sinh", "cosh", "tanh", "asinh", "acosh", "atanh", "log", "log2", "log10", "exp", "exp2", "expm1", "log1p", "sqrt", "cbrt", "pow", "hypot", "abs", "ceil", "floor", "round", "trunc", "sign", "max", "min", "clamp", "lerp", "random", "randomInt", "seed", "gcd", "lcm", "isPrime", "factorial", "combinations", "permutations", "degToRad", "radToDeg", "isNaN", "isFinite", "isInfinite", "sum", "product", "mean", "variance", "stdDev", "fib", "primes", "toBinary", "toHex", "toOctal"},
	"std.crypto":   {"hash", "hmac", "md5", "sha1", "sha256", "sha512", "randomBytes", "randomHex", "randomUUID", "token", "encrypt", "decrypt", "toHex", "fromHex", "pbkdf2", "hashPassword", "verifyPassword", "jwt", "compare"},
	"std.encoding": {"utf8Encode", "utf8Decode", "utf16Encode", "utf16Decode", "utf32Encode", "utf32Decode", "latin1Encode", "latin1Decode", "base64Encode", "base64Decode", "base64DecodeText", "base64UrlEncode", "base64UrlDecode", "base64UrlDecodeText", "base32Encode", "base32Decode", "base32DecodeText", "percentEncode", "percentDecode", "formEncode", "formDecode", "encode", "decode"},
	"std.datetime": {"now", "utcNow", "fromTimestamp", "toTimestamp", "parse", "format", "diff", "add", "subtract", "isBefore", "isAfter", "isEqual", "compare", "startOf", "endOf", "day", "hour", "minute", "month", "ms", "second", "timestamp", "unix", "year", "weekday", "weekdayName", "monthName", "dayOfYear", "weekOfYear", "daysInMonth", "isLeapYear", "isWeekend", "isValid", "timezone", "sleep", "fromParts"},
	"std.regex":    {"compile", "count", "escape", "extractEmails", "extractNumbers", "extractUrls", "groups", "groupsAll", "index", "indices", "isValid", "match", "matchAll", "namedGroups", "replace", "replaceAll", "replaceFunc", "split", "test"},
	"std.buffer":   {"alloc", "from", "concat", "copyFrom", "fill", "length", "readF32", "readF64", "readI16", "readI32", "readI64", "readI8", "readU16", "readU32", "readU64", "readU8", "resize", "slice", "toArray", "toBase64", "toHex", "toString", "writeF32", "writeF64", "writeI16", "writeI32", "writeI64", "writeI8", "writeU16", "writeU32", "writeU64", "writeU8"},
	"std.ints":     {"U8_MAX", "I8_MAX", "I8_MIN", "U16_MAX", "I16_MAX", "I16_MIN", "U32_MAX", "I32_MAX", "I32_MIN", "U64_MAX", "I64_MAX", "I64_MIN", "u8", "i8", "u16", "i16", "u32", "i32", "u64", "i64", "addU8", "addI8", "addU16", "addI16", "addU32", "addI32", "addU64", "addI64", "subU8", "subI8", "subU16", "subI16", "subU32", "subI32", "subU64", "subI64", "mulU8", "mulI8", "mulU16", "mulI16", "mulU32", "mulI32", "mulU64", "mulI64", "shlU32", "shrU32", "shlU64", "shrU64", "rotlU32", "rotrU32", "isU8Overflow", "isI32Overflow"},
	"std.image":    {"blur", "brightness", "contrast", "create", "crop", "decode", "encode", "flipH", "flipV", "gray", "invert", "open", "paste", "pixel", "resize", "rotate", "saturation", "save", "setPixel", "sharpen"},
	"std.utils":    {"sleep", "now", "timestamp", "noop", "identity", "range", "chunk", "flatten", "flatMap", "zip", "unzip", "intersection", "difference", "union", "uniq", "uniqBy", "groupBy", "countBy", "partition", "sortBy", "pick", "omit", "merge", "assign", "keys", "values", "entries", "fromEntries", "has", "invert", "mapValues", "sum", "mean", "median", "min", "max", "clamp", "lerp", "random", "randInt", "shuffle", "sample", "sampleSize", "camelCase", "snakeCase", "kebabCase", "titleCase", "slugify", "truncate", "pad", "padStart", "padEnd", "repeat", "template", "times", "pipe", "compose", "memoize", "once", "negate", "formatNumber", "formatBytes", "uuid", "isEmail", "isUrl", "isNumeric", "toNumber", "toString", "clone", "equal", "type", "isEmpty", "isNil"},
	"std.net":      {"dial", "connect", "dialTimeout", "dialTCP", "dialUDP", "listen", "listenTCP", "listenPacket", "listenUDP", "dialTLS", "connectTLS", "listenTLS", "lookupIP", "lookupHost", "lookupAddr", "reverseLookup", "lookupCNAME", "lookupPort", "lookupMX", "lookupNS", "lookupTXT", "lookupSRV", "parseIP", "isIP", "joinHostPort", "splitHostPort", "parseCIDR", "containsCIDR", "isPrivate", "isLoopback", "isGlobalUnicast", "isMulticast", "isUnspecified", "isLinkLocalUnicast", "isLinkLocalMulticast", "to4", "to16", "resolveTCPAddr", "resolveUDPAddr", "resolveIPAddr", "tcpAddr", "udpAddr", "ipAddr", "hostname", "interfaces", "localIPs"},
	"std.ws":       {"createServer", "connect", "closeClient"},
	"std.http":     {"buildQuery", "buildURL", "close", "createServer", "decode", "encode", "error", "listen", "parseCookies", "parseQuery", "parseURL", "request", "serializeCookie", "status", "statusText"},
	"std.db":       {"collection", "connect", "create", "drop", "list", "open", "table"},
	"std.jwt":      {"sign", "verify", "decode", "isExpired", "refresh"},
	"std.os":       {"abs", "arch", "args", "basename", "chdir", "cpus", "cwd", "dirname", "environ", "eol", "exec", "execSync", "exists", "exit", "expandEnv", "extname", "getcwd", "getenv", "getpid", "getppid", "glob", "homeDir", "hostname", "hrtime", "join", "listDir", "mkdir", "pathSep", "pid", "platform", "ppid", "remove", "rename", "sep", "setenv", "sleep", "spawn", "stat", "tempDir", "tempFile", "time", "unsetenv"},
	"std.json":     {"parse", "stringify", "pretty", "compact", "isValid", "readFile", "writeFile", "save"},
	"std.env":      {"get", "has", "set", "all", "parse", "populate", "load", "int", "bool", "config"},
	"std.testing":  {"test", "cases", "group", "beforeAll", "afterAll", "beforeEach", "afterEach", "ok", "falsey", "defined", "equal", "notEqual", "same", "contains", "type", "length", "empty", "approx", "raises", "notRaises", "fail", "skip", "todo", "note", "plan", "snapshot", "list", "run", "clear"},
	"std.runtime":  {"setGlobal", "getGlobal", "hasGlobal", "globals", "version"},
}

var stdModuleNames = []string{
	"io", "fs", "http", "crypto", "db", "env", "encoding", "testing", "ws", "utils", "json", "jwt", "math", "datetime", "os", "regex", "buffer", "ints", "image", "net", "runtime", "ffi", "internal.native", "http/router", "http/static",
}

var globalObjectMembers = map[string][]string{
	"Array":          {"isArray", "from", "of"},
	"Object":         {"keys", "values", "entries", "assign", "freeze", "seal", "create", "hasOwn"},
	"Math":           {"PI", "E", "LN2", "LN10", "LOG2E", "LOG10E", "SQRT2", "SQRT1_2", "PHI", "Infinity", "NaN", "abs", "ceil", "floor", "round", "sqrt", "cbrt", "sin", "cos", "tan", "asin", "acos", "atan", "atan2", "log", "log2", "log10", "exp", "sign", "trunc", "hypot", "max", "min", "pow", "random", "imul"},
	"JSON":           {"stringify", "parse"},
	"Promise":        {"resolve", "reject", "all"},
	"performance":    {"now"},
	"process":        {"env", "argv", "exit", "stdout", "stderr"},
	"process.stdout": {"write"},
	"process.stderr": {"write"},
}

var builtinCompletionNames = []string{
	"parseInt", "parseFloat", "isNaN", "isFinite", "String", "str", "Number", "num", "Boolean", "Array", "Object", "Math", "JSON", "Promise", "Error", "TypeError", "RangeError", "Map", "Set", "setTimeout", "setInterval", "clearTimeout", "clearInterval", "performance", "process", "encodeURIComponent", "decodeURIComponent", "encodeURI", "decodeURI", "btoa", "atob", "NaN", "Infinity", "lunex", "parallel",
}

var commonKeywords = []string{
	"fn", "val", "var", "if", "else", "elif", "unless", "while", "for", "each", "in", "of", "loop", "repeat", "match", "case", "default", "when", "try", "catch", "finally", "throw", "raise", "break", "continue", "defer", "guard", "struct", "enum", "namespace", "component", "new", "this", "super", "import", "export", "from", "as", "with", "using", "assert", "watch", "select", "have", "ifhave", "ifset", "immutable", "delete",
}

func completionKind(kind string) int {
	switch kind {
	case "Function":
		return completionFunction
	case "Parameter":
		return completionVariable
	case "Class", "Struct":
		return completionClass
	case "Enum":
		return completionEnum
	case "Namespace", "Module", "Import":
		return completionModule
	case "Property", "Field":
		return completionProperty
	default:
		return completionVariable
	}
}

func (s *Server) completion(params map[string]json.RawMessage) any {
	var request struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position position `json:"position"`
		Context  struct {
			TriggerKind      int    `json:"triggerKind"`
			TriggerCharacter string `json:"triggerCharacter"`
		} `json:"context"`
	}
	decodeField(params, "textDocument", &request.TextDocument)
	decodeField(params, "position", &request.Position)
	decodeField(params, "context", &request.Context)
	doc := s.ensureDoc(request.TextDocument.URI)
	if doc == nil {
		return map[string]any{"isIncomplete": false, "items": []completionItem{}}
	}
	prefix, prefixStart, offset := wordPrefix(doc.Text, request.Position)
	prefixOffset := offsetAt(doc.Text, prefixStart)
	beforePrefix := strings.TrimRight(doc.Text[:prefixOffset], " \t\r\n")
	items := make([]completionItem, 0, 64)
	if strings.HasSuffix(beforePrefix, ".") {
		items = s.memberCompletions(doc, beforePrefix, prefix, prefixStart)
	} else if inImportPath(doc.Text, offset) {
		items = s.importCompletions(doc, prefix, prefixStart)
	} else {
		items = s.generalCompletions(doc, request.Position, prefix, prefixStart)
	}
	return map[string]any{"isIncomplete": false, "items": items}
}

func (s *Server) generalCompletions(doc *Document, pos position, prefix string, start position) []completionItem {
	items := make([]completionItem, 0, 64)
	seen := map[string]bool{}
	cursorIndex := offsetTokenIndex(doc, pos)
	var cursorPath []int
	if cursorIndex >= 0 && cursorIndex < len(doc.TokenScopes) {
		cursorPath = doc.TokenScopes[cursorIndex]
	} else if len(doc.TokenScopes) > 0 {
		cursorPath = doc.TokenScopes[len(doc.TokenScopes)-1]
	}
	for _, symbol := range doc.Symbols {
		if symbol.Name == "" || !strings.HasPrefix(strings.ToLower(symbol.Name), strings.ToLower(prefix)) || !prefixPath(symbol.ScopePath, cursorPath) {
			continue
		}
		if symbol.TokenIndex >= 0 && symbol.TokenIndex < len(doc.Tokens) && doc.Tokens[symbol.TokenIndex].Line > pos.Line+1 {
			if symbol.Kind != "Function" && symbol.Kind != "Class" && symbol.Kind != "Enum" && symbol.Kind != "Namespace" {
				continue
			}
		}
		if seen[symbol.Name] {
			continue
		}
		seen[symbol.Name] = true
		kind := completionKind(symbol.Kind)
		items = append(items, completionItem{Label: symbol.Name, Kind: kind, Detail: symbol.Detail, SortText: "1_" + strings.ToLower(symbol.Name), FilterText: symbol.Name, TextEdit: replacementEdit(start, pos, prefix, symbol.Name)})
	}
	for _, name := range builtinCompletionNames {
		if seen[name] || !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			continue
		}
		seen[name] = true
		items = append(items, completionItem{Label: name, Kind: completionFunction, Detail: "Lunex built-in", SortText: "2_" + name, TextEdit: replacementEdit(start, pos, prefix, name)})
	}
	for _, keyword := range commonKeywords {
		if seen[keyword] || !strings.HasPrefix(strings.ToLower(keyword), strings.ToLower(prefix)) {
			continue
		}
		seen[keyword] = true
		items = append(items, completionItem{Label: keyword, Kind: completionKeyword, Detail: "Lunex keyword", SortText: "3_" + keyword, TextEdit: replacementEdit(start, pos, prefix, keyword)})
	}
	if strings.HasPrefix("true", strings.ToLower(prefix)) && prefix != "" {
		items = append(items, completionItem{Label: "true", Kind: completionKeyword, Detail: "Boolean literal", SortText: "3_true", TextEdit: replacementEdit(start, pos, prefix, "true")})
	}
	if strings.HasPrefix("false", strings.ToLower(prefix)) && prefix != "" {
		items = append(items, completionItem{Label: "false", Kind: completionKeyword, Detail: "Boolean literal", SortText: "3_false", TextEdit: replacementEdit(start, pos, prefix, "false")})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].SortText == items[j].SortText {
			return items[i].Label < items[j].Label
		}
		return items[i].SortText < items[j].SortText
	})
	return items
}

func (s *Server) memberCompletions(doc *Document, before, prefix string, start position) []completionItem {
	receiver := receiverExpression(before)
	if receiver == "" {
		return []completionItem{}
	}
	members := s.membersForReceiver(doc, receiver)
	items := make([]completionItem, 0, len(members))
	for _, member := range members {
		if !strings.HasPrefix(strings.ToLower(member.Name), strings.ToLower(prefix)) {
			continue
		}
		items = append(items, completionItem{Label: member.Name, Kind: member.Kind, Detail: member.Detail, Documentation: member.Documentation, SortText: "1_" + strings.ToLower(member.Name), TextEdit: replacementEdit(start, position{Line: start.Line, Character: start.Character + runeLengthUTF16(prefix)}, prefix, member.Name)})
	}
	return items
}

type memberInfo struct {
	Name          string
	Kind          int
	Detail        string
	Documentation string
}

func (s *Server) membersForReceiver(doc *Document, receiver string) []memberInfo {
	if members, ok := globalObjectMembers[receiver]; ok {
		return namesAsMembers(members, "built-in member")
	}
	parts := strings.Split(receiver, ".")
	if len(parts) == 1 {
		for i := len(doc.Symbols) - 1; i >= 0; i-- {
			symbol := doc.Symbols[i]
			if symbol.Name != parts[0] {
				continue
			}
			if strings.HasPrefix(symbol.Type, "module:") {
				module := strings.TrimPrefix(symbol.Type, "module:")
				if !strings.HasPrefix(module, "std.") {
					module = "std." + module
				}
				return namesAsMembers(stdModuleMembers[module], "module member")
			}
			if symbol.Type == "string" {
				return namesAsMembers([]string{"length", "toUpperCase", "upper", "toLowerCase", "lower", "trim", "trimStart", "trimLeft", "trimEnd", "trimRight", "split", "includes", "startsWith", "endsWith", "indexOf", "lastIndexOf", "slice", "substring", "replace", "replaceAll", "repeat", "charAt", "charCodeAt", "toString"}, "string member")
			}
			if symbol.Type == "array" {
				return namesAsMembers([]string{"length", "push", "pop", "shift", "unshift", "slice", "splice", "indexOf", "includes", "join", "reverse", "map", "filter", "reduce", "forEach", "find", "findIndex", "some", "every", "flat", "flatMap", "sort", "concat", "toString", "fill", "keys", "values", "entries"}, "array member")
			}
			if symbol.Node != nil && symbol.Node.Init != nil && (symbol.Node.Init.Type == ast.ObjectLit || symbol.Node.Init.Type == ast.StructLit) {
				return objectMembers(symbol.Node.Init)
			}
			if strings.HasPrefix(symbol.Type, "instance:") {
				return s.namedTypeMembers(doc, strings.TrimPrefix(symbol.Type, "instance:"))
			}
			if members, ok := globalObjectMembers[symbol.Name]; ok {
				return namesAsMembers(members, "built-in member")
			}
			if symbol.Node != nil && symbol.Node.Init != nil && symbol.Node.Init.Type == ast.AtImportExpr {
				module := symbol.Node.Init.Source
				if !strings.HasPrefix(module, "std.") {
					module = "std." + module
				}
				return namesAsMembers(stdModuleMembers[module], "module member")
			}
			return nil
		}
		return nil
	}
	if strings.HasPrefix(receiver, "std.") {
		return namesAsMembers(stdModuleMembers[receiver], "module member")
	}
	for _, symbol := range doc.Symbols {
		if symbol.Name == parts[0] && symbol.Node != nil {
			n := symbol.Node.Init
			for _, part := range parts[1:] {
				if n == nil || n.Type != ast.ObjectLit {
					return nil
				}
				n = objectPropertyValue(n, part)
			}
			if n != nil && n.Type == ast.ObjectLit {
				return objectMembers(n)
			}
		}
	}
	return nil
}

func (s *Server) namedTypeMembers(doc *Document, typeName string) []memberInfo {
	for _, symbol := range doc.Symbols {
		if symbol.Name == typeName && symbol.Node != nil && symbol.Node.Init != nil && symbol.Node.Init.Type == ast.StructLit {
			return objectMembers(symbol.Node.Init)
		}
	}
	for _, symbol := range doc.Symbols {
		if symbol.Name == typeName && symbol.Node != nil && symbol.Node.Type == ast.ClassDecl {
			return classMemberCompletions(symbol.Node)
		}
	}
	return nil
}

func classMemberCompletions(node *ast.Node) []memberInfo {
	if node == nil {
		return nil
	}
	out := make([]memberInfo, 0, len(node.Methods))
	for _, member := range node.Methods {
		if member == nil || member.Name == "" || member.IsPrivate {
			continue
		}
		kind := completionMethod
		detail := "method"
		if member.Kind == "field" {
			kind = completionProperty
			detail = "field"
		}
		out = append(out, memberInfo{Name: member.Name, Kind: kind, Detail: detail})
	}
	return out
}

func objectMembers(node *ast.Node) []memberInfo {
	if node == nil {
		return nil
	}
	var out []memberInfo
	if node.Type == ast.StructLit {
		return classMemberCompletions(node)
	}
	for _, prop := range node.Properties {
		if prop == nil {
			continue
		}
		name := fmt.Sprint(prop.Key)
		if name == "" || name == "<nil>" {
			continue
		}
		kind := completionProperty
		detail := "object property"
		if prop.Kind == "method" || prop.Body != nil {
			kind = completionMethod
			detail = "object method"
		}
		out = append(out, memberInfo{Name: name, Kind: kind, Detail: detail})
	}
	return out
}

func objectPropertyValue(node *ast.Node, name string) *ast.Node {
	for _, prop := range node.Properties {
		if prop != nil && fmt.Sprint(prop.Key) == name {
			return prop.Value
		}
	}
	return nil
}

func namesAsMembers(names []string, detail string) []memberInfo {
	out := make([]memberInfo, 0, len(names))
	for _, name := range names {
		kind := completionFunction
		if strings.ToUpper(name) == name || name == "length" {
			kind = completionProperty
		}
		out = append(out, memberInfo{Name: name, Kind: kind, Detail: detail})
	}
	return out
}

func (s *Server) importCompletions(doc *Document, prefix string, start position) []completionItem {
	items := make([]completionItem, 0, len(stdModuleNames))
	seen := map[string]bool{}
	for _, label := range stdModuleNames {
		if strings.HasPrefix(strings.ToLower(label), strings.ToLower(prefix)) && !seen[label] {
			seen[label] = true
			items = append(items, completionItem{Label: label, Kind: completionModule, Detail: "Lunex standard library module", SortText: "1_" + label, TextEdit: replacementEdit(start, position{Line: start.Line, Character: start.Character + runeLengthUTF16(prefix)}, prefix, label)})
		}
	}
	base := filepath.Dir(doc.Path)
	entries, err := os.ReadDir(base)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".lx" {
				continue
			}
			label := entry.Name()
			if strings.HasPrefix(strings.ToLower(label), strings.ToLower(prefix)) {
				items = append(items, completionItem{Label: label, Kind: completionFile, Detail: "Lunex source file", SortText: "2_" + label, TextEdit: replacementEdit(start, position{Line: start.Line, Character: start.Character + runeLengthUTF16(prefix)}, prefix, label)})
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].SortText < items[j].SortText })
	return items
}

const completionFile = 17

func wordPrefix(text string, pos position) (string, position, int) {
	offset := offsetAt(text, pos)
	if offset > len(text) {
		offset = len(text)
	}
	start := offset
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:start])
		if !isIdentifierPart(r) {
			break
		}
		start -= size
	}
	prefix := text[start:offset]
	return prefix, positionAt(text, start), offset
}

func replacementEdit(start, end position, old, replacement string) *textEdit {
	return &textEdit{Range: lspRange{Start: start, End: end}, NewText: replacement}
}

func offsetTokenIndex(doc *Document, pos position) int {
	for i := range doc.Tokens {
		r := tokenRange(doc, i)
		if pos.Line < r.Start.Line || pos.Line == r.Start.Line && pos.Character <= r.End.Character {
			return i
		}
	}
	if len(doc.Tokens) == 0 {
		return -1
	}
	return len(doc.Tokens) - 1
}

func receiverExpression(before string) string {
	before = strings.TrimSpace(before)
	if strings.HasSuffix(before, ".") {
		before = strings.TrimSpace(before[:len(before)-1])
	}
	if before == "" {
		return ""
	}
	runes := []rune(before)
	i := len(runes) - 1
	for i >= 0 && (isIdentifierPart(runes[i]) || runes[i] == '.') {
		i--
	}
	candidate := string(runes[i+1:])
	candidate = strings.Trim(candidate, ".")
	if candidate == "" {
		return ""
	}
	for _, part := range strings.Split(candidate, ".") {
		if part == "" {
			return ""
		}
		for j, r := range part {
			if j == 0 && !isIdentifierStart(r) || j > 0 && !isIdentifierPart(r) {
				return ""
			}
		}
	}
	return candidate
}

func inImportPath(text string, offset int) bool {
	if offset < 0 || offset > len(text) {
		return false
	}
	prefix := text[:offset]
	lineStart := strings.LastIndex(prefix, "\n") + 1
	line := prefix[lineStart:]
	return strings.Contains(line, "@import(") || strings.Contains(line, "@fimport(") || strings.Contains(line, " from ")
}

func (s *Server) resolveCompletion(params map[string]json.RawMessage) any {
	var item completionItem
	if raw, ok := params["label"]; ok {
		_ = json.Unmarshal(raw, &item.Label)
	}
	_ = json.Unmarshal(marshalRaw(params), &item)
	if item.Documentation == "" {
		item.Documentation = item.Detail
	}
	return item
}

func marshalRaw(params map[string]json.RawMessage) []byte {
	b, _ := json.Marshal(params)
	return b
}

func (s *Server) signatureHelp(params map[string]json.RawMessage) any {
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
	offset := offsetAt(doc.Text, req.Position)
	before := doc.Text[:offset]
	open := strings.LastIndex(before, "(")
	if open < 0 {
		return nil
	}
	prefix := strings.TrimSpace(before[:open])
	end := len(prefix)
	start := end
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(prefix[:start])
		if !isIdentifierPart(r) && r != '.' {
			break
		}
		start -= size
	}
	name := strings.Trim(prefix[start:end], ".")
	var sym *Symbol
	for _, candidate := range s.allSymbols() {
		if candidate.Name == name && (candidate.Kind == "Function" || candidate.Kind == "Method" || candidate.Kind == "Component") {
			sym = candidate
			break
		}
	}
	if sym == nil || sym.Node == nil {
		return nil
	}
	paramsNames := make([]string, 0, len(sym.Node.Params))
	infos := make([]parameterInformation, 0, len(sym.Node.Params))
	for _, param := range sym.Node.Params {
		if param == nil {
			continue
		}
		label := param.Name
		if param.Rest {
			label = "..." + label
		}
		if param.Optional || param.DefaultVal != nil {
			label += "?"
		}
		paramsNames = append(paramsNames, label)
		infos = append(infos, parameterInformation{Label: label})
	}
	active := strings.Count(before[open+1:], ",")
	if active >= len(infos) && len(infos) > 0 {
		active = len(infos) - 1
	}
	label := fmt.Sprintf("%s(%s)", name, strings.Join(paramsNames, ", "))
	return signatureHelp{Signatures: []signatureInformation{{Label: label, Parameters: infos, ActiveParameter: active}}, ActiveSignature: 0, ActiveParameter: active}
}
