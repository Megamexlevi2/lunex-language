package errfmt

import (
	"fmt"
	"lunex/internal/adaptor"
	"os"
	"strings"
)

var useColor = os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"

func terminalWidth() int {
	return adaptor.TerminalWidth()
}

func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}

	var lines []string
	var cur strings.Builder
	curLen := 0
	for _, word := range words {
		wordLen := len(word)
		if curLen == 0 {
			cur.WriteString(word)
			curLen = wordLen
			continue
		}
		if curLen+1+wordLen > width {
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(word)
			curLen = wordLen
			continue
		}
		cur.WriteByte(' ')
		cur.WriteString(word)
		curLen += 1 + wordLen
	}
	if curLen > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}

func wrapIndented(prefix, text string) string {
	width := terminalWidth() - visibleLen(prefix)
	if width < 20 {
		return prefix + text
	}
	wrapped := wrapText(text, width)
	if len(wrapped) <= 1 {
		return prefix + text
	}
	pad := strings.Repeat(" ", visibleLen(prefix))
	out := prefix + wrapped[0]
	for _, line := range wrapped[1:] {
		out += "\n" + pad + line
	}
	return out
}

func visibleLen(s string) int {
	n := 0
	inEscape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inEscape {
			if c == 'm' {
				inEscape = false
			}
			continue
		}
		if c == '\x1b' {
			inEscape = true
			continue
		}
		n++
	}
	return n
}

func esc(code, t string) string {
	if !useColor {
		return t
	}
	return "\x1b[" + code + "m" + t + "\x1b[0m"
}

func bold(t string) string          { return esc("1", t) }
func dim(t string) string           { return esc("2", t) }
func red(t string) string           { return esc("31", t) }
func green(t string) string         { return esc("32", t) }
func yellow(t string) string        { return esc("33", t) }
func blue(t string) string          { return esc("34", t) }
func cyan(t string) string          { return esc("36", t) }
func white(t string) string         { return esc("37", t) }
func gray(t string) string          { return esc("90", t) }
func brightRed(t string) string     { return esc("91", t) }
func brightGreen(t string) string   { return esc("92", t) }
func brightYellow(t string) string  { return esc("93", t) }
func brightCyan(t string) string    { return esc("96", t) }
func brightMagenta(t string) string { return esc("95", t) }

func errColor(t string) string  { return bold(brightRed(t)) }
func warnColor(t string) string { return bold(brightYellow(t)) }
func hintColor(t string) string { return bold(brightGreen(t)) }

type ErrorKind string
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
	SeverityHint    Severity = "hint"
)

const (
	KindSyntax      ErrorKind = "SyntaxError"
	KindType        ErrorKind = "TypeError"
	KindReference   ErrorKind = "ReferenceError"
	KindRuntime     ErrorKind = "RuntimeError"
	KindImport      ErrorKind = "ImportError"
	KindAssertion   ErrorKind = "AssertionError"
	KindRange       ErrorKind = "RangeError"
	KindIO          ErrorKind = "IOError"
	KindLex         ErrorKind = "LexError"
	KindParse       ErrorKind = "ParseError"
	KindPermission  ErrorKind = "PermissionError"
	KindOverflow    ErrorKind = "OverflowError"
	KindArithmetic  ErrorKind = "ArithmeticError"
	KindAttribute   ErrorKind = "AttributeError"
	KindMemory      ErrorKind = "MemoryError"
	KindTimeout     ErrorKind = "TimeoutError"
	KindNetwork     ErrorKind = "NetworkError"
	KindEncoding    ErrorKind = "EncodingError"
	KindRecursion   ErrorKind = "RecursionError"
	KindConcurrency ErrorKind = "ConcurrencyError"
	KindSemantic    ErrorKind = "SemanticError"
	KindDeprecated  ErrorKind = "DeprecationWarning"
	KindStyle       ErrorKind = "StyleWarning"
	KindSuspect     ErrorKind = "SuspectError"
)

type kindMeta struct {
	label    string
	severity Severity
	color    func(string) string
	icon     string
}

var kindRegistry = map[ErrorKind]kindMeta{
	KindLex:         {"error[lex]", SeverityError, errColor, "✗"},
	KindParse:       {"error[parse]", SeverityError, errColor, "✗"},
	KindSyntax:      {"error[syntax]", SeverityError, errColor, "✗"},
	KindType:        {"error[type]", SeverityError, errColor, "✗"},
	KindRuntime:     {"error[runtime]", SeverityError, errColor, "✗"},
	KindReference:   {"error[scope]", SeverityError, errColor, "✗"},
	KindImport:      {"error[module]", SeverityError, errColor, "✗"},
	KindIO:          {"error[io]", SeverityError, errColor, "✗"},
	KindAssertion:   {"error[assertion]", SeverityError, errColor, "✗"},
	KindRange:       {"error[range]", SeverityError, errColor, "✗"},
	KindPermission:  {"error[permission]", SeverityError, errColor, "✗"},
	KindOverflow:    {"error[overflow]", SeverityError, errColor, "✗"},
	KindArithmetic:  {"error[arithmetic]", SeverityError, errColor, "✗"},
	KindAttribute:   {"error[attribute]", SeverityError, errColor, "✗"},
	KindMemory:      {"error[memory]", SeverityError, errColor, "✗"},
	KindTimeout:     {"error[timeout]", SeverityError, errColor, "✗"},
	KindNetwork:     {"error[network]", SeverityError, errColor, "✗"},
	KindEncoding:    {"error[encoding]", SeverityError, errColor, "✗"},
	KindRecursion:   {"error[recursion]", SeverityError, errColor, "✗"},
	KindConcurrency: {"error[concurrency]", SeverityError, errColor, "✗"},
	KindSemantic:    {"error[semantic]", SeverityError, errColor, "✗"},
	KindDeprecated:  {"warning[deprecated]", SeverityWarning, warnColor, "⚠"},
	KindStyle:       {"warning[style]", SeverityWarning, warnColor, "⚠"},
	KindSuspect:     {"error[suspect]", SeverityError, warnColor, "⚠"},
}

type codeDisplay struct {
	Code      string
	Short     string
	Underline string
}

