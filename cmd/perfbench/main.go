package main

import (
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/jit"
	"lunex/internal/lexer"
	"lunex/internal/parser"
	"lunex/internal/resolver"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var outputPattern = regexp.MustCompile(`io\.log\(\s*([A-Za-z_][A-Za-z0-9_]*)\s*\)`)

type caseData struct {
	name      string
	prog      *jit.Program
	setup     map[int]float64
	outputRef int
}

func walk(n *ast.Node, fn func(*ast.Node)) {
	if n == nil {
		return
	}
	fn(n)
	for _, child := range []*ast.Node{n.Body, n.Init, n.Test, n.Alternate, n.Consequent, n.Left, n.Right, n.Object, n.Callee, n.Arg, n.Expr, n.Stmt, n.Subject, n.Count} {
		walk(child, fn)
	}
	for _, child := range n.Body_ {
		walk(child, fn)
	}
	for _, child := range n.Args {
		walk(child, fn)
	}
}

func findMain(n *ast.Node) *ast.Node {
	var out *ast.Node
	walk(n, func(node *ast.Node) {
		if out == nil && node.Type == ast.FnDecl && node.Name == "main" {
			out = node.Body
		}
	})
	return out
}

func parseInitialValue(n *ast.Node) (float64, bool) {
	if n == nil || n.Type != ast.NumberLit {
		return 0, false
	}
	s, ok := n.Value.(string)
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func prepare(path string) (caseData, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return caseData{}, err
	}
	tokens, err := lexer.Tokenize(string(src), path)
	if err != nil {
		return caseData{}, err
	}
	program, err := parser.New(tokens, path).Parse()
	if err != nil {
		return caseData{}, err
	}
	resolver.Resolve(program)
	mainBody := findMain(program)
	if mainBody == nil {
		return caseData{}, fmt.Errorf("main function not found")
	}
	var loop *ast.Node
	walk(mainBody, func(node *ast.Node) {
		if loop == nil && (node.Type == ast.WhileStmt || node.Type == ast.ForStmt || node.Type == ast.RepeatStmt || node.Type == ast.LoopStmt) {
			loop = node
		}
	})
	if loop == nil {
		return caseData{}, fmt.Errorf("loop not found")
	}
	compiled, err := jit.CompileLoop(loop, 2)
	if err != nil {
		return caseData{}, err
	}
	setup := make(map[int]float64)
	outputSlot := -1
	if match := outputPattern.FindStringSubmatch(string(src)); len(match) == 2 {
		outputSlot = resolver.SlotIndex(mainBody.ScopeInfo, match[1])
	}
	if mainBody.Type == ast.Block {
		for _, stmt := range mainBody.Body_ {
			if stmt == nil || stmt.Type != ast.VarDecl || stmt.Init == nil {
				continue
			}
			slot := resolver.SlotIndex(mainBody.ScopeInfo, stmt.Name)
			value, ok := parseInitialValue(stmt.Init)
			if ok && slot >= 0 {
				setup[slot] = value
			}
		}
	}
	outputRef := -1
	for i, ref := range compiled.Refs {
		if ref.Hops == 0 && ref.Slot == outputSlot {
			outputRef = i
			break
		}
	}
	if outputRef < 0 {
		return caseData{}, fmt.Errorf("benchmark output is not a direct numeric slot")
	}
	return caseData{name: filepath.Base(path), prog: compiled, setup: setup, outputRef: outputRef}, nil
}

func splitNonEmptyLines(s string) []string {
	lines := make([]string, 0)
	start := 0
	for start <= len(s) {
		end := start
		for end < len(s) && s[end] != '\n' {
			end++
		}
		line := s[start:end]
		if line != "" {
			lines = append(lines, line)
		}
		if end == len(s) {
			break
		}
		start = end + 1
	}
	return lines
}

func main() {
	root := "benchmarks"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	paths, err := filepath.Glob(filepath.Join(root, "native_*.lx"))
	if err != nil || len(paths) < 20 {
		panic("benchmark set is incomplete")
	}
	cases := make([]caseData, 0, len(paths))
	for _, path := range paths {
		c, err := prepare(path)
		if err != nil {
			panic(path + ": " + err.Error())
		}
		cases = append(cases, c)
	}

	for _, c := range cases {
		ctx := c.prog.NewContext()
		ctx.Budget = -1
		for i, ref := range c.prog.Refs {
			if ref.Hops == 0 {
				if value, ok := c.setup[ref.Slot]; ok {
					ctx.Slots[i] = value
				}
			}
		}
		if err := c.prog.Run(ctx); err != nil {
			panic(c.name + ": " + err.Error())
		}
		if ctx.Status != 0 {
			panic(c.name + ": native execution did not finish normally")
		}
	}

	const python = "python3"
	const rounds = 3
	fmt.Println("benchmark,native_seconds,python_seconds,python_over_native,result")
	wins := 0
	for _, c := range cases {
		var nativeBest float64 = 1e100
		var pythonBest float64 = 1e100
		var pythonResult float64
		for i := 0; i < rounds; i++ {
			ctx := c.prog.NewContext()
			ctx.Budget = -1
			for j, ref := range c.prog.Refs {
				if ref.Hops == 0 {
					if value, ok := c.setup[ref.Slot]; ok {
						ctx.Slots[j] = value
					}
				}
			}
			started := time.Now()
			if err := c.prog.Run(ctx); err != nil {
				panic(c.name + ": " + err.Error())
			}
			if ctx.Status != 0 {
				panic(fmt.Sprintf("%s: native execution status %d", c.name, ctx.Status))
			}
			nativeTime := time.Since(started).Seconds()
			if nativeTime < nativeBest {
				nativeBest = nativeTime
			}

			cmd := exec.Command(python, filepath.Join(root, "python", strings.TrimSuffix(c.name, ".lx")+".py"))
			output, err := cmd.Output()
			if err != nil {
				panic(c.name + ": Python benchmark failed")
			}
			lines := splitNonEmptyLines(string(output))
			if len(lines) < 2 {
				panic(c.name + ": Python benchmark output is incomplete")
			}
			result, err := strconv.ParseFloat(lines[len(lines)-2], 64)
			if err != nil {
				panic(c.name + ": invalid Python result")
			}
			seconds, err := strconv.ParseFloat(lines[len(lines)-1], 64)
			if err != nil {
				panic(c.name + ": invalid Python timing")
			}
			pythonResult = result
			if seconds < pythonBest {
				pythonBest = seconds
			}
			if !math.IsNaN(result) && !math.IsNaN(ctx.Slots[c.outputRef]) && !math.IsInf(result, 0) && !math.IsInf(ctx.Slots[c.outputRef], 0) {
				if !math.IsNaN(result) && math.Abs(result-ctx.Slots[c.outputRef]) > 1e-9*math.Max(1, math.Abs(result)) {
					panic(fmt.Sprintf("%s: result mismatch native=%v python=%v", c.name, ctx.Slots[c.outputRef], result))
				}
			} else if result != ctx.Slots[c.outputRef] {
				panic(fmt.Sprintf("%s: result mismatch native=%v python=%v", c.name, ctx.Slots[c.outputRef], result))
			}
		}
		speedup := pythonBest / nativeBest
		if speedup > 1 {
			wins++
		}
		fmt.Printf("%s,%.9f,%.9f,%.2fx,%v\n", c.name, nativeBest, pythonBest, speedup, pythonResult)
		_ = c.prog.Close()
	}
	fmt.Printf("native_faster_cases=%d/%d\n", wins, len(cases))

}
