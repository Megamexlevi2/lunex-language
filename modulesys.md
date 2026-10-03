# Lunex Module System

Lunex resolves four kinds of imports: standard library modules, local file
imports, and libraries declared in `lunex.toml` and installed either
globally or per-project.

---

## Standard Library Modules

Always available, no installation and no `lunex.toml` entry required:

```lx
val io     = @import("std.io")
val http   = @import("std.http")
val router = @import("std.http.router")
val files  = @import("std.http.static")
val fs     = @import("std.fs")
val crypto = @import("std.crypto")
val math   = @import("std.math")
val db     = @import("std.db")
val os     = @import("std.os")
val regex  = @import("std.regex")
val utils  = @import("std.utils")
val dt     = @import("std.datetime")
val env    = @import("std.env")
val ws     = @import("std.ws")
val jwt    = @import("std.jwt")
val json   = @import("std.json")
val ffi    = @import("std.ffi")
val buffer = @import("std.buffer")
val ints   = @import("std.ints")
val runtime = @import("runtime")
```

The complete list of standard library modules is documented in
`docs/stdlib.md`.

---

## Local File Imports (`.lx` and `.nax`)

```lx
val utils  = @fimport("./src/utils.lx")
val mylib  = @fimport("./mylib.nax")
val shared = @fimport("../shared/utils.nax")
```

A `.nax` file is a compiled Lunex archive produced by `lunex pack`. In the default mode it bundles the selected module and every local `@fimport` dependency as `.nlo` compiled objects inside a single binary archive:

```bash
lunex pack math.lx -o math.nax
```

```lx
val io   = @import("std.io")
val math = @fimport("./math.nax")

fn main() {
  io.log(math.divide(50, 2))
  io.log(math.add(10, 5))
}
```

---

## Project Layout

```
my-project/
├── lunex.toml       # you edit this
├── lunex.lock       # Lunex writes this — exact resolved versions
├── main.lx
└── .lunex/
    └── modules/     # dependencies installed locally to this project
```

`lunex.toml` declares project metadata and dependencies. `lunex.lock` is
generated automatically by `lunex install`/`lunex add` — it pins the exact
version, source, and content hash of every installed library so a build
stays reproducible across machines.

### `lunex.toml`

```toml
[project]
name = "my-app"
version = "1.0.0"
description = "My Lunex application"
license = "MIT"
repository = "https://github.com/user/my-app"
entry = "main.lx"

[lunex]
min_version = "0.9.3"
max_version = "1.x"

[libraries.http_client]
url = "https://github.com/user/http-client"
version = "1.4.0"

[libraries.logger]
url = "https://github.com/user/logger"
version = "latest"

[libraries.database]
source = "github-release"
url = "https://github.com/user/database"
release = "v2.1.0"

[libraries.ui]
source = "github"
url = "https://github.com/user/framework"
path = "modules/ui"
version = "0.5.0"

[libraries.auth]
url = "https://github.com/user/auth"
version = ">=1.0.0 <2.0.0"

[libraries.test]
source = "local"
path = "./modules/test"
```

Standard library modules (`std.io`, `std.http`, `std.fs`, `std.crypto`,
`std.db`, `std.os`, `std.regex`, `std.utils`, `std.datetime`, `std.env`, `std.ffi`,
`std.testing`, `std.ws`, `std.jwt`, `std.math`, `std.json`, `std.buffer`, `std.ints`, and
`runtime`) never need a `[libraries.*]` entry.

### `lunex.lock`

```toml
[modules.logger]
version = "1.3.2"
hash = "sha256:..."
source = "github:user/logger"
url = "https://github.com/user/logger"

```

Don't edit `lunex.lock` by hand — it's regenerated on every install.

---

## Global vs. Local Installs

Every installed version is kept isolated on disk under its own
`<name>@<version>` directory, so two projects — or two dependencies of the
same project — can each depend on a different version of the same library
without conflict.

| Store  | Location            | Scope                                |
|--------|----------------------|---------------------------------------|
| Global | `~/.lunex/modules`   | shared across every project on the machine |
| Local  | `./.lunex/modules`   | this project only                     |

Resolution checks the local store first, then the global store.

```bash
lunex install -g https://github.com/user/logger
lunex install -g https://github.com/user/logger@1.3.2
lunex install -l https://github.com/user/logger
```

`-g`/`-l` installs work without a `lunex.toml` at all — useful for a quick
one-off script. `lunex install -l` also records the library in
`lunex.toml` if one exists in the current directory.