var codeRegistry = map[string]codeDisplay{
	"E0001":  {"E0001", "undefined variable", "not found in scope"},
	"E0002":  {"E0002", "undefined method", "method not found on this type"},
	"E0003":  {"E0003", "value is not callable", "not a function — cannot call"},
	"E0004":  {"E0004", "null or undefined access", "value is null — property does not exist"},
	"E0005":  {"E0005", "cannot reassign immutable binding", "declared as `val` — immutable"},
	"E0006":  {"E0006", "undefined field", "field not found on this type"},
	"E0010":  {"E0010", "module not found", "unresolved module path"},
	"E0010F": {"E0010F", "local file not found", "unresolved local path"},
	"E0011":  {"E0011", "module has a syntax error", "syntax error in module"},
	"E0012":  {"E0012", "circular import detected", "circular dependency — cycle in import graph"},
	"E0013":  {"E0013", "module failed to load", "error while loading module"},
	"E0014":  {"E0014", "module is internal", "internal module — cannot be imported by user code"},
	"E0015":  {"E0015", "binary module load failed", "cannot decode .nax or .nax file"},
	"E0020":  {"E0020", "type mismatch", "incompatible types"},
	"E0021":  {"E0021", "wrong number of arguments", "arity mismatch"},
	"E0022":  {"E0022", "invalid argument type", "expected a different type here"},
	"E0023":  {"E0023", "return type mismatch", "returned type does not match the function signature"},
	"E0024":  {"E0024", "operator not defined for this type", "no operator implementation for this type"},
	"E0025":  {"E0025", "cannot coerce value to target type", "implicit coercion failed"},
	"E0030":  {"E0030", "division by zero", "divisor is zero"},
	"E0031":  {"E0031", "integer overflow", "value exceeds numeric bounds"},
	"E0032":  {"E0032", "result is NaN or Inf", "not-a-number or infinite result"},
	"E0040":  {"E0040", "index out of bounds", "index exceeds array or string length"},
	"E0041":  {"E0041", "invalid slice range", "start > end or negative index"},
	"E0050":  {"E0050", "unexpected token", "token not expected here"},
	"E0051":  {"E0051", "unexpected end of file", "file ended before the expression was complete"},
	"E0052":  {"E0052", "unclosed block or delimiter", "add the matching closing `}`"},
	"E0053":  {"E0053", "invalid escape sequence", "unrecognized escape sequence in string"},
	"E0054":  {"E0054", "invalid number literal", "malformed numeric literal"},
	"E0055":  {"E0055", "duplicate key in object literal", "key already defined"},
	"E0060":  {"E0060", "stack overflow", "maximum call depth exceeded"},
	"E0061":  {"E0061", "assertion failed", "assertion evaluated to false"},
	"E0062":  {"E0062", "explicit panic", "user-triggered panic"},
	"E0063":  {"E0063", "I/O operation failed", "I/O error"},
	"E0064":  {"E0064", "permission denied", "insufficient permissions"},
	"E0065":  {"E0065", "operation timed out", "deadline exceeded"},
	"E0066":  {"E0066", "network error", "network unreachable or connection refused"},
	"E0067":  {"E0067", "encoding error", "byte sequence is not valid UTF-8"},
	"E0068":  {"E0068", "memory allocation failed", "out of memory"},
	"E0069":  {"E0069", "concurrent write detected", "data race on shared value"},
	"W0001":  {"W0001", "use of deprecated symbol", "deprecated — will be removed in a future release"},
	"W0002":  {"W0002", "shadowed variable", "this declaration shadows an outer binding"},
	"W0003":  {"W0003", "unreachable code", "code after this point is never executed"},
	"W0004":  {"W0004", "unused variable", "declared but never used"},
	"W0005":  {"W0005", "implicit type coercion", "implicit coercion may lose precision"},
	"W0006":  {"W0006", "multiple statements on one line", "split each statement onto its own line"},
	"W0007":  {"W0007", "missing spacing", "add spaces around operators and after commas"},
	"W0008":  {"W0008", "missing space before `{`", "add a space before the opening brace"},
	"W0009":  {"W0009", "semicolon used as separator", "use newlines instead of semicolons"},
	"W0010":  {"W0010", "minified or single-line program", "expand the program across multiple lines"},
	"E0070":  {"E0070", "missing entry point", "`fn main()` is required"},
	"E0071":  {"E0071", "top-level statement not allowed", "only declarations are allowed at the top level"},
	"E0072":  {"E0072", "explicit main() call not allowed", "`main` is called automatically by the runtime"},
	"E0073":  {"E0073", "reserved keyword used as identifier", "this name is a Lunex reserved keyword"},
	"E0074":  {"E0074", "redeclaration of built-in", "shadows a built-in function or constant"},
	"E0075":  {"E0075", "invalid identifier name", "identifiers must start with a letter or underscore"},
	"E0076":  {"E0076", "reserved keyword as parameter name", "keyword cannot be used as a parameter"},
	"E0077":  {"E0077", "reserved keyword as field name", "keyword cannot be used as a field name"},
	"E0078":  {"E0078", "operator not defined for these types", "no implementation for this type combination"},
	"E0079":  {"E0079", "expression produced undefined", "sub-expression evaluated to undefined"},
	"E0080":  {"E0080", "legacy output function", "use std.io and call io.log(...)"},
	"E0432":  {"E0432", "unresolved export", "export a declared or imported name"},
	"E0110":  {"E0110", "required environment variable is not set", "env.require could not find the requested variable"},
	"E0111":  {"E0111", "environment file operation failed", "dotenv file could not be loaded or applied"},
	"E0112":  {"E0112", "NAX pack pipeline rejected the artifact", "source validation must succeed before archive emission"},
	"E0113":  {"E0113", "malformed JSON text", "std.json.parse could not make sense of the input at this position"},
	"E0114":  {"E0114", "circular structure in JSON output", "std.json.stringify cannot serialize a self-referencing value"},
	"E0115":  {"E0115", "JSON file operation failed", "std.json could not read or write the requested file"},
	"E0116":  {"E0116", "JSON value nested too deeply", "std.json refused to parse or serialize past the depth limit"},
	"E0120":  {"E0120", "native FFI is disabled", "enable FFI with the CLI-only `ffi = on` prefix"},
	"E0121":  {"E0121", "invalid FFI library path", "provide a non-empty native library path"},
	"E0122":  {"E0122", "native library load failed", "check the library path, architecture, dependencies, and loader rules"},
	"E0123":  {"E0123", "native symbol not found", "check the exported symbol name and ABI"},
	"E0124":  {"E0124", "native resource is closed", "keep the library or bound function open while it is in use"},
	"E0125":  {"E0125", "invalid FFI operation", "check the FFI function arguments and call shape"},
	"E0126":  {"E0126", "invalid FFI signature", "use `returnType(argType, ...)` or a valid descriptor object"},
	"E0127":  {"E0127", "unsupported FFI type", "use one of the supported scalar, pointer, array, or struct types"},
	"E0128":  {"E0128", "native call preparation failed", "verify the ABI declaration before calling the symbol"},
	"E0129":  {"E0129", "native callback creation failed", "use callback-compatible argument and return types"},
	"E0130":  {"E0130", "native memory allocation failed", "check native allocation size and available memory"},
	"E0131":  {"E0131", "invalid FFI value", "provide a value compatible with the declared native type"},
	"E0132":  {"E0132", "native memory bounds error", "keep reads and writes inside the known allocation length"},
	"E0133":  {"E0133", "invalid FFI option syntax", "use `ffi = on` or `ffi = off` before the Lunex command"},
	"S0001":  {"S0001", "for-of over non-iterable", "value is not iterable"},
	"S0002":  {"S0002", "match produced no result", "no arm matched — add a default case"},
	"S0003":  {"S0003", "arithmetic produced NaN", "operand is not a valid number"},
	"S0004":  {"S0004", "array index out of bounds", "index is outside the valid range"},
	"S0005":  {"S0005", "spread of non-iterable", "value cannot be spread"},
	"S0006":  {"S0006", "spread of null or undefined", "spread target is null or undefined"},
	"S0007":  {"S0007", "call on undefined return", "function returned undefined"},
}

