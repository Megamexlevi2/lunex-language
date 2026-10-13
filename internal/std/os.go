package std

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	gort "runtime"
	"strings"
	"time"

	rt "lunex/internal/runtime"
)

var (
	goOS   = gort.GOOS
	goArch = gort.GOARCH
	goCPUs = gort.NumCPU()
)

func findExecutableInPath(name string) (string, error) {
	pathEnv := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if info.Mode().IsDir() {
			continue
		}
		if info.Mode().Perm()&0111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("exec: %q: executable file not found in $PATH", name)
}

func OsModule() *rt.Value {
	obj := rt.ObjectVal(map[string]*rt.Value{})

	obj.ObjVal["exec"] = rt.FuncVal(&rt.Function{
		Name: "exec",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if isWasmRuntime() {
				return rt.Undefined, fmt.Errorf("os.exec does not exist")
			}
			if len(args) == 0 {
				return rt.Null, nil
			}
			if args[0].Tag != rt.TypeString {
				return rt.Null, fmt.Errorf("os.exec: command must be a string")
			}
			parts := strings.Fields(args[0].StrVal)
			if len(parts) == 0 {
				return rt.Null, nil
			}
			var envExtra []string
			var cwd string
			var timeoutMs int
			if len(args) > 1 && args[1].Tag == rt.TypeObject {
				if v, ok := args[1].ObjVal["cwd"]; ok {
					cwd = v.ToString()
				}
				if v, ok := args[1].ObjVal["env"]; ok && v.Tag == rt.TypeObject {
					for k, val := range v.ObjVal {
						envExtra = append(envExtra, k+"="+val.ToString())
					}
				}
				if v, ok := args[1].ObjVal["timeout"]; ok {
					timeoutMs = int(v.ToNumber())
				}
			}
			resolved := parts[0]
			if !strings.Contains(resolved, "/") {
				if found, findErr := findExecutableInPath(resolved); findErr == nil {
					resolved = found
				} else {
					return rt.ObjectVal(map[string]*rt.Value{
						"stdout": rt.StringVal(""),
						"stderr": rt.StringVal(fmt.Sprintf("os.exec: %v", findErr)),
						"code":   rt.NumberVal(float64(127)),
						"ok":     rt.BoolVal(false),
					}), nil
				}
			}
			cmd := exec.Command(resolved, parts[1:]...)
			if cwd != "" {
				cmd.Dir = cwd
			}
			if len(envExtra) > 0 {
				cmd.Env = append(os.Environ(), envExtra...)
			}
			var outBuf, errBuf strings.Builder
			cmd.Stdout = &outBuf
			cmd.Stderr = &errBuf
			var runErr error
			if timeoutMs > 0 {
				done := make(chan error, 1)
				go func() { done <- cmd.Run() }()
				select {
				case runErr = <-done:
				case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
					if cmd.Process != nil {
						cmd.Process.Kill()
					}
					runErr = fmt.Errorf("timeout after %dms", timeoutMs)
				}
			} else {
				runErr = cmd.Run()
			}
			exitCode := 0
			stderrText := strings.TrimRight(errBuf.String(), "\n")
			if runErr != nil {
				if exitErr, ok := runErr.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					exitCode = 1
					if stderrText == "" {
						stderrText = runErr.Error()
					}
				}
			}
			return rt.ObjectVal(map[string]*rt.Value{
				"stdout": rt.StringVal(strings.TrimRight(outBuf.String(), "\n")),
				"stderr": rt.StringVal(stderrText),
				"code":   rt.NumberVal(float64(exitCode)),
				"ok":     rt.BoolVal(exitCode == 0),
			}), nil
		},
	})

	obj.ObjVal["execSync"] = obj.ObjVal["exec"]

	obj.ObjVal["spawn"] = rt.FuncVal(&rt.Function{
		Name: "spawn",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if isWasmRuntime() {
				return rt.Undefined, fmt.Errorf("os.spawn does not exist")
			}
			if len(args) == 0 {
				return rt.Null, nil
			}
			if args[0].Tag != rt.TypeString {
				return rt.Null, fmt.Errorf("os.spawn: command must be a string")
			}
			parts := strings.Fields(args[0].StrVal)
			if len(parts) == 0 {
				return rt.Null, nil
			}
			resolved := parts[0]
			if !strings.Contains(resolved, "/") {
				if found, findErr := findExecutableInPath(resolved); findErr == nil {
					resolved = found
				} else {
					return rt.Null, fmt.Errorf("os.spawn: %v", findErr)
				}
			}
			cmd := exec.Command(resolved, parts[1:]...)
			if len(args) > 1 && args[1].Tag == rt.TypeObject {
				if v, ok := args[1].ObjVal["cwd"]; ok {
					cmd.Dir = v.ToString()
				}
			}
			if err := cmd.Start(); err != nil {
				return rt.Null, fmt.Errorf("os.spawn: %v", err)
			}
			pid := cmd.Process.Pid
			return rt.ObjectVal(map[string]*rt.Value{
				"pid": rt.NumberVal(float64(pid)),
				"wait": rt.FuncVal(&rt.Function{
					Name: "wait",
					Native: func(a []*rt.Value, t *rt.Value) (*rt.Value, error) {
						state, err := cmd.Process.Wait()
						code := 0
						if err != nil {
							code = 1
						} else {
							code = state.ExitCode()
						}
						return rt.NumberVal(float64(code)), nil
					},
				}),
				"kill": rt.FuncVal(&rt.Function{
					Name: "kill",
					Native: func(a []*rt.Value, t *rt.Value) (*rt.Value, error) {
						if cmd.Process != nil {
							cmd.Process.Kill()
						}
						return rt.Undefined, nil
					},
				}),
			}), nil
		},
	})

	obj.ObjVal["getenv"] = rt.FuncVal(&rt.Function{
		Name: "getenv",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if len(args) == 0 {
				return rt.StringVal(""), nil
			}
			return rt.StringVal(os.Getenv(args[0].ToString())), nil
		},
	})

	obj.ObjVal["setenv"] = rt.FuncVal(&rt.Function{
		Name: "setenv",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if len(args) < 2 {
				return rt.Undefined, nil
			}
			return rt.Undefined, os.Setenv(args[0].ToString(), args[1].ToString())
		},
	})

	obj.ObjVal["unsetenv"] = rt.FuncVal(&rt.Function{
		Name: "unsetenv",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if len(args) == 0 {
				return rt.Undefined, nil
			}
			return rt.Undefined, os.Unsetenv(args[0].ToString())
		},
	})

	obj.ObjVal["environ"] = rt.FuncVal(&rt.Function{
		Name: "environ",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			envObj := rt.ObjectVal(map[string]*rt.Value{})
			for _, e := range os.Environ() {
				parts := strings.SplitN(e, "=", 2)
				if len(parts) == 2 {
					envObj.ObjVal[parts[0]] = rt.StringVal(parts[1])
				}
			}
			return envObj, nil
		},
	})

	obj.ObjVal["getpid"] = rt.FuncVal(&rt.Function{
		Name: "getpid",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			return rt.NumberVal(float64(os.Getpid())), nil
		},
	})

	obj.ObjVal["getppid"] = rt.FuncVal(&rt.Function{
		Name: "getppid",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			return rt.NumberVal(float64(os.Getppid())), nil
		},
	})

	obj.ObjVal["getcwd"] = rt.FuncVal(&rt.Function{
		Name: "getcwd",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			cwd, err := os.Getwd()
			if err != nil {
				return rt.StringVal(""), err
			}
			return rt.StringVal(cwd), nil
		},
	})

	obj.ObjVal["chdir"] = rt.FuncVal(&rt.Function{
		Name: "chdir",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if len(args) == 0 {
				return rt.Undefined, nil
			}
			return rt.Undefined, os.Chdir(args[0].ToString())
		},
	})

	obj.ObjVal["hostname"] = rt.FuncVal(&rt.Function{
		Name: "hostname",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			h, err := os.Hostname()
			if err != nil {
				return rt.StringVal(""), nil
			}
			return rt.StringVal(h), nil
		},
	})

	obj.ObjVal["platform"] = rt.FuncVal(&rt.Function{
		Name: "platform",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			return rt.StringVal(goOS), nil
		},
	})

	obj.ObjVal["arch"] = rt.FuncVal(&rt.Function{
		Name: "arch",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			return rt.StringVal(goArch), nil
		},
	})

	obj.ObjVal["cpus"] = rt.FuncVal(&rt.Function{
		Name: "cpus",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			return rt.NumberVal(float64(goCPUs)), nil
		},
	})

	obj.ObjVal["exit"] = rt.FuncVal(&rt.Function{
		Name: "exit",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if isWasmRuntime() {
				return rt.Undefined, fmt.Errorf("os.exit does not exist")
			}
			code := 0
			if len(args) > 0 {
				code = int(args[0].ToNumber())
			}
			os.Exit(code)
			return rt.Undefined, nil
		},
	})

	obj.ObjVal["args"] = rt.FuncVal(&rt.Function{
		Name: "args",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			arr := make([]*rt.Value, len(os.Args))
			for i, a := range os.Args {
				arr[i] = rt.StringVal(a)
			}
			return rt.ArrayVal(arr), nil
		},
	})

	obj.ObjVal["stat"] = rt.FuncVal(&rt.Function{
		Name: "stat",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if len(args) == 0 {
				return rt.Null, nil
			}
			info, err := os.Stat(args[0].ToString())
			if err != nil {
				return rt.Null, nil
			}
			return rt.ObjectVal(map[string]*rt.Value{
				"name":    rt.StringVal(info.Name()),
				"size":    rt.NumberVal(float64(info.Size())),
				"isDir":   rt.BoolVal(info.IsDir()),
				"isFile":  rt.BoolVal(!info.IsDir()),
				"mode":    rt.StringVal(info.Mode().String()),
				"modTime": rt.StringVal(info.ModTime().Format(time.RFC3339)),
			}), nil
		},
	})

	obj.ObjVal["exists"] = rt.FuncVal(&rt.Function{
		Name: "exists",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if len(args) == 0 {
				return rt.False, nil
			}
			_, err := os.Stat(args[0].ToString())
			return rt.BoolVal(!os.IsNotExist(err)), nil
		},
	})

	obj.ObjVal["mkdir"] = rt.FuncVal(&rt.Function{
		Name: "mkdir",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if len(args) == 0 {
				return rt.False, nil
			}
			recursive := len(args) > 1 && args[1].Tag == rt.TypeBool && args[1].BoolVal
			var err error
			if recursive {
				err = os.MkdirAll(args[0].ToString(), 0755)
			} else {
				err = os.Mkdir(args[0].ToString(), 0755)
			}
			return rt.BoolVal(err == nil), nil
		},
	})

	obj.ObjVal["remove"] = rt.FuncVal(&rt.Function{
		Name: "remove",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if len(args) == 0 {
				return rt.False, nil
			}
			recursive := len(args) > 1 && args[1].Tag == rt.TypeBool && args[1].BoolVal
			var err error
			if recursive {
				err = os.RemoveAll(args[0].ToString())
			} else {
				err = os.Remove(args[0].ToString())
			}
			return rt.BoolVal(err == nil), nil
		},
	})

	obj.ObjVal["rename"] = rt.FuncVal(&rt.Function{
		Name: "rename",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if len(args) < 2 {
				return rt.False, nil
			}
			return rt.BoolVal(os.Rename(args[0].ToString(), args[1].ToString()) == nil), nil
		},
	})

	obj.ObjVal["listDir"] = rt.FuncVal(&rt.Function{
		Name: "listDir",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			dir := "."
			if len(args) > 0 {
				dir = args[0].ToString()
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				return rt.ArrayVal([]*rt.Value{}), nil
			}
			items := make([]*rt.Value, 0, len(entries))
			for _, e := range entries {
				info, _ := e.Info()
				size := int64(0)
				modTime := ""
				if info != nil {
					size = info.Size()
					modTime = info.ModTime().Format(time.RFC3339)
				}
				items = append(items, rt.ObjectVal(map[string]*rt.Value{
					"name":    rt.StringVal(e.Name()),
					"isDir":   rt.BoolVal(e.IsDir()),
					"isFile":  rt.BoolVal(!e.IsDir()),
					"size":    rt.NumberVal(float64(size)),
					"modTime": rt.StringVal(modTime),
				}))
			}
			return rt.ArrayVal(items), nil
		},
	})

	obj.ObjVal["glob"] = rt.FuncVal(&rt.Function{
		Name: "glob",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if len(args) == 0 {
				return rt.ArrayVal([]*rt.Value{}), nil
			}
			matches, err := filepath.Glob(args[0].ToString())
			if err != nil {
				return rt.ArrayVal([]*rt.Value{}), nil
			}
			items := make([]*rt.Value, len(matches))
			for i, m := range matches {
				items[i] = rt.StringVal(m)
			}
			return rt.ArrayVal(items), nil
		},
	})

	obj.ObjVal["tempDir"] = rt.FuncVal(&rt.Function{
		Name: "tempDir",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			return rt.StringVal(os.TempDir()), nil
		},
	})

	obj.ObjVal["tempFile"] = rt.FuncVal(&rt.Function{
		Name: "tempFile",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			prefix := "lunex-"
			if len(args) > 0 {
				prefix = args[0].ToString()
			}
			f, err := os.CreateTemp("", prefix)
			if err != nil {
				return rt.Null, nil
			}
			name := f.Name()
			f.Close()
			return rt.StringVal(name), nil
		},
	})

	obj.ObjVal["expandEnv"] = rt.FuncVal(&rt.Function{
		Name: "expandEnv",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if len(args) == 0 {
				return rt.StringVal(""), nil
			}
			return rt.StringVal(os.ExpandEnv(args[0].ToString())), nil
		},
	})

	obj.ObjVal["join"] = rt.FuncVal(&rt.Function{
		Name: "join",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			parts := make([]string, len(args))
			for i, a := range args {
				parts[i] = a.ToString()
			}
			return rt.StringVal(filepath.Join(parts...)), nil
		},
	})

	obj.ObjVal["dirname"] = rt.FuncVal(&rt.Function{
		Name: "dirname",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if len(args) == 0 {
				return rt.StringVal("."), nil
			}
			return rt.StringVal(filepath.Dir(args[0].ToString())), nil
		},
	})

	obj.ObjVal["basename"] = rt.FuncVal(&rt.Function{
		Name: "basename",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if len(args) == 0 {
				return rt.StringVal(""), nil
			}
			return rt.StringVal(filepath.Base(args[0].ToString())), nil
		},
	})

	obj.ObjVal["extname"] = rt.FuncVal(&rt.Function{
		Name: "extname",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if len(args) == 0 {
				return rt.StringVal(""), nil
			}
			return rt.StringVal(filepath.Ext(args[0].ToString())), nil
		},
	})

	obj.ObjVal["abs"] = rt.FuncVal(&rt.Function{
		Name: "abs",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if len(args) == 0 {
				return rt.StringVal(""), nil
			}
			abs, err := filepath.Abs(args[0].ToString())
			if err != nil {
				return rt.StringVal(args[0].ToString()), nil
			}
			return rt.StringVal(abs), nil
		},
	})

	homeDir, _ := os.UserHomeDir()

	obj.ObjVal["sep"] = rt.StringVal(string(filepath.Separator))
	obj.ObjVal["pathSep"] = rt.StringVal(string(os.PathListSeparator))
	obj.ObjVal["eol"] = rt.StringVal("\n")
	obj.ObjVal["homeDir"] = rt.StringVal(homeDir)

	obj.ObjVal["pid"] = obj.ObjVal["getpid"]
	obj.ObjVal["ppid"] = obj.ObjVal["getppid"]
	obj.ObjVal["cwd"] = obj.ObjVal["getcwd"]

	obj.ObjVal["time"] = rt.FuncVal(&rt.Function{
		Name: "time",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			return rt.NumberVal(float64(time.Now().UnixMilli())), nil
		},
	})

	obj.ObjVal["hrtime"] = rt.FuncVal(&rt.Function{
		Name: "hrtime",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			return rt.NumberVal(float64(time.Now().UnixNano()) / 1e6), nil
		},
	})

	obj.ObjVal["sleep"] = rt.FuncVal(&rt.Function{
		Name: "sleep",
		Native: func(args []*rt.Value, this *rt.Value) (*rt.Value, error) {
			if len(args) > 0 {
				ms := args[0].ToNumber()
				time.Sleep(time.Duration(ms) * time.Millisecond)
			}
			return rt.Undefined, nil
		},
	})

	return obj
}
