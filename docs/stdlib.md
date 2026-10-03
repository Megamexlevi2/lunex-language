# Standard Library Reference

Complete API reference for all built-in modules in Lunex v0.9.3.

All modules are embedded in the Lunex binary — no installation required.
Import any module with `@import("std.<name>")`.

---

## `std.io` — Console I/O

```lx
val io = @import("std.io")
```

### Output

| Function                   | Description                                        |
|----------------------------|----------------------------------------------------|
| `io.log(...args)`          | Print to stdout, space-separated                   |
| `io.err(...args)`          | Print to stderr, colored red                       |
| `io.warn(...args)`         | Print to stderr, colored yellow                    |
| `io.info(...args)`         | Print to stdout, colored cyan                      |
| `io.success(...args)`      | Print to stdout with a green `✔` prefix            |
| `io.write(s)`              | Write a string to stdout with no trailing newline  |
| `io.newline(n?)`           | Print `n` blank lines (default 1)                  |
| `io.table(rows)`           | Render an array of structs/objects as a table      |
| `io.json(val)`             | Pretty-print any value as formatted JSON            |
| `io.hr(len?, char?)`       | Print a horizontal rule                            |
| `io.banner(text)`          | Print a highlighted banner                         |
| `io.clear()`               | Clear the terminal screen                          |

None of `io.err`/`io.warn`/`io.info` add a text prefix like `[ERROR]` —
they colorize the message only. Colors are skipped automatically when
stdout isn't an interactive terminal.

### Spinner

`io.spinner()` starts an animated spinner and returns a control object.
Each call to `tick(message?)` advances the animation by one frame and
redraws the line with the given message; `stop()` finishes the line with
a newline:

```lx
val sp = io.spinner()
sp.tick("Loading data...")
sp.tick("Loading data...")
sp.stop()
```

### Progress bar

```lx
io.progress(current, total, label?)
```

Renders a progress bar for `current` out of `total` steps.

### Input

| Function                  | Returns  | Description                          |
|---------------------------|----------|--------------------------------------|
| `io.read(prompt?)`        | `string` | Read a line from stdin               |
| `io.readLine(prompt?)`    | `string` | Alias for `io.read`                  |
| `io.readInt(prompt?)`     | `number` | Read a line and parse it as integer  |

### Formatting

```lx
io.format("Hello, {}! You are {} years old.", name, age)
```

### Colors

These functions return a colorized string (no side effects — you can nest them):

```lx
io.red(s)      io.green(s)    io.yellow(s)
io.blue(s)     io.magenta(s)  io.cyan(s)
io.white(s)    io.gray(s)     io.bold(s)
io.dim(s)      io.italic(s)
io.color("red", s)   
io.strip(s)          
```

```lx
io.log(io.green("✓"), "Build passed")
io.log(io.bold("=== Report ==="))
io.log(io.red("error:"), "something went wrong")
```

### Terminal detection

```lx
io.isTerminal()   
```

---

## `std.math` — Mathematics

```lx
val math = @import("std.math")
```

### Constants

| Name             | Value                |
|-------------------|-----------------------|
| `math.PI`         | 3.141592653589793     |
| `math.E`          | 2.718281828459045     |
| `math.LN2`        | ln(2)                  |
| `math.LN10`       | ln(10)                 |
| `math.LOG2E`      | log₂(e)                |
| `math.LOG10E`     | log₁₀(e)               |
| `math.SQRT2`      | √2                     |
| `math.SQRT1_2`    | √(1/2)                 |
| `math.PHI`        | golden ratio           |
| `math.Infinity`   | positive infinity      |
| `math.NaN`        | not-a-number           |

### Basic functions

| Function                  | Description                            |
|-----------------------------|------------------------------------------|
| `math.abs(x)`               | Absolute value                          |
| `math.ceil(x)`              | Round up to nearest integer             |
| `math.floor(x)`             | Round down to nearest integer           |
| `math.round(x)`             | Round to nearest integer                |
| `math.trunc(x)`             | Truncate fractional part                |
| `math.sign(x)`              | −1, 0, or 1                             |
| `math.sqrt(x)`              | Square root                             |
| `math.cbrt(x)`              | Cube root                               |
| `math.pow(x, y)`            | x to the power y                        |
| `math.hypot(a, b)`          | sqrt(a² + b²)                           |
| `math.min(a, b, ...)`       | Smallest of all arguments               |
| `math.max(a, b, ...)`       | Largest of all arguments                |
| `math.clamp(v, lo, hi)`     | Clamp v to [lo, hi]                     |
| `math.lerp(a, b, t)`        | Linear interpolation between a and b    |

### Exponentials and logarithms

| Function          | Description                                  |
|---------------------|-------------------------------------------------|
| `math.exp(x)`       | e^x                                              |
| `math.exp2(x)`      | 2^x                                              |
| `math.expm1(x)`     | e^x − 1, accurate for small x                    |
| `math.log(x)`       | Natural logarithm                                |
| `math.log2(x)`      | Base-2 logarithm                                 |
| `math.log10(x)`     | Base-10 logarithm                                |
| `math.log1p(x)`     | Natural logarithm of 1 + x, accurate for small x |

### Trigonometry

| Function            | Description                    |
|-----------------------|-----------------------------------|
| `math.sin(x)`         | Sine (radians)                    |
| `math.cos(x)`         | Cosine (radians)                  |
| `math.tan(x)`         | Tangent (radians)                 |
| `math.asin(x)`        | Arcsine                           |
| `math.acos(x)`        | Arccosine                         |
| `math.atan(x)`        | Arctangent                        |
| `math.atan2(y, x)`    | Two-argument arctangent           |
| `math.sinh(x)`        | Hyperbolic sine                   |
| `math.cosh(x)`        | Hyperbolic cosine                 |
| `math.tanh(x)`        | Hyperbolic tangent                |
| `math.asinh(x)`       | Inverse hyperbolic sine           |
| `math.acosh(x)`       | Inverse hyperbolic cosine         |
| `math.atanh(x)`       | Inverse hyperbolic tangent        |
| `math.degToRad(d)`    | Degrees to radians                |
| `math.radToDeg(r)`    | Radians to degrees                |

### Random

| Function                   | Description                          |
|-------------------------------|------------------------------------------|
| `math.random()`               | Uniform float in [0, 1)                  |
| `math.random(max)`            | Uniform float in [0, max)                |
| `math.random(min, max)`       | Uniform float in [min, max)              |
| `math.randomInt()`            | Random non-negative integer               |
| `math.randomInt(max)`         | Random integer in [0, max)                |
| `math.randomInt(min, max)`    | Random integer in **[min, max)** — max is exclusive, not inclusive |
| `math.seed(n)`                | Seed the random number generator         |

### Number theory

| Function                   | Description                           |
|-------------------------------|-------------------------------------------|
| `math.gcd(a, b)`              | Greatest common divisor                   |
| `math.lcm(a, b)`              | Least common multiple                     |
| `math.isPrime(n)`             | True if n is prime                        |
| `math.factorial(n)`           | n!                                        |
| `math.combinations(n, k)`     | Binomial coefficient C(n, k)              |
| `math.permutations(n, k)`     | P(n, k)                                   |
| `math.fib(n)`                 | The nth Fibonacci number                  |
| `math.primes(n)`              | Array of all primes up to and including n |

### Statistics

Each of these accepts either a single array argument or multiple numeric
arguments (`math.mean([1, 2, 3])` and `math.mean(1, 2, 3)` are equivalent).
`math.variance` and `math.stdDev` require an array argument.

| Function              | Description                              |
|--------------------------|-----------------------------------------------|
| `math.sum(...)`          | Sum of the values                              |
| `math.product(...)`      | Product of the values                          |
| `math.mean(...)`         | Arithmetic mean                                |
| `math.variance(arr)`     | Population variance                            |
| `math.stdDev(arr)`       | Population standard deviation                  |

### Base conversion

| Function                    | Description                                       |
|--------------------------------|--------------------------------------------------------|
| `math.toBinary(n)`             | Binary string representation                            |
| `math.toHex(n, prefix?)`       | Hex string representation; pass `true` to include `0x`  |
| `math.toOctal(n)`              | Octal string representation                             |

### Type checks

| Function              | Description                        |
|--------------------------|---------------------------------------|
| `math.isNaN(v)`          | True if the value is NaN              |
| `math.isFinite(v)`       | True if the value is finite           |
| `math.isInfinite(v)`     | True if the value is +/- infinity     |

---

## `std.json` — JSON serialization

```lx
val json = @import("std.json")
```

### Parsing and formatting

| Function                         | Description                                     |
|----------------------------------|-------------------------------------------------|
| `json.parse(text)`               | Parse a JSON string into Lunex values           |
| `json.stringify(value)`          | Pretty-print JSON with 2-space indentation      |
| `json.stringify(value, indent)`  | Pretty-print JSON using `indent` spaces         |
| `json.pretty(value)`             | Alias for `json.stringify(value)`               |
| `json.compact(value)`            | Minified JSON output (no whitespace)            |
| `json.isValid(text)`             | True if the text is valid JSON                  |
| `json.toJSON(value)`             | Alias for `json.stringify(value)`               |
| `json.fromJSON(text)`            | Alias for `json.parse(text)`                    |
| `json.writeFile(path, value)`    | Save pretty JSON to a file                      |
| `json.writeFile(path, value, indent)` | Save JSON with custom indentation          |
| `json.save(path, value)`         | Alias for `json.writeFile(path, value)`         |

> `json.stringify` skips function fields and undefined values in objects, and
> writes array holes as `null`, keeping output readable and consistent.

---

## `std.utils` — Utilities

```lx
val utils = @import("std.utils")
```

> Common array and string operations (`map`, `filter`, `reduce`, `sort`,
> `push`, `includes`, etc.) are available as **native methods** directly on
> the value — no import needed. See the language reference for the full list.
> `std.utils` provides higher-level helpers that go beyond native methods.

### Array helpers

| Function                         | Description                                    |
|----------------------------------|------------------------------------------------|
| `utils.range(n)`                 | Array `[0, 1, …, n−1]`                        |
| `utils.range(start, end)`        | Array `[start, …, end−1]`                     |
| `utils.chunk(arr, n)`            | Split into chunks of size n                    |
| `utils.flatten(arr)`             | Flatten one level of nesting                   |
| `utils.flatMap(arr, fn)`         | Map then flatten one level                     |
| `utils.zip(a, b)`                | Pair elements: `[[a0,b0], [a1,b1], …]`        |
| `utils.unzip(pairs)`             | Inverse of zip: returns `[keys, values]`       |
| `utils.intersection(a, b)`       | Elements present in both arrays                |
| `utils.difference(a, b)`         | Elements in a not in b                         |
| `utils.union(a, b)`              | All unique elements from both arrays           |
| `utils.uniq(arr)`                | Remove duplicate values                        |
| `utils.uniqBy(arr, fn)`          | Remove duplicates by key function              |
| `utils.shuffle(arr)`             | Return a randomly shuffled copy                |
| `utils.sample(arr)`              | Pick one random element                        |
| `utils.sampleSize(arr, n)`       | Pick n random elements                         |
| `utils.sortBy(arr, fn)`          | Sort by a key function                         |
| `utils.sortBy(arr, fn, "desc")`  | Sort descending by a key function              |
| `utils.groupBy(arr, fn)`         | Group elements into an object by key           |
| `utils.countBy(arr, fn)`         | Count elements per group                       |
| `utils.partition(arr, fn)`       | Split into `[pass, fail]` by predicate         |

### Numeric helpers

| Function                  | Description                                |
|---------------------------|--------------------------------------------|
| `utils.sum(arr)`          | Sum of a numeric array                     |
| `utils.mean(arr)`         | Arithmetic mean                            |
| `utils.median(arr)`       | Median value                               |
| `utils.min(arr)`          | Minimum value                              |
| `utils.max(arr)`          | Maximum value                              |
| `utils.clamp(v, lo, hi)`  | Clamp v to [lo, hi]                        |
| `utils.lerp(a, b, t)`     | Linear interpolation                       |
| `utils.random(min?, max?)`| Random float in [min, max)                 |
| `utils.randInt(min?, max?)`| Random integer in **[min, max)** — max is exclusive, not inclusive |
| `utils.formatNumber(n)`   | Format with thousands separator            |
| `utils.formatBytes(n)`    | Human-readable byte size (KB, MB, …)       |

