# CLI Reference

Complete reference for the `lunex` command-line tool and its built-in package manager.

---
## Commands

### `lunex run`

Run a Lunex source file or archive.

```
lunex run <file> [--emit ast|ir]
```

| Flag          | Description                                        |
|---------------|----------------------------------------------------|
| `--emit ast`  | Print the parsed AST as JSON instead of running    |
| `--emit ir`   | Print the IR as JSON instead of running            |

Supported file extensions:

| Extension | Description               |
|-----------|---------------------------|
| `.lx`     | Lunex source file |
| `.nax`    | Compiled NAX archive |

**Examples:**

```bash
lunex run main.lx
lunex run build/app.nax
lunex run --emit ast main.lx
```

---

### `lunex fmt` / `lunex format`

Format a Lunex source file with syntax-aware token layout, AST-guided statement boundaries, and a source-integrity verifier. `format` is an alias for `fmt`.

```bash
lunex fmt <file.lx> [--check|--stdout|--write]
lunex format <file.lx> [--check|--stdout|--write]
```

By default, the command formats the file in place. `--write` explicitly selects the same in-place behavior. `--check` checks whether the file is already formatted without writing changes; it exits with status 1 when formatting changes are needed or validation fails. `--stdout` prints the verified result without changing the file. `--check` and `--stdout` cannot be used together.

The formatter tokenizes and parses the source with the Lunex lexer and parser, then uses AST source positions to distinguish statements, blocks, and object or array literals. It normalizes indentation, operator spacing, comma spacing, control-flow blocks, and long collections while preserving each token's original spelling. Comments, string and template literals, and source line-ending style are retained. The verifier compares token spellings and values, comment contents, and AST structure while ignoring source-position changes, and it parses the formatted result again. If validation fails, the formatter falls back to conservative whitespace changes; if the candidate still cannot be verified, the CLI leaves the file unchanged and exits with an error.

Lunex line comments use `/!`; block comments use `/* ... */`. The formatter preserves both forms, including their contents.

**Examples:**

```bash
lunex fmt main.lx
lunex fmt main.lx --write
lunex fmt main.lx --check
lunex fmt main.lx --stdout
lunex format main.lx
```

---

### `lunex repl`

Start the interactive REPL (Read-Eval-Print Loop).

```
lunex repl
```

Launches a persistent session where you can type Lunex code and see the
result immediately. All defined names persist across inputs within the session.

**REPL commands:**

| Command          | Description                                                |
|------------------|------------------------------------------------------------|
| `.help`          | Show available REPL commands                               |
| `.exit` / `.quit`| Exit the REPL                                              |
| `.clear`         | Reset the session (clears all variables and definitions)   |
| `.vars`          | List all currently defined names                           |
| `.history`       | Show input history for this session                        |
| `.load <file>`   | Load and evaluate a `.lx` file into the session            |
| `.type <expr>`   | Show the inferred type of an expression                    |
| `Ctrl+D`         | Exit (EOF)                                                 |

**Multi-line input:** open a `{` block and press Enter — the REPL keeps reading
until all braces are closed.

**Example session:**

```
lunex » val io = @import("std.io")
lunex » fn greet(name) { "Hello, " + name + "!" }
lunex » fn main() {
.....   val x = 42
.....   io.log(x * 2)
.....   greet("world")
..... }
84
← "Hello, world!"
```

---

### `lunex -e`

Run a code snippet directly from the command line.

```
lunex -e "<code>"
```

**Example:**

```bash
lunex -e 'val io = @import("std.io"); fn main() { io.log("hello") }'
```

---

### `lunex pack`

Validate Lunex source and emit a `.nax` archive. The command accepts either one `.lx` source file or a project directory. When a directory is supplied, `main.lx` at the project root is the executable entry point and every `.lx` file in the project is checked before the archive is emitted. Local `@fimport` dependencies are resolved and embedded automatically.

```
lunex pack <file.lx|directory> [--no-source] [-o <output.nax>]
```

For a file, the default output is `<file>.nax`. For a directory, the default output is `<directory>.nax`. A directory pack requires a root `main.lx`.

Examples:

```bash
lunex pack main.lx
lunex pack main.lx -o dist/app.nax
lunex pack ./my-project
lunex pack ./my-project -o dist/my-project.nax
```

