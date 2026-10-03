package std

import (
	"fmt"
	"math"
	"reflect"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"lunex/internal/errfmt"
	lunexruntime "lunex/internal/runtime"
)

type ffiType struct {
	token   string
	goType  reflect.Type
	kind    string
	pointee *ffiType
	fields  []ffiField
	elem    *ffiType
	count   int
}

type ffiField struct {
	name        string
	reflectName string
	typ         ffiType
}

type ffiSignature struct {
	args    []ffiType
	returns ffiType
	text    string
}

type ffiLibrary struct {
	mu          sync.Mutex
	handle      uintptr
	path        string
	active      int
	closeQueued bool
	closed      bool
}

type ffiFunction struct {
	library *ffiLibrary
	name    string
	sig     ffiSignature
	fn      reflect.Value
	mu      sync.Mutex
	closed  bool
}

type ffiPointer struct {
	mu       sync.RWMutex
	address  uintptr
	length   int
	bounded  bool
	owned    bool
	freed    bool
	buffer   *lunexBuffer
	callback *ffiCallback
}

type ffiCallback struct {
	mu      sync.Mutex
	address uintptr
	sig     ffiSignature
	handler *lunexruntime.Value
	lastErr string
	closed  bool
	tramp   reflect.Value
}

type ffiPointerMarker struct {
	ptr *ffiPointer
}

func (m ffiPointerMarker) Error() string { return "lunex ffi pointer" }

type ffiLibraryMarker struct {
	lib *ffiLibrary
}

func (m ffiLibraryMarker) Error() string { return "lunex ffi library" }

var ffiFunctionRegistry sync.Map

var ffiScalarTypes = map[string]reflect.Type{
	"bool":    reflect.TypeOf(bool(false)),
	"i8":      reflect.TypeOf(int8(0)),
	"u8":      reflect.TypeOf(uint8(0)),
	"i16":     reflect.TypeOf(int16(0)),
	"u16":     reflect.TypeOf(uint16(0)),
	"i32":     reflect.TypeOf(int32(0)),
	"u32":     reflect.TypeOf(uint32(0)),
	"i64":     reflect.TypeOf(int64(0)),
	"u64":     reflect.TypeOf(uint64(0)),
	"isize":   reflect.TypeOf(int(0)),
	"usize":   reflect.TypeOf(uintptr(0)),
	"intptr":  reflect.TypeOf(int(0)),
	"uintptr": reflect.TypeOf(uintptr(0)),
	"f32":     reflect.TypeOf(float32(0)),
	"f64":     reflect.TypeOf(float64(0)),
	"float":   reflect.TypeOf(float64(0)),
	"double":  reflect.TypeOf(float64(0)),
}

func ffiError(code, message string) error {
	kind := errfmt.KindRuntime
	switch code {
	case "E0120":
		kind = errfmt.KindPermission
	case "E0121", "E0122", "E0123", "E0124":
		kind = errfmt.KindImport
	case "E0125", "E0126", "E0127", "E0131":
		kind = errfmt.KindType
	case "E0128", "E0130", "E0132":
		kind = errfmt.KindRuntime
	case "E0129":
		kind = errfmt.KindRuntime
	}
	return &errfmt.LunexError{Message: message, Kind: kind, Code: code}
}

func ffiDisabledError() error {
	e := ffiError("E0120", "native FFI is disabled for this Lunex process").(*errfmt.LunexError)
	e.Suggestion = "start Lunex with `lunex ffi = on run <file>` to allow native library access"
	e.Notes = []string{
		"FFI is off by default",
		"only the Lunex command line can enable it for the current process",
		"environment variables and source code cannot enable or disable FFI",
	}
	return e
}

func ensureFFIEnabled() error {
	if !IsFFIEnabled() {
		return ffiDisabledError()
	}
	return nil
}

