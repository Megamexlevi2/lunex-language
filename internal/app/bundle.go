package lunex

import (
	"fmt"
	"lunex/internal/ast"
	"lunex/internal/bytecode"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

func buildNAXBundle(absInput, inputFile, outputFile, sourceText string, tree *ast.Node, includeSource bool) error {
	rootDir := filepath.Dir(absInput)

	mainRel, err := filepath.Rel(rootDir, absInput)
	if err != nil {
		return fmt.Errorf("error: cannot resolve bundle root for %s: %w", inputFile, err)
	}
	mainRel = filepath.ToSlash(mainRel)
	if strings.HasPrefix(mainRel, "..") {
		return fmt.Errorf("error: entry file %s must be inside the project root %s", inputFile, rootDir)
	}

	deps, err := collectBundleDependencies(rootDir, absInput, tree, includeSource)
	if err != nil {
		return err
	}

	mainEntry, err := bytecode.NewNAXCompiledEntry(mainRel, absInput, sourceText, tree, includeSource)
	if err != nil {
		return fmt.Errorf("error encoding main module: %w", err)
	}

	entries := make([]bytecode.NAXEntry, 0, len(deps)+1)
	entries = append(entries, mainEntry)

	keys := make([]string, 0, len(deps))
	for k := range deps {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		entries = append(entries, deps[k])
	}

	arch := &bytecode.NAXArchive{
		Entries:   entries,
		MainIndex: 0,
	}
	return bytecode.PackNAXArchive(arch, outputFile)
}

func collectBundleDependencies(rootDir, absInput string, tree *ast.Node, includeSource bool) (map[string]bytecode.NAXEntry, error) {
	deps := make(map[string]bytecode.NAXEntry)
	visited := map[string]bool{
		absInput: true,
	}

	var collect func(currentAbs string, node *ast.Node) error
	collect = func(currentAbs string, node *ast.Node) error {
		imports, err := findForceLocalImports(node)
		if err != nil {
			return err
		}

		for _, spec := range imports {
			importAbs, importRel, err := resolveBundleImport(rootDir, currentAbs, spec)
			if err != nil {
				return err
			}
			if visited[importAbs] {
				continue
			}
			visited[importAbs] = true

			data, err := os.ReadFile(importAbs)
			if err != nil {
				return fmt.Errorf("error reading local import %s: %w", importRel, err)
			}

			c := newCompiler()
			result := c.CompileSource(string(data), importAbs)
			if !result.Success {
				var msgs []string
				for _, e := range result.Errors {
					msgs = append(msgs, e.Message)
				}
				return fmt.Errorf("compile error for %s: %s", importRel, strings.Join(msgs, "; "))
			}

			entry, err := bytecode.NewNAXCompiledEntry(importRel, importAbs, string(data), result.AST, includeSource)
			if err != nil {
				return fmt.Errorf("error encoding local import %s: %w", importRel, err)
			}
			deps[importRel] = entry

			if err := collect(importAbs, result.AST); err != nil {
				return err
			}
		}
		return nil
	}

	if err := collect(absInput, tree); err != nil {
		return nil, err
	}
	return deps, nil
}

func findForceLocalImports(tree *ast.Node) ([]string, error) {
	if tree == nil {
		return nil, nil
	}

	seen := map[uintptr]bool{}
	imports := make([]string, 0, 8)

	var walk func(v reflect.Value) error
	walk = func(v reflect.Value) error {
		if !v.IsValid() {
			return nil
		}
		for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return nil
			}
			if v.Kind() == reflect.Pointer && v.Type() == reflect.TypeOf(&ast.Node{}) {
				ptr := v.Pointer()
				if seen[ptr] {
					return nil
				}
				seen[ptr] = true
				n := v.Interface().(*ast.Node)
				if n != nil && n.Type == ast.AtImportExpr && forceLocalImport(n) {
					spec := strings.TrimSpace(n.Source)
					if spec != "" {
						imports = append(imports, spec)
					}
				}
			}
			v = v.Elem()
		}

		switch v.Kind() {
		case reflect.Struct:
			if v.CanAddr() {
				if n, ok := v.Addr().Interface().(*ast.Node); ok && n != nil && n.Type == ast.AtImportExpr && forceLocalImport(n) {
					spec := strings.TrimSpace(n.Source)
					if spec != "" {
						imports = append(imports, spec)
					}
				}
			}
			for i := 0; i < v.NumField(); i++ {
				if err := walk(v.Field(i)); err != nil {
					return err
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				if err := walk(v.Index(i)); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := walk(reflect.ValueOf(tree)); err != nil {
		return nil, err
	}

	return uniqueStrings(imports), nil
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func resolveBundleImport(rootDir, currentAbs, spec string) (string, string, error) {
	candidates := []string{spec}
	if !strings.HasSuffix(strings.ToLower(spec), ".lx") {
		candidates = append(candidates, spec+".lx")
	}

	tryPaths := make([]string, 0, len(candidates)*3)
	currentDir := filepath.Dir(currentAbs)
	for _, cand := range candidates {
		tryPaths = append(tryPaths,
			filepath.Join(currentDir, cand),
			filepath.Join(rootDir, cand),
			cand,
		)
	}

	for _, p := range tryPaths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			continue
		}
		rel, err := filepath.Rel(rootDir, abs)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "..") {
			continue
		}
		return abs, rel, nil
	}

	return "", "", fmt.Errorf("error: local import %q not found under project root %s", spec, rootDir)
}

func forceLocalImport(node *ast.Node) bool {
	if node == nil {
		return false
	}
	if s, ok := node.Prop.(string); ok {
		return strings.EqualFold(s, "force-local") || strings.EqualFold(s, "fimport")
	}
	return false
}