### Object helpers

| Function                      | Description                                    |
|-------------------------------|------------------------------------------------|
| `utils.keys(obj)`             | Array of own keys                              |
| `utils.values(obj)`           | Array of own values                            |
| `utils.entries(obj)`          | Array of `[key, value]` pairs                  |
| `utils.fromEntries(pairs)`    | Build an object from `[key, value]` pairs      |
| `utils.has(obj, key)`         | True if key exists on the object               |
| `utils.pick(obj, keys)`       | New object with only the specified keys        |
| `utils.omit(obj, keys)`       | New object without the specified keys          |
| `utils.merge(a, b)`           | Merge b into a (shallow, returns new object)   |
| `utils.assign(target, source)`| Copy source properties into target             |
| `utils.invert(obj)`           | Swap keys and values                           |
| `utils.mapValues(obj, fn)`    | Transform each value with fn                   |

### String helpers

| Function                  | Description                                     |
|---------------------------|-------------------------------------------------|
| `utils.camelCase(s)`      | `"hello world"` → `"helloWorld"`               |
| `utils.snakeCase(s)`      | `"helloWorld"` → `"hello_world"`               |
| `utils.kebabCase(s)`      | `"helloWorld"` → `"hello-world"`               |
| `utils.titleCase(s)`      | `"hello world"` → `"Hello World"`              |
| `utils.slugify(s)`        | `"Hello, World!"` → `"hello-world"`            |
| `utils.truncate(s, n)`    | Truncate to n chars with `…` suffix             |
| `utils.pad(s, n, char?)`  | Pad to width n (centered)                       |
| `utils.padStart(s, n, char?)` | Pad to width n on the left                  |
| `utils.padEnd(s, n, char?)`   | Pad to width n on the right                 |
| `utils.repeat(s, n)`      | Repeat string n times                           |
| `utils.template(s, obj)`  | Fill `{{key}}` placeholders from obj            |

`utils.template` also accepts `${key}` placeholders — both styles work in the
same call, e.g. `utils.template("Hi {{name}}, ${name}!", { name: "Ada" })`.

### Functional helpers

| Function              | Description                                             |
|-----------------------|---------------------------------------------------------|
| `utils.pipe(fns)`     | Return a function that passes input through each fn     |
| `utils.compose(fns)`  | Like pipe but in reverse order                          |
| `utils.memoize(fn)`   | Return a version of fn with cached results              |
| `utils.once(fn)`      | Return a version of fn that only runs once              |
| `utils.negate(fn)`    | Return a function that inverts the boolean result       |
| `utils.times(n, fn)`  | Call fn n times with index, return results array        |

### Identity and time

| Function              | Description                                     |
|-----------------------|-------------------------------------------------|
| `utils.uuid()`        | Generate a RFC-4122 UUID v4                     |
| `utils.now()`         | Current Unix timestamp in milliseconds          |
| `utils.timestamp()`   | Alias for `utils.now()`                         |
| `utils.sleep(ms)`     | Pause execution for ms milliseconds             |
| `utils.noop()`        | A function that does nothing                    |
| `utils.identity(x)`   | A function that returns its argument            |

### Type helpers

| Function                | Description                                          |
|---------------------------|----------------------------------------------------------|
| `utils.type(v)`           | Runtime type name of a value, e.g. `"string"`, `"array"`  |
| `utils.isEmpty(v)`        | True for `""`, `[]`, `{}`, or a nullish value              |
| `utils.isNil(v)`          | True if the value is `null` or `undefined`                |
| `utils.isEmail(s)`        | True if the string looks like an email address             |
| `utils.isUrl(s)`          | True if the string looks like a URL                        |
| `utils.isNumeric(s)`      | True if the string parses as a number                      |
| `utils.toNumber(v)`       | Convert a value to a number (`NaN` on failure)              |
| `utils.toString(v)`       | Convert a value to its string representation                |
| `utils.clone(v)`          | Deep clone an array or object                               |
| `utils.equal(a, b)`       | Deep equality check                                          |

---

## `std.datetime` — Date and time

```lx
val datetime = @import("std.datetime")
```

### Creating datetime values

| Function                           | Returns   | Description                          |
|------------------------------------|-----------|--------------------------------------|
| `datetime.now()`                   | datetime  | Current local date-time              |
| `datetime.utcNow()`                | datetime  | Current UTC date-time                |
| `datetime.fromTimestamp(ms)`       | datetime  | Parse a Unix timestamp (ms)          |
| `datetime.fromTimestamp(ms, "s")`  | datetime  | Parse a Unix timestamp in seconds    |
| `datetime.parse(s)`                | datetime  | Parse an ISO 8601 string             |
| `datetime.parse(s, format)`        | datetime  | Parse `s` using a custom layout (same tokens as `datetime.format`) |

A datetime value has these readable fields:

```lx
val now = datetime.now()
io.log(now.iso)        
io.log(now.year)       
io.log(now.month)      
io.log(now.day)        
io.log(now.unix)       
io.log(now.timestamp)  
```

### Formatting

```lx
io.log(datetime.format(now, "YYYY-MM-DD HH:mm:ss"))
```

**Layout tokens:**

| Token   | Meaning                          |
|---------|-----------------------------------|
| `YYYY`  | 4-digit year                      |
| `YY`    | 2-digit year                      |
| `MMMM`  | Full month name (`January`)        |
| `MMM`   | Short month name (`Jan`)           |
| `MM`    | 2-digit month                      |
| `M`     | Month, no leading zero              |
| `DD`    | 2-digit day                        |
| `D`     | Day, no leading zero                 |
| `dddd`  | Full weekday name (`Monday`)         |
| `ddd`   | Short weekday name (`Mon`)            |
| `HH`    | 2-digit hour, 24-hour                 |
| `hh`    | 2-digit hour, 12-hour                  |
| `h`     | Hour, 12-hour, no leading zero          |
| `mm`    | 2-digit minute                          |
| `ss`    | 2-digit second                           |
| `SSS`   | Milliseconds, zero-padded to 3            |
| `A`     | `AM`/`PM`                                  |
| `a`     | `am`/`pm`                                    |
| `Z`     | UTC offset, e.g. `+00:00`                     |
| `ZZ`    | UTC offset without colon, e.g. `+0000`          |

### Converting

| Function                          | Returns | Description                          |
|-----------------------------------|---------|--------------------------------------|
| `datetime.toTimestamp(dt)`        | number  | Milliseconds since Unix epoch        |
| `datetime.toTimestamp(dt, "s")`   | number  | Seconds since Unix epoch             |
| `datetime.format(dt, layout)`     | string  | Format using layout tokens           |

### Arithmetic

| Function                      | Returns  | Description                       |
|-------------------------------|----------|-----------------------------------|
| `datetime.add(dt, n, unit?)`  | datetime | Add n units (default: `"ms"`)     |
| `datetime.subtract(dt, n, unit?)` | datetime | Subtract n units              |
| `datetime.diff(a, b, unit?)`  | number   | Difference from a to b (default: `"ms"`) |

**Unit values:** `"ms"` `"s"` `"m"` `"h"` `"d"` `"w"` `"month"` `"year"`

### Comparison

| Function                    | Returns | Description                          |
|-----------------------------|---------|--------------------------------------|
| `datetime.isBefore(a, b)`   | boolean | True if a is before b                |
| `datetime.isAfter(a, b)`    | boolean | True if a is after b                 |
| `datetime.isEqual(a, b)`    | boolean | True if a and b are the same instant |
| `datetime.compare(a, b)`    | number  | −1, 0, or 1                          |

### Inspection

| Function                  | Returns | Description                                  |
|---------------------------|---------|----------------------------------------------|
| `datetime.weekday(dt)`    | number  | Day of week (0=Sunday … 6=Saturday)          |
| `datetime.weekdayName(dt)`| string  | `"Monday"`, `"Tuesday"`, etc.                |
| `datetime.monthName(dt)`  | string  | `"January"`, `"February"`, etc.              |
| `datetime.dayOfYear(dt)`  | number  | Day number within the year (1–366)           |
| `datetime.weekOfYear(dt)` | number  | ISO week number (1–53)                       |
| `datetime.daysInMonth(dt)`| number  | Days in the month of dt                      |
| `datetime.isLeapYear(dt)` | boolean | True if the year is a leap year              |
| `datetime.isWeekend(dt)`  | boolean | True if Saturday or Sunday                   |
| `datetime.isValid(dt)`    | boolean | True if the value is a valid datetime        |

### Rounding

| Function                      | Returns  | Description                        |
|-------------------------------|----------|------------------------------------|
| `datetime.startOf(dt, unit)`  | datetime | Start of the given unit            |
| `datetime.endOf(dt, unit)`    | datetime | End of the given unit              |

### Constructing from parts

| Function                 | Returns  | Description                                              |
|----------------------------|----------|--------------------------------------------------------------|
| `datetime.fromParts(parts)`| datetime | Build from an object: `{ year, month, day, hour?, minute?, second?, ms? }` (all default to the epoch/midnight if omitted) |

```lx
val dt = datetime.fromParts({ year: 2026, month: 6, day: 25, hour: 14 })
```

> Note the field names are `minute` and `second` (not `min`/`sec`).

### Other

| Function                       | Description                                                  |
|-----------------------------------|-------------------------------------------------------------------|
| `datetime.timezone(dt, tzName)`   | Convert dt to another IANA timezone (e.g. `"America/Sao_Paulo"`)  |
| `datetime.sleep(ms, unit?)`       | Pause execution for `ms` in the given unit (default: `"ms"`)      |

---

## `std.crypto` — Cryptography

```lx
val crypto = @import("std.crypto")
```

### Hashing

| Function                    | Description                                              |
|-----------------------------|----------------------------------------------------------|
| `crypto.sha256(s)`          | SHA-256 hex digest                                       |
| `crypto.sha512(s)`          | SHA-512 hex digest                                       |
| `crypto.sha1(s)`            | SHA-1 hex digest                                         |
| `crypto.md5(s)`             | MD5 hex digest                                           |
| `crypto.hash(algo, s)`      | Hash with named algorithm: `"sha256"`, `"sha512"`, etc. |
| `crypto.hmac(algo, key, data)` | HMAC hex digest with the given algorithm             |

```lx
val hash = crypto.sha256("hello")
val hmac = crypto.hmac("sha256", "my-secret-key", "Hello, Lunex!")
```

### Encoding

| Function                    | Description                            |
|-----------------------------|----------------------------------------|
| `crypto.base64Encode(s)`    | Standard Base64 encode                 |
| `crypto.base64Decode(s)`    | Standard Base64 decode                 |
| `crypto.base64UrlEncode(s)` | URL-safe Base64 encode (no padding)    |
| `crypto.base64UrlDecode(s)` | URL-safe Base64 decode                 |
| `crypto.toHex(s)`           | Convert bytes to hex string            |
| `crypto.fromHex(s)`         | Convert hex string to bytes            |

### Symmetric encryption

| Function                         | Description                                     |
|----------------------------------|-------------------------------------------------|
| `crypto.encrypt(plaintext, key)` | AES-256 encrypt; returns base64 ciphertext      |
| `crypto.decrypt(ciphertext, key)`| AES-256 decrypt; returns plaintext              |

```lx
val key        = "my-32-char-key-here-padding-ok!!"
val ciphertext = crypto.encrypt("top secret message", key)
val plaintext  = crypto.decrypt(ciphertext, key)
```

### Random values

