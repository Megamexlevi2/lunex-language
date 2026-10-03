package std

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"lunex/internal/runtime"
	shared "lunex/internal/std/shared"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const httpGlueSource = `fn throwValue(value) {
  throw value
}

fn invoke2(handler, first, second) {
  var failure = null
  try {
    handler(first, second)
  } catch (caught) {
    failure = caught
  }
  failure
}

fn invoke3(handler, first, second, third) {
  var failure = null
  try {
    handler(first, second, third)
  } catch (caught) {
    failure = caught
  }
  failure
}

fn tick() {
  null
}

val __module__ = {
  throwValue: throwValue,
  invoke2: invoke2,
  invoke3: invoke3,
  tick: tick
}
`

const (
	httpDefaultMaxBody        int64 = 1 << 20
	httpDefaultMaxHeaderBytes       = 64 << 10
	httpDefaultMaxHeaderCount       = 100
	httpDefaultMaxResponse    int64 = 10 << 20
	httpDefaultReadHeader           = 10 * time.Second
	httpDefaultRead                 = 30 * time.Second
	httpDefaultWrite                = 60 * time.Second
	httpDefaultIdle                 = 60 * time.Second
	httpDefaultHandler              = 30 * time.Second
	httpDefaultClientTimeout        = 30 * time.Second
	httpDefaultShutdown             = 5 * time.Second
	httpDefaultMaxRedirects         = 10
)

const (
	httpErrArgument   = "E_HTTP_INVALID_ARGUMENT"
	httpErrFinished   = "E_HTTP_RESPONSE_FINISHED"
	httpErrHeader     = "E_HTTP_INVALID_HEADER"
	httpErrStatus     = "E_HTTP_INVALID_STATUS"
	httpErrJSON       = "E_HTTP_INVALID_JSON"
	httpErrURL        = "E_HTTP_INVALID_URL"
	httpErrCookie     = "E_HTTP_INVALID_COOKIE"
	httpErrListening  = "E_HTTP_ALREADY_LISTENING"
	httpErrListen     = "E_HTTP_LISTEN_FAILED"
	httpErrTimeout    = "E_HTTP_TIMEOUT"
	httpErrNetwork    = "E_HTTP_NETWORK"
	httpErrTooLarge   = "E_HTTP_RESPONSE_TOO_LARGE"
	httpErrRedirects  = "E_HTTP_TOO_MANY_REDIRECTS"
	httpErrBodyLarge  = "E_HTTP_BODY_TOO_LARGE"
	httpErrHeaders    = "E_HTTP_HEADERS_TOO_LARGE"
	httpErrReadTimout = "E_HTTP_REQUEST_TIMEOUT"
	httpErrBadRequest = "E_HTTP_BAD_REQUEST"
	httpErrHandler    = "E_HTTP_HANDLER_TIMEOUT"
	httpErrInternal   = "E_HTTP_INTERNAL"
)

var (
	httpInterpLock sync.Mutex
	httpResponses  sync.Map

	httpRuntimeMu   sync.Mutex
	httpRuntimeInst *httpRuntime
	httpRuntimeHost httpHost

	errHTTPTooManyRedirects = errors.New("stopped after too many redirects")

	httpClientTransport = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
)

type httpHost interface {
	ExecAsModule(source, filename string) (*runtime.Value, error)
	SetGlobal(name string, val *runtime.Value)
	CallExport(name string, args ...interface{}) (interface{}, error)
}

type httpRuntime struct {
	host     httpHost
	throwFn  *runtime.Value
	invoke2  *runtime.Value
	invoke3  *runtime.Value
}

func httpRuntimeFor(host httpHost) *httpRuntime {
	httpRuntimeMu.Lock()
	defer httpRuntimeMu.Unlock()
	if httpRuntimeInst != nil && httpRuntimeHost == host {
		return httpRuntimeInst
	}
	mod, err := host.ExecAsModule(httpGlueSource, "http_glue")
	if err != nil {
		panic(err)
	}
	if mod == nil || mod.Tag != runtime.TypeObject {
		panic("http: glue module did not load")
	}
	rt := &httpRuntime{
		host:    host,
		throwFn: mod.ObjVal["throwValue"],
		invoke2: mod.ObjVal["invoke2"],
		invoke3: mod.ObjVal["invoke3"],
	}
	tick := mod.ObjVal["tick"]
	if rt.throwFn == nil || rt.invoke2 == nil || rt.invoke3 == nil || tick == nil {
		panic("http: glue module is incomplete")
	}
	host.SetGlobal("__http_tick__", tick)
	httpRuntimeInst = rt
	httpRuntimeHost = host
	return rt
}

func httpErrorValue(status int, message, code string) *runtime.Value {
	return runtime.ObjectVal(map[string]*runtime.Value{
		"name":    runtime.StringVal("HttpError"),
		"code":    runtime.StringVal(code),
		"message": runtime.StringVal(message),
		"status":  runtime.NumberVal(float64(status)),
	})
}

func (rt *httpRuntime) fail(code, message string) error {
	if rt == nil || rt.throwFn == nil || runtime.CallFunction == nil {
		return fmt.Errorf("%s: %s", code, message)
	}
	status := 500
	switch code {
	case httpErrJSON, httpErrBadRequest:
		status = 400
	}
	_, err := runtime.CallFunction(rt.throwFn, []*runtime.Value{httpErrorValue(status, message, code)})
	if err == nil {
		return fmt.Errorf("%s: %s", code, message)
	}
	return err
}

func (rt *httpRuntime) failStatus(code, message string, status int) error {
	if rt == nil || rt.throwFn == nil || runtime.CallFunction == nil {
		return fmt.Errorf("%s: %s", code, message)
	}
	_, err := runtime.CallFunction(rt.throwFn, []*runtime.Value{httpErrorValue(status, message, code)})
	if err == nil {
		return fmt.Errorf("%s: %s", code, message)
	}
	return err
}

func httpFn(name string, f func(a []*runtime.Value) (*runtime.Value, error)) *runtime.Value {
	return runtime.FuncVal(&runtime.Function{
		Name: name,
		Native: func(a []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			return f(a)
		},
	})
}

func httpArg(a []*runtime.Value, i int) *runtime.Value {
	if i < len(a) && a[i] != nil {
		return a[i]
	}
	return runtime.Undefined
}

func httpIsNil(v *runtime.Value) bool {
	return v == nil || v.Tag == runtime.TypeUndefined || v.Tag == runtime.TypeNull
}

func (rt *httpRuntime) argString(fn string, a []*runtime.Value, i int) (string, error) {
	v := httpArg(a, i)
	if v.Tag != runtime.TypeString {
		return "", rt.fail(httpErrArgument, fn+": argument "+strconv.Itoa(i+1)+" must be a string")
	}
	return v.StrVal, nil
}

func (rt *httpRuntime) argStatus(fn string, a []*runtime.Value, i int, def int) (int, error) {
	v := httpArg(a, i)
	if httpIsNil(v) {
		return def, nil
	}
	if v.Tag != runtime.TypeNumber || v.NumVal != math.Trunc(v.NumVal) || v.NumVal < 200 || v.NumVal > 599 {
		return 0, rt.fail(httpErrStatus, fn+": status must be an integer between 200 and 599")
	}
	return int(v.NumVal), nil
}

func (rt *httpRuntime) optNumber(fn string, opts map[string]*runtime.Value, key string, def float64, min float64) (float64, error) {
	v, ok := opts[key]
	if !ok || httpIsNil(v) {
		return def, nil
	}
	if v.Tag != runtime.TypeNumber || math.IsNaN(v.NumVal) || math.IsInf(v.NumVal, 0) || v.NumVal < min {
		return 0, rt.fail(httpErrArgument, fn+": option '"+key+"' must be a finite number >= "+strconv.FormatFloat(min, 'f', -1, 64))
	}
	return v.NumVal, nil
}