const (
	ErrDivisionByZero           = "E0030"
	ErrTypeMismatch             = "E0020"
	ErrStackOverflow            = "E0060"
	ErrTimeout                  = "E0065"
	ErrReservedKeyword          = "E0073"
	ErrKeywordAsArg             = "E0076"
	ErrKeywordAsField           = "E0077"
	ErrPackPipeline             = "E0112"
	ErrSuspectForOfNonIterable  = "S0001"
	ErrSuspectMatchNoArm        = "S0002"
	ErrSuspectNaNResult         = "S0003"
	ErrSuspectIndexOutOfBounds  = "S0004"
	ErrSuspectSpreadNonIterable = "S0005"
	ErrSuspectNullSpread        = "S0006"
	ErrSuspectCallUndefined     = "S0007"

	ErrUnexpectedTokenGeneric = "E1000"
	ErrUnexpectedComma        = "E1001"
	ErrUnexpectedCloseParen   = "E1002"
	ErrUnexpectedCloseBrace   = "E1003"
	ErrUnexpectedCloseBracket = "E1004"
	ErrUnexpectedAssign       = "E1005"
	ErrUnexpectedSemicolon    = "E1006"
	ErrExpectedToken          = "E1010"
)

func LookupCode(code string) (codeDisplay, bool) {
	ec, ok := codeRegistry[code]
	return ec, ok
}

func KindForCode(code string) ErrorKind {
	switch {
	case strings.HasPrefix(code, "E005"), strings.HasPrefix(code, "E10"):
		return KindParse
	case strings.HasPrefix(code, "E040"), strings.HasPrefix(code, "E043"):
		return KindImport
	case strings.HasPrefix(code, "E041"), strings.HasPrefix(code, "E059"), strings.HasPrefix(code, "E002"):
		return KindType
	case code == "E0070", code == "E0071", code == "E0072", code == "E0073", code == "E0074", code == "E0075", code == "E0076", code == "E0077", code == "E0080":
		return KindSyntax
	case code == "E0060":
		return KindRecursion
	case code == "E0133":
		return KindSyntax
	case code == "E0120":
		return KindPermission
	case code == "E0121" || code == "E0122" || code == "E0123" || code == "E0124":
		return KindImport
	case code == "E0125" || code == "E0126" || code == "E0127" || code == "E0131":
		return KindType
	case code == "E0128" || code == "E0129" || code == "E0130" || code == "E0132":
		return KindRuntime
	case strings.HasPrefix(code, "S"):
		return KindSuspect
	default:
		return KindReference
	}
}

func CodeSuggestion(code string) string {
	sugs := buildSuggestions(code, "", "", nil)
	if len(sugs) > 0 {
		return sugs[0]
	}
	return ""
}

func CodeTitle(code string) string {
	if ec, ok := codeRegistry[code]; ok {
		return ec.Short
	}
	return ""
}

type StackFrame struct {
	FnName string
	File   string
	Line   int
	Col    int
}

type SecondaryLabel struct {
	Line    int
	Col     int
	Message string
}

type LunexError struct {
	Message         string
	File            string
	Line            int
	Col             int
	Kind            ErrorKind
	Code            string
	Lines           []string
	SecondaryLabels []SecondaryLabel
	Suggestion      string
	Notes           []string
	Similar         []string
	ExBad           string
	ExGood          string
	Stack           []StackFrame
	Phase           string
}

func (e *LunexError) Error() string { return e.Message }

func (e *LunexError) WithNote(note string) *LunexError {
	e.Notes = append(e.Notes, note)
	return e
}

func (e *LunexError) WithStack(frames []StackFrame) *LunexError {
	e.Stack = frames
	return e
}

func (e *LunexError) WithSecondary(line, col int, msg string) *LunexError {
	e.SecondaryLabels = append(e.SecondaryLabels, SecondaryLabel{line, col, msg})
	return e
}

func (e *LunexError) WithExample(bad, good string) *LunexError {
	e.ExBad = bad
	e.ExGood = good
	return e
}

func levenshtein(a, b string) int {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
		dp[i][0] = i
	}
	for j := 0; j <= n; j++ {
		dp[0][j] = j
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1]
			} else {
				d := dp[i-1][j]
				if dp[i][j-1] < d {
					d = dp[i][j-1]
				}
				if dp[i-1][j-1] < d {
					d = dp[i-1][j-1]
				}
				dp[i][j] = 1 + d
			}
		}
	}
	return dp[m][n]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func FindSimilar(name string, names []string) []string {
	nl := strings.ToLower(name)
	type scored struct {
		n    string
		dist int
	}
	var results []scored
	for _, n := range names {
		if n == name || len(n) <= 1 || strings.HasPrefix(n, "__") {
			continue
		}
		nl2 := strings.ToLower(n)
		dist := levenshtein(nl, nl2)
		maxLen := maxInt(len(nl), len(nl2))
		threshold := maxLen / 2
		if threshold < 3 {
			threshold = 3
		}
		prefixLen := minInt(4, minInt(len(nl), len(nl2)))
		shareStart := len(nl) >= 3 && len(nl2) >= 3 &&
			(strings.HasPrefix(nl, nl2[:prefixLen]) ||
				strings.HasPrefix(nl2, nl[:prefixLen]))
		if dist <= threshold || shareStart {
			results = append(results, scored{n, dist})
		}
	}
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].dist < results[j-1].dist; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
	out := make([]string, 0, 3)
	for i, r := range results {
		if i >= 3 {
			break
		}
		out = append(out, r.n)
	}
	return out
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_'
}

