package runtime

import (
	"lunex/internal/ast"
	"lunex/internal/lexer"
	"lunex/internal/parser"
	"lunex/internal/resolver"
	"testing"
)

func parseWatchTest(t *testing.T, source string) *ast.Node {
	t.Helper()
	tokens, err := lexer.Tokenize(source, "watch_test.lx")
	if err != nil {
		t.Fatal(err)
	}
	tree, err := parser.Parse(tokens, "watch_test.lx")
	if err != nil {
		t.Fatal(err)
	}
	resolver.Resolve(tree)
	return tree
}

func TestWatchVariable(t *testing.T) {
	source := `
var counter = 0
var hits = 0

fn main() {
  watch counter {
    hits = hits + 1
  }

  counter = 10
  counter = 20
  counter = 20
}
`

	interp := NewInterpreter()
	tree := parseWatchTest(t, source)
	if _, err := interp.Exec(tree); err != nil {
		t.Fatal(err)
	}
	if err := interp.CallMain(); err != nil {
		t.Fatal(err)
	}

	hits, ok := interp.topEnv.Get("hits")
	if !ok || hits == nil || hits.NumVal != 2 {
		t.Fatalf("expected 2 watcher invocations, got %v", hits)
	}
}

func TestWatchMember(t *testing.T) {
	source := `
val user = struct {
  name = "Alice"
}
var hits = 0

fn main() {
  watch user.name {
    hits = hits + 1
  }

  user.name = "Bob"
  user.name = "Bob"
  user.name = "Carol"
}
`

	interp := NewInterpreter()
	tree := parseWatchTest(t, source)
	if _, err := interp.Exec(tree); err != nil {
		t.Fatal(err)
	}
	if err := interp.CallMain(); err != nil {
		t.Fatal(err)
	}

	hits, ok := interp.topEnv.Get("hits")
	if !ok || hits == nil || hits.NumVal != 2 {
		t.Fatalf("expected 2 watcher invocations, got %v", hits)
	}
}
