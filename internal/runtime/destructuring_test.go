package runtime

import (
	"strings"
	"testing"
)

func TestObjectDestructuringUsesResolvedSlots(t *testing.T) {
	err := runRuntimeSource(t, `val { globalName } = { globalName: "Top" }
fn main() {
  if globalName != "Top" { throw "top-level binding failed" }
  val user = { name: "Ada", role: "admin" }
  val { name, role } = user
  if name != "Ada" { throw "name binding failed" }
  if role != "admin" { throw "role binding failed" }
}`)
	if err != nil {
		t.Fatalf("object destructuring failed: %v", err)
	}
}

func TestArrayDestructuringUsesResolvedSlots(t *testing.T) {
	err := runRuntimeSource(t, `fn main() {
  val items = ["first", "second"]
  val [first, second] = items
  if first != "first" { throw "first binding failed" }
  if second != "second" { throw "second binding failed" }
}`)
	if err != nil {
		t.Fatalf("array destructuring failed: %v", err)
	}
}

func TestDestructuringAliasesDefaultsAndRest(t *testing.T) {
	err := runRuntimeSource(t, `fn main() {
  val user = { name: "Ada", role: "admin", active: true }
  val { name: displayName, missing = "fallback", ...extra } = user
  if displayName != "Ada" { throw "alias binding failed" }
  if missing != "fallback" { throw "object default failed" }
  if extra.active != true { throw "object rest failed" }
  val [first, , third, ...tail] = [1, 2, 3, 4, 5]
  if first != 1 { throw "first array binding failed" }
  if third != 3 { throw "array hole failed" }
  if tail.length != 2 { throw "array rest failed" }
  val [fallback = 9] = []
  if fallback != 9 { throw "array default failed" }
}`)
	if err != nil {
		t.Fatalf("destructuring edge cases failed: %v", err)
	}
}

func TestDestructuringFunctionParameters(t *testing.T) {
	err := runRuntimeSource(t, `fn identity({ name }, [first, second]) {
  name + first + second
}
fn main() {
  val result = identity({ name: "Ada" }, ["-", "Lovelace"])
  if result != "Ada-Lovelace" { throw "parameter destructuring failed" }
}`)
	if err != nil {
		t.Fatalf("destructured function parameters failed: %v", err)
	}
}

func TestMutableDestructuredVarCanBeReassigned(t *testing.T) {
	err := runRuntimeSource(t, `fn main() {
  var { name } = { name: "Ada" }
  name = "Grace"
  if name != "Grace" { throw "mutable binding failed" }
}`)
	if err != nil {
		t.Fatalf("mutable destructured binding failed: %v", err)
	}
}

func TestDestructuredValIsImmutable(t *testing.T) {
	err := runRuntimeSource(t, `fn main() {
  val { name } = { name: "Ada" }
  name = "Grace"
}`)
	if err == nil {
		t.Fatal("expected reassignment of destructured val to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "const") && !strings.Contains(strings.ToLower(err.Error()), "immutable") {
		t.Fatalf("expected immutable binding error, got: %v", err)
	}
}

func TestDestructuringRejectsWrongSourceType(t *testing.T) {
	err := runRuntimeSource(t, `fn main() {
  val [first, second] = 12
}`)
	if err == nil || !strings.Contains(err.Error(), "cannot destructure array from number") {
		t.Fatalf("expected array destructuring type error, got: %v", err)
	}
}