func tokenEndCol(src string, col int) int {
	end := col
	for end < len(src) && isIdentChar(src[end]) {
		end++
	}
	if end-col < 1 {
		end = col + 1
	}
	return end
}

func buildSourceView(lines []string, line, col int, primaryLabel string, secondary []SecondaryLabel, severity Severity) string {
	if len(lines) == 0 || line < 1 || line > len(lines) {
		return ""
	}

	caretColor := red
	labelColor := yellow
	switch severity {
	case SeverityWarning:
		caretColor = yellow
		labelColor = brightYellow
	case SeverityInfo:
		caretColor = blue
		labelColor = cyan
	case SeverityHint:
		caretColor = green
		labelColor = brightGreen
	}

	w := len(fmt.Sprintf("%d", line+2))
	if w < 3 {
		w = 3
	}

	numFmt := func(n int, active bool) string {
		s := fmt.Sprintf("%*d", w, n)
		if active {
			return blue(" " + s + " │")
		}
		return gray(" " + s + " │")
	}
	blank := blue(" " + strings.Repeat(" ", w) + " │")
	sep := blue(" " + strings.Repeat("─", w+2) + "┤")

	var rows []string
	for i := maxInt(1, line-2); i < line; i++ {
		rows = append(rows, dim(numFmt(i, false)+" "+lines[i-1]))
	}
	rows = append(rows, numFmt(line, true)+" "+white(lines[line-1]))

	safeCol := maxInt(0, col-1)
	srcLine := lines[line-1]
	tokEnd := tokenEndCol(srcLine, safeCol)
	spanLen := maxInt(1, tokEnd-safeCol)
	caret := caretColor("^" + strings.Repeat("~", spanLen-1))
	labelLine := caretColor("╰── ") + labelColor(primaryLabel)
	rows = append(rows, blank+" "+strings.Repeat(" ", safeCol)+caret)
	rows = append(rows, blank+" "+strings.Repeat(" ", safeCol)+labelLine)

	for i := line + 1; i <= minInt(len(lines), line+1); i++ {
		rows = append(rows, dim(numFmt(i, false)+" "+lines[i-1]))
	}

	if len(secondary) > 0 {
		rows = append(rows, sep)
		for _, sl := range secondary {
			if sl.Line < 1 || sl.Line > len(lines) {
				continue
			}
			rows = append(rows, dim(numFmt(sl.Line, false)+" "+lines[sl.Line-1]))
			sc := maxInt(0, sl.Col-1)
			rows = append(rows, blank+" "+strings.Repeat(" ", sc)+cyan("^"))
			rows = append(rows, blank+" "+strings.Repeat(" ", sc)+cyan("╰── ")+dim(sl.Message))
		}
	}

	return strings.Join(rows, "\n")
}

func resolveUnderlineLabel(code, msg, name string) string {
	if ec, ok := codeRegistry[code]; ok && ec.Underline != "" {
		ul := ec.Underline
		if name != "" && strings.Contains(ul, "this") {
			ul = "`" + name + "` — " + ul
		}
		return ul
	}
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "reserved keyword"):
		return "reserved keyword — cannot be used as an identifier"
	case strings.Contains(lower, "not defined") || strings.Contains(lower, "not declared"):
		return "not declared in this scope"
	case strings.Contains(lower, "is not a function") || strings.Contains(lower, "not callable"):
		return "not a function — cannot invoke"
	case strings.Contains(lower, "immutable") || strings.Contains(lower, "cannot reassign"):
		return "immutable binding — declared with `val`"
	case strings.Contains(lower, "null") || strings.Contains(lower, "undefined"):
		return "may be null or undefined"
	case strings.Contains(lower, "type"):
		return "type error originates here"
	case strings.Contains(lower, "expected"):
		return "unexpected token here"
	}
	return "error originates here"
}

var knownStdlib = map[string]string{
	"http":     "std.http",
	"https":    "std.http",
	"fs":       "std.fs",
	"io":       "std.io",
	"crypto":   "std.crypto",
	"db":       "std.db",
	"ws":       "std.ws",
	"jwt":      "std.jwt",
	"math":     "std.math",
	"datetime": "std.datetime",
	"time":     "std.datetime",
	"os":       "std.os",
	"regex":    "std.regex",
	"re":       "std.regex",
	"env":      "std.env",
	"ffi":      "std.ffi",
	"testing":  "std.testing",
	"utils":    "std.utils",
	"json":     "std.json",
	"path":     "std.path",
	"url":      "std.url",
	"process":  "std.process",
	"rand":     "std.rand",
	"fmt":      "std.fmt",
	"bytes":    "std.bytes",
	"strings":  "std.strings",
	"net":      "std.net",
	"test":     "std.test",
	"assert":   "std.assert",
}