func ffiNativeModule(interp *lunexruntime.Interpreter) *lunexruntime.Value {
	return lunexruntime.ObjectVal(map[string]*lunexruntime.Value{
		"enabled": nativeFunc("enabled", func(_ []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			return lunexruntime.BoolVal(IsFFIEnabled()), nil
		}),
		"load": nativeFunc("load", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) == 0 || args[0] == nil || args[0].Tag != lunexruntime.TypeString || strings.TrimSpace(args[0].StrVal) == "" {
				return nil, ffiError("E0121", "FFI library path must be a non-empty string")
			}
			global := false
			lazy := false
			if len(args) > 1 && args[1] != nil && args[1].Tag == lunexruntime.TypeObject {
				global = ffiBoolOption(args[1], "global")
				lazy = ffiBoolOption(args[1], "lazy")
			}
			h, err := ffiOpenLibrary(args[0].StrVal, global, lazy)
			if err != nil {
				e := ffiError("E0122", fmt.Sprintf("failed to load native library %q: %v", args[0].StrVal, err)).(*errfmt.LunexError)
				e.Suggestion = "verify that the library exists, matches the process architecture, and can be loaded by the operating system"
				return nil, e
			}
			return ffiLibraryValue(&ffiLibrary{handle: h, path: args[0].StrVal}), nil
		}),
		"pointer": nativeFunc("pointer", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) == 0 || args[0] == nil || args[0].IsNullish() {
				return ffiPointerValue(&ffiPointer{}), nil
			}
			return ffiPointerFromValue(args[0])
		}),
		"null": nativeFunc("null", func(_ []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			return ffiPointerValue(&ffiPointer{}), nil
		}),
		"isNull": nativeFunc("isNull", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if len(args) == 0 || args[0] == nil || args[0].IsNullish() {
				return lunexruntime.True, nil
			}
			p, err := ffiPointerFromValue(args[0])
			if err != nil {
				return lunexruntime.False, nil
			}
			fp, ok := ffiPointerOf(p)
			return lunexruntime.BoolVal(ok && fp.addressAt(0) == 0), nil
		}),
		"alloc": nativeFunc("alloc", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			size, err := ffiUintptrArg(args, 0, "size")
			if err != nil {
				return nil, err
			}
			addr, err := ffiMalloc(size)
			if err != nil {
				return nil, ffiBackendError("E0130", "native allocation failed", err)
			}
			if addr == 0 && size != 0 {
				return nil, ffiError("E0130", "native allocation returned a null pointer")
			}
			if err := ffiZero(addr, size); err != nil {
				_ = ffiFree(addr)
				return nil, ffiBackendError("E0130", "native allocation could not be initialized", err)
			}
			return ffiPointerValue(&ffiPointer{address: addr, length: ffiSafeInt(size), bounded: true, owned: true}), nil
		}),
		"calloc": nativeFunc("calloc", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			count, err := ffiUintptrArg(args, 0, "count")
			if err != nil {
				return nil, err
			}
			size, err := ffiUintptrArg(args, 1, "size")
			if err != nil {
				return nil, err
			}
			if count != 0 && size > ^uintptr(0)/count {
				return nil, ffiError("E0130", "native allocation size overflow")
			}
			addr, err := ffiCalloc(count, size)
			if err != nil {
				return nil, ffiBackendError("E0130", "native allocation failed", err)
			}
			total := count * size
			if addr == 0 && total != 0 {
				return nil, ffiError("E0130", "native allocation returned a null pointer")
			}
			return ffiPointerValue(&ffiPointer{address: addr, length: ffiSafeInt(total), bounded: true, owned: true}), nil
		}),
		"realloc": nativeFunc("realloc", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) < 2 {
				return nil, ffiError("E0125", "realloc requires a pointer and a size")
			}
			p, err := ffiPointerFromValue(args[0])
			if err != nil {
				return nil, err
			}
			fp, ok := ffiPointerOf(p)
			if !ok || !fp.owned {
				return nil, ffiError("E0125", "realloc requires an owned FFI allocation")
			}
			size, err := ffiUintptrArg(args, 1, "size")
			if err != nil {
				return nil, err
			}
			fp.mu.RLock()
			oldAddr := fp.address
			fp.mu.RUnlock()
			addr, err := ffiRealloc(oldAddr, size)
			if err != nil {
				return nil, ffiBackendError("E0130", "native reallocation failed", err)
			}
			if addr == 0 && size != 0 {
				return nil, ffiError("E0130", "native reallocation returned a null pointer")
			}
			fp.mu.Lock()
			fp.address = addr
			fp.length = ffiSafeInt(size)
			fp.freed = addr == 0 && size == 0
			fp.mu.Unlock()
			return p, nil
		}),
		"free": nativeFunc("free", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) == 0 || args[0] == nil || args[0].IsNullish() {
				return lunexruntime.False, nil
			}
			p, err := ffiPointerFromValue(args[0])
			if err != nil {
				return nil, err
			}
			fp, ok := ffiPointerOf(p)
			if !ok {
				return nil, ffiError("E0125", "free requires a valid FFI pointer")
			}
			return ffiFreePointer(fp)
		}),
		"cstring": nativeFunc("cstring", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) == 0 || args[0] == nil || args[0].IsNullish() {
				return ffiPointerValue(&ffiPointer{}), nil
			}
			if args[0].Tag != lunexruntime.TypeString {
				return ffiPointerFromValue(args[0])
			}
			data := append([]byte(args[0].StrVal), 0)
			addr, err := ffiMalloc(uintptr(len(data)))
			if err != nil {
				return nil, ffiBackendError("E0130", "native string allocation failed", err)
			}
			if addr == 0 && len(data) != 0 {
				return nil, ffiError("E0130", "native string allocation returned a null pointer")
			}
			if err := copyBytesToNative(addr, data); err != nil {
				_ = ffiFree(addr)
				return nil, err
			}
			return ffiPointerValue(&ffiPointer{address: addr, length: len(data), bounded: true, owned: true}), nil
		}),
		"bind": nativeFunc("bind", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) < 3 {
				return nil, ffiError("E0125", "bind requires a library, symbol name, and signature")
			}
			lib, ok := ffiLibraryOf(args[0])
			if !ok {
				return nil, ffiError("E0131", "bind requires an FFI library returned by load()")
			}
			name := strings.TrimSpace(args[1].ToString())
			if name == "" {
				return nil, ffiError("E0125", "native symbol name cannot be empty")
			}
			sig, err := parseFFISignature(args[2])
			if err != nil {
				return nil, err
			}
			return ffiBind(lib, name, sig)
		}),
		"symbol": nativeFunc("symbol", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) < 2 {
				return nil, ffiError("E0125", "symbol requires a library and symbol name")
			}
			lib, ok := ffiLibraryOf(args[0])
			if !ok {
				return nil, ffiError("E0131", "symbol requires an FFI library returned by load()")
			}
			return ffiLookup(lib, strings.TrimSpace(args[1].ToString()))
		}),
		"call": nativeFunc("call", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			return ffiCallTarget(interp, args)
		}),
		"callback": nativeFunc("callback", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) < 2 {
				return nil, ffiError("E0125", "callback requires a signature and a handler")
			}
			return ffiCreateCallback(interp, args[0], args[1])
		}),
		"close": nativeFunc("close", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if len(args) == 0 {
				return lunexruntime.False, nil
			}
			if lib, ok := ffiLibraryOf(args[0]); ok {
				return ffiCloseLibraryValue(lib)
			}
			if fn, ok := ffiFunctionOf(args[0]); ok {
				return ffiCloseFunctionValue(args[0], fn)
			}
			if p, ok := ffiPointerOf(args[0]); ok && p.callback != nil {
				return ffiCloseCallback(p)
			}
			return lunexruntime.False, nil
		}),
		"closeFunction": nativeFunc("closeFunction", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if len(args) == 0 {
				return lunexruntime.False, nil
			}
			fn, ok := ffiFunctionOf(args[0])
			if !ok {
				return lunexruntime.False, nil
			}
			return ffiCloseFunctionValue(args[0], fn)
		}),
		"name": nativeFunc("name", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if len(args) == 0 {
				return lunexruntime.Undefined, nil
			}
			if fn, ok := ffiFunctionOf(args[0]); ok {
				return lunexruntime.StringVal(fn.name), nil
			}
			return lunexruntime.Undefined, nil
		}),
		"signature": nativeFunc("signature", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if len(args) == 0 {
				return lunexruntime.Undefined, nil
			}
			if fn, ok := ffiFunctionOf(args[0]); ok {
				return lunexruntime.StringVal(fn.sig.text), nil
			}
			return lunexruntime.Undefined, nil
		}),
		"read": nativeFunc("read", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) < 2 {
				return nil, ffiError("E0125", "read requires a pointer and a type")
			}
			typ, err := parseFFIType(args[1].ToString(), true)
			if err != nil {
				return nil, err
			}
			off, err := ffiOptionalOffset(args, 2)
			if err != nil {
				return nil, err
			}
			return ffiReadValue(args[0], typ, off)
		}),
		"write": nativeFunc("write", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) < 3 {
				return nil, ffiError("E0125", "write requires a pointer, a type, and a value")
			}
			typ, err := parseFFIType(args[1].ToString(), false)
			if err != nil {
				return nil, err
			}
			off, err := ffiOptionalOffset(args, 3)
			if err != nil {
				return nil, err
			}
			if err := ffiWriteValue(args[0], typ, args[2], off); err != nil {
				return nil, err
			}
			return lunexruntime.Undefined, nil
		}),
		"readBytes": nativeFunc("readBytes", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) < 2 {
				return nil, ffiError("E0125", "readBytes requires a pointer and a length")
			}
			length, err := ffiIntArg(args, 1, "length")
			if err != nil || length < 0 {
				if err != nil {
					return nil, err
				}
				return nil, ffiError("E0131", "length must be non-negative")
			}
			off, err := ffiOptionalOffset(args, 2)
			if err != nil {
				return nil, err
			}
			data, err := ffiReadBytes(args[0], length, off)
			if err != nil {
				return nil, err
			}
			return ffiByteArrayValue(data), nil
		}),
		"writeBytes": nativeFunc("writeBytes", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) < 2 {
				return nil, ffiError("E0125", "writeBytes requires a pointer and byte data")
			}
			data, err := ffiBytesFromValue(args[1])
			if err != nil {
				return nil, err
			}
			off, err := ffiOptionalOffset(args, 2)
			if err != nil {
				return nil, err
			}
			if err := ffiWriteBytes(args[0], data, off); err != nil {
				return nil, err
			}
			return lunexruntime.Undefined, nil
		}),
		"readCString": nativeFunc("readCString", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) == 0 {
				return lunexruntime.StringVal(""), nil
			}
			maxLen := 1 << 20
			if len(args) > 1 {
				value, err := ffiIntArg(args, 1, "maxLength")
				if err != nil {
					return nil, err
				}
				if value < 0 {
					return nil, ffiError("E0131", "maxLength must be non-negative")
				}
				maxLen = value
			}
			p, err := ffiPointerFromValue(args[0])
			if err != nil {
				return nil, err
			}
			fp, ok := ffiPointerOf(p)
			if !ok {
				return nil, ffiError("E0131", "readCString requires a valid FFI pointer")
			}
			return lunexruntime.StringVal(ffiCString(fp, maxLen)), nil
		}),
		"copy": nativeFunc("copy", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) < 3 {
				return nil, ffiError("E0125", "copy requires a destination pointer, source pointer, and length")
			}
			dst, err := ffiPointerFromValue(args[0])
			if err != nil {
				return nil, err
			}
			src, err := ffiPointerFromValue(args[1])
			if err != nil {
				return nil, err
			}
			n, err := ffiUintptrArg(args, 2, "length")
			if err != nil {
				return nil, err
			}
			dp, _ := ffiPointerOf(dst)
			sp, _ := ffiPointerOf(src)
			if dp == nil || sp == nil {
				return nil, ffiError("E0131", "copy requires valid FFI pointers")
			}
			da := dp.addressAt(0)
			sa := sp.addressAt(0)
			if n != 0 && (da == 0 || sa == 0) {
				return nil, ffiError("E0132", "copy cannot access a null pointer")
			}
			if dp.bounded && n > uintptr(dp.capacity()) {
				return nil, ffiError("E0132", "copy exceeds the known destination length")
			}
			if sp.bounded && n > uintptr(sp.capacity()) {
				return nil, ffiError("E0132", "copy exceeds the known source length")
			}
			if err := ffiCopy(da, sa, n); err != nil {
				return nil, ffiBackendError("E0132", "native memory copy failed", err)
			}
			return lunexruntime.Undefined, nil
		}),
		"fill": nativeFunc("fill", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) < 3 {
				return nil, ffiError("E0125", "fill requires a pointer, a byte value, and a length")
			}
			p, err := ffiPointerFromValue(args[0])
			if err != nil {
				return nil, err
			}
			fp, _ := ffiPointerOf(p)
			value, err := ffiUint8Arg(args[1], "value")
			if err != nil {
				return nil, err
			}
			length, err := ffiUintptrArg(args, 2, "length")
			if err != nil {
				return nil, err
			}
			if fp == nil {
				return nil, ffiError("E0131", "fill requires a valid FFI pointer")
			}
			if fp.bounded && length > uintptr(fp.capacity()) {
				return nil, ffiError("E0132", "fill exceeds the known pointer length")
			}
			addr := fp.addressAt(0)
			if length != 0 && addr == 0 {
				return nil, ffiError("E0132", "fill cannot access a null pointer")
			}
			if err := ffiFill(addr, value, length); err != nil {
				return nil, ffiBackendError("E0132", "native memory fill failed", err)
			}
			return lunexruntime.Undefined, nil
		}),
		"sizeof": nativeFunc("sizeof", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) == 0 {
				return nil, ffiError("E0125", "sizeof requires a type")
			}
			typ, err := parseFFIType(args[0].ToString(), true)
			if err != nil {
				return nil, err
			}
			return lunexruntime.NumberVal(float64(ffiTypeSize(typ))), nil
		}),
		"alignof": nativeFunc("alignof", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) == 0 {
				return nil, ffiError("E0125", "alignof requires a type")
			}
			typ, err := parseFFIType(args[0].ToString(), true)
			if err != nil {
				return nil, err
			}
			return lunexruntime.NumberVal(float64(ffiTypeAlign(typ))), nil
		}),
		"typeInfo": nativeFunc("typeInfo", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
			if err := ensureFFIEnabled(); err != nil {
				return nil, err
			}
			if len(args) == 0 {
				return nil, ffiError("E0125", "typeInfo requires a type")
			}
			typ, err := parseFFIType(args[0].ToString(), true)
			if err != nil {
				return nil, err
			}
			return ffiTypeInfo(typ), nil
		}),
	})
}

func nativeFunc(name string, fn func([]*lunexruntime.Value, *lunexruntime.Value) (*lunexruntime.Value, error)) *lunexruntime.Value {
	return lunexruntime.FuncVal(&lunexruntime.Function{Name: name, Native: fn})
}

func ffiBoolOption(v *lunexruntime.Value, key string) bool {
	if v == nil || v.Tag != lunexruntime.TypeObject {
		return false
	}
	value := v.Get(key)
	return value != nil && value.IsTruthy()
}

