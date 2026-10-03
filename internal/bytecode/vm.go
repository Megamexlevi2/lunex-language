package bytecode

import (
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/compiler"
	"lunex/internal/runtime"
	"lunex/internal/std"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

func RunObject(data []byte, ntlLoader func(string) (string, bool), pkgLoader func(string) (string, bool)) error {
	chunk, err := DecodeObject(data)
	if err != nil {
		return fmt.Errorf("cannot load object: %w", err)
	}
	c := newRuntimeCompiler(ntlLoader, pkgLoader)
	if chunk.AST != nil {
		if chunk.Optimized {
			return c.RunOptimizedAST(chunk.AST, chunk.SourceFile)
		}
		return c.RunAST(chunk.AST, chunk.SourceFile, "")
	}
	return c.RunSource(chunk.SourceText, chunk.SourceFile)
}

func RunNAX(data []byte, ntlLoader func(string) (string, bool), pkgLoader func(string) (string, bool)) error {
	arch, err := decodeNAX(data)
	if err != nil {
		return fmt.Errorf("cannot load archive: %w", err)
	}
	if len(arch.Entries) == 0 {
		return fmt.Errorf("entry not found in archive: <empty>")
	}

	if len(arch.Entries) == 1 {
		c := newFastRuntimeCompiler(ntlLoader, pkgLoader)
		return RunNAXEntryWithCompiler(arch.Entries[0], c)
	}

	archiveSources, archiveEntries := prepareArchiveEntries(arch.Entries)
	archiveASTLoader, archiveSourceLoader := makeArchiveLoaders(archiveSources, archiveEntries)

	combinedLoader := archiveSourceLoader
	if ntlLoader != nil {
		combinedLoader = func(name string) (string, bool) {
			if src, ok := archiveSourceLoader(name); ok {
				return src, true
			}
			return ntlLoader(name)
		}
	}
	c := newFastRuntimeCompiler(combinedLoader, pkgLoader)
	c.Interpreter().SetASTLoader(archiveASTLoader)

	idx := int(arch.MainIndex)
	if idx < 0 || idx >= len(arch.Entries) {
		idx = 0
	}
	return RunNAXEntryWithCompiler(arch.Entries[idx], c)
}

func prepareArchiveEntries(entries []NAXEntry) (map[string]string, map[string]NAXEntry) {
	sources := make(map[string]string)
	compiled := make(map[string]NAXEntry)
	for _, e := range entries {
		key := normalizeArchiveKey(e.Name)
		if e.Kind == NAXEntrySource {
			sources[key] = string(e.Data)
			sources[normalizeArchiveKey(strings.TrimSuffix(e.Name, filepath.Ext(e.Name)))] = string(e.Data)
			continue
		}
		if IsNAXCompiledEntry(e) {
			base := normalizeArchiveKey(strings.TrimSuffix(e.Name, filepath.Ext(e.Name)))
			if base != "" {
				compiled[base] = e
				compiled[normalizeArchiveKey(base+".lx")] = e
			}
		}
	}
	return sources, compiled
}

func makeArchiveLoaders(sources map[string]string, entries map[string]NAXEntry) (func(string) (*ast.Node, string, bool), func(string) (string, bool)) {
	astCache := make(map[string]struct {
		tree *ast.Node
		path string
	})
	sourceCache := make(map[string]string)
	var cacheMu sync.RWMutex

	findEntry := func(name string) (NAXEntry, bool) {
		for _, key := range archiveLookupKeys(name) {
			if entry, ok := entries[key]; ok {
				return entry, true
			}
		}
		return NAXEntry{}, false
	}

	findSource := func(name string) (string, bool) {
		for _, key := range archiveLookupKeys(name) {
			if src, ok := sources[key]; ok {
				return src, true
			}
			cacheMu.RLock()
			src, ok := sourceCache[key]
			cacheMu.RUnlock()
			if ok {
				return src, true
			}
		}
		return "", false
	}

	astLoader := func(name string) (*ast.Node, string, bool) {
		for _, key := range archiveLookupKeys(name) {
			cacheMu.RLock()
			item, ok := astCache[key]
			cacheMu.RUnlock()
			if ok {
				return item.tree, item.path, true
			}
		}
		entry, ok := findEntry(name)
		if !ok {
			return nil, "", false
		}
		chunk, err := DecodeNAXEntry(entry)
		if err != nil || chunk.AST == nil {
			return nil, "", false
		}
		path := entry.SourceFile
		if path == "" {
			path = entry.Name
		}
		item := struct {
			tree *ast.Node
			path string
		}{chunk.AST, path}
		base := normalizeArchiveKey(strings.TrimSuffix(entry.Name, filepath.Ext(entry.Name)))
		cacheMu.Lock()
		for _, key := range archiveLookupKeys(name) {
			astCache[key] = item
		}
		astCache[base] = item
		astCache[normalizeArchiveKey(base+".lx")] = item
		if entry.SourceText != "" {
			sourceCache[base] = entry.SourceText
			sourceCache[normalizeArchiveKey(base+".lx")] = entry.SourceText
		}
		cacheMu.Unlock()
		return item.tree, item.path, true
	}

	sourceLoader := func(name string) (string, bool) {
		if src, ok := findSource(name); ok {
			return src, true
		}
		entry, ok := findEntry(name)
		if !ok || entry.SourceText == "" {
			return "", false
		}
		base := normalizeArchiveKey(strings.TrimSuffix(entry.Name, filepath.Ext(entry.Name)))
		cacheMu.Lock()
		sourceCache[base] = entry.SourceText
		sourceCache[normalizeArchiveKey(base+".lx")] = entry.SourceText
		cacheMu.Unlock()
		return entry.SourceText, true
	}

	return astLoader, sourceLoader
}

func archiveLookupKeys(name string) []string {
	key := normalizeArchiveKey(name)
	if key == "" {
		return nil
	}
	keys := []string{key}
	if !strings.HasSuffix(strings.ToLower(key), ".lx") {
		keys = append(keys, normalizeArchiveKey(key+".lx"))
	}
	return keys
}

func RunNAXFile(path string, ntlLoader func(string) (string, bool), pkgLoader func(string) (string, bool)) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	return RunNAX(data, ntlLoader, pkgLoader)
}