func buildSuggestions(code, name, msg string, similar []string) []string {
	var out []string
	lower := strings.ToLower(msg)
	add := func(s string) { out = append(out, s) }

	switch code {
	case "E0001":
		if stdlib, ok := knownStdlib[strings.ToLower(name)]; ok {
			add("import it at the top:  val " + name + " = @import(\"" + stdlib + "\")")
			return out
		}
		add("declare it before using it:  val " + name + " = <value>")
		if len(similar) > 0 {
			add("did you mean `" + similar[0] + "`?")
		}
		return out

	case "E0002":
		if len(similar) > 0 {
			add("did you mean `" + similar[0] + "`?")
		} else {
			add("check the method name and the module's exported API")
		}
		add("check the value type with `typeof(value)` and use the documented methods for that type")
		return out

	case "E0003":
		add("`" + name + "` is not a function — only values declared with `fn` are callable")
		add("check that it was not accidentally overwritten or shadowed")
		return out

	case "E0004":
		add("guard the value before accessing it:  if " + name + " != null { ... }")
		add("use optional chaining:  " + name + "?.property")
		add("provide a fallback:  " + name + " ?? defaultValue")
		return out

	case "E0005":
		add("use `var` instead of `val` for a mutable binding:  var " + name + " = <value>")
		add("or introduce a new binding:  val " + name + "2 = newValue")
		return out

	case "E0006":
		add("check the field name against the struct or object definition")
		if len(similar) > 0 {
			add("did you mean `" + similar[0] + "`?")
		}
		return out

	case "E0010":
		add("for local files:   @fimport(\"./file.lx\")")
		add("for stdlib:        @import(\"std.io\"), @import(\"std.fs\"), @import(\"std.http\"), ...")
		add("for packages:      install with lunex-pm, then use @import(\"<pkg>\")")
		return out

	case "E0010F":
		add("path is resolved relative to the current file, then the working directory")
		add("accepted extensions: .lx (source), .nax (archive), .nax (bytecode)")
		return out

	case "E0011":
		add("the imported module has a syntax error — fix it before importing")
		add("run `lunex run <module-file>` directly to see the full error")
		return out

	case "E0012":
		add("module A imports B, and B imports A — break the cycle")
		add("move shared code into a third module that both can import")
		return out

	case "E0013":
		add("the module loaded but failed while executing — check the module's own code")
		return out

	case "E0014":
		add("this is an internal Lunex module and cannot be imported by user code")
		add("use the public standard library instead: @import(\"std.io\"), @import(\"std.fs\"), ...")
		return out

	case "E0015":
		add("the file could not be decoded as a Lunex binary module")
		add("accepted format: .nax (built with `lunex pack`)")
		add("rebuild the archive with `lunex pack <directory>` if it may be outdated")
		return out

	case "E0020":
		add("add an explicit cast:  as<TargetType>(value)")
		add("check the actual type with `typeof(value)`")
		return out

	case "E0021":
		add("check the function signature and count the required parameters")
		return out

	case "E0022":
		add("pass the correct type; use `as<T>(value)` to convert explicitly")
		return out

	case "E0023":
		add("adjust the return type in the function signature")
		add("or cast the return value:  return as<ExpectedType>(value)")
		return out

	case "E0024":
		add("convert the operands to compatible types before applying the operator")
		return out

	case "E0030":
		add("guard the divisor:  if denominator != 0 { result = a / denominator }")
		add("use `math.safeDivide(a, b, fallback)` from std.math")
		return out

	case "E0031":
		add("check the value fits the numeric range before the operation")
		return out

	case "E0032":
		add("check for division by zero or operations on infinity")
		add("use `math.isFinite(x)` to guard NaN or Inf results")
		return out

	case "E0040":
		add("check `arr.length` before indexing:  if i < arr.length { ... }")
		add("use `arr.get(i)` which returns null on out-of-range access")
		return out

	case "E0041":
		add("ensure start ≤ end and both are non-negative:  arr[start:end]")
		return out

	case "E0050":
		add("check for missing `{}`, `()`, or `:` near this line")
		add("look for an unclosed string or comment above this line")
		return out

	case "E0051":
		add("the file ended unexpectedly — check for unclosed `fn`, `if`, `for`, `{`, or `(`")
		return out

	case "E0052":
		add("find the opening `{` and add the matching closing `}`")
		return out

	case "E0053":
		add("valid escape sequences: \\n  \\t  \\r  \\\\  \\\"  \\0  \\uXXXX")
		return out

	case "E0054":
		add("numeric literals: 42  3.14  0xFF  0b1010  0o777  1_000_000")
		return out

	case "E0055":
		add("remove the duplicate key, or rename one of them")
		return out

	case "E0060":
		add("add a base case to your recursive function to stop infinite recursion")
		add("consider converting deep recursion to an iterative loop")
		return out

	case "E0061":
		add("review the condition that triggered the assertion failure")
		add("log the relevant value with `io.log(value)` after importing `std.io`")
		return out

	case "E0063":
		add("check that the file path exists and the process has read/write permissions")
		add("wrap I/O in error handling:  val result = try fs.read(path)")
		return out

	case "E0064":
		add("run with elevated permissions, or adjust file or directory ownership")
		return out

	case "E0065":
		add("increase the timeout limit, or add cancellation handling")
		return out

	case "E0066":
		add("check that the network interface is up and the remote host is reachable")
		add("handle connection errors:  val resp = try http.get(url)")
		return out

	case "E0067":
		add("ensure the byte sequence is valid UTF-8, or use bytes.readRaw() for binary data")
		return out

	case "E0068":
		add("reduce allocations, or increase the available heap limit")
		return out

	case "E0069":
		add("protect shared state with a Mutex or Channel before concurrent access")
		return out

	case "W0001":
		add("replace with the updated API — check the migration guide in the docs")
		return out

	case "W0002":
		add("rename the inner variable to avoid confusion:  var " + name + "2 = ...")
		return out

	case "W0003":
		add("remove the unreachable code, or check for an early `return` or `break` above")
		return out

	case "W0004":
		add("remove `" + name + "` if not needed, or prefix with `_` to suppress:  val _" + name + " = ...")
		return out

	case "W0005":
		add("use an explicit cast:  as<TargetType>(value)")
		return out

	case "W0006":
		add("put each statement on its own line — one statement per line is standard Lunex style")
		return out

	case "W0007":
		add("add spaces around `=` and after `,`:  fn add(a, b) { ... }  /  val x = 1")
		return out

	case "W0008":
		add("add a space before `{`:  fn main() {  not  fn main(){")
		return out

	case "W0009":
		add("replace `;` with a newline — Lunex uses line breaks as statement separators")
		return out

	case "W0010":
		add("expand the program — put each import, function, and statement on its own line")
		add("minified code produces inaccurate error locations and is hard to debug")
		return out

	case "E0070":
		add("every Lunex program must define `fn main()` as its entry point")
		add("move your top-level code inside main:\n\n  fn main() {\n    your code here\n  }")
		return out

	case "E0071":
		add("move this code inside `fn main() { ... }`")
		add("top-level scope only allows: `fn`, `val`, `var`, `class`, `@import`, `@fimport`")
		return out

	case "E0072":
		add("remove the `main()` call — Lunex invokes it automatically when the program starts")
		return out

	case "UNDEF_FUNC":
		add("define the function before calling it:  fn " + name + "(...) { ... }")
		if len(similar) > 0 {
			add("did you mean `" + similar[0] + "`?")
		}
		return out

	case "S0001":
		add("only arrays, strings, and objects are iterable with `for ... of`")
		add("check the value type with `typeof(value)` before iterating")
		return out

	case "S0002":
		add("add a default arm to handle unmatched cases:")
		add("  _ => {  }")
		return out

	case "S0003":
		add("one operand is likely undefined, null, or a non-numeric string")
		add("use explicit conversion: Number(x)  or guard: if typeof(x) == \"number\" { ... }")
		return out

	case "S0004":
		add("check the array length before indexing:  if i >= 0 && i < arr.length { arr[i] }")
		add("negative indices are not supported — use arr.length - 1 for the last element")
		return out

	case "S0005":
		add("spread (...) works on arrays and objects only")
		add("wrap in an array if needed:  [...[value]]  or convert first")
		return out

	case "S0006":
		add("guard the value before spreading:  if src != null { ...src }")
		return out

	case "S0007":
		add("make sure the function returns a value — a missing return produces undefined")
		add("check for optional chaining `?.` that may have short-circuited to undefined")
		return out

	case "E0073":
		add("`" + name + "` is a reserved keyword — choose a different name")
		add("reserved keywords include: val, var, fn, if, else, while, each, match, return, true, false, null")
		add("rename to something like `" + name + "_value` or `my_" + name + "`")
		return out

	case "E0074":
		add("`" + name + "` is a Lunex built-in — shadowing it may cause unexpected behavior")
		add("rename your declaration to avoid conflicts with built-in functions")
		return out

	case "E0075":
		add("identifier `" + name + "` is invalid — names must start with a letter or underscore")
		add("valid examples: `myVar`, `_count`, `item2`")
		return out

	case "E0076":
		add("parameter `" + name + "` is a reserved keyword — use a different name")
		return out

	case "E0077":
		add("field `" + name + "` is a reserved keyword — rename it")
		return out

	case "E0078":
		add("check the types of both operands with `typeof(x)` before applying the operator")
		add("use explicit conversion: Number(x), str(x), or bool(x)")
		return out

	case "E0079":
		add("a sub-expression evaluated to undefined — check all variables are initialized")
		add("guard against undefined:  if x != null { ... }")
		add("use optional chaining:  x?.property")
		return out

	case "E0080":
		add("Lunex has no global print, println, or log output function")
		add("import `std.io` and use `io.log(...)`")
		return out

	case "E0432":
		add("export only declared or imported names")
		return out

	case "E0110":
		add("define the variable in the environment before calling `env.require`")
		add("use `env.get(\"KEY\", fallback)` when a missing variable should not be an error")
		return out

	case "E0111":
		add("check that the dotenv file exists, is readable, and contains valid assignments")
		add("use `env.config(path)` to inspect the parsed values and the returned error object")
		return out

	case "E0112":
		add("resolve every lexer, parser, resolver, checker, or module-graph diagnostic before packing")
		add("run `lunex check <file.lx>` to inspect the same semantic pipeline without emitting an archive")
		return out

	case "E0113":
		add("check the line and column named in the message — that's exactly where the JSON text stops being valid")
		add("use `std.json.isValid(text)` first if the input might not be well-formed JSON")
		return out

	case "E0114":
		add("break the cycle before calling stringify, or provide a `toJSON()` that returns a plain, acyclic value")
		add("pass a replacer function to `stringify` to drop or flatten the self-referencing field")
		return out

	case "E0115":
		add("confirm the path exists, is readable or writable, and that the process has permission to access it")
		return out

	case "E0116":
		add("flatten or restructure the value — std.json caps nesting at 512 levels to avoid a runaway parse or serialize")
		return out
	}

	switch {
	case strings.Contains(lower, "is not a function") || strings.Contains(lower, "not callable"):
		add("make sure the value is declared as a function with `fn` before calling it")
	case strings.Contains(lower, "is not defined") || strings.Contains(lower, "not declared"):
		add("declare the variable before use, or check for a typo in the name")
		if len(similar) > 0 {
			add("did you mean `" + similar[0] + "`?")
		}
	case strings.Contains(lower, "cannot reassign") || strings.Contains(lower, "immutable"):
		add("use `var` instead of `val` to declare a mutable binding")
	case strings.Contains(lower, "unexpected token"):
		add("check for missing brackets, colons, or keywords near this position")
	case strings.Contains(lower, "division by zero"):
		add("guard the denominator with an `if` check before dividing")
	case strings.Contains(lower, "cannot resolve module"):
		add("install with lunex-pm, or verify the module name in the stdlib list")
	case strings.Contains(lower, "stack overflow") || strings.Contains(lower, "recursion"):
		add("add a base case to your recursive function to stop infinite recursion")
	case strings.Contains(lower, "index out of range") || strings.Contains(lower, "out of bounds"):
		add("check the array length before accessing an index:  if i < arr.length { ... }")
	case strings.Contains(lower, "null") || strings.Contains(lower, "undefined"):
		add("guard the value:  if x != null { ... }")
		add("use optional chaining:  x?.property")
	case strings.Contains(lower, "type"):
		add("verify the value's type with `typeof(value)` and convert explicitly if needed")
	case strings.Contains(lower, "expected"):
		add("review the syntax here — a keyword or delimiter may be missing")
	}
	return out
}

