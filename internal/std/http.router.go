package std

import (
	"lunex/internal/runtime"
)

const httpRouterSource = `val _http = @import("std.http")

val _MAX_SEGMENTS = 64
val _MAX_PATH_LENGTH = 2048

fn _invalid(message) {
  throw _http.error(500, "router: " + message, "E_ROUTER_INVALID_ROUTE")
}

fn _isNameChar(code) {
  (code >= 48 && code <= 57) || (code >= 65 && code <= 90) || (code >= 97 && code <= 122) || code == 95
}

fn _validParamName(name) {
  if name.length == 0 {
    false
  } else {
    val first = name.charCodeAt(0)
    var valid = !(first >= 48 && first <= 57)
    var i = 0
    while i < name.length && valid {
      if !_isNameChar(name.charCodeAt(i)) {
        valid = false
      }
      i = i + 1
    }
    valid
  }
}

fn _define(method, path, handler) {
  if typeof(path) != "string" {
    _invalid("path must be a string")
  }
  if typeof(handler) != "function" {
    _invalid("handler for " + method + " " + path + " must be a function")
  }
  val definition = { kind: "route", method: method, path: path, handler: handler }
  definition
}

fn _get(path, handler) {
  _define("GET", path, handler)
}

fn _post(path, handler) {
  _define("POST", path, handler)
}

fn _put(path, handler) {
  _define("PUT", path, handler)
}

fn _patch(path, handler) {
  _define("PATCH", path, handler)
}

fn _delete(path, handler) {
  _define("DELETE", path, handler)
}

fn _head(path, handler) {
  _define("HEAD", path, handler)
}

fn _options(path, handler) {
  _define("OPTIONS", path, handler)
}

fn _all(path, handler) {
  _define("*", path, handler)
}

fn _compilePath(path) {
  if !path.startsWith("/") {
    _invalid("path '" + path + "' must start with '/'")
  }
  if path.length > _MAX_PATH_LENGTH {
    _invalid("path '" + path + "' is too long")
  }
  if path.includes("?") || path.includes("#") || path.includes("//") {
    _invalid("path '" + path + "' contains an invalid sequence")
  }
  var body = path.slice(1)
  if body.length > 0 && body.endsWith("/") {
    body = body.slice(0, body.length - 1)
  }
  val segments = []
  val names = []
  if body.length > 0 {
    val parts = body.split("/")
    if parts.length > _MAX_SEGMENTS {
      _invalid("path '" + path + "' has too many segments")
    }
    var i = 0
    while i < parts.length {
      val part = parts[i]
      if part.startsWith(":") {
        val name = part.slice(1)
        if !_validParamName(name) {
          _invalid("path '" + path + "' has an invalid parameter name '" + name + "'")
        }
        if names.includes(name) {
          _invalid("path '" + path + "' repeats the parameter '" + name + "'")
        }
        names.push(name)
        segments.push({ kind: "param", value: name })
      } elif part == "*" {
        if i != parts.length - 1 {
          _invalid("path '" + path + "' uses '*' before the last segment")
        }
        names.push("*")
        segments.push({ kind: "wildcard", value: "*" })
      } else {
        if part.includes("*") {
          _invalid("path '" + path + "' uses '*' inside a segment")
        }
        segments.push({ kind: "static", value: part })
      }
      i = i + 1
    }
  }
  val compiled = { segments: segments, names: names }
  compiled
}

fn _newNode() {
  val node = { children: {}, param: null, wildcard: null, handlers: {} }
  node
}

fn _insert(root, compiled, method, handler, path) {
  var node = root
  var i = 0
  while i < compiled.segments.length {
    val segment = compiled.segments[i]
    if segment.kind == "static" {
      val key = "s:" + segment.value
      if node.children[key] == null {
        node.children[key] = _newNode()
      }
      node = node.children[key]
    } elif segment.kind == "param" {
      if node.param == null {
        node.param = _newNode()
      }
      node = node.param
    } else {
      if node.wildcard == null {
        node.wildcard = _newNode()
      }
      node = node.wildcard
    }
    i = i + 1
  }
  if node.handlers[method] != null {
    _invalid("duplicate route " + method + " " + path)
  }
  val entry = { handler: handler, names: compiled.names, path: path }
  node.handlers[method] = entry
  null
}

fn _splitPath(rawPath) {
  var result = null
  if typeof(rawPath) == "string" && rawPath.startsWith("/") && rawPath.length <= _MAX_PATH_LENGTH && !rawPath.includes("//") {
    var body = rawPath.slice(1)
    if body.endsWith("/") {
      body = body.slice(0, body.length - 1)
    }
    val segments = []
    var valid = true
    if body.length > 0 {
      val parts = body.split("/")
      if parts.length > _MAX_SEGMENTS {
        valid = false
      }
      var i = 0
      while i < parts.length && valid {
        var decoded = null
        try {
          decoded = _http.decode(parts[i])
        } catch (failure) {
          valid = false
        }
        if typeof(decoded) == "string" {
          segments.push(decoded)
        } else {
          valid = false
        }
        i = i + 1
      }
    }
    if valid {
      result = segments
    }
  }
  result
}

fn _collect(node, values, state) {
  val method = state.method
  var entry = node.handlers[method]
  if entry == null && method == "HEAD" {
    entry = node.handlers["GET"]
  }
  if entry == null {
    entry = node.handlers["*"]
  }
  if entry != null {
    state.found = entry
    state.values = values.slice(0)
  } else {
    each name in node.handlers {
      if !state.allowed.includes(name) {
        state.allowed.push(name)
      }
    }
  }
  null
}

fn _search(node, segments, index, values, state) {
  if state.found == null {
    if index == segments.length {
      _collect(node, values, state)
    } else {
      val segment = segments[index]
      val next = node.children["s:" + segment]
      if next != null {
        _search(next, segments, index + 1, values, state)
      }
      if state.found == null && node.param != null {
        values.push(segment)
        _search(node.param, segments, index + 1, values, state)
        if state.found == null {
          values.pop()
        }
      }
    }
    if state.found == null && node.wildcard != null {
      values.push(segments.slice(index).join("/"))
      _collect(node.wildcard, values, state)
      if state.found == null {
        values.pop()
      }
    }
  }
  null
}

fn _lookup(root, method, segments) {
  val state = { method: method, found: null, values: null, allowed: [] }
  _search(root, segments, 0, [], state)
  state
}

fn _buildParams(entry, values) {
  val params = {}
  var i = 0
  while i < entry.names.length {
    params[entry.names[i]] = values[i]
    i = i + 1
  }
  params
}

fn _allowList(methods) {
  val list = methods.slice(0)
  if list.length > 0 {
    if list.includes("GET") && !list.includes("HEAD") {
      list.push("HEAD")
    }
    if !list.includes("OPTIONS") {
      list.push("OPTIONS")
    }
    list.sort()
  }
  list
}

fn _reply(res, status, message, code) {
  res.json({ error: message, code: code }, status)
}

fn _run(settings, handler, req, res) {
  if settings.onError == null {
    handler(req, res)
  } else {
    try {
      handler(req, res)
    } catch (failure) {
      settings.onError(failure, req, res)
    }
  }
}

fn _dispatch(root, settings, req, res) {
  val empty = {}
  req.params = empty
  val segments = _splitPath(req.rawPath)
  if segments == null {
    _reply(res, 400, "Bad Request", "E_ROUTER_BAD_PATH")
  } else {
    val state = _lookup(root, req.method, segments)
    if state.found != null {
      req.params = _buildParams(state.found, state.values)
      _run(settings, state.found.handler, req, res)
    } elif state.allowed.length > 0 {
      res.setHeader("Allow", _allowList(state.allowed).join(", "))
      if req.method == "OPTIONS" {
        res.end("", 204)
      } else {
        _reply(res, 405, "Method Not Allowed", "E_ROUTER_METHOD_NOT_ALLOWED")
      }
    } elif settings.notFound != null {
      _run(settings, settings.notFound, req, res)
    } else {
      _reply(res, 404, "Not Found", "E_ROUTER_NOT_FOUND")
    }
  }
}

fn _find(root, method, rawPath) {
  val segments = _splitPath(rawPath)
  val result = { found: false, params: {}, pattern: null, allowed: [] }
  if segments != null {
    val state = _lookup(root, method, segments)
    if state.found != null {
      result.found = true
      result.params = _buildParams(state.found, state.values)
      result.pattern = state.found.path
    } else {
      result.allowed = _allowList(state.allowed)
    }
  }
  result
}

fn _readOptions(options) {
  val settings = { notFound: null, onError: null, server: null }
  if options != null {
    if typeof(options) != "object" {
      _invalid("options must be an object")
    }
    each key in options {
      if key != "notFound" && key != "onError" && key != "server" {
        _invalid("unknown option '" + key + "'")
      }
    }
    if options.notFound != null {
      if typeof(options.notFound) != "function" {
        _invalid("option 'notFound' must be a function")
      }
      settings.notFound = options.notFound
    }
    if options.onError != null {
      if typeof(options.onError) != "function" {
        _invalid("option 'onError' must be a function")
      }
      settings.onError = options.onError
    }
    if options.server != null {
      if typeof(options.server) != "object" {
        _invalid("option 'server' must be an object")
      }
      settings.server = options.server
    }
  }
  settings
}

fn create(routes, options = undefined) {
  if typeof(routes) != "array" {
    _invalid("routes must be an array")
  }
  val settings = _readOptions(options)
  val root = _newNode()
  var i = 0
  while i < routes.length {
    val route = routes[i]
    if typeof(route) != "object" || route.kind != "route" {
      _invalid("routes[" + str(i) + "] is not a route definition")
    }
    if typeof(route.method) != "string" || typeof(route.path) != "string" || typeof(route.handler) != "function" {
      _invalid("routes[" + str(i) + "] is malformed")
    }
    _insert(root, _compilePath(route.path), route.method, route.handler, route.path)
    i = i + 1
  }

  fn handle(req, res) {
    _dispatch(root, settings, req, res)
  }

  fn find(method, path) {
    _find(root, str(method).toUpperCase(), path)
  }

  fn listen(port, host = undefined, onReady = undefined) {
    val server = _http.createServer(handle, settings.server)
    server.listen(port, host, onReady)
  }

  val api = { handle: handle, find: find, listen: listen }
  api
}

val __module__ = {
  "get": _get,
  post: _post,
  put: _put,
  patch: _patch,
  "delete": _delete,
  head: _head,
  options: _options,
  all: _all,
  create: create
}
`

func HttpRouterModule(interp interface {
	ExecAsModule(source, filename string) (*runtime.Value, error)
}) *runtime.Value {
	mod, err := interp.ExecAsModule(httpRouterSource, "http_router")
	if err != nil {
		panic(err)
	}
	return mod
}
