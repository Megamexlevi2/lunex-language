# Contributing to Lunex

Thank you for your interest in contributing to Lunex. Contributions of all kinds are welcome: bug reports, feature suggestions, documentation improvements, and code changes.

---

## Reporting a bug

Before opening an issue, run `lunex check <file>` and `lunex --debug run <file>` to capture the full error output. Include the following in your report:

- Lunex version (`lunex version`)
- Operating system and architecture (`lunex platform`)
- A minimal `.lx` file that reproduces the problem
- The exact error message or unexpected output

---

## Suggesting a feature

Open an issue with the `enhancement` label. Describe:

- The problem you are trying to solve
- How you imagine it working in Lunex syntax
- Any similar feature in another language you found useful

---

## Making a code change

### 1. Fork and clone

```bash
git clone https://github.com/Megamexlevi2/lunex-language
cd lunex-language
```

### 2. Build

```bash
./build.sh
```

This produces a `lunex` binary in the project root. Use `build-termux.sh` for Android/Termux and `build-webasembly.sh` for a WebAssembly build.

### 3. Run the examples to verify nothing is broken

```bash
for f in examples/*.lx; do
  ./lunex run "$f" > /dev/null && echo "ok  $f" || echo "FAIL $f"
done
```

### 4. Make your changes

Key directories:

| Path | Contents |
|------|----------|
| `internal/lexer/` | Tokenizer |
| `internal/parser/` | AST builder |
| `internal/checker/` | Static checks |
| `internal/runtime/` | Tree-walking interpreter |
| `internal/bytecode/` | Archive compiler and VM |
| `internal/jit/` | JIT compilation |
| `internal/std/` | Standard library modules |
| `internal/errfmt/` | Error formatting and error codes |
| `internal/compiler/` | Source compiler pipeline |
| `internal/formatter/` | `lunex fmt` pretty-printer |
| `internal/lsp/` | Language server |
| `internal/pkg/` | Package manager (install/add/resolve) |
| `internal/resolver/` | Module/import resolution |
| `examples/` | Runnable example programs |
| `tests/` | Manual test scripts |
| `docs/` | Language reference, stdlib, CLI, and error documentation |

### 5. Style guidelines

- Follow the existing Go formatting conventions (`gofmt`).
- Keep error messages in English, consistent with the existing catalog in `internal/errfmt/`.
- When adding a new error code, add it to both `codes.go` (human title + suggestion) and `catalog.go` (runtime hint note).
- New standard library functions should be documented in the module's `register.go` or equivalent registration call.

### 6. Submit a pull request

Open a pull request against the `main` branch. Include a short description of what the change does and why. Reference any related issues.

---

## Project structure

```
lunex-language/
├── cmd/
│   ├── lunex/              CLI entry point
│   └── lunexwasm/          WebAssembly entry point
├── build.sh                Build script (local and release)
├── build-termux.sh         Build script for Android/Termux
├── build-webasembly.sh     Build script for WebAssembly
├── go.mod / go.sum         Go module files
├── internal/
│   ├── ast/                AST node definitions
│   ├── lexer/               Tokenizer
│   ├── parser/               Parser
│   ├── checker/              Static checks
│   ├── compiler/             Source compiler
│   ├── bytecode/             Archive format, VM, cache
│   ├── jit/                  JIT compilation
│   ├── runtime/              Tree-walking interpreter
│   ├── std/                  Standard library (one file per module)
│   ├── errfmt/                Error codes, formatting, catalog
│   ├── formatter/             Source code formatter
│   ├── lsp/                   Language server
│   ├── meta/                  Version metadata and integrity
│   ├── app/                   CLI command implementations
│   ├── pkg/                   Package manager (install/add/resolve)
│   ├── resolver/              Module/import resolution
│   ├── manifest/              lunex.toml / lunex.lock parsing
│   ├── buildfile/             Thin wrapper over manifest for build/start
│   ├── toml/                  Minimal TOML reader/writer
│   ├── semver/                Semantic version parsing and comparison
│   ├── adaptor/               Platform adaptor (cache paths, markers)
│   ├── firstrun/              First-run welcome animation
│   └── debug/                 Debug/verbose logging
├── examples/                Runnable example programs
├── tests/                   Manual test scripts
├── docs/                    Language reference, stdlib, CLI, and error docs
├── lunex-cli/               CLI helper module (packaged Lunex library)
├── lune-xml/                XML module (packaged Lunex library)
├── LICENSE
├── CONTRIBUTING.md
├── README.md
└── modulesys.md             Module system reference
```

---

## License

By contributing, you agree that your contributions will be licensed under the [Mozilla Public License 2.0](LICENSE).
