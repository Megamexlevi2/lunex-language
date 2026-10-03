package std

import (
	"lunex/internal/runtime"
)

const envSource = `val _os = @import("std.os")

fn _isWhitespace(ch) {
  ch.charCodeAt(0) == 32 || (ch.charCodeAt(0) >= 9 && ch.charCodeAt(0) <= 13) || ch.charCodeAt(0) == 160 || ch.charCodeAt(0) == 5760 || (ch.charCodeAt(0) >= 8192 && ch.charCodeAt(0) <= 8202) || ch.charCodeAt(0) == 8232 || ch.charCodeAt(0) == 8233 || ch.charCodeAt(0) == 8239 || ch.charCodeAt(0) == 8287 || ch.charCodeAt(0) == 12288 || ch.charCodeAt(0) == 65279
}

fn _isKeyChar(ch) {
  (ch.charCodeAt(0) >= 48 && ch.charCodeAt(0) <= 57) || (ch.charCodeAt(0) >= 65 && ch.charCodeAt(0) <= 90) || (ch.charCodeAt(0) >= 97 && ch.charCodeAt(0) <= 122) || ch.charCodeAt(0) == 45 || ch.charCodeAt(0) == 46 || ch.charCodeAt(0) == 95
}

fn _normalize(source) {
  source.replaceAll("\r\n", "\n").replaceAll("\r", "\n")
}

fn _validKey(key) {
  if key == "" {
    false
  } else {
    var i = 0
    var valid = true
    while i < key.length {
      if !_isKeyChar(key.charAt(i)) {
        valid = false
        break
      }
      i = i + 1
    }
    valid
  }
}

fn _findClosing(value, quote) {
  var i = 1
  var found = -1
  while i < value.length && found < 0 {
    val ch = value.charAt(i)
    if ch == quote && (i == 0 || value.charAt(i - 1) != "\\") {
      found = i
    }
    i = i + 1
  }
  found
}

fn _stripInlineComment(value) {
  var i = 0
  var found = -1
  while i < value.length && found < 0 {
    if value.charAt(i) == "#" {
      found = i
    }
    i = i + 1
  }
  if found < 0 {
    value.trimEnd()
  } else {
    value.slice(0, found).trimEnd()
  }
}

fn _lineEntry(line) {
  val clean = line.trimStart()
  if clean == "" || clean.charAt(0) == "#" {
    null
  } else {
    var text = clean
    if text.startsWith("export") && text.length > 6 && _isWhitespace(text.charAt(6)) {
      text = text.slice(7).trimStart()
    }

    var i = 0
    while i < text.length && _isKeyChar(text.charAt(i)) {
      i = i + 1
    }
    if i == 0 {
      null
    } else {
      val key = text.slice(0, i)
      val keyEnd = i
      var cursor = i
      while cursor < text.length && _isWhitespace(text.charAt(cursor)) {
        cursor = cursor + 1
      }
      var separator = ""
      if cursor < text.length && text.charAt(cursor) == "=" {
        separator = "="
      } elif cursor == keyEnd && cursor < text.length && text.charAt(cursor) == ":" && cursor + 1 < text.length && _isWhitespace(text.charAt(cursor + 1)) {
        separator = ":"
      }
      if separator == "" || !_validKey(key) {
        null
      } else {
        cursor = cursor + 1
        while cursor < text.length && _isWhitespace(text.charAt(cursor)) {
          cursor = cursor + 1
        }
        val raw = text.slice(cursor, text.length)
        val first = raw.charAt(0)
        if first == "'" || first == "\"" || first.charCodeAt(0) == 96 {
          val close = _findClosing(raw, first)
          if close >= 0 {
            val tail = raw.slice(close + 1, raw.length).trim()
            if tail == "" || tail.charAt(0) == "#" {
              var value = raw.slice(1, close)
              if first == "\"" {
                value = value.replaceAll("\\n", "\n").replaceAll("\\r", "\r")
              }
              { key: key, value: value, complete: true, quote: first }
            } else {
              null
            }
          } else {
            { key: key, value: raw, complete: false, quote: first }
          }
        } else {
          { key: key, value: _stripInlineComment(raw), complete: true, quote: "" }
        }
      }
    }
  }
}

fn _emptyObject() {
  val value = { __lunex_empty__: true }
  delete value.__lunex_empty__
  value
}

fn _parse(source) {
  val lines = _normalize(source).split("\n")
  val result = _emptyObject()
  var pending = false
  var pendingKey = ""
  var pendingQuote = ""
  var pendingValue = ""

  each line in lines {
    if pending {
      val candidate = pendingValue + "\n" + line
      val close = _findClosing(candidate, pendingQuote)
      if close >= 0 {
        var value = candidate.slice(1, close)
        if pendingQuote == "\"" {
          value = value.replaceAll("\\n", "\n").replaceAll("\\r", "\r")
        }
        result[pendingKey] = value
        pending = false
        pendingKey = ""
        pendingQuote = ""
        pendingValue = ""
      } else {
        pendingValue = candidate
      }
    } else {
      val entry = _lineEntry(line)
      if entry != null {
        if entry.complete {
          result[entry.key] = entry.value
        } else {
          pending = true
          pendingKey = entry.key
          pendingQuote = entry.quote
          pendingValue = entry.value
        }
      }
    }
  }

  result
}

fn _write(key, value) {
  try {
    _os.setenv(key.toString(), value.toString())
    true
  } catch (e) {
    false
  }
}

fn _populate(values, override = false) {
  val current = _os.environ()
  val populated = {}
  var success = true
  each key in values {
    val value = values[key]
    if override || typeof(current[key]) == "undefined" {
      if _write(key, value) {
        current[key] = value
        populated[key] = value
      } else {
        success = false
      }
    }
  }
  { ok: success, values: populated }
}

fn get(key, defaultValue = undefined) {
  if typeof(key) == "undefined" || key == null {
    defaultValue
  } else {
    val current = _os.environ()
    val value = current[key.toString()]
    if typeof(value) == "undefined" {
      defaultValue
    } else {
      value
    }
  }
}

fn has(key) {
  if typeof(key) == "undefined" || key == null {
    false
  } else {
    val current = _os.environ()
    typeof(current[key.toString()]) != "undefined"
  }
}

fn set(key, value) {
  if typeof(key) == "undefined" || key == null {
    false
  } else {
    _write(key, value)
  }
}

fn _delete(key) {
  if typeof(key) == "undefined" || key == null {
    false
  } else {
    try {
      _os.unsetenv(key.toString())
      true
    } catch (e) {
      false
    }
  }
}

fn all() {
  _os.environ()
}

fn parse(source) {
  _parse(source)
}

fn populate(values, override = false) {
  val result = _populate(values, override)
  result.values
}

fn _importFile(path) {
  val fs = @import("std.fs")
  fs.readFile(path)
}

fn load(path = ".env", override = false) {
  val loaded = _importFile(path)
  if loaded == null {
    false
  } else {
    val parsed = _parse(loaded)
    val result = _populate(parsed, override)
    result.ok
  }
}

fn _require(key) {
  if typeof(key) == "undefined" || key == null {
    throw { code: "E0110", message: "required environment variable is not set" }
  }
  val current = _os.environ()
  val value = current[key.toString()]
  if typeof(value) == "undefined" {
    throw { code: "E0110", name: key.toString(), message: "required environment variable '" + key.toString() + "' is not set" }
  }
  value
}

fn int(key, defaultValue = 0) {
  val value = get(key, undefined)
  if typeof(value) == "undefined" {
    defaultValue
  } else {
    val result = parseFloat(value)
    if typeof(result) == "undefined" || result != result {
      defaultValue
    } else {
      result
    }
  }
}

fn bool(key, defaultValue = false) {
  val value = get(key, undefined)
  if typeof(value) == "undefined" {
    defaultValue
  } else {
    val normalized = value.trim().toLowerCase()
    normalized == "true" || normalized == "1" || normalized == "yes" || normalized == "on"
  }
}

fn config(path = ".env", override = false) {
  val source = _importFile(path)
  if source == null {
    { parsed: _emptyObject(), error: { code: "E0111", message: "failed to load environment file '" + path + "'" } }
  } else {
    val parsed = _parse(source)
    val result = _populate(parsed, override)
    if result.ok {
      { parsed: parsed }
    } else {
      { parsed: parsed, error: { code: "E0111", message: "failed to apply environment file '" + path + "'" } }
    }
  }
}

val __module__ = {
  get: get,
  has: has,
  set: set,
  "delete": _delete,
  all: all,
  parse: parse,
  populate: populate,
  load: load,
  "require": _require,
  int: int,
  bool: bool,
  config: config
}

`

func EnvModule(interp interface {
	ExecAsModule(source, filename string) (*runtime.Value, error)
}) *runtime.Value {
	mod, err := interp.ExecAsModule(envSource, "env")
	if err != nil {
		panic(err)
	}
	return mod
}
