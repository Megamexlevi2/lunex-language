package bytecode

import "lunex/internal/ast"

type ExportedChunk struct {
	Name       string
	SourceFile string
	SourceText string

	NTZOpcodes []byte
}

func EncodeExported(e *ExportedChunk) ([]byte, error) {
	chunk := &Chunk{
		Name:       e.Name,
		SourceFile: e.SourceFile,
		SourceText: e.SourceText,
	}

	return encodeNCWithNTZ(chunk, nil)
}

func EncodeExportedWithAST(e *ExportedChunk, tree *ast.Node) ([]byte, error) {
	return EncodeExportedWithASTSource(e, tree)
}

func EncodeExportedWithASTSource(e *ExportedChunk, _ *ast.Node) ([]byte, error) {
	return EncodeExported(e)
}
