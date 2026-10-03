package lunex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"lunex/internal/adaptor"
	"lunex/internal/ast"
	"lunex/internal/buildfile"
	"lunex/internal/bytecode"
	"lunex/internal/checker"
	"lunex/internal/compiler"
	dbg "lunex/internal/debug"
	"lunex/internal/errfmt"
	"lunex/internal/firstrun"
	"lunex/internal/manifest"
	"lunex/internal/meta"
	"lunex/internal/pkg"
	"lunex/internal/runtime"
	"lunex/internal/std"
	"os"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"
)

var noCache bool

func init() {
	meta.SetVersionData(_versionJSON)
	tuneGC()
}

func tuneGC() {
	if adaptor.AndroidTuningApplied() {
		goruntime.LockOSThread()
		goruntime.UnlockOSThread()
		return
	}
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(50)
	}
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(200 << 20)
	}
	goruntime.LockOSThread()
	goruntime.UnlockOSThread()
}

func safeRecover() {
	if r := recover(); r != nil {
		msg := fmt.Sprintf("%v", r)
		switch {
		case strings.Contains(msg, "stack overflow"), strings.Contains(msg, "goroutine stack exceeds"):
			fmt.Fprintln(os.Stderr, "[31merror[RecursionError][0m: maximum call depth exceeded (infinite recursion detected)")
			fmt.Fprintln(os.Stderr, "  hint: check for a function that calls itself without a base case")
		case strings.Contains(msg, "nil pointer"), strings.Contains(msg, "invalid memory"):
			fmt.Fprintln(os.Stderr, "[31merror[RuntimeError][0m: internal null access — this is likely a Lunex bug, please report it")
		case strings.Contains(msg, "out of memory"), strings.Contains(msg, "runtime: out of memory"):
			fmt.Fprintln(os.Stderr, "[31merror[MemoryError][0m: program ran out of memory")
		default:
			fmt.Fprintf(os.Stderr, "\x1b[31merror[RuntimeError]\x1b[0m: %s\n", msg)
		}
		os.Exit(1)
	}
}

func Run() {
	defer safeRecover()
	meta.Seal()
	firstrun.Check(meta.Version())

	args := os.Args[1:]
	std.SetFFIEnabled(false)
	args = consumeFFIOption(args)

	if len(args) > 0 && args[0] == "*debug" {
		os.Setenv("NTL_DEBUG", "1")
		dbg.Enable()
		args = args[1:]
	}

	{
		filtered := args[:0]
		for _, a := range args {
			switch a {
			case "--debug", "-d":
				os.Setenv("NTL_DEBUG", "1")
				dbg.Enable()
			case "--verbose", "-V":
				dbg.EnableVerbose()
			case "--no-cache":
				noCache = true
			case "--no-jit":
				runtime.SetNativeEnabled(false)
			case "--jit":
				runtime.SetNativeEnabled(true)
			default:
				filtered = append(filtered, a)
			}
		}
		args = filtered
	}

	if len(args) == 0 {
		printHelp()
		return
	}

	cmd := args[0]
	switch cmd {
	case "run":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: lunex run <file.lx|file.nax> [--emit ast|ir]")
			os.Exit(1)
		}
		runFile(args[1], args[2:])

	case "-e", "execute":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: lunex -e \"<code>\"")
			os.Exit(1)
		}
		runString(args[1])

	case "version", "--version", "-v":
		meta.PrintVersion()

	case "help", "--help", "-h":
		printHelp()

	case "--help-extras":
		fmt.Println("--jit")
		fmt.Println("--no-jit")

	case "start":
		runStart(args[1:])

	case "init":
		if len(args) > 2 {
			if tmpl, ok := knownTemplates[args[1]]; ok {
				runInitTemplate(tmpl, args[2])
				return
			}
			fmt.Fprintf(os.Stderr, "error: unknown template '%s'\n", args[1])
			fmt.Fprintf(os.Stderr, "       available templates: %s\n", strings.Join(templateNames(), ", "))
			os.Exit(1)
		}

		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if name == "" {
			name = filepath.Base(cwd)
		}
		projectDir := filepath.Join(cwd, name)
		if st, err := os.Stat(projectDir); err == nil && !st.IsDir() {
			fmt.Fprintf(os.Stderr, "error: %s exists and is not a directory\n", projectDir)
			os.Exit(1)
		}
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if err := pkg.InitManifest(projectDir, name); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
		}

		srcDir := filepath.Join(projectDir, "src")
		if err := os.MkdirAll(srcDir, 0755); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

		mainPath := filepath.Join(projectDir, "main.lx")
		if _, err := os.Stat(mainPath); os.IsNotExist(err) {
			mainCode := `val io = @import("std.io")
val math = @fimport("./src/math.lx")

fn main() {
  io.log("Hello from ` + name + `!")
  io.log("2 + 3 =", math.add(2, 3))
}
`
			if err := os.WriteFile(mainPath, []byte(mainCode), 0644); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
		}

		mathPath := filepath.Join(srcDir, "math.lx")
		if _, err := os.Stat(mathPath); os.IsNotExist(err) {
			mathCode := `
fn add(a, b) {
  a + b
}

fn sub(a, b) {
  a - b
}

fn mul(a, b) {
  a * b
}
`
			if err := os.WriteFile(mathPath, []byte(mathCode), 0644); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
		}

		gitignorePath := filepath.Join(projectDir, ".gitignore")
		if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
			gitignoreContent := "dist/\n.lunex/\n*.nax\n"
			_ = os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644)
		}

		fmt.Printf("\n  ✓ Created project: %s\n\n", projectDir)
		fmt.Printf("  Files created:\n")
		fmt.Printf("    %-30s  project manifest\n", name+"/lunex.toml")
		fmt.Printf("    %-30s  entry point\n", name+"/main.lx")
		fmt.Printf("    %-30s  example local module\n", name+"/src/math.lx")
		fmt.Printf("    %-30s  git ignore rules\n", name+"/.gitignore")
		fmt.Printf("\n  Run your project:\n")
		fmt.Printf("    cd %s\n", name)
		fmt.Printf("    lunex start\n")
		fmt.Printf("\n  Templates:\n")
		fmt.Printf("    lunex init http_server <name>              REST HTTP server\n")
		fmt.Printf("    lunex init database <name>                 SQLite-backed database project\n")
		fmt.Printf("    lunex init website <name>                  static website\n")
		fmt.Printf("\n  Import guide:\n")
		fmt.Printf("    @fimport(\"./src/math.lx\")   local .lx source file\n")
		fmt.Printf("    @fimport(\"./lib/mod.nax\")   local archive file\n")
		fmt.Printf("    @import(\"std.io\")            standard library module\n")
		fmt.Printf("    @import(\"my-pkg\")            installed library (declared in lunex.toml)\n\n")
		fmt.Printf("  Add a dependency:\n")
		fmt.Printf("    lunex add https://github.com/user/repo     add to lunex.toml and install\n")
		fmt.Printf("    lunex install                              install everything in lunex.toml\n\n")

	case "install", "i":
		runInstallCommand(args[1:])

	case "add":
		if len(args) == 1 {
			fmt.Fprintln(os.Stderr, "usage: lunex add <url>[@version]")
			os.Exit(1)
		}
		runAddCommand(args[1:])

	case "remove", "uninstall", "rm":
		if len(args) == 1 {
			fmt.Fprintln(os.Stderr, "usage: lunex remove <library> [more libraries...]")
			os.Exit(1)
		}
		for _, name := range args[1:] {
			if err := pkg.Remove(name); err != nil {
				fmt.Fprintf(os.Stderr, "error removing %s: %v\n", name, err)
				os.Exit(1)
			}
			fmt.Printf("removed %s\n", name)
		}
		os.Exit(0)

	case "update", "upgrade":
		mods := pkg.List()
		if len(args) > 1 {
			mods = nil
			for _, name := range args[1:] {
				for _, mod := range pkg.List() {
					if mod.Name == name {
						mods = append(mods, mod)
						break
					}
				}
			}
		}
		if len(mods) == 0 {
			fmt.Fprintln(os.Stderr, "no installed libraries found")
			os.Exit(1)
		}
		proj, _ := manifest.Load(".")
		for _, mod := range mods {
			var lib *manifest.Library
			if proj != nil {
				lib = proj.Libraries[mod.Name]
			}
			if lib == nil {
				fmt.Fprintf(os.Stderr, "skipping %s: no lunex.toml entry to re-resolve against\n", mod.Name)
				continue
			}
			if _, err := pkg.InstallLibrary(lib, pkg.InstallOptions{Global: mod.Global}); err != nil {
				fmt.Fprintf(os.Stderr, "error updating %s: %v\n", mod.Name, err)
				os.Exit(1)
			}
			fmt.Printf("updated %s\n", mod.Name)
		}
		os.Exit(0)

	case "list", "ls":
		mods := pkg.List()
		if len(mods) == 0 {
			fmt.Println("no libraries installed")
			os.Exit(0)
		}
		for _, mod := range mods {
			scope := "local"
			if mod.Global {
				scope = "global"
			}
			if mod.Version != "" {
				fmt.Printf("%s@%s  (%s)\n", mod.Name, mod.Version, scope)
			} else {
				fmt.Printf("%s  (%s)\n", mod.Name, scope)
			}
		}
		os.Exit(0)

	case "env":
		printEnv()
		os.Exit(0)

	case "link":
		linked, err := pkg.LinkProject()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		binDir := filepath.Join(pkg.GlobalRoot(), "bin")
		for _, cmd := range linked {
			fmt.Printf("linked command %q -> %s\n", cmd, filepath.Join(binDir, cmd))
		}
		fmt.Printf("make sure %s is on your PATH to run %s directly\n", binDir, strings.Join(linked, ", "))
		os.Exit(0)

	case "debug":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: lunex debug <file.lx>")
			os.Exit(1)
		}
		debugFile(args[1], args[2:])

	case "repl":
		runREPL()

	case "pack":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: lunex pack <file.lx|directory> [--source] [-o output.nax]")
			os.Exit(1)
		}
		parsePackCommand(args[1:])

	case "check":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: lunex check <file.lx>")
			os.Exit(1)
		}
		checkFile(args[1])

	case "see_errors", "see-errors", "errors":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: lunex see_errors <file.lx>")
			os.Exit(1)
		}
		seeErrors(args[1])

	case "cache":
		if len(args) > 1 && args[1] == "clear" {
			clearCache()
		} else {
			showCacheInfo()
		}

	case "memcache":
		if len(args) > 1 && args[1] == "clear" {
			adaptor.MemCacheClear()
			fmt.Println("in-memory bytecode cache cleared")
		} else {
			showMemCacheInfo()
		}

	case "platform":
		fmt.Print(adaptor.Info())

	case "runtimes":
		showRuntimes()

	case "bench":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: lunex bench <file.lx>")
			os.Exit(1)
		}
		runBench(args[1])

	case "set":
		if len(args) < 3 || args[1] != "cache" {
			fmt.Fprintln(os.Stderr, "usage: lunex set cache <dir>")
			fmt.Fprintln(os.Stderr, "       lunex set cache reset")
			os.Exit(1)
		}
		setCacheDir(args[2])

	case "unpack":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: lunex unpack <file.nax>")
			os.Exit(1)
		}
		unpackNAX(args[1])

	default:
		ext := strings.ToLower(filepath.Ext(cmd))
		if ext == ".lx" || ext == ".nax" {
			runFile(cmd, args[1:])
		} else if dbg.Enabled() {
			dbg.StepWarn("unknown command", cmd)
			dbg.Log("tip: run 'lunex help' to see all available commands")
		} else {
			fmt.Fprintf(os.Stderr, "unknown command: %s\nRun 'lunex help' for usage.\n", cmd)
			os.Exit(1)
		}
	}
}

