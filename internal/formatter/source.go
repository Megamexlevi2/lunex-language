package formatter

import (
	"fmt"
	"reflect"
	"strings"

	"lunex/internal/lexer"
	"lunex/internal/parser"
)

type FormatVerifier struct{}

type SourceResult struct {
	Code        string
	Strategy    string
	Verified    bool
	SyntaxValid bool
	Reason      string
}

func FormatSource(source string) SourceResult {
	result := SourceResult{Code: source, Strategy: "safe-whitespace"}
	tokens, lexErr := lexer.TokenizeWithComments(source, "<fmt>")
	if lexErr != nil {
		result.Reason = lexErr.Error()
		return result
	}
	parseTokens := withoutComments(tokens)
	root, parseErr := parser.Parse(parseTokens, "<fmt>")
	result.SyntaxValid = parseErr == nil
	if parseErr == nil {
		candidate := formatTokenStream(source, tokens, root)
		if verifyErr := (FormatVerifier{}).Verify(source, candidate); verifyErr == nil {
			result.Code = candidate
			result.Strategy = "syntax"
			result.Verified = true
			return result
		} else {
			result.Reason = verifyErr.Error()
		}
		astCandidate := Format(root)
		if strings.ContainsAny(source, "/") {
			astCandidate, lexErr = preserveCommentGaps(source, astCandidate, tokens)
		}
		if lexErr == nil && (FormatVerifier{}).Verify(source, astCandidate) == nil {
			result.Code = astCandidate
			result.Strategy = "ast"
			result.Verified = true
			result.Reason = ""
			return result
		}
	} else {
		result.Reason = parseErr.Error()
	}
	result.Code = formatWhitespaceOnly(source, tokens)
	if err := (FormatVerifier{}).VerifyTokens(source, result.Code); err == nil {
		result.Verified = true
	} else {
		result.Code = source
		result.Reason = strings.TrimSpace(result.Reason + "; " + err.Error())
		return result
	}
	if result.SyntaxValid {
		if err := (FormatVerifier{}).Verify(source, result.Code); err != nil {
			result.Code = source
			result.Reason = strings.TrimSpace(result.Reason + "; " + err.Error())
			result.Verified = (FormatVerifier{}).VerifyTokens(source, result.Code) == nil
		}
	}
	return result
}

func preserveCommentGaps(original, candidate string, originalTokens []lexer.Token) (string, error) {
	originalRunes := []rune(original)
	candidateTokens, err := lexer.TokenizeWithComments(candidate, "<fmt-candidate>")
	if err != nil {
		return "", err
	}
	originalSpans := significantSpans(originalRunes, originalTokens)
	candidateRunes := []rune(candidate)
	candidateSpans := significantSpans(candidateRunes, candidateTokens)
	if len(originalSpans) != len(candidateSpans) {
		return "", fmt.Errorf("AST output changed the number of significant tokens")
	}
	for i := range originalSpans {
		if originalSpans[i].token.Type != candidateSpans[i].token.Type || originalSpans[i].token.Raw != candidateSpans[i].token.Raw || !reflect.DeepEqual(originalSpans[i].token.Value, candidateSpans[i].token.Value) {
			return "", fmt.Errorf("AST output changed significant token %d", i+1)
		}
	}
	originalLineStarts := lineStarts(originalRunes)
	candidateLineStarts := lineStarts(candidateRunes)
	commentSpans := make([]tokenSpan, 0)
	for _, token := range originalTokens {
		if token.Type == lexer.COMMENT {
			start := sourceOffset(originalLineStarts, token.Line, token.Col, len(originalRunes))
			commentSpans = append(commentSpans, tokenSpan{token: token, start: start, end: minInt(start+len([]rune(token.Raw)), len(originalRunes))})
		}
	}
	newline := "\n"
	edits := make([]gapEdit, 0)
	for gap := 0; gap <= len(originalSpans); gap++ {
		originalStart, originalEnd := 0, len(originalRunes)
		candidateStart, candidateEnd := 0, len(candidateRunes)
		if gap > 0 {
			originalStart = originalSpans[gap-1].end
			candidateStart = candidateSpans[gap-1].end
		}
		if gap < len(originalSpans) {
			originalEnd = originalSpans[gap].start
			candidateEnd = candidateSpans[gap].start
		}
		var comments []tokenSpan
		for _, span := range commentSpans {
			if span.start >= originalStart && span.start < originalEnd {
				comments = append(comments, span)
			}
		}
		if len(comments) == 0 {
			continue
		}
		if originalStart > originalEnd || candidateStart > candidateEnd || originalStart < 0 || candidateStart < 0 || originalEnd > len(originalRunes) || candidateEnd > len(candidateRunes) {
			return "", fmt.Errorf("comment gap could not be mapped safely")
		}
		targetIndent := 0
		if gap < len(candidateSpans) {
			targetIndent = indentationAt(candidateRunes, candidateLineStarts, candidateSpans[gap].start)
		} else if gap > 0 {
			targetIndent = indentationAt(candidateRunes, candidateLineStarts, candidateSpans[gap-1].start)
		}
		var replacement strings.Builder
		cursor := originalStart
		hasPrevious := gap > 0
		for _, span := range comments {
			if span.start < cursor || span.end > originalEnd {
				return "", fmt.Errorf("overlapping comment spans could not be mapped safely")
			}
			whitespace := originalRunes[cursor:span.start]
			replacement.WriteString(normalizeCommentWhitespace(whitespace, targetIndent, hasPrevious, true, newline))
			replacement.WriteString(span.token.Raw)
			cursor = span.end
			hasPrevious = true
		}
		trailingWhitespace := originalRunes[cursor:originalEnd]
		replacement.WriteString(normalizeCommentWhitespace(trailingWhitespace, targetIndent, hasPrevious, gap < len(originalSpans), newline))
		edits = append(edits, gapEdit{start: candidateStart, end: candidateEnd, replacement: []rune(replacement.String())})
	}
	for i := len(edits) - 1; i >= 0; i-- {
		edit := edits[i]
		updated := make([]rune, 0, len(candidateRunes)-(edit.end-edit.start)+len(edit.replacement))
		updated = append(updated, candidateRunes[:edit.start]...)
		updated = append(updated, edit.replacement...)
		updated = append(updated, candidateRunes[edit.end:]...)
		candidateRunes = updated
	}
	return string(candidateRunes), nil
}

