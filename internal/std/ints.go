package std

import (
	"lunex/internal/runtime"
)

const intsSource = `val U8_MAX = 255
val I8_MAX = 127
val I8_MIN = -128
val U16_MAX = 65535
val I16_MAX = 32767
val I16_MIN = -32768
val U32_MAX = 4294967295
val I32_MAX = 2147483647
val I32_MIN = -2147483648
val U64_MAX = 18446744073709551615
val I64_MAX = 9223372036854775807
val I64_MIN = -9223372036854775808

fn _i64(n) {
  n | 0
}

fn _signExtend(low, shift) {
  (low << shift) >> shift
}

fn u8(n) {
  _i64(n) & 255
}

fn i8(n) {
  _signExtend(_i64(n) & 255, 56)
}

fn u16(n) {
  _i64(n) & 65535
}

fn i16(n) {
  _signExtend(_i64(n) & 65535, 48)
}

fn u32(n) {
  _i64(n) & 4294967295
}

fn i32(n) {
  _signExtend(_i64(n) & 4294967295, 32)
}

fn u64(n) {
  _i64(n) >>> 0
}

fn i64(n) {
  _i64(n)
}

fn _words(n) {
  val u = _i64(n) >>> 0
  val words = [u & 65535, (u >>> 16) & 65535, (u >>> 32) & 65535, (u >>> 48) & 65535]
  words
}

fn _combine64(hi, lo) {
  (hi * 4294967296) + lo
}

fn _addRaw64(a, b) {
  val aw = _words(a)
  val bw = _words(b)

  val s0 = aw[0] + bw[0]
  val c0 = s0 >= 65536
  val r0 = c0 ? s0 - 65536 : s0
  var carry = c0 ? 1 : 0

  val s1 = aw[1] + bw[1] + carry
  val c1 = s1 >= 65536
  val r1 = c1 ? s1 - 65536 : s1
  carry = c1 ? 1 : 0

  val s2 = aw[2] + bw[2] + carry
  val c2 = s2 >= 65536
  val r2 = c2 ? s2 - 65536 : s2
  carry = c2 ? 1 : 0

  val r3 = (aw[3] + bw[3] + carry) & 65535

  _combine64(r2 + r3 * 65536, r0 + r1 * 65536)
}

fn _subRaw64(a, b) {
  val aw = _words(a)
  val bw = _words(b)

  val d0 = aw[0] - bw[0]
  val borrow0 = d0 < 0
  val r0 = borrow0 ? d0 + 65536 : d0
  var borrow = borrow0 ? 1 : 0

  val d1 = aw[1] - bw[1] - borrow
  val borrow1 = d1 < 0
  val r1 = borrow1 ? d1 + 65536 : d1
  borrow = borrow1 ? 1 : 0

  val d2 = aw[2] - bw[2] - borrow
  val borrow2 = d2 < 0
  val r2 = borrow2 ? d2 + 65536 : d2
  borrow = borrow2 ? 1 : 0

  val r3 = (aw[3] - bw[3] - borrow) & 65535

  _combine64(r2 + r3 * 65536, r0 + r1 * 65536)
}

fn _mulRaw64(a, b) {
  val aw = _words(a)
  val bw = _words(b)
  val acc = [0, 0, 0, 0]
  var i = 0
  while i < 4 {
    var j = 0
    while j < 4 {
      val k = i + j
      if k <= 3 {
        acc[k] = acc[k] + aw[i] * bw[j]
      }
      j = j + 1
    }
    i = i + 1
  }

  var carry = 0
  val rw = [0, 0, 0, 0]
  var k = 0
  while k < 4 {
    val t = acc[k] + carry
    rw[k] = t % 65536
    carry = (t - rw[k]) / 65536
    k = k + 1
  }

  _combine64(rw[2] + rw[3] * 65536, rw[0] + rw[1] * 65536)
}

fn addU8(a, b) { u8(a + b) }
fn addI8(a, b) { i8(a + b) }
fn addU16(a, b) { u16(a + b) }
fn addI16(a, b) { i16(a + b) }
fn addU32(a, b) { u32(a + b) }
fn addI32(a, b) { i32(a + b) }
fn addU64(a, b) { u64(_addRaw64(a, b)) }
fn addI64(a, b) { i64(_addRaw64(a, b)) }

fn subU8(a, b) { u8(a - b) }
fn subI8(a, b) { i8(a - b) }
fn subU16(a, b) { u16(a - b) }
fn subI16(a, b) { i16(a - b) }
fn subU32(a, b) { u32(a - b) }
fn subI32(a, b) { i32(a - b) }
fn subU64(a, b) { u64(_subRaw64(a, b)) }
fn subI64(a, b) { i64(_subRaw64(a, b)) }

fn mulU8(a, b) { u8(a * b) }
fn mulI8(a, b) { i8(a * b) }
fn mulU16(a, b) { u16(a * b) }
fn mulI16(a, b) { i16(a * b) }
fn mulU32(a, b) { u32(a * b) }
fn mulI32(a, b) { i32(a * b) }
fn mulU64(a, b) { u64(_mulRaw64(a, b)) }
fn mulI64(a, b) { i64(_mulRaw64(a, b)) }

fn shlU32(a, b) {
  (((_i64(a) >>> 0) & 4294967295) << ((_i64(b) >>> 0) & 31)) & 4294967295
}

fn shrU32(a, b) {
  ((_i64(a) >>> 0) & 4294967295) >>> ((_i64(b) >>> 0) & 31)
}

fn shlU64(a, b) {
  u64(_i64(a) << ((_i64(b) >>> 0) & 63))
}

fn shrU64(a, b) {
  _i64(a) >>> ((_i64(b) >>> 0) & 63)
}

fn rotlU32(a, b) {
  (((((_i64(a) >>> 0) & 4294967295) << ((_i64(b) >>> 0) & 31)) | (((_i64(a) >>> 0) & 4294967295) >>> ((32 - ((_i64(b) >>> 0) & 31)) & 31))) & 4294967295)
}

fn rotrU32(a, b) {
  (((((_i64(a) >>> 0) & 4294967295) >>> ((_i64(b) >>> 0) & 31)) | (((_i64(a) >>> 0) & 4294967295) << ((32 - ((_i64(b) >>> 0) & 31)) & 31))) & 4294967295)
}

fn isU8Overflow(n) {
  n < 0 || n > 255
}

fn isI32Overflow(n) {
  n < -2147483648 || n > 2147483647
}

val __module__ = {
  U8_MAX: U8_MAX,
  I8_MAX: I8_MAX,
  I8_MIN: I8_MIN,
  U16_MAX: U16_MAX,
  I16_MAX: I16_MAX,
  I16_MIN: I16_MIN,
  U32_MAX: U32_MAX,
  I32_MAX: I32_MAX,
  I32_MIN: I32_MIN,
  U64_MAX: U64_MAX,
  I64_MAX: I64_MAX,
  I64_MIN: I64_MIN,
  u8: u8,
  i8: i8,
  u16: u16,
  i16: i16,
  u32: u32,
  i32: i32,
  u64: u64,
  i64: i64,
  addU8: addU8,
  addI8: addI8,
  addU16: addU16,
  addI16: addI16,
  addU32: addU32,
  addI32: addI32,
  addU64: addU64,
  addI64: addI64,
  subU8: subU8,
  subI8: subI8,
  subU16: subU16,
  subI16: subI16,
  subU32: subU32,
  subI32: subI32,
  subU64: subU64,
  subI64: subI64,
  mulU8: mulU8,
  mulI8: mulI8,
  mulU16: mulU16,
  mulI16: mulI16,
  mulU32: mulU32,
  mulI32: mulI32,
  mulU64: mulU64,
  mulI64: mulI64,
  shlU32: shlU32,
  shrU32: shrU32,
  shlU64: shlU64,
  shrU64: shrU64,
  rotlU32: rotlU32,
  rotrU32: rotrU32,
  isU8Overflow: isU8Overflow,
  isI32Overflow: isI32Overflow
}
`

func IntsModule(interp interface {
	ExecAsModule(source, filename string) (*runtime.Value, error)
}) *runtime.Value {
	mod, err := interp.ExecAsModule(intsSource, "ints")
	if err != nil {
		panic(err)
	}
	return mod
}