func ffiBackendError(code, prefix string, err error) error {
	e := ffiError(code, fmt.Sprintf("%s: %v", prefix, err)).(*errfmt.LunexError)
	e.Notes = append(e.Notes, "the native runtime reported an allocation, loader, or operating-system failure")
	return e
}

func parseFFISignature(v *lunexruntime.Value) (ffiSignature, error) {
	if v == nil || v.IsNullish() {
		return ffiSignature{}, ffiError("E0126", "FFI signature cannot be null or undefined")
	}
	if v.Tag == lunexruntime.TypeString {
		return parseFFISignatureString(v.StrVal)
	}
	if v.Tag != lunexruntime.TypeObject {
		return ffiSignature{}, ffiError("E0126", "FFI signature must be a string or descriptor object")
	}
	argsValue := v.Get("args")
	returnsValue := v.Get("returns")
	if argsValue == nil || argsValue.Tag != lunexruntime.TypeArray {
		return ffiSignature{}, ffiError("E0126", "FFI descriptor requires an `args` array")
	}
	if returnsValue == nil || returnsValue.Tag != lunexruntime.TypeString {
		return ffiSignature{}, ffiError("E0126", "FFI descriptor requires a string `returns` type")
	}
	args := make([]ffiType, 0, len(argsValue.ArrVal))
	for i, item := range argsValue.ArrVal {
		if item == nil || item.Tag != lunexruntime.TypeString {
			return ffiSignature{}, ffiError("E0126", fmt.Sprintf("FFI descriptor argument %d must be a type string", i))
		}
		typ, err := parseFFIType(item.StrVal, false)
		if err != nil {
			return ffiSignature{}, err
		}
		args = append(args, typ)
	}
	ret, err := parseFFIType(returnsValue.StrVal, true)
	if err != nil {
		return ffiSignature{}, err
	}
	return ffiSignature{args: args, returns: ret, text: ffiSignatureText(args, ret)}, nil
}

func parseFFISignatureString(s string) (ffiSignature, error) {
	text := strings.TrimSpace(s)
	open := strings.IndexByte(text, '(')
	if open <= 0 || !strings.HasSuffix(text, ")") {
		return ffiSignature{}, ffiError("E0126", fmt.Sprintf("invalid FFI signature %q; expected `returnType(argType, ...)`", s))
	}
	retText := strings.TrimSpace(text[:open])
	argsText := text[open+1 : len(text)-1]
	parts, err := splitTopLevel(argsText, ',')
	if err != nil {
		return ffiSignature{}, ffiError("E0126", err.Error())
	}
	args := make([]ffiType, 0, len(parts))
	if strings.TrimSpace(argsText) != "" {
		for _, part := range parts {
			typ, err := parseFFIType(strings.TrimSpace(part), false)
			if err != nil {
				return ffiSignature{}, err
			}
			args = append(args, typ)
		}
	}
	ret, err := parseFFIType(retText, true)
	if err != nil {
		return ffiSignature{}, err
	}
	return ffiSignature{args: args, returns: ret, text: ffiSignatureText(args, ret)}, nil
}

func ffiSignatureText(args []ffiType, ret ffiType) string {
	parts := make([]string, len(args))
	for i, typ := range args {
		parts[i] = typ.token
	}
	return ret.token + "(" + strings.Join(parts, ", ") + ")"
}

func parseFFIType(raw string, allowVoid bool) (ffiType, error) {
	text := strings.TrimSpace(strings.ToLower(raw))
	text = strings.ReplaceAll(text, "const ", "")
	text = strings.ReplaceAll(text, "volatile ", "")
	text = strings.ReplaceAll(text, "restrict ", "")
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return ffiType{}, ffiError("E0127", "FFI type cannot be empty")
	}
	if strings.HasSuffix(text, "...") {
		return ffiType{}, ffiError("E0126", "variadic C functions require a fixed ABI signature")
	}
	if text == "void" {
		if !allowVoid {
			return ffiType{}, ffiError("E0127", "void is valid only as an FFI return type")
		}
		return ffiType{token: "void", kind: "void", goType: reflect.TypeOf(struct{}{})}, nil
	}

	if baseText, count, ok := trailingArrayType(text); ok {
		if count <= 0 {
			return ffiType{}, ffiError("E0127", fmt.Sprintf("invalid FFI array length in %q", raw))
		}
		base, err := parseFFIType(baseText, false)
		if err != nil {
			return ffiType{}, err
		}
		if base.kind == "void" {
			return ffiType{}, ffiError("E0127", "FFI arrays cannot contain void")
		}
		if base.kind == "cstring" {
			base = ffiType{token: "ptr", kind: "pointer", goType: reflect.TypeOf(uintptr(0))}
		}
		if count > 1<<20 {
			return ffiType{}, ffiError("E0127", "FFI array length exceeds the supported limit")
		}
		arrType := reflect.ArrayOf(count, base.goType)
		return ffiType{token: base.token + "[" + strconv.Itoa(count) + "]", kind: "array", goType: arrType, elem: &base, count: count}, nil
	}

	pointerDepth := 0
	for strings.HasSuffix(text, "*") {
		pointerDepth++
		text = strings.TrimSpace(strings.TrimSuffix(text, "*"))
	}
	if pointerDepth > 0 {
		if text == "char" || text == "signed char" || text == "unsigned char" {
			if pointerDepth == 1 && text == "char" {
				return ffiType{token: "cstring", kind: "cstring", goType: reflect.TypeOf(uintptr(0)), pointee: &ffiType{token: "i8", kind: "scalar", goType: ffiScalarTypes["i8"]}}, nil
			}
		}
		if text == "void" {
			return ffiType{token: "ptr", kind: "pointer", goType: reflect.TypeOf(uintptr(0)), pointee: &ffiType{token: "void", kind: "void", goType: reflect.TypeOf(struct{}{})}}, nil
		}
		base, err := parseFFIType(text, false)
		if err != nil {
			return ffiType{}, err
		}
		for i := 0; i < pointerDepth; i++ {
			base = ffiType{token: "*" + base.token, kind: "pointer", goType: reflect.TypeOf(uintptr(0)), pointee: &base}
		}
		return base, nil
	}

	if strings.HasPrefix(text, "struct{") && strings.HasSuffix(text, "}") {
		return parseFFIStructType(text)
	}

	aliases := map[string]string{
		"char": "i8", "signed char": "i8", "unsigned char": "u8",
		"short": "i16", "signed short": "i16", "unsigned short": "u16",
		"int": "i32", "signed int": "i32", "unsigned int": "u32",
		"int8": "i8", "uint8": "u8", "int16": "i16", "uint16": "u16",
		"int32": "i32", "uint32": "u32", "int64": "i64", "uint64": "u64",
		"int8_t": "i8", "uint8_t": "u8", "int16_t": "i16", "uint16_t": "u16",
		"int32_t": "i32", "uint32_t": "u32", "int64_t": "i64", "uint64_t": "u64",
		"size_t": "usize", "ssize_t": "isize", "ptrdiff_t": "isize",
		"intptr_t": "intptr", "uintptr_t": "uintptr",
		"c_int": "i32", "c_uint": "u32", "c_longlong": "i64", "c_ulonglong": "u64",
		"byte": "u8", "float32": "f32", "float64": "f64",
		"pointer": "ptr", "rawptr": "ptr", "voidptr": "ptr",
		"cstring": "cstring", "string": "cstring", "buffer": "ptr", "ptr": "ptr",
		"boolean": "bool",
	}
	if canonical, ok := aliases[text]; ok {
		text = canonical
	}
	if strings.HasPrefix(text, "enum ") {
		text = "i32"
	}
	if text == "long" || text == "signed long" || text == "c_long" {
		if goruntime.GOOS == "windows" {
			text = "i32"
		} else if strconv.IntSize == 64 {
			text = "i64"
		} else {
			text = "i32"
		}
	}
	if text == "unsigned long" || text == "ulong" || text == "c_ulong" {
		if goruntime.GOOS == "windows" {
			text = "u32"
		} else if strconv.IntSize == 64 {
			text = "u64"
		} else {
			text = "u32"
		}
	}
	if text == "cstring" {
		return ffiType{token: "cstring", kind: "cstring", goType: reflect.TypeOf(uintptr(0))}, nil
	}
	if text == "ptr" {
		return ffiType{token: "ptr", kind: "pointer", goType: reflect.TypeOf(uintptr(0))}, nil
	}
	if typ, ok := ffiScalarTypes[text]; ok {
		return ffiType{token: text, kind: "scalar", goType: typ}, nil
	}
	return ffiType{}, ffiError("E0127", fmt.Sprintf("unsupported FFI type %q", raw))
}

func trailingArrayType(text string) (string, int, bool) {
	if !strings.HasSuffix(text, "]") {
		return "", 0, false
	}
	open := strings.LastIndexByte(text, '[')
	if open <= 0 || open >= len(text)-1 {
		return "", 0, false
	}
	countText := strings.TrimSpace(text[open+1 : len(text)-1])
	if countText == "" {
		return "", 0, false
	}
	count, err := strconv.Atoi(countText)
	if err != nil {
		return "", 0, false
	}
	return strings.TrimSpace(text[:open]), count, true
}