| Function             | Description                                     |
|----------------------|-------------------------------------------------|
| `crypto.randomUUID()`| Generate a RFC-4122 UUID v4                     |
| `crypto.randomBytes(n)` | n cryptographically random bytes as hex      |
| `crypto.randomHex(n)`| n random bytes as a hex string                  |
| `crypto.token(n)`    | Random URL-safe token of n bytes                |
| `crypto.compare(a, b)` | Constant-time string comparison (safe for secrets/tokens) |

### Key derivation

| Function                                     | Description                                              |
|--------------------------------------------------|----------------------------------------------------------------|
| `crypto.pbkdf2(password, salt, iterations?, keyLen?)` | PBKDF2-HMAC-SHA256 key derivation, returns hex; defaults: 100000 iterations, 32-byte key |

### Password hashing

| Function                            | Description                         |
|-------------------------------------|-------------------------------------|
| `crypto.hashPassword(password)`     | Bcrypt hash at cost 10              |
| `crypto.verifyPassword(pwd, hash)`  | Verify a bcrypt hash                |

### JWT (embedded in crypto)

`std.crypto` also exposes a `jwt` sub-object:

```lx
val crypto = @import("std.crypto")

val token   = crypto.jwt.sign({ userId: 42, role: "admin" }, "secret", 3600) 
val payload = crypto.jwt.verify(token, "secret")  
val raw     = crypto.jwt.decode(token)            
```

> **`crypto.jwt` is a separate, simpler implementation from the dedicated
> `std.jwt` module below** — they don't share code and their return shapes
> differ. `crypto.jwt.verify` returns the payload object directly, or `null`
> if the token is invalid/expired. `std.jwt.verify` (below) instead always
> returns a `{ valid, payload }` / `{ valid: false, error }` wrapper object.
> Tokens signed with one are compatible with the other (both are standard
> HMAC-signed JWTs), but don't mix up which `verify`/`decode` shape to expect.

For the dedicated JWT module, see `std.jwt` below.

---

## `std.fs` — File system

```lx
val fs = @import("std.fs")
```

### Reading

| Function               | Returns | Description                        |
|-------------------------|---------|--------------------------------------|
| `fs.readFile(path)`     | string  | Read entire file as a UTF-8 string   |
| `fs.readLines(path)`    | array   | Read file and split by newline       |

### Writing

| Function                    | Description                   |
|-------------------------------|-----------------------------------|
| `fs.writeFile(path, data)`    | Write (overwrite) a file          |
| `fs.appendFile(path, data)`   | Append to a file                  |

### Data formats

| Function                            | Returns | Description                                          |
|----------------------------------------|---------|------------------------------------------------------------|
| `fs.readJSON(path)`                     | value   | Read a file and parse it as JSON                             |
| `fs.writeJSON(path, value, indent?)`    | boolean | Serialize a value to JSON and write it; `indent` is a space count |
| `fs.readCSV(path)`                      | array   | Read a CSV file into an array of rows                        |
| `fs.writeCSV(path, rows)`               | boolean | Write an array of objects (or arrays) as CSV                 |

### File operations

| Function              | Description                             |
|--------------------------|----------------------------------------------|
| `fs.delete(path)`        | Delete a file                                 |
| `fs.deleteAll(path)`     | Delete a file or directory recursively        |
| `fs.rename(src, dst)`    | Rename a file or directory                    |
| `fs.moveFile(src, dst)`  | Move a file to a new path                     |
| `fs.copy(src, dst)`      | Copy a file                                   |
| `fs.copyFile(src, dst)`  | Alias for `fs.copy`                           |

### Directory operations

| Function             | Returns | Description                         |
|------------------------|---------|-----------------------------------------|
| `fs.mkdir(path)`       | —       | Create directory and all parents        |
| `fs.ensureDir(path)`   | boolean | Alias for `fs.mkdir`, returns success   |
| `fs.rmdir(path)`       | —       | Remove an empty directory               |
| `fs.list(path)`        | array   | List directory entries                  |
| `fs.readDir(path)`     | array   | Alias for `fs.list`                     |

Each entry returned by `fs.list` / `fs.readDir` is an object:

```lx
{ name, path, isDir, isFile, size }
```

### Metadata

| Function            | Returns | Description                                           |
|-----------------------|---------|-------------------------------------------------------------|
| `fs.exists(path)`     | boolean | True if path exists                                          |
| `fs.stat(path)`       | object  | `{ name, size, isDir, isFile, mode, modTime }`                |
| `fs.isDir(path)`      | boolean | True if path is a directory                                  |
| `fs.isFile(path)`     | boolean | True if path is a regular file                               |
| `fs.size(path)`       | number  | File size in bytes                                           |

### Path helpers

| Function                     | Returns | Description                                     |
|---------------------------------|---------|-------------------------------------------------------|
| `fs.abs(path)`                  | string  | Absolute path                                          |
| `fs.join(...parts)`             | string  | Join path segments using the OS separator              |
| `fs.dirname(path)`              | string  | Parent directory                                        |
| `fs.basename(path, suffix?)`    | string  | Final path segment, with an optional suffix stripped    |
| `fs.extname(path)`              | string  | File extension, including the leading `.`               |
| `fs.glob(pattern)`              | array   | Paths matching a glob pattern                            |
| `fs.cwd()`                      | string  | Current working directory                                |
| `fs.home()`                     | string  | Current user's home directory                            |

### Temp files

| Function                | Returns | Description                                    |
|---------------------------|---------|------------------------------------------------------|
| `fs.tempFile(prefix?)`    | string  | Create and return the path to a new temp file          |
| `fs.tempDir(prefix?)`     | string  | Create and return the path to a new temp directory      |

---

## `std.http` — HTTP client and server

```lx
val http = @import("std.http")
```

`std.http` gives you the basics: a client, a server, request and response
objects, cookies, URL helpers and status codes. It doesn't route anything. If
you need routes, use `std.http.router`; for serving files from a folder, use
`std.http.static`. Both are documented below.

The module never guesses what you mean. JSON only goes out through `json`
calls and options, and text goes out through `text`, `html` and `end`.

### Client

| Function                           | Description                      |
|------------------------------------|----------------------------------|
| `http.request(method, url, opts?)` | Request with any method          |
| `http.get(url, opts?)`             | GET                              |
| `http.post(url, opts?)`            | POST                             |
| `http.put(url, opts?)`             | PUT                              |
| `http.patch(url, opts?)`           | PATCH                            |
| `http.delete(url, opts?)`          | DELETE                           |
| `http.head(url, opts?)`            | HEAD                             |

The URL has to be absolute and use `http` or `https`. Options:

| Option             | Default    | Description                                                         |
|--------------------|------------|---------------------------------------------------------------------|
| `headers`          | none       | Object mapping header names to strings or numbers                   |
| `body`             | none       | Request body, as a string                                           |
| `json`             | none       | Any value that can be turned into JSON; sets `Content-Type` unless you already did |
| `timeout`          | `30000`    | Milliseconds for the whole request, redirects and body included     |
| `maxResponseBytes` | `10485760` | Biggest response body you're willing to accept                      |
| `maxRedirects`     | `10`       | How many redirects to follow. `0` hands you the redirect itself     |

You can't pass both `body` and `json`, and a `HEAD` request can't have a
body. Options that don't exist are an error, which catches typos early.

What you get back is always an object:

```lx
{ ok, status, statusText, headers, cookies, text, error, errorCode, json() }
```

`ok` is true for 2xx. Header names are lower-case, and headers that appear
more than once are joined with `, `. `cookies` is an array with every raw
`Set-Cookie` value, since those can't be joined safely. `text` is the body
as received, and `json()` parses it (it throws if the body is empty or isn't
valid JSON).

When the request itself fails, nothing is thrown. You get `ok: false`,
`status: 0`, and `error` and `errorCode` describing what happened:

| `errorCode`                 | Meaning                                   |
|-----------------------------|-------------------------------------------|
| `E_HTTP_TIMEOUT`            | The time limit ran out                    |
| `E_HTTP_NETWORK`            | DNS, connection, TLS or other transport failure |
| `E_HTTP_TOO_MANY_REDIRECTS` | More redirects than `maxRedirects`        |
| `E_HTTP_RESPONSE_TOO_LARGE` | Body bigger than `maxResponseBytes`       |

```lx
val resp = http.post("https://api.example.com/items", {
  json: { name: "book" },
  headers: { Authorization: "Bearer token" },
  timeout: 5000
})

if resp.ok {
  io.log(resp.json())
} else {
  io.log(resp.status, resp.errorCode)
}
```

### Errors

Mistakes in how you call something (wrong types, unknown options, a bad URL,
a header with a line break in it) throw an object you can catch:

```lx
{ name: "HttpError", code, message, status }
```

`http.error(status, message, code?)` builds one, and you can throw it from a
handler to answer with that status. Statuses go from 400 to 599, and the
message is only sent to the client when the status is below 500.

| `code`                     | When                                                 |
|----------------------------|------------------------------------------------------|
| `E_HTTP_INVALID_ARGUMENT`  | An argument or option has the wrong type or value    |
| `E_HTTP_INVALID_URL`       | A URL can't be parsed or isn't http/https            |
| `E_HTTP_INVALID_HEADER`    | A header name or value isn't valid                   |
| `E_HTTP_INVALID_STATUS`    | A status code is out of range                        |
| `E_HTTP_INVALID_JSON`      | JSON couldn't be produced or parsed                  |
| `E_HTTP_INVALID_COOKIE`    | A cookie name, value or option isn't valid           |
| `E_HTTP_RESPONSE_FINISHED` | You used `res` after the response was already sent   |
| `E_HTTP_ALREADY_LISTENING` | `listen` was called twice, or after `close`          |
| `E_HTTP_LISTEN_FAILED`     | The port couldn't be bound                           |

### Server

```lx
val server = http.createServer(handler, options?)
server.listen(port, host?, onReady?)
server.close(timeoutMs?)
```

`handler` is `fn(req, res)`. `listen` needs a port (`0` lets the system pick
one). `host` defaults to `"0.0.0.0"`, and `onReady` is called with the port
that was bound. Once listening, `server.port` and `server.host` are filled in
and `listen` returns the server. `close` stops taking new connections and
gives running requests up to `timeoutMs` (default `5000`) to finish; calling
it again is harmless.

`http.listen(server, port, host?, onReady?)` and `http.close(server, timeoutMs?)`
do the same thing as the methods.

| Option              | Default   | Description                                                   |
|---------------------|-----------|---------------------------------------------------------------|
| `maxBodyBytes`      | `1048576` | Biggest request body. Bigger ones get a `413`                 |
| `maxHeaderBytes`    | `65536`   | Biggest header block                                          |
| `maxHeaderCount`    | `100`     | Most headers allowed. More get a `431`                        |
| `readHeaderTimeout` | `10000`   | Milliseconds to receive the headers                           |
| `readTimeout`       | `30000`   | Milliseconds to receive the whole request                     |
| `writeTimeout`      | `60000`   | Milliseconds to send the response                             |
| `idleTimeout`       | `60000`   | Milliseconds an idle keep-alive connection stays open         |
| `handlerTimeout`    | `30000`   | Milliseconds the handler has to respond. After that: `504`    |
| `onError`           | none      | `fn(err, req, res)`, called when the handler throws           |

The timeouts can be turned off with `0`. Unknown options are an error.

A few things worth knowing about how requests run. The body is read in full
(up to `maxBodyBytes`) before your handler is called, and requests that are
malformed or too large are answered without reaching it. Handlers run one at
a time, so don't have a handler make a blocking request to its own server.
Each request gets exactly one response: a second `res.json(...)` throws
`E_HTTP_RESPONSE_FINISHED`. The response doesn't have to be sent before the
handler returns, so spawned code can send it later, as long as that happens
within `handlerTimeout`.

If the handler throws, `onError` runs first when you set one. If the response
still hasn't been sent, the server sends the `HttpError` status, or a plain
`500` for any other error, and logs the error to stderr. If the client
disconnects, the pending response is dropped. Every response carries
`X-Content-Type-Options: nosniff`.

**Request**

