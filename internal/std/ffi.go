package std

import "lunex/internal/runtime"

const ffiSource = `val _native = @import("internal.native").ffi

fn load(path, options = {}) {
  _native.load(path, options)
}

fn open(path, options = {}) {
  load(path, options)
}

fn bind(library, symbolName, signature) {
  _native.bind(library, symbolName, signature)
}

fn symbol(library, symbolName) {
  _native.symbol(library, symbolName)
}

fn call(target, signature = undefined, args = undefined) {
  if typeof(target) == "function" {
    if typeof(signature) == "array" {
      _native.call(target, signature)
    } elif typeof(signature) == "undefined" && typeof(args) == "array" {
      _native.call(target, args)
    } else {
      _native.call(target, [])
    }
  } elif typeof(args) == "undefined" {
    _native.call(target, signature, [])
  } else {
    _native.call(target, signature, args)
  }
}

fn callback(signature, handler) {
  _native.callback(signature, handler)
}

fn pointer(value = null) {
  _native.pointer(value)
}

fn nullPtr() {
  _native.null()
}

fn isNull(value) {
  _native.isNull(value)
}

fn alloc(size) {
  _native.alloc(size)
}

fn calloc(count, size) {
  _native.calloc(count, size)
}

fn realloc(pointerValue, size) {
  _native.realloc(pointerValue, size)
}

fn free(pointerValue) {
  _native.free(pointerValue)
}

fn cstring(value) {
  _native.cstring(value)
}

fn read(pointerValue, typeName, offset = 0) {
  _native.read(pointerValue, typeName, offset)
}

fn write(pointerValue, typeName, value, offset = 0) {
  _native.write(pointerValue, typeName, value, offset)
}

fn readBytes(pointerValue, length, offset = 0) {
  _native.readBytes(pointerValue, length, offset)
}

fn writeBytes(pointerValue, data, offset = 0) {
  _native.writeBytes(pointerValue, data, offset)
}

fn readCString(pointerValue, maxLength = 1048576) {
  _native.readCString(pointerValue, maxLength)
}

fn copy(destination, source, length) {
  _native.copy(destination, source, length)
}

fn fill(pointerValue, value, length) {
  _native.fill(pointerValue, value, length)
}

fn close(resource) {
  _native.close(resource)
}

fn closeFunction(functionValue) {
  _native.closeFunction(functionValue)
}

fn name(functionValue) {
  _native.name(functionValue)
}

fn signature(functionValue) {
  _native.signature(functionValue)
}

fn sizeof(typeName) {
  _native.sizeof(typeName)
}

fn alignof(typeName) {
  _native.alignof(typeName)
}

fn typeInfo(typeName) {
  _native.typeInfo(typeName)
}

fn enabled() {
  _native.enabled()
}

val __module__ = {
  load: load,
  open: open,
  bind: bind,
  symbol: symbol,
  call: call,
  callback: callback,
  pointer: pointer,
  "null": _native.null,
  nullPtr: nullPtr,
  isNull: isNull,
  alloc: alloc,
  calloc: calloc,
  realloc: realloc,
  free: free,
  cstring: cstring,
  read: read,
  write: write,
  readBytes: readBytes,
  writeBytes: writeBytes,
  readCString: readCString,
  copy: copy,
  fill: fill,
  close: close,
  closeFunction: closeFunction,
  name: name,
  signature: signature,
  sizeof: sizeof,
  alignof: alignof,
  typeInfo: typeInfo,
  enabled: enabled
}
`

func FFIModule(interp interface {
	ExecAsModule(source, filename string) (*runtime.Value, error)
}) *runtime.Value {
	mod, err := interp.ExecAsModule(ffiSource, "ffi")
	if err != nil {
		panic(err)
	}
	return mod
}
