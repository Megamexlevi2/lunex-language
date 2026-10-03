package std

import (
	"lunex/internal/runtime"
)

const jsonSource = `
val _fs = @import("std.fs")
val _buffer = @import("std.buffer")

fn _isDigit(code) {
  code >= 48 && code <= 57
}

fn _isHexDigit(code) {
  (code >= 48 && code <= 57) || (code >= 65 && code <= 70) || (code >= 97 && code <= 102)
}

fn _hexValue(code) {
  if code >= 48 && code <= 57 {
    code - 48
  } elif code >= 65 && code <= 70 {
    code - 65 + 10
  } else {
    code - 97 + 10
  }
}

fn _isSpace(code) {
  code == 32 || code == 9 || code == 10 || code == 13
}

fn _isIdentChar(code) {
  (code >= 97 && code <= 122) || (code >= 65 && code <= 90) || (code >= 48 && code <= 57) || code == 95 || code == 36
}

fn _encodeCodePoint(cp) {
  var bytes = []
  if cp <= 127 {
    bytes.push(cp)
  } elif cp <= 2047 {
    bytes.push(192 | (cp >> 6))
    bytes.push(128 | (cp & 63))
  } elif cp <= 65535 {
    bytes.push(224 | (cp >> 12))
    bytes.push(128 | ((cp >> 6) & 63))
    bytes.push(128 | (cp & 63))
  } else {
    bytes.push(240 | (cp >> 18))
    bytes.push(128 | ((cp >> 12) & 63))
    bytes.push(128 | ((cp >> 6) & 63))
    bytes.push(128 | (cp & 63))
  }
  _buffer.from(bytes).toString()
}

fn _lineAndColumn(text, pos) {
  var line = 1
  var col = 1
  var i = 0
  val limit = if pos < text.length { pos } else { text.length }
  while i < limit {
    if text.charAt(i) == "\n" {
      line = line + 1
      col = 1
    } else {
      col = col + 1
    }
    i = i + 1
  }
  { line: line, column: col }
}

fn _parseError(message, text, pos) {
  val where = _lineAndColumn(text, pos)
  throw {
    code: "E0113",
    name: "SyntaxError",
    message: message + " (line " + where.line.toString() + ", column " + where.column.toString() + ")",
    line: where.line,
    column: where.column,
    position: pos
  }
}

fn _makeState(text) {
  { text: text, length: text.length, pos: 0 }
}

fn _peek(state) {
  if state.pos >= state.length {
    -1
  } else {
    state.text.charCodeAt(state.pos)
  }
}

fn _peekAt(state, offset) {
  val i = state.pos + offset
  if i >= state.length || i < 0 {
    -1
  } else {
    state.text.charCodeAt(i)
  }
}

fn _advance(state) {
  { text: state.text, length: state.length, pos: state.pos + 1 }
}

fn _advanceBy(state, n) {
  { text: state.text, length: state.length, pos: state.pos + n }
}

fn _skipSpace(state) {
  var s = state
  while s.pos < s.length && _isSpace(_peek(s)) {
    s = _advance(s)
  }
  s
}

fn _scanLiteral(state, word, value) {
  var i = 0
  while i < word.length {
    if _peekAt(state, i) != word.charCodeAt(i) {
      _parseError("invalid JSON literal", state.text, state.pos)
    }
    i = i + 1
  }
  val s = _advanceBy(state, word.length)
  if _isIdentChar(_peek(s)) {
    _parseError("invalid JSON literal", state.text, state.pos)
  }
  { value: value, state: s }
}

fn _isHighSurrogate(code) {
  code >= 55296 && code <= 56319
}

fn _isLowSurrogate(code) {
  code >= 56320 && code <= 57343
}

fn _combineSurrogates(high, low) {
  65536 + (high - 55296) * 1024 + (low - 56320)
}

fn _readHex4(state, startPos) {
  var s = state
  var value = 0
  var i = 0
  while i < 4 {
    val code = _peek(s)
    if code < 0 || !_isHexDigit(code) {
      _parseError("invalid \\u escape", s.text, startPos)
    }
    value = value * 16 + _hexValue(code)
    s = _advance(s)
    i = i + 1
  }
  { value: value, state: s }
}

fn _scanEscape(state) {
  val startPos = state.pos - 1
  if _peek(state) < 0 {
    _parseError("unterminated string escape", state.text, startPos)
  }
  val ch = state.text.charAt(state.pos)
  if ch == "\"" {
    { text: "\"", state: _advance(state) }
  } elif ch == "\\" {
    { text: "\\", state: _advance(state) }
  } elif ch == "/" {
    { text: "/", state: _advance(state) }
  } elif ch == "n" {
    { text: "\n", state: _advance(state) }
  } elif ch == "t" {
    { text: "\t", state: _advance(state) }
  } elif ch == "r" {
    { text: "\r", state: _advance(state) }
  } elif ch == "b" {
    { text: "\b", state: _advance(state) }
  } elif ch == "f" {
    { text: "\f", state: _advance(state) }
  } elif ch == "u" {
    val afterU = _advance(state)
    val first = _readHex4(afterU, startPos)
    var s = first.state
    if _isHighSurrogate(first.value) {
      if _peek(s) == 92 && _peekAt(s, 1) == 117 {
        val secondStart = _advanceBy(s, 2)
        val second = _readHex4(secondStart, startPos)
        if _isLowSurrogate(second.value) {
          { text: _encodeCodePoint(_combineSurrogates(first.value, second.value)), state: second.state }
        } else {
          _parseError("unpaired UTF-16 surrogate in \\u escape", state.text, startPos)
        }
      } else {
        _parseError("unpaired UTF-16 surrogate in \\u escape", state.text, startPos)
      }
    } elif _isLowSurrogate(first.value) {
      _parseError("unpaired UTF-16 surrogate in \\u escape", state.text, startPos)
    } else {
      { text: _encodeCodePoint(first.value), state: s }
    }
  } else {
    _parseError("invalid escape sequence '\\" + ch + "'", state.text, startPos)
  }
}

fn _scanString(state) {
  val startPos = state.pos
  var s = _advance(state)
  var parts = []
  var closed = false
  while !closed {
    if s.pos >= s.length {
      _parseError("unterminated string", state.text, startPos)
    }
    val code = _peek(s)
    if code == 34 {
      s = _advance(s)
      closed = true
    } elif code == 92 {
      val escaped = _scanEscape(_advance(s))
      parts.push(escaped.text)
      s = escaped.state
    } elif code >= 0 && code < 32 {
      _parseError("control character in string literal", state.text, s.pos)
    } else {
      parts.push(s.text.charAt(s.pos))
      s = _advance(s)
    }
  }
  { value: parts.join(""), state: s }
}

fn _scanNumber(state) {
  val startPos = state.pos
  var s = state
  if _peek(s) == 45 {
    s = _advance(s)
  }
  if _peek(s) == 48 {
    s = _advance(s)
    if _isDigit(_peek(s)) {
      _parseError("leading zeros are not allowed in a JSON number", state.text, startPos)
    }
  } elif _isDigit(_peek(s)) {
    while _isDigit(_peek(s)) {
      s = _advance(s)
    }
  } else {
    _parseError("invalid number literal", state.text, startPos)
  }
  if _peek(s) == 46 {
    s = _advance(s)
    if !_isDigit(_peek(s)) {
      _parseError("invalid number literal", state.text, startPos)
    }
    while _isDigit(_peek(s)) {
      s = _advance(s)
    }
  }
  if _peek(s) == 101 || _peek(s) == 69 {
    s = _advance(s)
    if _peek(s) == 43 || _peek(s) == 45 {
      s = _advance(s)
    }
    if !_isDigit(_peek(s)) {
      _parseError("invalid number literal", state.text, startPos)
    }
    while _isDigit(_peek(s)) {
      s = _advance(s)
    }
  }
  val raw = state.text.slice(startPos, s.pos)
  val num = Number(raw)
  if isNaN(num) {
    throw {
      code: "E0113",
      name: "RangeError",
      message: "number is outside the range a JSON value can represent: " + raw
    }
  }
  { value: num, state: s }
}

val _maxDepth = 512

fn _scanValue(state, depth) {
  if depth > _maxDepth {
    throw {
      code: "E0116",
      name: "RangeError",
      message: "JSON is nested too deeply (max depth " + _maxDepth.toString() + ")"
    }
  }
  val s = _skipSpace(state)
  val code = _peek(s)
  if code < 0 {
    _parseError("unexpected end of JSON input", state.text, s.pos)
  } elif code == 34 {
    _scanString(s)
  } elif code == 123 {
    _scanObject(s, depth)
  } elif code == 91 {
    _scanArray(s, depth)
  } elif code == 116 {
    _scanLiteral(s, "true", true)
  } elif code == 102 {
    _scanLiteral(s, "false", false)
  } elif code == 110 {
    _scanLiteral(s, "null", null)
  } elif code == 45 || _isDigit(code) {
    _scanNumber(s)
  } else {
    _parseError("unexpected character in JSON", state.text, s.pos)
  }
}

fn _scanArray(state, depth) {
  var s = _skipSpace(_advance(state))
  if _peek(s) == 93 {
    { value: [], state: _advance(s) }
  } else {
    var items = []
    var done = false
    while !done {
      val item = _scanValue(s, depth + 1)
      items.push(item.value)
      s = _skipSpace(item.state)
      val code = _peek(s)
      if code == 44 {
        s = _skipSpace(_advance(s))
        if _peek(s) == 93 {
          _parseError("trailing comma in array", state.text, s.pos)
        }
      } elif code == 93 {
        s = _advance(s)
        done = true
      } else {
        _parseError("expected ',' or ']' in array", state.text, s.pos)
      }
    }
    { value: items, state: s }
  }
}

fn _scanObject(state, depth) {
  var s = _skipSpace(_advance(state))
  if _peek(s) == 125 {
    { value: {}, state: _advance(s) }
  } else {
    var obj = {}
    var done = false
    while !done {
      s = _skipSpace(s)
      if _peek(s) != 34 {
        _parseError("expected string key in object", state.text, s.pos)
      }
      val key = _scanString(s)
      s = _skipSpace(key.state)
      if _peek(s) != 58 {
        _parseError("expected ':' after object key", state.text, s.pos)
      }
      s = _skipSpace(_advance(s))
      val item = _scanValue(s, depth + 1)
      obj[key.value] = item.value
      s = _skipSpace(item.state)
      val code = _peek(s)
      if code == 44 {
        s = _skipSpace(_advance(s))
        if _peek(s) == 125 {
          _parseError("trailing comma in object", state.text, s.pos)
        }
      } elif code == 125 {
        s = _advance(s)
        done = true
      } else {
        _parseError("expected ',' or '}' in object", state.text, s.pos)
      }
    }
    { value: obj, state: s }
  }
}

fn _parse(text) {
  val s = _makeState(text)
  val result = _scanValue(s, 0)
  val trailing = _skipSpace(result.state)
  if trailing.pos < trailing.length {
    _parseError("unexpected trailing character after JSON value", text, trailing.pos)
  }
  result.value
}

fn _stripLeadingZeros(digits) {
  var i = 0
  while i < digits.length - 1 && digits.charAt(i) == "0" {
    i = i + 1
  }
  digits.slice(i, digits.length)
}

fn _normalizeExponent(text) {
  val eIndex = text.indexOf("e")
  if eIndex < 0 {
    text
  } else {
    val mantissa = text.slice(0, eIndex)
    var rest = text.slice(eIndex + 1, text.length)
    var sign = "+"
    if rest.charAt(0) == "+" || rest.charAt(0) == "-" {
      sign = rest.charAt(0)
      rest = rest.slice(1, rest.length)
    }
    val digits = _stripLeadingZeros(rest)
    if sign == "+" {
      mantissa + "e" + digits
    } else {
      mantissa + "e-" + digits
    }
  }
}

fn _formatNumber(n) {
  if isNaN(n) || n == Infinity || n == -Infinity {
    
    
    
    
    "null"
  } else {
    _normalizeExponent(n.toString())
  }
}

fn _quoteString(s) {
  var out = "\""
  var i = 0
  while i < s.length {
    val ch = s.charAt(i)
    val code = s.charCodeAt(i)
    if ch == "\"" {
      out = out + "\\\""
    } elif ch == "\\" {
      out = out + "\\\\"
    } elif code == 10 {
      out = out + "\\n"
    } elif code == 13 {
      out = out + "\\r"
    } elif code == 9 {
      out = out + "\\t"
    } elif code == 8 {
      out = out + "\\b"
    } elif code == 12 {
      out = out + "\\f"
    } elif code < 32 {
      val hex = code.toString(16)
      val padded = if hex.length == 1 { "000" + hex } elif hex.length == 2 { "00" + hex } elif hex.length == 3 { "0" + hex } else { hex }
      out = out + "\\u" + padded
    } else {
      out = out + ch
    }
    i = i + 1
  }
  out + "\""
}

fn _stackHas(stack, value) {
  var found = false
  var i = 0
  while i < stack.length && !found {
    if stack[i] == value {
      found = true
    }
    i = i + 1
  }
  found
}

fn _keyAllowed(keyFilter, key) {
  if keyFilter == null {
    true
  } else {
    keyFilter.includes(key)
  }
}

fn _applyToJSON(value, key) {
  if typeof(value) == "object" && typeof(value.toJSON) == "function" {
    value.toJSON(key)
  } else {
    value
  }
}

fn _applyReplacer(replacer, holder, key, value) {
  val afterToJSON = _applyToJSON(value, key)
  if typeof(replacer) == "function" {
    replacer(key, afterToJSON, holder)
  } else {
    afterToJSON
  }
}

fn _serialize(value, indent, depth, stack, replacer, keyFilter) {
  if depth > _maxDepth {
    throw {
      code: "E0116",
      name: "RangeError",
      message: "value is nested too deeply to serialize (max depth " + _maxDepth.toString() + ")"
    }
  }
  if value == null {
    "null"
  } else {
    val kind = typeof(value)
    if kind == "undefined" || kind == "function" {
      null
    } elif kind == "boolean" {
      if value { "true" } else { "false" }
    } elif kind == "number" {
      _formatNumber(value)
    } elif kind == "string" {
      _quoteString(value)
    } elif kind == "array" {
      _serializeArray(value, indent, depth, stack, replacer, keyFilter)
    } elif kind == "object" {
      _serializeObject(value, indent, depth, stack, replacer, keyFilter)
    } else {
      null
    }
  }
}

fn _joinItems(items, indent, depth) {
  if indent == "" {
    items.join(",")
  } else {
    val pad = indent.repeat(depth + 1)
    val closePad = indent.repeat(depth)
    "\n" + pad + items.join(",\n" + pad) + "\n" + closePad
  }
}

fn _serializeArray(arr, indent, depth, stack, replacer, keyFilter) {
  if arr.length == 0 {
    "[]"
  } else {
    if _stackHas(stack, arr) {
      throw {
        code: "E0114",
        name: "TypeError",
        message: "cannot serialize a circular structure to JSON"
      }
    }
    val nextStack = stack.concat([arr])
    var items = []
    var i = 0
    while i < arr.length {
      val item = _applyReplacer(replacer, arr, i.toString(), arr[i])
      val piece = _serialize(item, indent, depth + 1, nextStack, replacer, keyFilter)
      if piece == null {
        items.push("null")
      } else {
        items.push(piece)
      }
      i = i + 1
    }
    "[" + _joinItems(items, indent, depth) + "]"
  }
}

fn _objectKeys(obj) {
  
  
  
  
  
  
  
  val keys = Object.keys(obj)
  keys.sort()
  keys
}

fn _serializeObject(obj, indent, depth, stack, replacer, keyFilter) {
  val allKeys = _objectKeys(obj)
  var keys = []
  var i = 0
  while i < allKeys.length {
    if _keyAllowed(keyFilter, allKeys[i]) {
      keys.push(allKeys[i])
    }
    i = i + 1
  }
  if keys.length == 0 {
    "{}"
  } else {
    if _stackHas(stack, obj) {
      throw {
        code: "E0114",
        name: "TypeError",
        message: "cannot serialize a circular structure to JSON"
      }
    }
    val nextStack = stack.concat([obj])
    var items = []
    var j = 0
    while j < keys.length {
      val key = keys[j]
      val rawValue = _applyReplacer(replacer, obj, key, obj[key])
      val piece = _serialize(rawValue, indent, depth + 1, nextStack, replacer, keyFilter)
      if piece != null {
        items.push(_quoteString(key) + ":" + (if indent == "" { "" } else { " " }) + piece)
      }
      j = j + 1
    }
    if items.length == 0 {
      "{}"
    } else {
      "{" + _joinItems(items, indent, depth) + "}"
    }
  }
}

fn _normalizeIndent(indentArg) {
  if typeof(indentArg) == "string" {
    indentArg.slice(0, 10)
  } elif typeof(indentArg) == "number" {
    val n = if indentArg < 0 { 0 } elif indentArg > 10 { 10 } else { indentArg }
    " ".repeat(n)
  } else {
    ""
  }
}

fn _resolveReplacer(replacerArg) {
  if typeof(replacerArg) == "function" {
    { fn: replacerArg, keys: null }
  } elif typeof(replacerArg) == "array" {
    { fn: null, keys: replacerArg }
  } else {
    { fn: null, keys: null }
  }
}

fn _stringify(value, replacerArg, indentArg) {
  val indent = _normalizeIndent(indentArg)
  val resolved = _resolveReplacer(replacerArg)
  val root = _applyReplacer(resolved.fn, { "": value }, "", value)
  val out = _serialize(root, indent, 0, [], resolved.fn, resolved.keys)
  if out == null { "null" } else { out }
}

fn _walk(holder, key, reviver) {
  val value = holder[key]
  if typeof(value) == "array" {
    var i = 0
    while i < value.length {
      val revived = _walk(value, i.toString(), reviver)
      if typeof(revived) == "undefined" {
        value[i] = null
      } else {
        value[i] = revived
      }
      i = i + 1
    }
  } elif typeof(value) == "object" {
    val keys = Object.keys(value)
    var i = 0
    while i < keys.length {
      val k = keys[i]
      val revived = _walk(value, k, reviver)
      if typeof(revived) == "undefined" {
        delete value[k]
      } else {
        value[k] = revived
      }
      i = i + 1
    }
  }
  reviver(key, value, holder)
}

fn _applyReviver(value, reviver) {
  if typeof(reviver) == "function" {
    _walk({ "": value }, "", reviver)
  } else {
    value
  }
}

fn parse(text, reviver = undefined) {
  val value = _parse(text.toString())
  _applyReviver(value, reviver)
}

fn stringify(value, replacer = undefined, indent = 2) {
  _stringify(value, replacer, indent)
}

fn pretty(value, indent = 2) {
  _stringify(value, undefined, indent)
}

fn compact(value) {
  _stringify(value, undefined, undefined)
}

fn isValid(text) {
  try {
    _parse(text.toString())
    true
  } catch err {
    if typeof(err) == "object" && err.code == "E0113" {
      false
    } else {
      throw err
    }
  }
}

fn readFile(path, reviver = undefined) {
  val content = _fs.readFile(path)
  if content == null {
    throw {
      code: "E0115",
      name: "IOError",
      message: "could not read JSON file '" + path.toString() + "' (not found, unreadable, or not valid UTF-8)"
    }
  }
  parse(content, reviver)
}

fn writeFile(path, value, indent = 2) {
  val text = _stringify(value, undefined, indent)
  val ok = _fs.writeFile(path, text)
  if !ok {
    throw {
      code: "E0115",
      name: "IOError",
      message: "could not write JSON file '" + path.toString() + "'"
    }
  }
  true
}

fn save(path, value, indent = 2) {
  writeFile(path, value, indent)
}

{
  parse: parse,
  stringify: stringify,
  pretty: pretty,
  compact: compact,
  isValid: isValid,
  readFile: readFile,
  writeFile: writeFile,
  save: save
}
`

func JsonModule(interp interface {
	ExecAsModule(source, filename string) (*runtime.Value, error)
}) *runtime.Value {
	mod, err := interp.ExecAsModule(jsonSource, "json")
	if err != nil {
		panic(err)
	}
	return mod
}