func newCompiler() *compiler.Compiler {
	c := compiler.New(compiler.DefaultOptions)
	std.RegisterAll(c)
	c.Interpreter().SetNTLFileLoader(pkgFileLoader)
	return c
}

func moduleSourceFromPath(resolvedPath string) (string, bool) {
	switch strings.ToLower(filepath.Ext(resolvedPath)) {
	case ".lx":
		data, err := os.ReadFile(resolvedPath)
		if err != nil {
			return "", false
		}
		return string(data), true
	case ".nax":
		arch, err := bytecode.LoadNAX(resolvedPath)
		if err != nil || arch == nil || len(arch.Entries) == 0 {
			return "", false
		}
		idx := int(arch.MainIndex)
		if idx < 0 || idx >= len(arch.Entries) {
			idx = 0
		}
		src, err := bytecode.RecoverNAXEntry(arch.Entries[idx])
		if err != nil {
			return "", false
		}
		return src, true
	}
	return "", false
}

func pkgFileLoader(name string) (src, realPath string, ok bool) {
	resolvedPath, found := pkg.Resolve(name)
	if found {
		if s, ok2 := moduleSourceFromPath(resolvedPath); ok2 {
			return s, resolvedPath, true
		}
	}

	if !strings.HasPrefix(name, "./") && !strings.HasPrefix(name, "../") && !strings.HasPrefix(name, "/") {
		fmt.Fprintf(os.Stderr,
			"\x1b[33mhint:\x1b[0m library %q not found — add it to lunex.toml with:\x1b[0m\n  lunex add https://github.com/<owner>/%s\n  lunex install\n\n",
			name, name,
		)
	}
	return "", "", false
}

func pkgLoader(name string) (string, bool) {
	src, _, ok := pkgFileLoader(name)
	return src, ok
}

func runString(source string) {
	t0 := time.Now()
	dbg.VHeader("<eval>")
	dbg.Header("<eval>")

	dbg.VSection("compiling snippet")
	dbg.Step("compiling...", fmt.Sprintf("%d bytes", len(source)))
	dbg.VKV("source size", len(source))
	c := newCompiler()
	result := c.CompileSource(source, "<eval>")
	if !result.Success {
		dbg.VStep("compile failed")
		for _, e := range result.Errors {
			fmt.Fprint(os.Stderr, errfmt.Format(e))
		}
		os.Exit(1)
	}
	dbg.StepOK("done", "compiled", "")

	dbg.VSection("generating bytecode and running")
	dbg.Step("generating bytecode...", "")
	chunk := &bytecode.ExportedChunk{
		Name:       "<eval>",
		SourceFile: "<eval>",
		SourceText: source,
	}
	objectData, err := bytecode.EncodeExportedWithAST(chunk, result.AST)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	dbg.StepOK("done", "bytecode ready", fmt.Sprintf("%d bytes", len(objectData)))
	dbg.VKV("bytecode size", fmt.Sprintf("%d bytes", len(objectData)))
	dbg.VStep("running...")
	execBinary(objectData)
	dbg.VFooter(time.Since(t0))
	dbg.Footer(time.Since(t0))
}

type emitMode string

const (
	emitModeAST emitMode = "ast"
	emitModeIR  emitMode = "ir"
)

func parseRunOptions(extraArgs []string) (emitMode, []string, error) {
	var emit emitMode
	var scriptArgs []string
	for i := 0; i < len(extraArgs); i++ {
		arg := extraArgs[i]
		switch {
		case arg == "--":
			scriptArgs = append(scriptArgs, extraArgs[i+1:]...)
			i = len(extraArgs)
		case arg == "--emit":
			if i+1 >= len(extraArgs) {
				return "", nil, fmt.Errorf("error: --emit requires a value")
			}
			emit = emitMode(strings.ToLower(extraArgs[i+1]))
			i++
		case strings.HasPrefix(arg, "--emit="):
			emit = emitMode(strings.ToLower(strings.TrimPrefix(arg, "--emit=")))
		case strings.TrimSpace(arg) == "":
			continue
		default:
			scriptArgs = append(scriptArgs, extraArgs[i:]...)
			i = len(extraArgs)
		}
	}
	if emit != "" && emit != emitModeAST && emit != emitModeIR {
		return "", nil, fmt.Errorf("unsupported emit mode: %s", emit)
	}
	return emit, scriptArgs, nil
}

