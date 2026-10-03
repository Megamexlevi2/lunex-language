package std

import (
	"lunex/internal/compiler"
	"lunex/internal/runtime"
)

func RegisterAll(c *compiler.Compiler) {
	interp := c.Interpreter()

	runtimeMod := RuntimeModule(interp)
	interp.RegisterModule("runtime", runtimeMod)

	ioMod := IoModule()
	fsMod := FsModule()
	httpMod := HttpModule(interp)
	cryptoMod := CryptoModule()
	dbMod := DbModule()
	wsMod := WsModule()
	utilsMod := UtilsModule()
	jwtMod := JWTModule()
	mathMod := MathModule()
	datetimeMod := DatetimeModule()
	osMod := OsModule()
	regexMod := RegexModule()
	bufferMod := BufferModule()
	intsMod := IntsModule()

	interp.RegisterModule("io", ioMod)
	interp.RegisterModule("fs", fsMod)
	interp.RegisterModule("http", httpMod)
	interp.RegisterModule("crypto", cryptoMod)
	interp.RegisterModule("db", dbMod)
	interp.RegisterModule("ws", wsMod)
	interp.RegisterModule("utils", utilsMod)
	interp.RegisterModule("jwt", jwtMod)
	interp.RegisterModule("math", mathMod)
	interp.RegisterModule("datetime", datetimeMod)
	interp.RegisterModule("os", osMod)
	interp.RegisterModule("regex", regexMod)
	interp.RegisterModule("buffer", bufferMod)
	interp.RegisterModule("ints", intsMod)

	envMod := EnvModule(interp)
	testingMod := TestingModule(interp)
	interp.RegisterModule("env", envMod)
	interp.RegisterModule("testing", testingMod)

	interp.RegisterModule("http/router", HttpRouterModule(interp))
	interp.RegisterModule("http/static", HttpStaticModule(interp))

	jsonMod := JsonModule(interp)
	interp.RegisterModule("json", jsonMod)

	native := NativeModule(interp)
	native.ObjVal["io"] = ioMod
	native.ObjVal["fs"] = fsMod
	native.ObjVal["http"] = httpMod
	native.ObjVal["crypto"] = cryptoMod
	native.ObjVal["db"] = dbMod
	native.ObjVal["env"] = envMod
	native.ObjVal["testing"] = testingMod
	native.ObjVal["ws"] = wsMod
	native.ObjVal["utils"] = utilsMod
	native.ObjVal["json"] = jsonMod
	native.ObjVal["jwt"] = jwtMod
	native.ObjVal["math"] = mathMod
	native.ObjVal["datetime"] = datetimeMod
	native.ObjVal["os"] = osMod
	native.ObjVal["regex"] = regexMod
	native.ObjVal["buffer"] = bufferMod
	native.ObjVal["ints"] = intsMod
	interp.RegisterModule("internal.native", native)
	interp.RegisterModule("native", native)

	ffiMod := FFIModule(interp)
	interp.RegisterModule("ffi", ffiMod)

}

func RegisterLazy(c *compiler.Compiler) {
	interp := c.Interpreter()
	interp.RegisterModule("runtime", RuntimeModule(interp))
	interp.SetModuleLoader(func(name string) (*runtime.Value, bool) {
		switch name {
		case "io":
			return IoModule(), true
		case "fs":
			return FsModule(), true
		case "http":
			return HttpModule(interp), true
		case "http/router":
			return HttpRouterModule(interp), true
		case "http/static":
			return HttpStaticModule(interp), true
		case "crypto":
			return CryptoModule(), true
		case "db":
			return DbModule(), true
		case "ws":
			return WsModule(), true
		case "utils":
			return UtilsModule(), true
		case "json":
			return JsonModule(interp), true
		case "jwt":
			return JWTModule(), true
		case "math":
			return MathModule(), true
		case "datetime":
			return DatetimeModule(), true
		case "os":
			return OsModule(), true
		case "regex":
			return RegexModule(), true
		case "buffer":
			return BufferModule(), true
		case "ints":
			return IntsModule(), true
		case "env":
			return EnvModule(interp), true
		case "testing":
			return TestingModule(interp), true
		case "ffi":
			return FFIModule(interp), true
		case "internal.native", "native":
			return NativeModule(interp), true
		default:
			return nil, false
		}
	})
}