func extractName(msg string) string {
	for _, q := range []byte{'\'', '"', '`'} {
		start := strings.IndexByte(msg, q)
		if start >= 0 {
			end := strings.IndexByte(msg[start+1:], q)
			if end >= 0 {
				return msg[start+1 : start+1+end]
			}
		}
	}
	return ""
}

func Format(err *LunexError) string {
	if err == nil {
		return ""
	}

	meta, hasMeta := kindRegistry[err.Kind]
	if !hasMeta {
		meta = kindMeta{
			label:    err.Phase,
			severity: SeverityError,
			color:    errColor,
			icon:     "✗",
		}
		if meta.label == "" {
			meta.label = "error"
		}
	}

	label := meta.label
	if err.Code != "" {
		if ec, ok := codeRegistry[err.Code]; ok && ec.Code != "" {
			if len(ec.Code) == 5 && (ec.Code[0] == 'E' || ec.Code[0] == 'W') {
				bracketIdx := strings.Index(label, "[")
				if bracketIdx >= 0 {
					label = label[:bracketIdx] + "[" + ec.Code + "]" + label[bracketIdx:]
				} else {
					label = label + "[" + ec.Code + "]"
				}
			}
		}
	}

	msg := err.Message
	name := extractName(msg)
	if name == "" && err.Code != "" && len(err.Code) > 5 {
		name = err.Code
	}

	sugs := buildSuggestions(err.Code, name, msg, err.Similar)
	if err.Suggestion != "" {
		sugs = []string{err.Suggestion}
	}

	ulLabel := resolveUnderlineLabel(err.Code, msg, name)

	var out []string
	out = append(out, "")
	headerPrefix := meta.color(bold(meta.icon+" "+label)) + bold(": ")
	out = append(out, wrapIndented(headerPrefix, msg))

	if err.File != "" || err.Line > 0 {
		var parts []string
		if err.File != "" && err.File != "<unknown>" {
			parts = append(parts, err.File)
		}
		if err.Line > 0 {
			parts = append(parts, fmt.Sprintf("%d", err.Line))
		}
		if err.Col > 1 {
			parts = append(parts, fmt.Sprintf("%d", err.Col))
		}
		out = append(out, blue("  ──▶ ")+white(strings.Join(parts, ":")))
	}

	if len(err.Lines) > 0 && err.Line > 0 {
		out = append(out, "")
		view := buildSourceView(err.Lines, err.Line, err.Col, ulLabel, err.SecondaryLabels, meta.severity)
		if view != "" {
			out = append(out, view)
		}
	}

	if len(sugs) > 0 {
		out = append(out, "")
		for i, s := range sugs {
			prefix := hintColor("     or: ")
			if i == 0 {
				prefix = hintColor("  help: ")
			}
			out = append(out, wrapIndented(prefix, s))
		}
	}

	if len(err.Similar) > 0 {
		out = append(out, "")
		out = append(out, yellow("  note: ")+"similar names in scope:")
		for _, s := range err.Similar {
			out = append(out, "        "+cyan("• ")+brightCyan(s))
		}
	}

	if err.ExBad != "" || err.ExGood != "" {
		out = append(out, "")
		if err.ExBad != "" {
			out = append(out, red("  ✗ avoid: ")+dim(err.ExBad))
		}
		if err.ExGood != "" {
			out = append(out, green("  ✓ use:   ")+brightGreen(err.ExGood))
		}
	}

	if len(err.Stack) > 0 {
		out = append(out, "")
		out = append(out, gray("  stack trace:"))
		for i, frame := range err.Stack {
			if i >= 12 {
				out = append(out, fmt.Sprintf("  %s", gray(fmt.Sprintf("    ··· %d more frames omitted", len(err.Stack)-i))))
				break
			}
			fn := frame.FnName
			if fn == "" {
				fn = "<anonymous>"
			}
			frameStr := "    " + gray("at") + " " + brightYellow(fn)
			if frame.File != "" {
				frameStr += " " + gray("(") + dim(frame.File)
				if frame.Line > 0 {
					frameStr += gray(fmt.Sprintf(":%d", frame.Line))
					if frame.Col > 0 {
						frameStr += gray(fmt.Sprintf(":%d", frame.Col))
					}
				}
				frameStr += gray(")")
			}
			out = append(out, frameStr)
		}
	}

	if len(err.Notes) > 0 {
		out = append(out, "")
		for _, note := range err.Notes {
			out = append(out, wrapIndented(brightMagenta("  note: "), note))
		}
	}

	out = append(out, "")
	return strings.Join(out, "\n") + "\n"
}