func RunObjectFile(path string, ntlLoader func(string) (string, bool), pkgLoader func(string) (string, bool)) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	return RunObject(data, ntlLoader, pkgLoader)
}

func LoadNAXAsModule(filePath string, c *compiler.Compiler) (*runtime.Value, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", filePath, err)
	}
	arch, err := decodeNAX(data)
	if err != nil {
		return nil, fmt.Errorf("cannot decode archive %s: %w", filePath, err)
	}
	if len(arch.Entries) == 0 {
		return nil, fmt.Errorf("archive %s has no entries", filePath)
	}
	archiveSources, archiveEntries := prepareArchiveEntries(arch.Entries)
	archiveASTLoader, archiveSourceLoader := makeArchiveLoaders(archiveSources, archiveEntries)
	prev := c.Interpreter().NTLLoader()
	combinedLoader := archiveSourceLoader
	if prev != nil {
		combinedLoader = func(name string) (string, bool) {
			if src, ok := archiveSourceLoader(name); ok {
				return src, true
			}
			return prev(name)
		}
	}
	c.Interpreter().SetNTLLoader(combinedLoader)
	prevAST := c.Interpreter().ASTLoader()
	c.Interpreter().SetASTLoader(archiveASTLoader)
	defer func() {
		c.Interpreter().SetNTLLoader(prev)
		c.Interpreter().SetASTLoader(prevAST)
	}()
	idx := int(arch.MainIndex)
	if idx < 0 || idx >= len(arch.Entries) {
		idx = 0
	}
	mainEntry := arch.Entries[idx]
	moduleFilename := filepath.Join(filepath.Dir(filePath), mainEntry.Name)
	chunk, err := DecodeNAXEntry(mainEntry)
	if err != nil {
		return nil, fmt.Errorf("cannot decode main entry in %s: %w", filePath, err)
	}
	if chunk.AST != nil {
		return c.Interpreter().ExecASTAsModule(chunk.AST, moduleFilename)
	}
	return c.RunSourceAsModule(chunk.SourceText, moduleFilename)
}