The complete lexer, parser, AST, resolver, checker, compiler, and local module graph are validated before archive publication. The archive stores compiled NAX entries containing a binary representation of the validated AST rather than Lunex VM bytecode. Original source text is included by default, so `lunex unpack` can restore the original files without reconstructing or rewriting their syntax. When any diagnostic is found, the command exits without creating or replacing the `.nax` artifact. Use `--no-source` only when exact source recovery is not required. The legacy `--source` flag remains accepted.

### `lunex unpack`

Extract a `.nax` archive into a new directory, named after the archive
file (e.g. `app.nax` extracts to `./app/`).

```
lunex unpack <file.nax>
```

There is no `-o` flag. The output directory name is always derived from
the input file. Unpack refuses to write into an existing path, protecting existing project files from accidental overwrites. By default, compiled NAX entries preserve the original `.lx` source text for exact recovery. With `--no-source`, `lunex unpack` reconstructs canonical source from the stored AST, which may not retain original formatting or comments.

---

### `lunex version`

Print version information.

```
lunex version
```

Output includes the version number, build date, Go runtime version, operating
system, and architecture.

---

### `lunex platform`

Print platform and adapter diagnostics.

```
lunex platform
```

---

### `lunex runtimes`

Show the execution engine used by the current Lunex build and the native JIT capabilities of the current platform.

```
lunex runtimes
```

The report distinguishes the Go-based AST interpreter from the adaptive native JIT. The AST interpreter is the default execution engine and the universal fallback. On supported `amd64` and `arm64` targets, eligible hot functions and loops can be compiled to native machine code using Tier 1 baseline and Tier 2 specialized compilation.

The report also displays the active JIT settings: enabled state, hotness threshold, native-code verification, and compilation tracing. The `LUNEX_NATIVE`, `LUNEX_NATIVE_HOT`, `LUNEX_NATIVE_VERIFY`, and `LUNEX_NATIVE_TRACE` environment variables configure these settings. The global `--jit` and `--no-jit` options override the enabled state for the current process.

`.nax` archives and cached compiled objects are serialized Lunex program data containing AST information; they are not independent execution engines or a separate bytecode VM. They use the same interpreter and can use the native JIT for eligible code.

---

### `lunex lsp`

Start the Lunex Language Server Protocol server for an editor or IDE.

```
lunex lsp
lunex lsp --help
```

I run as a long-lived process and communicate over standard input and standard output using JSON-RPC 2.0 messages framed with `Content-Length` headers. I reserve standard output exclusively for protocol traffic; I send startup or transport errors to standard error. I do not execute the source program while providing editor features.

#### Connecting an editor

I expect the editor's LSP client to launch the `lunex` executable with `lsp` as its argument. The editor must have a Lunex language configuration that associates `.lx` files with that client. I use the workspace root supplied by the client, or the current working directory when the client does not provide a root. The process should be managed by the editor client rather than started as a separate terminal session.

A client configuration uses this command and argument pair:

```json
{
  "command": "lunex",
  "args": ["lsp"]
}
```

The executable must be available on the environment's `PATH`, or the client must supply its absolute path. I use UTF-16 document positions, incremental document synchronization, and normal LSP lifecycle messages. The editor should send `initialize`, `initialized`, document open/change/save/close notifications, then `shutdown` and `exit` when it is finished.

#### Features I provide

| Feature | Behavior |
|---------|----------|
| Completion | I suggest visible Lunex declarations, supported built-ins, language keywords, recognized standard-library members, object-literal properties, array members, and string members. |
| Completion resolution | I enrich selected items with the information already attached to the item. |
| Diagnostics | I run the Lunex parser and checker for open buffers and publish syntax and semantic diagnostics with document ranges. |
| Hover | I show symbol details and inferred literal or declaration information where the current analysis can establish it. |
| Definition and declaration | I navigate to known symbol declarations in the current file or workspace index. |
| Type definition | I navigate to a statically resolved type declaration when the available type information identifies one. |
| References | I find matching identifier occurrences in the current file and relevant files that import the target source. |
| Rename | I prepare identifier renames and return workspace edits for occurrences connected by the available symbol and scope information. |
| Document symbols | I list top-level declarations and recognized members for editor outlines and breadcrumbs. |
| Workspace symbols | I search indexed top-level Lunex declarations across workspace `.lx` files. |
| Document highlights | I highlight same-name identifier occurrences in the active document. |
| Semantic tokens | I classify recognized symbols, parameters, keywords, literals, operators, and member names. |
| Signature help | I show known function or component parameters while the cursor is inside a call. |
| Code actions | I offer replacement quick fixes when a Lunex diagnostic includes a concrete “did you mean” suggestion. |
| Folding ranges | I expose multi-line brace-delimited regions for editor folding. |