func parseFFIStructType(text string) (ffiType, error) {
	inner := strings.TrimSpace(text[len("struct{") : len(text)-1])
	parts, err := splitTopLevelAny(inner, ',', ';')
	if err != nil {
		return ffiType{}, ffiError("E0126", err.Error())
	}
	fields := make([]ffiField, 0, len(parts))
	reflectFields := make([]reflect.StructField, 0, len(parts))
	seen := make(map[string]struct{})
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name := fmt.Sprintf("F%d", i)
		reflectName := name
		typeText := part
		if idx := topLevelColon(part); idx > 0 {
			candidate := strings.TrimSpace(part[:idx])
			if candidate != "" {
				name = candidate
				reflectName = exportedStructField(candidate, i)
			}
			typeText = strings.TrimSpace(part[idx+1:])
		}
		if typeText == "" {
			return ffiType{}, ffiError("E0126", fmt.Sprintf("struct field %q has no type", name))
		}
		fieldType, err := parseFFIType(typeText, false)
		if err != nil {
			return ffiType{}, err
		}
		if fieldType.kind == "scalar" && fieldType.goType.Kind() == reflect.String {
			return ffiType{}, ffiError("E0127", fmt.Sprintf("FFI struct field %q cannot use the Lunex string type; use cstring", name))
		}
		if fieldType.kind == "cstring" {
			fieldType = ffiType{token: "ptr", kind: "pointer", goType: reflect.TypeOf(uintptr(0))}
		}
		if _, exists := seen[name]; exists {
			return ffiType{}, ffiError("E0126", fmt.Sprintf("duplicate FFI struct field %q", name))
		}
		seen[name] = struct{}{}
		fields = append(fields, ffiField{name: name, reflectName: reflectName, typ: fieldType})
		reflectFields = append(reflectFields, reflect.StructField{Name: reflectName, Type: fieldType.goType})
	}
	if len(reflectFields) == 0 {
		return ffiType{}, ffiError("E0126", "FFI structs require at least one field")
	}
	goType := reflect.StructOf(reflectFields)
	return ffiType{token: text, kind: "struct", goType: goType, fields: fields}, nil
}

