# CLI Reference

Complete reference for the `lunex` command-line tool and its built-in package manager.

---

## Global flags

| Flag               | Description                                              |
|--------------------|----------------------------------------------------------|
| `--debug`, `-d`    | Enable debug output (AST, IR, and runtime traces)        |
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
lunex pack <file.lx|directory> [--source] [-o <output.nax>]
```

For a file, the default output is `<file>.nax`. For a directory, the default output is `<directory>.nax`. A directory pack requires a root `main.lx`.

Examples:

```bash
lunex pack main.lx
lunex pack main.lx -o dist/app.nax
lunex pack ./my-project
lunex pack ./my-project -o dist/my-project.nax
```

The complete lexer, parser, AST, resolver, checker, compiler, and local module graph are validated before archive publication. The default archive stores compiled NAX entries containing a binary representation of the validated AST rather than source text or Lunex VM bytecode. When any diagnostic is found, the command exits without creating or replacing the `.nax` artifact.

### `lunex unpack`

Extract a `.nax` archive into a new directory, named after the archive
file (e.g. `app.nax` extracts to `./app/`).

```
lunex unpack <file.nax>
```

There is no `-o` flag. The output directory name is always derived from
the input file. Compiled NAX entries recover readable `.lx` files from their stored AST; `--source` entries also preserve the original `.lx` source text.

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

List the available Lunex execution engine.

```
lunex runtimes
```

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
| `LUNEX_NATIVE_HOT`         | Loop iterations before native compilation (default: `32`)         |
| `LUNEX_NATIVE_TRACE`       | Set to `1` to print JIT decisions to stderr                       |
| `LUNEX_NATIVE_VERIFY`      | Set to `0` to skip the compile-time native self-check (default: enabled) |
| `GOGC`                     | Go GC percentage (Lunex sets `50` by default if unset)            |
| `GOMEMLIMIT`               | Go memory limit (Lunex sets `200MiB` by default if unset)         |

There is no environment variable for enabling `std.ffi`. FFI activation is
handled only by the command-line prefix described above.

---

## Exit codes

| Code | Meaning                              |
|------|--------------------------------------|
| `0`  | Success                              |
| `1`  | Compile or runtime error             |
| `2`  | Usage error (bad flag or missing argument) |
