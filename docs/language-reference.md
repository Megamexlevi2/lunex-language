# Language Reference

Complete reference for the Lunex programming language, version 0.9.3.

---

## Lexical structure

### Comments

```lx

```

### Identifiers

Identifiers start with a letter or underscore and may contain letters, digits,
and underscores. Case-sensitive.

```lx
myVar   _private   score99   HTTP_PORT
```

### Keywords

```
val var fn struct if else elif guard unless while watch each in for
loop repeat do break continue match case default when defer spawn channel
try catch finally throw raise enum namespace component import export from
require module as extends new this super interface implements trait typeof
instanceof keyof infer alias void undefined range sleep have ifhave ifset
between matches startsWith endsWith select macro immutable freeze with
using assert delete use satisfies readonly private public protected static
abstract override get set null true false
```

Not every keyword above has full runtime support; some are recognized by
the parser but only partially implemented, or reserved for future use. The
sections below document the ones with confirmed, working behavior.

> **`return` is a reserved word, not a working keyword.** Lunex's parser
> recognizes `return` specifically in order to reject it with a pointed
> error message ("Lunex does not use 'return' — the last expression in a
> function is its result automatically") rather than treating it as a plain
> unknown identifier. The last expression in a function body is always its
> result — there is no way to return early from the middle of a function
> body other than by structuring the code (e.g. with `if`/`else` producing
> the value on every branch). `class` gets the same treatment: it's
> rejected with a hint to use `val TypeName = struct { ... }` instead.

### Literals

| Kind     | Examples                                 |
|----------|------------------------------------------|
| Number   | `0` `42` `3.14` `-1` `1e6`              |
| String   | `"hello"` `"line\nbreak"` `"tab\there"` |
| Template | `` `Hello, ${name}!` ``                 |
| Boolean  | `true` `false`                           |
| Null     | `null`                                   |
| Array    | `[1, 2, 3]`                              |
| Object   | `{ key: value, other: 42 }`              |

---

## Bindings

### `val` — immutable binding

```lx
val x    = 42
val name = "Lunex"
val pi   = 3.14159
```

`val` bindings cannot be reassigned. Trying to do so is a compile-time error.
`const` is accepted as an alias for `val` — the two behave identically.

### `var` — mutable binding

```lx
var count = 0
count = count + 1
```

`let` is accepted as an alias for `var` — the two behave identically.

### Destructuring

Object destructuring:

```lx
val { name, role, score } = getUser()
```

Array destructuring:

```lx
val [first, second, third] = getItems()
```

---

## Types

Lunex is dynamically typed. The runtime types are:

| Type       | Description                         |
|------------|-------------------------------------|
| `number`   | 64-bit IEEE 754 floating point      |
| `string`   | UTF-8 string                        |
| `boolean`  | `true` or `false`                   |
| `null`     | absence of a value                  |
| `array`    | ordered collection                  |
| `object`   | key-value map                       |
| `struct`   | object with methods                 |
| `function` | first-class callable                |
| `channel`  | concurrent message-passing channel  |

Use `typeof(v)` anywhere to inspect a value's type at runtime:

```lx
typeof("hello")  
typeof(42)       
typeof(true)     
typeof(null)     
typeof([])       
typeof({})       
```

---

## Operators

### Arithmetic

```lx
a + b    
a - b    
a * b    
a / b    
a % b    
```

### Comparison

```lx
a == b   
a != b   
a < b    a > b    a <= b    a >= b
```

### Logical

```lx
a && b   
a || b   
!a       
```

### Bitwise

Bitwise operators treat both operands as 64-bit integers.

```lx
a & b    
a | b    
a ^ b    
~a       
a << b   
a >> b   
a >>> b  
```

For overflow behavior that matches C's fixed-width integer types (`uint8_t`,
`int32_t`, and so on), see [`std.ints`](stdlib.md#std-ints--fixed-width-integer-arithmetic).

### Assignment

```lx
x = expr          
obj.field = expr  
arr[i] = expr     
```

---

## Global built-in functions

These are available everywhere without an import:

| Function          | Description                                   |
|-------------------|-----------------------------------------------|
| `str(v)`          | Convert any value to its string representation |
| `num(v)`          | Convert a value to a number                   |
| `typeof(v)`       | Return the type name as a string              |
| `parseInt(s)`     | Parse a string as an integer                  |
| `parseFloat(s)`   | Parse a string as a floating-point number     |
| `isNaN(v)`        | True if the value is NaN                      |
| `isFinite(v)`     | True if the value is a finite number          |
| `len(v)`          | Length of an array, string, or object         |
| `channel()`       | Create an unbuffered concurrent channel       |

```lx
str(42)          
str(true)        
num("3.14")      
typeof("hello")  
parseInt("10")   
isNaN(0 / 0)     
len([1, 2, 3])   
```

### JavaScript-compatibility globals

Lunex also exposes a set of globals that mirror common JavaScript built-ins,
for porting convenience. They are real, usable objects/functions — not just
reserved names — but a few have limited or stubbed behavior, noted below.

| Global                    | Description                                             |
|------------------------------|-----------------------------------------------------------|
| `String(v)` / `Number(v)` / `Boolean(v)` | Type-conversion functions, similar to `str`/`num`   |
| `io.log(...)`                 | Log values to stdout through `std.io`                      |

Global `print(...)`, `println(...)`, and `log(...)` output functions are not part of the Lunex language. Use `val io = @import("std.io")` followed by `io.log(...)`.
| `Array`                          | Array constructor/statics (e.g. `Array.isArray`)             |
| `Object`                          | Object statics (e.g. `Object.keys`, `Object.values`)            |
| `Math`                              | A JS-style `Math` object, independent from `std.math`             |
| `JSON`                                | `JSON.stringify` / `JSON.parse`, independent from `std.json`         |
| `Map` / `Set`                          | Constructors for map/set-like collections                              |
| `Error` / `TypeError` / `RangeError`     | Construct a plain `{ name, message }`-shaped error object              |
| `Promise`                                  | A minimal `Promise`-shaped object — see caveat below                     |
| `setTimeout(fn, ms)` / `setInterval(fn, ms)` / `clearInterval(id)` | Timers      |
| `clearTimeout(id)`                            | **No-op** — see caveat below                                                |
| `performance`                                    | `{ now() }` for high-resolution timing                                       |
| `process`                                          | `{ env, argv, ... }` — see caveat below                                        |
| `encodeURIComponent(s)` / `decodeURIComponent(s)`    | URI component encoding                                                          |
| `encodeURI(s)` / `decodeURI(s)`                        | Full-URI encoding                                                                  |
| `btoa(s)` / `atob(s)`                                    | Base64 encode/decode                                                                 |

> **Known limitations of the JS-compatibility layer:**
> - `Promise.all(arr)` does not actually await or resolve anything — it
>   returns the input array unchanged. There is no real async scheduling
>   behind these globals; don't rely on `Promise` for real concurrency, use
>   `spawn`/`channel()` instead (see [Concurrency](#concurrency)).
> - `clearTimeout(id)` is a complete no-op — it never cancels a pending
>   `setTimeout`. `clearInterval(id)` does work correctly.
> - `process.env` and `process.argv` are always empty (an empty object and
>   an empty array, respectively) — they do not reflect the real
>   environment or command-line arguments. Use `std.env` and `std.os.args`
>   for that instead.
> - `Error`/`TypeError`/`RangeError` produce plain data objects, not
>   throwable/catchable exception objects with stack traces — Lunex's error
>   handling is done through its own diagnostic system (see
>   [Error messages](#error-messages)), not through these constructors.

---

## Functions

### Declaration

```lx
fn add(a, b) {
  a + b
}
```

The last expression in the body is the return value. There is no `return`
keyword.

### Anonymous functions

```lx
val square = fn(x) { x * x }
```

### Immediately invoked

```lx
val result = fn(a, b) { a + b }(3, 4)
```

### First-class functions

Functions are values — pass them as arguments, return them, store them in arrays
or objects.

```lx
fn apply(f, x) { f(x) }

fn compose(f, g) {
  fn(x) { f(g(x)) }
}

val ops = [
  fn(x) { x + 1 }
  fn(x) { x * 2 }
]
```

### Closures

Functions capture variables from the enclosing scope, including mutations.

```lx
fn makeAdder(n) {
  fn(x) { x + n }
}

val add5 = makeAdder(5)
io.log(add5(10))  
```

---

## Control flow

### `if` / `else if` / `else`

```lx
if score >= 90 {
  "A"
} else if score >= 80 {
  "B"
} else {
  "C"
}
```

`if` is an expression — its value can be assigned:

```lx
val label = if n > 0 { "positive" } else { "non-positive" }
```

`elif` is accepted as a shorthand for `else if`:

```lx
if score >= 90 {
  "A"
} elif score >= 80 {
  "B"
} else {
  "C"
}
```

### `try` / `catch` / `finally`

```lx
try {
  val result = riskyOperation()
  io.log(result)
} catch (e) {
  io.error("failed:", e)
} finally {
  io.log("cleanup")
}
```

`catch` accepts an optional parameter name for the caught error, either
parenthesized (`catch (e)`) or bare (`catch e`); both `catch` and
`finally` are optional. `throw expr` and `raise expr` are interchangeable
aliases and both raise the given value to be caught by an enclosing `try`.

Lunex also has a "safe try" expression form, `try? expr`, which evaluates
`expr` and suppresses any error it raises.

### `guard`

Runs the `else` block when the condition is **false**, then continues execution.
The most common use is early-out / safety checks at the top of a function.

```lx
fn process(user) {
  guard user != null else {
    io.err("no user — skipping")
  }
  
  io.log("processing:", user)
}
```

### `unless`

Runs its block when the condition is **false**. A cleaner alternative to
`if !condition { ... }`:

```lx
unless connected {
  io.warn("no connection — retrying")
}
```

### `match`

Tests a value against exact arms, top-to-bottom. First match wins.

```lx
val label = match status {
  "ok"      => "success"
  "pending" => "waiting"
  "fail"    => "error"
  _         => "unknown"
}
```

`match` is an expression and can be used as a value. The `_` wildcard catches
anything not already matched.

For range-based classification, use `if` / `else if` chains instead:

```lx
fn classify(n) {
  if n == 0       { "zero" }
  else if n <= 9  { "single digit" }
  else if n <= 99 { "double digit" }
  else            { "large" }
}
```

### `defer`

Schedules a block to run when the enclosing function exits, regardless of how it
exits. Multiple defers run in reverse order (LIFO).

```lx
fn readAndProcess(path) {
  val fs = @import("std.fs")
  defer { io.log("done with", path) }
  fs.readFile(path)
}
```

---

## Loops

### `while`

```lx
var i = 0
while i < 10 {
  io.log(i)
  i = i + 1
}
```

Use `break` to exit early, `continue` to skip to the next iteration.

### `each … in`

Iterate over an array:

```lx
each name in ["Alice", "Bob", "Carol"] {
  io.log("Hello,", name)
}
```

Iterate over a string (character by character):

```lx
each ch in "lunex" {
  io.log(ch)
}
```

Iterate over an object (yields each key):

```lx
each key in config {
  io.log(key, "=", config[key])
}
```

### `for`

`for` is an alternative to `each … in` with the same iteration semantics,
plus an optional index/key variable:

```lx
for name in ["Alice", "Bob", "Carol"] {
  io.log(name)
}

for value, index in ["Alice", "Bob", "Carol"] {
  io.log(index, value)
}
```

`of` is accepted as a synonym for `in`. `for each name in x { ... }` is
also accepted, behaving the same as `each name in x { ... }`.

### `repeat`

Runs a block a fixed number of times, or forever if no count is given:

```lx
repeat 5 {
  io.log("tick")
}

repeat {
  
}
```

### `watch`

`watch` registers a reactive block that runs when the value of a variable or member expression changes. The block does not run when it is registered; it runs after a later assignment produces a different value.

```lx
var counter = 0

watch counter {
  io.log("Counter changed: " + counter)
}

counter = 10
counter = 20
```

Output:

```text
Counter changed: 10
Counter changed: 20
```

Member expressions are supported as well:

```lx
val user = struct {
  name = "Alice"
}

watch user.name {
  io.log("Name: " + user.name)
}

user.name = "Bob"
```

A `watch` target must be an identifier or a member expression. The watched value is captured when the watcher is registered and compared after Lunex assignments.

---

### `loop`

An explicit infinite loop, exited with `break`:

```lx
var i = 0
loop {
  i = i + 1
  if i > 3 { break }
}
```

---

## Native array methods

No import needed — arrays have these built in:

| Method                      | Returns        | Description                                        |
|-----------------------------|----------------|----------------------------------------------------|
| `arr.length`                | number         | Number of elements                                 |
| `arr.push(v)`               | —              | Append a value in place                            |
| `arr.pop()`                 | value          | Remove and return the last element                 |
| `arr.map(fn)`               | array          | New array with each element transformed            |
| `arr.filter(fn)`            | array          | New array of elements where fn is truthy           |
| `arr.reduce(fn, init)`      | value          | Fold the array left to a single value              |
| `arr.find(fn)`              | value \| null  | First element where fn is truthy                   |
| `arr.includes(v)`           | boolean        | True if the value is in the array                  |
| `arr.indexOf(v)`            | number         | Index of first occurrence (−1 if absent)           |
| `arr.sort()`                | array          | Sort ascending (returns new array)                 |
| `arr.reverse()`             | array          | Reverse order (returns new array)                  |
| `arr.slice(start, end)`     | array          | Sub-array from start to end (exclusive)            |
| `arr.join(sep)`             | string         | Concatenate elements with a separator              |
| `arr.every(fn)`             | boolean        | True if all elements satisfy fn                    |
| `arr.some(fn)`              | boolean        | True if any element satisfies fn                   |
| `arr.flat()`                | array          | Flatten one level of nesting                       |

```lx
val nums = [3, 1, 4, 1, 5, 9, 2, 6]
io.log(nums.sort())                              
io.log(nums.filter(fn(x) { x > 4 }))           
io.log(nums.map(fn(x) { x * 2 }))              
io.log(nums.reduce(fn(acc, x) { acc + x }, 0)) 
```

---

## Native string methods

No import needed — strings have these built in:

| Method                     | Returns | Description                                    |
|----------------------------|---------|------------------------------------------------|
| `s.length`                 | number  | Number of UTF-8 characters                     |
| `s.toUpperCase()`          | string  | Convert to uppercase                           |
| `s.toLowerCase()`          | string  | Convert to lowercase                           |
| `s.trim()`                 | string  | Remove leading and trailing whitespace         |
| `s.trimStart()`            | string  | Remove leading whitespace                      |
| `s.trimEnd()`              | string  | Remove trailing whitespace                     |
| `s.startsWith(prefix)`     | boolean | True if string starts with prefix              |
| `s.endsWith(suffix)`       | boolean | True if string ends with suffix                |
| `s.includes(sub)`          | boolean | True if string contains sub                    |
| `s.indexOf(sub)`           | number  | Index of first occurrence (−1 if absent)       |
| `s.split(sep)`             | array   | Split on separator                             |
| `s.slice(start, end)`      | string  | Substring from start to end (exclusive)        |
| `s.replace(old, new)`      | string  | Replace first occurrence                       |
| `s.replaceAll(old, new)`   | string  | Replace all occurrences                        |
| `s.repeat(n)`              | string  | Repeat the string n times                      |

```lx
"  hello  ".trim()              
"lunex".toUpperCase()           
"hello world".includes("world") 
"hello".split("")               
```

---

## Structs

A `struct` is a value with named fields and methods. Factory functions are the
standard way to create structs in Lunex — no `class` keyword needed.

```lx
fn Counter(start) {
  var count = start
  struct {
    fn inc()   { count = count + 1 }
    fn dec()   { count = count - 1 }
    fn value() { count }
    fn reset() { count = 0 }
  }
}

val c = Counter(0)
c.inc()
c.inc()
io.log(c.value())  
```

Plain assignments inside a `struct` body become fields. Methods can reference
the struct instance through a captured variable or `this`:

```lx
fn Animal(name, sound) {
  val self = struct {
    name  = name
    sound = sound

    fn speak() {
      self.name + " says " + self.sound
    }
  }
  self
}

val cat = Animal("Cat", "Meow")
io.log(cat.speak())  
```

You can also create simple structs inline without a factory function:

```lx
val user = struct {
  name = "Alice"
  role = "admin"
}
io.log(user.name)  
```

---

## Concurrency

### `channel()`

Creates an unbuffered FIFO channel:

```lx
val ch = channel()
```

### `ch.send(value)` / `ch.recv()`

`send` blocks until a receiver is ready. `recv` blocks until a sender sends.

```lx
spawn fn() {
  ch.send(compute())
}()

val result = ch.recv()
```

### `spawn`

Launches a function call in a new goroutine:

```lx
spawn fn() {
  heavyWork()
}()
```

---

## Modules

### Standard library

```lx
val io   = @import("std.io")
val math = @import("std.math")
val http = @import("std.http")
```

See [stdlib.md](stdlib.md) for the full module reference.

### Local files

```lx
val utils = @fimport("./src/utils.lx")
val lib   = @fimport("./build/math.nax")
```

### External packages

```lx
val pkg = @import("github.com/user/repo")
val pkg = @import("https://example.com/mylib")
```

---

## Error messages

Lunex error messages include a source window, a caret pointing at the problem,
and an automatic fix suggestion where possible. Every error carries a code
(`E0001`–`E0071`) for reference in the error documentation.

```
error[E0021] UndefinedVariable: 'usr' is not defined
   → main.lx:12:3
 10 │
 11 │  fn greet(user) {
 12 │    io.log(usr.name)
    │           ^^^
    │           did you mean 'user'?
```

---

## Type conversions

Lunex doesn't implicitly coerce between types. Use the global built-in
functions for explicit conversions:

```lx
str(42)           
str(true)         
num("3.14")       
parseInt("10")    
parseFloat("1.5") 
```

String concatenation with `+` does coerce the right-hand side to a string when
the left-hand side is a string:

```lx
"score: " + 99    
"items: " + 5     
```

---

## Scope rules

- Variables are lexically scoped to the block they are declared in.
- `fn` declarations at the top level of a file are visible throughout the file.
- Closures capture variables by reference — mutations are visible through the closure.
- `@import` and `@fimport` are scoped to the binding they are assigned to.