### Installing everything a project declares

```bash
lunex install
```

Reads every `[libraries.*]` entry in `lunex.toml`, installs each one into
the local store, and writes `lunex.lock`.

### Adding a new dependency

```bash
lunex add https://github.com/user/repo
lunex add https://github.com/user/repo@v1.2.3
```

Adds a `[libraries.*]` entry to `lunex.toml` and installs it locally in
the same step.

### Managing installed libraries

```bash
lunex list
lunex remove logger
lunex update
lunex update logger
```

---

## Importing Libraries

```lx
val xml = @import("lune-xml")
xml.parse("<root/>")
```

`@import("name")` is resolved in this order:

1. Standard library (always wins, can't be shadowed)
2. Local store (`./.lunex/modules`), matching the version pinned in
   `lunex.lock` if one is present
3. Global store (`~/.lunex/modules`)

If nothing resolves, Lunex prints:

```
hint: library "pkg-name" not found — add it to lunex.toml with:
  lunex add https://github.com/<owner>/pkg-name
  lunex install
```

---

## `.nax` File Format

A `.nax` file is a custom binary container, not a zip or tar archive. Only the Lunex runtime can read it. In optimized mode it stores compiled `.nlo` objects and module metadata without storing the original source text.

The `.nlo` format is separate from the legacy `x102c` object format. It stores a validated compiled AST in binary form and does not contain the legacy NTZ bytecode section. Optimized NLO objects omit plaintext source and are lazily decoded by the NAX runtime. `lunex unpack` can reconstruct canonical `.lx` source from the stored AST.

```bash
lunex run mylib.nax
```

---

## Running and Debugging a Project

```bash
lunex start
lunex debug main.lx
```

`lunex debug` compiles with complete diagnostics (not just the first
error) and, if compilation succeeds, runs the file with debug mode
enabled so every execution step and any runtime error is printed with a
full trace — useful when `lunex run` gives you too little detail to find
a bug.

---

## Executable Commands (`bin`)

`lunex.toml` can declare `bin`, the same idea as `"bin"` in `package.json`:

```toml
[project]
bin = "./cli.lx"
```

for a single command named after the project, or a table for multiple
named commands:

```toml
[project.bin]
build = "./bin/build.lx"
serve = "./bin/serve.lx"
```

When a library that declares `bin` is installed — via `lunex install`,
`lunex install -g/-l`, or as a `lunex.toml` dependency — Lunex writes an
executable shim per command into the bin directory of whichever store it
was installed into:

| Store  | Bin directory     |
|--------|--------------------|
| Global | `~/.lunex/bin`     |
| Local  | `./.lunex/bin`     |

Each shim is a small shell script that runs `lunex run <entry>`. Add
`~/.lunex/bin` to your `PATH` to run globally installed commands directly:

```bash
export PATH="$HOME/.lunex/bin:$PATH"
```

### Developing a command locally

```bash
lunex link
```

Reads `lunex.toml` in the current directory and links its `[project.bin]`
commands into `~/.lunex/bin` immediately, pointing at your working
directory — the same idea as `npm link`. No install step, and edits to
your source take effect the next time the command runs.

Removing a library (`lunex remove <name>`) also removes any command
shims it registered.

---

## Example: End-to-End Workflow

```bash
lunex init my-app
cd my-app
lunex add https://github.com/Megamexlevi2/lunex-language/lune-xml
lunex install
```

```lx
val io  = @import("std.io")
val xml = @import("lune-xml")

fn main() {
  val doc = xml.parse("<greet>Hello, Lunex!</greet>")
  io.log(doc.root.text)
}
```

```bash
lunex start
```

## Pack mode

The default `lunex pack` mode stores a validated, resolver-ready binary AST in an NLO object inside the NAX archive. NAX execution skips source reading, lexing, parsing, and resolution, uses lazy standard-library initialization, and avoids repeated top-level validation for trusted optimized objects. This reduces startup work compared with executing the same `.lx` file directly, without CGo and without adding a new execution backend. The runtime still uses the existing Go interpreter.

The `--source` option preserves the original `.lx` files in the NAX archive for source-first recovery. Optimized NLO objects can also recover canonical Lunex source from their stored AST without embedding the original source text.

`lunex pack <file.lx|directory>` uses the `.nlo` compiled-object representation by default. The default archive does not store source text and does not use the legacy `x102c` object format. Use `--source` to preserve source files. `lunex unpack` extracts the entries stored in either representation.