| Field / method     | Description                                                      |
|--------------------|------------------------------------------------------------------|
| `req.method`       | Request method                                                   |
| `req.url`          | Path plus query string, as requested                             |
| `req.path`         | Decoded path                                                     |
| `req.rawPath`      | Path exactly as sent, still percent-encoded                      |
| `req.query`        | First value of each query parameter                              |
| `req.queryAll`     | Every value of each query parameter, as arrays                   |
| `req.headers`      | Headers with lower-case names                                    |
| `req.cookies`      | Cookies from the `Cookie` header                                 |
| `req.body`         | Raw body as a string                                             |
| `req.ip`           | Client address, without the port                                 |
| `req.host`         | The `Host` header                                                |
| `req.text()`       | Same as `req.body`                                               |
| `req.json()`       | Parses the body as JSON. Throws `E_HTTP_INVALID_JSON` (status 400) if it's empty or invalid |
| `req.header(name)` | One header, case-insensitive, or `null`                          |

**Response**

| Method                           | Description                                                   |
|----------------------------------|---------------------------------------------------------------|
| `res.status(code)`               | Sets the status (200–599) for the body call that follows; returns `res` |
| `res.setHeader(name, value)`     | Sets a header; returns `res`                                  |
| `res.getHeader(name)`            | Reads a header you set, or `null`                             |
| `res.removeHeader(name)`         | Removes a header; returns `res`                               |
| `res.cookie(name, value, opts?)` | Adds a `Set-Cookie`; returns `res`                            |
| `res.clearCookie(name, opts?)`   | Expires a cookie; returns `res`                               |
| `res.json(value, status?)`       | Sends `value` as JSON                                         |
| `res.text(string, status?)`      | Sends `text/plain`                                            |
| `res.html(string, status?)`      | Sends `text/html`                                             |
| `res.end(string?, status?)`      | Sends a raw string and leaves `Content-Type` alone            |
| `res.redirect(url, status?)`     | Redirects with 301, 302 (default), 303, 307 or 308            |
| `res.finished()`                 | `true` once the response has been sent                        |

`json`, `text`, `html`, `end` and `redirect` all finish the response. `json`
wants a value (`undefined` is refused), `text` and `html` want a string, and
`end` takes a string or nothing. A `204` or `304` never carries a body.
`Content-Length` and `Transfer-Encoding` belong to the server, and cookies go
through `res.cookie` rather than `setHeader`.

If you prefer functions over methods, `http.json(res, value, status?)`,
`http.text(res, string, status?)`, `http.html(res, string, status?)`,
`http.end(res, string?, status?)` and `http.redirect(res, url, status?)` call
the matching method.

```lx
fn handle(req, res) {
  if req.path == "/health" {
    res.json({ status: "ok" })
  } else {
    res.text("not found", 404)
  }
}

val server = http.createServer(handle, { maxBodyBytes: 65536 })
server.listen(3000)
```

### Cookies

`res.cookie` and `http.serializeCookie(name, value, opts?)` take the same
options. A bad name, value or option throws `E_HTTP_INVALID_COOKIE`.

| Option     | Default | Description                                       |
|------------|---------|---------------------------------------------------|
| `path`     | `"/"`   | Cookie path                                       |
| `domain`   | none    | Cookie domain                                     |
| `maxAge`   | none    | Lifetime in whole seconds                         |
| `secure`   | `false` | Only send over HTTPS                              |
| `httpOnly` | `true`  | Keep it away from scripts in the browser          |
| `sameSite` | `"lax"` | `"lax"`, `"strict"` or `"none"` (`"none"` needs `secure: true`) |

`http.parseCookies(header)` turns a `Cookie` header value into an object. If
a name shows up twice, the first one wins. Values are not percent-decoded.

### URL helpers

| Function                       | Description                                                          |
|--------------------------------|----------------------------------------------------------------------|
| `http.parseURL(url)`           | Returns `{ protocol, username, host, hostname, port, path, query, search, hash }` |
| `http.buildURL(base, params?)` | Adds query parameters to a URL; arrays repeat the key, keys are sorted |
| `http.parseQuery(s, all?)`     | Parses a query string; with `all: true` every value is an array      |
| `http.buildQuery(obj)`         | Builds an encoded query string, sorted by key                        |
| `http.encode(s)`               | Percent-encodes everything except `A-Z a-z 0-9 - _ . ~`              |
| `http.decode(s)`               | Percent-decodes; `+` stays `+`. Throws on bad escapes                |
| `http.statusText(code)`        | The standard reason phrase                                           |

`http.status` has the usual names: `OK`, `CREATED`, `NO_CONTENT`,
`BAD_REQUEST`, `UNAUTHORIZED`, `FORBIDDEN`, `NOT_FOUND`, `CONFLICT`,
`UNPROCESSABLE_ENTITY`, `TOO_MANY_REQUESTS`, `INTERNAL_SERVER_ERROR`,
`SERVICE_UNAVAILABLE` and a few more.

---

## `std.http.router` — Routing

```lx
val http   = @import("std.http")
val router = @import("std.http.router")

val api = router.create([
  router.get("/users/:id", users.show),
  router.post("/users", users.create),
  router.delete("/users/:id", users.remove)
])

api.listen(3000)
```

`router.get`, `post`, `put`, `patch`, `delete`, `head`, `options` and `all`
each take `(path, handler)` and return a route definition. `all` matches any
method. `router.create(routes, options?)` turns an array of definitions into
a router. It throws `E_ROUTER_INVALID_ROUTE` if something is wrong, such as a
bad path or two routes that are the same.

Paths start with `/`. A segment written `:name` matches any single segment
and puts the decoded text in `req.params.name`; names are letters, digits and
`_`, can't start with a digit, and can't repeat within a path. A final `*`
matches whatever is left, including nothing, and lands in `req.params["*"]`.
Everything else must match exactly, including case. A trailing `/` on the
request is ignored, while an empty segment (`//`), more than 64 segments or
more than 2048 characters never match.

Routes are stored in a trie of path segments, so lookup cost depends on the
length of the path and not on how many routes you have. At each segment the
exact match is tried first, then `:name`, then `*`, and the search backs up
if a branch has nothing for the request's method.

Method rules:

- A route made with `all` is used when nothing matches the exact method.
- `HEAD` falls back to the `GET` route.
- If the path exists but not for that method, the answer is `405` with an
  `Allow` header. An `OPTIONS` request gets `204` with `Allow`.
- If nothing matches, the answer is `404`. A malformed path gets `400`.
- Those replies are JSON, `{ "error": ..., "code": ... }`, with the codes
  `E_ROUTER_NOT_FOUND`, `E_ROUTER_METHOD_NOT_ALLOWED` and `E_ROUTER_BAD_PATH`.

Handlers get the normal `(req, res)` from `std.http` with `req.params` added.
There's no middleware. When you want shared behavior, wrap the handler in a
function:

```lx
fn requireAuth(handler) {
  fn(req, res) {
    if req.header("authorization") == null {
      throw http.error(401, "unauthorized")
    }
    handler(req, res)
  }
}

router.get("/account", requireAuth(account.show))
```

Options for `router.create`:

| Option     | Description                                                           |
|------------|-----------------------------------------------------------------------|
| `notFound` | `fn(req, res)` used in place of the default `404` reply               |
| `onError`  | `fn(err, req, res)` called when a route handler throws                |
| `server`   | Options handed to `http.createServer` when you call `api.listen`      |

The router object has three members:

| Member                              | Description                                                   |
|-------------------------------------|---------------------------------------------------------------|
| `api.handle(req, res)`              | The request handler, if you'd rather call `http.createServer(api.handle)` yourself |
| `api.listen(port, host?, onReady?)` | Creates a server (using `options.server`), listens, and returns it |
| `api.find(method, rawPath)`         | Returns `{ found, params, pattern, allowed }` without running any handler |

---

## `std.http.static` — Static files

```lx
val router = @import("std.http.router")
val files  = @import("std.http.static")

val assets = files.create("./public", { prefix: "/assets", maxAge: 3600 })

val api = router.create([
  router.get("/assets", assets),
  router.get("/assets/*", assets)
])
```

`files.create(root, options?)` returns a `fn(req, res)` handler that serves
files from `root` on `GET` and `HEAD` requests (other methods get a `405`).
`root` has to be an existing directory.

| Option     | Default        | Description                                                      |
|------------|----------------|------------------------------------------------------------------|
| `prefix`   | `"/"`          | Part of the URL path to drop before looking for the file         |
| `index`    | `"index.html"` | File to serve for a directory. `""` turns directory indexes off  |
| `dotfiles` | `"deny"`       | `"allow"` serves files and folders whose names start with `.`    |
| `maxAge`   | `0`            | Seconds for `Cache-Control: public, max-age=...`. `0` sends none |

Anything that isn't a regular file inside `root` gets a `404`. Paths with
`..` segments, backslashes or NUL bytes are refused, and so are symbolic links
that point outside `root`. The content type comes from the file extension, and
range and conditional requests work.

---

## `std.ws` — WebSockets

```lx
val ws = @import("std.ws")
```

`std.ws` implements RFC 6455 WebSockets with text messages, control frames,
fragmentation, client masking, random handshake keys, and both `ws://` and
`wss://` client connections. DNS resolution uses the operating system
resolver.

### Server

`ws.createServer(port, connHandler?, options?)` starts listening immediately and
returns a server handle. `connHandler(client)` is called once per new
connection with a client object. The optional options object supports TLS with
`certFile` and `keyFile`.

```lx
val server = ws.createServer(8081, fn(client) {
  io.log("client connected")
  client.onMessage(fn(msg) { client.send("echo: " + msg) })
  client.onClose(fn() { io.log("client left") })
})

io.log("listening on port", server.port)
```

| Function | Description |
|---|---|
| `ws.createServer(port, connHandler?, options?)` | Start a WebSocket server; returns `{ port, secure, broadcast(msg), clientCount(), close() }` |
| `ws.send(client, msg)` | Shorthand for `client.send(msg)` |
| `ws.onMessage(client, fn)` | Shorthand for `client.onMessage(fn)` |
| `ws.onClose(client, fn)` | Shorthand for `client.onClose(fn)` |
| `ws.closeServer(server)` | Shorthand for `server.close()` |

A **client object** (passed into `connHandler`, or returned by `ws.connect`)
has these methods:

| Method | Description |
|---|---|
| `client.send(msg)` | Send a text message; objects and arrays are JSON-encoded |
| `client.close()` | Close the connection |
| `client.onMessage(fn)` | Register `fn(message)` |
| `client.onClose(fn)` | Register a close callback |
| `client.isClosed()` | Return whether the connection is closed |

A **server object** has:

| Method | Description |
|---|---|
| `server.port` | The listening port |
| `server.secure` | `true` when TLS is enabled |
| `server.broadcast(msg)` | Send a message to every connected client |
| `server.clientCount()` | Return the number of connected clients |
| `server.close()` | Stop accepting new connections |

For a secure server, provide PEM certificate and private-key files:

```lx
val server = ws.createServer(8443, fn(client) {
  client.onMessage(fn(msg) { client.send(msg) })
}, { certFile: "server.crt", keyFile: "server.key" })
```

TLS servers use TLS 1.2 or newer. The `server.secure` property is `true` when
TLS is enabled.

### Client

```lx
val client = ws.connect("ws://localhost:8081")
client.onMessage(fn(msg) { io.log("server:", msg) })
client.send("hello")
client.close()
```

Secure WebSockets are supported through `wss://` and use TLS 1.2 or newer.

```lx
val client = ws.connect("wss://example.com/socket")
client.onMessage(fn(msg) { io.log(msg) })
client.send("hello")
```

| Function | Description |
|---|---|
| `ws.connect(url)` | Connect using `ws://` or `wss://`; returns a client object |
| `ws.closeClient(client)` | Shorthand for `client.close()` |

The client generates a cryptographically random `Sec-WebSocket-Key` for every
handshake. Client-to-server frames are masked with a fresh random masking key,
while server-to-client frames are sent unmasked as required by RFC 6455.

---

## `std.db` — SQLite-backed document database

