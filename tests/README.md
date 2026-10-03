# Lunex Test Suite

129 tests covering the major language and standard library features.

## Running all tests

```bash
./tests/run_all.sh

./tests/run_all.sh ./lunex
```

## Running a single test

```bash
lunex run tests/variables/01_val_basic.lx
```

## Test categories

| Category | Tests | What is covered |
|---|---|---|
| `variables/` | 10 | `val`, `var`, strings, numbers, booleans, null, arrays, objects, destructuring |
| `functions/` | 10 | Basic functions, closures, higher-order, recursion, first-class, memoize, implicit/explicit return |
| `control_flow/` | 10 | `if`/`else`, nested conditions, `guard`, `unless`, `match` literals, `match` ranges, `match` as expression, `defer`, short-circuit |
| `loops/` | 10 | `while`, `break`, `continue`, `each` over arrays/strings/objects, nested loops |
| `structs/` | 8 | Fields, methods, `this`, factory functions, composition, event emitter, linked list, state machine, builder |
| `stdlib/io/` | 5 | `log`, `warn`, `err`, `info`, `success`, `table`, template formatting, spinner |
| `stdlib/math/` | 5 | `sqrt`, `pow`, `abs`, `floor`, `ceil`, `round`, constants, trig, random, statistics |
| `stdlib/utils/` | 5 | String helpers, array helpers, object helpers, UUID, range |
| `stdlib/json/` | 6 | Parse, stringify, validate, round-trip, and save JSON |
| `stdlib/datetime/` | 5 | `now`, `format`, `diff`, `sleep`, `parse` |
| `stdlib/crypto/` | 5 | SHA-256, MD5, Base64, UUID v4, HMAC-SHA256, AES encrypt/decrypt |
| `stdlib/fs/` | 5 | Write/read, exists, append, readLines, stat |
| `stdlib/os/` | 5 | Platform, env vars, exec, args, cwd |
| `stdlib/regex/` | 5 | `match`, `findAll`, `replace`, `split`, capture groups |
| `stdlib/testing/` | 2 | Test registration, groups, lifecycle hooks, parameterized cases, assertions, retries, tags, snapshots, and summaries |
| `stdlib/ffi/` | 8 | FFI permission state, native library loading, symbol binding, calls, pointers, memory, strings, callbacks, descriptors, and ABI metadata |
| `stdlib/ints/` | 4 | Integer and bitwise operations |
| `stdlib/buffer/` | 4 | Buffer creation, conversion, slicing, and operations |
| `stdlib/http/` | 7 | Server basics, router matching, limits and timeouts, static files, URL and cookie helpers |
| `concurrency/` | 5 | Basic channel, fan-out, pipeline, worker pool, collect |
| `advanced/` | 9 | Closures, data pipeline, observer, retry/backoff, lazy eval, middleware, event loop, runtime introspection, comprehensive |

**Total: 138 tests**

## Test file naming

Each file is named `NN_description.lx` where `NN` is a two-digit index within its category. Each test prints `PASS <name>` on success. The runner checks the exit code.

## Legacy tests

The original test scripts are also preserved at the top of `tests/`:

| File | Description |
|---|---|
| `test.lx` | General language smoke test |
| `test_io.lx` | I/O module test |
| `test_timing.lx` | `std.utils.now()` timestamp sanity check |
| `test_utils.lx` | Utils module test |
| `test_xml.lx` | XML parsing (requires `lune-xml` package) |
| `main.lx` | Local module import via `@fimport` |
| `math.lx` | Math module test |
