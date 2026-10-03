# Error Reference

Lunex assigns a code to every diagnostic it can detect. This document lists
the codes that the current build actually emits, grouped by where they come
from.

Error codes follow the pattern `E####` (or `W####` for warnings, `S####`
for runtime "suspect" warnings). `lunex run` and `lunex check` use the same
error-code system.

---

## How to read an error

```
✗ error[scope][E0001]: variable `usr` was not defined
  ──▶ main.lx:12:3

 10 │
 11 │  fn greet(user) {
 12 │    io.log(usr.name)
    │           ^^^
    │           ╰── did you mean `user`? (closest match by name)
```

| Field                       | Meaning                                                |
|-------------------------------|------------------------------------------------------------|
| `error[scope]`                 | Error category (see the category table below)               |
| `E0001`                         | Error code                                                    |
| The rest of the first line       | The actual error message                                       |
| `main.lx:12:3`                   | File, line, column                                               |
| Source window                     | A few lines of context around the error                           |
| `^^^`                               | Underline pointing at the problem                                   |
| `╰──` note                          | Suggestion or extra context (where available)                        |

---

## `lunex run` and `lunex check` use the same diagnostics

`lunex check` performs the same source validation rules used before
execution by `lunex run`, but stops before the program is executed. Both
commands use the same `errfmt.LunexError` representation, error codes,
messages, source locations, suggestions, notes, and formatting.

A diagnostic discovered statically is therefore reported with the same code
and message that the runtime uses for the corresponding failure. `check` does
not maintain a second error-code namespace.

## Error categories

The bracketed word before the code (`error[scope]`, `error[type]`, etc.)
comes from one of these categories:

| Label                  | Meaning                                  |
|--------------------------|---------------------------------------------|
| `error[lex]`               | Tokenizing failed                              |
| `error[parse]`              | The token stream doesn't form valid syntax       |
| `error[syntax]`              | A structural rule was violated (e.g. top-level code) |
| `error[type]`                 | A type-related problem                              |
| `error[runtime]`               | Failed while evaluating a program                    |
| `error[scope]`                   | A name couldn't be resolved, or was reassigned incorrectly |
| `error[module]`                    | An `@import`/`@fimport` problem                       |
| `error[io]`                          | A file/stream operation failed                          |
| `error[assertion]`                    | An `assert` failed                                        |
| `error[range]`                          | An index or slice was out of bounds                          |
| `error[permission]`                       | The OS denied an operation                                     |
| `error[overflow]`                           | A numeric value exceeded its type's bounds                       |
| `error[arithmetic]`                           | An arithmetic operation failed (e.g. division by zero)             |
| `error[encoding]`                               | Malformed JSON or similar encoded data                                |
| `error[recursion]`                                | Call stack depth exceeded                                               |
| `error[suspect]`                                    | A runtime "suspect" warning (see below) — treated as an error by default |
| `warning[deprecated]`                                 | A deprecated name was used                                                 |

---

## Parser and runtime errors (`lunex run`, REPL)

### Scope and reference (E0001–E0006)

| Code   | Message you'll see                          | Cause                                                   |
|--------|-----------------------------------------------|------------------------------------------------------------|
| E0001  | `variable <name> was not defined`             | Variable used before it was declared, or misspelled       |
| E0002F | `'<name>' is not defined` (function form)         | A function call target doesn't resolve to anything          |
| E0003  | `'<name>' is not a function`                        | Tried to call something that isn't a function                  |
| E0004  | `cannot read property of null: '<name>'`              | Field or method access on a `null` value                         |
| E0005  | `cannot reassign immutable binding '<name>'`            | Attempted to reassign a `val` binding                               |

> `E0002` (`undefined method`) and `E0006` (`undefined field`) are
> reserved in the code registry for method/field lookup failures but have
> no confirmed live call site in this build — if you ever see one, the
> message text will tell you more than the code does. `E0002F` is the code
> actually used for calling an undefined function.

### Type and arithmetic errors (E0020–E0032)

| Code  | Message you'll see                          | Cause                                                   |
|-------|-----------------------------------------------|------------------------------------------------------------|
| E0020 | type-mismatch messages (varies by operation)     | An operation was applied to incompatible types (e.g. adding a number to a struct) |
| E0030 | `division by zero`                                 | Division or modulo by zero                                     |

E0021–E0025 (arity, argument type, return type, missing operator,
coercion failure) and E0031–E0032 (integer overflow, NaN/Inf result) are
defined in the registry with titles, but this build's runtime doesn't
currently raise them directly — coercion/overflow problems generally
surface as `E0020` today, with the specific type names in the message.

### Module errors (E0010–E0015)