func normalizeCommentWhitespace(source []rune, indent int, hasPrevious, hasNext bool, newline string) string {
	if indent < 0 {
		indent = 0
	}
	newlines := 0
	for _, ch := range source {
		if ch == '\n' {
			newlines++
		}
	}
	if newlines > 0 {
		return strings.Repeat(newline, newlines) + strings.Repeat(" ", indent)
	}
	if hasPrevious && hasNext {
		return " "
	}
	if !hasPrevious && hasNext {
		return strings.Repeat(" ", indent)
	}
	return string(source)
}

func indentationAt(source []rune, starts []int, offset int) int {
	line := 0
	for i := len(starts) - 1; i >= 0; i-- {
		if starts[i] <= offset {
			line = i
			break
		}
	}
	indent := 0
	for i := starts[line]; i < len(source) && i < offset; i++ {
		if source[i] == ' ' {
			indent++
			continue
		}
		if source[i] == '\t' {
			indent += 2
			continue
		}
		break
	}
	return indent
}

type tokenSpan struct {
	token lexer.Token
	start int
	end   int
}

type gapEdit struct {
	start       int
	end         int
	replacement []rune
}

func significantSpans(source []rune, tokens []lexer.Token) []tokenSpan {
	starts := lineStarts(source)
	spans := make([]tokenSpan, 0, len(tokens))
	for _, token := range tokens {
		if token.Type == lexer.COMMENT || token.Type == lexer.EOF {
			continue
		}
		start := sourceOffset(starts, token.Line, token.Col, len(source))
		end := minInt(start+len([]rune(token.Raw)), len(source))
		spans = append(spans, tokenSpan{token: token, start: start, end: end})
	}
	return spans
}

