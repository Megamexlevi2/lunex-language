package formatter

import (
	"reflect"
	"strings"
	"testing"

	"lunex/internal/ast"
	"lunex/internal/lexer"
)

func TestFormatSourceUsesSyntaxAwareFormatterWhenVerified(t *testing.T) {
	source := "var answer=1\nif answer>0 {\nlog answer\n}\n"
	result := FormatSource(source)
	if !result.SyntaxValid {
		t.Fatalf("expected valid source, got %q", result.Reason)
	}
	if !result.Verified {
		t.Fatalf("expected verified formatting, got %q", result.Reason)
	}
	if result.Strategy != "syntax" {
		t.Fatalf("expected syntax-aware formatting, got %q", result.Strategy)
	}
	if err := VerifySource(source, result.Code); err != nil {
		t.Fatalf("formatted source did not verify: %v", err)
	}
	if strings.Count(result.Code, "if answer > 0 {") != 1 {
		t.Fatalf("unexpected if formatting: %s", result.Code)
	}
}

func TestFormatSourcePreservesNewCommentSyntax(t *testing.T) {
	source := "/! keep this line comment\nvar answer=1\n/* keep this\n   block comment */\nlog answer\n"
	result := FormatSource(source)
	if !strings.Contains(result.Code, "/! keep this line comment") {
		t.Fatal("line comment was removed")
	}
	if !strings.Contains(result.Code, "/* keep this\n   block comment */") {
		t.Fatal("block comment contents were changed")
	}
	if !result.Verified {
		t.Fatalf("comment-preserving fallback did not verify: %q", result.Reason)
	}
	if result.Strategy != "syntax" {
		t.Fatalf("expected syntax-aware formatting with comment preservation, got %q", result.Strategy)
	}
	if err := VerifySource(source, result.Code); err != nil {
		t.Fatalf("comment-preserving output did not verify: %v", err)
	}
}

func TestVerifySourceRejectsCorruption(t *testing.T) {
	if err := VerifySource("var answer=1", "var answer=2"); err == nil {
		t.Fatal("expected token corruption to be rejected")
	}
}

func TestFormatSourceUsesWhitespaceFallbackForInvalidSyntax(t *testing.T) {
	source := "if true {\nlog 1\n"
	result := FormatSource(source)
	if result.SyntaxValid {
		t.Fatal("expected invalid input syntax")
	}
	if !strings.Contains(result.Code, "log 1") {
		t.Fatal("fallback removed source text")
	}
	if strings.TrimSpace(result.Code) == "" {
		t.Fatal("fallback returned empty source")
	}
}

func TestVerifySourceRejectsTemplateSpellingChanges(t *testing.T) {
	original := "var message = `\\!{value}`"
	formatted := "var message = `!{value}`"
	if err := VerifyTokenPreservation(original, formatted); err == nil {
		t.Fatal("expected template source spelling changes to be rejected")
	}
}

func TestFormatSourceLeavesLexicallyInvalidInputUntouched(t *testing.T) {
	source := "var value = 1 §  "
	result := FormatSource(source)
	if result.Code != source {
		t.Fatalf("lexically invalid source was modified: %q", result.Code)
	}
	if result.Verified {
		t.Fatal("lexically invalid source must not be marked verified")
	}
}

func TestASTStructuralVerifierIgnoresLocationsButDetectsChanges(t *testing.T) {
	left := &ast.Node{Type: ast.ExprStmt, Line: 1, Col: 1, Expr: &ast.Node{Type: ast.Identifier, Name: "value", Line: 1, Col: 5}}
	right := &ast.Node{Type: ast.ExprStmt, Line: 8, Col: 4, Expr: &ast.Node{Type: ast.Identifier, Name: "value", Line: 8, Col: 9}}
	if !structurallyEqual(reflect.ValueOf(left), reflect.ValueOf(right)) {
		t.Fatal("source locations must not affect AST structure verification")
	}
	right.Expr.Type = ast.CallExpr
	if structurallyEqual(reflect.ValueOf(left), reflect.ValueOf(right)) {
		t.Fatal("AST node changes must be rejected")
	}
}

func TestFormatterPreservesFunctionExpressionBodies(t *testing.T) {
	source := "var transform = fn(value) { value + 1 }"
	result := FormatSource(source)
	if !result.SyntaxValid {
		t.Fatalf("expected valid source, got %q", result.Reason)
	}
	if !result.Verified {
		t.Fatalf("expected preserved fallback, got %q", result.Reason)
	}
	if result.Strategy != "syntax" {
		t.Fatalf("expected syntax-aware formatting, got %q", result.Strategy)
	}
	if !strings.Contains(result.Code, "fn(value) {") || !strings.Contains(result.Code, "value + 1") {
		t.Fatalf("function expression body was not formatted: %s", result.Code)
	}
}

func TestFormatSourcePreservesCommentsFromCRLFInput(t *testing.T) {
	source := "/! header\r\nfn calculate() {\r\n/* block\r\n   inside */\r\nvar result=1\r\n}\r\n"
	result := FormatSource(source)
	if !result.Verified || !result.SyntaxValid {
		t.Fatalf("expected verified formatting, got syntax=%v verified=%v reason=%q", result.SyntaxValid, result.Verified, result.Reason)
	}
	if err := VerifyTokenPreservation(source, result.Code); err != nil {
		t.Fatalf("comment preservation failed: %v", err)
	}
	if !strings.Contains(result.Code, "/* block\r\n   inside */") || !strings.Contains(result.Code, "/! header") {
		t.Fatalf("comments were changed: %q", result.Code)
	}
}

