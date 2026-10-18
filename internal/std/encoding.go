package std

import "lunex/internal/runtime"

const encodingSource = `val _buffer = @import("std.buffer")
val _math = @import("std.math")
val _hex = "0123456789ABCDEF"
val _b64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
val _b64url = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
val _b32 = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

fn _fail(message) {
  throw { code: "E0140", message: message }
}

fn _asText(value, name) {
  if typeof(value) == "undefined" || value == null {
    _fail(name + " expects a non-null value")
  }
  value.toString()
}

fn _isInteger(value) {
  typeof(value) == "number" && value == _math.floor(value)
}

fn _validateBytes(bytes) {
  var i = 0
  while i < bytes.length {
    val value = bytes[i]
    if !_isInteger(value) || value < 0 || value > 255 {
      _fail("byte at index " + i.toString() + " must be an integer from 0 to 255")
    }
    i = i + 1
  }
  bytes
}

fn _bytes(input) {
  if input == null || typeof(input) == "undefined" {
    _fail("expected a string, byte array, or buffer")
  } elif typeof(input) == "string" {
    _utf8Encode(input)
  } elif typeof(input.toArray) == "function" {
    _validateBytes(input.toArray())
  } elif typeof(input.length) == "number" {
    _validateBytes(input)
  } else {
    _fail("expected a string, byte array, or buffer")
  }
}

fn _utf8Byte(out, cp) {
  if cp <= 127 {
    out.push(cp)
  } elif cp <= 2047 {
    out.push(192 + _math.floor(cp / 64))
    out.push(128 + (cp % 64))
  } elif cp <= 65535 {
    out.push(224 + _math.floor(cp / 4096))
    out.push(128 + (_math.floor(cp / 64) % 64))
    out.push(128 + (cp % 64))
  } else {
    out.push(240 + _math.floor(cp / 262144))
    out.push(128 + (_math.floor(cp / 4096) % 64))
    out.push(128 + (_math.floor(cp / 64) % 64))
    out.push(128 + (cp % 64))
  }
}

fn _utf8Encode(text) {
  if typeof(text) == "undefined" || text == null {
    _fail("utf8Encode expects a non-null value")
  }
  val source = text.toString()
  val out = []
  var i = 0
  while i < source.length {
    _utf8Byte(out, source.charCodeAt(i))
    i = i + 1
  }
  out
}

fn _continuation(value) {
  value >= 128 && value <= 191
}

fn _utf8Decode(input, fatal = false) {
  val bytes = _bytes(input)
  val out = []
  var i = 0
  while i < bytes.length {
    val first = bytes[i]
    var cp = 0
    var width = 0
    var valid = true
    if first <= 127 {
      cp = first
      width = 1
    } elif first >= 194 && first <= 223 {
      if i + 1 < bytes.length && _continuation(bytes[i + 1]) {
        cp = (first - 192) * 64 + (bytes[i + 1] - 128)
        width = 2
      } else {
        valid = false
      }
    } elif first >= 224 && first <= 239 {
      if i + 2 < bytes.length && _continuation(bytes[i + 1]) && _continuation(bytes[i + 2]) {
        cp = (first - 224) * 4096 + (bytes[i + 1] - 128) * 64 + (bytes[i + 2] - 128)
        if cp < 2048 || (cp >= 55296 && cp <= 57343) {
          valid = false
        } else {
          width = 3
        }
      } else {
        valid = false
      }
    } elif first >= 240 && first <= 244 {
      if i + 3 < bytes.length && _continuation(bytes[i + 1]) && _continuation(bytes[i + 2]) && _continuation(bytes[i + 3]) {
        cp = (first - 240) * 262144 + (bytes[i + 1] - 128) * 4096 + (bytes[i + 2] - 128) * 64 + (bytes[i + 3] - 128)
        if cp < 65536 || cp > 1114111 {
          valid = false
        } else {
          width = 4
        }
      } else {
        valid = false
      }
    } else {
      valid = false
    }
    if valid {
      _utf8Byte(out, cp)
      i = i + width
    } else {
      if fatal {
        _fail("invalid UTF-8 sequence at byte " + i.toString())
      }
      _utf8Byte(out, 65533)
      i = i + 1
    }
  }
  _buffer.from(out).toString()
}

fn _order(value) {
  if typeof(value) == "undefined" || value == null {
    _fail("endianness must be 'le', 'be', or 'auto'")
  }
  val normalized = value.toString().toLowerCase()
  if normalized == "le" || normalized == "little" || normalized == "little-endian" {
    "le"
  } elif normalized == "be" || normalized == "big" || normalized == "big-endian" {
    "be"
  } elif normalized == "auto" {
    "auto"
  } else {
    _fail("endianness must be 'le', 'be', or 'auto'")
  }
}

fn _push16(out, unit, endian) {
  if endian == "be" {
    out.push(_math.floor(unit / 256))
    out.push(unit % 256)
  } else {
    out.push(unit % 256)
    out.push(_math.floor(unit / 256))
  }
}

fn _push32(out, cp, endian) {
  val b0 = _math.floor(cp / 16777216) % 256
  val b1 = _math.floor(cp / 65536) % 256
  val b2 = _math.floor(cp / 256) % 256
  val b3 = cp % 256
  if endian == "be" {
    out.push(b0)
    out.push(b1)
    out.push(b2)
    out.push(b3)
  } else {
    out.push(b3)
    out.push(b2)
    out.push(b1)
    out.push(b0)
  }
}

fn utf16Encode(text, endian = "le", bom = false) {
  val order = _order(endian)
  if order == "auto" {
    _fail("utf16Encode does not accept auto endianness")
  }
  val source = _asText(text, "utf16Encode")
  val out = []
  if bom {
    if order == "be" {
      out.push(254)
      out.push(255)
    } else {
      out.push(255)
      out.push(254)
    }
  }
  var i = 0
  while i < source.length {
    val cp = source.charCodeAt(i)
    if cp <= 65535 {
      _push16(out, cp, order)
    } else {
      val adjusted = cp - 65536
      _push16(out, 55296 + _math.floor(adjusted / 1024), order)
      _push16(out, 56320 + (adjusted % 1024), order)
    }
    i = i + 1
  }
  out
}

fn utf16Decode(input, endian = "auto", fatal = false) {
  val bytes = _bytes(input)
  if bytes.length % 2 != 0 {
    _fail("UTF-16 input length must be divisible by 2")
  }
  var order = _order(endian)
  var start = 0
  if order == "auto" {
    if bytes.length >= 2 && bytes[0] == 255 && bytes[1] == 254 {
      order = "le"
      start = 2
    } elif bytes.length >= 2 && bytes[0] == 254 && bytes[1] == 255 {
      order = "be"
      start = 2
    } else {
      order = "le"
    }
  } elif bytes.length >= 2 && ((order == "le" && bytes[0] == 255 && bytes[1] == 254) || (order == "be" && bytes[0] == 254 && bytes[1] == 255)) {
    start = 2
  }
  val out = []
  var i = start
  while i < bytes.length {
    var unit = 0
    if order == "be" {
      unit = bytes[i] * 256 + bytes[i + 1]
    } else {
      unit = bytes[i + 1] * 256 + bytes[i]
    }
    i = i + 2
    if unit >= 55296 && unit <= 56319 {
      if i + 1 < bytes.length {
        var low = 0
        if order == "be" {
          low = bytes[i] * 256 + bytes[i + 1]
        } else {
          low = bytes[i + 1] * 256 + bytes[i]
        }
        if low >= 56320 && low <= 57343 {
          _utf8Byte(out, 65536 + (unit - 55296) * 1024 + (low - 56320))
          i = i + 2
        } elif fatal {
          _fail("unpaired UTF-16 high surrogate at byte " + (i - 2).toString())
        } else {
          _utf8Byte(out, 65533)
        }
      } elif fatal {
        _fail("unpaired UTF-16 high surrogate at byte " + (i - 2).toString())
      } else {
        _utf8Byte(out, 65533)
      }
    } elif unit >= 56320 && unit <= 57343 {
      if fatal {
        _fail("unpaired UTF-16 low surrogate at byte " + (i - 2).toString())
      }
      _utf8Byte(out, 65533)
    } else {
      _utf8Byte(out, unit)
    }
  }
  _buffer.from(out).toString()
}

fn utf32Encode(text, endian = "le", bom = false) {
  val order = _order(endian)
  if order == "auto" {
    _fail("utf32Encode does not accept auto endianness")
  }
  val source = _asText(text, "utf32Encode")
  val out = []
  if bom {
    if order == "be" {
      out.push(0)
      out.push(0)
      out.push(254)
      out.push(255)
    } else {
      out.push(255)
      out.push(254)
      out.push(0)
      out.push(0)
    }
  }
  var i = 0
  while i < source.length {
    _push32(out, source.charCodeAt(i), order)
    i = i + 1
  }
  out
}

fn utf32Decode(input, endian = "auto", fatal = false) {
  val bytes = _bytes(input)
  if bytes.length % 4 != 0 {
    _fail("UTF-32 input length must be divisible by 4")
  }
  var order = _order(endian)
  var start = 0
  if order == "auto" {
    if bytes.length >= 4 && bytes[0] == 255 && bytes[1] == 254 && bytes[2] == 0 && bytes[3] == 0 {
      order = "le"
      start = 4
    } elif bytes.length >= 4 && bytes[0] == 0 && bytes[1] == 0 && bytes[2] == 254 && bytes[3] == 255 {
      order = "be"
      start = 4
    } else {
      order = "le"
    }
  } elif bytes.length >= 4 && ((order == "le" && bytes[0] == 255 && bytes[1] == 254 && bytes[2] == 0 && bytes[3] == 0) || (order == "be" && bytes[0] == 0 && bytes[1] == 0 && bytes[2] == 254 && bytes[3] == 255)) {
    start = 4
  }
  val out = []
  var i = start
  while i < bytes.length {
    var cp = 0
    if order == "be" {
      cp = bytes[i] * 16777216 + bytes[i + 1] * 65536 + bytes[i + 2] * 256 + bytes[i + 3]
    } else {
      cp = bytes[i + 3] * 16777216 + bytes[i + 2] * 65536 + bytes[i + 1] * 256 + bytes[i]
    }
    if cp > 1114111 || (cp >= 55296 && cp <= 57343) {
      if fatal {
        _fail("invalid UTF-32 code point at byte " + i.toString())
      }
      _utf8Byte(out, 65533)
    } else {
      _utf8Byte(out, cp)
    }
    i = i + 4
  }
  _buffer.from(out).toString()
}

fn latin1Encode(text) {
  if typeof(text) == "undefined" || text == null {
    _fail("latin1Encode expects a non-null value")
  }
  val source = text.toString()
  val out = []
  var i = 0
  while i < source.length {
    val cp = source.charCodeAt(i)
    if cp > 255 {
      _fail("character at index " + i.toString() + " is not representable in Latin-1")
    }
    out.push(cp)
    i = i + 1
  }
  out
}

fn latin1Decode(input) {
  val bytes = _bytes(input)
  val out = []
  var i = 0
  while i < bytes.length {
    _utf8Byte(out, bytes[i])
    i = i + 1
  }
  _buffer.from(out).toString()
}

fn _b64Value(ch, alphabet) {
  alphabet.indexOf(ch)
}

fn base64Encode(input, urlSafe = false, padding = true) {
  val bytes = _bytes(input)
  val alphabet = if urlSafe { _b64url } else { _b64 }
  var out = ""
  var i = 0
  while i < bytes.length {
    val remaining = bytes.length - i
    val b0 = bytes[i]
    val b1 = if remaining > 1 { bytes[i + 1] } else { 0 }
    val b2 = if remaining > 2 { bytes[i + 2] } else { 0 }
    out = out + alphabet.charAt(_math.floor(b0 / 4))
    out = out + alphabet.charAt((b0 % 4) * 16 + _math.floor(b1 / 16))
    if remaining > 1 {
      out = out + alphabet.charAt((b1 % 16) * 4 + _math.floor(b2 / 64))
    } elif padding {
      out = out + "="
    }
    if remaining > 2 {
      out = out + alphabet.charAt(b2 % 64)
    } elif padding {
      out = out + "="
    }
    i = i + 3
  }
  out
}

fn base64Decode(input, urlSafe = false) {
  if typeof(input) == "undefined" || input == null {
    _fail("base64Decode expects a string")
  }
  val source = input.toString()
  var clean = ""
  var paddingCount = 0
  var sawPadding = false
  var i = 0
  while i < source.length {
    val ch = source.charAt(i)
    if ch == " " || ch == "\t" || ch == "\r" || ch == "\n" {
      i = i + 1
    } elif ch == "=" {
      sawPadding = true
      paddingCount = paddingCount + 1
      i = i + 1
    } else {
      if sawPadding {
        _fail("invalid Base64 padding")
      }
      var normalized = ch
      if urlSafe {
        if ch == "-" { normalized = "+" } elif ch == "_" { normalized = "/" } elif ch == "+" || ch == "/" {
          _fail("standard Base64 character in URL-safe input")
        }
      } elif ch == "-" || ch == "_" {
        _fail("URL-safe Base64 character in standard input")
      }
      if _b64Value(normalized, _b64) < 0 {
        _fail("invalid Base64 character at index " + i.toString())
      }
      clean = clean + normalized
      i = i + 1
    }
  }
  if paddingCount > 2 || clean.length % 4 == 1 || (paddingCount == 1 && clean.length % 4 != 3) || (paddingCount == 2 && clean.length % 4 != 2) {
    _fail("invalid Base64 length or padding")
  }
  val out = []
  var accumulator = 0
  var bits = 0
  i = 0
  while i < clean.length {
    accumulator = accumulator * 64 + _b64Value(clean.charAt(i), _b64)
    bits = bits + 6
    if bits >= 8 {
      bits = bits - 8
      out.push(_math.floor(accumulator / _math.pow(2, bits)) % 256)
      accumulator = accumulator % _math.pow(2, bits)
    }
    i = i + 1
  }
  if bits > 0 && accumulator != 0 {
    _fail("non-zero trailing bits in Base64 input")
  }
  out
}

fn base64UrlEncode(input, padding = false) {
  base64Encode(input, true, padding)
}

fn base64UrlDecode(input) {
  base64Decode(input, true)
}

fn base64UrlDecodeText(input, fatal = false) {
  _utf8Decode(base64UrlDecode(input), fatal)
}

fn base32Encode(input, padding = true) {
  val bytes = _bytes(input)
  var out = ""
  var accumulator = 0
  var bits = 0
  var i = 0
  while i < bytes.length {
    var bitPos = 7
    while bitPos >= 0 {
      accumulator = accumulator * 2 + (_math.floor(bytes[i] / _math.pow(2, bitPos)) % 2)
      bits = bits + 1
      if bits == 5 {
        out = out + _b32.charAt(accumulator)
        accumulator = 0
        bits = 0
      }
      bitPos = bitPos - 1
    }
    i = i + 1
  }
  if bits > 0 {
    out = out + _b32.charAt(accumulator * _math.pow(2, 5 - bits))
  }
  if padding {
    while out.length % 8 != 0 {
      out = out + "="
    }
  }
  out
}

fn base32Decode(input) {
  if typeof(input) == "undefined" || input == null {
    _fail("base32Decode expects a string")
  }
  val source = input.toString().toUpperCase()
  var clean = ""
  var sawPadding = false
  var padCount = 0
  var i = 0
  while i < source.length {
    val ch = source.charAt(i)
    if ch == " " || ch == "\t" || ch == "\r" || ch == "\n" {
      i = i + 1
    } elif ch == "=" {
      sawPadding = true
      padCount = padCount + 1
      i = i + 1
    } else {
      if sawPadding || _b32.indexOf(ch) < 0 {
        _fail("invalid Base32 character or padding at index " + i.toString())
      }
      clean = clean + ch
      i = i + 1
    }
  }
  val remainder = clean.length % 8
  if remainder == 1 || remainder == 3 || remainder == 6 {
    _fail("invalid Base32 length")
  }
  if padCount > 0 {
    var expectedPadding = 0
    if remainder == 2 {
      expectedPadding = 6
    } elif remainder == 4 {
      expectedPadding = 4
    } elif remainder == 5 {
      expectedPadding = 3
    } elif remainder == 7 {
      expectedPadding = 1
    }
    if expectedPadding == 0 || padCount != expectedPadding || (clean.length + padCount) % 8 != 0 {
      _fail("invalid Base32 padding")
    }
  }
  val out = []
  var accumulator = 0
  var bits = 0
  i = 0
  while i < clean.length {
    accumulator = accumulator * 32 + _b32.indexOf(clean.charAt(i))
    bits = bits + 5
    if bits >= 8 {
      bits = bits - 8
      out.push(_math.floor(accumulator / _math.pow(2, bits)) % 256)
      accumulator = accumulator % _math.pow(2, bits)
    }
    i = i + 1
  }
  if bits > 0 && accumulator != 0 {
    _fail("non-zero trailing bits in Base32 input")
  }
  out
}

fn _hexValue(ch) {
  val upper = ch.toUpperCase()
  _hex.indexOf(upper)
}

fn _hexByte(value) {
  _hex.charAt(_math.floor(value / 16)) + _hex.charAt(value % 16)
}

fn percentEncode(text) {
  if typeof(text) == "undefined" || text == null {
    _fail("percentEncode expects a non-null value")
  }
  val source = text.toString()
  var out = ""
  var i = 0
  while i < source.length {
    val cp = source.charCodeAt(i)
    if cp < 128 && ((cp >= 48 && cp <= 57) || (cp >= 65 && cp <= 90) || (cp >= 97 && cp <= 122) || cp == 45 || cp == 46 || cp == 95 || cp == 126) {
      out = out + source.charAt(i)
    } else {
      val bytes = []
      _utf8Byte(bytes, cp)
      var j = 0
      while j < bytes.length {
        out = out + "%" + _hexByte(bytes[j])
        j = j + 1
      }
    }
    i = i + 1
  }
  out
}

fn percentDecode(input, plusAsSpace = false) {
  if typeof(input) == "undefined" || input == null {
    _fail("percentDecode expects a string")
  }
  val source = input.toString()
  val out = []
  var raw = ""
  var i = 0
  while i < source.length {
    val ch = source.charAt(i)
    if ch == "%" {
      if i + 2 >= source.length {
        _fail("incomplete percent escape at index " + i.toString())
      }
      val high = _hexValue(source.charAt(i + 1))
      val low = _hexValue(source.charAt(i + 2))
      if high < 0 || low < 0 {
        _fail("invalid percent escape at index " + i.toString())
      }
      if raw.length > 0 {
        val rawBytes = _utf8Encode(raw)
        var j = 0
        while j < rawBytes.length {
          out.push(rawBytes[j])
          j = j + 1
        }
        raw = ""
      }
      out.push(high * 16 + low)
      i = i + 3
    } elif ch == "+" && plusAsSpace {
      if raw.length > 0 {
        val rawBytes = _utf8Encode(raw)
        var j = 0
        while j < rawBytes.length {
          out.push(rawBytes[j])
          j = j + 1
        }
        raw = ""
      }
      out.push(32)
      i = i + 1
    } else {
      raw = raw + ch
      i = i + 1
    }
  }
  if raw.length > 0 {
    val rawBytes = _utf8Encode(raw)
    var j = 0
    while j < rawBytes.length {
      out.push(rawBytes[j])
      j = j + 1
    }
  }
  _utf8Decode(out, true)
}

fn formEncode(text) {
  percentEncode(text).replaceAll("%20", "+")
}

fn formDecode(text) {
  percentDecode(text, true)
}

fn encode(text, format = "utf8") {
  val kind = _asText(format, "encode format").toLowerCase().replaceAll("_", "-")
  if kind == "utf8" || kind == "utf-8" {
    utf8Encode(text)
  } elif kind == "utf16" || kind == "utf-16" || kind == "utf16le" || kind == "utf-16le" {
    utf16Encode(text, "le")
  } elif kind == "utf16be" || kind == "utf-16be" {
    utf16Encode(text, "be")
  } elif kind == "utf32" || kind == "utf-32" || kind == "utf32le" || kind == "utf-32le" {
    utf32Encode(text, "le")
  } elif kind == "utf32be" || kind == "utf-32be" {
    utf32Encode(text, "be")
  } elif kind == "latin1" || kind == "latin-1" || kind == "iso-8859-1" {
    latin1Encode(text)
  } elif kind == "base64" {
    base64Encode(text)
  } elif kind == "base64url" || kind == "base64-url" {
    base64Encode(text, true, false)
  } elif kind == "base32" {
    base32Encode(text)
  } elif kind == "url" || kind == "percent" || kind == "percent-encoding" {
    percentEncode(text)
  } elif kind == "form" || kind == "url-form" {
    formEncode(text)
  } else {
    _fail("unsupported encoding '" + format.toString() + "'")
  }
}

fn decode(input, format = "utf8", fatal = false) {
  val kind = _asText(format, "decode format").toLowerCase().replaceAll("_", "-")
  if kind == "utf8" || kind == "utf-8" {
    _utf8Decode(input, fatal)
  } elif kind == "utf16" || kind == "utf-16" || kind == "utf16le" || kind == "utf-16le" {
    utf16Decode(input, "le", fatal)
  } elif kind == "utf16be" || kind == "utf-16be" {
    utf16Decode(input, "be", fatal)
  } elif kind == "utf32" || kind == "utf-32" || kind == "utf32le" || kind == "utf-32le" {
    utf32Decode(input, "le", fatal)
  } elif kind == "utf32be" || kind == "utf-32be" {
    utf32Decode(input, "be", fatal)
  } elif kind == "latin1" || kind == "latin-1" || kind == "iso-8859-1" {
    latin1Decode(input)
  } elif kind == "base64" {
    _utf8Decode(base64Decode(input), fatal)
  } elif kind == "base64url" || kind == "base64-url" {
    _utf8Decode(base64Decode(input, true), fatal)
  } elif kind == "base32" {
    _utf8Decode(base32Decode(input), fatal)
  } elif kind == "url" || kind == "percent" || kind == "percent-encoding" {
    percentDecode(input)
  } elif kind == "form" || kind == "url-form" {
    formDecode(input)
  } else {
    _fail("unsupported encoding '" + format.toString() + "'")
  }
}

fn base64DecodeText(input, fatal = false) {
  _utf8Decode(base64Decode(input), fatal)
}

fn base32DecodeText(input, fatal = false) {
  _utf8Decode(base32Decode(input), fatal)
}

val __module__ = {
  utf8Encode: _utf8Encode,
  utf8Decode: _utf8Decode,
  utf16Encode: utf16Encode,
  utf16Decode: utf16Decode,
  utf32Encode: utf32Encode,
  utf32Decode: utf32Decode,
  latin1Encode: latin1Encode,
  latin1Decode: latin1Decode,
  base64Encode: base64Encode,
  base64Decode: base64Decode,
  base64DecodeText: base64DecodeText,
  base64UrlEncode: base64UrlEncode,
  base64UrlDecode: base64UrlDecode,
  base64UrlDecodeText: base64UrlDecodeText,
  base32Encode: base32Encode,
  base32Decode: base32Decode,
  base32DecodeText: base32DecodeText,
  percentEncode: percentEncode,
  percentDecode: percentDecode,
  formEncode: formEncode,
  formDecode: formDecode,
  encode: encode,
  decode: decode
}
`

func EncodingModule(interp interface {
	ExecAsModule(source, filename string) (*runtime.Value, error)
}) *runtime.Value {
	mod, err := interp.ExecAsModule(encodingSource, "encoding")
	if err != nil {
		panic(err)
	}
	return mod
}