func emitAST(tree *ast.Node) error {
	data, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

type irNode struct {
	Type     string    `json:"type"`
	Name     string    `json:"name,omitempty"`
	Op       string    `json:"op,omitempty"`
	Value    any       `json:"value,omitempty"`
	Children []*irNode `json:"children,omitempty"`
}

func buildIRNode(n *ast.Node) *irNode {
	if n == nil {
		return nil
	}
	out := &irNode{
		Type:  string(n.Type),
		Name:  n.Name,
		Op:    n.Op,
		Value: n.Value,
	}
	for _, child := range nodeChildren(n) {
		if built := buildIRNode(child); built != nil {
			out.Children = append(out.Children, built)
		}
	}
	return out
}

func nodeChildren(n *ast.Node) []*ast.Node {
	seen := make(map[*ast.Node]struct{})
	out := make([]*ast.Node, 0, 16)
	var walk func(v any)
	walk = func(v any) {
		if v == nil {
			return
		}
		rv := reflectValue(v)
		if !rv.IsValid() {
			return
		}
		switch rv.Kind() {
		case reflect.Pointer:
			if rv.IsNil() {
				return
			}
			if node, ok := rv.Interface().(*ast.Node); ok {
				if _, exists := seen[node]; !exists {
					seen[node] = struct{}{}
					out = append(out, node)
				}
				return
			}
			walk(rv.Elem().Interface())
		case reflect.Interface:
			if rv.IsNil() {
				return
			}
			walk(rv.Elem().Interface())
		case reflect.Struct:
			for i := 0; i < rv.NumField(); i++ {
				walk(rv.Field(i).Interface())
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < rv.Len(); i++ {
				walk(rv.Index(i).Interface())
			}
		}
	}
	walk(*n)
	return out
}

func reflectValue(v any) (rv reflect.Value) {
	rv = reflect.ValueOf(v)
	return
}

func emitIR(tree *ast.Node) error {
	data, err := json.MarshalIndent(buildIRNode(tree), "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func emitSource(absPath string, mode emitMode) {
	source, err := os.ReadFile(absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading file: %v\n", err)
		os.Exit(1)
	}
	srcText := string(source)
	c := newCompiler()
	result := c.CompileSource(srcText, absPath)
	if !result.Success {
		for _, e := range result.Errors {
			fmt.Fprint(os.Stderr, errfmt.Format(e))
		}
		os.Exit(1)
	}
	switch mode {
	case emitModeAST:
		if err := emitAST(result.AST); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case emitModeIR:
		if err := emitIR(result.AST); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "error: unsupported emit mode")
		os.Exit(1)
	}
}

func runFile(filePath string, extraArgs []string) {
	emit, scriptArgs, err := parseRunOptions(extraArgs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".nax":
		if emit != "" {
			fmt.Fprintln(os.Stderr, "error: --emit is only supported for .lx sources")
			os.Exit(1)
		}
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		prevArgs := os.Args
		os.Args = append([]string{absPath}, scriptArgs...)
		defer func() { os.Args = prevArgs }()
		data, err := os.ReadFile(absPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		execBinary(data)
	default:
		if !strings.HasSuffix(filePath, ".lx") && ext == "" {
			filePath += ".lx"
		}
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error resolving path: %v\n", err)
			os.Exit(1)
		}
		if emit != "" {
			emitSource(absPath, emit)
			return
		}
		prevArgs := os.Args
		os.Args = append([]string{absPath}, scriptArgs...)
		defer func() { os.Args = prevArgs }()
		runNTLWithCache(absPath)
	}
}

func packSourceFile(absInput, inputFile, outputFile string, includeSource bool) error {
	source, err := os.ReadFile(absInput)
	if err != nil {
		return errfmt.New(errfmt.KindIO, "E0063", fmt.Sprintf("cannot read pack input `%s`: %v", inputFile, err), absInput, 1, 1, strings.Split(string(source), "\n"))
	}
	if err := validatePackSource(string(source), absInput, false); err != nil {
		return err
	}
	tree, err := validatePackCompilerGraph(absInput, string(source))
	if err != nil {
		return err
	}

	if outputFile == "" {
		baseName := strings.TrimSuffix(filepath.Base(inputFile), filepath.Ext(inputFile))
		outputFile = baseName + ".nax"
	}
	outputFile = normalizeNAXOutput(outputFile)
	if outDir := filepath.Dir(outputFile); outDir != "." && outDir != "" {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return errfmt.New(errfmt.KindIO, "E0063", fmt.Sprintf("cannot create output directory `%s`: %v", outDir, err), outDir, 1, 1, nil)
		}
	}

	if err := buildNAXBundle(absInput, inputFile, outputFile, string(source), tree, includeSource); err != nil {
		return newPackInternalError(absInput, err.Error())
	}
	fi, err := os.Stat(outputFile)
	if err != nil {
		return newPackInternalError(absInput, fmt.Sprintf("archive was emitted but could not be stat'ed: %v", err))
	}
	fmt.Printf("%s → %s (%s)\n", inputFile, outputFile, formatBytes(fi.Size()))
	fmt.Println("help: You can also use the --source flag to facilitate future code retrieval using NAX.")
	return nil
}

func packSourceDirectory(absDir, inputPath, outputFile string, includeSource bool) error {
	mainPath := filepath.Join(absDir, "main.lx")
	if _, err := os.Stat(mainPath); err != nil {
		return errfmt.New(errfmt.KindSyntax, "E0050", "pack directory requires `main.lx` at the project root", absDir, 1, 1, nil)
	}

	files := make([]string, 0, 32)
	if err := filepath.WalkDir(absDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".lx") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return errfmt.New(errfmt.KindIO, "E0063", fmt.Sprintf("cannot scan pack directory `%s`: %v", inputPath, err), absDir, 1, 1, nil)
	}

	if len(files) == 0 {
		return errfmt.New(errfmt.KindSyntax, "E0050", "pack directory contains no Lunex source files", absDir, 1, 1, nil)
	}

	sort.Slice(files, func(i, j int) bool {
		reli, _ := filepath.Rel(absDir, files[i])
		relj, _ := filepath.Rel(absDir, files[j])
		return filepath.ToSlash(reli) < filepath.ToSlash(relj)
	})

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return errfmt.New(errfmt.KindIO, "E0063", fmt.Sprintf("cannot read `%s`: %v", file, err), file, 1, 1, nil)
		}
		isMain := filepath.Clean(file) == filepath.Clean(mainPath)
		if err := validatePackSource(string(data), file, !isMain); err != nil {
			return err
		}
		if _, err := validatePackCompilerGraphWithRoot(absDir, file, string(data)); err != nil {
			return err
		}
	}

	if outputFile == "" {
		base := filepath.Base(filepath.Clean(absDir))
		if base == "." || base == string(filepath.Separator) || base == "" {
			base = "app"
		}
		outputFile = base + ".nax"
	}
	outputFile = normalizeNAXOutput(outputFile)
	if outDir := filepath.Dir(outputFile); outDir != "." && outDir != "" {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return errfmt.New(errfmt.KindIO, "E0063", fmt.Sprintf("cannot create output directory `%s`: %v", outDir, err), outDir, 1, 1, nil)
		}
	}

	var packErr error
	if includeSource {
		packErr = bytecode.PackDirectorySource(absDir, outputFile)
	} else {
		packErr = bytecode.PackDirectory(absDir, outputFile)
	}
	if packErr != nil {
		return newPackInternalError(absDir, packErr.Error())
	}

	fi, err := os.Stat(outputFile)
	if err != nil {
		return newPackInternalError(absDir, fmt.Sprintf("directory archive was emitted but could not be stat'ed: %v", err))
	}
	fmt.Printf("%s → %s (%s, directory bundle)\n", inputPath, outputFile, formatBytes(fi.Size()))
	fmt.Println("help: You can also use the --source flag to facilitate future code retrieval using NAX.")
	return nil
}

func parsePackCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lunex pack <file.lx|directory> [--source] [-o output.nax]")
		os.Exit(1)
	}
	inputPath := args[0]
	outputFile := ""
	includeSource := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--source":
			includeSource = true
		case "-o", "--output":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				fmt.Fprintln(os.Stderr, "error: -o requires a value")
				os.Exit(1)
			}
			outputFile = args[i+1]
			i++
		default:
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
			fmt.Fprintln(os.Stderr, "  usage: lunex pack <file.lx|directory> [--source] [-o output.nax]")
			os.Exit(1)
		}
	}

	absInput, err := filepath.Abs(inputPath)
	if err != nil {
		fmt.Fprint(os.Stderr, errfmt.Format(errfmt.New(errfmt.KindIO, "E0063", fmt.Sprintf("cannot resolve pack input `%s`: %v", inputPath, err), inputPath, 1, 1, nil)))
		os.Exit(1)
	}
	info, err := os.Stat(absInput)
	if err != nil {
		fmt.Fprint(os.Stderr, errfmt.Format(errfmt.New(errfmt.KindIO, "E0063", fmt.Sprintf("cannot access pack input `%s`: %v", inputPath, err), absInput, 1, 1, nil)))
		os.Exit(1)
	}

	if info.IsDir() {
		if err := packSourceDirectory(absInput, inputPath, outputFile, includeSource); err != nil {
			if le, ok := err.(*errfmt.LunexError); ok {
				fmt.Fprint(os.Stderr, errfmt.Format(le))
			} else {
				fmt.Fprint(os.Stderr, err.Error())
			}
			os.Exit(1)
		}
		return
	}

	if !strings.EqualFold(filepath.Ext(absInput), ".lx") {
		fmt.Fprint(os.Stderr, errfmt.Format(errfmt.New(errfmt.KindSyntax, "E0050", fmt.Sprintf("pack accepts a Lunex source file or directory, got `%s`", filepath.Ext(absInput)), absInput, 1, 1, nil)))
		os.Exit(1)
	}
	if err := packSourceFile(absInput, inputPath, outputFile, includeSource); err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func validatePackSource(source, filename string, allowMissingMain bool) error {
	c := newCompiler()
	ch := checker.New(checker.Options{
		Loader:           packCheckerLoader(),
		KnownModules:     checkerKnownModules(c),
		CheckStdMembers:  true,
		AllowMissingMain: allowMissingMain,
	})
	result := ch.Check(source, filename)
	errorCount := 0
	for i := range result.Diagnostics {
		errorCount++
		fmt.Fprint(os.Stderr, formatCheckerDiagnostic(&result.Diagnostics[i]))
	}
	if errorCount > 0 {
		return newPackInternalError(filename, fmt.Sprintf("semantic validation aborted NAX emission: files=%d imports=%d symbols=%d errors=%d", result.Files, result.Imports, result.Symbols, errorCount))
	}
	return nil
}

func validatePackCompilerGraph(absInput, source string) (*ast.Node, error) {
	return validatePackCompilerGraphWithRoot(filepath.Dir(absInput), absInput, source)
}

func validatePackCompilerGraphWithRoot(rootDir, absInput, source string) (*ast.Node, error) {
	visited := map[string]bool{absInput: true}
	var validate func(string, string) (*ast.Node, error)
	validate = func(filename, sourceText string) (*ast.Node, error) {
		compilerInstance := newCompiler()
		result := compilerInstance.CompileSource(sourceText, filename)
		if !result.Success {
			for _, e := range result.Errors {
				fmt.Fprint(os.Stderr, errfmt.Format(e))
			}
			return nil, newPackInternalError(filename, "compiler rejected a source after semantic validation")
		}
		for _, spec := range mustFindPackImports(result.AST) {
			importAbs, _, err := resolveBundleImport(rootDir, filename, spec)
			if err != nil {
				e := errfmt.New(errfmt.KindImport, "E0010", fmt.Sprintf("module not found: '%s'", spec), filename, 1, 1, nil)
				e.Notes = []string{err.Error()}
				e.Suggestion = "check the @fimport path and make sure the .lx file exists"
				fmt.Fprint(os.Stderr, errfmt.Format(e))
				return nil, newPackInternalError(filename, "local import graph could not be resolved")
			}
			if visited[importAbs] {
				continue
			}
			visited[importAbs] = true
			data, err := os.ReadFile(importAbs)
			if err != nil {
				e := errfmt.New(errfmt.KindIO, "E0063", fmt.Sprintf("cannot read local import '%s': %v", importAbs, err), importAbs, 1, 1, nil)
				fmt.Fprint(os.Stderr, errfmt.Format(e))
				return nil, newPackInternalError(filename, "local import source could not be read")
			}
			if _, err := validate(importAbs, string(data)); err != nil {
				return nil, err
			}
		}
		return result.AST, nil
	}
	return validate(absInput, source)
}

func mustFindPackImports(tree *ast.Node) []string {
	imports, err := findForceLocalImports(tree)
	if err != nil {
		return nil
	}
	return imports
}

func packCheckerLoader() checker.Loader {
	return func(path, from string) (string, string, bool) {
		if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") || strings.HasPrefix(path, "/") || strings.HasSuffix(strings.ToLower(path), ".lx") || strings.HasSuffix(strings.ToLower(path), ".nax") {
			candidate := path
			if !filepath.IsAbs(candidate) {
				candidate = filepath.Join(filepath.Dir(from), filepath.FromSlash(candidate))
			}
			candidate, _ = filepath.Abs(candidate)
			if src, ok := moduleSourceFromPath(candidate); ok {
				return src, candidate, true
			}
			return "", candidate, false
		}
		resolved, ok := pkg.Resolve(path)
		if !ok {
			resolved = strings.TrimPrefix(path, "std.")
			if !containsString([]string{"io", "fs", "http", "crypto", "db", "env", "testing", "ffi", "ws", "utils", "json", "jwt", "math", "datetime", "os", "regex", "buffer", "ints", "runtime", "http.router", "http.static"}, resolved) {
				return "", "", false
			}
			return "", "<std:" + resolved + ">", true
		}
		src, ok := moduleSourceFromPath(resolved)
		return src, resolved, ok
	}
}

func formatCheckerDiagnostic(d *errfmt.LunexError) string {
	return errfmt.Format(d)
}

func newPackInternalError(filename, detail string) error {
	message := "NAX emission aborted: compiler/checker invariant violated"
	if detail != "" {
		message += ": " + detail
	}
	e := errfmt.New(errfmt.KindRuntime, errfmt.ErrPackPipeline, message, filename, 1, 1, nil)
	e.Notes = []string{"pipeline=lexer -> parser -> resolver -> checker -> bundle -> NAX", "archive emission was stopped because an internal pipeline invariant failed"}
	e.Suggestion = "fix the reported diagnostics and rerun `lunex pack`"
	return fmt.Errorf("%s", errfmt.Format(e))
}

func normalizeNAXOutput(output string) string {
	if strings.EqualFold(filepath.Ext(output), ".nax") {
		return output
	}
	return strings.TrimSuffix(output, filepath.Ext(output)) + ".nax"
}

func formatBytes(size int64) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%d B", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
}

