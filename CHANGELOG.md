
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