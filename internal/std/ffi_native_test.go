package std

import (
	goruntime "runtime"
	"testing"

	"lunex/internal/errfmt"
	"lunex/internal/runtime"
)

func TestFFIStateAndError(t *testing.T) {
	SetFFIEnabled(false)
	if IsFFIEnabled() {
		t.Fatal("FFI should be disabled")
	}
	err := ensureFFIEnabled()
	if err == nil {
		t.Fatal("expected FFI disabled error")
	}
	le, ok := err.(*errfmt.LunexError)
	if !ok {
		t.Fatalf("expected LunexError, got %T", err)
	}
	if le.Code != "E0120" || le.Kind != errfmt.KindPermission {
		t.Fatalf("unexpected FFI error: code=%s kind=%s", le.Code, le.Kind)
	}
	SetFFIEnabled(true)
	if err := ensureFFIEnabled(); err != nil {
		t.Fatal(err)
	}
	SetFFIEnabled(false)
}

func TestFFISignatureParsing(t *testing.T) {
	sig, err := parseFFISignatureString("i32(i32, u8*, f64)")
	if err != nil {
		t.Fatal(err)
	}
	if sig.text != "i32(i32, *u8, f64)" {
		t.Fatalf("unexpected normalized signature: %q", sig.text)
	}
	if len(sig.args) != 3 || sig.args[1].kind != "pointer" || sig.returns.kind != "scalar" {
		t.Fatalf("unexpected parsed signature: %#v", sig)
	}

	structType, err := parseFFIType("struct{x:i32,y:f64}", true)
	if err != nil {
		t.Fatal(err)
	}
	if structType.kind != "struct" || len(structType.fields) != 2 {
		t.Fatalf("unexpected struct type: %#v", structType)
	}
	if structType.fields[0].name != "x" || structType.fields[0].reflectName != "X" {
		t.Fatalf("struct field names were not preserved: %#v", structType.fields[0])
	}
	arrayType, err := parseFFIType("u8[16]", false)
	if err != nil {
		t.Fatal(err)
	}
	if arrayType.kind != "array" || arrayType.count != 16 {
		t.Fatalf("unexpected array type: %#v", arrayType)
	}
	stringType, err := parseFFIType("string", false)
	if err != nil || stringType.kind != "cstring" {
		t.Fatalf("string alias should resolve to cstring: %#v %v", stringType, err)
	}
	cstringArray, err := parseFFIType("cstring[2]", false)
	if err != nil {
		t.Fatal(err)
	}
	if cstringArray.elem == nil || cstringArray.elem.kind != "pointer" {
		t.Fatalf("cstring arrays must use stable pointer elements: %#v", cstringArray)
	}
	value, _, err := scalarToReflect(runtime.StringVal("18446744073709551615"), ffiScalarTypes["u64"])
	if err != nil || value.Uint() != ^uint64(0) {
		t.Fatalf("full-width unsigned integer string conversion failed: %v", err)
	}
}

func TestFFIPointerMarker(t *testing.T) {
	ptr := ffiPointerValue(&ffiPointer{address: 0x1000, length: 8})
	if _, ok := ffiPointerOf(ptr); !ok {
		t.Fatal("pointer marker was not retained")
	}
	if got := ptr.Get("address"); got == nil || got.Tag != runtime.TypeFunction {
		t.Fatal("pointer address accessor missing")
	}
}

func TestFFIAndroidLibrarySelection(t *testing.T) {
	if goruntime.GOOS != "android" {
		t.Setenv("ANDROID_ROOT", "/system")
		t.Setenv("ANDROID_DATA", "/data")
		if !ffiAndroidRuntime() {
			t.Fatal("Android runtime detection failed")
		}
		if got := ffiSystemLibrary(); got != "libc.so" {
			t.Fatalf("unexpected Android system library: %q", got)
		}
		got := ffiLibraryCandidates("libc.so.6")
		if len(got) != 2 || got[1] != "libc.so" {
			t.Fatalf("unexpected Android library candidates: %#v", got)
		}
		return
	}
	if got := ffiSystemLibrary(); got != "libc.so" {
		t.Fatalf("unexpected Android system library: %q", got)
	}
}