#### Analysis and completion behavior

I reuse the Lunex lexer, parser, AST, and checker. I analyze the current unsaved buffer after each document change, so diagnostics and suggestions do not require the file to be saved first. I scan workspace `.lx` files for symbols and skip common generated or dependency directories such as `.git`, `.lunex`, `node_modules`, `vendor`, `dist`, `build`, `target`, `coverage`, and `.cache`. I cap this initial workspace scan at 25,000 source files.

I filter ordinary completion candidates using the current lexical scope and, when I can identify it, the receiver's type. For example, after a string variable and a dot, I suggest the Lunex string operations rather than unrelated global names. For an object literal, I use its declared property and method names. I do not inject `null` into the general completion list, and I avoid suggesting names that are not recognized by the current analysis. Boolean literals are offered only when the typed prefix matches them.

I recognize the standard-library members explicitly listed by the server and local members that can be recovered from the parsed source. I do not invent members for a module whose exported shape I cannot establish. When the current buffer is incomplete or cannot be parsed, I attempt prefix recovery from earlier lines so existing declarations remain available. I still report syntax errors, and suggestions that require the missing syntax or type information may be limited until the buffer can be analyzed.

#### Workspace and source resolution

I index `.lx` files under the workspace folders sent by the editor. I resolve local imports relative to the importing file, trying the supplied path, a `.lx` suffix when no extension is present, and a `main.lx` entry inside a directory. Standard-library identifiers are suggested from the server's known member lists. Project dependency and generated archive formats are not treated as editable `.lx` source documents by the workspace scanner.

#### Troubleshooting

If the editor shows no language features, I first check that `lunex lsp` is executable from the editor's environment and that the client associates `.lx` files with the LSP process. I then inspect the editor's LSP logs and the process standard error. I keep standard output free of banners, prompts, and debug text because any non-protocol output can break the JSON-RPC stream. I use `lunex help lsp` for the concise command help.

---

## Cache management

```
lunex set cache <dir>
lunex set cache reset
lunex cache
lunex cache clear
lunex memcache
lunex memcache clear
```

This is the runtime/adapter cache (compiled artifacts, embedded runtime
files), separate from the package module stores shown by `lunex env`.

---

## Environment variables

| Variable                | Description                                              |
|----------------------------|------------------------------------------------------------------|
| `LUNEX_DATA_DIR`           | Override the base Lunex data directory (default: `~/.lunex`)      |
| `LUNEX_RT_DIR`             | Override where the embedded runtime is extracted/cached           |
| `LUNEX_USE_CWD_CACHE`      | Set to `1` to use a cache directory relative to the current working directory instead of the home-based one |
| `NTL_DEBUG`                | Set automatically by `lunex debug`; set to `1` yourself to get the same verbose diagnostics from any command |
| `LUNEX_NATIVE`             | Set to `0` to disable the native loop JIT (default: enabled)      |
| `LUNEX_NATIVE_HOT`         | Loop/function hotness threshold before background native compilation (default: `32`) |
| `LUNEX_NATIVE_TRACE`       | Set to `1` to print JIT decisions to stderr                       |
| `LUNEX_NATIVE_VERIFY`      | Set to `0` to skip the compile-time native self-check (default: enabled) |
| `GOGC`                     | Explicit Go GC percentage; if unset, Lunex adjusts it adaptively  |
| `GOMEMLIMIT`               | Go memory limit (Lunex uses a `200MiB` default unless platform tuning applies) |
| `LUNEX_GC_ADAPTIVE`         | Set to `0` to disable adaptive garbage-collection tuning          |

There is no environment variable for enabling `std.ffi`. FFI activation is
handled only by the command-line prefix described above.

---

## Exit codes

| Code | Meaning                              |
|------|--------------------------------------|
| `0`  | Success                              |
| `1`  | Compile or runtime error             |
| `2`  | Usage error (bad flag or missing argument) |