func (rt *httpRuntime) checkKnownKeys(fn string, opts map[string]*runtime.Value, known ...string) error {
	for k := range opts {
		found := false
		for _, name := range known {
			if k == name {
				found = true
				break
			}
		}
		if !found {
			return rt.fail(httpErrArgument, fn+": unknown option '"+k+"'")
		}
	}
	return nil
}

func (rt *httpRuntime) optObject(fn string, v *runtime.Value) (map[string]*runtime.Value, error) {
	if httpIsNil(v) {
		return map[string]*runtime.Value{}, nil
	}
	if v.Tag != runtime.TypeObject {
		return nil, rt.fail(httpErrArgument, fn+": options must be an object")
	}
	if v.ObjVal == nil {
		return map[string]*runtime.Value{}, nil
	}
	return v.ObjVal, nil
}

func httpIsTokenChar(c byte) bool {
	if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
		return true
	}
	return strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0
}

func httpValidHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !httpIsTokenChar(name[i]) {
			return false
		}
	}
	return true
}

func httpValidHeaderValue(value string) bool {
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c == 0 || c == '\r' || c == '\n' || c == 0x7f {
			return false
		}
		if c < 0x20 && c != '\t' {
			return false
		}
	}
	return true
}

func (rt *httpRuntime) headerValueString(fn string, v *runtime.Value) (string, error) {
	switch v.Tag {
	case runtime.TypeString:
		if !httpValidHeaderValue(v.StrVal) {
			return "", rt.fail(httpErrHeader, fn+": header value contains forbidden characters")
		}
		return v.StrVal, nil
	case runtime.TypeNumber:
		return v.ToString(), nil
	}
	return "", rt.fail(httpErrHeader, fn+": header value must be a string or a number")
}

func httpHeadersToObj(h http.Header) *runtime.Value {
	obj := make(map[string]*runtime.Value, len(h))
	for k, vals := range h {
		if len(vals) == 0 {
			continue
		}
		name := strings.ToLower(k)
		sep := ", "
		if name == "cookie" {
			sep = "; "
		}
		obj[name] = runtime.StringVal(strings.Join(vals, sep))
	}
	return runtime.ObjectVal(obj)
}

func httpCookiesToObj(r *http.Request) *runtime.Value {
	out := make(map[string]*runtime.Value)
	for _, c := range r.Cookies() {
		if _, exists := out[c.Name]; !exists {
			out[c.Name] = runtime.StringVal(c.Value)
		}
	}
	return runtime.ObjectVal(out)
}

func (rt *httpRuntime) buildCookie(fn, name, value string, opts *runtime.Value, clear bool) (string, error) {
	c := &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}
	o, err := rt.optObject(fn, opts)
	if err != nil {
		return "", err
	}
	if err := rt.checkKnownKeys(fn, o, "path", "domain", "maxAge", "secure", "httpOnly", "sameSite"); err != nil {
		return "", err
	}
	if v, ok := o["path"]; ok && !httpIsNil(v) {
		if v.Tag != runtime.TypeString {
			return "", rt.fail(httpErrCookie, fn+": option 'path' must be a string")
		}
		c.Path = v.StrVal
	}
	if v, ok := o["domain"]; ok && !httpIsNil(v) {
		if v.Tag != runtime.TypeString {
			return "", rt.fail(httpErrCookie, fn+": option 'domain' must be a string")
		}
		c.Domain = v.StrVal
	}
	if v, ok := o["maxAge"]; ok && !httpIsNil(v) {
		if v.Tag != runtime.TypeNumber || v.NumVal != math.Trunc(v.NumVal) || v.NumVal < 0 || v.NumVal > 31536000*10 {
			return "", rt.fail(httpErrCookie, fn+": option 'maxAge' must be a non-negative integer number of seconds")
		}
		c.MaxAge = int(v.NumVal)
	}
	if v, ok := o["secure"]; ok && !httpIsNil(v) {
		if v.Tag != runtime.TypeBool {
			return "", rt.fail(httpErrCookie, fn+": option 'secure' must be a boolean")
		}
		c.Secure = v.BoolVal
	}
	if v, ok := o["httpOnly"]; ok && !httpIsNil(v) {
		if v.Tag != runtime.TypeBool {
			return "", rt.fail(httpErrCookie, fn+": option 'httpOnly' must be a boolean")
		}
		c.HttpOnly = v.BoolVal
	}
	if v, ok := o["sameSite"]; ok && !httpIsNil(v) {
		if v.Tag != runtime.TypeString {
			return "", rt.fail(httpErrCookie, fn+": option 'sameSite' must be 'lax', 'strict' or 'none'")
		}
		switch strings.ToLower(v.StrVal) {
		case "lax":
			c.SameSite = http.SameSiteLaxMode
		case "strict":
			c.SameSite = http.SameSiteStrictMode
		case "none":
			c.SameSite = http.SameSiteNoneMode
		default:
			return "", rt.fail(httpErrCookie, fn+": option 'sameSite' must be 'lax', 'strict' or 'none'")
		}
	}
	if c.SameSite == http.SameSiteNoneMode && !c.Secure {
		return "", rt.fail(httpErrCookie, fn+": sameSite 'none' requires secure: true")
	}
	if clear {
		c.Value = ""
		c.MaxAge = -1
		c.Expires = time.Unix(0, 0)
	}
	if err := c.Valid(); err != nil {
		return "", rt.fail(httpErrCookie, fn+": "+err.Error())
	}
	return c.String(), nil
}

func httpWriteError(w http.ResponseWriter, status int, code, message string, closeConn bool) {
	body, _ := json.Marshal(map[string]string{"error": message, "code": code})
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	if closeConn {
		h.Set("Connection", "close")
	}
	w.WriteHeader(status)
	w.Write(body)
}

func httpJSONError(message, code string) []byte {
	body, _ := json.Marshal(map[string]string{"error": message, "code": code})
	return body
}

type httpFileBody struct {
	file    *os.File
	name    string
	modTime time.Time
}

type httpResponse struct {
	rt       *httpRuntime
	mu       sync.Mutex
	status   int
	header   http.Header
	body     []byte
	file     *httpFileBody
	finished bool
	aborted  bool
	done     chan struct{}
}

func newHTTPResponse(rt *httpRuntime) *httpResponse {
	return &httpResponse{rt: rt, status: 200, header: make(http.Header), done: make(chan struct{})}
}

func (r *httpResponse) mutate(fn func()) error {
	r.mu.Lock()
	if r.finished {
		r.mu.Unlock()
		return r.rt.fail(httpErrFinished, "response has already been sent")
	}
	fn()
	r.mu.Unlock()
	return nil
}

func (r *httpResponse) isFinished() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.finished
}

func (r *httpResponse) finish(status int, contentType string, body []byte, file *httpFileBody) error {
	r.mu.Lock()
	if r.finished {
		r.mu.Unlock()
		if file != nil {
			file.file.Close()
		}
		return r.rt.fail(httpErrFinished, "response has already been sent")
	}
	if status != 0 {
		r.status = status
	}
	if contentType != "" {
		r.header.Set("Content-Type", contentType)
	}
	r.body = body
	r.file = file
	r.finished = true
	close(r.done)
	r.mu.Unlock()
	return nil
}

func (r *httpResponse) forceFinish(status int, body []byte, aborted bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		return
	}
	r.status = status
	r.aborted = aborted
	if !aborted {
		r.header = make(http.Header)
		r.header.Set("Content-Type", "application/json; charset=utf-8")
		r.body = body
	}
	r.finished = true
	close(r.done)
}

