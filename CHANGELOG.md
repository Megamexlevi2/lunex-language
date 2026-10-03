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

Functionally similar idea — compile to a `.nax` archive — but the internals changed a lot. By default, `pack` now produces a `.nlo` compiled-object format instead of the old bytecode-based archive. The `.nlo` objects store a validated binary AST and skip the old NTZ bytecode section entirely. Source text isn't embedded by default anymore (pass `--source` if you want the original `.lx` preserved in the archive for debugging or recovery — and even without that flag, `lunex unpack` can reconstruct readable source from the stored AST).

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
