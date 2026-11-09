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

Requires Go 1.25.5 or later.

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

#### Using PowerShell (Windows)

```bash
git clone https://github.com/Megamexlevi2/lunex-language
cd lunex-language
.\build.ps1
```

Try Lunex Online

You can try the Lunex programming language directly in your browser without installing or building anything.

<p align="center">
  <a href="https://lunex-language.github.io/lunex/playground.html">
    <img src="https://img.shields.io/badge/Open%20Lunex-Playground-6C5CE7?style=for-the-badge" alt="Open Lunex Playground">
  </a>
</p>

---

## Quick Start

```bash
cat << 'EOF' > hello.lx
val io = @import("std.io")

fn main() {
  io.log("Hello, World!")
}
EOF

./lunex run hello.lx
```

---

## Language at a Glance

### Variables

`val` is immutable. `var` is mutable.

```lx
val io = @import("std.io")

fn main() {
  val name = "Lunex"
  val pi = 3.14159
  val active = true
  var counter = 0
  counter = counter + 1

  io.log("name:", name)
  io.log("pi:", pi)
  io.log("active:", active)
  io.log("counter:", counter)
}
```

Destructuring works on objects and arrays at the top level and inside functions or blocks. Object fields can be renamed, defaults can be supplied, and `...rest` collects remaining fields or array elements:

```lx
val io = @import("std.io")

fn main() {
  val user = struct {
    name = "Alice"
    role = "admin"
    team = "runtime"
  }
  val items = [10, 20, 30, 40]
  val { name, role: access, missing = "guest", ...extra } = user
  val [first, second, ...remaining] = items

  io.log("name:", name)
  io.log("access:", access)
  io.log("missing:", missing)
  io.log("extra:", extra)
  io.log("first:", first, "second:", second)
  io.log("remaining:", remaining)
}
```

### Functions

The last expression in a function body is its return value.
Lunex has no `return` keyword.

```lx
val io = @import("std.io")

fn add(a, b) {
  a + b
}

val square = fn(x) { x * x }

fn main() {
  io.log(add(2, 3))
  io.log(square(5))
}
```

### Structs and Factory Functions

Struct values and factory functions are useful for creating lightweight
objects:

```lx
val io = @import("std.io")

fn Animal(name, sound) {
  val self = struct {
    name = name
    sound = sound

    fn speak() {
      self.name + " says " + self.sound
      /! or: "!{self.name} says !{self.sound}"
    }
  }
  self
}

fn main() {
  val cat = Animal("Cat", "Meow")
  io.log(cat.speak())
}
```

### Control Flow

```lx
val io = @import("std.io")

fn classify(n) {
  if n < 0 {
    "negative"
  } else if n == 0 {
    "zero"
  } else {
    "positive"
  }
}

fn main() {
  io.log(classify(-1))
  io.log(classify(0))
  io.log(classify(1))
}
```

`guard` runs its `else` block when the condition is **false**:

```lx
val io = @import("std.io")

fn greet(user) {
  guard user != null else {
    io.err("no user provided")
  }
  io.log("Hello,", user)
}

fn main() {
  greet("Alice")
}
```

`unless` runs its block when the condition is **false**:

```lx
val io = @import("std.io")

fn main() {
  val connected = false
  unless connected {
    io.warn("not connected — retrying")
  }
}
```

`watch` runs a block whenever a watched variable or member expression changes:

```lx
val io = @import("std.io")

fn main() {
  var counter = 0

  watch counter {
    io.log("Counter changed: " + str(counter))
    /! or: io.log("Counter changed: !{counter}")
  }

  counter = 10
  counter = 20
}
```

`match` tests exact values — top-to-bottom, first match wins:

```lx
val io = @import("std.io")

fn main() {
  val status = "ok"
  val label = match status {
    "ok" => "success"
    "pending" => "waiting"
    "fail" => "error"
    _ => "unknown"
  }
  io.log(label)
}
```

### Loops

```lx
val io = @import("std.io")

fn main() {
  var i = 0
  while i < 3 {
    io.log("i:", i)
    i = i + 1
  }

  each name in ["Alice", "Bob", "Carol"] {
    io.log("Hello,", name, "!")
  }
}
```

### Native Array and String Methods

Arrays and strings have built-in methods — no import needed:

```lx
val io = @import("std.io")

fn main() {
  val nums = [3, 1, 4, 1, 5]

  io.log("sorted:", nums.sort())
  io.log("map:", nums.map(fn(x) { x * 2 }))
  io.log("filter:", nums.filter(fn(x) { x > 2 }))
  io.log("sum:", nums.reduce(fn(acc, x) { acc + x }, 0))
  io.log("includes 4:", nums.includes(4))
  io.log("length:", nums.length)

  io.log("uppercase:", "lunex".toUpperCase())
  io.log("trim:", "  hello  ".trim())
  io.log("starts with lun:", "lunex".startsWith("lun"))
}
```

### Concurrency

```lx
val io = @import("std.io")

fn calculate(n) {
  n * n
}

fn main() {
  val task = new.task(calculate, 12)
  io.log(task.await())

  val batch = [new.task(calculate, 2), new.task(calculate, 3)]
  io.log(tasks.all(batch))

  val results = parallel.map(calculate, [1, 2, 3, 4, 5])
  io.log(results)
}
```

### Defer

Schedules a block to run when the enclosing function exits:

