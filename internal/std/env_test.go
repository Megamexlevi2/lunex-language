package std

import (
	"os"
	"path/filepath"
	"testing"

	"lunex/internal/runtime"
)

func callEnv(t *testing.T, module *runtime.Value, name string, args ...*runtime.Value) *runtime.Value {
	t.Helper()
	fn := module.ObjVal[name]
	if fn == nil || fn.Tag != runtime.TypeFunction {
		t.Fatalf("missing env.%s", name)
	}
	value, err := runtime.CallFunction(fn, args)
	if err != nil {
		t.Fatalf("env.%s: %v", name, err)
	}
	return value
}

func TestEnvDotenvCompatibility(t *testing.T) {
	interpreter := runtime.NewInterpreter()
	interpreter.RegisterModule("os", OsModule())
	interpreter.RegisterModule("fs", FsModule())
	module := EnvModule(interpreter)

	if module.ObjVal["delete"] == nil || module.ObjVal["require"] == nil {
		t.Fatal("expected public environment functions")
	}

	source := "# comment\nexport APP_NAME = Lunex\nPORT='3000'\nEMPTY=\nMESSAGE=\"hello\\nworld\" # comment\nMULTI=\"line one\nline two\"\nBACK=`value`\nCOLON: value\nHASH=abc#def\n"
	parsed := callEnv(t, module, "parse", runtime.StringVal(source))
	expected := map[string]string{
		"APP_NAME": "Lunex",
		"PORT":     "3000",
		"EMPTY":    "",
		"MESSAGE":  "hello\nworld",
		"MULTI":    "line one\nline two",
		"BACK":     "value",
		"COLON":    "value",
		"HASH":     "abc",
	}
	for key, want := range expected {
		got := parsed.ObjVal[key]
		if got == nil || got.StrVal != want {
			t.Fatalf("%s=%#v want %q", key, got, want)
		}
	}

	os.Setenv("LUNEX_ENV_TEST", "old")
	defer os.Unsetenv("LUNEX_ENV_TEST")
	if !callEnv(t, module, "set", runtime.StringVal("LUNEX_ENV_TEST"), runtime.StringVal("new")).BoolVal {
		t.Fatal("set returned false")
	}
	if os.Getenv("LUNEX_ENV_TEST") != "new" {
		t.Fatal("set did not update the environment")
	}
	if callEnv(t, module, "set", runtime.StringVal("bad=name"), runtime.StringVal("x")).BoolVal {
		t.Fatal("invalid key was accepted")
	}
	if callEnv(t, module, "set", runtime.Undefined, runtime.StringVal("x")).BoolVal {
		t.Fatal("undefined key was accepted")
	}
	if !callEnv(t, module, "delete", runtime.StringVal("LUNEX_ENV_TEST")).BoolVal {
		t.Fatal("delete returned false")
	}
	if callEnv(t, module, "has", runtime.StringVal("LUNEX_ENV_TEST")).BoolVal {
		t.Fatal("variable still exists")
	}
	if callEnv(t, module, "get", runtime.StringVal("LUNEX_ENV_MISSING"), runtime.StringVal("fallback")).StrVal != "fallback" {
		t.Fatal("default value was not returned")
	}

	os.Setenv("LUNEX_ENV_POP", "first")
	defer os.Unsetenv("LUNEX_ENV_POP")
	values := runtime.ObjectVal(map[string]*runtime.Value{
		"LUNEX_ENV_POP": runtime.StringVal("second"),
		"LUNEX_ENV_NEW": runtime.StringVal("new"),
	})
	populated := callEnv(t, module, "populate", values)
	if populated.ObjVal["LUNEX_ENV_POP"] != nil {
		t.Fatal("populate overwrote an existing variable")
	}
	if populated.ObjVal["LUNEX_ENV_NEW"] == nil || populated.ObjVal["LUNEX_ENV_NEW"].StrVal != "new" {
		t.Fatal("populate did not return applied values")
	}
	override := callEnv(t, module, "populate", values, runtime.True)
	if override.ObjVal["LUNEX_ENV_POP"] == nil || override.ObjVal["LUNEX_ENV_POP"].StrVal != "second" {
		t.Fatal("populate override did not return the applied value")
	}
	if os.Getenv("LUNEX_ENV_POP") != "second" {
		t.Fatal("populate override failed")
	}
	os.Unsetenv("LUNEX_ENV_NEW")

	os.Setenv("LUNEX_ENV_REQ", "")
	defer os.Unsetenv("LUNEX_ENV_REQ")
	if callEnv(t, module, "require", runtime.StringVal("LUNEX_ENV_REQ")).StrVal != "" {
		t.Fatal("empty variable should be accepted by require")
	}
	os.Unsetenv("LUNEX_ENV_REQ")
	if _, err := runtime.CallFunction(module.ObjVal["require"], []*runtime.Value{runtime.StringVal("LUNEX_ENV_REQ")}); err == nil {
		t.Fatal("require did not fail for a missing variable")
	}

	dir := t.TempDir()
	file := filepath.Join(dir, ".env")
	if err := os.WriteFile(file, []byte("FILE_KEY=file\nPORT=1234\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !callEnv(t, module, "load", runtime.StringVal(file)).BoolVal {
		t.Fatal("load returned false")
	}
	if os.Getenv("FILE_KEY") != "file" {
		t.Fatal("load did not populate the file variable")
	}
	os.Unsetenv("FILE_KEY")

	config := callEnv(t, module, "config", runtime.StringVal(filepath.Join(dir, "missing.env")))
	if config.ObjVal["error"] == nil || config.ObjVal["error"].ObjVal["code"].StrVal != "E0111" {
		t.Fatal("config did not return E0111")
	}
}
