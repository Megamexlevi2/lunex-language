
---

# Lunex changelog — 0.9.5 → 0.9.6

This one is a mix of a syntax change I'd been putting off, a lot of tooling, and a big rework of the JIT. The syntax change is string interpolation: it's `!{expr}` now and it works in every kind of string. Around that I added a real formatter (`lunex fmt`), rebuilt `lunex debug` into a proper profiler/diagnostics tool, made the JIT compile in the background and tier up in the middle of hot loops, sped up the interpreter's object/instance handling, taught `std.db` to open databases at any path, and made the WebAssembly build actually usable (virtual filesystem, project files for `@fimport`, Web Worker support). I also fixed a handful of real bugs, including `Math.hypot` returning the wrong number.

## String interpolation is `!{expr}` now (breaking)

```lx
val name = "Lunex"
val age = 30

io.log("Hello, !{name}! You are !{age} years old.")
io.log('Hello, !{name}!')
io.log(`Hello, !{name}!`)
```

Before, only backtick strings interpolated, and they used `${expr}`. Now any string literal — `"..."`, `'...'` or `` `...` `` — interpolates with `!{expr}`, and backticks are just another way to quote a string. A string with no `!{` in it is a plain string no matter which quote it uses.

`${expr}` no longer does anything. It stays as literal text. If you have 0.9.5 code like `` `Hello, ${name}` ``, it has to become `` `Hello, !{name}` ``.

A few details:

- The lexer now scans the inside of `!{ ... }` properly. Quotes inside the expression are tracked, so a `}` inside a string inside the interpolation doesn't close it early, and nested `{}` are balanced.
- A quoted string that contains `!{...}` is tokenized as a template, the same as a backtick string. A quoted string without it stays a plain string.
- To get a literal `!{` in a normal quoted string, escape the bang: `"price: \!{10}"`.
- If the expression inside `!{...}` fails to evaluate, the placeholder is replaced with `!{error}` (it used to be `${error}`).
- `std.utils.template` still supports both `{{key}}` and `${key}` placeholders. That one is a plain string replace and has nothing to do with the language syntax, so I left it alone.

## Lexer fixes

- **Block comments close with `*/` now.** Before this, a `/* ... */` comment was actually terminated by the next `/*`, which is why the docs example looked broken. It ends at `*/` like you'd expect, and the language reference is fixed to match.
- **An unterminated block comment is an error** (`unterminated block comment at line:col`) instead of silently eating the rest of the file.
- **Unknown characters are an error** (`unexpected character ... at line:col`). The lexer used to skip anything it didn't recognize without telling you.
- Line comments (`/!`) now handle `\r\n` line endings correctly.
- Tokens now carry their raw source text, and there's a new `COMMENT` token plus `TokenizeWithComments` for tools that need comments. The normal path still throws comments away, so nothing changes for the parser.

## `lunex fmt` — a real formatter

```bash
lunex fmt main.lx            # format in place (default)
lunex fmt main.lx --write    # same thing, explicit
lunex fmt main.lx --check    # exit 1 if it would change, don't touch the file
lunex fmt main.lx --stdout   # print the result, don't touch the file
lunex format main.lx         # alias
```

The old formatter rebuilt the code from the AST, and the AST doesn't carry comments, with a crude brace-counting fallback behind it. The new one is token-based and AST-guided: it uses the lexer for layout and the parser's source positions to tell statements, blocks, and object/array literals apart. It normalizes indentation, operator spacing, comma spacing, control-flow blocks, and wraps long arrays/objects, while keeping every token's original spelling, string and template literals, comments (both `/!` and `/* */`) and your line endings (CRLF stays CRLF).

The part I care most about is that it never writes something it hasn't verified. Before anything is written, the candidate is checked against the original: same tokens and values, same comment contents, same AST structure (ignoring positions), and it has to parse again. If the syntax-aware output fails that check, it tries the AST output, then a conservative whitespace-only pass. If nothing verifies, it leaves your file alone and exits with an error. A file that doesn't parse at all gets the whitespace-only treatment.

Other things worth knowing:

- Writes go through a temp file and a rename, keep the file's permissions, and follow symlinks.
- Exit codes: `0` ok, `1` not formatted (`--check`) or refused/unverifiable, `2` usage error. `--check` and `--stdout` can't be combined.
- `compiler.Format` (what the rest of the toolchain calls) now goes through the same formatter.
- Tiny fix in the old AST printer: template literals are printed from their parts instead of an empty value.
- There's a `lunex help fmt` page, and `format` works as a help topic too.

## `lunex debug` was rebuilt

`lunex debug` used to just turn on verbose output and run your file. Now it's a proper diagnostics tool that works on `.lx` files and `.nax` archives:

```bash
lunex debug app.lx
lunex debug app.lx --report debug-report.json
lunex debug app.lx --cpu-profile cpu.pprof --mem-profile heap.pprof
lunex debug app.lx --alloc-profile allocs.pprof --block-profile block.pprof --mutex-profile mutex.pprof
lunex debug app.lx --trace runtime.trace --goroutines goroutines.txt
lunex debug server.lx --pprof 127.0.0.1:6060
lunex debug app.lx --preserve-jit
lunex debug app.lx --report r.json -- arg1 arg2
```

What you get:

- **Lunex call stacks with source locations.** Runtime errors now show a stack trace of Lunex functions (file, line, column), and the trace lists the most recent call first, capped at 12 frames with a "N more frames omitted" line. Tasks inherit the caller's stack. Go panics print the Go stack and the Lunex stack together.
- **Sampled function profile.** A table of calls and estimated time per function, sorted by time. The timings are sampled and weighted (the first 8 calls of a function are always timed, then roughly one in 32, scaled back up), so they're good for finding hot functions but they're estimates, not exact per-call numbers. `--profile-limit <n>` (1–1000, default 20) and `--no-profile` control it.
- **Go runtime artifacts:** `--cpu-profile`, `--mem-profile` (alias `--heap-profile`), `--alloc-profile`, `--block-profile` (about 1 ms sampling), `--mutex-profile`, `--trace`, and `--goroutines`. Everything except the trace is something you can open with `go tool pprof`.
- **`--report <file>`**: a JSON report (`schema_version: 1`) with the tool version, Go version, OS/arch, PID and GOMAXPROCS; the target path, kind (source, NAX archive, or compiled object), size, line count and SHA-256; the execution status (`ok`, `compile_failed`, `runtime_failed`, `panic`, `program_exit`), failed phase, exit code and compile/run/total timings; memory and GC snapshots; the function profile; the error details; and the Lunex and Go stacks. It's written even when compilation fails.
- **`--pprof <host:port>`** serves live diagnostics while the program runs: `/debug/pprof/…`, `/debug/lunex/functions` and `/debug/lunex/runtime`. It only accepts `localhost` and loopback addresses, and refuses anything public. Port-forward if you need to look at a remote process.
- **JIT is bypassed by default in debug mode** so stacks and source locations stay accurate. `--preserve-jit` keeps the normal JIT path when you're chasing a JIT-only problem or profiling something production-like; frames may be missing or less precise in that mode.
- `--no-color` (and `NO_COLOR` / `TERM=dumb`) turn colors off in both debug output and error output. `--` ends debugger options and forwards the rest to your program.
- The output is calmer and more structured: sections, aligned `label  detail` rows, and a resource summary (heap, GC runs, goroutines) at the end. `LUNEX_DEBUG`, `NTL_DEBUG` and `DEBUG` still turn on debug output for any command.
- `lunex debug` with no file now prints its help and exits with status `2` (it used to print one usage line and exit `1`). Bad options also exit `2`. `lunex help debug` has the full option list.