func Print(err *LunexError) {
	fmt.Fprint(os.Stderr, Format(err))
}

func New(kind ErrorKind, code, msg, file string, line, col int, lines []string) *LunexError {
	return &LunexError{
		Kind:    kind,
		Code:    code,
		Message: msg,
		File:    file,
		Line:    line,
		Col:     col,
		Lines:   lines,
	}
}

func FormatSimple(msg, file string, line, col int) string {
	return Format(&LunexError{
		Message: msg,
		File:    file,
		Line:    line,
		Col:     col,
		Kind:    KindRuntime,
	})
}

func SyntaxError(msg, suggestion, file string, line, col int, lines []string) *LunexError {
	return &LunexError{
		Message:    msg,
		File:       file,
		Line:       line,
		Col:        col,
		Kind:       KindSyntax,
		Code:       "E0050",
		Suggestion: suggestion,
		Lines:      lines,
	}
}

func ParseError(msg, file string, line, col int, lines []string) *LunexError {
	return &LunexError{
		Message: msg,
		File:    file,
		Line:    line,
		Col:     col,
		Kind:    KindParse,
		Code:    "E0050",
		Lines:   lines,
	}
}

func NewTypeError(msg, suggestion string) *LunexError {
	return &LunexError{
		Message:    msg,
		Kind:       KindType,
		Code:       "E0020",
		Suggestion: suggestion,
	}
}

func TypeError(msg, file string, line, col int, lines []string) *LunexError {
	return &LunexError{
		Message: msg,
		File:    file,
		Line:    line,
		Col:     col,
		Kind:    KindType,
		Code:    "E0020",
		Lines:   lines,
	}
}

func TopLevelStatementError(kind, file string, line, col int, lines []string) *LunexError {
	return &LunexError{
		Message:    fmt.Sprintf("statement of type `%s` is not allowed at the top level", kind),
		File:       file,
		Line:       line,
		Col:        col,
		Kind:       KindSyntax,
		Code:       "E0071",
		Lines:      lines,
		Notes:      []string{"only declarations (`fn`, `val`, `var`, `class`) and imports are allowed outside `main`"},
		Suggestion: "move this code inside `fn main() { ... }`",
	}
}

func TopLevelCallError(callee, file string, line, col int, lines []string) *LunexError {
	suggestion := "move this call inside `fn main() { ... }`"
	if callee != "" {
		suggestion = "move `" + callee + "(...)` inside `fn main() { ... }`"
	}
	return &LunexError{
		Message: fmt.Sprintf("function call `%s(...)` is not allowed at the top level", callee),
		File:    file,
		Line:    line,
		Col:     col,
		Kind:    KindSyntax,
		Code:    "E0071",
		Lines:   lines,
		Notes: []string{
			"top-level code is limited to declarations and imports",
			"all executable logic must live inside `fn main()`",
		},
		Suggestion: suggestion,
		ExBad:      "test()   top-level call not allowed",
		ExGood:     "fn main() {\n  test()\n}",
	}
}

func TopLevelAssignmentError(file string, line, col int, lines []string) *LunexError {
	return &LunexError{
		Message:    "top-level assignment is not allowed; use `val` or `var` instead",
		File:       file,
		Line:       line,
		Col:        col,
		Kind:       KindSyntax,
		Code:       "E0071",
		Lines:      lines,
		Suggestion: "use a declaration:  val name = <value>",
	}
}

func ExplicitMainCallError(file string, line, col int, lines []string) *LunexError {
	return &LunexError{
		Message: "explicit call to `main()` is not allowed",
		File:    file,
		Line:    line,
		Col:     col,
		Kind:    KindSyntax,
		Code:    "E0072",
		Lines:   lines,
		Notes: []string{
			"`main` is the entry point and Lunex calls it automatically",
			"calling `main()` manually re-enters the program",
		},
		Suggestion: "remove the `main()` call; Lunex runs it automatically",
	}
}

func MissingMainError(file string, lines []string) *LunexError {
	return &LunexError{
		Message: "entry point `main` is not defined",
		File:    file,
		Kind:    KindReference,
		Code:    "E0070",
		Lines:   lines,
		Notes: []string{
			"every Lunex program requires a `fn main()` entry point",
			"top-level code outside `main` is not allowed in executable files",
		},
		Suggestion: "add a main function:\n\n  fn main() {\n    your code here\n  }",
		ExGood:     "fn main() {\n  val io = @import(\"std.io\")\n  io.log(\"hello\")\n}",
		ExBad:      "val io = @import(\"std.io\")\nio.log(\"hello\")",
	}
}

