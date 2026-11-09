//go:build js || wasip1

package std

import (
	"encoding/json"
	"errors"
	"lunex/internal/runtime"
	shared "lunex/internal/std/shared"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var errVfsNotExist = errors.New("file does not exist")

type vfsNode struct {
	isDir   bool
	data    []byte
	mode    string
	modTime time.Time
}

type virtualFS struct {
	mu    sync.RWMutex
	nodes map[string]*vfsNode
	cwd   string
}

var globalVFS = newVirtualFS()

func newVirtualFS() *virtualFS {
	v := &virtualFS{nodes: make(map[string]*vfsNode), cwd: "/"}
	v.nodes["/"] = &vfsNode{isDir: true, mode: "drwxr-xr-x", modTime: time.Now()}
	return v
}

func (v *virtualFS) resolve(p string) string {
	if p == "" {
		p = "."
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(v.cwd, p)
	}
	return filepath.Clean(strings.ReplaceAll(p, "\\", "/"))
}

func (v *virtualFS) ensureParentsLocked(clean string) {
	dir := filepath.Dir(clean)
	for dir != "/" && dir != "." {
		if _, ok := v.nodes[dir]; !ok {
			v.nodes[dir] = &vfsNode{isDir: true, mode: "drwxr-xr-x", modTime: time.Now()}
			fsPersistDir(dir)
		}
		dir = filepath.Dir(dir)
	}
	if _, ok := v.nodes["/"]; !ok {
		v.nodes["/"] = &vfsNode{isDir: true, mode: "drwxr-xr-x", modTime: time.Now()}
	}
}

func (v *virtualFS) writeFile(p string, data []byte) error {
	key := v.resolve(p)
	v.mu.Lock()
	if n, ok := v.nodes[key]; ok && n.isDir {
		v.mu.Unlock()
		return errors.New("is a directory")
	}
	v.ensureParentsLocked(key)
	v.nodes[key] = &vfsNode{isDir: false, data: data, mode: "-rw-r--r--", modTime: time.Now()}
	v.mu.Unlock()
	fsPersistFile(key, data)
	return nil
}

func (v *virtualFS) appendFile(p string, data []byte) error {
	key := v.resolve(p)
	v.mu.Lock()
	existing, ok := v.nodes[key]
	if ok && existing.isDir {
		v.mu.Unlock()
		return errors.New("is a directory")
	}
	v.ensureParentsLocked(key)
	var combined []byte
	if ok {
		combined = append(append([]byte{}, existing.data...), data...)
	} else {
		combined = data
	}
	v.nodes[key] = &vfsNode{isDir: false, data: combined, mode: "-rw-r--r--", modTime: time.Now()}
	v.mu.Unlock()
	fsPersistFile(key, combined)
	return nil
}

func (v *virtualFS) readFile(p string) ([]byte, error) {
	key := v.resolve(p)
	v.mu.RLock()
	defer v.mu.RUnlock()
	n, ok := v.nodes[key]
	if !ok || n.isDir {
		return nil, errVfsNotExist
	}
	return n.data, nil
}

func (v *virtualFS) statNode(p string) (string, *vfsNode, error) {
	key := v.resolve(p)
	v.mu.RLock()
	defer v.mu.RUnlock()
	n, ok := v.nodes[key]
	if !ok {
		return key, nil, errVfsNotExist
	}
	return key, n, nil
}

func (v *virtualFS) remove(p string) error {
	key := v.resolve(p)
	v.mu.Lock()
	n, ok := v.nodes[key]
	if !ok {
		v.mu.Unlock()
		return errVfsNotExist
	}
	if n.isDir {
		for k := range v.nodes {
			if k != key && strings.HasPrefix(k, key+"/") {
				v.mu.Unlock()
				return errors.New("directory not empty")
			}
		}
	}
	delete(v.nodes, key)
	v.mu.Unlock()
	fsPersistDelete(key)
	return nil
}

func (v *virtualFS) removeAll(p string) error {
	key := v.resolve(p)
	prefix := key
	if prefix != "/" {
		prefix += "/"
	}
	v.mu.Lock()
	for k := range v.nodes {
		if k == key || strings.HasPrefix(k, prefix) {
			delete(v.nodes, k)
		}
	}
	v.mu.Unlock()
	fsPersistDeletePrefix(key)
	return nil
}

func (v *virtualFS) mkdir(p string, recursive bool) error {
	key := v.resolve(p)
	v.mu.Lock()
	if !recursive {
		parent := filepath.Dir(key)
		if _, ok := v.nodes[parent]; !ok && parent != "/" {
			v.mu.Unlock()
			return errors.New("parent directory does not exist")
		}
		if _, ok := v.nodes[key]; ok {
			v.mu.Unlock()
			return errors.New("file exists")
		}
	}
	v.ensureParentsLocked(key)
	if _, ok := v.nodes[key]; !ok {
		v.nodes[key] = &vfsNode{isDir: true, mode: "drwxr-xr-x", modTime: time.Now()}
	}
	v.mu.Unlock()
	fsPersistDir(key)
	return nil
}

func (v *virtualFS) rename(oldPath, newPath string) error {
	oldKey := v.resolve(oldPath)
	newKey := v.resolve(newPath)
	v.mu.Lock()
	n, ok := v.nodes[oldKey]
	if !ok {
		v.mu.Unlock()
		return errVfsNotExist
	}
	prefix := oldKey
	if n.isDir {
		prefix += "/"
	}
	moved := map[string]*vfsNode{}
	for k, node := range v.nodes {
		if k == oldKey {
			moved[newKey] = node
			delete(v.nodes, k)
		} else if n.isDir && strings.HasPrefix(k, prefix) {
			rest := strings.TrimPrefix(k, oldKey)
			moved[newKey+rest] = node
			delete(v.nodes, k)
		}
	}
	v.ensureParentsLocked(newKey)
	for k, node := range moved {
		node.modTime = time.Now()
		v.nodes[k] = node
	}
	v.mu.Unlock()
	for k, node := range moved {
		if node.isDir {
			fsPersistDir(k)
		} else {
			fsPersistFile(k, node.data)
		}
	}
	fsPersistDelete(oldKey)
	if n.isDir {
		fsPersistDeletePrefix(oldKey)
	}
	return nil
}

type vfsEntry struct {
	name  string
	isDir bool
	size  int64
	path  string
}

func (v *virtualFS) readDirEntries(p string) ([]vfsEntry, error) {
	key := v.resolve(p)
	v.mu.RLock()
	defer v.mu.RUnlock()
	if n, ok := v.nodes[key]; !ok || !n.isDir {
		return nil, errVfsNotExist
	}
	prefix := key
	if prefix != "/" {
		prefix += "/"
	}
	seen := make(map[string]bool)
	var out []vfsEntry
	for k, n := range v.nodes {
		if k == key || !strings.HasPrefix(k, prefix) {
			continue
		}
		rest := strings.TrimPrefix(k, prefix)
		parts := strings.SplitN(rest, "/", 2)
		name := parts[0]
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		isDir := len(parts) > 1
		size := int64(0)
		if !isDir {
			isDir = n.isDir
			size = int64(len(n.data))
		}
		out = append(out, vfsEntry{name: name, isDir: isDir, size: size, path: filepath.Join(p, name)})
	}
	return out, nil
}

func (v *virtualFS) loadFromStore(key string, data []byte, isDir bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.ensureParentsLocked(key)
	if isDir {
		v.nodes[key] = &vfsNode{isDir: true, mode: "drwxr-xr-x", modTime: time.Now()}
		return
	}
	v.nodes[key] = &vfsNode{isDir: false, data: data, mode: "-rw-r--r--", modTime: time.Now()}
}

func FsModule() *runtime.Value {
	fsEnsureHydrated()
	return runtime.ObjectVal(map[string]*runtime.Value{
		"readFile": runtime.FuncVal(&runtime.Function{Name: "readFile", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.Null, nil
			}
			data, err := globalVFS.readFile(args[0].ToString())
			if err != nil {
				return runtime.Null, nil
			}
			return runtime.StringVal(string(data)), nil
		}}),

		"writeFile": runtime.FuncVal(&runtime.Function{Name: "writeFile", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.False, nil
			}
			err := globalVFS.writeFile(args[0].ToString(), []byte(args[1].ToString()))
			return runtime.BoolVal(err == nil), nil
		}}),

		"appendFile": runtime.FuncVal(&runtime.Function{Name: "appendFile", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.False, nil
			}
			err := globalVFS.appendFile(args[0].ToString(), []byte(args[1].ToString()))
			return runtime.BoolVal(err == nil), nil
		}}),

		"delete": runtime.FuncVal(&runtime.Function{Name: "delete", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			err := globalVFS.remove(args[0].ToString())
			return runtime.BoolVal(err == nil), nil
		}}),

		"deleteAll": runtime.FuncVal(&runtime.Function{Name: "deleteAll", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			err := globalVFS.removeAll(args[0].ToString())
			return runtime.BoolVal(err == nil), nil
		}}),

		"exists": runtime.FuncVal(&runtime.Function{Name: "exists", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			_, _, err := globalVFS.statNode(args[0].ToString())
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
			err := globalVFS.mkdir(args[0].ToString(), recursive)
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
				err = globalVFS.removeAll(args[0].ToString())
			} else {
				err = globalVFS.remove(args[0].ToString())
			}
			return runtime.BoolVal(err == nil), nil
		}}),

		"rename": runtime.FuncVal(&runtime.Function{Name: "rename", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.False, nil
			}
			err := globalVFS.rename(args[0].ToString(), args[1].ToString())
			return runtime.BoolVal(err == nil), nil
		}}),

		"moveFile": runtime.FuncVal(&runtime.Function{Name: "moveFile", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.False, nil
			}
			err := globalVFS.rename(args[0].ToString(), args[1].ToString())
			return runtime.BoolVal(err == nil), nil
		}}),

		"copy": runtime.FuncVal(&runtime.Function{Name: "copy", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.False, nil
			}
			data, err := globalVFS.readFile(args[0].ToString())
			if err != nil {
				return runtime.False, nil
			}
			err = globalVFS.writeFile(args[1].ToString(), data)
			return runtime.BoolVal(err == nil), nil
		}}),

		"copyFile": runtime.FuncVal(&runtime.Function{Name: "copyFile", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.False, nil
			}
			data, err := globalVFS.readFile(args[0].ToString())
			if err != nil {
				return runtime.False, nil
			}
			err = globalVFS.writeFile(args[1].ToString(), data)
			return runtime.BoolVal(err == nil), nil
		}}),

		"list":    fsListFn(),
		"readDir": fsListFn(),

		"stat": runtime.FuncVal(&runtime.Function{Name: "stat", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.Null, nil
			}
			key, n, err := globalVFS.statNode(args[0].ToString())
			if err != nil {
				return runtime.Null, nil
			}
			return runtime.ObjectVal(map[string]*runtime.Value{
				"name":    runtime.StringVal(filepath.Base(key)),
				"size":    runtime.NumberVal(float64(len(n.data))),
				"isDir":   runtime.BoolVal(n.isDir),
				"isFile":  runtime.BoolVal(!n.isDir),
				"mode":    runtime.StringVal(n.mode),
				"modTime": runtime.NumberVal(float64(n.modTime.UnixNano() / 1e6)),
			}), nil
		}}),

		"isDir": runtime.FuncVal(&runtime.Function{Name: "isDir", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			_, n, err := globalVFS.statNode(args[0].ToString())
			if err != nil {
				return runtime.False, nil
			}
			return runtime.BoolVal(n.isDir), nil
		}}),

		"isFile": runtime.FuncVal(&runtime.Function{Name: "isFile", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			_, n, err := globalVFS.statNode(args[0].ToString())
			if err != nil {
				return runtime.False, nil
			}
			return runtime.BoolVal(!n.isDir), nil
		}}),

		"size": runtime.FuncVal(&runtime.Function{Name: "size", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.NumberVal(-1), nil
			}
			_, n, err := globalVFS.statNode(args[0].ToString())
			if err != nil {
				return runtime.NumberVal(-1), nil
			}
			return runtime.NumberVal(float64(len(n.data))), nil
		}}),

		"readLines": runtime.FuncVal(&runtime.Function{Name: "readLines", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.ArrayVal(nil), nil
			}
			data, err := globalVFS.readFile(args[0].ToString())
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
			data, err := globalVFS.readFile(args[0].ToString())
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
			err = globalVFS.writeFile(args[0].ToString(), data)
			return runtime.BoolVal(err == nil), nil
		}}),

		"readCSV": runtime.FuncVal(&runtime.Function{Name: "readCSV", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.ArrayVal(nil), nil
			}
			data, err := globalVFS.readFile(args[0].ToString())
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
			err := globalVFS.writeFile(args[0].ToString(), []byte(strings.Join(lines, "\n")))
			return runtime.BoolVal(err == nil), nil
		}}),

		"tempFile": runtime.FuncVal(&runtime.Function{Name: "tempFile", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			prefix := "lunex-"
			if len(args) > 0 {
				prefix = args[0].ToString()
			}
			name := filepath.Join("/tmp", prefix+fsRandomSuffix())
			if err := globalVFS.writeFile(name, nil); err != nil {
				return runtime.Null, nil
			}
			return runtime.StringVal(name), nil
		}}),

		"tempDir": runtime.FuncVal(&runtime.Function{Name: "tempDir", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			prefix := "lunex-"
			if len(args) > 0 {
				prefix = args[0].ToString()
			}
			name := filepath.Join("/tmp", prefix+fsRandomSuffix())
			if err := globalVFS.mkdir(name, true); err != nil {
				return runtime.Null, nil
			}
			return runtime.StringVal(name), nil
		}}),

		"abs": runtime.FuncVal(&runtime.Function{Name: "abs", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.StringVal(""), nil
			}
			return runtime.StringVal(globalVFS.resolve(args[0].ToString())), nil
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
			pattern := args[0].ToString()
			globalVFS.mu.RLock()
			var matches []string
			for k := range globalVFS.nodes {
				rel := k
				if filepath.IsAbs(pattern) {
					rel = k
				} else {
					rel = strings.TrimPrefix(k, "/")
				}
				if ok, _ := filepath.Match(pattern, rel); ok {
					matches = append(matches, rel)
				}
			}
			globalVFS.mu.RUnlock()
			out := make([]*runtime.Value, len(matches))
			for i, m := range matches {
				out[i] = runtime.StringVal(m)
			}
			return runtime.ArrayVal(out), nil
		}}),

		"cwd": runtime.FuncVal(&runtime.Function{Name: "cwd", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			globalVFS.mu.RLock()
			defer globalVFS.mu.RUnlock()
			return runtime.StringVal(globalVFS.cwd), nil
		}}),

		"home": runtime.FuncVal(&runtime.Function{Name: "home", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			return runtime.StringVal("/home"), nil
		}}),

		"ensureDir": runtime.FuncVal(&runtime.Function{Name: "ensureDir", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.False, nil
			}
			err := globalVFS.mkdir(args[0].ToString(), true)
			return runtime.BoolVal(err == nil), nil
		}}),
	})
}

func fsListFn() *runtime.Value {
	return runtime.FuncVal(&runtime.Function{Name: "list", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
		p := "."
		if len(args) > 0 {
			p = args[0].ToString()
		}
		entries, err := globalVFS.readDirEntries(p)
		if err != nil {
			return runtime.ArrayVal(nil), nil
		}
		out := make([]*runtime.Value, 0, len(entries))
		for _, e := range entries {
			out = append(out, runtime.ObjectVal(map[string]*runtime.Value{
				"name":   runtime.StringVal(e.name),
				"isDir":  runtime.BoolVal(e.isDir),
				"isFile": runtime.BoolVal(!e.isDir),
				"size":   runtime.NumberVal(float64(e.size)),
				"path":   runtime.StringVal(e.path),
			}))
		}
		return runtime.ArrayVal(out), nil
	}})
}

var fsRandomCounter uint64
var fsRandomMu sync.Mutex

func fsRandomSuffix() string {
	fsRandomMu.Lock()
	fsRandomCounter++
	n := fsRandomCounter
	fsRandomMu.Unlock()
	return time.Now().Format("20060102150405") + strconvItoa(n)
}

func strconvItoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
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