func (r *httpResponse) commit(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	aborted := r.aborted
	status := r.status
	header := r.header
	body := r.body
	file := r.file
	r.mu.Unlock()
	if aborted {
		if file != nil {
			file.file.Close()
		}
		return
	}
	dst := w.Header()
	for k, vals := range header {
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
	if dst.Get("X-Content-Type-Options") == "" {
		dst.Set("X-Content-Type-Options", "nosniff")
	}
	if file != nil {
		defer file.file.Close()
		if status == 204 || status == 304 {
			dst.Del("Content-Type")
			w.WriteHeader(status)
			return
		}
		http.ServeContent(w, req, file.name, file.modTime, file.file)
		return
	}
	if status == 204 || status == 304 {
		dst.Del("Content-Type")
		dst.Del("Content-Length")
		w.WriteHeader(status)
		return
	}
	dst.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	if req.Method != http.MethodHead {
		w.Write(body)
	}
}

func (rt *httpRuntime) responseObject(resp *httpResponse) *runtime.Value {
	var obj *runtime.Value
	m := make(map[string]*runtime.Value)

	m["status"] = httpFn("status", func(a []*runtime.Value) (*runtime.Value, error) {
		code, err := rt.argStatus("res.status", a, 0, -1)
		if err != nil {
			return runtime.Undefined, err
		}
		if code < 0 {
			return runtime.Undefined, rt.fail(httpErrStatus, "res.status: a status code is required")
		}
		if err := resp.mutate(func() { resp.status = code }); err != nil {
			return runtime.Undefined, err
		}
		return obj, nil
	})

	m["setHeader"] = httpFn("setHeader", func(a []*runtime.Value) (*runtime.Value, error) {
		name, err := rt.argString("res.setHeader", a, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		if !httpValidHeaderName(name) {
			return runtime.Undefined, rt.fail(httpErrHeader, "res.setHeader: invalid header name")
		}
		switch strings.ToLower(name) {
		case "content-length", "transfer-encoding":
			return runtime.Undefined, rt.fail(httpErrHeader, "res.setHeader: '"+name+"' is managed by the server")
		case "set-cookie":
			return runtime.Undefined, rt.fail(httpErrHeader, "res.setHeader: use res.cookie to set cookies")
		}
		value, err := rt.headerValueString("res.setHeader", httpArg(a, 1))
		if err != nil {
			return runtime.Undefined, err
		}
		if err := resp.mutate(func() { resp.header.Set(name, value) }); err != nil {
			return runtime.Undefined, err
		}
		return obj, nil
	})

	m["getHeader"] = httpFn("getHeader", func(a []*runtime.Value) (*runtime.Value, error) {
		name, err := rt.argString("res.getHeader", a, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		resp.mu.Lock()
		vals := resp.header.Values(name)
		resp.mu.Unlock()
		if len(vals) == 0 {
			return runtime.Null, nil
		}
		return runtime.StringVal(strings.Join(vals, ", ")), nil
	})

	m["removeHeader"] = httpFn("removeHeader", func(a []*runtime.Value) (*runtime.Value, error) {
		name, err := rt.argString("res.removeHeader", a, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		if err := resp.mutate(func() { resp.header.Del(name) }); err != nil {
			return runtime.Undefined, err
		}
		return obj, nil
	})

	m["cookie"] = httpFn("cookie", func(a []*runtime.Value) (*runtime.Value, error) {
		name, err := rt.argString("res.cookie", a, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		value, err := rt.argString("res.cookie", a, 1)
		if err != nil {
			return runtime.Undefined, err
		}
		line, err := rt.buildCookie("res.cookie", name, value, httpArg(a, 2), false)
		if err != nil {
			return runtime.Undefined, err
		}
		if err := resp.mutate(func() { resp.header.Add("Set-Cookie", line) }); err != nil {
			return runtime.Undefined, err
		}
		return obj, nil
	})

	m["clearCookie"] = httpFn("clearCookie", func(a []*runtime.Value) (*runtime.Value, error) {
		name, err := rt.argString("res.clearCookie", a, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		line, err := rt.buildCookie("res.clearCookie", name, "", httpArg(a, 1), true)
		if err != nil {
			return runtime.Undefined, err
		}
		if err := resp.mutate(func() { resp.header.Add("Set-Cookie", line) }); err != nil {
			return runtime.Undefined, err
		}
		return obj, nil
	})

	m["json"] = httpFn("json", func(a []*runtime.Value) (*runtime.Value, error) {
		v := httpArg(a, 0)
		if v.Tag == runtime.TypeUndefined {
			return runtime.Undefined, rt.fail(httpErrArgument, "res.json: a value is required")
		}
		status, err := rt.argStatus("res.json", a, 1, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		data, jerr := shared.StringifyJSON(v, 0)
		if jerr != nil {
			return runtime.Undefined, rt.fail(httpErrJSON, "res.json: "+jerr.Error())
		}
		if err := resp.finish(status, "application/json; charset=utf-8", []byte(data), nil); err != nil {
			return runtime.Undefined, err
		}
		return runtime.Undefined, nil
	})

	m["text"] = httpFn("text", func(a []*runtime.Value) (*runtime.Value, error) {
		body, err := rt.argString("res.text", a, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		status, err := rt.argStatus("res.text", a, 1, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		if err := resp.finish(status, "text/plain; charset=utf-8", []byte(body), nil); err != nil {
			return runtime.Undefined, err
		}
		return runtime.Undefined, nil
	})

	m["html"] = httpFn("html", func(a []*runtime.Value) (*runtime.Value, error) {
		body, err := rt.argString("res.html", a, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		status, err := rt.argStatus("res.html", a, 1, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		if err := resp.finish(status, "text/html; charset=utf-8", []byte(body), nil); err != nil {
			return runtime.Undefined, err
		}
		return runtime.Undefined, nil
	})

	m["end"] = httpFn("end", func(a []*runtime.Value) (*runtime.Value, error) {
		body := ""
		if v := httpArg(a, 0); !httpIsNil(v) {
			if v.Tag != runtime.TypeString {
				return runtime.Undefined, rt.fail(httpErrArgument, "res.end: body must be a string")
			}
			body = v.StrVal
		}
		status, err := rt.argStatus("res.end", a, 1, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		if err := resp.finish(status, "", []byte(body), nil); err != nil {
			return runtime.Undefined, err
		}
		return runtime.Undefined, nil
	})

	m["redirect"] = httpFn("redirect", func(a []*runtime.Value) (*runtime.Value, error) {
		target, err := rt.argString("res.redirect", a, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		if target == "" || !httpValidHeaderValue(target) {
			return runtime.Undefined, rt.fail(httpErrHeader, "res.redirect: invalid location")
		}
		status := 302
		if v := httpArg(a, 1); !httpIsNil(v) {
			if v.Tag != runtime.TypeNumber {
				return runtime.Undefined, rt.fail(httpErrStatus, "res.redirect: status must be 301, 302, 303, 307 or 308")
			}
			status = int(v.NumVal)
			if float64(status) != v.NumVal || (status != 301 && status != 302 && status != 303 && status != 307 && status != 308) {
				return runtime.Undefined, rt.fail(httpErrStatus, "res.redirect: status must be 301, 302, 303, 307 or 308")
			}
		}
		if err := resp.mutate(func() { resp.header.Set("Location", target) }); err != nil {
			return runtime.Undefined, err
		}
		if err := resp.finish(status, "", nil, nil); err != nil {
			return runtime.Undefined, err
		}
		return runtime.Undefined, nil
	})

	m["finished"] = httpFn("finished", func(a []*runtime.Value) (*runtime.Value, error) {
		return runtime.BoolVal(resp.isFinished()), nil
	})

	obj = runtime.ObjectVal(m)
	return obj
}

func (rt *httpRuntime) requestObject(r *http.Request, body string) *runtime.Value {
	query := make(map[string]*runtime.Value)
	queryAll := make(map[string]*runtime.Value)
	values, _ := url.ParseQuery(r.URL.RawQuery)
	for k, vals := range values {
		if len(vals) == 0 {
			continue
		}
		query[k] = runtime.StringVal(vals[0])
		all := make([]*runtime.Value, len(vals))
		for i, v := range vals {
			all[i] = runtime.StringVal(v)
		}
		queryAll[k] = runtime.ArrayVal(all)
	}
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		ip = host
	}
	headers := httpHeadersToObj(r.Header)
	return runtime.ObjectVal(map[string]*runtime.Value{
		"method":   runtime.StringVal(r.Method),
		"url":      runtime.StringVal(r.URL.RequestURI()),
		"path":     runtime.StringVal(r.URL.Path),
		"rawPath":  runtime.StringVal(r.URL.EscapedPath()),
		"query":    runtime.ObjectVal(query),
		"queryAll": runtime.ObjectVal(queryAll),
		"headers":  headers,
		"cookies":  httpCookiesToObj(r),
		"body":     runtime.StringVal(body),
		"ip":       runtime.StringVal(ip),
		"host":     runtime.StringVal(r.Host),
		"text": httpFn("text", func(a []*runtime.Value) (*runtime.Value, error) {
			return runtime.StringVal(body), nil
		}),
		"json": httpFn("json", func(a []*runtime.Value) (*runtime.Value, error) {
			if strings.TrimSpace(body) == "" {
				return runtime.Undefined, rt.failStatus(httpErrJSON, "request body is empty", 400)
			}
			v, err := shared.ParseJSON(body)
			if err != nil {
				return runtime.Undefined, rt.failStatus(httpErrJSON, "request body is not valid JSON: "+err.Error(), 400)
			}
			return v, nil
		}),
		"header": httpFn("header", func(a []*runtime.Value) (*runtime.Value, error) {
			name, err := rt.argString("req.header", a, 0)
			if err != nil {
				return runtime.Undefined, err
			}
			if v, ok := headers.ObjVal[strings.ToLower(name)]; ok {
				return v, nil
			}
			return runtime.Null, nil
		}),
	})
}

type httpServerConfig struct {
	maxBody           int64
	maxHeaderBytes    int
	maxHeaderCount    int
	readHeaderTimeout time.Duration
	readTimeout       time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	handlerTimeout    time.Duration
	onError           *runtime.Value
}

func httpMillis(ms float64) time.Duration {
	return time.Duration(ms * float64(time.Millisecond))
}

func (rt *httpRuntime) serverConfig(opts *runtime.Value) (*httpServerConfig, error) {
	o, err := rt.optObject("http.createServer", opts)
	if err != nil {
		return nil, err
	}
	if err := rt.checkKnownKeys("http.createServer", o, "maxBodyBytes", "maxHeaderBytes", "maxHeaderCount",
		"readHeaderTimeout", "readTimeout", "writeTimeout", "idleTimeout", "handlerTimeout", "onError"); err != nil {
		return nil, err
	}
	cfg := &httpServerConfig{}
	maxBody, err := rt.optNumber("http.createServer", o, "maxBodyBytes", float64(httpDefaultMaxBody), 0)
	if err != nil {
		return nil, err
	}
	maxHeaderBytes, err := rt.optNumber("http.createServer", o, "maxHeaderBytes", httpDefaultMaxHeaderBytes, 1024)
	if err != nil {
		return nil, err
	}
	maxHeaderCount, err := rt.optNumber("http.createServer", o, "maxHeaderCount", httpDefaultMaxHeaderCount, 1)
	if err != nil {
		return nil, err
	}
	readHeader, err := rt.optNumber("http.createServer", o, "readHeaderTimeout", float64(httpDefaultReadHeader/time.Millisecond), 0)
	if err != nil {
		return nil, err
	}
	read, err := rt.optNumber("http.createServer", o, "readTimeout", float64(httpDefaultRead/time.Millisecond), 0)
	if err != nil {
		return nil, err
	}
	write, err := rt.optNumber("http.createServer", o, "writeTimeout", float64(httpDefaultWrite/time.Millisecond), 0)
	if err != nil {
		return nil, err
	}
	idle, err := rt.optNumber("http.createServer", o, "idleTimeout", float64(httpDefaultIdle/time.Millisecond), 0)
	if err != nil {
		return nil, err
	}
	handler, err := rt.optNumber("http.createServer", o, "handlerTimeout", float64(httpDefaultHandler/time.Millisecond), 0)
	if err != nil {
		return nil, err
	}
	cfg.maxBody = int64(maxBody)
	cfg.maxHeaderBytes = int(maxHeaderBytes)
	cfg.maxHeaderCount = int(maxHeaderCount)
	cfg.readHeaderTimeout = httpMillis(readHeader)
	cfg.readTimeout = httpMillis(read)
	cfg.writeTimeout = httpMillis(write)
	cfg.idleTimeout = httpMillis(idle)
	cfg.handlerTimeout = httpMillis(handler)
	if v, ok := o["onError"]; ok && !httpIsNil(v) {
		if v.Tag != runtime.TypeFunction {
			return nil, rt.fail(httpErrArgument, "http.createServer: option 'onError' must be a function")
		}
		cfg.onError = v
	}
	return cfg, nil
}

func (rt *httpRuntime) invokeHandler(fn *runtime.Value, args ...*runtime.Value) (failure *runtime.Value) {
	httpInterpLock.Lock()
	defer httpInterpLock.Unlock()
	defer func() {
		if rec := recover(); rec != nil {
			failure = httpErrorValue(500, fmt.Sprint("internal error: ", rec), httpErrInternal)
		}
	}()
	if runtime.CallFunction == nil {
		return httpErrorValue(500, "interpreter is not available", httpErrInternal)
	}
	rt.host.CallExport("__http_tick__")
	var result *runtime.Value
	var err error
	if len(args) == 2 {
		result, err = runtime.CallFunction(rt.invoke2, []*runtime.Value{fn, args[0], args[1]})
	} else {
		result, err = runtime.CallFunction(rt.invoke3, []*runtime.Value{fn, args[0], args[1], args[2]})
	}
	if err != nil {
		return httpErrorValue(500, err.Error(), httpErrInternal)
	}
	if httpIsNil(result) {
		return nil
	}
	return result
}

func httpFailureInfo(failure *runtime.Value) (int, string, string, string) {
	status := 500
	code := httpErrInternal
	message := ""
	if failure != nil && failure.Tag == runtime.TypeObject {
		if v, ok := failure.ObjVal["message"]; ok && v.Tag == runtime.TypeString {
			message = v.StrVal
		}
		if v, ok := failure.ObjVal["code"]; ok && v.Tag == runtime.TypeString {
			code = v.StrVal
		}
		name := ""
		if v, ok := failure.ObjVal["name"]; ok && v.Tag == runtime.TypeString {
			name = v.StrVal
		}
		if v, ok := failure.ObjVal["status"]; ok && name == "HttpError" && v.Tag == runtime.TypeNumber {
			s := int(v.NumVal)
			if float64(s) == v.NumVal && s >= 400 && s <= 599 {
				status = s
			}
		}
	} else if failure != nil {
		message = failure.ToString()
	}
	exposed := "Internal Server Error"
	if status < 500 {
		exposed = message
		if exposed == "" {
			exposed = http.StatusText(status)
		}
	} else {
		code = httpErrInternal
	}
	return status, code, message, exposed
}

func (rt *httpRuntime) handleFailure(cfg *httpServerConfig, r *http.Request, failure, reqObj, resObj *runtime.Value, resp *httpResponse) {
	_, code, message, _ := httpFailureInfo(failure)
	if cfg.onError != nil {
		if second := rt.invokeHandler(cfg.onError, failure, reqObj, resObj); second != nil {
			_, c2, m2, _ := httpFailureInfo(second)
			fmt.Fprintf(os.Stderr, "http: onError failed for %s %q: %s (%s)\n", r.Method, r.URL.Path, m2, c2)
		}
	} else {
		fmt.Fprintf(os.Stderr, "http: handler error for %s %q: %s (%s)\n", r.Method, r.URL.Path, message, code)
	}
	if resp.isFinished() {
		return
	}
	status, code2, _, exposed := httpFailureInfo(failure)
	resp.finish(status, "application/json; charset=utf-8", httpJSONError(exposed, code2), nil)
}

func (rt *httpRuntime) await(cfg *httpServerConfig, resp *httpResponse, r *http.Request) {
	var timer <-chan time.Time
	if cfg.handlerTimeout > 0 {
		t := time.NewTimer(cfg.handlerTimeout)
		defer t.Stop()
		timer = t.C
	}
	select {
	case <-resp.done:
	case <-timer:
		resp.forceFinish(504, httpJSONError("Gateway Timeout", httpErrHandler), false)
	case <-r.Context().Done():
		resp.forceFinish(499, nil, true)
	}
}

func httpReadBody(w http.ResponseWriter, r *http.Request, limit int64) (string, int, string, string) {
	if r.Body == nil || r.Body == http.NoBody {
		return "", 0, "", ""
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return "", 413, httpErrBodyLarge, "request body exceeds the configured limit"
		}
		var ne net.Error
		if errors.Is(err, os.ErrDeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
			return "", 408, httpErrReadTimout, "timed out while reading the request body"
		}
		return "", 400, httpErrBadRequest, "could not read the request body"
	}
	return string(data), 0, "", ""
}

func (rt *httpRuntime) serveHTTP(cfg *httpServerConfig, handler *runtime.Value, w http.ResponseWriter, r *http.Request) {
	if len(r.Header) > cfg.maxHeaderCount {
		httpWriteError(w, 431, httpErrHeaders, "too many request headers", true)
		return
	}
	if strings.IndexByte(r.URL.Path, 0) >= 0 || strings.IndexByte(r.URL.RawQuery, 0) >= 0 {
		httpWriteError(w, 400, httpErrBadRequest, "invalid request target", true)
		return
	}
	if r.ContentLength > cfg.maxBody {
		httpWriteError(w, 413, httpErrBodyLarge, "request body exceeds the configured limit", true)
		return
	}
	body, status, code, message := httpReadBody(w, r, cfg.maxBody)
	if status != 0 {
		httpWriteError(w, status, code, message, true)
		return
	}
	resp := newHTTPResponse(rt)
	resObj := rt.responseObject(resp)
	httpResponses.Store(resObj, resp)
	defer httpResponses.Delete(resObj)
	reqObj := rt.requestObject(r, body)
	if failure := rt.invokeHandler(handler, reqObj, resObj); failure != nil {
		rt.handleFailure(cfg, r, failure, reqObj, resObj, resp)
	}
	rt.await(cfg, resp, r)
	resp.commit(w, r)
}

type httpServerState struct {
	mu        sync.Mutex
	cfg       *httpServerConfig
	handler   *runtime.Value
	srv       *http.Server
	ln        net.Listener
	listening bool
	closed    bool
}

func (st *httpServerState) isClosed() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.closed
}

func (st *httpServerState) shutdown(timeout time.Duration) {
	st.mu.Lock()
	if !st.listening || st.closed {
		st.mu.Unlock()
		return
	}
	st.closed = true
	srv := st.srv
	ln := st.ln
	st.mu.Unlock()
	runtime.KeepAliveAdd()
	ln.Close()
	go func() {
		defer runtime.KeepAliveDone()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			srv.Close()
		}
	}()
}

func (rt *httpRuntime) serverObject(st *httpServerState) *runtime.Value {
	m := make(map[string]*runtime.Value)
	obj := runtime.ObjectVal(m)
	m["port"] = runtime.NumberVal(0)
	m["host"] = runtime.StringVal("")

	m["listen"] = httpFn("listen", func(a []*runtime.Value) (*runtime.Value, error) {
		pv := httpArg(a, 0)
		if pv.Tag != runtime.TypeNumber || pv.NumVal != math.Trunc(pv.NumVal) || pv.NumVal < 0 || pv.NumVal > 65535 {
			return runtime.Undefined, rt.fail(httpErrArgument, "server.listen: port must be an integer between 0 and 65535")
		}
		host := "0.0.0.0"
		var onReady *runtime.Value
		if v := httpArg(a, 1); !httpIsNil(v) {
			if v.Tag == runtime.TypeString && v.StrVal != "" {
				host = v.StrVal
			} else if v.Tag == runtime.TypeFunction {
				onReady = v
			} else {
				return runtime.Undefined, rt.fail(httpErrArgument, "server.listen: host must be a non-empty string")
			}
		}
		if v := httpArg(a, 2); !httpIsNil(v) {
			if v.Tag != runtime.TypeFunction {
				return runtime.Undefined, rt.fail(httpErrArgument, "server.listen: callback must be a function")
			}
			onReady = v
		}
		st.mu.Lock()
		if st.listening || st.closed {
			st.mu.Unlock()
			return runtime.Undefined, rt.fail(httpErrListening, "server is already listening or has been closed")
		}
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(int(pv.NumVal))))
		if err != nil {
			st.mu.Unlock()
			return runtime.Undefined, rt.fail(httpErrListen, err.Error())
		}
		port := int(pv.NumVal)
		if ta, ok := ln.Addr().(*net.TCPAddr); ok {
			port = ta.Port
		}
		cfg := st.cfg
		srv := &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				rt.serveHTTP(cfg, st.handler, w, r)
			}),
			ReadHeaderTimeout: cfg.readHeaderTimeout,
			ReadTimeout:       cfg.readTimeout,
			WriteTimeout:      cfg.writeTimeout,
			IdleTimeout:       cfg.idleTimeout,
			MaxHeaderBytes:    cfg.maxHeaderBytes,
		}
		st.srv = srv
		st.ln = ln
		st.listening = true
		st.mu.Unlock()
		runtime.KeepAliveAdd()
		go func() {
			defer runtime.KeepAliveDone()
			if serr := srv.Serve(ln); serr != nil && !errors.Is(serr, http.ErrServerClosed) && !st.isClosed() {
				fmt.Fprintf(os.Stderr, "http: server stopped: %s\n", serr)
			}
		}()
		m["port"] = runtime.NumberVal(float64(port))
		m["host"] = runtime.StringVal(host)
		if onReady != nil && runtime.CallFunction != nil {
			if _, cerr := runtime.CallFunction(onReady, []*runtime.Value{runtime.NumberVal(float64(port))}); cerr != nil {
				st.shutdown(0)
				return runtime.Undefined, cerr
			}
		}
		return obj, nil
	})

	m["close"] = httpFn("close", func(a []*runtime.Value) (*runtime.Value, error) {
		timeout := httpDefaultShutdown
		if v := httpArg(a, 0); !httpIsNil(v) {
			if v.Tag != runtime.TypeNumber || math.IsNaN(v.NumVal) || v.NumVal < 0 || math.IsInf(v.NumVal, 0) {
				return runtime.Undefined, rt.fail(httpErrArgument, "server.close: timeout must be a non-negative number of milliseconds")
			}
			timeout = httpMillis(v.NumVal)
		}
		st.shutdown(timeout)
		return runtime.Undefined, nil
	})

	return obj
}

func httpMimeType(ext string) string {
	ext = strings.ToLower(ext)
	switch ext {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "application/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json", ".map":
		return "application/json; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".wasm":
		return "application/wasm"
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

func httpWithin(root, candidate string) bool {
	if candidate == root {
		return true
	}
	sep := string(os.PathSeparator)
	return strings.HasPrefix(candidate, strings.TrimSuffix(root, sep)+sep)
}

func httpResolveStatic(root, rel, index string, allowDot bool) (string, os.FileInfo, bool) {
	if strings.IndexByte(rel, 0) >= 0 || strings.Contains(rel, "\\") {
		return "", nil, false
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return "", nil, false
		}
		if !allowDot && strings.HasPrefix(seg, ".") && seg != "." {
			return "", nil, false
		}
	}
	full := filepath.Join(root, filepath.FromSlash(rel))
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil || !httpWithin(root, resolved) {
		return "", nil, false
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", nil, false
	}
	if info.IsDir() {
		if index == "" {
			return "", nil, false
		}
		resolved, err = filepath.EvalSymlinks(filepath.Join(resolved, index))
		if err != nil || !httpWithin(root, resolved) {
			return "", nil, false
		}
		info, err = os.Stat(resolved)
		if err != nil {
			return "", nil, false
		}
	}
	if !info.Mode().IsRegular() {
		return "", nil, false
	}
	return resolved, info, true
}

func (rt *httpRuntime) staticCreate(a []*runtime.Value) (*runtime.Value, error) {
	rootArg, err := rt.argString("static.create", a, 0)
	if err != nil {
		return runtime.Undefined, err
	}
	o, err := rt.optObject("static.create", httpArg(a, 1))
	if err != nil {
		return runtime.Undefined, err
	}
	if err := rt.checkKnownKeys("static.create", o, "prefix", "index", "dotfiles", "maxAge"); err != nil {
		return runtime.Undefined, err
	}
	prefix := "/"
	if v, ok := o["prefix"]; ok && !httpIsNil(v) {
		if v.Tag != runtime.TypeString || !strings.HasPrefix(v.StrVal, "/") || strings.Contains(v.StrVal, "..") || strings.Contains(v.StrVal, "\\") {
			return runtime.Undefined, rt.fail(httpErrArgument, "static.create: option 'prefix' must be a path starting with '/'")
		}
		prefix = v.StrVal
		if prefix != "/" {
			prefix = strings.TrimRight(prefix, "/")
			if prefix == "" {
				prefix = "/"
			}
		}
	}
	index := "index.html"
	if v, ok := o["index"]; ok && !httpIsNil(v) {
		if v.Tag != runtime.TypeString || strings.ContainsAny(v.StrVal, "/\\") || v.StrVal == ".." {
			return runtime.Undefined, rt.fail(httpErrArgument, "static.create: option 'index' must be a file name")
		}
		index = v.StrVal
	}
	allowDot := false
	if v, ok := o["dotfiles"]; ok && !httpIsNil(v) {
		if v.Tag != runtime.TypeString || (v.StrVal != "allow" && v.StrVal != "deny") {
			return runtime.Undefined, rt.fail(httpErrArgument, "static.create: option 'dotfiles' must be 'allow' or 'deny'")
		}
		allowDot = v.StrVal == "allow"
	}
	maxAge, err := rt.optNumber("static.create", o, "maxAge", 0, 0)
	if err != nil {
		return runtime.Undefined, err
	}
	abs, err := filepath.Abs(rootArg)
	if err != nil {
		return runtime.Undefined, rt.fail(httpErrArgument, "static.create: invalid root directory")
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return runtime.Undefined, rt.fail(httpErrArgument, "static.create: root directory does not exist")
	}
	if info, serr := os.Stat(root); serr != nil || !info.IsDir() {
		return runtime.Undefined, rt.fail(httpErrArgument, "static.create: root must be a directory")
	}
	cacheControl := ""
	if maxAge > 0 {
		cacheControl = "public, max-age=" + strconv.FormatInt(int64(maxAge), 10)
	}

	return httpFn("static", func(args []*runtime.Value) (*runtime.Value, error) {
		reqV := httpArg(args, 0)
		resV := httpArg(args, 1)
		found, ok := httpResponses.Load(resV)
		if reqV.Tag != runtime.TypeObject || !ok {
			return runtime.Undefined, rt.fail(httpErrArgument, "static handler: expected (req, res) from an http server")
		}
		resp := found.(*httpResponse)
		method := ""
		if v, ok := reqV.ObjVal["method"]; ok && v.Tag == runtime.TypeString {
			method = v.StrVal
		}
		reqPath := ""
		if v, ok := reqV.ObjVal["path"]; ok && v.Tag == runtime.TypeString {
			reqPath = v.StrVal
		}
		if method != "GET" && method != "HEAD" {
			if err := resp.mutate(func() { resp.header.Set("Allow", "GET, HEAD") }); err != nil {
				return runtime.Undefined, err
			}
			return runtime.Undefined, resp.finish(405, "application/json; charset=utf-8", httpJSONError("Method Not Allowed", "E_HTTP_METHOD_NOT_ALLOWED"), nil)
		}
		notFound := func() (*runtime.Value, error) {
			return runtime.Undefined, resp.finish(404, "application/json; charset=utf-8", httpJSONError("Not Found", "E_HTTP_NOT_FOUND"), nil)
		}
		var rel string
		if prefix == "/" {
			if !strings.HasPrefix(reqPath, "/") {
				return notFound()
			}
			rel = reqPath[1:]
		} else if reqPath == prefix {
			rel = ""
		} else if strings.HasPrefix(reqPath, prefix+"/") {
			rel = reqPath[len(prefix)+1:]
		} else {
			return notFound()
		}
		rel = strings.TrimRight(rel, "/")
		resolved, info, okPath := httpResolveStatic(root, rel, index, allowDot)
		if !okPath {
			return notFound()
		}
		f, oerr := os.Open(resolved)
		if oerr != nil {
			return notFound()
		}
		if cacheControl != "" {
			if err := resp.mutate(func() { resp.header.Set("Cache-Control", cacheControl) }); err != nil {
				f.Close()
				return runtime.Undefined, err
			}
		}
		return runtime.Undefined, resp.finish(200, httpMimeType(filepath.Ext(resolved)), nil, &httpFileBody{file: f, name: info.Name(), modTime: info.ModTime()})
	}), nil
}

func httpClassifyError(err error) (string, string) {
	if errors.Is(err, errHTTPTooManyRedirects) {
		return httpErrRedirects, "stopped after too many redirects"
	}
	msg := err.Error()
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		msg = ue.Op + ": " + ue.Err.Error()
	}
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return httpErrTimeout, "request timed out"
	}
	return httpErrNetwork, msg
}

func (rt *httpRuntime) clientResult(status int, header http.Header, text, errMsg, errCode string) *runtime.Value {
	statusText := ""
	if status > 0 {
		statusText = http.StatusText(status)
	}
	cookies := []*runtime.Value{}
	for _, c := range header.Values("Set-Cookie") {
		cookies = append(cookies, runtime.StringVal(c))
	}
	res := map[string]*runtime.Value{
		"ok":         runtime.BoolVal(errCode == "" && status >= 200 && status < 300),
		"status":     runtime.NumberVal(float64(status)),
		"statusText": runtime.StringVal(statusText),
		"headers":    httpHeadersToObj(header),
		"cookies":    runtime.ArrayVal(cookies),
		"text":       runtime.StringVal(text),
		"error":      runtime.Null,
		"errorCode":  runtime.Null,
		"json": httpFn("json", func(a []*runtime.Value) (*runtime.Value, error) {
			if strings.TrimSpace(text) == "" {
				return runtime.Undefined, rt.failStatus(httpErrJSON, "response body is empty", 500)
			}
			v, err := shared.ParseJSON(text)
			if err != nil {
				return runtime.Undefined, rt.failStatus(httpErrJSON, "response body is not valid JSON: "+err.Error(), 500)
			}
			return v, nil
		}),
	}
	if errCode != "" {
		res["error"] = runtime.StringVal(errMsg)
		res["errorCode"] = runtime.StringVal(errCode)
	}
	return runtime.ObjectVal(res)
}

func (rt *httpRuntime) doRequest(fn, method string, urlArg, optsArg *runtime.Value) (*runtime.Value, error) {
	if urlArg == nil || urlArg.Tag != runtime.TypeString {
		return runtime.Undefined, rt.fail(httpErrArgument, fn+": url must be a string")
	}
	u, err := url.Parse(urlArg.StrVal)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" {
		return runtime.Undefined, rt.fail(httpErrURL, fn+": url must be an absolute http or https URL")
	}
	o, err := rt.optObject(fn, optsArg)
	if err != nil {
		return runtime.Undefined, err
	}
	if err := rt.checkKnownKeys(fn, o, "headers", "body", "json", "timeout", "maxResponseBytes", "maxRedirects"); err != nil {
		return runtime.Undefined, err
	}
	timeoutMs, err := rt.optNumber(fn, o, "timeout", float64(httpDefaultClientTimeout/time.Millisecond), 1)
	if err != nil {
		return runtime.Undefined, err
	}
	maxBytes, err := rt.optNumber(fn, o, "maxResponseBytes", float64(httpDefaultMaxResponse), 0)
	if err != nil {
		return runtime.Undefined, err
	}
	maxRedirects, err := rt.optNumber(fn, o, "maxRedirects", httpDefaultMaxRedirects, 0)
	if err != nil {
		return runtime.Undefined, err
	}
	headers := make(http.Header)
	if hv, ok := o["headers"]; ok && !httpIsNil(hv) {
		if hv.Tag != runtime.TypeObject {
			return runtime.Undefined, rt.fail(httpErrHeader, fn+": option 'headers' must be an object")
		}
		for k, v := range hv.ObjVal {
			if !httpValidHeaderName(k) {
				return runtime.Undefined, rt.fail(httpErrHeader, fn+": invalid header name '"+k+"'")
			}
			value, herr := rt.headerValueString(fn, v)
			if herr != nil {
				return runtime.Undefined, herr
			}
			headers.Set(k, value)
		}
	}
	var payload []byte
	hasBody := false
	if bv, ok := o["body"]; ok && !httpIsNil(bv) {
		if bv.Tag != runtime.TypeString {
			return runtime.Undefined, rt.fail(httpErrArgument, fn+": option 'body' must be a string; use 'json' for JSON payloads")
		}
		payload = []byte(bv.StrVal)
		hasBody = true
	}
	if jv, ok := o["json"]; ok && jv.Tag != runtime.TypeUndefined {
		if hasBody {
			return runtime.Undefined, rt.fail(httpErrArgument, fn+": options 'body' and 'json' cannot be used together")
		}
		data, jerr := shared.StringifyJSON(jv, 0)
		if jerr != nil {
			return runtime.Undefined, rt.fail(httpErrJSON, fn+": "+jerr.Error())
		}
		payload = []byte(data)
		hasBody = true
		if headers.Get("Content-Type") == "" {
			headers.Set("Content-Type", "application/json")
		}
	}
	if method == http.MethodHead && hasBody {
		return runtime.Undefined, rt.fail(httpErrArgument, fn+": HEAD requests cannot have a body")
	}

	ctx, cancel := context.WithTimeout(context.Background(), httpMillis(timeoutMs))
	defer cancel()
	var reader io.Reader
	if hasBody {
		reader = strings.NewReader(string(payload))
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return runtime.Undefined, rt.fail(httpErrURL, fn+": "+err.Error())
	}
	for k, vals := range headers {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "lunex-http")
	}
	limit := int(maxRedirects)
	client := &http.Client{
		Transport: httpClientTransport,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if limit == 0 {
				return http.ErrUseLastResponse
			}
			if len(via) > limit {
				return errHTTPTooManyRedirects
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		code, msg := httpClassifyError(err)
		return rt.clientResult(0, http.Header{}, "", msg, code), nil
	}
	defer resp.Body.Close()
	data, rerr := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)+1))
	if rerr != nil {
		code, msg := httpClassifyError(rerr)
		return rt.clientResult(resp.StatusCode, resp.Header, "", msg, code), nil
	}
	if float64(len(data)) > maxBytes {
		return rt.clientResult(resp.StatusCode, resp.Header, "", "response body exceeds the configured limit", httpErrTooLarge), nil
	}
	return rt.clientResult(resp.StatusCode, resp.Header, string(data), "", ""), nil
}