func runNTLWithCache(absPath string) {
	t0 := time.Now()
	dbg.VHeader(absPath)
	dbg.Header(absPath)

	if noCache {
		dbg.Step("--no-cache active", "compiling fresh (memory-only)")
		source, err := os.ReadFile(absPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading file: %v\n", err)
			os.Exit(1)
		}
		srcText := string(source)
		c := newCompiler()
		result := c.CompileSource(srcText, absPath)
		if !result.Success {
			for _, e := range result.Errors {
				fmt.Fprint(os.Stderr, errfmt.Format(e))
			}
			os.Exit(1)
		}
		chunk := &bytecode.ExportedChunk{
			Name:       filepath.Base(absPath),
			SourceFile: absPath,
			SourceText: srcText,
		}
		objectData, err := bytecode.EncodeExportedWithAST(chunk, result.AST)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		dbg.StepOK("done", fmt.Sprintf("bytecode %d bytes (memory-only, not cached)", len(objectData)), "")
		execBinary(objectData)
		dbg.VFooter(time.Since(t0))
		dbg.Footer(time.Since(t0))
		return
	}

	dbg.VSection("checking cache")
	dbg.VStep("is there a compiled version of this file?", absPath)
	dbg.Step("checking cache...", absPath)
	if cached, ok := bytecode.CacheLookup(absPath); ok {
		dbg.StepOK("hit", "found in cache, skipping compile", absPath)
		dbg.VStep("found in cache, no need to recompile")
		dbg.VSection("running")
		execBinary(cached)
		dbg.VFooter(time.Since(t0))
		dbg.Footer(time.Since(t0))
		return
	}
	dbg.StepWarn("cache miss", "compiling from scratch")
	dbg.VStep("nothing cached, reading source file")

	dbg.VSection("reading file")
	dbg.Step("opening file...", absPath)
	source, err := os.ReadFile(absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading file: %v\n", err)
		os.Exit(1)
	}
	srcText := string(source)
	dbg.StepOK("done", "file loaded", fmt.Sprintf("%d bytes", len(srcText)))
	dbg.VKV("file", absPath)
	dbg.VKV("size", len(srcText))
	dbg.VKV("lines", strings.Count(srcText, "\n")+1)

	dbg.VSection("compiling")
	dbg.Step("compiling...", absPath)
	t1 := time.Now()
	c := newCompiler()
	result := c.CompileSource(srcText, absPath)
	compileElapsed := time.Since(t1)
	if !result.Success {
		dbg.VStep("compile failed")
		for _, e := range result.Errors {
			fmt.Fprint(os.Stderr, errfmt.Format(e))
		}
		os.Exit(1)
	}
	dbg.StepOK("done", "compiled", compileElapsed.Round(time.Microsecond).String())
	dbg.VKV("compile time", compileElapsed.Round(time.Microsecond))

	dbg.VSection("generating bytecode")
	dbg.Step("generating bytecode...", "")
	chunk := &bytecode.ExportedChunk{
		Name:       filepath.Base(absPath),
		SourceFile: absPath,
		SourceText: srcText,
	}
	objectData, err := bytecode.EncodeExportedWithAST(chunk, result.AST)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	dbg.StepOK("done", "bytecode ready", fmt.Sprintf("%d bytes", len(objectData)))
	dbg.VKV("bytecode size", fmt.Sprintf("%d bytes", len(objectData)))

	dbg.VStep("saving to cache for next time")
	dbg.Step("caching...", "")
	_ = bytecode.CacheStore(absPath, objectData)
	dbg.StepOK("done", "cached", "")

	dbg.VSection("running")
	dbg.Section("running")
	execBinary(objectData)
	dbg.VFooter(time.Since(t0))
	dbg.Footer(time.Since(t0))
}