func exportedStructField(name string, index int) string {
	var b strings.Builder
	for _, r := range name {
		if r == '_' || r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	clean := b.String()
	if clean == "" {
		return fmt.Sprintf("F%d", index)
	}
	if clean[0] >= 'a' && clean[0] <= 'z' {
		clean = strings.ToUpper(clean[:1]) + clean[1:]
	}
	if clean[0] >= '0' && clean[0] <= '9' {
		return fmt.Sprintf("F%d_%s", index, clean)
	}
	return clean
}

func splitTopLevel(s string, delimiter rune) ([]string, error) {
	return splitTopLevelAny(s, delimiter)
}

func splitTopLevelAny(s string, delimiters ...rune) ([]string, error) {
	depth := 0
	start := 0
	parts := make([]string, 0, 4)
	isDelimiter := func(r rune) bool {
		for _, d := range delimiters {
			if r == d {
				return true
			}
		}
		return false
	}
	for i, r := range s {
		switch r {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced FFI signature delimiters")
			}
		}
		if depth == 0 && isDelimiter(r) {
			parts = append(parts, s[start:i])
			start = i + len(string(r))
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced FFI signature delimiters")
	}
	parts = append(parts, s[start:])
	return parts, nil
}

func topLevelColon(s string) int {
	depth := 0
	for i, r := range s {
		switch r {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ':':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func ffiBind(lib *ffiLibrary, name string, sig ffiSignature) (*lunexruntime.Value, error) {
	lib.mu.Lock()
	if lib.closed || lib.handle == 0 || lib.closeQueued {
		lib.mu.Unlock()
		return nil, ffiError("E0124", fmt.Sprintf("cannot bind %q from a closed native library", name))
	}
	handle := lib.handle
	lib.active++
	lib.mu.Unlock()

	symbol, err := ffiLookupSymbol(handle, name)
	if err != nil || symbol == 0 {
		lib.mu.Lock()
		lib.active--
		lib.mu.Unlock()
		if err == nil {
			err = fmt.Errorf("symbol resolved to a null address")
		}
		e := ffiError("E0123", fmt.Sprintf("symbol %q was not found: %v", name, err)).(*errfmt.LunexError)
		e.Suggestion = "check the exact exported symbol name and the library ABI"
		return nil, e
	}

	fnType, err := ffiGoFuncType(sig)
	if err != nil {
		lib.mu.Lock()
		lib.active--
		lib.mu.Unlock()
		return nil, err
	}
	fnSlot := reflect.New(fnType)
	if err := ffiRegisterFunc(fnSlot.Interface(), symbol); err != nil {
		lib.mu.Lock()
		lib.active--
		lib.mu.Unlock()
		return nil, ffiBackendError("E0128", fmt.Sprintf("failed to prepare callable symbol %q", name), err)
	}
	bound := &ffiFunction{library: lib, name: name, sig: sig, fn: fnSlot.Elem()}
	return ffiFunctionValue(bound), nil
}

func ffiGoFuncType(sig ffiSignature) (reflect.Type, error) {
	inTypes := make([]reflect.Type, len(sig.args))
	for i, typ := range sig.args {
		if typ.kind == "void" {
			return nil, ffiError("E0127", fmt.Sprintf("argument %d cannot have type void", i))
		}
		inTypes[i] = typ.goType
	}
	outTypes := []reflect.Type{}
	if sig.returns.kind != "void" {
		outTypes = []reflect.Type{sig.returns.goType}
	}
	return reflect.FuncOf(inTypes, outTypes, false), nil
}

func ffiLookup(lib *ffiLibrary, name string) (*lunexruntime.Value, error) {
	if strings.TrimSpace(name) == "" {
		return nil, ffiError("E0125", "native symbol name cannot be empty")
	}
	lib.mu.Lock()
	closed := lib.closed || lib.handle == 0
	h := lib.handle
	lib.mu.Unlock()
	if closed {
		return nil, ffiError("E0124", "cannot resolve a symbol from a closed native library")
	}
	sym, err := ffiLookupSymbol(h, name)
	if err != nil || sym == 0 {
		if err == nil {
			err = fmt.Errorf("symbol resolved to a null address")
		}
		return nil, ffiError("E0123", fmt.Sprintf("symbol %q was not found: %v", name, err))
	}
	return ffiPointerValue(&ffiPointer{address: sym}), nil
}

func ffiLibraryValue(lib *ffiLibrary) *lunexruntime.Value {
	obj := lunexruntime.ObjectVal(map[string]*lunexruntime.Value{})
	obj.ObjVal["path"] = lunexruntime.StringVal(lib.path)
	obj.ObjVal["bind"] = nativeFunc("bind", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
		if err := ensureFFIEnabled(); err != nil {
			return nil, err
		}
		if len(args) < 2 {
			return nil, ffiError("E0125", "library.bind requires a symbol name and signature")
		}
		sig, err := parseFFISignature(args[1])
		if err != nil {
			return nil, err
		}
		return ffiBind(lib, args[0].ToString(), sig)
	})
	obj.ObjVal["symbol"] = nativeFunc("symbol", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
		if err := ensureFFIEnabled(); err != nil {
			return nil, err
		}
		if len(args) < 1 {
			return nil, ffiError("E0125", "library.symbol requires a symbol name")
		}
		return ffiLookup(lib, args[0].ToString())
	})
	obj.ObjVal["close"] = nativeFunc("close", func(_ []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
		return ffiCloseLibraryValue(lib)
	})
	obj.ObjVal["isClosed"] = nativeFunc("isClosed", func(_ []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
		lib.mu.Lock()
		closed := lib.closed
		lib.mu.Unlock()
		return lunexruntime.BoolVal(closed), nil
	})
	obj.ObjVal["handle"] = nativeFunc("handle", func(_ []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
		lib.mu.Lock()
		h := lib.handle
		lib.mu.Unlock()
		return ffiPointerValue(&ffiPointer{address: h}), nil
	})
	obj.ObjVal["active"] = nativeFunc("active", func(_ []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
		lib.mu.Lock()
		active := lib.active
		lib.mu.Unlock()
		return lunexruntime.NumberVal(float64(active)), nil
	})
	obj.ObjVal["__lunex_ffi_library"] = lunexruntime.ErrorVal(ffiLibraryMarker{lib: lib})
	return obj
}

func ffiFunctionValue(fn *ffiFunction) *lunexruntime.Value {
	value := lunexruntime.FuncVal(&lunexruntime.Function{Name: fn.name, Native: func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
		fn.mu.Lock()
		if fn.closed {
			fn.mu.Unlock()
			return nil, ffiError("E0124", fmt.Sprintf("native function %q is closed", fn.name))
		}
		callable := fn.fn
		sig := fn.sig
		fn.mu.Unlock()
		return ffiInvoke(fn.name, callable, sig, args)
	}})
	ffiFunctionRegistry.Store(value, fn)
	return value
}

func ffiFunctionOf(value *lunexruntime.Value) (*ffiFunction, bool) {
	if value == nil || value.Tag != lunexruntime.TypeFunction {
		return nil, false
	}
	fn, ok := ffiFunctionRegistry.Load(value)
	if !ok {
		return nil, false
	}
	f, ok := fn.(*ffiFunction)
	return f, ok
}

func ffiLibraryOf(value *lunexruntime.Value) (*ffiLibrary, bool) {
	if value == nil || value.Tag != lunexruntime.TypeObject || value.ObjVal == nil {
		return nil, false
	}
	marker := value.ObjVal["__lunex_ffi_library"]
	if marker == nil || marker.Tag != lunexruntime.TypeError {
		return nil, false
	}
	m, ok := marker.ErrVal.(ffiLibraryMarker)
	return m.lib, ok && m.lib != nil
}

func ffiCloseFunctionValue(value *lunexruntime.Value, fn *ffiFunction) (*lunexruntime.Value, error) {
	fn.mu.Lock()
	if fn.closed {
		fn.mu.Unlock()
		return lunexruntime.False, nil
	}
	fn.closed = true
	lib := fn.library
	fn.mu.Unlock()
	ffiFunctionRegistry.Delete(value)
	if lib != nil {
		return ffiReleaseLibraryBinding(lib)
	}
	return lunexruntime.True, nil
}

func ffiReleaseLibraryBinding(lib *ffiLibrary) (*lunexruntime.Value, error) {
	lib.mu.Lock()
	if lib.active > 0 {
		lib.active--
	}
	shouldClose := lib.closeQueued && lib.active == 0 && !lib.closed && lib.handle != 0
	h := lib.handle
	if shouldClose {
		lib.closed = true
		lib.handle = 0
	}
	lib.mu.Unlock()
	if shouldClose && h != 0 {
		if err := ffiCloseLibrary(h); err != nil {
			return nil, ffiBackendError("E0124", "native library close failed", err)
		}
	}
	return lunexruntime.True, nil
}

func ffiCloseLibraryValue(lib *ffiLibrary) (*lunexruntime.Value, error) {
	lib.mu.Lock()
	if lib.closed || lib.closeQueued {
		lib.mu.Unlock()
		return lunexruntime.False, nil
	}
	lib.closeQueued = true
	shouldClose := lib.active == 0 && lib.handle != 0
	h := lib.handle
	if shouldClose {
		lib.closed = true
		lib.handle = 0
	}
	lib.mu.Unlock()
	if shouldClose && h != 0 {
		if err := ffiCloseLibrary(h); err != nil {
			return nil, ffiBackendError("E0124", "native library close failed", err)
		}
	}
	return lunexruntime.True, nil
}

func ffiInvoke(name string, fn reflect.Value, sig ffiSignature, args []*lunexruntime.Value) (result *lunexruntime.Value, err error) {
	if len(args) != len(sig.args) {
		return nil, ffiError("E0125", fmt.Sprintf("native function %q expects %d argument(s), got %d", name, len(sig.args), len(args)))
	}
	in := make([]reflect.Value, len(args))
	allocated := make([]uintptr, 0, len(args))
	defer func() {
		for _, ptr := range allocated {
			_ = ffiFree(ptr)
		}
		if recovered := recover(); recovered != nil {
			e := ffiError("E0128", fmt.Sprintf("native call %q failed: %v", name, recovered)).(*errfmt.LunexError)
			e.Suggestion = "verify the declared ABI signature, argument types, pointer ownership, and native calling convention"
			result = nil
			err = e
		}
	}()
	for i, typ := range sig.args {
		value, temps, convErr := ffiToReflect(args[i], typ)
		if convErr != nil {
			return nil, convErr
		}
		allocated = append(allocated, temps...)
		in[i] = value
	}
	out := fn.Call(in)
	if len(out) == 0 {
		return lunexruntime.Undefined, nil
	}
	return ffiFromReflect(out[0], sig.returns)
}

func ffiCallTarget(interp *lunexruntime.Interpreter, args []*lunexruntime.Value) (*lunexruntime.Value, error) {
	if len(args) == 0 {
		return nil, ffiError("E0125", "call requires a bound function or function pointer")
	}
	if fn, ok := ffiFunctionOf(args[0]); ok {
		fn.mu.Lock()
		if fn.closed {
			fn.mu.Unlock()
			return nil, ffiError("E0124", fmt.Sprintf("native function %q is closed", fn.name))
		}
		callable := fn.fn
		sig := fn.sig
		name := fn.name
		fn.mu.Unlock()
		callArgs := args[1:]
		if len(args) == 2 && args[1] != nil && args[1].Tag == lunexruntime.TypeArray {
			callArgs = args[1].ArrVal
		}
		return ffiInvoke(name, callable, sig, callArgs)
	}
	if len(args) < 3 {
		return nil, ffiError("E0125", "raw pointer calls require a pointer, a signature, and an argument array")
	}
	ptr, err := ffiPointerFromValue(args[0])
	if err != nil {
		return nil, err
	}
	fp, ok := ffiPointerOf(ptr)
	if !ok || fp.addressAt(0) == 0 {
		return nil, ffiError("E0132", "cannot call through a null function pointer")
	}
	sig, err := parseFFISignature(args[1])
	if err != nil {
		return nil, err
	}
	callArgs := args[2]
	if callArgs == nil || callArgs.Tag != lunexruntime.TypeArray {
		return nil, ffiError("E0125", "raw pointer calls require the argument list to be an array")
	}
	fnType, err := ffiGoFuncType(sig)
	if err != nil {
		return nil, err
	}
	slot := reflect.New(fnType)
	if err := ffiRegisterFunc(slot.Interface(), fp.addressAt(0)); err != nil {
		return nil, ffiBackendError("E0128", "failed to prepare function pointer", err)
	}
	_ = interp
	return ffiInvoke("<function pointer>", slot.Elem(), sig, callArgs.ArrVal)
}

func ffiToReflect(value *lunexruntime.Value, typ ffiType) (reflect.Value, []uintptr, error) {
	if value == nil {
		value = lunexruntime.Undefined
	}
	switch typ.kind {
	case "scalar":
		return scalarToReflect(value, typ.goType)
	case "cstring":
		if value.IsNullish() {
			return reflect.ValueOf(uintptr(0)), nil, nil
		}
		if value.Tag == lunexruntime.TypeString {
			data := append([]byte(value.StrVal), 0)
			addr, err := ffiMalloc(uintptr(len(data)))
			if err != nil {
				return reflect.Value{}, nil, ffiBackendError("E0130", "native string allocation failed", err)
			}
			if addr == 0 && len(data) != 0 {
				return reflect.Value{}, nil, ffiError("E0130", "native string allocation returned a null pointer")
			}
			if err := copyBytesToNative(addr, data); err != nil {
				_ = ffiFree(addr)
				return reflect.Value{}, nil, err
			}
			return reflect.ValueOf(addr), []uintptr{addr}, nil
		}
		ptrValue, err := ffiPointerFromValue(value)
		if err != nil {
			return reflect.Value{}, nil, ffiError("E0131", "cstring argument must be a string, pointer, or null")
		}
		ptr, ok := ffiPointerOf(ptrValue)
		if !ok {
			return reflect.Value{}, nil, ffiError("E0131", "invalid cstring pointer")
		}
		return reflect.ValueOf(ptr.addressAt(0)), nil, nil
	case "pointer":
		ptrValue, err := ffiPointerFromValue(value)
		if err != nil {
			return reflect.Value{}, nil, err
		}
		ptr, ok := ffiPointerOf(ptrValue)
		if !ok {
			return reflect.Value{}, nil, ffiError("E0131", "pointer argument requires a valid FFI pointer")
		}
		return reflect.ValueOf(ptr.addressAt(0)), nil, nil
	case "struct":
		return structToReflect(value, typ)
	case "array":
		return arrayToReflect(value, typ)
	default:
		return reflect.Value{}, nil, ffiError("E0127", fmt.Sprintf("unsupported native argument kind %q", typ.kind))
	}
}

func scalarToReflect(value *lunexruntime.Value, target reflect.Type) (reflect.Value, []uintptr, error) {
	if target.Kind() == reflect.String {
		if value.Tag != lunexruntime.TypeString {
			return reflect.Value{}, nil, ffiError("E0131", "native string arguments require a Lunex string")
		}
		return reflect.ValueOf(value.StrVal), nil, nil
	}
	if value != nil && value.Tag == lunexruntime.TypeString && isFFIIntegral(target.Kind()) {
		out := reflect.New(target).Elem()
		text := strings.TrimSpace(value.StrVal)
		if text == "" {
			return reflect.Value{}, nil, ffiError("E0131", "native integer strings cannot be empty")
		}
		if target.Kind() == reflect.Uint || target.Kind() == reflect.Uint8 || target.Kind() == reflect.Uint16 || target.Kind() == reflect.Uint32 || target.Kind() == reflect.Uint64 || target.Kind() == reflect.Uintptr {
			n, err := strconv.ParseUint(text, 0, target.Bits())
			if err != nil {
				return reflect.Value{}, nil, ffiError("E0131", fmt.Sprintf("value %q does not fit native %s", value.StrVal, target.String()))
			}
			out.SetUint(n)
			return out, nil, nil
		}
		n, err := strconv.ParseInt(text, 0, target.Bits())
		if err != nil {
			return reflect.Value{}, nil, ffiError("E0131", fmt.Sprintf("value %q does not fit native %s", value.StrVal, target.String()))
		}
		out.SetInt(n)
		return out, nil, nil
	}
	if target.Kind() == reflect.Bool {
		return reflect.ValueOf(value.IsTruthy()), nil, nil
	}
	if value.Tag != lunexruntime.TypeNumber && value.Tag != lunexruntime.TypeBool {
		return reflect.Value{}, nil, ffiError("E0131", fmt.Sprintf("native scalar %s requires a number or boolean", target.String()))
	}
	n := value.ToNumber()
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return reflect.Value{}, nil, ffiError("E0131", "NaN and Infinity are not valid native scalar arguments")
	}
	out := reflect.New(target).Elem()
	switch target.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if math.Trunc(n) != n || n < float64(minSigned(target.Bits())) || n > float64(maxSigned(target.Bits())) {
			return reflect.Value{}, nil, ffiError("E0131", fmt.Sprintf("value %v does not fit native %s", n, target.String()))
		}
		out.SetInt(int64(n))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if math.Trunc(n) != n || n < 0 || n > float64(maxUnsigned(target.Bits())) {
			return reflect.Value{}, nil, ffiError("E0131", fmt.Sprintf("value %v does not fit native %s", n, target.String()))
		}
		if target.Kind() == reflect.Uintptr && n > maxExactFloatInteger() {
			return reflect.Value{}, nil, ffiError("E0131", "numeric pointer values above 2^53-1 must be supplied as hexadecimal addresses")
		}
		out.SetUint(uint64(n))
	case reflect.Float32, reflect.Float64:
		out.SetFloat(n)
	default:
		return reflect.Value{}, nil, ffiError("E0131", fmt.Sprintf("unsupported native scalar %s", target.String()))
	}
	return out, nil, nil
}