func httpEncode(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}

func (rt *httpRuntime) scalarString(fn, key string, v *runtime.Value) (string, error) {
	switch v.Tag {
	case runtime.TypeString:
		return v.StrVal, nil
	case runtime.TypeNumber, runtime.TypeBool:
		return v.ToString(), nil
	}
	return "", rt.fail(httpErrArgument, fn+": value for '"+key+"' must be a string, number, boolean or an array of those")
}

func (rt *httpRuntime) valuesFromObject(fn string, v *runtime.Value) (url.Values, error) {
	out := url.Values{}
	if httpIsNil(v) {
		return out, nil
	}
	if v.Tag != runtime.TypeObject {
		return nil, rt.fail(httpErrArgument, fn+": params must be an object")
	}
	for k, val := range v.ObjVal {
		if val.Tag == runtime.TypeUndefined || val.Tag == runtime.TypeNull {
			continue
		}
		if val.Tag == runtime.TypeArray {
			for _, item := range val.ArrVal {
				s, err := rt.scalarString(fn, k, item)
				if err != nil {
					return nil, err
				}
				out.Add(k, s)
			}
			continue
		}
		s, err := rt.scalarString(fn, k, val)
		if err != nil {
			return nil, err
		}
		out.Add(k, s)
	}
	return out, nil
}