func execBinary(objectData []byte) {
	defer safeRecover()
	if len(objectData) >= 5 && string(objectData[:5]) == "LXNAX" {
		dbg.Step("running NAX archive", "Go interpreter executes the compiled AST")
		if err := bytecode.RunNAX(objectData, pkgLoader, pkgLoader); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	ntz := bytecode.NTZSection(objectData)
	dbg.BytecodeSection(len(objectData), len(ntz), len(ntz) > 0)
	dbg.Step("running compiled Lunex object", "Go interpreter executes the AST")
	if err := bytecode.RunObject(objectData, pkgLoader, pkgLoader); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func seeErrors(filePath string) {
	source, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	c := newCompiler()
	result := c.CompileSource(string(source), filePath)
	if result.Success {
		fmt.Printf("%s: no errors\n", filePath)
		return
	}
	for _, e := range result.Errors {
		fmt.Fprint(os.Stderr, errfmt.Format(e))
	}
	os.Exit(1)
}

func debugFile(filePath string, scriptArgs []string) {
	os.Setenv("NTL_DEBUG", "1")
	dbg.Enable()

	source, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	c := newCompiler()
	result := c.CompileSource(string(source), absPath)
	if !result.Success {
		fmt.Fprintf(os.Stderr, "%s: %d compile error(s)\n\n", filePath, len(result.Errors))
		for _, e := range result.Errors {
			fmt.Fprint(os.Stderr, errfmt.Format(e))
		}
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "%s: compiled cleanly, running with full trace...\n\n", filePath)

	prevArgs := os.Args
	os.Args = append([]string{absPath}, scriptArgs...)
	defer func() { os.Args = prevArgs }()

	chunk := &bytecode.ExportedChunk{
		Name:       absPath,
		SourceFile: absPath,
		SourceText: string(source),
	}
	objectData, err := bytecode.EncodeExportedWithAST(chunk, result.AST)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	execBinary(objectData)
}

func checkFile(filePath string) {
	started := time.Now()
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	source, err := os.ReadFile(absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	dbg.VHeader(absPath)
	dbg.Header(absPath)
	dbg.Section("Lunex production checker")
	dbg.Step("loading source", fmt.Sprintf("%d bytes", len(source)))
	dbg.VKV("file", absPath)
	dbg.VKV("source bytes", len(source))

	loader := packCheckerLoader()
	c := newCompiler()

	ch := checker.New(checker.Options{
		Loader:          loader,
		KnownModules:    checkerKnownModules(c),
		CheckStdMembers: true,
		Debug: func(format string, args ...any) {
			dbg.Log(format, args...)
		},
		Verbose: func(format string, args ...any) {
			dbg.VStep(format, args...)
		},
	})

	dbg.Step("parsing and resolving", "module graph + semantic analysis")
	result := ch.Check(string(source), absPath)
	dbg.VKV("files checked", result.Files)
	dbg.VKV("imports resolved", result.Imports)
	dbg.VKV("symbols indexed", result.Symbols)

	errors := len(result.Diagnostics)

	for i := range result.Diagnostics {
		printCheckDiagnostic(&result.Diagnostics[i])
	}

	dbg.VStep("diagnostics complete", "errors", errors)
	dbg.Footer(time.Since(started))
	dbg.VFooter(time.Since(started))

	if errors > 0 {
		fmt.Fprintf(os.Stderr, "\nerror: could not compile `%s` due to %d previous error(s)\n", filePath, errors)
		os.Exit(1)
	}
	fmt.Printf("\nFinished checking %s: no errors\n", filePath)
}

func checkerKnownModules(c *compiler.Compiler) map[string]map[string]struct{} {
	known := make(map[string]map[string]struct{})
	names := []string{"runtime", "io", "fs", "http", "crypto", "db", "env", "testing", "ffi", "ws", "utils", "json", "jwt", "math", "datetime", "os", "regex", "buffer", "ints", "http/router", "http/static", "internal.native"}
	for _, name := range names {
		mod, ok := c.Interpreter().GetModule(name)
		if !ok || mod == nil || mod.ObjVal == nil {
			continue
		}
		members := make(map[string]struct{}, len(mod.ObjVal))
		for key := range mod.ObjVal {
			members[key] = struct{}{}
		}
		known[name] = members
	}
	return known
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func printCheckDiagnostic(d *errfmt.LunexError) {
	fmt.Fprint(os.Stderr, errfmt.Format(d))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func disassembleFile(filePath string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", filePath, err)
		os.Exit(1)
	}

	chunk, err := bytecode.DecodeObject(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	output := chunk.Disassemble()

	outFile := filePath + ".lx"

	err = os.WriteFile(outFile, []byte(output), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", outFile, err)
		os.Exit(1)
	}

	fmt.Printf("disassembled file written to %s\n", outFile)
}

func showCacheInfo() {
	dir := bytecode.CacheDir()
	fmt.Printf("cache dir    : %s\n", dir)

	entries, err := os.ReadDir(dir)
	if err == nil {
		count := 0
		total := int64(0)
		for _, e := range entries {
			if filepath.Ext(e.Name()) == ".nax" {
				count++
				if info, err := e.Info(); err == nil {
					total += info.Size()
				}
			}
		}
		fmt.Printf("disk entries : %d  (%d KB)\n", count, total/1024)
	}

	mc, mb := adaptor.MemCacheStats()
	fmt.Printf("mem entries  : %d  (%d bytes)\n", mc, mb)
}

func showMemCacheInfo() {
	count, totalBytes := adaptor.MemCacheStats()
	fmt.Printf("in-memory bytecode cache\n")
	fmt.Printf("  entries : %d\n", count)
	fmt.Printf("  size    : %d bytes\n", totalBytes)
	fmt.Printf("  note    : cache lives only for this process; use 'lunex cache' for disk cache\n")
}

func clearCache() {
	dir := bytecode.CacheDir()
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println("cache cleared")
}

func showRuntimes() {
	fmt.Printf("Go interpreter:   available  (handles all Lunex execution)\n")
}

func setCacheDir(dir string) {
	if dir == "reset" {
		if err := bytecode.SetCacheDir(""); err != nil {
			fmt.Fprintln(os.Stderr, "error resetting cache dir:", err)
			os.Exit(1)
		}
		fmt.Printf("cache directory reset to default: %s\n", bytecode.CacheDir())
		return
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(absDir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, "error creating cache directory:", err)
		os.Exit(1)
	}
	if err := bytecode.SetCacheDir(absDir); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("cache directory set to: %s\n", absDir)
}

func unpackNAX(naxPath string) {
	absPath, err := filepath.Abs(naxPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", naxPath, err)
		os.Exit(1)
	}
	baseName := strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath))
	outDir := filepath.Join(filepath.Dir(absPath), baseName)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, "error creating output directory:", err)
		os.Exit(1)
	}
	count, err := bytecode.UnpackNAX(data, outDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("unpacked %d files from %s → %s/\n", count, naxPath, outDir)
}

func runBench(filePath string) {
	if !strings.HasSuffix(filePath, ".lx") {
		filePath += ".lx"
	}
	source, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	srcText := string(source)
	absPath, _ := filepath.Abs(filePath)

	c := newCompiler()
	t0 := time.Now()
	result := c.CompileSource(srcText, absPath)
	if !result.Success {
		for _, e := range result.Errors {
			fmt.Fprint(os.Stderr, errfmt.Format(e))
		}
		os.Exit(1)
	}
	compileTime := time.Since(t0)
	fmt.Printf("compile: %v\n", compileTime)

	chunk := &bytecode.ExportedChunk{
		Name:       filepath.Base(filePath),
		SourceFile: absPath,
		SourceText: srcText,
	}
	objectData, err := bytecode.EncodeExportedWithAST(chunk, result.AST)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	t1 := time.Now()
	execBinary(objectData)
	runTime := time.Since(t1)
	fmt.Printf("run:     %v\n", runTime)
}

func runStart(extraArgs []string) {
	bfPath, ok := buildfile.Find()
	if !ok {
		fmt.Fprintln(os.Stderr, "error: no lunex.toml found in current directory")
		fmt.Fprintln(os.Stderr, "  run 'lunex init' to create one, or specify a project directory")
		os.Exit(1)
	}

	cfg, err := buildfile.Parse(bfPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", bfPath, err)
		os.Exit(1)
	}

	entry := cfg.Entry
	if entry == "" {
		entry = "main.lx"
	}
	if !filepath.IsAbs(entry) {
		entry = filepath.Join(filepath.Dir(bfPath), entry)
	}

	runFile(entry, extraArgs)
}

func runInstallCommand(args []string) {
	global := false
	local := false
	rest := args[:0]
	for _, a := range args {
		switch a {
		case "-g", "--global":
			global = true
		case "-l", "--local":
			local = true
		default:
			rest = append(rest, a)
		}
	}
	args = rest

	if !global && !local {
		if len(args) > 0 {
			fmt.Fprintln(os.Stderr, "usage: lunex install -g <url>[@version]   install globally")
			fmt.Fprintln(os.Stderr, "       lunex install -l <url>[@version]   install locally")
			fmt.Fprintln(os.Stderr, "       lunex install                       install everything in lunex.toml")
			os.Exit(1)
		}
		if _, ok := manifest.Find(); !ok {
			fmt.Fprintln(os.Stderr, "error: no lunex.toml found in current directory")
			fmt.Fprintln(os.Stderr, "  run 'lunex init' to create one, or use 'lunex install -g/-l <url>'")
			os.Exit(1)
		}
		if err := pkg.InstallAll(pkg.InstallOptions{Global: false}); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lunex install -g|-l <url>[@version] [more urls...]")
		os.Exit(1)
	}

	opts := pkg.InstallOptions{Global: global}
	for _, spec := range args {
		mod, lib, err := pkg.InstallFromSpec(spec, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error installing %s: %v\n", spec, err)
			os.Exit(1)
		}
		if local {
			if manifestPath, ok := manifest.Find(); ok {
				_ = pkg.AddLibraryToManifest(manifestPath, lib)
			}
		}
		scope := "locally"
		if global {
			scope = "globally"
		}
		fmt.Printf("installed %s@%s %s\n", mod.Name, mod.Version, scope)
	}
	os.Exit(0)
}

func runAddCommand(args []string) {
	if _, ok := manifest.Find(); !ok {
		fmt.Fprintln(os.Stderr, "error: no lunex.toml found in current directory")
		fmt.Fprintln(os.Stderr, "  run 'lunex init' first")
		os.Exit(1)
	}
	for _, spec := range args {
		mod, lib, err := pkg.InstallFromSpec(spec, pkg.InstallOptions{Global: false})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error adding %s: %v\n", spec, err)
			os.Exit(1)
		}
		manifestPath, _ := manifest.Find()
		if err := pkg.AddLibraryToManifest(manifestPath, lib); err != nil {
			fmt.Fprintf(os.Stderr, "error updating lunex.toml: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("added %s@%s to lunex.toml\n", mod.Name, mod.Version)
	}
	os.Exit(0)
}

func printEnv() {
	info := pkg.Env()
	fmt.Printf("Lunex %s\n\n", meta.Version())
	fmt.Printf("global store  : %s\n", filepath.Join(info.GlobalRoot, "modules"))
	fmt.Printf("  installed   : %d version(s)\n", info.GlobalCount)
	fmt.Printf("local store   : %s\n", filepath.Join(info.LocalRoot, "modules"))
	fmt.Printf("  installed   : %d version(s)\n", info.LocalCount)
	fmt.Println()
	if info.HasManifest {
		fmt.Printf("lunex.toml    : found (%s)\n", info.ManifestPath)
	} else {
		fmt.Printf("lunex.toml    : not found in current directory\n")
	}
	if info.HasLock {
		fmt.Printf("lunex.lock    : found\n")
	} else {
		fmt.Printf("lunex.lock    : not found\n")
	}
}

func printHelpCompact() {
	type entry struct{ cmd, desc string }
	sections := []struct {
		title   string
		entries []entry
	}{
		{"Usage", []entry{
			{"lunex run <file>", "run a .lx source or .nax archive"},
			{"lunex start", "run the project entry from lunex.toml"},
			{"lunex debug <file>", "run with full diagnostics + stack trace"},
			{"lunex -e \"<code>\"", "run a code snippet directly"},
			{"lunex repl", "start the interactive REPL"},
			{"lunex check <file>", "check for errors without running"},
			{"lunex init [name]", "create a new project folder"},
			{"lunex pack <file.lx|directory>", "validate and bundle source or project directory to .nax"},
			{"lunex unpack <file.nax>", "recover .lx code from the compiled NAX archive"},
			{"lunex cache [clear]", "show or clear the on-disk cache"},
			{"lunex platform", "show platform / adapter diagnostics"},
			{"lunex bench <file>", "run with timing output"},
			{"lunex env", "show module store paths and status"},
			{"lunex version", "print version"},
			{"lunex help", "show this help"},
		}},
		{"Modules", []entry{
			{"@import(\"std.io\")", "standard library module"},
			{"lunex ffi = on run app.lx", "enable native FFI for this process"},
			{"@import(\"pkg-name\")", "installed library"},
			{"@fimport(\"./f.nax\")", "local .nax archive file"},
			{"@fimport(\"./f.lx\")", "local .lx source file"},
		}},
		{"Dependencies", []entry{
			{"lunex install", "install all lunex.toml libraries"},
			{"lunex add <url>[@v]", "add + install a dependency"},
			{"lunex remove <lib>", "remove an installed library"},
			{"lunex update [lib]", "re-resolve one or all libraries"},
			{"lunex list", "list installed libraries"},
		}},
	}

	for _, s := range sections {
		fmt.Printf("%s:\n", s.title)
		for _, e := range s.entries {
			fmt.Printf("  %s\n      %s\n", e.cmd, e.desc)
		}
		fmt.Println()
	}

	fmt.Print(`Flags: --debug/-d  --verbose/-V  --no-cache

Run 'lunex help' in a wider terminal for the full reference,
including the standard library module list.

`)
}

func consumeFFIOption(args []string) []string {
	if len(args) == 0 || args[0] != "ffi" {
		return args
	}
	if len(args) < 3 || args[1] != "=" || (args[2] != "on" && args[2] != "off") {
		e := errfmt.New(errfmt.KindSyntax, "E0133", "invalid FFI option syntax", "<command line>", 1, 1, nil)
		e.Suggestion = "use `lunex ffi = on run <file>` or `lunex ffi = off run <file>`"
		e.Notes = []string{"the FFI switch is a command-line prefix", "FFI is disabled when no prefix is supplied"}
		fmt.Fprint(os.Stderr, errfmt.Format(e))
		os.Exit(2)
	}
	std.SetFFIEnabled(args[2] == "on")
	remaining := args[3:]
	if len(remaining) == 0 {
		state := "off"
		if std.IsFFIEnabled() {
			state = "on"
		}
		fmt.Printf("FFI is %s for this Lunex process.\n", state)
		fmt.Println("Provide a command after the option, for example: lunex ffi = on run app.lx")
		os.Exit(0)
	}
	return remaining
}

func printHelp() {
	fmt.Printf("Lunex %s\n\n", meta.Version())

	if adaptor.TerminalWidth() < 64 {
		printHelpCompact()
		return
	}

	fmt.Print(`Usage:
  lunex run <file> [--emit ast|ir]   run a .lx source or .nax archive
  lunex start                        run the project entry from lunex.toml
  lunex debug <file>                 run with full compile diagnostics and a stack trace on error
  lunex -e "<code>"                  run a code snippet directly
  lunex repl                         start the interactive REPL
  lunex check <file>                 check for errors without running
  lunex see_errors <file>            show detailed compile errors
  lunex init [name]                  create a new project folder
  lunex init <template> <name>       create a project from a template
                                        (http_server, database, website)
  lunex pack <file.lx|directory> [--source] validate and bundle source or project directory to an optimized .nax archive
  lunex unpack <file.nax>            recover .lx code from the compiled NAX archive to a directory
  lunex set cache <dir>              set the on-disk runtime cache directory
  lunex set cache reset              reset the cache directory to default
  lunex cache [clear]                show or clear the on-disk runtime cache
  lunex memcache [clear]             show or clear the in-process memory cache
  lunex platform                     show platform / adapter diagnostics
  lunex runtimes                     show available execution engines
  lunex bench <file>                 run with timing output
  lunex env                          show global/local module store paths and project status
  lunex link                         link this project's [project.bin] commands globally
  lunex version                      print version
  lunex help                         show this help

Module system:
  @import("std.io")                  standard library module (always available)
  @import("pkg-name")                library installed via lunex install/add
  @fimport("./mylib.nax")            local .nax archive file
  @fimport("./src/utils.lx")         local .lx source file

Dependency management (lunex.toml + lunex.lock):
  lunex install                      install every [libraries.*] entry in lunex.toml
  lunex install -g <url>[@version]   install a library globally (~/.lunex), no lunex.toml needed
  lunex install -l <url>[@version]   install a library locally (./.lunex) for this project only
  lunex add <url>[@version]          add a dependency to lunex.toml and install it
  lunex remove <library>             remove an installed library
  lunex update [library]             re-resolve one or all libraries against lunex.toml
  lunex list                         list installed libraries and their scope (local/global)

  Each installed version is kept isolated on disk, so two projects — or
  two dependencies of the same project — can each depend on a different
  version of the same library without conflict. lunex.lock records the
  exact resolved version, source, and hash for every library so a build
  stays reproducible.

Executable commands ("bin", like package.json):
  [project]
  bin = "./cli.lx"                   single command, named after the project

  [project.bin]                      or multiple named commands
  build = "./bin/build.lx"
  serve = "./bin/serve.lx"

  When a library declaring [project.bin] is installed (lunex install -g/-l,
  or a dependency in lunex.toml), Lunex writes an executable shim per
  command into ~/.lunex/bin (global) or ./.lunex/bin (local) that runs
  ` + "`lunex run <entry>`" + `. Add ~/.lunex/bin to PATH to run those commands
  directly. Use 'lunex link' to expose the project you're developing the
  same way, without installing it first.

Global flags (place before the command or file):
  --debug, -d   enable debug mode (shows every execution step on stderr)
  --verbose, -V enable verbose debug output (implies --debug)
  --no-cache    compile fresh every run; store nothing to disk or memory

Native FFI (disabled by default; CLI-only):
  lunex ffi = on run app.lx      enable FFI for this process
  lunex ffi = off run app.lx     explicitly disable FFI for this process
  The FFI switch is not read from source code or environment variables.

Environment variables:
  LUNEX_DEBUG=1   enable debug mode
  LUNEX_VERBOSE=1 verbose debug output (implies LUNEX_DEBUG=1)

Standard library modules:
  io         Console I/O: print, log, warn, table, colors
  fs         File system: read, write, list, stat, copy, glob
  http       HTTP client and server
  crypto     Hashing, encryption, JWT, passwords, UUIDs
  db         SQLite-backed document database (stored in .lunex/data/)
  ws         WebSocket server and client
  jwt        JSON Web Token sign and verify
  json       Parse, stringify, validate, read/write JSON files
  math       Math functions and constants (PI, E, sqrt, pow, ...)
  datetime   Date and time utilities
  os         OS interaction: exec, env, platform, paths
  regex      Regular expression matching and replacement
  env        Read and write environment variables
  ffi        Native shared-library loading, symbol binding, calls, callbacks, and memory access
  utils      String, array, and object helpers

`)
}

const (
	replPrompt   = "\x1b[1;36mlunex\x1b[0m \x1b[90m»\x1b[0m "
	replContinue = "      \x1b[90m·\x1b[0m "
	replReset    = "\x1b[0m"
	replBold     = "\x1b[1m"
	replDim      = "\x1b[90m"
	replGreen    = "\x1b[32m"
	replYellow   = "\x1b[33m"
	replCyan     = "\x1b[36m"
	replMagenta  = "\x1b[35m"
	replRed      = "\x1b[31m"
)

type replState struct {
	c       *compiler.Compiler
	interp  *runtime.Interpreter
	history []string
	session int
}

func newReplState() *replState {
	c := compiler.New(compiler.Options{REPL: true, Silent: true})
	std.RegisterAll(c)
	c.Interpreter().SetNTLLoader(pkgLoader)
	return &replState{
		c:      c,
		interp: c.Interpreter(),
	}
}

func runREPL() {
	printReplBanner()

	state := newReplState()
	reader := bufio.NewReader(os.Stdin)
	var buf strings.Builder

	for {
		prompt := replPrompt
		if buf.Len() > 0 {
			prompt = replContinue
		}

		fmt.Print(prompt)
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println()
			printReplBye()
			return
		}

		line = strings.TrimRight(line, "\r\n")

		trimmed := strings.TrimSpace(line)

		if trimmed == "" && buf.Len() > 0 {
			buf.WriteString("\n")
			continue
		}

		switch trimmed {
		case ".exit", ".quit", "exit", "quit":
			printReplBye()
			return

		case ".help":
			printReplHelp()
			continue

		case ".clear":
			state = newReplState()
			fmt.Printf("%s  session cleared — all variables and definitions reset%s\n", replDim, replReset)
			continue

		case ".history":
			if len(state.history) == 0 {
				fmt.Printf("%s  (no history yet)%s\n", replDim, replReset)
			} else {
				for i, h := range state.history {
					fmt.Printf("%s%3d%s  %s\n", replDim, i+1, replReset, h)
				}
			}
			continue

		case ".vars":
			names := state.interp.GetAllGlobalNames()
			if len(names) == 0 {
				fmt.Printf("%s  (no variables defined yet)%s\n", replDim, replReset)
			} else {
				fmt.Printf("%s  defined: %s%s\n", replDim, strings.Join(names, ", "), replReset)
			}
			continue
		}

		if strings.HasPrefix(trimmed, ".load ") {
			filePath := strings.TrimSpace(strings.TrimPrefix(trimmed, ".load "))
			replLoadFile(state, filePath)
			continue
		}

		if strings.HasPrefix(trimmed, ".type ") {
			expr := strings.TrimSpace(strings.TrimPrefix(trimmed, ".type "))
			replShowType(state, expr)
			continue
		}

		if buf.Len() > 0 {
			buf.WriteString("\n")
		}
		buf.WriteString(line)

		src := buf.String()

		if isIncomplete(src) {
			continue
		}

		buf.Reset()
		src = strings.TrimSpace(src)
		if src == "" {
			continue
		}

		state.history = append(state.history, src)
		state.session++

		replEval(state, src)
	}
}

func replEval(state *replState, src string) {
	wrapped, wasWrapped := replWrap(src)

	result := state.c.CompileSource(wrapped, "<repl>")
	if !result.Success {
		if wasWrapped {
			result2 := state.c.CompileSource(src, "<repl>")
			if result2.Success {
				replExec(state, result2, src, false)
				return
			}
		}
		for _, e := range result.Errors {
			fmt.Fprint(os.Stderr, errfmt.Format(e))
		}
		return
	}

	replExec(state, result, wrapped, wasWrapped)
}

func replExec(state *replState, result *compiler.CompileResult, src string, wasWrapped bool) {
	interp := state.interp
	interp.SetFilename("<repl>")
	interp.SetSourceLines(strings.Split(src, "\n"))

	if !wasWrapped {
		_, err := interp.Exec(result.AST)
		if err != nil {
			printReplError(err, src)
			return
		}
		return
	}

	_, err := interp.Exec(result.AST)
	if err != nil {
		printReplError(err, src)
		return
	}
	val, err := interp.CallExport("__repl_expr__")
	if err != nil {
		if err := interp.CallMain(); err != nil {
			printReplError(err, src)
		}
		return
	}

	if val != nil {
		printReplValue(val)
	} else {
		if err := interp.CallMain(); err != nil {
			printReplError(err, src)
		}
	}
}

func replWrap(src string) (string, bool) {
	trimmed := strings.TrimSpace(src)

	if looksLikeDeclaration(trimmed) {
		return src, false
	}

	wrapped := "fn main() {\n" + indent(src, "  ") + "\n}"
	return wrapped, true
}

func looksLikeDeclaration(src string) bool {
	keywords := []string{
		"fn ", "val ", "var ", "class ", "enum ", "namespace ",
		"@import", "@fimport",
	}
	for _, kw := range keywords {
		if strings.HasPrefix(src, kw) {
			return true
		}
	}
	return false
}

func indent(src, prefix string) string {
	lines := strings.Split(src, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}

func isIncomplete(src string) bool {
	depth := 0
	inStr := false
	var strChar rune
	runes := []rune(src)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if inStr {
			if r == '\\' {
				i++
				continue
			}
			if r == strChar {
				inStr = false
			}
			continue
		}
		switch r {
		case '"', '\'', '`':
			inStr = true
			strChar = r
		case '{', '(', '[':
			depth++
		case '}', ')', ']':
			depth--
		case '/':
			if i+1 < len(runes) && runes[i+1] == '/' {
				for i < len(runes) && runes[i] != '\n' {
					i++
				}
			}
		}
	}
	return depth > 0
}

func replLoadFile(state *replState, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%serror: cannot read file %q: %v%s\n", replRed, path, err, replReset)
		return
	}
	src := strings.TrimSpace(string(data))
	if src == "" {
		return
	}
	fmt.Printf("%s  loading %s…%s\n", replDim, path, replReset)
	replEval(state, src)
}

func replShowType(state *replState, expr string) {
	wrapped := "fn main() {\n  " + expr + "\n}"
	result := state.c.CompileSource(wrapped, "<repl:type>")
	if !result.Success {
		for _, e := range result.Errors {
			fmt.Fprint(os.Stderr, errfmt.Format(e))
		}
		return
	}
	fmt.Printf("%s  %s%s : %sunknown%s  (type inference runs at execution time)%s\n",
		replDim, replBold, expr, replCyan, replDim, replReset)
}

func printReplValue(v interface{}) {
	if v == nil {
		return
	}
	s := fmt.Sprintf("%v", v)
	if s == "<nil>" || s == "" {
		return
	}
	fmt.Printf("%s← %s%s%s\n", replDim, replGreen+replBold, s, replReset)
}

func printReplError(err error, src string) {
	if lunexErr, ok := err.(*errfmt.LunexError); ok {
		if len(lunexErr.Lines) == 0 {
			lunexErr.Lines = strings.Split(src, "\n")
		}
		if lunexErr.File == "" {
			lunexErr.File = "<repl>"
		}
		fmt.Fprint(os.Stderr, errfmt.Format(lunexErr))
		return
	}
	fmt.Fprintf(os.Stderr, "%serror: %v%s\n", replRed, err, replReset)
}

func printReplBanner() {
	v := meta.Version()

	width := adaptor.TerminalWidth()
	ruleWidth := width - 2
	if ruleWidth > 56 {
		ruleWidth = 56
	}
	if ruleWidth < 10 {
		ruleWidth = 10
	}
	rule := replDim + strings.Repeat("─", ruleWidth) + replReset

	fmt.Printf("\n  %s\n", rule)
	fmt.Printf("  %s%sLunex %s%s  — interactive REPL\n", replBold, replCyan, v, replReset)
	if adaptor.IsAndroidLike() {
		fmt.Printf("  %srunning on %s%s\n", replDim, adaptor.Current.String(), replReset)
	}
	fmt.Printf("  %sType Lunex code and press Enter to evaluate.%s\n", replDim, replReset)
	fmt.Printf("  %s.help for commands  ·  .exit or Ctrl+D to quit%s\n", replDim, replReset)
	fmt.Printf("  %s\n\n", rule)
}

func printReplBye() {
	fmt.Printf("\n  %sgoodbye%s\n\n", replDim, replReset)
}

func printReplHelp() {
	fmt.Printf("\n  %s%sREPL commands%s\n\n", replBold, replCyan, replReset)
	cmds := [][2]string{
		{".help", "show this help"},
		{".exit / .quit", "exit the REPL"},
		{".clear", "reset the session (clear all variables and definitions)"},
		{".vars", "list all currently defined names"},
		{".history", "show input history for this session"},
		{".load <file>", "load and evaluate a .lx file into this session"},
		{".type <expr>", "show the type of an expression"},
		{"Ctrl+D", "exit (EOF)"},
	}
	for _, cmd := range cmds {
		fmt.Printf("  %s%-22s%s  %s%s%s\n", replBold+replCyan, cmd[0], replReset, replDim, cmd[1], replReset)
	}
	fmt.Printf("\n  %sMulti-line input: open a { block and press Enter — the REPL%s\n", replDim, replReset)
	fmt.Printf("  %scontinues reading until the block is closed.%s\n\n", replDim, replReset)
}