## Exit status from `std.testing`

If `testing.run()` reports a failed run, the process now exits with status `1`. The script isn't interrupted: `run()` still returns the structured result and the rest of your code (cleanup, printing a summary) still executes, and the exit status is applied once execution finishes. It works for `lunex run`, `lunex -e`, packed `.nax` programs and `lunex debug`, so `lunex run tests/main.lx` can finally fail a CI job. The exit code is clamped to 1–255 and the first requested non-zero code wins.

## JIT rework

This is where most of the engine work went. The goal was to stop the JIT from stalling the interpreter and to let it keep going instead of throwing work away. Nothing here changes what your code returns; it changes what gets compiled, when, and how much of a run stays native.

**Compilation happens in the background.** When a loop or function gets hot, compilation is queued to a small worker pool (up to 4 workers, 2 on Android, capped by `GOMAXPROCS`) and the interpreter keeps running the code while it waits. The result is swapped in when it's ready. If the queue (64 deep) is full, it just tries again later. A panic in a compile worker is caught and reported instead of taking the process down.

**Tier-up in the middle of a hot loop.** Tier 1 loops count iterations as they go. After 2048 iterations they hand control back, the loop's state is committed, and the tier 2 version is compiled. A long-running loop no longer has to finish (or be re-entered) before it can get the faster code. Counted loops and range loops account for the iterations already done, so nothing runs twice.

**Side exits that resume instead of restarting.** Native code used to bail out ("deopt") when it hit something it couldn't finish, for example a modulo on huge operands like `1e308 % 2`, and the interpreter re-ran the work from scratch. Now the machine code records exactly where it stopped (bytecode position and stack depth) and a resume step continues from that point. That also covers self-recursive functions: a side exit deep in a recursive call resumes at the call site. The compile-time self-check now exercises this path too, and it compares hot-loop counts as well as results.

**Type feedback and value specialization.** The interpreter records operand types on arithmetic and comparison nodes. Tier 2 code is only produced when the numeric feedback is stable (at least 95% numeric samples). Variables that stay constant across a loop's observed iterations get baked into the compiled code as constants, with a guard. If a guard stops matching, the loop falls back to a generic tier 1 version instead of running wrong code.

**Inlining into loops.** Small single-expression functions called from a hot loop can be inlined, as long as they're not recursive and don't use defaults, rest or destructured parameters. The inlined call carries an identity guard on the function, so if you reassign the function the guard fails and you fall back safely.

**Other code generation changes:**

- Tier 2 unrolls counted and range loops 4×, and keeps the remaining count and start index correct.
- Tier 2 keeps hot local slots in registers, including across self-recursive calls.
- `Math.sqrt` compiles to a single hardware square-root instruction on amd64 and arm64 (previously a libm call), next to `abs`.
- The program close/run path is guarded, so a program being replaced by a faster tier can't be closed under a run that's still using it.

**Math on more platforms.** Native `Math` calls that go through the system math library now work on **Windows** too, using `ucrtbase.dll` (falling back to `msvcrt.dll`). In 0.9.5 I said those calls stayed on the interpreter on Windows; that's no longer true. FreeBSD is also recognized now (`libm.so.5` / `libm.so` / `libc.so.7`). Everything still goes through `purego`, so there's still no CGo and no C compiler.

**`Math.hypot` fix.** `Math.hypot(a, b)` was wired to `abs`, so both the interpreter and the JIT returned the absolute value of the first argument (`Math.hypot(3, 4)` gave `3`). It's now a real two-argument `hypot` in the interpreter and a native op in the JIT. This is a behavior change, but a correct one.

**Diagnostics and `lunex runtimes`.** `lunex runtimes` now gives a real report: host OS/arch, whether the native JIT is available / enabled / disabled / unavailable (and why), the tiers, the hotness threshold, whether native verification and tracing are on, a note that `.nax` archives and cached objects are serialized AST that run through the same interpreter (not a separate VM), and how to control it. `--jit` / `--no-jit` and the `LUNEX_NATIVE`, `LUNEX_NATIVE_HOT`, `LUNEX_NATIVE_VERIFY`, `LUNEX_NATIVE_TRACE` variables work as before. `LUNEX_NATIVE_HOT` now describes the threshold for both loops and functions before background compilation. Tasks and `parallel.map` skip the JIT while `lunex debug` is running without `--preserve-jit`.

## Interpreter performance work

None of this is meant to change behavior, but it all touches hot paths:

- **Object and instance shapes.** Plain objects with more than one property are promoted to an interned "shape" (sorted property names with values in slots) after a few property reads, and class/struct instances now store their fields in slots described by a shape built from the class (and its parents). Deleting a property drops an object back to the plain map representation.
- **Member access caches.** Each `a.b` site remembers the shape and slot it last saw and skips the lookup when the shape matches.
- **Smaller values.** The rarely used parts of a value (function, class, instance, regex, error, task) moved behind a pointer, so every number, string and boolean is lighter.
- **Cheaper scopes.** Slot layouts for scopes are shared instead of rebuilt each time a scope is entered, scope objects reuse their slices, and `val` constness is tracked per slot.
- **Instance lookup** now walks the class chain (and prototype) directly. `in`, `keys`, object spread, rest destructuring and JSON output all go through the new field accessors so slot-based instances behave exactly like before.
- **Adaptive garbage collection.** If you haven't set `GOGC`, Lunex still starts at 50, then every 2 seconds adjusts the GC percentage between 10 and 70 based on how close the heap is to the memory limit (`GOMEMLIMIT`, default 200 MiB; on Android it uses the tuned limit). Setting `GOGC` yourself disables this, and `LUNEX_GC_ADAPTIVE=0` turns it off explicitly.