func httpQueryToObject(values url.Values, all bool) *runtime.Value {
	out := make(map[string]*runtime.Value)
	for k, vals := range values {
		if len(vals) == 0 {
			continue
		}
		if all {
			items := make([]*runtime.Value, len(vals))
			for i, s := range vals {
				items[i] = runtime.StringVal(s)
			}
			out[k] = runtime.ArrayVal(items)
		} else {
			out[k] = runtime.StringVal(vals[0])
		}
	}
	return runtime.ObjectVal(out)
}

func httpStatusConstants() *runtime.Value {
	entries := []struct {
		name string
		code int
	}{
		{"OK", 200}, {"CREATED", 201}, {"ACCEPTED", 202}, {"NO_CONTENT", 204},
		{"MOVED_PERMANENTLY", 301}, {"FOUND", 302}, {"SEE_OTHER", 303}, {"NOT_MODIFIED", 304},
		{"TEMPORARY_REDIRECT", 307}, {"PERMANENT_REDIRECT", 308},
		{"BAD_REQUEST", 400}, {"UNAUTHORIZED", 401}, {"FORBIDDEN", 403}, {"NOT_FOUND", 404},
		{"METHOD_NOT_ALLOWED", 405}, {"NOT_ACCEPTABLE", 406}, {"REQUEST_TIMEOUT", 408}, {"CONFLICT", 409},
		{"GONE", 410}, {"PAYLOAD_TOO_LARGE", 413}, {"UNSUPPORTED_MEDIA_TYPE", 415},
		{"UNPROCESSABLE_ENTITY", 422}, {"TOO_MANY_REQUESTS", 429},
		{"INTERNAL_SERVER_ERROR", 500}, {"NOT_IMPLEMENTED", 501}, {"BAD_GATEWAY", 502},
		{"SERVICE_UNAVAILABLE", 503}, {"GATEWAY_TIMEOUT", 504},
	}
	out := make(map[string]*runtime.Value, len(entries))
	for _, e := range entries {
		out[e.name] = runtime.NumberVal(float64(e.code))
	}
	return runtime.ObjectVal(out)
}