| Code   | Message you'll see                                    | Cause                                              |
|--------|-----------------------------------------------------------|---------------------------------------------------------|
| E0010  | `module not found: '<path>'`                                 | `@import` target could not be resolved                     |
| E0012  | `circular import: '<path>' is already being loaded`             | Two or more modules import each other in a cycle             |

E0010F (local file not found), E0011 (module has a syntax error), E0013
(module failed to load), E0014 (internal module), and E0015 (binary module
load failed) are registered but not confirmed to be live in this build.

### Syntax errors (E0050–E0055, E1000–E1010)

| Code    | Message you'll see                                    | Cause                                                  |
|---------|-------------------------------------------------------|----------------------------------------------------------|
| E0050   | generic parse/syntax error                                | A general parsing failure that doesn't fit a more specific code |
| E0052   | `unclosed block — '{' on line <n> was never closed with '}'` | A `{` was never closed                                  |
| E1000   | `unexpected token: '<tok>'` (general case)                | A token appeared where it wasn't expected                    |
| E1001   | `unexpected token: ','`                                      | Stray or misplaced comma                                        |
| E1002   | `unexpected token: ')'`                                        | Extra or mismatched closing parenthesis                            |
| E1003   | `unexpected token: '}'`                                          | Extra or mismatched closing brace                                    |
| E1004   | `unexpected token: ']'`                                            | Extra or mismatched closing bracket                                    |
| E1005   | `unexpected token: '='`                                              | Misused `=` (e.g. in a comparison — use `==`)                            |
| E1006   | `unexpected token: ';'`                                                | Stray semicolon                                                            |
| E1010   | `expected ...` (general case)                                            | A required token was missing                                                  |

E0051 (unexpected EOF), E0053 (invalid escape sequence), E0054 (invalid
number literal), and E0055 (duplicate object key) are registered but the
live "unexpected token"/"expected" paths in this build raise the more
specific E1000-series codes above instead.

### Reserved keywords (E0073, E0076, E0077)

| Code  | Message you'll see                                    | Cause                                              |
|-------|-----------------------------------------------------------|-----------------------------------------------------------|
| E0073 | `'<word>' is a reserved keyword`                          | A reserved Lunex keyword was used as an identifier             |
| E0076 | `'<word>' is a reserved keyword` (as a parameter)            | A reserved keyword can't be used as a parameter name              |
| E0077 | `'<word>' is a reserved keyword` (as a field)                  | A reserved keyword can't be used as a struct field name              |

### Encoding errors (E0067)

| Code  | Message you'll see                                | Cause                                        |
|-------|---------------------------------------------------------|------------------------------------------------|
| E0067 | `invalid JSON value: <text>` / `invalid JSON string: ...` | Malformed input to `std.json` parsing              |

### Entry point and top-level errors (E0070–E0072)

| Code  | Message you'll see                                    | Cause                                                    |
|-------|-----------------------------------------------------------|-----------------------------------------------------------------|
| E0070 | `entry point 'main' is not defined`                          | The file has no `fn main()`                                       |
| E0071 | `statement of type '<kind>' is not allowed at the top level`   | Executable code outside `fn main() { ... }`                        |
| E0072 | `explicit call to 'main()' is not allowed`                        | `main` is invoked automatically — user code can't call it directly |

### Runtime control-flow, I/O, and misc errors (E0060–E0101)

| Code  | Message you'll see                       | Cause                                                       |
|-------|----------------------------------------------|------------------------------------------------------------------|
| E0060 | `call stack depth exceeded (<n> frames)`         | Infinite or excessive recursion                                     |
| E0061 | `assertion failed: <expr>`                          | An `assert` failed at runtime |
| E0063 | I/O operation failed                                   | A file/stream operation failed                                            |
| E0064 | permission denied                                        | The OS denied an operation                                                   |
| E0065 | operation timed out                                        | An operation (e.g. a configured execution budget) exceeded its deadline        |
| E0080 | `` `return` is not valid outside a function body ``          | `return` used at the top level or outside any function                          |
| E0081 | `break` is not valid outside a loop                             | `break` used outside `while`/`for`/`each`/`repeat`/`loop`                          |
| E0082 | `continue` is not valid outside a loop                            | `continue` used outside a loop                                                       |

E0062 (explicit panic), E0066 (network error), E0068 (memory allocation
failure), E0069 (concurrent write detected), E0083–E0101 (invalid pattern,
not-implemented, and the format/crypto/db/auth/rate-limit/file/type/
nullable/uninitialized-const family) are all registered with titles and
suggestions, for use by the standard library and future checks, but don't
currently have a confirmed live call site outside the registry itself.

## Native FFI errors (E0120–E0133)