```lx
val io = @import("std.io")
val fs = @import("std.fs")

fn process(path) {
  defer { io.log("finished:", path) }
  fs.readFile(path)
}

fn main() {
  process("./examples/01_hello_world.lx")
}
```

---

## CLI Reference

```text
lunex run <file> [--emit ast|ir]   run a .lx source or .nax archive
lunex fmt <file.lx> [options]      format and verify Lunex source
lunex format <file.lx> [options]   alias for fmt
lunex start                        run the project entry from lunex.toml
lunex debug <file>                 run with call stacks, runtime profiles, and diagnostics
lunex -e "<code>"                  run a code snippet directly
lunex repl                         start the interactive REPL
lunex pack <file.lx|directory>     compile and bundle a file or directory into .nax (source preserved by default)
lunex check <file>                 static checks and diagnostics without running user code
lunex see_errors <file>            show detailed compile errors
lunex init [name]                  create a new project folder
lunex init <template> <name>       create a project from a template
                                      (http_server, database, website)
lunex unpack <file.nax>             recover .lx files from compiled entries or stored source
lunex set cache <dir>              set the on-disk runtime cache directory
lunex set cache reset              reset the cache directory to default
lunex cache [clear]                show or clear the on-disk runtime cache
lunex memcache [clear]             show or clear the in-process memory cache
lunex platform                     show platform / adapter diagnostics
lunex runtimes                     show the AST interpreter and native JIT capabilities
lunex bench <file>                 run with timing output
lunex env                          show global/local module store paths and project status
lunex link                         link this project's [project.bin] commands globally
lunex version                      print version
lunex help                         show this help
```

Format source files with the syntax-aware formatter and integrity verifier:

```bash
lunex fmt main.lx
lunex fmt main.lx --check
lunex fmt main.lx --stdout
lunex format main.lx --write
```

The default behavior writes verified formatting back to the file. `--check`
checks formatting without modifying the source and exits with status 1 when
changes are needed. `--stdout` prints the candidate without changing the file.
The formatter uses the lexer, parser, and AST to lay out statements, blocks,
operators, calls, arrays, and objects. It preserves token spellings, string
literals, comments, and line endings, and validates the formatted source against
the original AST before writing. When validation fails, Lunex refuses to write
an unverified candidate. See the [CLI reference](docs/cli.md) for details.

Extra options: `lunex --help-extras`

```text
--jit
--no-jit
```

> Package management is built into Lunex and implemented in Go. See below.

---

## Package Management

Lunex includes a Go-based package manager in the CLI, backed by `lunex.toml`
and `lunex.lock`. Use `lunex add <url>[@version]` to declare and install a dependency,
then `lunex install` to install the dependencies declared by the current project.
See [`modulesys.md`](modulesys.md) for the full picture.

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
project) or `./.lunex/modules` (local, scoped to a single project). Each installed version gets its own
`<name>@<version>` directory, so two projects can depend on different
versions of the same library without conflict. Resolution when you write
`@import("pkg-name")` checks the local store first, then the global store.

---

## Module System

```lx
val io = @import("std.io")
val http = @import("std.http")
val crypto = @import("std.crypto")

fn main() {
  io.log("Standard library modules loaded")
}
```

Import a local source file or compiled archive:

```lx
val math = @fimport("./tests/math.lx")
val io = @import("std.io")

fn main() {
  io.log("Sum:", math.add(10, 5))
  io.log("Product:", math.mul(4, 3))
}
```

Import an external library installed by Lunex:

```lx
val xml = @import("lune-xml")
val io = @import("std.io")

fn main() {
  io.log("XML library loaded")
}
```

---

## Standard Library

| Module         | Purpose                                                  |
|----------------|----------------------------------------------------------|
| `std.io`       | Console output, input, colors, tables, spinner           |
| `std.fs`       | File system: read, write, list, stat                     |
| `std.buffer`   | Byte buffer creation, conversion, slicing, and operations |
| `std.ints`     | Fixed-width integer and bitwise operations                |
| `std.image`    | Image decoding, encoding, pixel access, transformations, and compositing |
| `std.http`     | HTTP client and server primitives                        |
| `std.http.router` | Request routing on top of `std.http`                  |
| `std.http.static` | Static file handler on top of `std.http`              |
| `std.crypto`   | Hashing, encryption, passwords, UUIDs, secure randomness |
| `std.encoding` | UTF-8/16/32, Latin-1, Base64/Base32, URL percent encoding |
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

FFI targets Linux, Windows, macOS, and Android arm64. Each target uses its native dynamic loader and ABI, with Android using Bionic and `CGO_ENABLED=0`. FFI is disabled by default; enable it for a command with `lunex ffi = on run <file.lx>`.

---

## Examples

Check out [`examples/`](examples/) for runnable code covering everything from the basics to the more advanced stuff:

- Hello World and basic I/O
- Variables, destructuring, template strings
- Structs and factory functions
- Control flow: `if`, `while`, `each`, `guard`, `unless`, `watch`, `match`
- Standard library: math, crypto, fs, datetime, os, HTTP, byte buffers, fixed-width integers
- Higher-order functions: map, filter, reduce, compose, memoize
- Concurrent tasks with `new.task` and `parallel.map`
- HTTP servers and REST APIs

---

## License

[Mozilla Public License Version 2.0](LICENSE)

© 2026 David Dev · [github.com/Megamexlevi2](https://github.com/Megamexlevi2)