func newFastRuntimeCompiler(ntlLoader func(string) (string, bool), pkgLoader func(string) (string, bool)) *compiler.Compiler {
	c := compiler.New(compiler.DefaultOptions)
	std.RegisterLazy(c)
	if ntlLoader != nil {
		c.Interpreter().SetNTLLoader(ntlLoader)
	}
	if pkgLoader != nil {
		c.Interpreter().SetPkgLoader(pkgLoader)
	}

	c.Interpreter().SetNaxLoader(func(absPath string) (*runtime.Value, error) {
		ext := strings.ToLower(filepath.Ext(absPath))
		switch ext {
		case ".nax":
			return LoadNAXAsModule(absPath, c)
		default:
			return nil, fmt.Errorf("unsupported binary module extension: %s", ext)
		}
	})
	return c
}

func newRuntimeCompiler(ntlLoader func(string) (string, bool), pkgLoader func(string) (string, bool)) *compiler.Compiler {
	c := compiler.New(compiler.DefaultOptions)
	std.RegisterAll(c)
	if ntlLoader != nil {
		c.Interpreter().SetNTLLoader(ntlLoader)
	}
	if pkgLoader != nil {
		c.Interpreter().SetPkgLoader(pkgLoader)
	}

	c.Interpreter().SetNaxLoader(func(absPath string) (*runtime.Value, error) {
		ext := strings.ToLower(filepath.Ext(absPath))
		switch ext {
		case ".nax":
			return LoadNAXAsModule(absPath, c)
		default:
			return nil, fmt.Errorf("unsupported binary module extension: %s", ext)
		}
	})
	return c
}

func normalizeArchiveKey(name string) string {
	key := strings.TrimSpace(name)
	key = strings.ReplaceAll(key, "\\", "/")
	key = strings.TrimPrefix(key, "./")
	key = strings.TrimPrefix(key, "/")
	key = path.Clean(key)
	if key == "." {
		return ""
	}
	return key
}

func BuildNCFile(sourcePath string, outputPath string) error {
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", sourcePath, err)
	}
	srcText := string(source)

	absSource, _ := filepath.Abs(sourcePath)

	c := newRuntimeCompiler(nil, nil)
	result := c.CompileSource(srcText, absSource)
	if !result.Success {
		var msgs []string
		for _, e := range result.Errors {
			msgs = append(msgs, e.Message)
		}
		return fmt.Errorf("compile error: %s", strings.Join(msgs, "; "))
	}

	chunk := &Chunk{
		Name:       filepath.Base(sourcePath),
		SourceFile: absSource,
		SourceText: srcText,
	}

	nc, err := EncodeObject(chunk)
	if err != nil {
		return fmt.Errorf("encode error: %w", err)
	}

	return os.WriteFile(outputPath, nc, 0644)
}

func RunNAXEntryWithCompiler(entry NAXEntry, c *compiler.Compiler) error {
	if entry.Kind == NAXEntryArchive {
		return RunNAX(entry.Data, c.Interpreter().NTLLoader(), c.Interpreter().NTLLoader())
	}
	chunk, err := DecodeNAXEntry(entry)
	if err != nil {
		return fmt.Errorf("cannot load NAX entry %q: %w", entry.Name, err)
	}
	if chunk.AST != nil {
		return c.RunOptimizedAST(chunk.AST, chunk.SourceFile)
	}
	return c.RunSource(chunk.SourceText, chunk.SourceFile)
}

func RunObjectWithCompiler(data []byte, c *compiler.Compiler) error {
	chunk, err := DecodeObject(data)
	if err != nil {
		return fmt.Errorf("cannot load object: %w", err)
	}
	if chunk.AST != nil {
		if chunk.Optimized {
			return c.RunOptimizedAST(chunk.AST, chunk.SourceFile)
		}
		return c.RunAST(chunk.AST, chunk.SourceFile, "")
	}
	return c.RunSource(chunk.SourceText, chunk.SourceFile)
}