## Better runtime errors

- Calls now update the current source location, so errors raised inside calls point at the right line and column.
- Errors in packed programs show the source snippet again: `.nax` archives and compiled objects that carry source text (the default for `lunex pack`) now pass it to the runtime, where before the source lines were blank.
- The "calling X resulted in undefined" hint no longer tells you to add a `return` statement (Lunex doesn't have one); it now says to end the function with a value expression, and the example matches.
- Running a `.nax` archive through a shared compiler (what `lunex debug` does) now restores the previous module loaders when the run finishes, instead of leaving the archive's loaders installed.

## `std.db`: open a database anywhere

```lx
val db = @import("std.db")

val a = db.open()                         // .lunex/data/default.db
val b = db.open("app")                    // .lunex/data/app.db
val c = db.open("./data/app.db")          // exactly that file, parent folders created
val d = db.open("./data", "app")          // ./data/app.db
val e = db.open("./data/")                // ./data/default.db

db.drop("./data", "app")                  // deletes ./data/app.db and its SQLite sidecar files
```

- A plain name (`"app"`) keeps working the way it always did.
- Anything that looks like a path (an absolute path, something with `/` or `\`, or a name starting with `.`) is used as a path, relative to the current working directory. Missing parent folders are created. A path with no extension gets `.db`, and a path ending in `/` (or an existing directory) uses `default.db` inside it.
- The two-argument form `db.open(dir, name)` puts `name` (with `.db` added if it has no extension) inside `dir`. The name has to be a file name.
- `db.create` and `db.connect` behave like `db.open`. `db.drop(path)` and `db.drop(dir, name)` use the same rules, so use the same spelling you opened it with.
- Databases are now tracked by their resolved file path, so opening the same file two different ways gives you the same handle. `database.path` tells you which file was opened, and `database.name` is the file name without the extension for path-style opens.

One behavior change to watch for: before, `db.open("./data/app")` quietly threw away the folder part and used `.lunex/data/app.db`. It now really uses `./data/app.db`. If you relied on that, switch to a plain name.

## WebAssembly build

- **`@fimport` works with your project files.** The browser loader was already fetching your imports in 0.9.5, but the Go side ignored them. `Lunex.run(source, project)` now actually reads `{ filename, files }` and resolves `@fimport("./x.lx")` / `.nax` from those files (paths are normalized, so `./`, `\` and leading `/` all work). `filename` also becomes the name used in error messages instead of `<browser>`.
- **`Lunex.ready()`** returns a promise that resolves when the virtual filesystem has finished loading its saved state. The loader awaits it before exposing `run`, `runFile` and `runWithImports`.
- **`std.fs` is a virtual filesystem in the browser and WASI builds.** It's an in-memory tree with the usual `std.fs` surface (`readFile`, `writeFile`, `appendFile`, `readDir`, `list`, `mkdir`, `ensureDir`, `rename`, `copy`, `moveFile`, `delete`, `deleteAll`, `rmdir`, `stat`, `exists`, `isFile`, `isDir`, `size`, `glob`, `readJSON`/`writeJSON`, `readCSV`/`writeCSV`, `readLines`, `tempDir`/`tempFile`, and the path helpers). In the browser it's saved to IndexedDB (database `lunex_fs`), so files survive a reload; where IndexedDB isn't there, it still works in memory. In the WASI build it's in memory only. The native `std.fs` is only built for non-WASM targets now, which also means the WASI build no longer reaches the host's directories through `std.fs`.
- **The loader works in Web Workers.** It no longer requires `document`: it loads `wasm_exec.js` with a script tag in a page or `importScripts` in a worker, and uses `self.location.href` as the default base URL. It only throws now if it can do neither.
- The `std.db` and `std.ffi` exclusions and the other "does not exist" cases from 0.9.4 are unchanged.

## Docs and repo cleanup

- **Docs:** the README examples were rewritten to be complete, runnable programs (wrapped in `fn main()`, with the imports they need), and `hello.lx` is run as `./lunex run hello.lx`. The README, `docs/cli.md` and `lunex help` document `lunex fmt`, the new `lunex debug`, and the new `lunex runtimes`. `docs/cli.md` has a new "Runtime debugger" section, "Global flags" and the FFI note moved to the end of the file, and the environment table now lists `LUNEX_GC_ADAPTIVE`, `LUNEX_NATIVE_VERIFY` and the updated `GOGC`/`GOMEMLIMIT`/`LUNEX_NATIVE_HOT` descriptions. The language reference has a new "String interpolation" section with a migration note, the corrected block-comment example, and the `!{}` template example. `docs/stdlib.md` has the new `std.db` open/drop tables and a note about `std.testing` exit status. The language reference and stdlib headers say `0.9.6-beta`.
- **`CMakeLists.txt` and `cmake/` are actually gone.** I said in 0.9.5 that they were removed, but they were still sitting in the 0.9.5 tree. They're really deleted now (the Windows build is `build.ps1`). I also deleted a stray root `test.lx` benchmark scratch file.
- **Version:** `internal/app/version.json` is `0.9.6`, build date `2026-10-23`. The minimum Go version is still 1.25.5.

## Smaller stuff

- **`std.image` always releases its lock.** `resize`, `flipH`, `flipV`, `gray`, `invert`, `brightness`, `contrast`, `saturation`, `paste`, `blur` and `sharpen` used to unlock the image by hand after doing their work, so if the underlying image operation panicked, the image stayed locked and the next call on it would hang. They now unlock with `defer`, the same way `pixel`, `setPixel`, `crop` and `rotate` already did. No inputs, outputs or limits changed.

## Breaking changes, summarized

If you're upgrading from 0.9.5, here's what will actually bite you:

1. `${expr}` doesn't interpolate anymore. Use `!{expr}`, and note that it now works in `"..."` and `'...'` too. That also means any existing string that happens to contain a literal `!{` will now be treated as an interpolation; escape it as `\!{`. Update your own `.lx` files and any libraries you depend on.
2. Block comments end at `*/`, not `/*`, and an unterminated block comment is an error.
3. Characters the lexer doesn't recognize are now an error instead of being skipped.
4. `std.db`: a path-looking name now means a real path. `db.open("./data/app")` used to land in `.lunex/data/app.db`, and now it creates `./data/app.db`.
5. `std.fs` in the browser and WASI builds is a virtual filesystem. WASI programs no longer see the host's directories through `std.fs`.
6. `testing.run()` with a failing result makes the process exit with status `1`. A CI step that used to pass with a red test run will fail now (which is the point).
7. `Math.hypot(a, b)` returns the real hypotenuse instead of `abs(a)`.
8. `lunex fmt` formats in place by default (use `--check` or `--stdout` if you don't want that), and `lunex debug` usage errors exit with `2`.
9. Stack traces print the most recent call first.

Same as always: if something breaks that isn't listed here, open an issue. It's probably a doc gap and not on purpose.


---

# Lunex changelog — 0.9.4 → 0.9.5

The big thing here is that I ripped out `spawn`/`channel` and replaced them with a task-based concurrency model. That touched the lexer, parser, checker, resolver, formatter and runtime, so it's the center of gravity for this release. Around that I added two sizable new modules (`std.net` and `std.encoding`), a real Language Server, native math in the JIT, a Windows build script, and a checker bug that's been bugging me for a while.

## Concurrency: `spawn`/`channel` are gone, tasks are here

This is a breaking change and I'm not going to pretend otherwise. `spawn fn() { ... }()` and `channel()` are no longer valid syntax — the lexer doesn't know those keywords anymore. If you have 0.9.4 code using them, it won't parse.

What replaces it:

```lx
fn square(n) {
  n * n
}

val task = new.task(square, 21)
val result = task.await()
```

`new.task(fn, ...args)` schedules a function call on a worker pool and gives you back a task handle immediately. The handle has four methods:

- `task.await()` — block until it finishes and return the result (or propagate the error).
- `task.done()` — `true` once it's finished, failed, or been cancelled.
- `task.state()` — `"pending"`, `"running"`, `"completed"`, `"failed"`, or `"cancelled"`.
- `task.cancel()` — cancel it, but only while it's still pending. Returns whether that worked. A task that's already running keeps running; cancellation doesn't interrupt it.

For groups of tasks there's `tasks.all(handles)`, which waits for everything and returns results in input order (first failure wins and gets propagated), and `tasks.any(handles)`, which returns as soon as one finishes, as `{ index, value }`. Both return sane values on an empty array instead of hanging — `tasks.all([])` is `[]`, `tasks.any([])` is `{ index: -1, value: undefined }`.

`parallel.map(fn, array)` splits an array across the worker pool and gives you back results in order:

```lx
val squares = parallel.map(square, [1, 2, 3, 4, 5])
```

`select` is still around, but it waits on task handles now instead of channels — `case value = someTask => { ... }` instead of reading off a channel. Whichever task is done first wins, with source order breaking ties.

```lx
select {
  case value = first => {
    io.log("First completed:", value)
  }
  case value = second => {
    io.log("First completed:", value)
  }
}
```

One thing I didn't expect going in: because `new.task` feeds through the same JIT path as hot functions, a task running a pure numeric function can get compiled to native code the same way a hot loop would, instead of always going through the interpreter. `parallel.map` does this too when every element is a number. It's opportunistic — nothing you need to configure, and anything that doesn't qualify just runs on the interpreter like before.

`Promise.all(arr)` in the JS-compatibility layer is built on top of this now too, so it waits on real Lunex task handles. It still only takes arrays, not arbitrary iterables or thenables.

## `std.net` — new

A full networking module: TCP, UDP, Unix sockets, TLS, DNS, and IP/CIDR utilities, all synchronous.

```lx
val net = @import("std.net")

val conn = net.dial("tcp", "example.com:80")
conn.write("GET / HTTP/1.0\r\n\r\n")
io.log(conn.readLine())
conn.close()
```

Connections: `dial`, `dialTimeout`, `dialTCP`, `dialUDP`, `dialTLS`, with `listen`, `listenTCP`, `listenUDP`, `listenPacket`, and `listenTLS` on the server side. A connection gives you `read`/`readBytes`/`readFull`/`readLine`, `write`/`writeBytes`, deadlines (`setDeadline`, `setReadDeadline`, `setWriteDeadline`), TCP-specific knobs (`setNoDelay`, `setKeepAlive`, `setKeepAlivePeriod`), and `tlsState()` for the negotiated version, cipher suite and peer certificates on a TLS connection. Listeners give you `accept()`, `address()`/`addr()`, and `close()`. Packet connections (UDP) get `readFrom`/`writeTo` instead, since there's no single peer.

DNS is in there too: `lookupIP`, `lookupAddr` (reverse lookup), `lookupCNAME`, `lookupMX`, `lookupNS`, `lookupTXT`, `lookupSRV`, `lookupPort`, plus `hostname()`, `interfaces()`, and `localIPs()` for local machine info. DNS failures are real errors now, not silently-empty results.

And a set of IP/CIDR helpers that don't touch the network at all — `parseIP`, `isIP`, `parseCIDR`, `containsCIDR`, `isPrivate`, `isLoopback`, `isMulticast`, `joinHostPort`/`splitHostPort`, and similar — useful for validating input or writing access rules without spinning up a socket.

Timeouts throughout are `timeoutMs` (or the alias `timeout`), always in milliseconds.

## `std.encoding` — new, and `base64*` moved out of `std.crypto`

```lx
val encoding = @import("std.encoding")

val bytes = encoding.utf8Encode("Lunex ✓")
val text = encoding.utf8Decode(bytes, true)
val encoded = encoding.base64Encode("Hello, Lunex!")
val decoded = encoding.base64DecodeText(encoded)
```

This is the breaking part: `crypto.base64Encode`, `crypto.base64Decode`, `crypto.base64UrlEncode`, and `crypto.base64UrlDecode` are gone from `std.crypto`. `std.crypto` keeps Base64 only internally for its own cryptographic formats now. If you were calling those four functions off `crypto`, switch to `std.encoding` — `base64Encode`/`base64Decode` plus `base64DecodeText` if you want the bytes decoded straight to UTF-8 text, since the raw decoder now returns a byte array instead of a string.

Beyond Base64, `std.encoding` covers a lot of ground: UTF-8/UTF-16/UTF-32 encode and decode (with explicit `le`/`be`/`auto` endianness and BOM handling), Latin-1, Base32, and URL percent-encoding (`percentEncode`/`percentDecode`, plus `formEncode`/`formDecode` for the `application/x-www-form-urlencoded` variant that uses `+` for spaces). There's also a generic `encode(text, format)` / `decode(input, format, fatal)` pair if you'd rather pick the format by string than call a format-specific function.

Decoders replace invalid sequences with U+FFFD by default; pass `fatal=true` to reject malformed input instead of silently substituting. Anything that fails in this module — bad padding, an unsupported format name, an invalid percent escape, a Latin-1 character outside U+0000–U+00FF — throws with the new `E0140` code.

## A real Language Server

```bash
lunex lsp
```

`lunex lsp` starts a standard JSON-RPC 2.0 language server over stdin/stdout, so any editor with an LSP client can talk to it. It covers the usual set: completion (filtered by lexical scope and receiver type, so after a string and a dot you get string methods, not every global in scope), hover, signature help, go-to-definition/declaration/type-definition, find references, rename, document and workspace symbols, document highlights, semantic tokens, diagnostics from the real parser and checker, a few code actions for "did you mean" suggestions, and folding ranges.

It analyzes your buffer as you type rather than waiting for a save, and indexes `.lx` files across the workspace (skipping the usual suspects — `.git`, `node_modules`, `vendor`, `dist`, `build`, and so on) up to 25,000 files. Point your editor's Lunex extension at the `lunex` binary with `lsp` as the argument and it should just connect.

## Native math in the JIT

Functions and loops that already qualified for native compilation can now use `Math` directly instead of falling back to the interpreter the moment they touch it:

```lx
fn distance(x, y) {
  Math.sqrt(x ** 2 + y ** 2)
}
```

`abs`, `ceil`, `floor`, `round`, `sign`, `min`, and `max` compile straight to inline SSE2 instructions on amd64. Everything else — `sqrt`, `cbrt`, the trig functions, `log`/`log2`/`log10`, `exp`, `pow`, `atan2` — resolves against the system's own math library (`libm`) through `purego`, so there's still no CGo dependency and no C compiler needed to build Lunex. This only works on Linux, Android, and macOS for now; on Windows those calls just stay on the interpreter. `**` compiles to the same native `pow` as `Math.pow`, and `Math.PI`, `Math.E`, and the other `Math` constants are recognized as compile-time constants rather than falling through to an object lookup.

None of this changes what your code returns — it's purely about what qualifies for the fast path.

## `std.ints` rewritten in Lunex itself

`std.ints` used to be plain Go, leaning on real `int64`/`uint32` machine types for the wrapping arithmetic. It's now written entirely in Lunex — the module source is Lunex code executed at startup, the same trick `std.encoding` uses. Since Lunex only has one numeric type (a float64), the 64-bit operations (`addU64`, `subU64`, `mulU64`, `shlU64`, `shrU64`, and so on) now do their own carry/borrow arithmetic across 16-bit words instead of relying on the host language's integer types.

I didn't change any inputs or outputs doing this — same functions, same signatures, same wrapping behavior at every width, and the existing test suite for this module passes unmodified. The point was consistency: one less module relying on native Go types for behavior that's supposed to be part of the language itself.

## Fixed: a `catch` binding could leak out of its block during `lunex check`

```lx
fn main() {
  try {
    riskyThing()
  } catch (err) {
    io.log(err)
  }
  io.log(err)   // used to pass lunex check; now correctly flagged
}
```

The runtime always scoped `err` correctly — this was purely a static-analysis gap. `lunex check` was adding the catch parameter straight into the surrounding scope instead of giving the catch block its own, so referencing it after the block closed didn't get flagged even though it would fail at runtime. The checker now builds a dedicated scope for the catch block, matching what the resolver already did, so `lunex check` and `lunex run` agree on this again.

## Windows builds without CMake

```powershell
.\build.ps1
```

Previously, building on Windows from source meant going through CMake. `build.ps1` is a native PowerShell build script that does the same job directly — no CMake, no separate `cmake/` directory to maintain. `CMakeLists.txt` and `cmake/` are gone from the repo.

The Unix-side scripts (`build.sh`, `build-termux.sh`, and the `Makefile`'s own installer) got friendlier too: if Go isn't found, they now detect your package manager (`apt`, `dnf`, `pacman`, `apk`, `zypper`, `brew`, or Termux's `pkg`) and offer to install Go through it, with a `curl`-from-go.dev download as the fallback if you'd rather not use the package manager, or if none was found.

## Smaller stuff

- **`E0067` was redefined.** It used to mean "invalid UTF-8 byte sequence"; that behavior is covered by `std.encoding`'s `E0140` now. `E0067` means malformed JSON input to `std.json` parsing.
- **Error hints updated for the new concurrency model** — the "protect shared state" hint now says "Mutex or task-safe synchronization" instead of mentioning Channels specifically.
- **Tests** — `tests/concurrency/` was rewritten around tasks (background tasks, numeric and object mapping, task workers, ordered task groups, empty-input behavior, nested tasks, lifecycle state and cancellation), plus new `tests/stdlib/net/` and `tests/stdlib/encoding/` suites. The two task example files (`examples/06_tasks.lx`, `examples/15_tasks.lx`) are now run as part of the test suite, not just sitting there as documentation.
- **`CONTRIBUTING.md`** documents `internal/lsp/`, `internal/checker/`, `internal/resolver/`, and `internal/jit/` in the repo-layout table, which were missing before even though some of them already existed.

## Breaking changes, summarized

If you're upgrading from 0.9.4, here's what will actually bite you:

1. `spawn` and `channel()` no longer parse. Rewrite that code against `new.task`, `tasks.all`/`tasks.any`, or `parallel.map`.
2. `select` cases now evaluate to task handles, not channels. `case x := someChannel` style code needs to produce a task instead.
3. `crypto.base64Encode`, `crypto.base64Decode`, `crypto.base64UrlEncode`, and `crypto.base64UrlDecode` are gone. Use the equivalents in `std.encoding`, and note that the raw decoders now return byte arrays — use `base64DecodeText` if you want text back directly.
4. If you were matching on `E0067` expecting "invalid UTF-8," that's `E0140` from `std.encoding` now. `E0067` means malformed JSON.
5. Building on Windows from source no longer goes through CMake — use `build.ps1`.

---

# Lunex changelog — 0.9.3 → 0.9.4

Smaller than 0.9.3, but it touches a lot of the core. Most of it is finishing things I started last time (`watch`, destructuring, packing), plus three new pieces: native code for functions, a `std.image` module, and a WebAssembly build.

## `watch` actually works end to end now

In 0.9.3 `watch` was in the docs and had an example, but the pieces behind it weren't all connected. Now the keyword goes through the lexer, parser, resolver, checker, formatter and runtime, and it behaves the way the 0.9.3 notes described:

```lx
var counter = 0

watch counter {
  io.log("Counter changed: " + counter)
}

counter = 10
counter = 20
counter = 20
```

The block runs twice there, not three times. Assigning the same value again doesn't count as a change. A few other things you can rely on:

- Compound assignments and `++` / `--` count as assignments, so `counter += 1` triggers the watcher.
- `watch user.name { ... }` also fires if you replace `user` with a completely new object that has a different `name`, not only when you assign to `user.name` directly.
- A watcher doesn't re-trigger itself while its own block is running, so writing to the watched variable inside the block won't loop forever.
- `lunex check` looks inside `watch` blocks now, and the formatter knows how to print them.

Error codes moved too. The docs had "invalid watch declaration" under `E0114`, but that code already belongs to the circular JSON error, so it's `E0117` now. There's also a new `E0118` for when the callback itself fails. If the error inside the block is already a Lunex error you get that one back with its own location, not a wrapper around it.

## Destructuring got a proper pass

```lx
val { name, role: access, missing = "guest", ...extra } = user
val [first, , third, ...remaining] = items
```

Things that changed:

- Object rest (`...extra`) works. It collects every field you didn't name, for plain objects and for struct instances.
- Bindings follow the keyword. `val`  are immutable, `var` and are mutable. Before this, everything you destructured was mutable no matter what you declared it with.
- Destructuring the wrong kind of value is an error now. An object pattern on a number or an array pattern on a string used to bind nothing and move on; now it tells you what it got.
- The array rest (`...remaining`) is a copy. Before, it pointed at the same elements as the original array, so changing one could leak into the other.
- Destructured names go through the same slot-based lookup as every other variable, so destructuring inside functions, blocks and loops behaves like a normal declaration. Function parameters and `each` loop variables can be destructured too.

## Wrong argument counts are caught at runtime

```lx
fn add(a, b) { a + b }

add(1, 2, 3, 4)
```

That throws `E0021` now ("function add expects 2 arguments, got 4") and points at the call. It's the same code `lunex check` already uses for the same mistake. Default parameters and rest parameters are counted properly, so you'll see messages like "expects 1 to 2 arguments" or "expects at least 1 arguments" when they apply.

Before this there was no check at runtime, so a call with the wrong number of arguments just went through.

Callbacks from `map`, `filter`, `reduce` and friends hand your function extra things (index, the collection). Those extras are trimmed for you when your callback doesn't take them, so `nums.map(fn(x) { x * 2 })` keeps working exactly like before.

Two smaller error fixes in the same area: assigning to a property of `null` or `undefined` now throws a clear error, and errors raised while `++` / `--` write their result are no longer dropped.

## Native code for functions, not just loops

Hot loops could already be compiled to native code. Now hot functions can too, including self-recursive ones, so the usual benchmark function is covered:

```lx
fn fib(n) {
  if n < 2 {
    n
  } else {
    fib(n - 1) + fib(n - 2)
  }
}
```

It only kicks in when it's safe to. Every argument has to be a number, the call has to pass exactly as many arguments as the function has parameters, and the function can't use defaults, rest or destructured parameters. It also needs at most 32 parameters, a return value on every path, and no calls to anything except itself. Anything else just stays on the interpreter, so there's nothing to turn on and nothing to change in your code.

How it works in practice:

- A function is compiled after 32 calls (`LUNEX_NATIVE_HOT` changes that number). Non-recursive functions get promoted to the optimized tier after four times as many calls.
- Recursion depth is capped at 1000. Going past it gives you the normal stack overflow error, not a crash.
- The execution budget, division by zero and modulo by zero are still reported with the same errors and messages as the interpreter.
- If a native run fails for any reason, that function goes back to the interpreter for the rest of the run.
- `LUNEX_NATIVE=0` turns the whole thing off and `LUNEX_NATIVE_TRACE=1` prints what got compiled and what got rejected to stderr, which is handy when something is slower than you expected.

Loops changed a bit as well. They try the optimized tier first and fall back to tier 1, instead of compiling twice. The optimized tier has a new bytecode pass that folds negated constants, drops values that get pushed and immediately thrown away, resolves branches on constant conditions and fixes the jump targets afterwards.

On the platform side, arm64 needs the instruction cache flushed after native code is written, so that's done now on Linux, Android, macOS and Windows. On macOS, builds with cgo use `MAP_JIT` and toggle write protection around the write; builds without cgo use plain `mmap` / `mprotect` and flush afterwards.

One unrelated startup change while I was in there: the runtime no longer pre-allocates about 66,000 number values at launch. Small integers (-128 to 4096) are still ready from the start, and the rest of the cached range is created the first time it's needed.

## `std.image` — new

A new built-in module for loading, editing and saving images:

```lx
val image = @import("std.image")

val img = image.open("input.png")
img.resize(800, 600)
img.gray()
img.save("output.png")
```

- **Creating and loading:** `image.create(width, height)` for a transparent image, `image.create(width, height, r, g, b, a?)` for a filled one, `image.open(path)` (JPEG orientation is applied), and `image.decode(bytes)` for data from a `std.buffer`, a byte array or a byte string.
- **Pixels:** `img.width`, `img.height`, `img.pixel(x, y)` returning `{ r, g, b, a }`, and `img.setPixel(x, y, r, g, b, a?)`.
- **Transforms:** `resize` (Lanczos), `crop`, `rotate`, `flipH`, `flipV`, `gray`, `invert`, `brightness`, `contrast`, `saturation`, `blur`, `sharpen` and `paste(source, x, y, opacity?)`. They change the image and return the same object, so you can chain them.
- **Output:** `img.encode(format?, quality?)` gives you a `std.buffer` and `img.save(path, format?, quality?)` writes a file, picking the format from the extension when you don't pass one. Formats are `png`, `jpeg` / `jpg`, `gif`, `bmp` and `tiff` / `tif`. JPEG quality goes from 1 to 100 and defaults to 95.

Images are capped at 16,384 pixels per side and 16,777,216 pixels total. Bad dimensions, out-of-range coordinates, unsupported formats and file problems come back as errors. The full table is in `docs/stdlib.md`.

It's built on `disintegration/imaging`, which pulls in `golang.org/x/image` and `golang.org/x/text` as new dependencies.

## Lunex in WebAssembly

There's a new `cmd/lunexwasm` entry point and a `build-webasembly.sh` script (yes, that's how the file is spelled for now):

```bash
./build-webasembly.sh
./build-webasembly.sh browser
./build-webasembly.sh wasi
```

`browser` builds `lunex-js.wasm` plus `wasm_exec.js` and a small `lunex-browser.js` loader into `dist/wasm`. From the page, `Lunex.run(source)` returns `{ ok, error }`, and the loader adds `runFile(url)` and `runWithImports(source, baseURL)`, which fetch your `@fimport` dependencies before running. `wasi` builds the normal CLI as `lunex-wasi.wasm`. Both are built with `CGO_ENABLED=0`.

Some things can't exist in a browser or a WASI sandbox, so in these builds they throw "does not exist" instead of failing in some random way: `os.exec`, `os.spawn`, `os.exit`, `server.listen`, `ws.createServer`, `ws.connect`, everything in `std.db` and everything in `std.ffi`. The FFI stubs on other unsupported platforms use the same wording now.

## Packing: source is kept by default

This reverses what I wrote in 0.9.3. Leaving the source out looked fine on paper, but then `lunex unpack` could only rebuild canonical code from the AST, which rewrites your syntax and throws away every comment. That's a bad surprise when you're trying to get your own files back.

So `lunex pack` keeps the original source text by default:

```bash
lunex pack ./my-project -o dist/app.nax
lunex pack ./my-project --no-source -o dist/app.nax
```

`--no-source` gives you the 0.9.3 behavior (smaller archive, canonical source on unpack) and `--source` is still accepted so existing scripts don't break. Single files and directories go through the same code now, and the "you can also use --source" hint after packing is gone.

`lunex unpack` got safer too. It refuses to write into a path that already exists, so it can't overwrite a project by accident, and if unpacking fails halfway it removes the folder it just created.

Related imports change: `@fimport` now resolves through the file loader relative to the file doing the import, tries `.lx` and `.nax` when you leave the extension off, and can load a `.nax` as a module.

Also, `lunex run`, the REPL and packed archives now build standard library modules the first time they're imported instead of all up front.

## Smaller stuff

- **Go version** — the minimum is 1.25.5 everywhere now. `go.mod` already said so, but `Makefile`, `build.sh`, `CMakeLists.txt`, `version.json` and the README still said 1.25.
- **`std.image` is known to the toolchain** — `lunex check` and the import resolver recognize it, and the README module table now lists `std.buffer`, `std.ints` and `std.image`.
- **Docs** — the language reference has a real destructuring section (aliases, defaults, rest, immutability rules), the CLI docs and `lunex help pack` / `lunex help unpack` describe the new source behavior, and the README explains `lunex add` versus `lunex install`.
- **Tests** — new `tests/advanced/watch.lx` and `tests/variables/11_destructuring_advanced.lx`, plus Go tests for `watch`, destructuring, arity errors, callbacks and the native function tiers. The test README counts are updated.
- **Removed `cmd/perfbench`** — it was an internal benchmarking tool and nothing else depended on it.

## Breaking changes, summarized

If you're upgrading from 0.9.3, here's what can actually bite you:

1. Calling a function with the wrong number of arguments throws `E0021` now. If something of yours was passing extra or missing arguments and getting away with it, it will stop.
2. Destructuring is stricter. A val destructured binding can't be reassigned anymore, and destructuring the wrong kind of value throws.
3. `lunex pack` includes source by default, so archives are bigger. Add `--no-source` to get the old output.
4. `lunex unpack` won't write into a folder that already exists. Move it or rename the archive first.
5. If you matched on the `watch` error code, it's `E0117` now (and `E0118` for a failing callback), not `E0114`.
6. You need Go 1.25.5 or newer to build from source.

Same as always: if something breaks that isn't listed here, open an issue. It's probably a doc gap and not on purpose.


# Lunex changelog — 0.9.2 → 0.9.3

Alright, this one ended up being bigger than I planned. What started as "let me just clean up the HTTP module" turned into a full rewrite of a bunch of core pieces. Sorry in advance for the size of this changelog, but there's a lot to go over.

## The headline: `watch` is here

This is the thing I'm most excited about. You can now write:

```lx
var counter = 0

watch counter {
  io.log("Counter changed: " + counter)
}

counter = 10
counter = 20
```

and it just reacts. No polling, no manual "did this change" checks. It also works on member expressions, so `watch user.name { ... }` is valid too. The block doesn't fire on registration — only after a later assignment actually changes the value. Under the hood the watched value gets captured when you register the watcher, and it's compared after every assignment that could touch it.

I added `watch` as a real keyword in the lexer/parser, not a library hack, so it gets proper syntax highlighting treatment and shows up in error messages like any other control structure. There's a dedicated example at `examples/26_watch.lx` if you want to poke at it.

One thing to note: a watch target has to be an identifier or a member expression. You can't watch an arbitrary computed expression (yet — might revisit this later if people ask for it).

## `std.http` got torn apart and rebuilt

Honestly this was overdue. The old `http` module was doing three jobs at once — client, server, and router — and it showed. The options were inconsistent, errors were just... whatever happened to bubble up, and there was no real way to reason about what a malformed request would do to your server.

So now it's three modules:

- **`std.http`** — client + raw server primitives (req/res, cookies, status codes). Doesn't route anything.
- **`std.http.router`** — actual routing, as a separate import.
- **`std.http.static`** — serving files from a directory.

### Client side

Every request option is now validated — pass something it doesn't recognize and it throws instead of silently ignoring it, which was biting people before. You get a real response shape back every time:

```lx
{ ok, status, statusText, headers, cookies, text, error, errorCode, json() }
```

Header names come back lowercased now, repeated headers get joined with `, `, and `cookies` is its own array instead of getting mashed into `headers` (you can't safely join multiple `Set-Cookie` values, so we stopped pretending you could).

If the request itself fails — timeout, DNS, TLS, whatever — nothing throws. You get `ok: false`, `status: 0`, and an `errorCode` you can actually branch on: `E_HTTP_TIMEOUT`, `E_HTTP_NETWORK`, `E_HTTP_TOO_MANY_REDIRECTS`, `E_HTTP_RESPONSE_TOO_LARGE`. There's also a `maxResponseBytes` option now (defaults to 10MB) so a misbehaving server can't make you buffer forever, and `maxRedirects` (default 10) with a way to get the raw redirect back if you set it to 0.

Also: you can't pass both `body` and `json` anymore. Pick one. And a `HEAD` request with a body is now an error instead of something weird happening silently.

### Server side

`createServer` now takes an options object for the stuff that used to be invisible defaults — body size limits, header limits, timeouts for reading/writing/idling, and a `handlerTimeout` so a stuck handler gets a `504` instead of hanging a connection forever:

```lx
val server = http.createServer(handle, { maxBodyBytes: 65536 })
server.listen(3000)
```

Added an `onError` hook (`fn(err, req, res)`) that runs before the server falls back to its own default error response. If your handler throws and you haven't sent anything yet, Lunex now sends the right status from an `HttpError` if you threw one, or a plain 500 otherwise, and logs it to stderr so it doesn't vanish silently.

`req` picked up a few things it should've had from the start: `rawPath` (still percent-encoded, in case you need it), `queryAll` (every value per query param, as arrays — useful for `?tag=a&tag=b`), and a proper `.header(name)` lookup that's case-insensitive.

`res` is more or less the same surface but tightened up — calling `.json()` twice now throws `E_HTTP_RESPONSE_FINISHED` instead of just sending garbage, and every response gets `X-Content-Type-Options: nosniff` automatically.

### Routing moved out

If you were using the built-in `server.get(...)` / `.post(...)` chaining before — that's gone from `std.http` itself. It's now `std.http.router`:

```lx
val router = @import("std.http.router")

val api = router.create([
  router.get("/users/:id", users.show),
  router.post("/users", users.create),
  router.delete("/users/:id", users.remove)
])

api.listen(3000)
```

Routes live in a trie now instead of a flat list, so lookup time depends on path length, not how many routes you've registered. `:name` params and trailing `*` wildcards both work the way you'd expect. No built-in middleware chain anymore — if you want shared behavior across routes, wrap the handler yourself. I know some people will miss `.use()`, but honestly the old middleware ordering had edge cases that were hard to explain, let alone debug.

Static file serving is its own thing now too (`std.http.static`), separate from both of the above.

This is a breaking change if you were using the old router API directly off `std.http`. Sorry — but the new error codes (`E_ROUTER_NOT_FOUND`, `E_ROUTER_METHOD_NOT_ALLOWED`, `E_ROUTER_BAD_PATH`) and the trie-based matching make it worth it.

## Native FFI (`std.ffi`) — brand new

This is a big one. Lunex can now call into native shared libraries directly:

```bash
lunex ffi = on run main.lx
```

FFI is **off by default**, on purpose, and the switch only works as a CLI prefix — you can't flip it on from inside a Lunex program, and setting an environment variable won't do it either. That was a deliberate call: I didn't want a script to be able to silently turn on native calls just because something set an env var somewhere upstream.

The backend runs through `goffi` over a purego-compatible layer, so there's no CGo dependency and no C compiler needed to build Lunex itself. New error codes `E0120`–`E0133` cover the whole lifecycle — library load failures, missing symbols, bad signatures, allocation failures, the works. If you're doing native interop, read through `internal/std/ffi.go` and friends; there's a fair amount of surface area here (loaders for unix/windows, callback bridging, native memory handling) that didn't exist in 0.9.2 at all.

## `std.env` — also new

Small but genuinely useful:

```lx
val env = @import("std.env")

env.load()
val name = env.get("APP_NAME", "Lunex")
```

`env.get(key, default)` for the common "might not be set" case, `env.require(key)` when you want it to blow up loudly (`E0110`) if it's missing, and `env.config(path)` if you want the parsed object plus a structured error instead of a boolean. Dotenv file loading is built in.

## `std.testing` — also new

There's now an actual testing module instead of everyone rolling their own assert-and-log pattern. Didn't dig deep into the API surface for this changelog, but it's there and the test suite uses it internally now.

## Error handling: one scheme instead of two

This was the thing that annoyed me most about 0.9.2 so I'm glad it's fixed. Previously `lunex run` and `lunex check` used **completely different, independently-assigned numbering schemes** for errors. Same code, different meaning depending on which command produced it — `E0061` meant "assertion failed" at runtime but "wrong number of arguments" under `lunex check`. That's... not great. You'd search for an error code and find documentation for the wrong failure mode.

As of 0.9.3, `check` and `run` share the exact same `errfmt.LunexError` representation — same codes, same messages, same formatting. A diagnostic caught statically now reports with the identical code the runtime would've used for the same mistake. No more "which command produced this number" guessing game.

Also fixed: `E0010` used to be double-booked for both "module not found" and "stack overflow." Stack overflow now has its own code (`E0060`).

New error ranges added for the new modules: `E0110`–`E0111` for env, `E0120`–`E0133` for FFI, `E0112` for NAX packing failures.

## The build pipeline: `lunex build` is gone, say hi to `lunex pack`

```bash
lunex pack main.lx
lunex pack ./my-project -o dist/app.nax
```

Functionally similar idea — compile to a `.nax` archive — but the internals changed a lot. By default, `pack` now produces a `.nlo` compiled-object format instead of the old bytecode-based archive. The `.nlo` objects store a validated binary AST and skip the old NTZ bytecode section entirely. Original source text is now preserved by default so `lunex unpack` does not rewrite syntax or discard comments. Use `--no-source` only when exact recovery is not needed; in that mode, unpacking reconstructs canonical source from the stored AST. The legacy `--source` flag remains accepted.

The practical upshot: running a packed `.nax` file skips source reading, lexing, parsing, and resolution at startup, and uses lazy stdlib initialization. It's still the same Go interpreter underneath — no new execution backend, no CGo — just less redundant work at launch for trusted, already-checked code.

Also folded into this release: `lunex check` and `lunex see_errors` got clearer separation of concerns in the docs (they were previously described in a confusing way given the error-code unification above), and the package-management commands (`install`, `add`, `list`, `remove`, `update`) got their docs cleaned up with actual usage examples instead of bare command names.

## Smaller stuff

- **JWT, URL, and `http.parseURL`** — `parseURL` now returns a fuller object (`protocol`, `username`, `host`, `hostname`, `port`, `path`, `query`, `search`, `hash` — it was missing several of these). Added `parseQuery` / `buildQuery` as standalone helpers.
- **Cookies** got their own proper parsing/serialization path (`http.parseCookies`, `http.serializeCookie`) with real validation instead of just string-smashing.
- **`http.status`** now exposes named constants (`OK`, `NOT_FOUND`, `UNPROCESSABLE_ENTITY`, etc.) instead of making you remember magic numbers.
- **Go version bump** — minimum Go version is now 1.25 (was 1.23).
- **Termux note** — README now explicitly calls out Android support through Termux rather than just "Android" vaguely.
- Removed a handful of internal-only files that were really just scaffolding left over from earlier development (`bench.lx`, the old JIT experiment under `internal/jit`, a few now-redundant test files). The JIT stuff in particular wasn't going anywhere good — pulling it out now rather than letting it rot.
- A couple of HTTP test files got split up (`02_http_router.lx`, `03_http_errors_and_limits.lx`, `04_http_static.lx`, `05_http_url_cookies.lx`) to match the new module boundaries instead of being crammed into one file.

## Breaking changes, summarized

If you're upgrading from 0.9.2, here's what will actually bite you:

1. `server.get/post/put/...` chaining directly off `std.http.createServer()` is gone — you need `std.http.router` now.
2. `lunex build` doesn't exist anymore — use `lunex pack`. Output format changed too (`.nlo` instead of the old bytecode archive by default).
3. If you were pattern-matching on error codes from `lunex check`, double check them against the new unified scheme — some codes that used to mean one thing under `check` now align with the `run` meaning instead.
4. HTTP client options are now strictly validated — if you were passing extra/misspelled options before and they were just getting ignored, they'll throw now.

That's it for this one. As always, open an issue if something in here breaks your project in a way that isn't mentioned above — there's a decent chance it's a doc gap rather than intentional.