func ReferenceError(name, file string, line, col int, lines []string) *LunexError {
	return &LunexError{
		Message: fmt.Sprintf("variable `%s` was not defined", name),
		File:    file,
		Line:    line,
		Col:     col,
		Kind:    KindReference,
		Code:    "E0001",
		Lines:   lines,
	}
}

func ReferenceErrorWithSimilar(name, file string, line, col int, lines []string, similar []string) *LunexError {
	return &LunexError{
		Message: fmt.Sprintf("variable `%s` was not defined", name),
		File:    file,
		Line:    line,
		Col:     col,
		Kind:    KindReference,
		Code:    "E0001",
		Lines:   lines,
		Similar: similar,
	}
}

func ConstReassignError(name, file string, line, col int, lines []string) *LunexError {
	return &LunexError{
		Message: fmt.Sprintf("cannot reassign immutable binding '%s'", name),
		File:    file,
		Line:    line,
		Col:     col,
		Kind:    KindReference,
		Code:    "E0005",
		Lines:   lines,
	}
}

func NotCallableError(name, file string, line, col int, lines []string) *LunexError {
	return &LunexError{
		Message: fmt.Sprintf("'%s' is not a function", name),
		File:    file,
		Line:    line,
		Col:     col,
		Kind:    KindType,
		Code:    "E0003",
		Lines:   lines,
	}
}

func NullAccessError(name, file string, line, col int, lines []string) *LunexError {
	return &LunexError{
		Message: fmt.Sprintf("cannot read property of null: '%s'", name),
		File:    file,
		Line:    line,
		Col:     col,
		Kind:    KindRuntime,
		Code:    "E0004",
		Lines:   lines,
	}
}

func ImportError(mod, file string, line int) *LunexError {
	return &LunexError{
		Message: fmt.Sprintf("module not found: '%s'", mod),
		File:    file,
		Line:    line,
		Kind:    KindImport,
		Code:    "E0010",
	}
}

func ImportNoExportError(mod, export, file string, line int) *LunexError {
	return &LunexError{
		Message: fmt.Sprintf("module '%s' has no export named '%s'", mod, export),
		File:    file,
		Line:    line,
		Kind:    KindImport,
		Code:    "E0011",
	}
}

func CircularImportError(mod, file string, line int) *LunexError {
	return &LunexError{
		Message: fmt.Sprintf("circular import: '%s' is already being loaded", mod),
		File:    file,
		Line:    line,
		Kind:    KindImport,
		Code:    "E0012",
	}
}

func UnresolvedExportError(name, file string, line, col int, lines []string) *LunexError {
	return &LunexError{
		Message: fmt.Sprintf("unresolved export `%s`", name),
		File:    file,
		Line:    line,
		Col:     col,
		Kind:    KindImport,
		Code:    "E0432",
		Lines:   lines,
	}
}

func AssertionError(expr, file string, line int, lines []string) *LunexError {
	return &LunexError{
		Message: fmt.Sprintf("assertion failed: %s", expr),
		File:    file,
		Line:    line,
		Kind:    KindAssertion,
		Code:    "E0061",
		Lines:   lines,
	}
}

func DivisionByZeroError(file string, line, col int, lines []string) *LunexError {
	return &LunexError{
		Message: "division by zero",
		File:    file,
		Line:    line,
		Col:     col,
		Kind:    KindArithmetic,
		Code:    "E0030",
		Lines:   lines,
	}
}

func IndexOutOfBoundsError(index, length int, file string, line, col int, lines []string) *LunexError {
	return &LunexError{
		Message: fmt.Sprintf("index %d out of bounds (length %d)", index, length),
		File:    file,
		Line:    line,
		Col:     col,
		Kind:    KindRange,
		Code:    "E0040",
		Lines:   lines,
	}
}

func StackOverflowError(depth int, file string, line int) *LunexError {
	return &LunexError{
		Message: fmt.Sprintf("call stack depth exceeded (%d frames)", depth),
		File:    file,
		Line:    line,
		Kind:    KindRecursion,
		Code:    "E0060",
	}
}

func DeprecationWarning(name, replacement, file string, line, col int, lines []string) *LunexError {
	e := &LunexError{
		Message: fmt.Sprintf("'%s' is deprecated", name),
		File:    file,
		Line:    line,
		Col:     col,
		Kind:    KindDeprecated,
		Code:    "W0001",
		Lines:   lines,
	}
	if replacement != "" {
		e.Notes = []string{"use `" + replacement + "` instead"}
	}
	return e
}

func UnusedVariableWarning(name, file string, line, col int, lines []string) *LunexError {
	return &LunexError{
		Message: fmt.Sprintf("variable '%s' is declared but never used", name),
		File:    file,
		Line:    line,
		Col:     col,
		Kind:    KindDeprecated,
		Code:    "W0004",
		Lines:   lines,
	}
}

func SuggestForMessage(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "is not a function") || strings.Contains(lower, "not callable"):
		return "make sure the value is declared as a function with `fn` before calling it"
	case strings.Contains(lower, "is not defined") || strings.Contains(lower, "not declared"):
		return "declare the variable before use, or check for typos in the name"
	case strings.Contains(lower, "cannot reassign") || strings.Contains(lower, "immutable"):
		return "use `var` instead of `val` if you need a mutable binding"
	case strings.Contains(lower, "unexpected token"):
		return "check for missing brackets, colons, or keywords near this position"
	case strings.Contains(lower, "expected"):
		return "review the syntax here — a keyword or delimiter may be missing"
	case strings.Contains(lower, "division by zero"):
		return "guard the denominator with an `if` check before dividing"
	case strings.Contains(lower, "null") || strings.Contains(lower, "undefined"):
		return "use `if x != null` or optional chaining `x?.prop` to safely access nullable values"
	case strings.Contains(lower, "cannot resolve module"):
		return "install with lunex-pm, or check the module name in the stdlib list"
	case strings.Contains(lower, "stack overflow") || strings.Contains(lower, "recursion"):
		return "add a base case to your recursive function to stop infinite recursion"
	case strings.Contains(lower, "index out of range") || strings.Contains(lower, "out of bounds"):
		return "check the array length before accessing an index:  if i < arr.length { ... }"
	case strings.Contains(lower, "cannot read"):
		return "the value may be null or undefined — use optional chaining (?.) or a guard check"
	case strings.Contains(lower, "timeout"):
		return "increase the timeout limit or add cancellation logic"
	case strings.Contains(lower, "permission"):
		return "check file ownership and process permissions"
	case strings.Contains(lower, "encoding") || strings.Contains(lower, "utf"):
		return "ensure the input is valid UTF-8, or use bytes.readRaw() for binary data"
	}
	return ""
}
