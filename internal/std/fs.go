//go:build !js && !wasip1

package std

import (
	"encoding/json"
	"io"
	"lunex/internal/runtime"
	shared "lunex/internal/std/shared"
	"os"
	"path/filepath"
	"strings"
)

func FsModule() *runtime.Value {
	return runtime.ObjectVal(map[string]*runtime.Value{
		"readFile": runtime.FuncVal(&runtime.Function{Name: "readFile", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.Null, nil
			}
			data, err := os.ReadFile(args[0].ToString())
			if err != nil {
				return runtime.Null, nil
			}
			return runtime.StringVal(string(data)), nil
		}}),

		"writeFile": runtime.FuncVal(&runtime.Function{Name: "writeFile", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.False, nil
			}
			err := os.WriteFile(args[0].ToString(), []byte(args[1].ToString()), 0644)
			return runtime.BoolVal(err == nil), nil
		}}),

		"appendFile": runtime.FuncVal(&runtime.Function{Name: "appendFile", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.False, nil
			}
			f, err := os.OpenFile(args[0].ToString(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return runtime.False, nil
			}
			defer f.Close()
			_, err = f.WriteString(args[1].ToString())
			return runtime.BoolVal(err == nil), nil
		}}),

		"delete": runtime.FuncVal(&runtime.Function{Name: "delete", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			err := os.Remove(args[0].ToString())
			return runtime.BoolVal(err == nil), nil
		}}),

		"deleteAll": runtime.FuncVal(&runtime.Function{Name: "deleteAll", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			err := os.RemoveAll(args[0].ToString())
			return runtime.BoolVal(err == nil), nil
		}}),

		"exists": runtime.FuncVal(&runtime.Function{Name: "exists", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			_, err := os.Stat(args[0].ToString())
			return runtime.BoolVal(err == nil), nil
		}}),

		"mkdir": runtime.FuncVal(&runtime.Function{Name: "mkdir", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			recursive := true
			if len(args) > 1 && args[1].Tag == runtime.TypeBool {
				recursive = args[1].BoolVal
			}
			var err error
			if recursive {
				err = os.MkdirAll(args[0].ToString(), 0755)
			} else {
				err = os.Mkdir(args[0].ToString(), 0755)
			}
			return runtime.BoolVal(err == nil), nil
		}}),

		"rmdir": runtime.FuncVal(&runtime.Function{Name: "rmdir", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			recursive := false
			if len(args) > 1 && args[1].Tag == runtime.TypeBool {
				recursive = args[1].BoolVal
			}
			var err error
			if recursive {
				err = os.RemoveAll(args[0].ToString())
			} else {
				err = os.Remove(args[0].ToString())
			}
			return runtime.BoolVal(err == nil), nil
		}}),

		"rename": runtime.FuncVal(&runtime.Function{Name: "rename", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.False, nil
			}
			err := os.Rename(args[0].ToString(), args[1].ToString())
			return runtime.BoolVal(err == nil), nil
		}}),

		"moveFile": runtime.FuncVal(&runtime.Function{Name: "moveFile", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.False, nil
			}
			err := os.Rename(args[0].ToString(), args[1].ToString())
			return runtime.BoolVal(err == nil), nil
		}}),

		"copy": runtime.FuncVal(&runtime.Function{Name: "copy", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.False, nil
			}
			src, err := os.Open(args[0].ToString())
			if err != nil {
				return runtime.False, nil
			}
			defer src.Close()
			dst, err := os.Create(args[1].ToString())
			if err != nil {
				return runtime.False, nil
			}
			defer dst.Close()
			_, err = io.Copy(dst, src)
			return runtime.BoolVal(err == nil), nil
		}}),

		"copyFile": runtime.FuncVal(&runtime.Function{Name: "copyFile", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.False, nil
			}
			src, err := os.Open(args[0].ToString())
			if err != nil {
				return runtime.False, nil
			}
			defer src.Close()

			srcInfo, err := src.Stat()
			if err != nil {
				return runtime.False, nil
			}
			dst, err := os.OpenFile(args[1].ToString(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
			if err != nil {
				return runtime.False, nil
			}
			defer dst.Close()
			_, err = io.Copy(dst, src)
			return runtime.BoolVal(err == nil), nil
		}}),

		"list": runtime.FuncVal(&runtime.Function{Name: "list", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			path := "."
			if len(args) > 0 {
				path = args[0].ToString()
			}
			entries, err := os.ReadDir(path)
			if err != nil {
				return runtime.ArrayVal(nil), nil
			}
			out := make([]*runtime.Value, 0, len(entries))
			for _, e := range entries {
				info, _ := e.Info()
				size := int64(0)
				if info != nil {
					size = info.Size()
				}
				out = append(out, runtime.ObjectVal(map[string]*runtime.Value{
					"name":   runtime.StringVal(e.Name()),
					"isDir":  runtime.BoolVal(e.IsDir()),
					"isFile": runtime.BoolVal(!e.IsDir()),
					"size":   runtime.NumberVal(float64(size)),
					"path":   runtime.StringVal(filepath.Join(path, e.Name())),
				}))
			}
			return runtime.ArrayVal(out), nil
		}}),

		"readDir": runtime.FuncVal(&runtime.Function{Name: "readDir", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			path := "."
			if len(args) > 0 {
				path = args[0].ToString()
			}
			entries, err := os.ReadDir(path)
			if err != nil {
				return runtime.ArrayVal(nil), nil
			}
			out := make([]*runtime.Value, 0, len(entries))
			for _, e := range entries {
				info, _ := e.Info()
				size := int64(0)
				if info != nil {
					size = info.Size()
				}
				out = append(out, runtime.ObjectVal(map[string]*runtime.Value{
					"name":   runtime.StringVal(e.Name()),
					"isDir":  runtime.BoolVal(e.IsDir()),
					"isFile": runtime.BoolVal(!e.IsDir()),
					"size":   runtime.NumberVal(float64(size)),
					"path":   runtime.StringVal(filepath.Join(path, e.Name())),
				}))
			}
			return runtime.ArrayVal(out), nil
		}}),

		"stat": runtime.FuncVal(&runtime.Function{Name: "stat", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.Null, nil
			}
			info, err := os.Stat(args[0].ToString())
			if err != nil {
				return runtime.Null, nil
			}
			return runtime.ObjectVal(map[string]*runtime.Value{
				"name":    runtime.StringVal(info.Name()),
				"size":    runtime.NumberVal(float64(info.Size())),
				"isDir":   runtime.BoolVal(info.IsDir()),
				"isFile":  runtime.BoolVal(!info.IsDir()),
				"mode":    runtime.StringVal(info.Mode().String()),
				"modTime": runtime.NumberVal(float64(info.ModTime().UnixNano() / 1e6)),
			}), nil
		}}),

		"isDir": runtime.FuncVal(&runtime.Function{Name: "isDir", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			info, err := os.Stat(args[0].ToString())
			if err != nil {
				return runtime.False, nil
			}
			return runtime.BoolVal(info.IsDir()), nil
		}}),

		"isFile": runtime.FuncVal(&runtime.Function{Name: "isFile", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			info, err := os.Stat(args[0].ToString())
			if err != nil {
				return runtime.False, nil
			}
			return runtime.BoolVal(!info.IsDir()), nil
		}}),

		"size": runtime.FuncVal(&runtime.Function{Name: "size", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(-1), nil
			}
			info, err := os.Stat(args[0].ToString())
			if err != nil {
				return runtime.NumberVal(-1), nil
			}
			return runtime.NumberVal(float64(info.Size())), nil
		}}),

		"readLines": runtime.FuncVal(&runtime.Function{Name: "readLines", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.ArrayVal(nil), nil
			}
			data, err := os.ReadFile(args[0].ToString())
			if err != nil {
				return runtime.ArrayVal(nil), nil
			}
			lines := strings.Split(string(data), "\n")
			out := make([]*runtime.Value, len(lines))
			for i, l := range lines {
				out[i] = runtime.StringVal(strings.TrimRight(l, "\r"))
			}
			return runtime.ArrayVal(out), nil
		}}),

		"readJSON": runtime.FuncVal(&runtime.Function{Name: "readJSON", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.Null, nil
			}
			data, err := os.ReadFile(args[0].ToString())
			if err != nil {
				return runtime.Null, nil
			}
			var raw interface{}
			if err := json.Unmarshal(data, &raw); err != nil {
				return runtime.Null, nil
			}
			return shared.JsonToValue(raw), nil
		}}),

		"writeJSON": runtime.FuncVal(&runtime.Function{Name: "writeJSON", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.False, nil
			}
			indent := ""
			if len(args) > 2 {
				n := int(args[2].ToNumber())
				if n > 0 {
					indent = strings.Repeat(" ", n)
				}
			}
			var data []byte
			var err error
			if indent != "" {
				data, err = json.MarshalIndent(shared.ValueToInterface(args[1]), "", indent)
			} else {
				data, err = json.Marshal(shared.ValueToInterface(args[1]))
			}
			if err != nil {
				return runtime.False, nil
			}
			err = os.WriteFile(args[0].ToString(), data, 0644)
			return runtime.BoolVal(err == nil), nil
		}}),

		"readCSV": runtime.FuncVal(&runtime.Function{Name: "readCSV", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.ArrayVal(nil), nil
			}
			data, err := os.ReadFile(args[0].ToString())
			if err != nil {
				return runtime.ArrayVal(nil), nil
			}
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) == 0 {
				return runtime.ArrayVal(nil), nil
			}
			sep := ","
			headers := parseCSVLine(lines[0], sep)
			var rows []*runtime.Value
			for _, line := range lines[1:] {
				if strings.TrimSpace(line) == "" {
					continue
				}
				values := parseCSVLine(line, sep)
				row := make(map[string]*runtime.Value)
				for i, h := range headers {
					if i < len(values) {
						row[h] = runtime.StringVal(values[i])
					} else {
						row[h] = runtime.StringVal("")
					}
				}
				rows = append(rows, runtime.ObjectVal(row))
			}
			return runtime.ArrayVal(rows), nil
		}}),

		"writeCSV": runtime.FuncVal(&runtime.Function{Name: "writeCSV", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 || args[1].Tag != runtime.TypeArray {
				return runtime.False, nil
			}
			var lines []string
			if len(args[1].ArrVal) > 0 && args[1].ArrVal[0].Tag == runtime.TypeObject {
				var headers []string
				for k := range args[1].ArrVal[0].ObjVal {
					headers = append(headers, k)
				}
				lines = append(lines, strings.Join(headers, ","))
				for _, row := range args[1].ArrVal {
					var values []string
					for _, h := range headers {
						v := ""
						if row.Tag == runtime.TypeObject {
							if val, ok := row.ObjVal[h]; ok {
								v = val.ToString()
							}
						}
						if strings.ContainsAny(v, ",\"\n") {
							v = "\"" + strings.ReplaceAll(v, "\"", "\"\"") + "\""
						}
						values = append(values, v)
					}
					lines = append(lines, strings.Join(values, ","))
				}
			}
			err := os.WriteFile(args[0].ToString(), []byte(strings.Join(lines, "\n")), 0644)
			return runtime.BoolVal(err == nil), nil
		}}),

		"tempFile": runtime.FuncVal(&runtime.Function{Name: "tempFile", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			prefix := "lunex-"
			if len(args) > 0 {
				prefix = args[0].ToString()
			}
			f, err := os.CreateTemp("", prefix)
			if err != nil {
				return runtime.Null, nil
			}
			f.Close()
			return runtime.StringVal(f.Name()), nil
		}}),

		"tempDir": runtime.FuncVal(&runtime.Function{Name: "tempDir", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			prefix := "lunex-"
			if len(args) > 0 {
				prefix = args[0].ToString()
			}
			dir, err := os.MkdirTemp("", prefix)
			if err != nil {
				return runtime.Null, nil
			}
			return runtime.StringVal(dir), nil
		}}),

		"abs": runtime.FuncVal(&runtime.Function{Name: "abs", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.StringVal(""), nil
			}
			abs, err := filepath.Abs(args[0].ToString())
			if err != nil {
				return runtime.StringVal(args[0].ToString()), nil
			}
			return runtime.StringVal(abs), nil
		}}),

		"join": runtime.FuncVal(&runtime.Function{Name: "join", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			parts := make([]string, len(args))
			for i, a := range args {
				parts[i] = a.ToString()
			}
			return runtime.StringVal(filepath.Join(parts...)), nil
		}}),

		"dirname": runtime.FuncVal(&runtime.Function{Name: "dirname", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.StringVal("."), nil
			}
			return runtime.StringVal(filepath.Dir(args[0].ToString())), nil
		}}),

		"basename": runtime.FuncVal(&runtime.Function{Name: "basename", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.StringVal(""), nil
			}
			base := filepath.Base(args[0].ToString())
			if len(args) > 1 {
				ext := args[1].ToString()
				base = strings.TrimSuffix(base, ext)
			}
			return runtime.StringVal(base), nil
		}}),

		"extname": runtime.FuncVal(&runtime.Function{Name: "extname", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.StringVal(""), nil
			}
			return runtime.StringVal(filepath.Ext(args[0].ToString())), nil
		}}),

		"glob": runtime.FuncVal(&runtime.Function{Name: "glob", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.ArrayVal(nil), nil
			}
			matches, err := filepath.Glob(args[0].ToString())
			if err != nil {
				return runtime.ArrayVal(nil), nil
			}
			out := make([]*runtime.Value, len(matches))
			for i, m := range matches {
				out[i] = runtime.StringVal(m)
			}
			return runtime.ArrayVal(out), nil
		}}),

		"cwd": runtime.FuncVal(&runtime.Function{Name: "cwd", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			cwd, err := os.Getwd()
			if err != nil {
				return runtime.StringVal("."), nil
			}
			return runtime.StringVal(cwd), nil
		}}),

		"home": runtime.FuncVal(&runtime.Function{Name: "home", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			home, err := os.UserHomeDir()
			if err != nil {
				return runtime.StringVal(""), nil
			}
			return runtime.StringVal(home), nil
		}}),

		"ensureDir": runtime.FuncVal(&runtime.Function{Name: "ensureDir", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			err := os.MkdirAll(args[0].ToString(), 0755)
			return runtime.BoolVal(err == nil), nil
		}}),
	})
}

func parseCSVLine(line, sep string) []string {
	var fields []string
	var current strings.Builder
	inQuote := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == '"' {
			if inQuote && i+1 < len(line) && line[i+1] == '"' {
				current.WriteByte('"')
				i++
			} else {
				inQuote = !inQuote
			}
		} else if !inQuote && string(c) == sep {
			fields = append(fields, current.String())
			current.Reset()
		} else {
			current.WriteByte(c)
		}
	}
	fields = append(fields, current.String())
	return fields
}