| Code  | Message you'll see | Cause |
|---|---|---|
| E0120 | `native FFI is disabled for this Lunex process` | A native FFI operation was requested without the CLI-only FFI switch |
| E0121 | `FFI library path must be a non-empty string` | The library path is missing or not a string |
| E0122 | `failed to load native library ...` | The operating-system dynamic loader rejected the library |
| E0123 | `symbol ... was not found` | The requested native symbol is unavailable |
| E0124 | `cannot bind ... from a closed native library` | The library or bound function has already been closed |
| E0125 | `... requires ...` | An FFI operation received the wrong number or shape of arguments |
| E0126 | `invalid FFI signature ...` | The signature or descriptor cannot be parsed |
| E0127 | `unsupported FFI type ...` | The declared ABI type is not supported by `std.ffi` |
| E0128 | `native call ... failed` | The native call could not be prepared or invoked safely |
| E0129 | `failed to create native callback` | The callback ABI could not be represented by the host callback bridge |
| E0130 | `native allocation ...` | Native allocation or reallocation failed |
| E0131 | `expected an FFI pointer ...` | A value could not be converted to the requested native representation |
| E0132 | `native memory ... exceeds ...` | A bounded native memory operation would exceed the known allocation length |
| E0133 | `invalid FFI option syntax` | The CLI FFI switch was not written as `ffi = on|off` |

All FFI diagnostics are emitted through the same `errfmt.LunexError` formatter
used by the rest of the language. FFI activation itself is checked before
native operations begin, so an application that leaves FFI disabled cannot load
a native library accidentally through source code or environment configuration.

## Environment errors (E0110–E0111)

| Code | Message you'll see | Cause |
|---|---|---|
| E0110 | `required environment variable is not set` | `env.require(key)` was called for a variable that does not exist |
| E0111 | `environment file operation failed` | A dotenv file could not be read or its parsed values could not be applied |

`env.require` uses the normal Lunex diagnostic formatter. When the requested
key is known, the runtime message identifies it directly, for example
`required environment variable 'PORT' is not set`.

Use `env.get(key, default)` when a missing variable is expected. Use
`env.config(path)` when the caller needs the parsed object and a structured file
error instead of a boolean result.

## NAX packing errors (E0112)

| Code | Message you'll see | Cause |
|---|---|---|
| E0112 | `NAX pack pipeline rejected the artifact` | An internal checker, resolver, compiler, or module-graph invariant failed during `lunex pack` before archive emission |

`E0112` is reserved for internal packing invariants, not ordinary source mistakes. Lexer, parser, resolver, checker, and module-graph diagnostics are reported first with their normal Lunex codes, and no `.nax` artifact is emitted while validation fails.

### Deprecation warning (W0001)

| Code  | Message you'll see              | Cause                                     |
|-------|-------------------------------------|------------------------------------------------|
| W0001 | `'<name>' is deprecated`               | Code referenced a name marked deprecated           |

This is a warning, not an error — it doesn't stop execution. W0002–W0010
(shadowed variable, unreachable code, unused variable, implicit coercion,
and several style warnings about spacing/semicolons/line length) are
registered but not currently emitted by this build.

---

## Runtime "suspect" warnings (S0001–S0007)

Separately from hard errors, the interpreter watches for patterns that are
almost always mistakes and flags them even though they don't stop the
program by themselves. These show up as `error[suspect][S000X]` by default.

| Code  | Short title                     | Typical cause                                        |
|-------|------------------------------------|------------------------------------------------------------|
| S0001 | for-of over non-iterable              | `for ... of` (or `each ... in`) target isn't iterable    |
| S0002 | match produced no result                | A `match` expression had no matching arm and no default  |
| S0003 | arithmetic produced NaN                   | An operand wasn't a valid number                            |
| S0004 | array index out of bounds                   | An index fell outside the valid range                          |
| S0005 | spread of non-iterable                        | `...value` was used on something that can't be spread            |
| S0006 | spread of null or undefined                     | The spread target was `null` or `undefined`                         |
| S0007 | call on undefined return                          | A function call's result was `undefined` where a value was expected  |

---

## Filing a bug

If you hit an error whose message looks like an internal inconsistency
rather than a mistake in your own code, please open an issue at:

```
https://github.com/Megamexlevi2/lunex-language/issues
```

Include:
- Lunex version (`lunex version`)
- Platform info (`lunex platform`)
- A minimal `.lx` file that reproduces the issue
- The complete error output, including which command produced it
  (`lunex run` vs. `lunex check`)

## `watch`

### E0114 — Invalid watch declaration

The runtime received an invalid `watch` declaration. Source code normally catches this earlier during parsing. A valid target is a variable or member expression followed by a block.