func lineStarts(source []rune) []int {
	starts := []int{0}
	for i, ch := range source {
		if ch == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func sourceOffset(starts []int, line, col, sourceLength int) int {
	if line < 1 || line > len(starts) || col < 1 {
		return sourceLength
	}
	offset := starts[line-1] + col - 1
	if offset < 0 {
		return 0
	}
	if offset > sourceLength {
		return sourceLength
	}
	return offset
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func VerifyTokenPreservation(original, formatted string) error {
	return (FormatVerifier{}).VerifyTokens(original, formatted)
}

func (FormatVerifier) VerifyTokens(original, formatted string) error {
	before, err := lexer.TokenizeWithComments(original, "<fmt-original>")
	if err != nil {
		return fmt.Errorf("original source cannot be tokenized: %w", err)
	}
	after, err := lexer.TokenizeWithComments(formatted, "<fmt-formatted>")
	if err != nil {
		return fmt.Errorf("formatted source cannot be tokenized: %w", err)
	}
	if len(before) != len(after) {
		return fmt.Errorf("token count changed from %d to %d", len(before), len(after))
	}
	for i := range before {
		if before[i].Type != after[i].Type {
			return fmt.Errorf("token %d changed type from %s to %s", i+1, before[i].Type, after[i].Type)
		}
		if before[i].Raw != after[i].Raw {
			return fmt.Errorf("token %d changed source spelling from %q to %q", i+1, before[i].Raw, after[i].Raw)
		}
		if before[i].Type == lexer.COMMENT {
			continue
		}
		if !reflect.DeepEqual(before[i].Value, after[i].Value) {
			return fmt.Errorf("token %d changed value from %q to %q", i+1, fmt.Sprint(before[i].Value), fmt.Sprint(after[i].Value))
		}
	}
	return nil
}

func VerifySource(original, formatted string) error {
	return (FormatVerifier{}).Verify(original, formatted)
}

func (FormatVerifier) Verify(original, formatted string) error {
	if err := (FormatVerifier{}).VerifyTokens(original, formatted); err != nil {
		return err
	}
	before, _ := lexer.TokenizeWithComments(original, "<fmt-original>")
	after, _ := lexer.TokenizeWithComments(formatted, "<fmt-formatted>")
	beforeAST, err := parser.Parse(withoutComments(before), "<fmt-original>")
	if err != nil {
		return fmt.Errorf("original source does not parse: %w", err)
	}
	afterAST, err := parser.Parse(withoutComments(after), "<fmt-formatted>")
	if err != nil {
		return fmt.Errorf("formatted source does not parse: %w", err)
	}
	if !structurallyEqual(reflect.ValueOf(beforeAST), reflect.ValueOf(afterAST)) {
		return fmt.Errorf("formatted source changed the parsed AST structure")
	}
	return nil
}

func structurallyEqual(left, right reflect.Value) bool {
	if !left.IsValid() || !right.IsValid() {
		return left.IsValid() == right.IsValid()
	}
	if left.Type() != right.Type() {
		return false
	}
	switch left.Kind() {
	case reflect.Interface, reflect.Pointer:
		if left.IsNil() || right.IsNil() {
			return left.IsNil() == right.IsNil()
		}
		return structurallyEqual(left.Elem(), right.Elem())
	case reflect.Struct:
		for i := 0; i < left.NumField(); i++ {
			name := left.Type().Field(i).Name
			if name == "Line" || name == "Col" {
				continue
			}
			if !structurallyEqual(left.Field(i), right.Field(i)) {
				return false
			}
		}
		return true
	case reflect.Slice, reflect.Array:
		if left.Len() != right.Len() {
			return false
		}
		for i := 0; i < left.Len(); i++ {
			if !structurallyEqual(left.Index(i), right.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Map:
		if left.Len() != right.Len() {
			return false
		}
		iter := left.MapRange()
		for iter.Next() {
			other := right.MapIndex(iter.Key())
			if !other.IsValid() || !structurallyEqual(iter.Value(), other) {
				return false
			}
		}
		return true
	default:
		if !left.CanInterface() || !right.CanInterface() {
			return false
		}
		return reflect.DeepEqual(left.Interface(), right.Interface())
	}
}

func withoutComments(tokens []lexer.Token) []lexer.Token {
	out := make([]lexer.Token, 0, len(tokens))
	for _, token := range tokens {
		if token.Type != lexer.COMMENT {
			out = append(out, token)
		}
	}
	return out
}

func formatWhitespaceOnly(source string, tokens []lexer.Token) string {
	lines := strings.Split(source, "\n")
	lineTokens := make([][]lexer.Token, len(lines))
	protectedLines := make([]bool, len(lines))
	for _, token := range tokens {
		if token.Type == lexer.EOF || token.Line < 1 || token.Line > len(lines) {
			continue
		}
		lineTokens[token.Line-1] = append(lineTokens[token.Line-1], token)
		if token.Type == lexer.COMMENT && strings.HasPrefix(token.Raw, "/*") {
			start := token.Line - 1
			end := start + strings.Count(token.Raw, "\n")
			if end >= len(protectedLines) {
				end = len(protectedLines) - 1
			}
			for line := start; line <= end; line++ {
				protectedLines[line] = true
			}
		}
	}
	depth := 0
	for i, line := range lines {
		lineItems := lineTokens[i]
		leadingClosers := 0
		for _, token := range lineItems {
			if token.Type != lexer.PUNCTUATION {
				continue
			}
			if token.StrVal() == "}" {
				leadingClosers++
				continue
			}
			if token.Type == lexer.EOF {
				continue
			}
			break
		}
		opens, closes := 0, 0
		hasComment := false
		for _, token := range lineItems {
			if token.Type == lexer.COMMENT {
				hasComment = true
				continue
			}
			if token.Type != lexer.PUNCTUATION {
				continue
			}
			switch token.StrVal() {
			case "{":
				opens++
			case "}":
				closes++
			}
		}
		if !protectedLines[i] {
			lineEnding := ""
			if strings.HasSuffix(line, "\r") {
				lineEnding = "\r"
			}
			rawLine := strings.TrimSuffix(line, "\r")
			content := strings.TrimLeft(rawLine, " \t")
			if strings.TrimSpace(content) == "" {
				lines[i] = lineEnding
			} else {
				indent := depth - leadingClosers
				if indent < 0 {
					indent = 0
				}
				if !hasComment {
					content = strings.TrimRight(content, " \t\r")
				}
				lines[i] = strings.Repeat("  ", indent) + content + lineEnding
			}
		}
		depth += opens - closes
		if depth < 0 {
			depth = 0
		}
	}
	return strings.Join(lines, "\n")
}
