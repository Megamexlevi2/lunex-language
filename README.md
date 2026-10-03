Lunex

A fast, stable scripting language for backend development.

Lunex is a statically scoped scripting language built in Go.
It is designed to be readable, practical, and consistent for everyday backend work. The language includes a built-in standard library with support for HTTP, file system access, cryptography, databases, WebSockets, and more.

It runs on Linux, macOS, Windows, and Android.

Two ready-made modules written in Lunex are included: lune-xml and lunex-cli.

---

## Installation

### Pre-built binary

Download the binary for your platform from the
[releases page](https://github.com/Megamexlevi2/lunex-language/releases).

### Build from source

Requires Go 1.25 or later.

```bash
git clone https://github.com/Megamexlevi2/lunex-language
cd lunex-language
./build.sh
```

#### Using Make

```bash
git clone https://github.com/Megamexlevi2/lunex-language
cd lunex-language
make
```

#### Using CMake

```bash
git clone https://github.com/Megamexlevi2/lunex-language
cd lunex-language
cmake -B build
cmake --build build
```

---

## Quick Start

```bash
cat << 'EOF' > hello.lx
val io = @import("std.io")

fn main() {
  io.log("Hello, World!")
}
EOF

lunex run hello.lx
```

---

## Language at a Glance

### Variables

`val` is immutable. `var` is mutable.

```lx
val name   = "Lunex"
val pi     = 3.14159
val active = true

var counter = 0
counter = counter + 1
```

Destructuring works on objects and arrays:

```lx
val { name, role } = user
val [first, second] = items
```

### Functions

The last expression in a function body is its return value.
Lunex has no `return` keyword.

```lx
fn add(a, b) {
  a + b          
}

val square = fn(x) { x * x }

io.log(add(2, 3))    
io.log(square(5))    
```

### Structs

No `class` keyword. Factory functions return a `struct`:

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

### Control Flow

```lx
if n < 0 {
  "negative"
} else if n == 0 {
  "zero"
} else {
  "positive"
}
```

`guard` runs its `else` block when the condition is **false**:

```lx
guard user != null else {
  io.err("no user provided")
}

```

`unless` runs its block when the condition is **false**:

```lx
unless connected {
  io.warn("not connected — retrying")
}
```

`watch` runs a block whenever a watched variable or member expression changes:

```lx
var counter = 0

watch counter {
  io.log("Counter changed: " + counter)
}

counter = 10
counter = 20
```

`match` tests exact values — top-to-bottom, first match wins:

```lx
val label = match status {
  "ok"      => "success"
  "pending" => "waiting"
  "fail"    => "error"
  _         => "unknown"
}
```

### Loops

```lx
var i = 0
while i < 10 {
  io.log(i)
  i = i + 1
}

each name in ["Alice", "Bob", "Carol"] {
  io.log("Hello, " + name + "!")
}
```

### Native Array and String Methods

Arrays and strings have built-in methods — no import needed:

```lx
val nums = [3, 1, 4, 1, 5]

nums.sort()                              
nums.map(fn(x) { x * 2 })              
nums.filter(fn(x) { x > 2 })           
nums.reduce(fn(acc, x) { acc + x }, 0) 
nums.includes(4)                         
nums.length                              

"lunex".toUpperCase()                    
"  hello  ".trim()                       
"lunex".startsWith("lun")               
```

### Concurrency

```lx
val ch = channel()

spawn fn() {
  ch.send(computeSomething())
}()

val result = ch.recv()
io.log(result)
```

### Defer

Schedules a block to run when the enclosing function exits:

```lx
fn process(path) {
  val fs = @import("std.fs")
  defer { io.log("finished:", path) }
  fs.readFile(path)
}
```

---

## CLI Reference

```text
lunex run <file> [--emit ast|ir]   run a .lx source or .nax archive
lunex start                        run the project entry from lunex.toml
lunex debug <file>                 run with full compile diagnostics and a stack trace on error
lunex -e "<code>"                  run a code snippet directly
lunex repl                         start the interactive REPL
lunex pack <file.lx|directory>    validate and bundle to .nax archive with optimized .nlo objects
lunex check <file>                 production semantic check without running user code
lunex see_errors <file>            show detailed compile errors
lunex init [name]                  create a new project folder
lunex init <template> <name>       create a project from a template
                                      (http_server, database, website)
lunex unpack <file.nax>            recover .lx code from optimized objects or extract stored source files
lunex recover <file.nlo>               recover canonical .lx code from an NLO object
lunex set cache <dir>              set the on-disk runtime cache directory
lunex set cache reset              reset the cache directory to default
lunex cache [clear]                show or clear the on-disk runtime cache
lunex memcache [clear]             show or clear the in-process memory cache
lunex platform                     show platform / adapter diagnostics
lunex runtimes                     show available execution engines
lunex bench <file>                 run with timing output
lunex env                          show global/local module store paths and project status
lunex link                         link this project's [project.bin] commands globally
lunex version                      print version
lunex help                         show this help
```

Extra options: `lunex --help-extras`

```text
--jit
--no-jit
```

> Package management is built into Lunex and implemented in Go. See below.

---

## Package Management

Lunex includes a Go-based package manager in the CLI, backed by `lunex.toml`
and `lunex.lock`. See [`modulesys.md`](modulesys.md) for the full picture.

```bash
lunex install
lunex install -g <url>[@version]
lunex install -l <url>[@version]
lunex add <url>[@version]
lunex remove <library>
lunex update [library]
lunex list
lunex env
lunex link
```

Installed libraries live in `~/.lunex/modules` (global, shared across every
project) or `./.lunex/modules` (local, scoped to a single project) — never
directly in a `cache/` folder. Each installed version gets its own
`<name>@<version>` directory, so two projects can depend on different
versions of the same library without conflict. Resolution when you write
`@import("pkg-name")` checks the local store first, then the global store.

---

## Module System

```lx
val io     = @import("std.io")
val http   = @import("std.http")
val crypto = @import("std.crypto")
```

Import a local source file or compiled archive:

```lx
val lib = @fimport("./src/utils.lx")
val pkg = @fimport("./dist/math.nax")
```

Import an external library installed by Lunex:

```lx
val xml = @import("lune-xml")
```

---

## Standard Library

| Module         | Purpose                                                  |
|----------------|----------------------------------------------------------|
| `std.io`       | Console output, input, colors, tables, spinner           |
| `std.fs`       | File system: read, write, list, stat                     |
| `std.http`     | HTTP client and server primitives                        |
| `std.http.router` | Request routing on top of `std.http`                  |
| `std.http.static` | Static file handler on top of `std.http`              |
| `std.crypto`   | Hashing, encoding, encryption, passwords, UUIDs          |
| `std.db`       | SQLite-backed document database (stored on disk)         |
| `std.ws`       | WebSocket server and client                              |
| `std.jwt`      | JSON Web Token sign and verify                           |
| `std.json`     | Parse, stringify, validate, read/write JSON files        |
| `std.math`     | Math functions and constants                             |
| `std.datetime` | Date, time, formatting, arithmetic                       |
| `std.os`       | Process, environment variables, shell execution          |
| `std.regex`    | Pattern matching and replacement (RE2 syntax)            |
| `std.env`      | Dotenv-compatible environment variables                  |
| `std.ffi`      | Native shared-library loading, symbol binding, calls, callbacks, and memory access |
| `std.testing`  | Lunex-native test registration, assertions, lifecycle hooks, diagnostics, snapshots, and execution |
| `std.utils`    | Array, object, string, and functional helpers            |

FFI targets Linux, Windows, macOS, and Android arm64. Each target uses its native dynamic loader and ABI, with Android using Bionic and `CGO_ENABLED=0`.

---

## Examples

Check out [`examples/`](examples/) for runnable code covering everything from the basics to the more advanced stuff:

- Hello World and basic I/O
- Variables, destructuring, template strings
- Structs and factory functions
- Control flow: `if`, `while`, `each`, `guard`, `unless`, `watch`, `match`, `defer`
- Standard library: math, crypto, fs, datetime, regex, os, http
- Higher-order functions: map, filter, reduce, compose, memoize
- Concurrent workers with `spawn` and `channel`
- WebSockets, HTTP servers, and REST APIs

---

## License

[Mozilla Public License Version 2.0](LICENSE)

© 2026 David Dev · [github.com/Megamexlevi2](https://github.com/Megamexlevi2)