```lx
val db = @import("std.db")
```

`std.db` is a persistent document database backed by SQLite. Databases are
stored as `.db` files under `.lunex/data/`, and data survives process
restarts.

### Database lifecycle

| Function | Description |
|---|---|
| `db.open(name?)` | Open or create a database. The default name is `default`. |
| `db.create(name?)` | Alias for `db.open`. |
| `db.connect(name?)` | Alias for `db.open`. |
| `db.drop(name)` | Delete a database file and its SQLite sidecar files. |
| `db.list()` | Return database names found under `.lunex/data/`. |
| `db.table(name)` | Get or create a table on the default database. |
| `db.collection(name)` | Alias for `db.table`. |

A database object exposes `table`, `collection`, `tables`, `drop`, `transaction`,
`dump`, `load`, `close`, `name`, and `path`.

```lx
val database = db.open("app")
val users = database.table("users")
io.log(database.path)
database.close()
```

### Schema

```lx
users.schema({
  id: { type: "string", default: "$uuid" }
  active: { type: "boolean", default: true }
  name: { type: "string", required: true }
  email: { type: "string", required: true, unique: true }
  age: { type: "number", default: 0, min: 0 }
})
```

`table.define(...)` is an alias for `table.schema(...)`.

Supported field definition keys:

| Key | Description |
|---|---|
| `type` | `string`, `number`, `boolean`, `date`, `object`, `array`, `function`, or `any`. `bool` is accepted as an alias for `boolean`. Integer-like aliases are normalized to `number`. |
| `required` | Reject records where the field is missing or nullish. |
| `unique` | Enforce uniqueness and create a unique SQLite index. |
| `index` | Create a non-unique SQLite index. |
| `primary` | Make the field the canonical public identifier and enforce required uniqueness. |
| `min` / `max` | Numeric value limits. |
| `minLength` / `maxLength` | String length limits. |
| `enum` | Restrict values to the listed values. |
| `ref` | Store a table reference used by application code. |
| `default` | Static value or generated `$uuid`, `$now`, or `$seq`. |
| `onUpdate` | Generated update value such as `"$now"` on every update. |

The field name `_id` is reserved by the database API. When a schema contains
`id`, that field is the canonical public identifier. The exposed `_id` value is
an alias of the same public identifier, not a second application identifier.
Without a schema identifier field, `_id` is the automatically generated record
identifier.

Schema and index metadata are persisted inside the database and are restored
when the table is reopened.

### Writing data

| Method | Description |
|---|---|
| `table.insert(record)` | Insert one record, apply defaults, and validate the schema. |
| `table.insertMany(records)` | Insert a complete batch atomically. Invalid input or a constraint failure rolls the whole batch back. |
| `table.upsert(query, patch)` | Update the first match or insert a document composed from equality query fields and the patch. |
| `table.update(query, patch)` | Update all matching records. Supports plain fields and `$set`, `$unset`, `$inc`, `$push`. |
| `table.updateOne(query, patch)` | Update only the first matching record. |
| `table.delete(query)` | Delete all matching records. |
| `table.deleteOne(query)` | Delete only the first matching record. |
| `table.deleteById(id)` | Delete the record identified by the schema's canonical identifier. |
| `table.clear()` | Remove all records while keeping the schema and indexes. |
| `table.drop()` | Drop the table, schema metadata, indexes, and records. |

All write operations propagate SQLite errors and preserve in-memory state when
a write fails.

### Reading data

| Method | Description |
|---|---|
| `table.find(query?, options?)` | Find matching records. Options include `select`, `sort`, `orderBy`, `limit`, `offset`, and `skip`. |
| `table.findOne(query?)` | Return the first matching record or `null`. |
| `table.findById(id)` | Find a record by its canonical public identifier. |
| `table.count(query?)` | Count matching records. |
| `table.exists(query?)` | Return `true` when a matching record exists. |
| `table.distinct(field, query?)` | Return type-aware distinct field values. |
| `table.search(text, fields?)` | Case-insensitive substring search. |
| `table.dump()` | Return all records. |
| `table.indexes()` | Return persisted index definitions. |

There is no `table.all()`. Use `table.find()` or `table.dump()`.

### Query builder

`table.where(query)`, `table.select(fields)`, `table.orderBy(field, direction?)`,
and `table.limit(n)` return a chainable query builder.

```lx
val activeAdults = users
  .where({ active: true })
  .and({ age: { $gte: 18 } })
  .orderBy("age", "desc")
  .limit(10)
  .find()
```

| Method | Description |
|---|---|
| `.where(query)` | Set or replace the filter. Supports an object filter or `(field, operator, value)`. |
| `.and(query)` / `.or(query)` | Combine the current filter. |
| `.select(fields)` | Project only the listed fields. |
| `.orderBy(field, "asc"\|"desc"?)` | Add a sort key. |
| `.limit(n)` | Limit results. `0` returns no records. |
| `.offset(n)` / `.skip(n)` | Skip records. |
| `.page(page, size)` | Apply page-based offset and limit. |
| `.find()` | Execute and return an array. This matches `table.find(...)`. |
| `.exec()` | Execute and return an array. Alias of `.find()`. |
| `.first()` | Return the first result or `null`. |
| `.last()` | Return the last result or `null`. |
| `.count()` | Count matches. |
| `.exists()` | Test whether a match exists. |
| `.update(patch)` | Update all matches. |
| `.delete()` | Delete all matches. |

### Filters

Filters use plain objects with equality values and Mongo-style operators:

`$eq`, `$ne`, `$gt`, `$gte`, `$lt`, `$lte`, `$in`, `$nin`, `$like`, `$ilike`,
`$regex`, `$exists`, `$between`, `$contains`, `$size`, `$type`, `$startsWith`,
`$endsWith`, plus `$and`, `$or`, `$not`, and `$nor`.

```lx
users.find({ age: { $gte: 18, $lt: 65 }, name: { $startsWith: "A" } })
users.find({ $or: [{ role: "admin" }, { role: "owner" }] })
users.where("age", ">", 38).find()
users.orderBy("age", "desc").where("age", ">=", 18).find()
```

Three-argument `where(field, operator, value)` filters are translated to the
same internal operator form used by object filters. Supported comparison
operators are `=`, `==`, `!=`, `<>`, `>`, `>=`, `<`, `<=` and their `$eq`, `$ne`,
`$gt`, `$gte`, `$lt`, and `$lte` forms. Invalid filter types and unsupported
operators fail with an error instead of being treated as an unfiltered query.

### Aggregation

`table.aggregate(pipeline)` supports `$match`, `$sort`, `$limit`, `$skip`,
`$project`, `$group`, `$unwind`, and `$count`.

`$group` supports `$count`, `$sum`, `$avg`, `$min`, `$max`, `$first`, `$last`,
`$push`, and `$addToSet`. Group keys support the documented Mongo-style `_id`
expression syntax.

```lx
val byRole = users.aggregate([
  { $match: { active: true } }
  { $group: { _id: "$role", total: { $count: {} }, averageAge: { $avg: "$age" } } }
])
```

Dedicated aggregate helpers are also available:
`table.sum(field, query?)`, `table.avg(field, query?)`, `table.min(field, query?)`,
and `table.max(field, query?)`.

### Joins

`table.join(otherTable, localField, foreignField, options?)` performs an
in-memory cross-table join and returns a new record array without modifying
either table.

```lx
val orders = database.table("orders")
val joined = orders.join(users, "userId", "id", { as: "user", type: "left", single: true })
```

`type` accepts `left` or `inner`. The default is `left`. `single: true` places
one matching record or `null` in the joined field; otherwise the joined field
contains an array.

### Transactions

`database.transaction(fn)` executes the callback inside a real SQLite
transaction. A callback error rolls the transaction back. A successful callback
is committed atomically, and cached table state is refreshed after commit.

```lx
val result = database.transaction(fn(tx) {
  val accounts = tx.table("accounts")
  accounts.update({ id: "a" }, { $inc: { balance: -100 } })
  accounts.update({ id: "b" }, { $inc: { balance: 100 } })
  "committed"
})
```

Nested transactions are rejected.

### Indexes

`table.index(field, { unique? })` creates a persistent SQLite index. Compound
indexes can be created with an array of fields.

```lx
table.index("email", { unique: true })
table.index(["tenantId", "createdAt"])
```

### Change notifications

`table.watch(query?, fn)` registers a synchronous callback for matching insert,
update, and delete events and returns an `unwatch()` function.

## `std.buffer` — Byte buffers

```lx
val buffer = @import("std.buffer")
```

A `buffer` is a mutable, addressable block of bytes, separate from `string`
and `array`. It exists for binary formats, network protocols, and file
parsing — anywhere fixed-width integers and raw byte layout matter.

### Creating buffers

| Function                       | Description                                       |
|---------------------------------|----------------------------------------------------|
| `buffer.alloc(size)`            | New zero-filled buffer of `size` bytes             |
| `buffer.from(string)`           | Buffer from a UTF-8 string                         |
| `buffer.from(string, "hex")`    | Buffer decoded from a hex string                   |
| `buffer.from(string, "base64")` | Buffer decoded from a base64 string                |
| `buffer.from(array)`            | Buffer from an array of byte values (0-255)        |
| `buffer.concat(buf1, buf2, …)`  | New buffer with all inputs concatenated            |

### Reading and writing

Every `readX`/`writeX` method takes a byte offset first. Multi-byte methods
take an optional byte order argument last: `"le"` (default) or `"be"`.

| Method                                | Description                             |
|-----------------------------------------|--------------------------------------------|
| `buf.readU8(offset)` / `buf.writeU8(offset, v)`   | Unsigned 8-bit integer          |
| `buf.readI8(offset)` / `buf.writeI8(offset, v)`   | Signed 8-bit integer            |
| `buf.readU16(offset, order?)` / `buf.writeU16(offset, v, order?)` | Unsigned 16-bit integer |
| `buf.readI16(offset, order?)` / `buf.writeI16(offset, v, order?)` | Signed 16-bit integer   |
| `buf.readU32(offset, order?)` / `buf.writeU32(offset, v, order?)` | Unsigned 32-bit integer |
| `buf.readI32(offset, order?)` / `buf.writeI32(offset, v, order?)` | Signed 32-bit integer   |
| `buf.readU64(offset, order?)` / `buf.writeU64(offset, v, order?)` | Unsigned 64-bit integer |
| `buf.readI64(offset, order?)` / `buf.writeI64(offset, v, order?)` | Signed 64-bit integer   |
| `buf.readF32(offset, order?)` / `buf.writeF32(offset, v, order?)` | 32-bit float             |
| `buf.readF64(offset, order?)` / `buf.writeF64(offset, v, order?)` | 64-bit float             |

Reads and writes outside the buffer's bounds are no-ops rather than errors —
`read*` returns `0`.

### Other operations

| Method                     | Description                                       |
|------------------------------|-----------------------------------------------------|
| `buf.length`                | Number of bytes                                    |
| `buf.slice(start, end)`     | New buffer copied from a byte range                |
| `buf.copyFrom(src, offset?)`| Copy another buffer's bytes into this one           |
| `buf.fill(byteValue)`       | Fill every byte with a value                        |
| `buf.resize(newSize)`       | Grow (zero-padded) or shrink in place                |
| `buf.toHex()`               | Encode as a hex string                              |
| `buf.toBase64()`            | Encode as a base64 string                           |
| `buf.toString()`            | Decode as a UTF-8 string                            |
| `buf.toArray()`             | Convert to an array of byte values                   |

```lx
val buffer = @import("std.buffer")

val buf = buffer.alloc(8)
buf.writeU32(0, 4021, "be")
buf.writeI16(4, -12, "be")
io.log(buf.readU32(0, "be"))  
io.log(buf.toHex())
```

---

## `std.ints` — Fixed-width integer arithmetic

```lx
val ints = @import("std.ints")
```

Lunex numbers are 64-bit floats, so they don't overflow or wrap the way C's
fixed-width integers do. `std.ints` reproduces that wraparound behavior
explicitly, which matters when porting code that depends on exact overflow
semantics (checksums, hashing, binary protocol fields).