## Runtime debugger

`lunex debug` runs a `.lx` source file or `.nax` application with source-aware Lunex call stacks, sampled
function profiling, runtime resource snapshots, and optional Go runtime
artifacts. Diagnostics are written to standard error; the program keeps its
normal standard output.

```bash
lunex debug app.lx
lunex debug app.lx --report debug-report.json
lunex debug app.lx --cpu-profile cpu.pprof --mem-profile heap.pprof
lunex debug app.lx --alloc-profile allocs.pprof --block-profile block.pprof --mutex-profile mutex.pprof
lunex debug app.lx --trace runtime.trace --goroutines goroutines.txt
lunex debug server.lx --pprof 127.0.0.1:6060
lunex debug app.lx --preserve-jit --cpu-profile production-cpu.pprof
lunex debug app.lx --report debug-report.json -- arg1 arg2
```

| Option | Description |
|---|---|
| `--cpu-profile <file>` | Capture a CPU profile that can be inspected with `go tool pprof`. |
| `--mem-profile <file>` | Capture the in-use Go heap profile. |
| `--alloc-profile <file>` | Capture cumulative allocation samples. |
| `--block-profile <file>` | Capture goroutine blocking data (sampling at approximately 1 ms). |
| `--mutex-profile <file>` | Capture contended mutex data. |
| `--trace <file>` | Capture scheduler, goroutine, and runtime events in a Go execution trace. |
| `--goroutines <file>` | Save all goroutine stacks when execution ends. |
| `--report <file>` | Save a JSON report with source SHA-256, compiler/runtime timings, error details, Lunex stack, function samples, and memory/GC metrics. |
| `--pprof <host:port>` | Serve live Go pprof endpoints and Lunex function/runtime snapshots. Only `localhost` and loopback IPs are accepted. |
| `--preserve-jit` | Preserve normal JIT execution. Optimized frames and source locations may be less precise; use this when reproducing JIT-only issues or profiling production-like execution. |
| `--profile-limit <n>` | Limit the displayed function profile to `n` entries. |
| `--no-profile` | Hide the function profile table. |
| `--no-color` | Disable ANSI colors in diagnostics. |
| `--` | End debugger options; forward the remaining arguments to the program. |

Function timings are sampled and weighted, so they are useful for locating
hot functions but are estimates rather than exact per-call measurements. By
default, diagnostic tracing bypasses JIT-optimized paths to preserve source
locations and call stacks. Use `--preserve-jit` to keep the production execution path;
optimized frames may then be missing from the Lunex call stack. CPU, heap, allocation, block, and mutex profiles are Go runtime profiles; they expose
runtime-level costs and can be analyzed separately from the Lunex function
profile. Block profiling samples waits at approximately 1 ms; mutex profiling
records contended locks. These instruments add overhead, so enable only the
profiles needed for an investigation. The JSON report
records both the source-aware Lunex stack and, when a Go panic occurs, the Go
stack. The optional pprof server remains local to the machine and exposes
`/debug/pprof/`, `/debug/lunex/functions`, and `/debug/lunex/runtime` while the
program is running. Use a local port-forward when diagnosing a remote process;
the debugger refuses public bind addresses.

## Global flags

| Flag               | Description                                              |
|--------------------|----------------------------------------------------------|
| `--debug`, `-d`    | Enable runtime diagnostic output                       |
| `--verbose`, `-V`  | Verbose debug output (implies `--debug`)                 |
| `--no-cache`       | Skip both disk and memory caches; force a fresh compile  |
| `--version`        | Print version and exit                                   |
| `--help`           | Print usage and exit                                     |

### Native FFI control

FFI is disabled by default. The switch is a command-line prefix and applies
only to the current Lunex process. It is not read from source code or from an
environment variable.

```bash
lunex ffi = on run main.lx
lunex ffi = off run main.lx
```

`ffi = on` is intentionally accepted only before the command. Values supplied
inside a Lunex program or through process environment variables do not change
the FFI state. Build scripts use `CGO_ENABLED=0`. Android arm64 builds use the Android Go
target, position-independent executable mode, and the Bionic ABI. The FFI backend
uses goffi through the purego-compatible layer and does not require a C compiler.

---