func TestFormatSourceFormatsInlineBlockComments(t *testing.T) {
	source := "var total=1 /* preserve inline */ + 2"
	result := FormatSource(source)
	if !result.SyntaxValid || !result.Verified {
		t.Fatalf("expected verified source: syntax=%v verified=%v reason=%q", result.SyntaxValid, result.Verified, result.Reason)
	}
	if !strings.Contains(result.Code, "1 /* preserve inline */ + 2") {
		t.Fatalf("inline comment was not preserved: %s", result.Code)
	}
}

func TestFormatSourceIsIdempotent(t *testing.T) {
	source := "/! file note\nfn calculate() {\n/* header\n   preserved */\nvar answer=1 /* inline */ + 2\n/! local note\nlog answer\n}"
	first := FormatSource(source)
	if !first.Verified {
		t.Fatalf("first pass was not verified: %q", first.Reason)
	}
	second := FormatSource(first.Code)
	if first.Code != second.Code {
		t.Fatalf("formatter output is not stable\nfirst:\n%s\nsecond:\n%s", first.Code, second.Code)
	}
}

func TestWhitespaceFallbackIgnoresStringBracesAndPreservesCRLF(t *testing.T) {
	source := "if true {\r\nvar marker = \"}\"\r\n}\r\n"
	tokens, err := lexer.TokenizeWithComments(source, "fallback.lx")
	if err != nil {
		t.Fatalf("unexpected tokenization error: %v", err)
	}
	formatted := formatWhitespaceOnly(source, tokens)
	if !strings.Contains(formatted, "\r\n  var marker = \"}\"\r\n}\r\n") {
		t.Fatalf("fallback mishandled braces in a string or CRLF endings: %q", formatted)
	}
}

func TestFormatSourceFormatsMinifiedFunctionsAndOperators(t *testing.T) {
	source := "fn add(a,b){val result=a+b;result}"
	result := FormatSource(source)
	if !result.SyntaxValid || !result.Verified {
		t.Fatalf("expected verified formatting: syntax=%v verified=%v reason=%q", result.SyntaxValid, result.Verified, result.Reason)
	}
	want := "fn add(a, b) {\n  val result = a + b;\n  result\n}\n"
	if result.Code != want {
		t.Fatalf("unexpected formatted output\nwant:\n%s\ngot:\n%s", want, result.Code)
	}
}

func TestFormatSourceKeepsShortObjectLiteralsInline(t *testing.T) {
	source := "val config={name:\"lunex\",items:[1,2,3],enabled:true}"
	result := FormatSource(source)
	if !result.Verified {
		t.Fatalf("expected verified formatting: %q", result.Reason)
	}
	want := "val config = { name: \"lunex\", items: [1, 2, 3], enabled: true }\n"
	if result.Code != want {
		t.Fatalf("unexpected object formatting\nwant:\n%s\ngot:\n%s", want, result.Code)
	}
}

func TestFormatSourceWrapsLongArrays(t *testing.T) {
	source := "val values=[1001,1002,1003,1004,1005,1006,1007,1008,1009,1010,1011,1012,1013,1014,1015,1016,1017,1018,1019,1020]"
	result := FormatSource(source)
	if !result.SyntaxValid || !result.Verified {
		t.Fatalf("expected verified formatting: syntax=%v verified=%v reason=%q", result.SyntaxValid, result.Verified, result.Reason)
	}
	if !strings.Contains(result.Code, "val values = [\n") || !strings.Contains(result.Code, "\n]\n") {
		t.Fatalf("expected a multiline array, got:\n%s", result.Code)
	}
}

func TestFormatSourcePreservesElseIfLayoutAndUnarySpacing(t *testing.T) {
	source := "fn adjust(x){if !x {x=-1}else if x<0{x=-x}else{x=x+1}}"
	result := FormatSource(source)
	if !result.SyntaxValid || !result.Verified {
		t.Fatalf("expected verified formatting: syntax=%v verified=%v reason=%q", result.SyntaxValid, result.Verified, result.Reason)
	}
	if !strings.Contains(result.Code, "} else if x < 0 {") {
		t.Fatalf("else-if chain was not formatted consistently:\n%s", result.Code)
	}
	if !strings.Contains(result.Code, "if !x {") || !strings.Contains(result.Code, "x = -1") || !strings.Contains(result.Code, "x = -x") {
		t.Fatalf("unary operators were formatted incorrectly:\n%s", result.Code)
	}
}

func TestFormatSourcePreservesCRLFLineEndings(t *testing.T) {
	source := "fn add(a,b){\r\nval result=a+b\r\nresult\r\n}\r\n"
	result := FormatSource(source)
	if !result.SyntaxValid || !result.Verified {
		t.Fatalf("expected verified formatting: syntax=%v verified=%v reason=%q", result.SyntaxValid, result.Verified, result.Reason)
	}
	if strings.Contains(strings.ReplaceAll(result.Code, "\r\n", ""), "\n") {
		t.Fatalf("expected consistent CRLF line endings, got %q", result.Code)
	}
}

func TestFormatSourceIsIdempotentWithNestedFunctionsAndCalls(t *testing.T) {
	source := `fn main() { val nums = [1, 2, 3]; io.log("map", nums.map(fn(x) { x * 2 })) }`
	first := FormatSource(source)
	if !first.SyntaxValid || !first.Verified {
		t.Fatalf("expected verified formatting: syntax=%v verified=%v reason=%q", first.SyntaxValid, first.Verified, first.Reason)
	}
	second := FormatSource(first.Code)
	if first.Code != second.Code {
		t.Fatalf("nested-call formatting is not stable\nfirst:\n%s\nsecond:\n%s", first.Code, second.Code)
	}
}