### Casts

Each cast truncates the input to the target width and reinterprets the bit
pattern, matching a C-style `(uint32_t)x` cast.

| Function     | Range                                    |
|---------------|--------------------------------------------|
| `ints.u8(x)`  | 0 to 255                                   |
| `ints.i8(x)`  | −128 to 127                                |
| `ints.u16(x)` | 0 to 65535                                 |
| `ints.i16(x)` | −32768 to 32767                            |
| `ints.u32(x)` | 0 to 4294967295                            |
| `ints.i32(x)` | −2147483648 to 2147483647                  |
| `ints.u64(x)` | 0 to 18446744073709551615                  |
| `ints.i64(x)` | −9223372036854775808 to 9223372036854775807 |

### Wrapping arithmetic

`add`, `sub`, and `mul` are provided for every width, each suffixed with the
target type (`U8`, `I8`, `U16`, `I16`, `U32`, `I32`, `U64`, `I64`). All wrap
silently on overflow instead of losing precision as plain `+`/`-`/`*` would
past 2^53.

```lx
ints.addU8(250, 10)   
ints.subI8(-128, 1)   
ints.mulU32(200000, 200000)
```

### Shifts and rotation

| Function                 | Description                          |
|----------------------------|------------------------------------------|
| `ints.shlU32(x, n)`        | Logical left shift, 32-bit               |
| `ints.shrU32(x, n)`        | Logical right shift, 32-bit              |
| `ints.shlU64(x, n)`        | Logical left shift, 64-bit               |
| `ints.shrU64(x, n)`        | Logical right shift, 64-bit              |
| `ints.rotlU32(x, n)`       | Rotate left, 32-bit                      |
| `ints.rotrU32(x, n)`       | Rotate right, 32-bit                     |

Bitwise `&`, `|`, `^`, `~`, `<<`, `>>`, and `>>>` are also available directly
as operators anywhere in Lunex — no import needed.

### Overflow checks

| Function                  | Description                             |
|-----------------------------|--------------------------------------------|
| `ints.isU8Overflow(x)`      | True if x falls outside 0-255              |
| `ints.isI32Overflow(x)`     | True if x falls outside the i32 range      |

### Constants

`ints.U8_MAX`, `ints.I8_MAX`, `ints.I8_MIN`, `ints.U16_MAX`, `ints.I16_MAX`,
`ints.I16_MIN`, `ints.U32_MAX`, `ints.I32_MAX`, `ints.I32_MIN`,
`ints.U64_MAX`, `ints.I64_MAX`, `ints.I64_MIN`.

---

## `std.jwt` — JSON Web Tokens

```lx
val jwt = @import("std.jwt")
```

| Function                                | Returns  | Description                                   |
|-------------------------------------------|----------|-----------------------------------------------|
| `jwt.sign(payload, secret, options?)`      | string   | Sign a payload; returns a JWT string           |
| `jwt.verify(token, secret)`                | object   | Verify a token — see return shape below         |
| `jwt.decode(token)`                        | object   | `{ header, payload }` — decoded without verifying the signature |
| `jwt.isExpired(token)`                     | boolean  | True if the token's `exp` claim is in the past   |
| `jwt.refresh(token, secret, expiresIn?)`   | string   | Verify `token`, then issue a new one with a fresh `iat`/`exp` |

`jwt.sign` automatically adds `iat` (issued-at) to the payload, and adds
`exp` if `expiresIn` is set. `options` is an object supporting:

| Key          | Description                                        |
|--------------|-------------------------------------------------------|
| `algorithm`   | Signing algorithm (default `"HS256"`)                  |
| `expiresIn`   | Lifetime in seconds; sets the `exp` claim                |
| `issuer`      | Sets the `iss` claim                                       |
| `audience`    | Sets the `aud` claim                                          |
| `subject`     | Sets the `sub` claim                                            |

`jwt.verify` **never returns `null`** — it always returns an object:

```lx
val result = jwt.verify(token, "secret")
if result.valid {
  io.log(result.payload)
} else {
  io.log("invalid:", result.error)
}
```

> **Note:** `std.jwt` is a distinct implementation from the `jwt` sub-object
> exposed by `std.crypto` (`crypto.jwt`) — see the note in the `std.crypto`
> section above. In particular, `crypto.jwt.verify` returns the payload
> directly (or `null`), while `jwt.verify` here always returns a
> `{ valid, payload }` / `{ valid: false, error }` wrapper.

---

## `std.os` — Operating system

```lx
val os = @import("std.os")
```

### Process

| Function         | Returns | Description                  |
|--------------------|---------|----------------------------------|
| `os.getpid()`      | number  | Current process ID               |
| `os.pid()`         | number  | Alias for `os.getpid()`          |
| `os.getppid()`     | number  | Parent process ID                |
| `os.ppid()`        | number  | Alias for `os.getppid()`         |
| `os.exit(code?)`   | —       | Exit the process                 |
| `os.args()`        | array   | Command-line arguments           |

### Platform info

| Function         | Returns | Description                                              |
|--------------------|---------|----------------------------------------------------------------|
| `os.platform()`    | string  | `"linux"`, `"darwin"`, `"windows"`, `"android"`                 |
| `os.arch()`        | string  | `"amd64"`, `"arm64"`, etc.                                      |
| `os.hostname()`    | string  | Machine hostname                                                |
| `os.cpus()`        | number  | Number of logical CPUs                                         |
| `os.sep`           | string  | OS path separator (`"/"` or `"\"`)                              |
| `os.pathSep`       | string  | OS path-list separator (`:` or `;`)                             |
| `os.eol`           | string  | Line ending Lunex uses (`"\n"`)                                 |
| `os.homeDir`       | string  | Current user's home directory                                   |

`sep`, `pathSep`, `eol`, and `homeDir` are plain values, not functions.

### Working directory

| Function         | Returns | Description                        |
|--------------------|---------|------------------------------------------|
| `os.cwd()`         | string  | Alias for `os.getcwd()`                   |
| `os.getcwd()`      | string  | Current working directory                 |
| `os.chdir(path)`   | —       | Change working directory                  |

### Environment variables

| Function                    | Returns         | Description                                    |
|-------------------------------|------------------|------------------------------------------------------|
| `os.getenv(key)`               | string \| null   | Read environment variable                              |
| `os.setenv(key, value)`        | —                | Write an environment variable                          |
| `os.unsetenv(key)`             | —                | Remove an environment variable                         |
| `os.environ()`                 | object           | All environment variables as an object                 |
| `os.expandEnv(s)`               | string           | Expand `$VAR` and `${VAR}` in a string                  |

### Shell execution

| Function                  | Returns | Description                          |
|------------------------------|---------|--------------------------------------------|
| `os.exec(cmd, opts?)`         | object  | Run a command synchronously                 |
| `os.execSync(cmd, opts?)`     | object  | Alias for `os.exec`                         |
| `os.spawn(cmd, opts?)`        | object  | Run a command in the background             |

`os.exec` returns `{ stdout, stderr, code, ok }`.
`os.spawn` returns `{ pid, wait(), kill() }` — `wait()` blocks until the
process exits and returns its exit code (a number), and does not capture
`stdout`/`stderr`.

Optional opts object: `{ cwd, env, timeout }`.