func isFFIIntegral(kind reflect.Kind) bool {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	default:
		return false
	}
}

func minSigned(bits int) int64 {
	if bits >= 64 {
		return math.MinInt64
	}
	return -(int64(1) << uint(bits-1))
}

func maxSigned(bits int) int64 {
	if bits >= 64 {
		return math.MaxInt64
	}
	return (int64(1) << uint(bits-1)) - 1
}

func maxUnsigned(bits int) uint64 {
	if bits >= 64 {
		return math.MaxUint64
	}
	return (uint64(1) << uint(bits)) - 1
}

func maxExactFloatInteger() float64 {
	return 9007199254740991
}

func ffiFromReflect(value reflect.Value, typ ffiType) (*lunexruntime.Value, error) {
	switch typ.kind {
	case "void":
		return lunexruntime.Undefined, nil
	case "cstring":
		addr, err := reflectUintptr(value)
		if err != nil {
			return nil, err
		}
		if addr == 0 {
			return lunexruntime.Null, nil
		}
		return lunexruntime.StringVal(readNativeCString(addr, 1<<20)), nil
	case "pointer":
		addr, err := reflectUintptr(value)
		if err != nil {
			return nil, err
		}
		return ffiPointerValue(&ffiPointer{address: addr}), nil
	case "scalar":
		switch value.Kind() {
		case reflect.Bool:
			return lunexruntime.BoolVal(value.Bool()), nil
		case reflect.String:
			return lunexruntime.StringVal(value.String()), nil
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n := value.Int()
			if n > int64(maxExactFloatInteger()) || n < -int64(maxExactFloatInteger()) {
				return lunexruntime.StringVal(strconv.FormatInt(n, 10)), nil
			}
			return lunexruntime.NumberVal(float64(n)), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			n := value.Uint()
			if n > uint64(maxExactFloatInteger()) {
				return lunexruntime.StringVal(strconv.FormatUint(n, 10)), nil
			}
			return lunexruntime.NumberVal(float64(n)), nil
		case reflect.Float32, reflect.Float64:
			return lunexruntime.NumberVal(value.Float()), nil
		}
	case "struct":
		return structFromReflect(value, typ)
	case "array":
		return arrayFromReflect(value, typ)
	}
	return nil, ffiError("E0128", fmt.Sprintf("unsupported native return type %q", typ.token))
}

func reflectUintptr(value reflect.Value) (uintptr, error) {
	switch value.Kind() {
	case reflect.Uintptr, reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
		return uintptr(value.Uint()), nil
	case reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8, reflect.Int:
		if value.Int() < 0 {
			return 0, ffiError("E0131", "native pointer return is negative")
		}
		return uintptr(value.Int()), nil
	default:
		return 0, ffiError("E0128", "native pointer return has an unsupported representation")
	}
}

func structToReflect(value *lunexruntime.Value, typ ffiType) (reflect.Value, []uintptr, error) {
	if value == nil || (value.Tag != lunexruntime.TypeObject && value.Tag != lunexruntime.TypeArray) {
		return reflect.Value{}, nil, ffiError("E0131", "native struct arguments require a Lunex object or array")
	}
	out := reflect.New(typ.goType).Elem()
	allocated := make([]uintptr, 0)
	for i, field := range typ.fields {
		var fieldValue *lunexruntime.Value
		if value.Tag == lunexruntime.TypeObject {
			fieldValue = value.Get(field.name)
			if fieldValue == nil || fieldValue.Tag == lunexruntime.TypeUndefined {
				fieldValue = value.Get(strconv.Itoa(i))
			}
		} else if i < len(value.ArrVal) {
			fieldValue = value.ArrVal[i]
		}
		if fieldValue == nil {
			fieldValue = lunexruntime.Undefined
		}
		rv, temps, err := ffiToReflect(fieldValue, field.typ)
		if err != nil {
			return reflect.Value{}, nil, err
		}
		allocated = append(allocated, temps...)
		out.Field(i).Set(rv)
	}
	return out, allocated, nil
}

func structFromReflect(value reflect.Value, typ ffiType) (*lunexruntime.Value, error) {
	obj := lunexruntime.ObjectVal(map[string]*lunexruntime.Value{})
	for i, field := range typ.fields {
		fieldValue, err := ffiFromReflect(value.Field(i), field.typ)
		if err != nil {
			return nil, err
		}
		obj.ObjVal[field.name] = fieldValue
	}
	return obj, nil
}

func arrayToReflect(value *lunexruntime.Value, typ ffiType) (reflect.Value, []uintptr, error) {
	out := reflect.New(typ.goType).Elem()
	if value == nil {
		return out, nil, nil
	}
	if b := extractBuffer(value); b != nil && typ.elem.kind == "scalar" && typ.elem.goType.Kind() == reflect.Uint8 {
		b.mu.Lock()
		defer b.mu.Unlock()
		if len(b.data) > typ.count {
			return reflect.Value{}, nil, ffiError("E0131", "std.buffer value is larger than the declared native array")
		}
		for i, byteValue := range b.data {
			out.Index(i).SetUint(uint64(byteValue))
		}
		return out, nil, nil
	}
	if value.Tag != lunexruntime.TypeArray {
		return reflect.Value{}, nil, ffiError("E0131", "native array arguments require a Lunex array or compatible std.buffer")
	}
	if len(value.ArrVal) > typ.count {
		return reflect.Value{}, nil, ffiError("E0131", "native array argument is larger than its declared length")
	}
	allocated := make([]uintptr, 0)
	for i, item := range value.ArrVal {
		rv, temps, err := ffiToReflect(item, *typ.elem)
		if err != nil {
			return reflect.Value{}, nil, err
		}
		allocated = append(allocated, temps...)
		out.Index(i).Set(rv)
	}
	return out, allocated, nil
}

func arrayFromReflect(value reflect.Value, typ ffiType) (*lunexruntime.Value, error) {
	arr := make([]*lunexruntime.Value, typ.count)
	for i := 0; i < typ.count; i++ {
		item, err := ffiFromReflect(value.Index(i), *typ.elem)
		if err != nil {
			return nil, err
		}
		arr[i] = item
	}
	return lunexruntime.ArrayVal(arr), nil
}

func ffiPointerValue(ptr *ffiPointer) *lunexruntime.Value {
	obj := lunexruntime.ObjectVal(map[string]*lunexruntime.Value{})
	obj.ObjVal["address"] = nativeFunc("address", func(_ []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
		return lunexruntime.StringVal(fmt.Sprintf("0x%x", ptr.addressAt(0))), nil
	})
	obj.ObjVal["length"] = nativeFunc("length", func(_ []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
		return lunexruntime.NumberVal(float64(ptr.capacity())), nil
	})
	obj.ObjVal["isNull"] = nativeFunc("isNull", func(_ []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
		return lunexruntime.BoolVal(ptr.addressAt(0) == 0), nil
	})
	obj.ObjVal["readCString"] = nativeFunc("readCString", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
		maxLen := 1 << 20
		if len(args) > 0 {
			value, err := ffiIntArg(args, 0, "maxLength")
			if err != nil {
				return nil, err
			}
			if value < 0 {
				return nil, ffiError("E0131", "maxLength must be non-negative")
			}
			maxLen = value
		}
		return lunexruntime.StringVal(ffiCString(ptr, maxLen)), nil
	})
	obj.ObjVal["slice"] = nativeFunc("slice", func(args []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
		off, err := ffiOptionalOffset(args, 0)
		if err != nil {
			return nil, err
		}
		length := -1
		if len(args) > 1 {
			length, err = ffiIntArg(args, 1, "length")
			if err != nil {
				return nil, err
			}
		}
		return ffiPointerValue(ptr.slice(off, length)), nil
	})
	obj.ObjVal["close"] = nativeFunc("close", func(_ []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
		if ptr.callback == nil {
			return lunexruntime.False, nil
		}
		return ffiCloseCallback(ptr)
	})
	obj.ObjVal["free"] = nativeFunc("free", func(_ []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
		if err := ensureFFIEnabled(); err != nil {
			return nil, err
		}
		return ffiFreePointer(ptr)
	})
	obj.ObjVal["lastError"] = nativeFunc("lastError", func(_ []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
		if ptr.callback == nil {
			return lunexruntime.StringVal(""), nil
		}
		ptr.callback.mu.Lock()
		err := ptr.callback.lastErr
		ptr.callback.mu.Unlock()
		return lunexruntime.StringVal(err), nil
	})
	obj.ObjVal["callbackClosed"] = nativeFunc("callbackClosed", func(_ []*lunexruntime.Value, _ *lunexruntime.Value) (*lunexruntime.Value, error) {
		if ptr.callback == nil {
			return lunexruntime.False, nil
		}
		ptr.callback.mu.Lock()
		closed := ptr.callback.closed
		ptr.callback.mu.Unlock()
		return lunexruntime.BoolVal(closed), nil
	})
	obj.ObjVal["__lunex_ffi_ptr"] = lunexruntime.ErrorVal(ffiPointerMarker{ptr: ptr})
	return obj
}

