# Lunex Examples

Compatible with Lunex v0.9.3.

## Running

```
lunex run <file.lx>
lunex pack <file.lx>
```

## Index

| File | Topic |
|------|-------|
| 01_hello_world.lx         | Hello World, each loop |
| 02_variables.lx           | val / var, destructuring, template strings |
| 03_structs.lx             | Factory functions, structs with methods |
| 04_control_flow.lx        | if/else, while, each, guard, unless |
| 05_stdlib.lx              | math, string methods, utils |
| 06_channels.lx            | spawn, channel, fan-out workers |
| 07_pattern_matching.lx    | match, if/else classification |
| 08_http_server.lx         | HTTP server (blocking — run separately) |
| 09_crypto.lx              | sha256, md5, hmac, base64, encrypt/decrypt |
| 10_database.lx            | std.db: table, insert, find, update, delete |
| 11_fs.lx                  | readFile, writeFile, appendFile, stat |
| 12_datetime.lx            | now, parse, format, diff, fromTimestamp |
| 13_arrays_strings.lx      | Array and string methods |
| 14_os.lx                  | platform, arch, exec, getenv/setenv |
| 15_spawn.lx               | Concurrent workers sending results over channels |
| 16_events.lx              | Event emitter pattern (on, once, emit, off) |
| 17_structs_composition.lx | Struct composition, polymorphism via factory fns |
| 18_loops.lx               | Summation, primes, fibonacci, nested loops |
| 19_higher_order.lx        | Closures, compose, memoize, map/filter/reduce |
| 20_io_display.lx          | io.log, io.warn, io.err, colors, io.table |
| 21_http_rest.lx           | Full REST server (blocking — run separately) |
| 22_state_machine.lx       | State machine pattern with transitions and actions |
| 23_pipeline.lx            | Chainable data pipeline builder pattern |
| 24_env_dotenv.lx           | std.env dotenv-compatible environment loading |
| 25_pack_validation.lx     | Pack validation before NAX emission |
| 26_watch.lx               | Reactive variable and member watching |

## Notes

- `struct { }` fields use newlines, not commas: `struct { x = 1` / `  y = 2 }`
- String methods are called directly on the value: `s.toUpperCase()`, `s.split(",")`, `s.length`
- Lunex has no `return` keyword — the last expression in a function is its result
- `match` supports literal values and `_` wildcard; use `if/else if` chains for ranges
- `crypto.hmac(algo, key, msg)` — e.g. `crypto.hmac("sha256", key, msg)`
- HTTP server: `http.createServer(handler)` then `server.listen(port, host, cb)`
- HTTP routing: `std.http.router` — `router.create([router.get("/users/:id", handler)])` then `api.listen(port)`
- Database: `db.table("name")` returns a table object with `.insert()`, `.find()`, `.findOne()`, etc.