func HttpModule(host httpHost) *runtime.Value {
	rt := httpRuntimeFor(host)
	m := make(map[string]*runtime.Value)

	m["request"] = httpFn("request", func(a []*runtime.Value) (*runtime.Value, error) {
		method, err := rt.argString("http.request", a, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		method = strings.ToUpper(method)
		if !httpValidHeaderName(method) {
			return runtime.Undefined, rt.fail(httpErrArgument, "http.request: invalid method")
		}
		return rt.doRequest("http.request", method, httpArg(a, 1), httpArg(a, 2))
	})

	for _, spec := range []struct{ name, method string }{
		{"get", "GET"}, {"post", "POST"}, {"put", "PUT"}, {"patch", "PATCH"},
		{"delete", "DELETE"}, {"head", "HEAD"},
	} {
		name, method := spec.name, spec.method
		m[name] = httpFn(name, func(a []*runtime.Value) (*runtime.Value, error) {
			return rt.doRequest("http."+name, method, httpArg(a, 0), httpArg(a, 1))
		})
	}

	m["createServer"] = httpFn("createServer", func(a []*runtime.Value) (*runtime.Value, error) {
		handler := httpArg(a, 0)
		if handler.Tag != runtime.TypeFunction {
			return runtime.Undefined, rt.fail(httpErrArgument, "http.createServer: handler must be a function")
		}
		cfg, err := rt.serverConfig(httpArg(a, 1))
		if err != nil {
			return runtime.Undefined, err
		}
		return rt.serverObject(&httpServerState{cfg: cfg, handler: handler}), nil
	})

	callMethod := func(fn, method string, a []*runtime.Value) (*runtime.Value, error) {
		target := httpArg(a, 0)
		if target.Tag != runtime.TypeObject {
			return runtime.Undefined, rt.fail(httpErrArgument, fn+": first argument must be a server")
		}
		f, ok := target.ObjVal[method]
		if !ok || f.Tag != runtime.TypeFunction {
			return runtime.Undefined, rt.fail(httpErrArgument, fn+": first argument must be a server")
		}
		return runtime.CallFunction(f, a[1:])
	}
	m["listen"] = httpFn("listen", func(a []*runtime.Value) (*runtime.Value, error) {
		return callMethod("http.listen", "listen", a)
	})
	m["close"] = httpFn("close", func(a []*runtime.Value) (*runtime.Value, error) {
		return callMethod("http.close", "close", a)
	})

	for _, name := range []string{"json", "text", "html", "redirect", "end"} {
		method := name
		m[method] = httpFn(method, func(a []*runtime.Value) (*runtime.Value, error) {
			target := httpArg(a, 0)
			if target.Tag != runtime.TypeObject {
				return runtime.Undefined, rt.fail(httpErrArgument, "http."+method+": first argument must be a response")
			}
			f, ok := target.ObjVal[method]
			if !ok || f.Tag != runtime.TypeFunction {
				return runtime.Undefined, rt.fail(httpErrArgument, "http."+method+": first argument must be a response")
			}
			return runtime.CallFunction(f, a[1:])
		})
	}

	m["error"] = httpFn("error", func(a []*runtime.Value) (*runtime.Value, error) {
		sv := httpArg(a, 0)
		if sv.Tag != runtime.TypeNumber || sv.NumVal != math.Trunc(sv.NumVal) || sv.NumVal < 400 || sv.NumVal > 599 {
			return runtime.Undefined, rt.fail(httpErrStatus, "http.error: status must be an integer between 400 and 599")
		}
		message, err := rt.argString("http.error", a, 1)
		if err != nil {
			return runtime.Undefined, err
		}
		code := "E_HTTP_ERROR"
		if cv := httpArg(a, 2); !httpIsNil(cv) {
			if cv.Tag != runtime.TypeString || cv.StrVal == "" {
				return runtime.Undefined, rt.fail(httpErrArgument, "http.error: code must be a non-empty string")
			}
			code = cv.StrVal
		}
		return httpErrorValue(int(sv.NumVal), message, code), nil
	})

	m["parseURL"] = httpFn("parseURL", func(a []*runtime.Value) (*runtime.Value, error) {
		raw, err := rt.argString("http.parseURL", a, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		u, perr := url.Parse(raw)
		if perr != nil {
			return runtime.Undefined, rt.fail(httpErrURL, "http.parseURL: "+perr.Error())
		}
		path := u.Path
		if path == "" && u.Host != "" {
			path = "/"
		}
		values, _ := url.ParseQuery(u.RawQuery)
		username := ""
		if u.User != nil {
			username = u.User.Username()
		}
		return runtime.ObjectVal(map[string]*runtime.Value{
			"protocol": runtime.StringVal(u.Scheme),
			"username": runtime.StringVal(username),
			"host":     runtime.StringVal(u.Host),
			"hostname": runtime.StringVal(u.Hostname()),
			"port":     runtime.StringVal(u.Port()),
			"path":     runtime.StringVal(path),
			"query":    httpQueryToObject(values, false),
			"search":   runtime.StringVal(u.RawQuery),
			"hash":     runtime.StringVal(u.Fragment),
		}), nil
	})

	m["buildURL"] = httpFn("buildURL", func(a []*runtime.Value) (*runtime.Value, error) {
		base, err := rt.argString("http.buildURL", a, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		u, perr := url.Parse(base)
		if perr != nil {
			return runtime.Undefined, rt.fail(httpErrURL, "http.buildURL: "+perr.Error())
		}
		extra, err := rt.valuesFromObject("http.buildURL", httpArg(a, 1))
		if err != nil {
			return runtime.Undefined, err
		}
		q := u.Query()
		for k, vals := range extra {
			for _, v := range vals {
				q.Add(k, v)
			}
		}
		u.RawQuery = q.Encode()
		return runtime.StringVal(u.String()), nil
	})

	m["parseQuery"] = httpFn("parseQuery", func(a []*runtime.Value) (*runtime.Value, error) {
		raw, err := rt.argString("http.parseQuery", a, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		all := false
		if v := httpArg(a, 1); !httpIsNil(v) {
			if v.Tag != runtime.TypeBool {
				return runtime.Undefined, rt.fail(httpErrArgument, "http.parseQuery: second argument must be a boolean")
			}
			all = v.BoolVal
		}
		values, perr := url.ParseQuery(strings.TrimPrefix(raw, "?"))
		if perr != nil {
			return runtime.Undefined, rt.fail(httpErrURL, "http.parseQuery: "+perr.Error())
		}
		return httpQueryToObject(values, all), nil
	})

	m["buildQuery"] = httpFn("buildQuery", func(a []*runtime.Value) (*runtime.Value, error) {
		values, err := rt.valuesFromObject("http.buildQuery", httpArg(a, 0))
		if err != nil {
			return runtime.Undefined, err
		}
		return runtime.StringVal(values.Encode()), nil
	})

	m["encode"] = httpFn("encode", func(a []*runtime.Value) (*runtime.Value, error) {
		s, err := rt.argString("http.encode", a, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		return runtime.StringVal(httpEncode(s)), nil
	})

	m["decode"] = httpFn("decode", func(a []*runtime.Value) (*runtime.Value, error) {
		s, err := rt.argString("http.decode", a, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		out, derr := url.PathUnescape(s)
		if derr != nil {
			return runtime.Undefined, rt.fail(httpErrURL, "http.decode: invalid percent-encoding")
		}
		return runtime.StringVal(out), nil
	})

	m["parseCookies"] = httpFn("parseCookies", func(a []*runtime.Value) (*runtime.Value, error) {
		header, err := rt.argString("http.parseCookies", a, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		r := &http.Request{Header: http.Header{"Cookie": []string{header}}}
		return httpCookiesToObj(r), nil
	})

	m["serializeCookie"] = httpFn("serializeCookie", func(a []*runtime.Value) (*runtime.Value, error) {
		name, err := rt.argString("http.serializeCookie", a, 0)
		if err != nil {
			return runtime.Undefined, err
		}
		value, err := rt.argString("http.serializeCookie", a, 1)
		if err != nil {
			return runtime.Undefined, err
		}
		line, err := rt.buildCookie("http.serializeCookie", name, value, httpArg(a, 2), false)
		if err != nil {
			return runtime.Undefined, err
		}
		return runtime.StringVal(line), nil
	})

	m["statusText"] = httpFn("statusText", func(a []*runtime.Value) (*runtime.Value, error) {
		v := httpArg(a, 0)
		if v.Tag != runtime.TypeNumber || v.NumVal != math.Trunc(v.NumVal) {
			return runtime.Undefined, rt.fail(httpErrStatus, "http.statusText: status must be an integer")
		}
		return runtime.StringVal(http.StatusText(int(v.NumVal))), nil
	})

	m["status"] = httpStatusConstants()

	return runtime.ObjectVal(m)
}

func HttpStaticModule(host httpHost) *runtime.Value {
	rt := httpRuntimeFor(host)
	return runtime.ObjectVal(map[string]*runtime.Value{
		"create": httpFn("create", func(a []*runtime.Value) (*runtime.Value, error) {
			return rt.staticCreate(a)
		}),
	})
}