func ffiPointerFromValue(value *lunexruntime.Value) (*lunexruntime.Value, error) {
	if value == nil || value.IsNullish() {
		return ffiPointerValue(&ffiPointer{}), nil
	}
	if value.Tag == lunexruntime.TypeObject {
		if _, ok := ffiPointerOf(value); ok {
			return value, nil
		}
		if b := extractBuffer(value); b != nil {
			return ffiPointerValue(&ffiPointer{buffer: b, length: len(b.data), bounded: true}), nil
		}
		return nil, ffiError("E0131", "expected an FFI pointer or std.buffer value")
	}
	if value.Tag == lunexruntime.TypeNumber {
		if value.NumVal < 0 || math.Trunc(value.NumVal) != value.NumVal || value.NumVal > maxExactFloatInteger() {
			return nil, ffiError("E0131", "numeric pointer values must be exact non-negative integers no larger than 2^53-1")
		}
		return ffiPointerValue(&ffiPointer{address: uintptr(value.NumVal)}), nil
	}
	if value.Tag == lunexruntime.TypeString {
		s := strings.TrimSpace(value.StrVal)
		if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
			digits := strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
			n, err := strconv.ParseUint(digits, 16, strconv.IntSize)
			if err != nil {
				return nil, ffiError("E0131", fmt.Sprintf("invalid pointer address %q", value.StrVal))
			}
			return ffiPointerValue(&ffiPointer{address: uintptr(n)}), nil
		}
	}
	return nil, ffiError("E0131", "expected an FFI pointer, std.buffer value, numeric address, or hexadecimal address string")
}

func ffiPointerOf(value *lunexruntime.Value) (*ffiPointer, bool) {
	if value == nil || value.Tag != lunexruntime.TypeObject || value.ObjVal == nil {
		return nil, false
	}
	marker := value.ObjVal["__lunex_ffi_ptr"]
	if marker == nil || marker.Tag != lunexruntime.TypeError {
		return nil, false
	}
	m, ok := marker.ErrVal.(ffiPointerMarker)
	return m.ptr, ok && m.ptr != nil
}

func (ptr *ffiPointer) addressAt(offset int) uintptr {
	if ptr == nil || offset < 0 {
		return 0
	}
	ptr.mu.RLock()
	if ptr.freed {
		ptr.mu.RUnlock()
		return 0
	}
	if ptr.buffer != nil {
		b := ptr.buffer
		ptr.mu.RUnlock()
		b.mu.Lock()
		defer b.mu.Unlock()
		if offset < 0 || offset >= len(b.data) {
			if offset == 0 && len(b.data) == 0 {
				return 0
			}
			return 0
		}
		return uintptr(unsafe.Pointer(&b.data[offset]))
	}
	addr := ptr.address
	ptr.mu.RUnlock()
	if addr == 0 {
		return 0
	}
	return addr + uintptr(offset)
}

func (ptr *ffiPointer) capacity() int {
	if ptr == nil {
		return 0
	}
	ptr.mu.RLock()
	if ptr.buffer != nil {
		b := ptr.buffer
		ptr.mu.RUnlock()
		b.mu.Lock()
		n := len(b.data)
		b.mu.Unlock()
		return n
	}
	n := ptr.length
	ptr.mu.RUnlock()
	return n
}

func (ptr *ffiPointer) slice(offset, length int) *ffiPointer {
	capLen := ptr.capacity()
	if ptr.bounded && (offset < 0 || offset > capLen) {
		return &ffiPointer{}
	}
	if offset < 0 {
		return &ffiPointer{}
	}
	if ptr.bounded {
		remaining := capLen - offset
		if length < 0 || length > remaining {
			length = remaining
		}
		return &ffiPointer{address: ptr.addressAt(offset), length: length, bounded: true, buffer: ptr.buffer}
	}
	if length < 0 {
		return &ffiPointer{address: ptr.addressAt(offset), buffer: ptr.buffer}
	}
	return &ffiPointer{address: ptr.addressAt(offset), length: length, bounded: true, buffer: ptr.buffer}
}

func ffiFreePointer(ptr *ffiPointer) (*lunexruntime.Value, error) {
	if ptr == nil {
		return lunexruntime.False, nil
	}
	ptr.mu.Lock()
	if ptr.freed || !ptr.owned {
		ptr.mu.Unlock()
		return lunexruntime.False, nil
	}
	addr := ptr.address
	ptr.address = 0
	ptr.freed = true
	ptr.mu.Unlock()
	if err := ffiFree(addr); err != nil {
		return nil, ffiBackendError("E0130", "native free failed", err)
	}
	return lunexruntime.True, nil
}

func ffiOptionalOffset(args []*lunexruntime.Value, index int) (int, error) {
	if len(args) <= index || args[index] == nil || args[index].IsNullish() {
		return 0, nil
	}
	return ffiIntArg(args, index, "offset")
}

func ffiUintptrArg(args []*lunexruntime.Value, index int, name string) (uintptr, error) {
	if len(args) <= index || args[index] == nil || args[index].Tag != lunexruntime.TypeNumber {
		return 0, ffiError("E0131", fmt.Sprintf("%s must be a non-negative integer", name))
	}
	value := args[index].NumVal
	if value < 0 || math.Trunc(value) != value || value > maxExactFloatInteger() {
		return 0, ffiError("E0131", fmt.Sprintf("%s must be an exact non-negative integer no larger than 2^53-1", name))
	}
	return uintptr(value), nil
}

func ffiIntArg(args []*lunexruntime.Value, index int, name string) (int, error) {
	if len(args) <= index || args[index] == nil || args[index].Tag != lunexruntime.TypeNumber {
		return 0, ffiError("E0131", fmt.Sprintf("%s must be an integer", name))
	}
	value := args[index].NumVal
	if math.Trunc(value) != value || value < float64(minHostInt()) || value > float64(maxHostInt()) {
		return 0, ffiError("E0131", fmt.Sprintf("%s is outside the host integer range", name))
	}
	return int(value), nil
}

func minHostInt() int64 {
	return -int64(^uint(0)>>1) - 1
}

func maxHostInt() int64 {
	return int64(^uint(0) >> 1)
}

func ffiUint8Arg(value *lunexruntime.Value, name string) (uint8, error) {
	if value == nil || value.Tag != lunexruntime.TypeNumber || math.Trunc(value.NumVal) != value.NumVal || value.NumVal < 0 || value.NumVal > 255 {
		return 0, ffiError("E0131", fmt.Sprintf("%s must be an integer from 0 to 255", name))
	}
	return uint8(value.NumVal), nil
}

func ffiSafeInt(value uintptr) int {
	max := uintptr(^uint(0) >> 1)
	if value > max {
		return int(max)
	}
	return int(value)
}

