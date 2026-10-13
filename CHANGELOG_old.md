# Lunex changelog

> **Heads-up:** this is the old changelog, and I'm not going to modify it anymore. I never kept a changelog while I was building these releases, so I went back and read through the versions one by one and wrote everything up after the fact, newest release first and oldest last. I didn't write one for the versions between 0.6.0 and 0.7.3, so there's no entry covering that range. This one is done and won't be touched again.

---

## 0.9.0 → 0.9.2

This covers everything that changed between the 0.9.0 and 0.9.2 releases, including whatever went out in between. It's a long one, mostly because three things I'd been putting off all landed together: a real dependency system, a real `lunex check`, and a database that actually keeps your data. Sorry for the wall of text, but there's a lot to go over.

### The headline: `lunex.toml` and a proper package manager

`config.lx` is gone. Projects are now described by a `lunex.toml`, the same way you'd expect from Cargo or npm:

```toml
[project]
name = "my-app"
version = "1.0.0"
description = "My Lunex application"
entry = "main.lx"

[lunex]
min_version = "0.9.2"

[libraries.http_client]
url = "https://github.com/user/http-client"
version = "1.4.0"

[libraries.auth]
url = "https://github.com/user/auth"
version = ">=1.0.0 <2.0.0"

[libraries.test]
source = "local"
path = "./modules/test"
```

Next to it there's a `lunex.lock`, which Lunex writes for you. It pins the exact version, source and `sha256` hash of every installed library so a build comes out the same on another machine. Don't edit it by hand.

A library can come from a few places:

- a GitHub repo, at its default branch, a tag, an exact version, `latest`, or a version range (there's a small semver parser behind this now)
- a GitHub release, downloaded as a release asset (`source = "github-release"`, `release = "v2.1.0"`)
- a subdirectory of a repo (`path = "modules/ui"`)
- a folder on disk (`source = "local"`)

#### Global and local installs

Installed libraries used to live in `~/.lunex/cache`. Now there are two stores, and every version gets its own `<name>@<version>` directory, so two projects (or two dependencies of the same project) can use different versions of the same library without stepping on each other:

| Store  | Location           | Scope                       |
|--------|--------------------|-----------------------------|
| Global | `~/.lunex/modules` | every project on the machine |
| Local  | `./.lunex/modules` | this project only           |

`@import("name")` looks in the standard library first, then the local store (honoring the version in `lunex.lock`), then the global one. If nothing matches you get a hint that tells you to run `lunex add`, instead of the old "install it with `lunex install`" line.

#### Commands

```bash
lunex install                      # install everything in lunex.toml
lunex install -g <url>[@version]   # install one library globally, no lunex.toml needed
lunex install -l <url>[@version]   # install one library locally
lunex add <url>[@version]          # add it to lunex.toml and install it
lunex remove <library>             # remove it from both stores
lunex update [library]             # re-resolve one or all libraries
lunex list                         # now shows (local) or (global) next to each library
lunex env                          # store paths, how many versions are installed, lunex.toml/lunex.lock status
lunex link                         # expose this project's commands globally
```

`install` and `add` used to be the same command. They aren't anymore: `install` is for restoring what's declared (or a one-off `-g`/`-l`), `add` is for putting a new dependency in `lunex.toml`.

#### Commands that ship with a library (`bin`)

Same idea as `bin` in `package.json`:

```toml
[project.bin]
build = "./bin/build.lx"
serve = "./bin/serve.lx"
```

When a library that declares `bin` gets installed, Lunex writes a small shim for each command into `~/.lunex/bin` (global) or `./.lunex/bin` (local). Put `~/.lunex/bin` on your `PATH` and you can run them directly. `lunex link` does the same for the project you're working on without installing it first, and `lunex remove` cleans the shims up again.

#### Everything that reads the manifest moved too

`lunex init` now creates `lunex.toml` (plus `main.lx`, `src/math.lx` and a `.gitignore`), `lunex start` runs the `entry` from `lunex.toml`, and `lunex build` with no arguments reads it too. The old `.lunex/cache/modules` scan inside the module loader is removed, since resolution goes through the package manager now.

`min_version` and `max_version` under `[lunex]` are read and saved, but nothing checks them against the running version yet. They're there for when that lands.

### `lunex check` was rewritten

In 0.9.0 `lunex check` was basically a parse plus three little checks: an `each` with no iterable, an empty computed index, and an empty import path. It looked at one file and that was it.

It's now a real static checker (`internal/checker`) that never runs your code. It follows `@import` and `@fimport` into other `.lx` and `.nax` files, local or installed, and checks the whole graph:

- names and scopes: unresolved variables, duplicate declarations, duplicate parameters, unknown types
- function arity
- assignments to `val` / `const` bindings
- members that don't exist on a module, including standard library modules (`math.missing(1, 2)` is caught)
- export and import consistency
- `break` / `continue` outside a loop, `return` outside a function, statements at the top level
- import cycles
- the `main` entry point

Errors look like compiler output now, with a code, a location, the source line, notes and a help line:

```
[E0428]: the name "add" is defined multiple times
  --> main.lx:7:1
  |
     7 | fn add(a, b) { a + b }
  |     ^
  = help: rename one declaration or remove the duplicate
```

It exits with `1` on any error and prints `Finished checking main.lx: no errors` when it's clean, instead of the old `main.lx: ok`. Add `--debug` for checker phase tracing and `--verbose` for resolver and symbol details. The codes it uses are `E0061`, `E0071`, `E0268`, `E0400`–`E0403`, `E0412`, `E0415`, `E0425`, `E0428`, `E0432`, `E0572`, `E0594`, `E0599` and `E0601`; the full table is in `docs/errors.md`.

There's also a new `lunex debug <file>`. It compiles with every error reported (not just the first), and if that works it runs the file with debug mode on so each step and any runtime error comes with a full trace. It's what I kept wishing `lunex run` did when something was off.

### Project templates

```bash
lunex init http_server my-api
lunex init database my-data
lunex init website my-site
```

`http_server` gives you a small REST server with JSON routes, `database` a SQLite-backed project, and `website` a static site. `lunex init <name>` without a template still works and the output now points you at `lunex start` and `lunex add`.

### `std.db` is SQLite now

This is the one I'm happiest to get rid of. `std.db` used to be an in-memory store, so everything you inserted was gone the moment the process exited. It's now backed by a real SQLite file, written to `.lunex/data/<name>.db` the first time you use it:

```lx
val db = @import("std.db")

val users = db.table("users")
users.insert({ name: "Ada" })
```

Run that twice and the second run still sees Ada.

The API you already know is the same (`table`, `find`, `where`, `orderBy`, `aggregate`, `schema`, indexes, `watch` and so on), only the storage changed. Documents are stored as JSON in ordinary SQLite tables. A few things worth knowing:

- It uses `modernc.org/sqlite`, a pure Go driver, so there's still no CGo and no C compiler involved.
- Databases open in WAL mode with a 5 second busy timeout and foreign keys on.
- `db.open(name)` / `db.create(name)` / `db.connect(name)` give you a named database, `db.list()` lists the ones on disk and `db.drop(name)` deletes the file (and its `-wal` / `-shm` files).
- Field names that end up inside SQL fragments are validated against a strict pattern. Row values always go through bound parameters.
- `database.transaction(fn)` just runs `fn(database)` and returns the result. It does not roll anything back if `fn` fails. I'd rather say that out loud than have you assume it does.
- `table.join(...)` still doesn't join anything.

### `std.buffer` and `std.ints`

Two new modules for when you're dealing with bytes: binary formats, protocols, checksums.

```lx
val buffer = @import("std.buffer")
val ints   = @import("std.ints")

val buf = buffer.alloc(8)
buf.writeU32(0, 4021, "be")
buf.writeI16(4, -12, "be")
io.log(buf.readU32(0, "be"))   // 4021
io.log(buf.toHex())

ints.addU8(250, 10)            // 4, wraps at 256
ints.mulU32(200000, 200000)    // wraps like a C uint32_t
```

`std.buffer` is a mutable block of bytes. You create one with `alloc`, `from` (string, hex, base64 or an array of bytes) or `concat`, and read and write `U8`/`I8`/`U16`/`I16`/`U32`/`I32`/`U64`/`I64`/`F32`/`F64` at a byte offset, little endian by default or `"be"` if you ask. It also has `slice`, `copyFrom`, `fill`, `resize`, and `toHex` / `toBase64` / `toString` / `toArray`. Reading or writing outside the buffer does nothing instead of throwing, and reads return `0`.

`std.ints` exists because Lunex numbers are 64-bit floats and don't wrap. It has casts (`u8` through `i64`), wrapping `add` / `sub` / `mul` for every width, shifts and rotations (`shlU32`, `rotlU32`, ...), overflow checks and the `*_MAX` / `*_MIN` constants. There's a new `examples/21_buffers_ints.lx` that uses both.

Both are known to `lunex check`, so member typos on them get caught.

### HTTP server fixes

Two things were broken in `std.http` servers:

- **`server.use(fn)` didn't run your function.** In 0.9.0 only the static-files descriptor from `http.serveStatic` was honored by `use`; a function handler was registered and then skipped. Now middleware runs in the order you registered it, before the matching route handler, and it stops as soon as a response has been sent. Static directories take part in the same ordering. There's a test for it (`tests/stdlib/http/03_http_middleware_order.lx`).
- **The root path was split wrong.** An empty path turned into one empty segment, which could make `/` mismatch against param and wildcard patterns. It's an empty list now.

Middleware handlers are called with `(req, res)`. There's no `next` argument yet.

### Smaller fixes

- **`>>>` works.** The lexer tried `>>` before `>>>`, so an unsigned right shift was read as `>>` followed by `>`. The operators are in the right order now, and `-1 >>> 28` gives what you'd expect.
- **`spawn` and variable scopes.** Scopes are protected by a read/write lock now, and `spawn` marks the scope it's in as escaped so it isn't handed back to the pool while the spawned function is still using it. This was the kind of bug that only shows up under load, so I'd rather it be boring.
- **`os.exec` / `os.spawn` find programs on `PATH` themselves.** If the program isn't there, `os.exec` returns `{ ok: false, code: 127, stderr: "os.exec: ... not found in $PATH" }` and `os.spawn` throws a matching error. Before, you got whatever Go's exec happened to say. When a command fails to even start, `stderr` now carries the reason instead of coming back empty.
- **bcrypt in `std.crypto`** now uses `golang.org/x/crypto/bcrypt` instead of the Blowfish implementation I'd written by hand. Several hundred lines of my own crypto code are gone, which is the right trade. The cost is clamped to bcrypt's allowed range.
- **`lunex build` and `lunex start`** report `no lunex.toml found` when there isn't one.
- **`lunex init`'s example `src/math.lx`** was missing the `//` on its first comment line, so the generated module didn't parse. Fixed.

### Docs

A good chunk of this release is documentation catching up with what the code actually does. If you've been confused by a page, it was probably wrong, not you.

- `docs/errors.md` was rewritten from scratch. It lists the codes the build really emits, grouped by where they come from, and explains that `lunex run` and `lunex check` use two different numbering schemes (with `E0061` as the one place they collide). It also covers the `S0001`–`S0007` runtime warnings.
- `docs/language-reference.md` documents the full keyword list, `elif`, `try` / `catch` / `finally`, `for`, `repeat` and `loop`, the bitwise operators, `const` and `let` as aliases of `val` and `var`, and the JavaScript-style globals, with an honest list of their limits (`Promise.all` doesn't wait for anything, `clearTimeout` is a no-op, `process.env` and `process.argv` are always empty).
- `docs/cli.md`, `modulesys.md` and the README cover everything in the package manager section above.
- `docs/stdlib.md` got corrections where the old text was just wrong:
  - `io.err` and `io.warn` write to stderr and only add color, not an `[ERROR]` / `[WARN]` prefix
  - `io.hr` takes `(len?, char?)`, `io.color` takes `(name, text)`, and the spinner is `io.spinner()` with `tick(message)`
  - `math.randomInt` and `utils.randInt` treat the upper bound as exclusive
  - `regex.replace` replaces every match, same as `replaceAll`
  - `jwt.verify` always returns `{ valid, payload }` or `{ valid: false, error }`, while `crypto.jwt.verify` returns the payload or `null`
  - `runtime.typeOf` and `runtime.gc` don't exist, and neither does `table.all()` in `std.db` (use `find()`)
  - `os.exec` doesn't run a shell, and `ws.connect` only supports `ws://`
  - `std.http`, `std.ws`, `std.fs`, `std.os`, `std.datetime`, `std.math`, `std.utils`, `std.regex` and `std.env` now list the functions they really have, and `std.json` is in the module table

### Cleanup and build

- **Go version** — `go.mod` now needs Go 1.25.0 (it was 1.23) and the README says 1.25 or later. Heads up: `build.sh`, the `Makefile` and `CMakeLists.txt` still say 1.23 for their minimum and for the toolchain they download. I haven't gotten to them yet, so on a machine without a recent Go, build with a toolchain you installed yourself for now.
- **New dependencies** — `golang.org/x/crypto` and `modernc.org/sqlite`, plus the packages SQLite pulls in.
- **`./build.sh test` actually tests now.** It was looking for `tests/run_tests.sh`, which doesn't exist, so it skipped the tests and said everything passed. It runs `tests/run_all.sh` against the binary it just built and fails if anything fails. `run_all.sh` itself is a bash script now.
- **Tests** — new suites for `stdlib/db`, `stdlib/http`, `stdlib/ints` and `stdlib/buffer` (15 files in total), all picked up by `run_all.sh`.
- **`lune-xml` and `lunex-cli`** each got a `lunex.toml` (`min_version = "0.9.2"`), and the `lune-xml` README now says `lunex.toml` instead of `config.lx`.
- **Removed** the old `semanticCheck` code, a stale copy of the CLI entry point at the repo root (`main.go`, the real one is `cmd/lunex`), two empty files under `internal/bytecode`, and a batch of unused helpers from `internal/debug`.

### Breaking changes, summarized

If you're upgrading from 0.9.0, here's what will actually bite you:

1. `config.lx` isn't read anymore. Create a `lunex.toml` (`lunex init` in a fresh folder shows the layout), or `lunex build` and `lunex start` will stop with `no lunex.toml found`.
2. Libraries from the old `~/.lunex/cache` aren't picked up. Reinstall them with `lunex add` or `lunex install`.
3. `lunex install <url>` on its own is now an error. Use `lunex add <url>` for a project dependency or `lunex install -g` / `-l` for a single library. `lunex add` also needs a `lunex.toml` in the current folder, and `lunex update` skips libraries that don't have an entry there.
4. `std.db` data is on disk now. A test or script that assumed an empty table at the start will see the rows from the last run, so `clear()` first if you need a clean slate. Add `.lunex/` to your `.gitignore` (`lunex init` already does).
5. `lunex check` is much stricter. A file that passed before can fail now, with unresolved names, wrong argument counts, missing module members or import cycles, and the exit code is `1`.
6. `server.use(fn)` handlers now actually run. If you registered some that were being ignored, they'll start executing before your routes.
7. You need Go 1.25 or newer to build from source.

That's it for this one. As always, open an issue if something breaks in a way that isn't mentioned above. There's a decent chance it's a doc gap and not on purpose.

---

## 0.8.1 → 0.9.0

This one is mostly about structure. There's no big new syntax this time. I moved the CLI out of the repo root, dropped the `.nc` format so everything compiled is `.nax`, removed the JIT package, and added a hard limit on how much work a single run can do. The README, docs and build scripts were updated to match.

### The CLI moved into `internal/app`

`main.go` used to hold the whole CLI. Now the code lives in `internal/app/app.go`, and the entry point is a tiny file:

```go
// cmd/lunex/main.go
package main

import app "lunex/internal/app"

func main() {
	app.Run()
}
```

`bundle.go` and `embed.go` moved into `internal/app` too, and so did `version.json`. `build.sh` and `build-termux.sh` read the version from the new path and build `./cmd/lunex` instead of `.`:

```bash
go build -trimpath -tags netgo -ldflags="-s -w" -o lunex ./cmd/lunex
```

If you have your own scripts that run `go build .` from the repo root, point them at `./cmd/lunex`.

### Build with Make or CMake

You don't have to go through `build.sh` anymore. There's a `Makefile` and a `CMakeLists.txt`, and both are documented in the README:

```bash
make
```

```bash
cmake -B build
cmake --build build
```

Both detect your OS and architecture (Linux, macOS, Windows, FreeBSD and Android; `amd64`, `arm64` and `riscv64`), build with `CGO_ENABLED=0`, and download Go into `~/.local/bin` if it's missing or older than 1.23. You can override the target:

```bash
make build OS=linux ARCH=arm64
```

`make release` (or the `release` target in CMake, which uses `cmake/BuildRelease.cmake`) builds seven binaries into `release/`: Linux amd64 and arm64, Windows amd64, macOS amd64 and arm64, Android arm64 and FreeBSD amd64. There's also `make test`, `make clean` and `make help`.

### `.nc` is gone, everything is `.nax`

Before this release there were two compiled formats: `.nc` for a single file and `.nax` for an archive. Now there's one. The container format itself is the same; what changed is the names and the extension:

- `lunex run` accepts `.lx` and `.nax`. It doesn't accept `.nc` anymore.
- `lunex build` writes `<input>.nax` by default. `--format nax` is still accepted.
- `lunex dis` takes a `.nax` file.
- `@fimport` tries `.lx` and `.nax` when you leave the extension off, and no longer tries `.nc`.
- Entries inside a packed archive are stored as `.nax` objects instead of `.nc`.
- `lunex init` puts `*.nax` in the generated `.gitignore` instead of `*.nc`.
- The on-disk cache uses `.nax` file names, and `lunex cache` counts those. Cache files left over from 0.8.1 are not reused or counted, so you can delete them by hand.

On the Go side, `EncodeNC`, `DecodeNC`, `RunNC` and `RunNCFile` became `EncodeObject`, `DecodeObject`, `RunObject` and `RunObjectFile`, and `LoadNCAsModule` was removed. `lunex-cli` doesn't treat `.nc` as a file extension anymore either.

### The JIT package is removed

`internal/jit` was a profiler plus a few pure-Go loop fast paths. The only thing the interpreter actually used from it was the call counter, so I took the package out and kept a plain version of the counter in `internal/runtime/profiler.go`. It records calls and average time per function and prints the same profile report. Functions are no longer promoted to a "hot" tier.

What that removes from the surface:

- `lunex jitcache` and `lunex jitcache clear` don't exist anymore, and the JIT cache directory is gone.
- The JIT error codes `5001`–`5003` (can't allocate executable memory, unsupported CPU, code generation failed) were removed.
- `E0065` is documented as `TimeoutError` now. It used to be listed as `JITCompileError`. `E0062` is `ArchiveVersionMismatch` instead of `BytecodeVersionMismatch`.
- The first-run screen no longer has a "Configuring JIT runtime" step.
- `lunex runtimes` and `lunex platform` only list the interpreter and the archive loader, and the debug trace has no `[jit]` lines.

I also removed two packages that nothing referenced: `internal/bootstrap` (the old Luna installer) and `internal/lunaresolver`. The loose `test.lx` and `test1.lx` files in the repo root are gone too.

### A run can't use unlimited steps anymore

The interpreter now counts every statement and every expression it evaluates, and stops once a run goes past 2,000,000 of them:

```lx
fn main() {
  while true {
  }
}
```

That used to hang forever. Now it ends with a timeout error:

```
execution budget exceeded after 2000000 steps; possible infinite loop, runaway recursion, or unbounded spawn
```

The counter resets at the start of each program run, each module run and each `CallExport`. The limit is fixed at 2,000,000 in this release and there's no option to change it yet. For normal scripts you'll never notice it. A program that legitimately does more work than that in one run will hit it, so keep that in mind for long loops and processing jobs.

### Printing circular values

If an object or array contains itself, printing it used to recurse until it blew up. `ToString` and `Inspect` now keep track of what they've already visited and print `[Circular]` (or `{ Circular }` for objects) when they run into a loop. Missing entries inside arrays and objects print as `undefined` now instead of being dereferenced.

### `let` and `const` are no longer keywords

The lexer's keyword list now has `var` and `val` only. `let` and `const` were never in the language reference and none of the examples use them, so they're out. If you wrote `let x = 1` or `const x = 1`, switch to `var` and `val`.

### Smaller stuff

- **README** — new tagline ("a fast, stable scripting language for backend development"), the "still evolving" note is gone, the clone instructions use the `lunex-language` folder name instead of `lunex-lang-gz`, the CLI list describes `lunex build` as "compile source or create a .nax archive", and the project layout shows `cmd/lunex` and no `jit`. `CONTRIBUTING.md` got the same layout update.
- **Docs** — `docs/cli.md` and `docs/errors.md` describe the `.nax`-only flow. The language reference, the stdlib reference and `examples/README.md` say 0.9.0 now. They still said 0.8.1.
- **Tests** — `tests/test_jit.lx` is described as `test_timing.lx` in the test README (a `std.utils.now()` sanity check), and `tests/test_timing.lx` was added. The banner in `tests/run_all.sh` says "Lunex Test Suite" without a version number. It used to say v0.7.1.
- **Cleanup** — header comments were removed from the Go sources, the examples, the tests, `lune-xml` and `lunex-cli`.
- **Go version** — `go.mod` still requires Go 1.23. `version.json` still says `minGo` 1.23.

### Breaking changes, summarized

If you're upgrading from 0.8.1, here's what can actually bite you:

1. `.nc` files aren't supported anymore. `lunex run`, `lunex dis` and `@fimport` only work with `.lx` and `.nax`, and `lunex build` writes `.nax` by default.
2. Archives you packed with 0.8.1 store their entries as `.nc`, so rebuild them with 0.9.0.
3. `lunex jitcache` doesn't exist anymore.
4. A single run is capped at 2,000,000 steps. Anything past that stops with a timeout error.
5. `let` and `const` don't work as declarations anymore. Use `var` and `val`.
6. Build from `./cmd/lunex`, not from the repo root.

Same as always: if something breaks that isn't listed here, open an issue. It's probably a doc gap and not on purpose.

---

## 0.7.3 → 0.8.1

This one covers everything between 0.7.3 and 0.8.1. There's no big new module and no new keyword this time, but a lot moved underneath: the package commands were reworked, the parser now refuses reserved words as names, `ws.connect` stopped being a stub, `lunex run` can finally hand arguments to your script, and the 127 KB interpreter file is split into pieces you can actually navigate. I also went through the docs and fixed the places where they described an API the code doesn't have. It's a long list, so let's get into it.

### `lunex run` passes arguments to your script

```bash
lunex run main.lx build --fast out/
```

```lx
val os = @import("std.os")
val io = @import("std.io")

fn main() {
  io.log(os.args())
}
```

`os.args()` now returns the absolute path of the script as the first item, followed by everything you typed after the file name. It works for `.lx`, `.nc` and `.nax` files.

In 0.7.3 `lunex run` only understood `--emit` and threw `unknown flag` at everything else, so there was simply no way to get arguments into a script. Now `--emit` (and `--emit=`) is still handled by the CLI, the first thing that isn't one of those starts the script's arguments, and a bare `--` sends everything after it straight to your script.

One side effect: a typo like `--emti` used to be rejected, and now it just shows up in `os.args()`. I think that's the right trade, but you should know about it.

### Package commands: stricter, and no more fake installs

```bash
lunex install github.com/user/repo
lunex install github.com/user/repo@v1.2.0
lunex install github.com/user/repo/subdir

lunex update              # every installed package
lunex update lune-xml     # just this one
lunex remove lune-xml other-pkg
lunex list
```

I wasn't happy with how loose these were, so here's what changed:

`lunex install`, `lunex i` and `lunex add` are one command now. It needs at least one spec. Running it bare used to install everything listed in `config.lx`; now it prints a usage line and exits with 1. If there's a `config.lx` in the directory you ran it from, the package is recorded there after it installs.

Specs have to point at GitHub: `github.com/owner/repo`, optionally with an `@ref` and/or a subdirectory, or the full `https://github.com/...` form. This is the part of 0.7.3 I'm least proud of. Anything without a slash in it (`lunex install foo`) went down a path that never downloaded anything. It wrote a metadata file pointing at an `index.nax` that didn't exist and then cheerfully printed `installed foo@latest`. That now fails with `invalid package spec ... expected github.com/user/repo or https://github.com/user/repo`.

`lunex update` no longer removes a package and then reinstalls it, which meant a failed download left you with nothing. It reinstalls from the source recorded at install time, for every installed package or only the ones you name, prints `updated <name>` for each, and exits with 1 on the first failure. It doesn't read `config.lx` anymore.

`lunex remove` accepts several names and exits with 1 when something fails. Before, it printed the error and carried on with exit code 0, which made it useless in scripts.

`lunex list` prints one `name@version` per line. No more table header.

Two new things on top of that:

- Every successful install now writes a small launcher to `~/.lunex/bin/<package-name>`. It's a plain `/bin/sh` script that runs `lunex run <package entry> "$@"`, using the absolute path of the `lunex` binary that did the install, so it's really aimed at Linux, macOS and Termux. I don't touch your `PATH`, so add `~/.lunex/bin` yourself if you want installed packages to behave like commands. Failing to write the launcher doesn't fail the install.
- `@import` of a package that isn't installed prints a hint with the fix: `package "x" not found — install it with: lunex install x`. Packages also go through a file-aware loader now, so Lunex knows the real path of a package file and errors inside it point at the right place.

While I was in here I removed the lines from the README, `modulesys.md` and `lunex help` that said `@import("github.com/user/repo")` or `@import("https://...")` would install a package on first use. I couldn't find any code in 0.7.3 that did that, so it was a promise the language never kept.

A heads-up: the docs now say packages live in `~/.lunex/cache/`, but the code still keeps the cache in `.lunex/cache` inside the directory you run Lunex from. They disagree and I haven't touched the behavior, so trust the code for now.

### Reserved words can't be names anymore

```lx
val range = 10        // E0073
fn select(c) { c }    // E0073
fn run(use) { use }   // E0076
```

All three used to slip through and then cause confusing problems later, because those words mean something to the lexer. Now the parser stops right there with a message like:

```
reserved keyword 'select' cannot be used as a function name — choose a different name (e.g. 'select_fn' or 'my_select')
```

Variables and parameters are checked against a "hard" list: the control-flow and declaration words (`val`, `var`, `fn`, `if`, `match`, `try`, `return`, `true`, `null`, `this`, `typeof`, `import`, ...) plus the Lunex-specific ones (`range`, `sleep`, `use`, `loop`, `when`, `between`, `matches`, `startsWith`, `endsWith`, `select`, `channel`, `spawn`, `delete`, `assert`, `with`, `freeze`, `macro`, `nax`, `lunex`, `unless`, `guard`, `defer`, `struct`, `each`, ...). Words that only act like keywords in some positions, such as `get`, `set`, `from`, `as`, `of`, `in`, `type`, `default`, `async`, `await`, `yield`, `enum` and `alias`, are still fine as names. Function names are checked a bit more strictly: any keyword that isn't on that "still fine" list is rejected.

I hit this myself. `tests/structs/08_struct_builder.lx` had a builder method called `select`, and it's now `select_fn`. If you have a variable called `range` or `use` somewhere, expect to rename it.

The new codes are `E0073` (reserved keyword as a name), `E0076` (as a parameter name) and `E0077` (as a field name). To be upfront about it: `E0073` through `E0079` are all in the error catalog now, but the parser currently only raises `E0073` and `E0076`. `E0074`, `E0075`, `E0077`, `E0078` and `E0079` are registered and nothing raises them yet.

### Better errors

**Unclosed blocks point at the opening brace.** Forget a `}` and you now get `E0052`, "unclosed block", telling you which line the `{` was opened on, with the underline on that brace and a suggestion to add the matching `}`.

**`+` with a string and `null` / `undefined`** has its own message now:

```lx
var label = null
io.log("Label: " + label)
```

Instead of the generic "cannot combine" message you get one that says the right-hand operand is `null` and asks whether you forgot to initialize a variable. `+=` has its own wording for the same situation. Other mismatches (say, a string plus a boolean) keep a clearer generic message that mentions `str(x)` and `Number(x)`.

**Functions remember which file they came from.** Every function now carries the file name and source lines of where it was defined, and the interpreter swaps to them while the function runs. Module loading also restores the source lines and the current line and column when it's done. In practice the error window points at the file the failing code actually lives in, even when the function was imported from somewhere else.

Smaller error things:

- A handful of titles in the error catalog got shorter and clearer (`E0002` is just "undefined method" now, `E0004` is "null or undefined access", `E0060` is "stack overflow").
- The messages for top-level `main()` calls (`E0072`) and top-level assignments (`E0071`) were reworded.
- All the error constants now live in `errfmt.go`. `codes.go` is an empty file on purpose, kept so nothing that points at it breaks. There are new `errfmt.LookupCode`, `CodeTitle` and `CodeSuggestion` helpers.
- The expression evaluator now also handles `if` and `unless` nodes, so they can produce a value in more places than before.

Something worth knowing: `docs/errors.md` still describes the older numbering table, and it doesn't match the catalog in the code (the doc calls `E0021` "UndefinedVariable", the code calls it "wrong number of arguments"). I haven't reconciled those yet, so if a code looks wrong, believe the code.

### `ws.connect` is real now

In 0.7.3, `ws.connect(url)` returned an object whose `send` and `close` did nothing at all. It looked like it worked right up until you wondered why nothing was arriving. Now it opens a TCP connection, does the HTTP upgrade handshake, checks for a `101` response, starts a read loop and gives you back the usual connection object:

```lx
val ws = @import("std.ws")
val io = @import("std.io")

val client = ws.connect("ws://localhost:8081")

client.onMessage(fn(msg) {
  io.log("server said: " + msg)
})
client.onClose(fn() {
  io.log("connection closed")
})

client.send("hello")
client.close()
```

`send`, `close`, `onMessage`, `onClose` and `isClosed` are all there. Objects and arrays passed to `send` get serialized to JSON for you.

It's not finished, and I'd rather say so here than have you find out the hard way:

- `wss://` isn't supported. It returns an error saying so. Use `ws://` for now.
- The `Sec-WebSocket-Key` in the handshake is a fixed value, not a random one.
- Frames sent by the client aren't masked. Lunex's own `ws.createServer` is happy with that, but a strict server may drop the connection.

Anything other than `ws://` or `wss://` fails with `unsupported scheme`.

### The interpreter, in pieces

`internal/runtime/interpreter.go` was one 127 KB file and getting hard to work in. It's split up now: `interp.go` (the interpreter struct and setup), `eval.go`, `exec.go`, `functions.go`, `declarations.go`, `classes.go`, `collections.go`, `control.go`, `literals.go`, `modules.go`, `builtins.go`, `helpers.go`, `misc.go`, `json.go` and `uri.go`.

It's meant as a pure move, and no functions were dropped. Only a handful of them changed behavior while they were being moved, and those are all described elsewhere in this changelog: the arithmetic error messages, the source-file tracking, module and package loading, and the `if` / `unless` handling.

The helpers that `json`, `crypto`, `fs`, `http`, `io`, `ws`, `db`, `jwt`, `utils` and `native` used to share by living in the same package (deep copy and deep equal, JSON conversion, type names, UUIDs, flatten, argument printing) moved into a new `internal/std/shared` package. Nothing changes on the Lunex side. It just stops every module from reaching into `json.go` for its tools.

### `lune-xml` got a short API

`lune-xml/main.lx` has a set of shorter helpers for the common cases, with the full API still underneath:

```lx
val xml = @fimport("./lune-xml/main.lx")

val title = xml.tag("title", "Lunex Programming")
val book  = xml.set(xml.tag("book"), "id", "1")
xml.add(book, title)

io.log(xml.toString(book))

val doc  = xml.fromString("<users><user id=\"42\"><name>Alice</name></user></users>")
val user = xml.find(doc, "user")
io.log(xml.attr(user, "id"))   // 42
io.log(xml.get(xml.find(doc, "name")))   // Alice
```

- `xml.tag(name)` and `xml.tag(name, text)` create elements.
- `xml.set(node, key, value)` sets one attribute and returns the node. `xml.setAll(node, attrs)` does a whole object, and it sets the keys in alphabetical order.
- `xml.add(parent, ...)` appends children and returns the parent. It takes up to five children per call.
- `xml.get(node)` returns the text, `xml.toString(root)` serializes, `xml.fromString(src)` parses.
- `xml.find(root, tag)` and `xml.findAll(root, tag)` are the quick queries.

The README is rewritten around a "Beginner Guide" that uses these. Internally, every parameter that was called `tag` is now `tag_` so it doesn't shadow the new `tag` function.

Important caveat: `lune-xml/main.nax` was **not** rebuilt. It's byte-for-byte the same file as in 0.7.3, so the new helpers only exist if you import `main.lx`. The README still says the `.nax` is the recommended way to import it, which is no longer true if you want the short API. I'll rebuild it next time.

### Docs, fixed against the code

This is where I found the most embarrassing mismatches. Things that were documented one way and implemented another:

- **HTTP server.** `stdlib.md` showed `http.listen(8080)` with `server.get(...)`, `server.post(...)` and `res.send(...)`. The module actually works like this: `http.createServer(handler)`, then `http.listen(server, port, host, onReady?)`, with `http.text(res, text, status)` and `http.json(res, value, status)` for responses, and a request shaped like `{ method, url, headers, body }`. Rewritten with a real example.
- **Database.** The docs listed flat functions like `db.insert(table, record)`. The real API is `db.table(name)`, which gives you a table object with `.define()`, `.insert()`, `.find()`, `.findOne()`, `.update()`, `.delete()`, `.all()`, `.count()` and `.clear()`. There's a complete example now, including a schema.
- **`crypto.hmac`** takes `(algo, key, data)`. The docs said it only did HMAC-SHA256 with two arguments. Added short examples for `sha256`, `hmac`, `encrypt` and `decrypt` too.
- **`os.cwd()`** is what the docs use now (`getcwd` still exists as the original name).
- **Datetime** values list their fields (`iso`, `year`, `month`, `day`, `unix`, `timestamp`), and the format and convert sections were regrouped.
- **Structs.** The README and language reference now show the factory pattern with a captured `self` (`val self = struct { ... }` and then `self` as the last line) and inline `struct { ... }` values, instead of leaning on `this`. The line about `this` being the implicit receiver is gone from the language reference.
- **README.** Removed `lunex fmt <file>` from the CLI list because there's no such command. Added a proper Package Management section, reworded the intro, and updated the repository links.
- **`modulesys.md`.** `@fimport` is documented for both `.lx` and `.nax` files, and the install section is shorter and only shows what works.
- **`lunex-cli` README.** Import it with `@fimport("./lunex-cli/main.lx")` and run its tests with `lunex run lunex-cli/test/run_all.lx`.
- Version numbers in `language-reference.md`, `stdlib.md` and `examples/README.md` said 0.7.1. They say 0.8.1 now.

Still off, as far as I can tell:

- The WebSocket section of `stdlib.md` shows `ws.listen(...)` and `.on("message", ...)`, but the module exposes `ws.createServer` and `client.onMessage(...)`.
- The `errors.md` table, as mentioned above.
- The package cache location, also mentioned above.

### Smaller stuff

- **`go.mod` and `go.sum`** were trimmed. 0.7.3 required `pgx`, `golang.org/x/arch`, `x/crypto`, `x/sys`, `x/sync`, `x/text` and a few indirect ones, and none of them were imported by any file. The only dependency left is `github.com/golang-jwt/jwt/v5`.
- **Repository moved.** Links in `version.json`, the README, `CONTRIBUTING.md`, `errors.md` and `lune-xml/config.lx` now point at `Megamexlevi2/lunex-language` instead of `lunex-lang-gz`. A few test file headers still have the old name.
- **`version.json`** has the new version, a build date of 2026-06-25 and a slightly reworded description.
- **Housekeeping.** I removed the "Created by David Dev" banner comments from the Go files and build scripts, cleared out a lot of old comments, and reformatted several files (`lexer.go`, `ast.go`, `errfmt.go` and others). Some diffs look huge because of that and are really not.
- **Three packages are in the tree but not wired in yet:** `internal/bootstrap` (an installer for a separate "Luna" package manager, which downloads `luna-pm` from GitHub into `~/.luna` and writes a `luna` shim), `internal/lunaresolver` (resolves `@import` names against `~/.luna/packages`) and `internal/manifest` (reads and writes `config.lx` project manifests). Nothing in `main.go` imports them, so they don't do anything for you today.
- **`lunex init`** now tells you to use `lunex install <pkg>` (it said `lunex add`) and prints the package-management commands.
- **`lunex help`** only lists `LUNEX_DEBUG` and `LUNEX_VERBOSE` now. `NTL_DEBUG` still works internally, it's just not advertised anymore.
- **Tests.** Headers say 0.8.1, the JSON tests use "Lunex" as sample data instead of "Luna", and the `select` rename from above.
- **Stray files:** `test.lx` and `test1.lx` ended up in the repo root. They're my scratch files and you can delete them.

### Breaking changes, summarized

If you're upgrading from 0.7.3, here's what will actually bite you:

1. Reserved words can't be used as variable, function or parameter names anymore. Watch for `range`, `use`, `select`, `loop`, `when`, `channel`, `delete`, `assert`, `with` and similar. Those programs now fail to parse with `E0073` or `E0076`.
2. `lunex install` with no arguments no longer installs from `config.lx`, `lunex update` ignores `config.lx`, and `lunex list` prints `name@version` lines instead of a table. If a script parses that output, fix it.
3. Package specs must point at GitHub. `lunex install some-name` and non-GitHub URLs are errors now (before, they printed a success message and installed nothing).
4. Anything after the script name in `lunex run` goes to the script. A misspelled flag is no longer rejected, it ends up in `os.args()`.
5. `ws.connect` really connects now. A script that was calling it as a no-op will try to open a socket, and `wss://` will return an error.
6. `lunex remove` exits with 1 when it fails, so a script relying on it always succeeding will notice.

That's it for this one. If something breaks that isn't listed here, open an issue. There's a good chance it's something I missed in the diff and not something I meant to change.

---

## 0.4.1 → 0.6.0

This was more of a reset than a normal version bump. Between these two releases, I removed the Zig runtime, cut down the built-in module set, changed how projects are configured, and spent more time on errors and the day-to-day tools. Lunex is still Lunex, but the way you build it and lay out a project is different now.

A quick heads-up before getting into the details: **0.6.0 is not fully backward-compatible with 0.4.1**. The migration notes at the end call out the changes most likely to affect existing projects.

### The big change: Lunex no longer needs Zig

In 0.4.1, the project shipped a Zig runtime alongside the Go code. The build scripts could install Zig, compile `lunex-rt`, and include that runtime in the resulting setup. That added another toolchain to maintain and another thing that could go wrong when building on a different device.

That runtime and its build path are gone in 0.6.0. The project now builds as a Go program, and the build scripts use `CGO_ENABLED=0`, so you don't need a C compiler or CGo just to build Lunex. The old `zig/`, `internal/zigrt`, bridge, and AOT-related source trees are no longer part of the release.

The normal build is now simply:

```bash
./build.sh
```

For Termux, there's a separate script that can build for the current device or produce binaries for ARM64, ARM, and AMD64:

```bash
./build-termux.sh native
./build-termux.sh build
```

The Go runtime still handles execution, and the JIT code is still present. This isn't a claim that every program suddenly runs faster; the practical win here is a much simpler build and release setup, especially on Android.

### Projects now use `config.lx`

Project setup has changed quite a bit. Version 0.4.1 used a `lunex.mod` manifest plus a `build.lx` file. In 0.6.0, `lunex init` creates a `config.lx` project manifest instead, along with a starter `main.lx`, a small local module example, and a `.gitignore`.

The config is regular Lunex code. For example:

```lx
val project = {
  name: "my-app"
  version: "1.0.0"
  main: "main.lx"
  entry: "main.lx"
  output: "dist"
  optimize: true
  targets: []
  dependencies: {
    "mathlib": "1.0.0"
  }
}
```

That means project settings and dependencies can live in the same place, and the loader can read the project object rather than relying only on the old line-based manifest format. `main` and `entry` are accepted as the entry point, and dependencies are read from the `dependencies` object.

There's also a `lunex start` command now. From a project directory, it reads `config.lx` and runs the configured entry point. `lunex build` can use the same config when you don't pass a source file directly.

### Imports have a clearer split

The standard library is now registered directly with the runtime. Imports such as `@import("std.io")` and `@import("std.http")` no longer need the old `.lx` wrapper files from the top-level `lib/` directory in order to reach the built-in modules.

Local file imports are more explicit too. `@fimport` is for files on your machine, and 0.6.0 resolves local source and compiled files relative to the file doing the import. You can use `.lx`, `.nc`, or `.nax` files:

```lx
val math = @fimport("./src/math.lx")
val io = @import("std.io")

fn main() {
  io.log(math.add(2, 3))
}
```

If you leave a supported extension off, the resolver tries the known extensions. Nested imports inside a local module are resolved relative to that module, rather than being treated as though they came from the original entry file.

External packages use `@import`, not `@fimport`. Packages installed with `lunex install` or `lunex add` are cached locally, so they can be used again without downloading them on every run. GitHub sources are supported, and HTTPS sources outside GitHub go through a stricter verification flow before being cached.

### The built-in library has a smaller core

This is the other change worth reading carefully. Version 0.4.1 registered a much larger set of built-in modules, including integrations for services and file formats. In 0.6.0, the built-in native set is focused on these 13 modules:

- `std.io` — console input and output, formatting, colors, tables, and progress helpers.
- `std.fs` — reading, writing, listing, watching, copying, and moving files.
- `std.http` — HTTP client and server functions.
- `std.crypto` — hashing, encoding, encryption, password helpers, and random data.
- `std.db` — the built-in database module.
- `std.ws` — WebSocket client and server support.
- `std.jwt` — JSON Web Token helpers.
- `std.math` — math functions and constants.
- `std.datetime` — date and time utilities.
- `std.os` — process and operating-system information.
- `std.regex` — regular-expression matching and replacement.
- `std.env` — environment variables.
- `std.utils` — general utility helpers.

There are a few practical additions in that core. The filesystem module now exposes `copyFile` and `moveFile`. The I/O module has clearer output helpers such as `err`, `warn`, `info`, and `success`, plus utilities for tables, progress, spinners, formatting, and terminal output.

What didn't make the built-in registry is just as important. The modules available in 0.4.1 for AI, allocation, compression, CSV, Excel, GraphQL, mail, Markdown, Mustache, MySQL, OAuth2, path helpers, PDF, PostgreSQL, RabbitMQ, Redis, Stripe, validation, XML, YAML, and ZIP are not registered as built-ins in 0.6.0. Some of that functionality can live in separately installed packages, but you should not expect those old imports to work out of the box in this release.

The old `lib/` shims are gone as well. The runtime now owns the standard module registry, so there is less duplicated glue between Lunex code and the native implementation. It's a smaller standard library, not a claim that all of the old integrations have been replaced one-for-one.

### Building and unpacking compiled files

The `.nc` bytecode and `.nax` archive workflows are still here. `lunex build` compiles a file, and the output extension determines whether you're producing a `.nc` file or a `.nax` archive. `lunex pack` bundles a project directory into a `.nax` file.

The new command in this area is `lunex unpack`, which extracts a `.nax` archive into a directory:

```bash
lunex build main.lx -o main.nc
lunex build main.lx -o app.nax
lunex pack ./my-project -o dist/app.nax
lunex unpack dist/app.nax
```

The runner also accepts `.lx`, `.nc`, and `.nax` inputs. For source files, `--emit ast` and `--emit ir` are available when you need to inspect what the compiler is producing.

Cache handling is more exposed from the CLI now. You can inspect or clear the disk cache with `lunex cache`, inspect or clear the in-process cache with `lunex memcache`, choose a cache directory with `lunex set cache <dir>`, and use `--no-cache` when you want a run to compile fresh without storing results. `lunex platform` prints platform and adapter diagnostics, and the existing benchmark command remains available as `lunex bench <file>`.

### Syntax errors now give you something useful to work with

The parser now creates the same structured error type used by the rest of the compiler, with source lines, locations, error codes, and context-sensitive suggestions. So instead of only being told that a token was unexpected, you can get a hint that points at the likely mistake.

A few examples of the kinds of hints it can give:

- An extra `)` or `}` points you toward checking the matching opening delimiter.
- A comma in the wrong place explains when a list needs `[]` or `()` around it.
- A single `=` in a comparison points out that `==` is the equality operator.
- A semicolon warning reminds you that line breaks are the normal way to separate statements in Lunex.

The compiler also reports style issues it can recognize, including several statements crammed onto one line, spacing problems, missing whitespace before `{`, and implicit conversions between values of different types. Those are warnings intended to help you catch problems and keep the code readable; they don't mean every program with a warning is invalid.

### The formatter and compiler have had a cleanup

`lunex fmt` was already available in 0.4.1, but the formatter implementation has been reorganized around the AST in 0.6.0. That gives the formatter a better view of the code structure, and it includes a pass for fixing the coercions it knows how to rewrite.

The compiler now passes source lines into the parser, so diagnostics can show the relevant line and column instead of losing that context along the way. Type checking can also be enabled through the file flags recognized by the compiler, rather than being hard-wired into every compilation.

### Runtime failures and command-line options

The top-level runner now has a recovery path for panics that escape the normal error handling. It recognizes common cases such as stack overflow, invalid memory access, and running out of memory, and prints a clearer runtime error instead of dumping a raw panic message.

There are also public flags for debugging and cache control:

```bash
lunex --debug run main.lx
lunex --verbose run main.lx
lunex --no-cache run main.lx
```

`--debug` enables the execution diagnostics, `--verbose` adds more detail, and `--no-cache` skips cached compiled results. The runtime also sets a default Go garbage-collection target and a soft memory limit unless the corresponding environment variables are already set, so existing `GOGC` and `GOMEMLIMIT` settings are respected.

### The examples have been reorganized, and XML has its own package

The old example folder had a handful of numbered tutorials mixed with separate demos. Version 0.6.0 organizes the examples into 23 numbered files, with separate examples for channels, structs and composition, loops, higher-order functions, HTTP REST requests, filesystem operations, date and time, state machines, pipelines, and more. There's also an `examples/README.md` to explain the set.

The release includes a separate `lune-xml` project. It has an XML parser and builder, tree queries, transformations, object conversion, schema checks, a prebuilt `.nax` file, and tests split across node, builder, parser, query, transform, and validation behavior. It is imported as a local module rather than being part of the 13-module built-in registry.

### Smaller changes

- **Fewer build dependencies** — the Go module now declares Go 1.23 rather than Go 1.25, and the direct dependency list is much smaller. The build no longer has to bring in the libraries used only by the removed integrations.
- **Project commands** — `lunex update` / `lunex upgrade` can update one installed package or the dependencies listed in `config.lx`.
- **CLI cleanup** — the 0.4.1 `ide`, `edit`, and `rt-info` commands are no longer present in the 0.6.0 command switch. Cache, platform, package, and build commands are the main supported workflow now.
- **Documentation layout** — the old collection of individual files under `docs/` is not included in the 0.6.0 archive. The main overview is in `README.md`, and the module system has its own `modulesys.md` guide.
- **Installer files** — `install.sh` and `install.ps1` shipped in the 0.4.1 archive but are not included in the 0.6.0 archive. The 0.6.0 README still points to the hosted `install.sh` URL, so check that the script is available in the branch you're installing from.
- **Version metadata** — the 0.4.1 archive's `version.json` still says `0.4.0`, even though the archive is named `0.4.1`. The 0.6.0 metadata reports `0.6.0`. The 0.4.1 metadata also says Apache-2.0 while its bundled `LICENSE` text is MPL-2.0; the 0.6.0 metadata now says MPL-2.0 as well.

### Breaking changes, summarized

If you're moving a project from 0.4.1 to 0.6.0, these are the changes most likely to matter:

1. **Zig is no longer part of the build.** Remove steps that install Zig or build `lunex-rt`; use the Go build scripts instead.
2. **Project configuration moved.** `lunex.mod` and `build.lx` are no longer the config files discovered by the 0.6.0 project build flow. Move the relevant settings and dependencies into `config.lx`.
3. **Several built-in modules are gone.** Imports for the integrations listed above will not resolve as built-ins in 0.6.0. Replace them with packages or remove those imports.
4. **The I/O API is not identical.** In particular, code using `io.error` should be checked against the 0.6.0 `io.err` API, and older helper functions may not all be present.
5. **Keep local and package imports separate.** Use `@fimport` for local `.lx`, `.nc`, and `.nax` files; use `@import` for standard modules and installed packages.
6. **The bundled editor and IDE commands were removed.** `lunex ide`, `lunex edit`, and `lunex rt-info` are not available in this release.
7. **Don't rely on the old source archive layout.** The `lib/`, `docs/`, Zig runtime, and installer files from 0.4.1 are not all present in the 0.6.0 archive.

That's the main story: 0.6.0 is smaller and easier to build, with a clearer project and import setup and more useful diagnostics. The trade-off is that it drops some of the extra integrations and bundled tools from 0.4.1, so it's worth checking your imports and project config before switching over.