> **Known limitation:** `cmd` is split into arguments by whitespace only
> (there's no shell involved). Quoted arguments containing spaces, `&&`,
> pipes, globbing, and other shell syntax are **not** interpreted — they're
> passed through literally as part of the split tokens. For anything beyond
> a simple `program arg1 arg2` command, invoke a shell explicitly, e.g.
> `os.exec("sh -c \"...\"")` on Unix.

```lx
val result = os.exec("git --version")
if result.ok {
  io.success(result.stdout)
} else {
  io.warn("git not found")
}
```

### File system (path utilities)

| Function                  | Returns | Description                              |
|------------------------------|---------|-------------------------------------------------|
| `os.join(...parts)`           | string  | Join path segments                                |
| `os.dirname(path)`            | string  | Parent directory of a path                        |
| `os.basename(path)`           | string  | File name portion of a path                       |
| `os.extname(path)`            | string  | File extension, including the leading `.`         |
| `os.abs(path)`                | string  | Absolute path                                     |
| `os.stat(path)`               | object  | `{ name, size, isDir, isFile, mode, modTime }`    |
| `os.exists(path)`             | boolean | True if path exists                               |
| `os.mkdir(path)`              | —       | Create directory and all parents                  |
| `os.remove(path)`             | —       | Delete a file or empty directory                  |
| `os.rename(src, dst)`         | —       | Rename or move a path                             |
| `os.listDir(path)`            | array   | List directory entries                            |
| `os.glob(pattern)`            | array   | Expand a glob pattern                             |
| `os.tempDir()`                | string  | Path to a system temporary directory              |
| `os.tempFile()`               | string  | Path to a new temporary file                      |

### Timing

| Function           | Returns | Description                                        |
|-----------------------|---------|------------------------------------------------------------|
| `os.time()`           | number  | Current Unix time in milliseconds                             |
| `os.hrtime()`         | number  | High-resolution monotonic-ish timestamp in milliseconds       |
| `os.sleep(ms)`        | —       | Pause execution for `ms` milliseconds                         |

---

## `std.regex` — Regular expressions

```lx
val regex = @import("std.regex")
```

Uses Go's RE2 syntax (no lookaheads or backreferences).

### Compiling

| Function                        | Returns | Description                                        |
|-------------------------------------|---------|---------------------------------------------------------|
| `regex.compile(pattern, flags?)`    | regex   | Precompile a pattern into a reusable regex value        |

```lx
val re = regex.compile("\\d+", "i")
```

### Flags support

Flags (e.g. `"i"` for case-insensitive) are only accepted by `regex.test`,
`regex.match`, `regex.matchAll`, `regex.groups`, `regex.groupsAll`, and
`regex.replace`. **`regex.replaceAll`, `regex.replaceFunc`, `regex.split`,
`regex.namedGroups`, `regex.isValid`, `regex.count`, `regex.index`, and
`regex.indices` do not take a flags parameter at all** — inline flags
(e.g. `(?i)` at the start of the pattern) are the only way to affect
case-sensitivity for those functions. A `regex.compile(pattern, flags)`
value carries its flags with it and works consistently everywhere it's
accepted.

### Testing

| Function                   | Returns | Description                               |
|----------------------------|---------|-------------------------------------------|
| `regex.test(s, pattern, flags?)` | boolean | True if pattern matches anywhere in s |
| `regex.isValid(pattern)`   | boolean | True if pattern is valid RE2 syntax       |

### Matching

| Function                     | Returns        | Description                                  |
|------------------------------|----------------|----------------------------------------------|
| `regex.match(s, pattern, flags?)`    | string \| null | First matching substring              |
| `regex.matchAll(s, pattern, flags?)` | array          | All non-overlapping matches           |
| `regex.index(s, pattern)`    | number         | Start index of first match (−1 if none)      |
| `regex.indices(s, pattern)`  | array          | Start indices of all matches                 |
| `regex.count(s, pattern)`    | number         | Number of non-overlapping matches            |

### Capture groups

| Function                         | Returns | Description                                   |
|----------------------------------|---------|-----------------------------------------------|
| `regex.groups(s, pattern, flags?)`    | array   | Capture groups from the first match      |
| `regex.groupsAll(s, pattern, flags?)` | array   | Capture groups from every match          |
| `regex.namedGroups(s, pattern)`  | object  | Named capture groups as an object             |

### Replacement

| Function                              | Returns | Description                     |
|---------------------------------------|---------|---------------------------------|
| `regex.replace(s, pattern, repl, flags?)` | string | Replace **all** matches      |
| `regex.replaceAll(s, pattern, repl)`  | string  | Replace all matches — identical behavior to `regex.replace` |
| `regex.replaceFunc(s, pattern, fn)`   | string  | Replace every match with the output of `fn(match)` |

> **Known behavior:** despite the name, `regex.replace` does **not** stop
> after the first match — it replaces every match in the string, exactly
> like `regex.replaceAll`. There is currently no built-in way to replace
> only the first occurrence; work around it with `regex.replaceFunc` and a
> counter, or `regex.index` plus manual string slicing.

### Splitting

| Function                  | Returns | Description          |
|---------------------------|---------|----------------------|
| `regex.split(s, pattern)` | array   | Split s on pattern   |

### Extraction helpers

| Function                   | Returns | Description                         |
|----------------------------|---------|-------------------------------------|
| `regex.extractNumbers(s)`  | array   | Extract all numeric substrings      |
| `regex.extractEmails(s)`   | array   | Extract all email addresses         |
| `regex.extractUrls(s)`     | array   | Extract all URLs                    |

### Escaping

| Function           | Returns | Description                            |
|--------------------|---------|----------------------------------------|
| `regex.escape(s)`  | string  | Escape all RE2 metacharacters in s     |

---

## `std.env` — Environment variables

```lx
val env = @import("std.env")
```
The module provides access to environment variables. Native access is limited to the underlying operating-system and filesystem primitives exposed by ""std.os"" and ""std.fs"".

The parser supports optional prefixes, keys composed of letters, digits, "_", "-", and ".", "=" or whitespace-sensitive ":" separators, unquoted values, single-quoted values, double-quoted values, backtick-quoted values, inline comments, empty values, and quoted multiline values. Double-quoted "\n" and "\r" sequences are converted to real line breaks.

### API

| Function | Returns | Description |
|---|---|---|
| `env.get(key)` | string \| undefined | Read an environment variable; returns `undefined` when it does not exist |
| `env.get(key, default)` | value | Read a variable with an explicit fallback |
| `env.has(key)` | boolean | Test whether the variable exists |
| `env.set(key, value)` | boolean | Set a variable; returns `false` when the OS rejects the name or write |
| `env.delete(key)` | boolean | Remove a variable; returns `false` when the OS rejects the name or operation |
| `env.all()` | object | Snapshot of the process environment |
| `env.parse(source)` | object | Parse dotenv source without modifying the process environment |
| `env.populate(values, override?)` | object | Apply parsed values and return only the keys written; existing variables are preserved unless `override` is `true` |
| `env.load(path?, override?)` | boolean | Read and parse a dotenv file, then populate the process environment |
| `env.require(key)` | string | Return a required variable; throws `E0110` when the key does not exist |
| `env.config(path?, override?)` | object | Load a dotenv file and return `{ parsed }` or `{ parsed, error }` |
| `env.int(key, default?)` | number | Parse an environment value as a number |
| `env.bool(key, default?)` | boolean | Parse `true`, `1`, `yes`, or `on` as `true` |

`env.get(key)` and `env.require(key)` intentionally have different contracts.
`get` is optional and returns `undefined` for a missing key; `require` is
explicit and throws a formatted Lunex runtime diagnostic when a key is absent.
An existing variable containing an empty string is still considered present.

`env.set` and `env.delete` no longer hide operating-system errors. Their boolean
result lets application code handle invalid names and failed environment
operations explicitly.

### Loading files

```lx
env.load()
env.load(".env")
env.load(".env.local")
env.load(".env.local", true)
```

The default path is `.env`, and `override` defaults to `false`. This follows
dotenv's normal non-overwrite population behavior; pass `true` when a later
file must replace an already defined variable.

### Application example

```lx
val env = @import("std.env")
val io = @import("std.io")

fn main() {
  env.load()

  val name = env.get("APP_NAME", "Lunex")
  val port = env.get("PORT", "3000")
  val debug = env.get("DEBUG", "false")

  io.log(name)
  io.log(port)
  io.log(debug)
}
```

### Parsing without loading

```lx
val parsed = env.parse("APP_NAME=Lunex\nPORT=3000\nDEBUG=true\n")
env.populate(parsed)
```

### Required variables

```lx
val port = env.require("PORT")
```

A missing required key emits `E0110`. Empty values are not treated as missing.
For code that should continue when a variable is absent, use `env.get(key,
default)` instead.

### Inspecting file errors

```lx
val result = env.config(".env")
if result.error != undefined {
  io.log(result.error.code)
  io.log(result.error.message)
}
```

`env.config` separates parsing/loading diagnostics from the boolean `env.load`
API, which is useful when the application needs the parsed values and an
explicit error object at the same time.

---

## `std.ffi` — Native library interface

```lx
val ffi = @import("std.ffi")
```

`std.ffi` provides native shared-library access through an explicit ABI signature.
Dynamic libraries are opened by the operating system, symbols are resolved by
name, calls use the native ABI bridge, and native memory can be
managed through Lunex pointer values.

FFI is disabled by default. Enable it for a process from the Lunex command
line:

```bash
lunex ffi = on run main.lx
```

The switch is process-local. It is not controlled by Lunex source code and
there is no environment-variable override.

### API

| Function | Returns | Description |
|---|---|---|
| `ffi.enabled()` | boolean | Report whether FFI is enabled for the current process |
| `ffi.load(path, options?)` | library | Open a native shared library |
| `ffi.open(path, options?)` | library | Alias for `load` |
| `ffi.bind(library, symbol, signature)` | function | Resolve and bind a native symbol to a typed callable |
| `ffi.symbol(library, symbol)` | pointer | Resolve a symbol to its native address |
| `ffi.call(target, args?)` | value | Call an existing bound function |
| `ffi.call(pointer, signature, args)` | value | Call a native function pointer using an explicit signature |
| `ffi.callback(signature, handler)` | pointer | Create a native callback that dispatches into Lunex |
| `ffi.pointer(value?)` | pointer | Convert a native address or `std.buffer` value to a pointer |
| `ffi.null()` | pointer | Return a null pointer |
| `ffi.nullPtr()` | pointer | Identifier-safe alias for `ffi.null()` |
| `ffi.isNull(pointer)` | boolean | Test whether a pointer is null |
| `ffi.alloc(size)` | pointer | Allocate zero-initialized native memory |
| `ffi.calloc(count, size)` | pointer | Allocate zero-initialized native memory using C `calloc` semantics |
| `ffi.realloc(pointer, size)` | pointer | Resize an owned native allocation |
| `ffi.free(pointer)` | boolean | Release an owned native allocation |
| `ffi.cstring(value)` | pointer | Allocate a NUL-terminated native string |
| `ffi.read(pointer, type, offset?)` | value | Read a typed value from native memory |
| `ffi.write(pointer, type, value, offset?)` | undefined | Write a typed value to native memory |
| `ffi.readBytes(pointer, length, offset?)` | array | Read raw bytes from native memory |
| `ffi.writeBytes(pointer, data, offset?)` | undefined | Write bytes from a string, array, or `std.buffer` |
| `ffi.readCString(pointer, maxLength?)` | string | Read a bounded NUL-terminated string |
| `ffi.copy(destination, source, length)` | undefined | Copy native memory using the host C runtime |
| `ffi.fill(pointer, value, length)` | undefined | Fill native memory with one byte value |
| `ffi.close(library)` | boolean | Close a native library; live bindings keep it referenced until released |
| `ffi.closeFunction(function)` | boolean | Release a bound native function and its library reference |
| `ffi.name(function)` | string | Return the bound native symbol name |
| `ffi.signature(function)` | string | Return the normalized ABI signature |
| `ffi.sizeof(type)` | number | Return the native size of a type |
| `ffi.alignof(type)` | number | Return the native alignment of a type |
| `ffi.typeInfo(type)` | object | Return normalized type, size, alignment, pointer, array, and field metadata |

Library values expose `path`, `bind`, `symbol`, `close`, `isClosed`, `handle`,
and `active`. Pointer values expose `address`, `length`, `isNull`, `readCString`,
`slice`, and `free`.

### Platform support

| Platform | FFI core | Native ABI | Dynamic libraries | Native callbacks |
|---|---|---|---|---|
| Linux | supported | System V AMD64, AAPCS64 | `.so` | supported by backend ABI |
| Windows | supported | Win64, AAPCS64 | `.dll` | supported by backend ABI |
| macOS | supported | System V AMD64, AAPCS64 | `.dylib` | supported by backend ABI |
| Android arm64 | supported | AAPCS64 with Bionic | `.so` | backend-dependent |

### Library loading

The library path is passed directly to the host dynamic loader. Linux and
Android commonly use `.so` libraries, while macOS uses `.dylib` libraries.
Windows uses `.dll` libraries through the Windows loader. On Android arm64,
the native loader uses the Bionic ABI and `libc.so` as the system C library.
Common Linux SONAME spellings such as `libc.so.6` are normalized to their
Android equivalents when necessary. Lunex Android arm64 builds use the Android Go
target with `CGO_ENABLED=0`, position-independent executable mode, and the Bionic
loader. The FFI backend uses goffi through the purego-compatible layer without
requiring a C compiler.

```lx
val ffi = @import("std.ffi")

fn main() {
  val lib = ffi.load("libm.so.6", { global: false, lazy: false })
  val sqrt = lib.bind("sqrt", "f64(f64)")
  io.log(sqrt(81))
  lib.close()
}
```

`global` requests process-wide symbol visibility when the platform loader
supports it. `lazy` selects lazy symbol relocation where supported. The
portable default is eager, local loading.

### Signatures

Signatures use `returnType(argumentType, ...)` notation. A descriptor object is
also accepted:

```lx
val strlen = lib.bind("strlen", "usize(cstring)")
val add = lib.bind("add", { args: ["i32", "i32"], returns: "i32" })
```

Supported scalar types are `bool`, `i8`, `u8`, `i16`, `u16`, `i32`, `u32`,
`i64`, `u64`, `isize`, `usize`, `intptr`, `uintptr`, `f32`, `f64`, `float`,
`double`, and `string`.

Pointer types use `ptr` or C-style `T*`. `char*` is represented as `cstring`.
Common C spellings such as `size_t`, `ssize_t`, `ptrdiff_t`, `intptr_t`,
`uintptr_t`, `short`, `unsigned int`, `long`, `unsigned long`, and fixed-width
integer aliases are normalized to the corresponding ABI type. For integral
FFI arguments, decimal or base-prefixed strings can be used when the full
64-bit value cannot be represented exactly by a Lunex number.

Fixed-size native arrays use `T[N]`. Structs use `struct{field:type,...}`:

```lx
val pairType = "struct{x:i32, y:f64}"
val pairSize = ffi.sizeof(pairType)
val pairAlign = ffi.alignof(pairType)
```

`ffi.typeInfo()` exposes the resulting field layout and native size so a
binding can verify the declared ABI before the first call.

### Strings and memory

A `cstring` argument accepts a Lunex string, an FFI pointer, or null. Lunex
strings passed as `cstring` arguments are copied into temporary NUL-terminated
native storage for the duration of the call.

```lx
val text = ffi.cstring("Lunex")
val n = strlen(text)
io.log(n)
ffi.free(text)
```

`ffi.alloc()` returns zero-initialized memory with a known length. Pointer
bounds are checked when the pointer has a known allocation or buffer length;
addresses with unknown bounds can still be used for operations whose requested
size is explicit.

`std.buffer` values can be passed to `ffi.pointer(buffer)` and to
`ffi.writeBytes()` without first copying them into a separate Lunex array.

### Callbacks

Callbacks use the same signature syntax and keep their Lunex handler alive as
long as the returned pointer remains reachable:

```lx
val onValue = ffi.callback("i32(i32)", fn(value) {
  value * 2
})
```

The callback pointer can be passed directly to native APIs expecting a function
pointer. If a handler raises a Lunex error, the callback stores the last error
text and returns the zero value for its declared return type; the native call
itself cannot receive a Lunex exception object through a C ABI callback.

### Raw function pointers

A symbol can be resolved first and called later:

```lx
val address = ffi.symbol(lib, "strlen")
val length = ffi.call(address, "usize(cstring)", ["hello"])
```

`ffi.call(pointer, signature, args)` always requires an explicit signature.
This keeps the native ABI declaration at the call site instead of guessing it
from a raw address.

### Lifetime rules

Native allocations returned by `alloc`, `calloc`, and `cstring` are owned by
the returned pointer and should be released with `ffi.free(pointer)` or
`pointer.free()`.

A library remains open while bound functions reference it. Calling `library.close()`
marks the library for closing and the handle is actually released after the
last bound function is closed. `ffi.closeFunction()` releases that binding.

A pointer returned by `ffi.symbol()` or `library.handle()` is borrowed and must
not be passed to `ffi.free()`.

---

## `std.testing` — Lunex test support

```lx
val testing = @import("std.testing")
```

`std.testing` provides test registration, named groups, lifecycle hooks, parameterized cases, assertions, expected failures, retries, tags, assertion plans, deterministic snapshots, diagnostics, filtering, and structured results. The implementation is written in Lunex and uses standard library modules for host interaction.

### API

| Function | Returns | Description |
|---|---|---|
| `testing.test(name, body, options?)` | string | Register one test case |
| `testing.cases(name, values, body, options?)` | number | Register one test for every value in an array |
| `testing.group(name, body, options?)` | string | Register tests under a named group |
| `testing.beforeAll(body)` | boolean | Register group setup that runs once before selected tests |
| `testing.afterAll(body)` | boolean | Register group cleanup that runs after selected tests |
| `testing.beforeEach(body)` | boolean | Register setup that runs before each selected test |
| `testing.afterEach(body)` | boolean | Register cleanup that runs after each selected test |
| `testing.ok(value, message?)` | boolean | Require a truthy value |
| `testing.falsey(value, message?)` | boolean | Require a falsy value |
| `testing.null(value, message?)` | boolean | Require a null value |
| `testing.defined(value, message?)` | boolean | Require a value other than `undefined` |
| `testing.equal(actual, expected, message?)` | boolean | Require deep value equality |
| `testing.notEqual(actual, expected, message?)` | boolean | Require deep value inequality |
| `testing.same(actual, expected, message?)` | boolean | Require matching types and deep equality |
| `testing.contains(value, expected, message?)` | boolean | Require a string, array, or object to contain a value |
| `testing.type(value, expected, message?)` | boolean | Require an exact Lunex value type |
| `testing.length(value, expected, message?)` | boolean | Require an exact string or array length |
| `testing.empty(value, message?)` | boolean | Require an empty string, array, object, `null`, or `undefined` |
| `testing.approx(actual, expected, tolerance?, message?)` | boolean | Compare numbers with an absolute tolerance |
| `testing.inRange(value, minimum, maximum, message?)` | boolean | Require an inclusive numeric range |
| `testing.matchesPattern(value, pattern, message?)` | boolean | Require a string to match a regular expression |
| `testing.raises(body, expected?, message?)` | value | Require a function to raise and optionally match its error |
| `testing.notRaises(body, message?)` | boolean | Require a function not to raise |
| `testing.fail(message?)` | never | Fail the active test immediately |
| `testing.skip(reason?)` | never | Skip the active test without treating it as a failure |
| `testing.todo(reason?)` | never | Mark the active test as an expected failure |
| `testing.note(message)` | boolean | Attach diagnostic text to the active test |
| `testing.plan(count)` | boolean | Require an exact assertion count |
| `testing.snapshot(value, path, options?)` | boolean | Compare or update a deterministic snapshot file |
| `testing.list()` | array | Inspect registered tests without executing them |
| `testing.run(options?)` | object | Run selected tests and return structured results |
| `testing.clear()` | boolean | Reset all tests, groups, hooks, and runner state |

### Registration and groups

```lx
val testing = @import("std.testing")

testing.group("math", fn() {
  testing.test("addition", fn() {
    testing.equal(2 + 3, 5)
  })

  testing.test("division", fn() {
    testing.equal(12 / 3, 4)
  })
}, { tags: ["unit"] })

fn main() {
  val result = testing.run({ tags: ["unit"] })
  if !result.ok {
    throw result.failures
  }
}
```

Groups are scoped while their registration callback executes. Nested groups inherit parent tags and hook scopes. Test registration order remains deterministic.

`testing.group` accepts either the classic two-argument form or a third options argument. The options currently support `tags`.

`testing.test` options are `skip`, `todo`, `reason`, `tags`, `retries`, and `timeout`. `skip` and `todo` change the test status, `reason` supplies its explanation, `tags` adds selection metadata, `retries` bounds local retries, and `timeout` sets a post-execution duration budget.

### Lifecycle

```lx
val testing = @import("std.testing")

var opened = false

testing.beforeAll(fn() {
  opened = true
})

testing.beforeEach(fn() {
  testing.ok(opened)
})

testing.afterEach(fn() {
  testing.note("test cleanup completed")
})
testing.afterAll(fn() {
  opened = false
})

testing.test("uses shared setup", fn() {
  testing.ok(opened)
})
```

`beforeAll` and `afterAll` run once for every group that has selected tests. `beforeEach` runs from parent group to child group. `afterEach` runs from child group to parent group and reverses hook registration order inside each group. Cleanup hooks still run when setup or the test body fails.

A failing `beforeAll` blocks the affected group and its descendants. An `afterAll` failure is reported separately in `result.failures` and makes the overall run unsuccessful.

### Parameterized cases

```lx
val testing = @import("std.testing")

testing.cases("double", [1, 2, 3, 4], fn(value, index) {
  testing.note("case " + str(index))
  testing.equal(value * 2, (index + 1) * 2)
})
```

`testing.cases` creates independent tests named with a stable numeric suffix. Each callback receives the case value and zero-based case index.

### Assertions

`testing.equal` performs recursive equality for arrays and objects and normalizes object key order during comparison. `testing.same` additionally requires matching Lunex value types. `testing.approx` is intended for floating-point calculations where exact equality is too strict. `testing.inRange` uses inclusive bounds.

`testing.raises` accepts several forms of expectation. A string matches an error `code`, `name`, or a thrown primitive string. An object matches the supplied error fields deeply. A function receives the raised value and must return `true`.

```lx
testing.test("validation", fn() {
  testing.plan(5)
  testing.ok(8 > 2)
  testing.falsey(false)
  testing.null(null)
  testing.defined("ready")
  testing.approx(0.1 + 0.2, 0.3, 0.000001)
})

testing.test("invalid input", fn() {
  val err = testing.raises(fn() {
    throw { code: "E_INPUT", name: "InputError", message: "bad value" }
  }, { code: "E_INPUT", name: "InputError" })
  testing.equal(err.code, "E_INPUT")
  testing.matchesPattern(err.message, "bad")
})
```

Every public assertion increments the active test assertion count. `testing.plan(count)` detects missing or extra assertions before a test can pass. The plan itself does not count as an assertion.

Assertion failures raise an object containing `name`, `code`, `message`, `actual`, and `expected`. Assertion failures use `E_ASSERT`. Assertion-plan failures use `E_PLAN`.

### Skips and expected failures

Registration-time control keeps collection separate from execution:

```lx
testing.test("platform-only", fn() {
  testing.fail("not executed")
}, { skip: true, reason: "requires target platform" })

testing.test("future behavior", fn() {
  testing.fail("pending")
}, { todo: true, reason: "pending implementation" })
```

Runtime control is also available:

```lx
val supported = false

testing.test("conditional support", fn() {
  if !supported {
    testing.skip("feature is unavailable")
  }
  testing.ok(true)
})
```

A skipped test is never executed. A todo test that fails is counted as expected. A todo test that passes is an unexpected pass and fails the run.

### Retries and flakiness

```lx
testing.test("eventually stable", fn() {
  testing.equal(loadValue(), 42)
}, { retries: 2 })
```

Retries are explicit and bounded. A test that passes only after retry is reported as `flaky` instead of being silently treated like a clean first-attempt pass. Use `failOnFlaky: true` in `testing.run` when CI should reject that result.

The runner is deterministic by default: tests execute in registration order, no implicit randomization is performed, and retries are bounded by the declared count.

### Tags and selection

```lx
testing.group("database", fn() {
  testing.test("insert", fn() {
    testing.ok(true)
  })
}, { tags: ["integration", "slow"] })

testing.test("fast unit", fn() {
  testing.ok(true)
}, { tags: ["unit", "fast"] })

val result = testing.run({
  tags: ["unit"],
  excludeTags: ["slow"],
  filter: "fast"
})
```

`tags` requires every requested tag. `excludeTags` removes any matching tag. `filter` selects tests whose full group and test name contains the supplied text. Selection happens before test execution, so unselected tests do not run hooks.

### Snapshots

```lx
val data = { name: "Lunex", values: [1, 2, 3] }

testing.test("stable output", fn() {
  testing.snapshot(data, "snapshots/data.snap")
})
```

Snapshot serialization is deterministic for strings, numbers, booleans, `null`, arrays, and objects. Object keys are sorted before serialization. A missing or different snapshot fails the test.

To intentionally create or refresh a snapshot, use:

```lx
testing.snapshot(data, "snapshots/data.snap", { update: true })
```

Snapshot files are ordinary text files, so they can be reviewed and committed with the project. Snapshot updates are explicit and never happen during normal verification.

### Diagnostics and results

```lx
testing.test("parser", fn() {
  testing.note("checking nested expression")
  testing.equal(parseThing(), expected)
})

val result = testing.run({ retries: 1, failOnFlaky: true })

if !result.ok {
  each failure in result.failures {
    io.log(failure.test, failure.error)
  }
}
```

`testing.note` stores diagnostic text on the current test. It does not affect the result.

Each entry in `result.results` contains the test name, group, tags, status, elapsed time, attempt count, flakiness state, assertion count, assertion plan, notes, setup error, cleanup error, and captured error value.

The top-level result contains:

| Field | Description |
|---|---|
| `total` | Selected tests considered by the run |
| `passed` | Tests that passed without being todo cases |
| `failed` | Tests that failed or were rejected by `failOnFlaky` |
| `skipped` | Tests skipped before or during execution |
| `todo` | Expected-failure tests that did not unexpectedly pass |
| `unexpected` | Todo tests that passed unexpectedly |
| `flaky` | Tests that passed only after a retry |
| `duration` | Sum of executed test durations in milliseconds |
| `ok` | Overall success state |
| `stopped` | Whether `failFast` stopped further execution |
| `failures` | Structured failure entries |
| `results` | Per-test execution records |
| `afterAllError` | Final cleanup error, when any group cleanup failed |

### Runner options

| Option | Type | Description |
|---|---|---|
| `filter` | string | Select tests by full-name substring |
| `tags` | string or array | Require every requested tag |
| `excludeTags` | string or array | Exclude tests carrying any listed tag |
| `failFast` | boolean | Stop after the first unsuccessful result |
| `quiet` | boolean | Suppress per-test output and keep the summary silent |
| `failOnFlaky` | boolean | Convert a retry-passing flaky test into a failure |
| `retries` | number | Default retry count for tests without a local value |

The `timeout` test option is a duration budget. The runner measures the completed execution and reports `E_TIMEOUT` when the budget is exceeded. It does not forcibly interrupt code that is still executing.

### Resetting state

`testing.clear()` removes registered tests, groups, lifecycle hooks, and runner state. This makes repeated test runs in the same Lunex process isolated from previous registrations.

## `runtime` — Runtime introspection

```lx
val runtime = @import("runtime")
```

| Function                      | Returns | Description                                |
|-------------------------------|---------|--------------------------------------------|
| `runtime.version()`           | string  | Lunex version string                       |
| `runtime.globals()`           | array   | Names of all globally visible bindings     |
| `runtime.getGlobal(name)`     | value   | Read a global by name                      |
| `runtime.setGlobal(name, v)`  | —       | Write a global by name                     |
| `runtime.hasGlobal(name)`     | boolean | True if global exists                      |

There is no `runtime.typeOf()` or `runtime.gc()` in the `runtime` module.
To get the type name of a value, use the global `typeof(v)` keyword
described in `language-reference.md` — it's a language construct, not a
`runtime` module function. There is no way to force a garbage-collection
pass from Lunex code.