func copyBytesToNative(dst uintptr, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return ffiCopy(dst, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
}

func ffiBytesFromValue(value *lunexruntime.Value) ([]byte, error) {
	if value == nil || value.IsNullish() {
		return []byte{}, nil
	}
	if value.Tag == lunexruntime.TypeString {
		return []byte(value.StrVal), nil
	}
	if b := extractBuffer(value); b != nil {
		b.mu.Lock()
		data := append([]byte(nil), b.data...)
		b.mu.Unlock()
		return data, nil
	}
	if value.Tag == lunexruntime.TypeArray {
		data := make([]byte, len(value.ArrVal))
		for i, item := range value.ArrVal {
			if item == nil || item.Tag != lunexruntime.TypeNumber || math.Trunc(item.NumVal) != item.NumVal || item.NumVal < 0 || item.NumVal > 255 {
				return nil, ffiError("E0131", fmt.Sprintf("byte array entry %d must be an integer from 0 to 255", i))
			}
			data[i] = byte(item.NumVal)
		}
		return data, nil
	}
	return nil, ffiError("E0131", "byte data must be a string, array, or std.buffer")
}

func ffiByteArrayValue(data []byte) *lunexruntime.Value {
	arr := make([]*lunexruntime.Value, len(data))
	for i, b := range data {
		arr[i] = lunexruntime.NumberVal(float64(b))
	}
	return lunexruntime.ArrayVal(arr)
}

func ffiReadBytes(value *lunexruntime.Value, length, offset int) ([]byte, error) {
	ptrValue, err := ffiPointerFromValue(value)
	if err != nil {
		return nil, err
	}
	ptr, ok := ffiPointerOf(ptrValue)
	if !ok {
		return nil, ffiError("E0131", "invalid FFI pointer")
	}
	if length < 0 || offset < 0 {
		return nil, ffiError("E0131", "negative native memory range")
	}
	if ptr.bounded {
		capLen := ptr.capacity()
		if offset > capLen || length > capLen-offset {
			return nil, ffiError("E0132", "native memory read exceeds the known pointer length")
		}
	}
	addr := ptr.addressAt(offset)
	if length == 0 {
		return []byte{}, nil
	}
	if addr == 0 {
		return nil, ffiError("E0132", "cannot read through a null pointer")
	}
	data := unsafe.Slice((*byte)(unsafe.Pointer(addr)), length)
	return append([]byte(nil), data...), nil
}

func ffiWriteBytes(value *lunexruntime.Value, data []byte, offset int) error {
	ptrValue, err := ffiPointerFromValue(value)
	if err != nil {
		return err
	}
	ptr, ok := ffiPointerOf(ptrValue)
	if !ok {
		return ffiError("E0131", "invalid FFI pointer")
	}
	if offset < 0 {
		return ffiError("E0131", "negative native memory offset")
	}
	if ptr.bounded {
		capLen := ptr.capacity()
		if offset > capLen || len(data) > capLen-offset {
			return ffiError("E0132", "native memory write exceeds the known pointer length")
		}
	}
	if len(data) == 0 {
		return nil
	}
	addr := ptr.addressAt(offset)
	if addr == 0 {
		return ffiError("E0132", "cannot write through a null pointer")
	}
	return copyBytesToNative(addr, data)
}

func ffiReadValue(value *lunexruntime.Value, typ ffiType, offset int) (*lunexruntime.Value, error) {
	ptrValue, err := ffiPointerFromValue(value)
	if err != nil {
		return nil, err
	}
	ptr, ok := ffiPointerOf(ptrValue)
	if !ok {
		return nil, ffiError("E0131", "invalid FFI pointer")
	}
	if offset < 0 {
		return nil, ffiError("E0131", "negative native memory offset")
	}
	if typ.kind == "void" {
		return lunexruntime.Undefined, nil
	}
	size := int(typ.goType.Size())
	if ptr.bounded {
		capLen := ptr.capacity()
		if offset > capLen || size > capLen-offset {
			return nil, ffiError("E0132", "native memory read exceeds the known pointer length")
		}
	}
	addr := ptr.addressAt(offset)
	if size != 0 && addr == 0 {
		return nil, ffiError("E0132", "cannot read through a null pointer")
	}
	if typ.kind == "cstring" {
		return lunexruntime.StringVal(ffiCString(ptr, 1<<20)), nil
	}
	valueAt := reflect.NewAt(typ.goType, unsafe.Pointer(addr)).Elem()
	return ffiFromReflect(valueAt, typ)
}

func ffiWriteValue(target *lunexruntime.Value, typ ffiType, source *lunexruntime.Value, offset int) error {
	ptrValue, err := ffiPointerFromValue(target)
	if err != nil {
		return err
	}
	ptr, ok := ffiPointerOf(ptrValue)
	if !ok {
		return ffiError("E0131", "invalid FFI pointer")
	}
	if offset < 0 {
		return ffiError("E0131", "negative native memory offset")
	}
	if typ.kind == "void" {
		return nil
	}
	size := int(typ.goType.Size())
	if ptr.bounded {
		capLen := ptr.capacity()
		if offset > capLen || size > capLen-offset {
			return ffiError("E0132", "native memory write exceeds the known pointer length")
		}
	}
	addr := ptr.addressAt(offset)
	if size != 0 && addr == 0 {
		return ffiError("E0132", "cannot write through a null pointer")
	}
	if typ.kind == "cstring" && source != nil && source.Tag == lunexruntime.TypeString {
		return ffiError("E0131", "writing a cstring field requires an FFI pointer; use cstring() to allocate storage")
	}
	rv, temps, err := ffiToReflect(source, typ)
	if err != nil {
		return err
	}
	defer func() {
		for _, temp := range temps {
			_ = ffiFree(temp)
		}
	}()
	if typ.kind == "cstring" || typ.kind == "pointer" {
		value, err := reflectUintptr(rv)
		if err != nil {
			return err
		}
		*(*uintptr)(unsafe.Pointer(addr)) = value
		return nil
	}
	valueAt := reflect.NewAt(typ.goType, unsafe.Pointer(addr)).Elem()
	valueAt.Set(rv)
	return nil
}

func ffiCString(ptr *ffiPointer, maxLen int) string {
	if ptr == nil || maxLen <= 0 {
		return ""
	}
	if ptr.bounded {
		if capLen := ptr.capacity(); capLen < maxLen {
			maxLen = capLen
		}
	}
	addr := ptr.addressAt(0)
	if addr == 0 || maxLen <= 0 {
		return ""
	}
	data := unsafe.Slice((*byte)(unsafe.Pointer(addr)), maxLen)
	end := len(data)
	for i, b := range data {
		if b == 0 {
			end = i
			break
		}
	}
	return string(data[:end])
}

func readNativeCString(addr uintptr, maxLen int) string {
	if addr == 0 || maxLen <= 0 {
		return ""
	}
	data := unsafe.Slice((*byte)(unsafe.Pointer(addr)), maxLen)
	end := len(data)
	for i, b := range data {
		if b == 0 {
			end = i
			break
		}
	}
	return string(data[:end])
}

func ffiTypeSize(typ ffiType) uintptr {
	if typ.kind == "void" {
		return 0
	}
	return typ.goType.Size()
}

func ffiTypeAlign(typ ffiType) uintptr {
	if typ.kind == "void" {
		return 1
	}
	return uintptr(typ.goType.Align())
}

func ffiTypeInfo(typ ffiType) *lunexruntime.Value {
	obj := lunexruntime.ObjectVal(map[string]*lunexruntime.Value{
		"type":      lunexruntime.StringVal(typ.token),
		"kind":      lunexruntime.StringVal(typ.kind),
		"size":      lunexruntime.NumberVal(float64(ffiTypeSize(typ))),
		"align":     lunexruntime.NumberVal(float64(ffiTypeAlign(typ))),
		"alignment": lunexruntime.NumberVal(float64(ffiTypeAlign(typ))),
		"pointer":   lunexruntime.BoolVal(typ.kind == "pointer" || typ.kind == "cstring"),
	})
	if typ.pointee != nil {
		obj.ObjVal["pointee"] = ffiTypeInfo(*typ.pointee)
	}
	if typ.elem != nil {
		obj.ObjVal["element"] = ffiTypeInfo(*typ.elem)
		obj.ObjVal["count"] = lunexruntime.NumberVal(float64(typ.count))
	}
	if len(typ.fields) > 0 {
		fields := make([]*lunexruntime.Value, len(typ.fields))
		for i, field := range typ.fields {
			fields[i] = lunexruntime.ObjectVal(map[string]*lunexruntime.Value{
				"name": lunexruntime.StringVal(field.name),
				"type": ffiTypeInfo(field.typ),
			})
		}
		obj.ObjVal["fields"] = lunexruntime.ArrayVal(fields)
	}
	return obj
}

func ffiCreateCallback(interp *lunexruntime.Interpreter, signatureValue, handler *lunexruntime.Value) (*lunexruntime.Value, error) {
	if handler == nil || handler.Tag != lunexruntime.TypeFunction {
		return nil, ffiError("E0125", "FFI callback handler must be a Lunex function")
	}
	sig, err := parseFFISignature(signatureValue)
	if err != nil {
		return nil, err
	}
	if sig.returns.kind == "struct" || sig.returns.kind == "array" || sig.returns.kind == "cstring" || (sig.returns.kind == "scalar" && sig.returns.goType.Kind() == reflect.String) || sig.returns.goType.Size() > unsafe.Sizeof(uintptr(0)) {
		return nil, ffiError("E0129", "FFI callback return values must be void or fit in one machine word; string and cstring returns are not supported")
	}
	for i, arg := range sig.args {
		if arg.goType.Size() > unsafe.Sizeof(uintptr(0)) {
			return nil, ffiError("E0129", fmt.Sprintf("FFI callback argument %d is larger than one machine word", i))
		}
	}
	fnType, err := ffiGoFuncType(sig)
	if err != nil {
		return nil, err
	}
	cb := &ffiCallback{sig: sig, handler: handler}
	tramp := reflect.MakeFunc(fnType, func(in []reflect.Value) []reflect.Value {
		cb.mu.Lock()
		closed := cb.closed
		callbackHandler := cb.handler
		cb.mu.Unlock()
		if closed {
			return ffiCallbackZeroResults(sig)
		}
		values := make([]*lunexruntime.Value, len(in))
		for i, input := range in {
			v, convErr := ffiFromReflect(input, sig.args[i])
			if convErr != nil {
				cb.mu.Lock()
				cb.lastErr = convErr.Error()
				cb.mu.Unlock()
				return ffiCallbackZeroResults(sig)
			}
			values[i] = v
		}
		result, callErr := interp.CallValue(callbackHandler, values...)
		if callErr != nil {
			cb.mu.Lock()
			cb.lastErr = callErr.Error()
			cb.mu.Unlock()
			return ffiCallbackZeroResults(sig)
		}
		return ffiCallbackResults(result, sig)
	})
	address, err := ffiNewCallback(tramp.Interface())
	if err != nil {
		return nil, ffiBackendError("E0129", "failed to create native callback", err)
	}
	cb.address = address
	cb.tramp = tramp
	return ffiPointerValue(&ffiPointer{address: address, callback: cb}), nil
}

func ffiCloseCallback(ptr *ffiPointer) (*lunexruntime.Value, error) {
	if ptr == nil || ptr.callback == nil {
		return lunexruntime.False, nil
	}
	cb := ptr.callback
	cb.mu.Lock()
	if cb.closed {
		cb.mu.Unlock()
		return lunexruntime.False, nil
	}
	cb.closed = true
	cb.handler = nil
	cb.mu.Unlock()
	return lunexruntime.True, nil
}

func ffiCallbackZeroResults(sig ffiSignature) []reflect.Value {
	if sig.returns.kind == "void" {
		return nil
	}
	return []reflect.Value{reflect.Zero(sig.returns.goType)}
}

func ffiCallbackResults(result *lunexruntime.Value, sig ffiSignature) []reflect.Value {
	if sig.returns.kind == "void" {
		return nil
	}
	value, _, err := ffiToReflect(result, sig.returns)
	if err != nil {
		return []reflect.Value{reflect.Zero(sig.returns.goType)}
	}
	return []reflect.Value{value}
}
