package checker

import (
	"fmt"
	"lunex/internal/errfmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionChecker(t *testing.T) {
	dir := t.TempDir()
	mod := filepath.Join(dir, "math.lx")
	if err := os.WriteFile(mod, []byte("fn add(a, b) { a + b }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	loader := func(path, from string) (string, string, bool) {
		p := path
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(from), p)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return "", p, false
		}
		return string(b), p, true
	}
	good := `val math = @fimport("./math.lx")
fn main() {
  val x = math.add(1, 2)
}`
	r := New(Options{Loader: loader}).Check(good, filepath.Join(dir, "main.lx"))
	if len(r.Diagnostics) != 0 {
		t.Fatalf("good source diagnostics: %+v", r.Diagnostics)
	}

	bad := `val math = @fimport("./math.lx")
fn main() {
  val x = math.missing(1, 2)
  nope(x)
}`
	r = New(Options{Loader: loader}).Check(bad, filepath.Join(dir, "main2.lx"))
	if len(r.Diagnostics) != 1 {
		t.Fatalf("expected unresolved symbol error, got %+v", r.Diagnostics)
	}
}

func TestProductionCheckerRules(t *testing.T) {
	dir := t.TempDir()
	loader := func(path, from string) (string, string, bool) {
		p := path
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(from), p)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return "", p, false
		}
		return string(b), p, true
	}
	cases := []struct{ name, src, code string }{
		{"missing main", `val x = 1`, "E0070"},
		{"top level call", `io.log("x")
fn main() {}`, "E0071"},
		{"top level undefined", `fn main() {
  missing
}`, "E0001"},
		{"const assignment", `val x = 1
fn main() { x = 2 }`, "E0005"},
		{"unresolved import", `val missing = @import("./missing.lx")
fn main() {}`, "E0010"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := New(Options{Loader: loader}).Check(tc.src, filepath.Join(dir, tc.name+".lx"))
			found := false
			for _, d := range r.Diagnostics {
				if d.Code == tc.code {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected %s, got %+v", tc.code, r.Diagnostics)
			}
		})
	}
	mathPath := filepath.Join(dir, "math.lx")
	if err := os.WriteFile(mathPath, []byte("fn add(a, b) { a + b }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	r := New(Options{Loader: loader}).Check(`import { add } from "./math.lx"
fn main() { add(1, 2) }`, filepath.Join(dir, "import.lx"))
	if len(r.Diagnostics) != 0 {
		t.Fatalf("import syntax diagnostics: %+v", r.Diagnostics)
	}
}

func TestCheckerErrorCodesRegistered(t *testing.T) {
	codes := []string{"E0001", "E0005", "E0003", "E0010", "E0012", "E0070", "E0071", "E0072", "E0432", "E0080"}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			if _, ok := errfmt.LookupCode(code); !ok {
				t.Fatalf("checker error code %s is not registered in errfmt", code)
			}
		})
	}
}

func TestCheckerUsesErrfmt(t *testing.T) {
	r := New(Options{}).Check(`fn main() { missing() }`, "main.lx")
	if len(r.Diagnostics) == 0 {
		t.Fatal("expected diagnostic")
	}
	e := &r.Diagnostics[0]
	if _, ok := errfmt.LookupCode(e.Code); !ok {
		t.Fatalf("checker returned unregistered error code %q", e.Code)
	}
	if got := errfmt.Format(e); got == "" {
		t.Fatal("errfmt returned an empty diagnostic")
	}
}

func TestCheckerDoesNotReclassifyTopLevelReference(t *testing.T) {
	src := `fn main() {
  io.log("test")
}
g`
	r := New(Options{}).Check(src, "test.lx")
	if len(r.Diagnostics) != 1 {
		t.Fatalf("expected exactly one diagnostic, got %+v", r.Diagnostics)
	}
	e := &r.Diagnostics[0]
	if e.Code != "E0001" {
		t.Fatalf("expected E0001, got %s", e.Code)
	}
	if e.Message != "variable `g` was not defined" {
		t.Fatalf("unexpected message: %s", e.Message)
	}
}

func TestCheckerDoesNotRejectCallsInsideDeclarations(t *testing.T) {
	src := `fn makeValue() { 1 }
val x = makeValue()
fn main() {}`
	r := New(Options{}).Check(src, "test.lx")
	if len(r.Diagnostics) != 0 {
		t.Fatalf("declaration initializer produced diagnostics: %+v", r.Diagnostics)
	}
}

func TestCheckerMatchesRuntimeUndefinedVariableDiagnostic(t *testing.T) {
	src := `fn main() {
  missing
}`
	r := New(Options{}).Check(src, "main.lx")
	if len(r.Diagnostics) != 1 {
		t.Fatalf("expected one diagnostic, got %+v", r.Diagnostics)
	}
	e := &r.Diagnostics[0]
	if e.Code != "E0001" {
		t.Fatalf("expected E0001, got %s", e.Code)
	}
	if e.Message != "variable `missing` was not defined" {
		t.Fatalf("unexpected message: %s", e.Message)
	}
	expected := errfmt.ReferenceErrorWithSimilar("missing", "main.lx", 2, 3, strings.Split(src, "\n"), nil)
	if e.Code != expected.Code || e.Message != expected.Message || e.Kind != expected.Kind {
		t.Fatalf("checker diagnostic is not the canonical runtime diagnostic: got=%+v expected=%+v", *e, *expected)
	}
}
