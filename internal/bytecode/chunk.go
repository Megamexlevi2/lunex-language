package bytecode

import (
	"lunex/internal/ast"
	"strings"
)

type Chunk struct {
	Name       string
	SourceFile string
	SourceText string
	AST        *ast.Node
	Optimized  bool
	SubChunks  []*Chunk
}

func NewChunk(name string) *Chunk {
	return &Chunk{Name: name}
}

func (c *Chunk) Disassemble() string {
	var sb strings.Builder

	sb.WriteString(c.SourceText)
	sb.WriteString("\n")

	return sb.String()
}

func indent(text, pad string) string {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = pad + lines[i]
	}
	return strings.Join(lines, "\n")
}
