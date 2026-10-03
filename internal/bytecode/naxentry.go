package bytecode

import (
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/formatter"
)

const (
	NAXEntrySource         byte = 0x01
	NAXEntryCompiled       byte = 0x02
	NAXEntryCompiledSource byte = 0x03
	NAXEntryArchive        byte = 0x04
)

func NewNAXCompiledEntry(name, sourceFile, sourceText string, tree *ast.Node, includeSource bool) (NAXEntry, error) {
	if tree == nil {
		return NAXEntry{}, fmt.Errorf("cannot encode NAX entry without a compiled AST")
	}
	data, err := encodeAST(tree)
	if err != nil {
		return NAXEntry{}, fmt.Errorf("encode compiled AST: %w", err)
	}
	kind := NAXEntryCompiled
	if includeSource {
		kind = NAXEntryCompiledSource
	}
	entry := NAXEntry{
		Name:       name,
		Kind:       kind,
		SourceFile: name,
		Data:       data,
	}
	if includeSource {
		entry.SourceText = sourceText
	}
	return entry, nil
}

func IsNAXCompiledEntry(entry NAXEntry) bool {
	return entry.Kind == NAXEntryCompiled || entry.Kind == NAXEntryCompiledSource
}

func DecodeNAXEntry(entry NAXEntry) (*Chunk, error) {
	switch entry.Kind {
	case NAXEntryCompiled, NAXEntryCompiledSource:
		tree, err := decodeAST(entry.Data)
		if err != nil {
			return nil, fmt.Errorf("invalid compiled NAX entry %q: %w", entry.Name, err)
		}
		return &Chunk{
			Name:       entry.Name,
			SourceFile: entry.SourceFile,
			SourceText: entry.SourceText,
			AST:        tree,
			Optimized:  true,
		}, nil
	case NAXEntrySource:
		return &Chunk{
			Name:       entry.Name,
			SourceFile: entry.SourceFile,
			SourceText: string(entry.Data),
		}, nil
	default:
		return nil, fmt.Errorf("entry %q is not a Lunex compiled module", entry.Name)
	}
}

func RecoverNAXEntry(entry NAXEntry) (string, error) {
	if !IsNAXCompiledEntry(entry) {
		if entry.Kind == NAXEntrySource {
			return string(entry.Data), nil
		}
		return "", fmt.Errorf("entry %q does not contain recoverable Lunex code", entry.Name)
	}
	if entry.SourceText != "" {
		return entry.SourceText, nil
	}
	chunk, err := DecodeNAXEntry(entry)
	if err != nil {
		return "", err
	}
	if chunk.AST == nil {
		return "", fmt.Errorf("entry %q does not contain a recoverable Lunex program", entry.Name)
	}
	return formatter.Format(chunk.AST), nil
}
