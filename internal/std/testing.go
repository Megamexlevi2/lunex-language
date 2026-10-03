package std

import "lunex/internal/runtime"

const testingSource = `val _io = @import("std.io")
val _os = @import("std.os")
val _fs = @import("std.fs")
val _regex = @import("std.regex")

var _tests = []
var _groups = []
var _root = {
  name: "",
  path: "",
  parent: null,
  tags: [],
  beforeAll: [],
  afterAll: [],
  beforeEach: [],
  afterEach: [],
  started: false,
  blocked: false,
  setupError: null
}
var _group = _root
var _running = false
var _current = null

_groups.push(_root)

fn _emptyObject() {
  val value = { __lunex_testing_empty__: true }
  delete value.__lunex_testing_empty__
  value
}

fn _text(value) {
  if typeof(value) == "undefined" {
    "undefined"
  } elif value == null {
    "null"
  } elif typeof(value) == "string" {
    value
  } else {
    str(value)
  }
}

fn _displayName(entry) {
  if entry.group == "" {
    entry.name
  } else {
    entry.group + " / " + entry.name
  }
}

fn _fail(message, actual = undefined, expected = undefined) {
  throw {
    name: "AssertionError",
    code: "E_ASSERT",
    message: message,
    actual: actual,
    expected: expected
  }
}

fn _skipFailure(message = "test skipped") {
  throw {
    name: "SkipTest",
    code: "E_SKIP",
    message: message
  }
}

fn _todoFailure(message = "test marked todo") {
  throw {
    name: "TodoTest",
    code: "E_TODO",
    message: message
  }
}

fn _beginAssertion() {
  if _current != null {
    _current.assertions = _current.assertions + 1
  }
  true
}

fn _noteValue(value) {
  if _current != null {
    _current.notes.push(_text(value))
  }
  true
}

fn _deepEqual(left, right, depth = 0) {
  if depth > 128 {
    false
  } elif typeof(left) != typeof(right) {
    false
  } elif typeof(left) == "undefined" {
    true
  } elif left == null && right == null {
    true
  } elif typeof(left) == "number" {
    if isNaN(left) && isNaN(right) {
      true
    } else {
      left == right
    }
  } elif typeof(left) == "string" || typeof(left) == "boolean" {
    left == right
  } elif typeof(left) == "array" {
    if left.length != right.length {
      false
    } else {
      var i = 0
      var equal = true
      while i < left.length {
        if !_deepEqual(left[i], right[i], depth + 1) {
          equal = false
          break
        }
        i = i + 1
      }
      equal
    }
  } elif typeof(left) == "object" {
    val leftKeys = Object.keys(left)
    val rightKeys = Object.keys(right)
    if leftKeys.length != rightKeys.length {
      false
    } else {
      leftKeys.sort()
      rightKeys.sort()
      var i = 0
      var equal = true
      while i < leftKeys.length {
        val leftKey = leftKeys[i]
        if leftKey != rightKeys[i] {
          equal = false
          break
        }
        if !_deepEqual(left[leftKey], right[leftKey], depth + 1) {
          equal = false
          break
        }
        i = i + 1
      }
      equal
    }
  } else {
    left == right
  }
}

fn _escape(value) {
  value.replaceAll("\\", "\\\\").replaceAll("\"", "\\\"").replaceAll("\r", "\\r").replaceAll("\n", "\\n")
}

fn _snapshotText(value, depth = 0) {
  if depth > 64 {
    "<depth-limit>"
  } elif typeof(value) == "undefined" {
    "undefined"
  } elif value == null {
    "null"
  } elif typeof(value) == "string" {
    "\"" + _escape(value) + "\""
  } elif typeof(value) == "number" {
    if isNaN(value) {
      "NaN"
    } elif value == Infinity {
      "Infinity"
    } elif value == -Infinity {
      "-Infinity"
    } else {
      str(value)
    }
  } elif typeof(value) == "boolean" {
    str(value)
  } elif typeof(value) == "array" {
    var items = []
    var i = 0
    while i < value.length {
      items.push(_snapshotText(value[i], depth + 1))
      i = i + 1
    }
    "[" + items.join(", ") + "]"
  } elif typeof(value) == "object" {
    val keys = Object.keys(value)
    keys.sort()
    var items = []
    var i = 0
    while i < keys.length {
      val key = keys[i]
      items.push("\"" + _escape(key) + "\": " + _snapshotText(value[key], depth + 1))
      i = i + 1
    }
    "{" + items.join(", ") + "}"
  } else {
    "<" + typeof(value) + ">"
  }
}

fn _assertCallback(name, body, label) {
  if typeof(name) != "string" {
    _fail(label + " name must be a string")
  }
  if typeof(body) != "function" {
    _fail(label + " body must be a function")
  }
  true
}

fn _normalizeTags(value) {
  val result = []
  if typeof(value) == "string" {
    result.push(value)
  } elif typeof(value) == "array" {
    each item in value {
      if typeof(item) == "string" && item != "" {
        if !result.includes(item) {
          result.push(item)
        }
      }
    }
  }
  result
}

fn _mergeTags(parent, local) {
  val result = []
  each item in parent {
    if !result.includes(item) {
      result.push(item)
    }
  }
  each item in local {
    if !result.includes(item) {
      result.push(item)
    }
  }
  result
}

fn _normalizeRetries(value) {
  if typeof(value) != "number" || isNaN(value) || value < 0 {
    0
  } else {
    parseInt(value)
  }
}

fn _pathFor(group) {
  if group.path == "" {
    []
  } else {
    group.path.split(" /")
  }
}

fn _chain(group) {
  val result = []
  var current = group
  while current != null {
    result.push(current)
    current = current.parent
  }
  result.reverse()
  result
}

fn _runHooks(hooks) {
  var error = null
  each hook in hooks {
    try {
      hook()
    } catch e {
      if error == null {
        error = e
      }
    }
  }
  error
}

fn _runHooksReverse(hooks) {
  var error = null
  var i = hooks.length - 1
  while i >= 0 {
    try {
      hooks[i]()
    } catch e {
      if error == null {
        error = e
      }
    }
    i = i - 1
  }
  error
}

fn _assertCollecting(name) {
  if _running {
    _fail("testing." + name + " cannot be used while a test run is active")
  }
  true
}

fn _collectBeforeEach(group) {
  val result = []
  val chain = _chain(group)
  each item in chain {
    each hook in item.beforeEach {
      result.push(hook)
    }
  }
  result
}

fn _collectAfterEach(group) {
  val result = []
  val chain = _chain(group)
  chain.reverse()
  each item in chain {
    var i = item.afterEach.length - 1
    while i >= 0 {
      result.push(item.afterEach[i])
      i = i - 1
    }
  }
  result
}

fn _ensureGroups(group) {
  val chain = _chain(group)
  var error = null
  each item in chain {
    if item.blocked {
      error = item.setupError
      break
    }
    if !item.started {
      item.started = true
      val setup = _runHooks(item.beforeAll)
      if setup != null {
        item.blocked = true
        item.setupError = setup
        error = setup
        break
      }
    }
  }
  error
}

fn _finishGroups() {
  var error = null
  var i = _groups.length - 1
  while i >= 0 {
    val group = _groups[i]
    if group.started {
      val currentError = _runHooksReverse(group.afterAll)
      if currentError != null && error == null {
        error = currentError
      }
    }
    i = i - 1
  }
  error
}

fn _matchesTags(entry, required, excluded) {
  var matched = true
  if required.length > 0 {
    each tag in required {
      if !entry.tags.includes(tag) {
        matched = false
        break
      }
    }
  }
  if matched && excluded.length > 0 {
    each tag in excluded {
      if entry.tags.includes(tag) {
        matched = false
        break
      }
    }
  }
  matched
}

fn _matchesFilter(entry, filter) {
  if typeof(filter) != "string" || filter == "" {
    true
  } else {
    _displayName(entry).includes(filter)
  }
}

fn _errorText(error) {
  if typeof(error) == "object" && error != null {
    val message = if typeof(error.message) == "undefined" { "" } else { str(error.message) }
    val code = if typeof(error.code) == "undefined" { "" } else { str(error.code) }
    if code == "" {
      message
    } elif message == "" {
      code
    } else {
      code + ": " + message
    }
  } else {
    _text(error)
  }
}

fn _matchError(error, expected) {
  if typeof(expected) == "undefined" || expected == null {
    true
  } elif typeof(expected) == "string" {
    if typeof(error) == "object" && error != null {
      val code = if typeof(error.code) == "undefined" { "" } else { str(error.code) }
      val name = if typeof(error.name) == "undefined" { "" } else { str(error.name) }
      code == expected || name == expected
    } else {
      str(error) == expected
    }
  } elif typeof(expected) == "function" {
    try {
      expected(error) == true
    } catch e {
      false
    }
  } elif typeof(expected) == "object" {
    var matched = true
    val keys = Object.keys(expected)
    each key in keys {
      if typeof(error) != "object" || error == null || !_deepEqual(error[key], expected[key]) {
        matched = false
        break
      }
    }
    matched
  } else {
    _deepEqual(error, expected)
  }
}

fn _register(entry) {
  _assertCollecting("test")
  _tests.push(entry)
  entry.name
}

fn test(name, body, options = {}) {
  _assertCallback(name, body, "testing.test")
  var enabled = true
  var todoCase = false
  var reason = ""
  var retries = 0
  var timeout = 0
  var tags = _group.tags
  if typeof(options) == "object" {
    if options.skip == true {
      enabled = false
      reason = options.reason ?? "skipped"
    }
    if options.todo == true {
      todoCase = true
      reason = options.reason ?? "todo"
    }
    retries = _normalizeRetries(options.retries)
    if typeof(options.timeout) == "number" && options.timeout >= 0 {
      timeout = options.timeout
    }
    tags = _mergeTags(tags, _normalizeTags(options.tags))
  }
  _register({
    name: name,
    body: body,
    group: _group.path,
    groupState: _group,
    enabled: enabled,
    todo: todoCase,
    reason: reason,
    retries: retries,
    timeout: timeout,
    tags: tags
  })
}

fn _caseBody(body, value, index) {
  fn _case() {
    body(value, index)
  }
  _case
}

fn cases(name, values, body, options = {}) {
  if typeof(name) != "string" || typeof(body) != "function" || typeof(values) != "array" {
    _fail("testing.cases expects a name, an array, and a function")
  }
  var i = 0
  while i < values.length {
    val value = values[i]
    val index = i
    val caseBody = _caseBody(body, value, index)
    test(name + "[" + str(index) + "]", caseBody, options)
    i = i + 1
  }
  values.length
}

fn group(name, body, options = {}) {
  _assertCollecting("group")
  _assertCallback(name, body, "testing.group")
  val localTags = if typeof(options) == "object" { _normalizeTags(options.tags) } else { [] }
  val path = if _group.path == "" { name } else { _group.path + " / " + name }
  val item = {
    name: name,
    path: path,
    parent: _group,
    tags: _mergeTags(_group.tags, localTags),
    beforeAll: [],
    afterAll: [],
    beforeEach: [],
    afterEach: [],
    started: false,
    blocked: false,
    setupError: null
  }
  _groups.push(item)
  val previous = _group
  _group = item
  try {
    body()
  } finally {
    _group = previous
  }
  name
}

fn beforeAll(body) {
  _assertCollecting("beforeAll")
  _assertCallback("beforeAll", body, "testing.beforeAll")
  _group.beforeAll.push(body)
  true
}

fn afterAll(body) {
  _assertCollecting("afterAll")
  _assertCallback("afterAll", body, "testing.afterAll")
  _group.afterAll.push(body)
  true
}

fn beforeEach(body) {
  _assertCollecting("beforeEach")
  _assertCallback("beforeEach", body, "testing.beforeEach")
  _group.beforeEach.push(body)
  true
}

fn afterEach(body) {
  _assertCollecting("afterEach")
  _assertCallback("afterEach", body, "testing.afterEach")
  _group.afterEach.push(body)
  true
}

fn ok(value, message = "expected value to be truthy") {
  _beginAssertion()
  if !value {
    _fail(message, value, true)
  }
  true
}

fn falsey(value, message = "expected value to be falsy") {
  _beginAssertion()
  if value {
    _fail(message, value, false)
  }
  true
}

fn _isNull(value, message = "expected value to be null") {
  _beginAssertion()
  if value != null {
    _fail(message, value, null)
  }
  true
}

fn defined(value, message = "expected value to be defined") {
  _beginAssertion()
  if typeof(value) == "undefined" {
    _fail(message, value, "defined")
  }
  true
}

fn equal(actual, expected, message = "") {
  _beginAssertion()
  if !_deepEqual(actual, expected) {
    val detail = if message == "" { "values are not equal" } else { message }
    _fail(detail + "\n  expected: " + _text(expected) + "\n  received: " + _text(actual), actual, expected)
  }
  true
}

fn notEqual(actual, expected, message = "") {
  _beginAssertion()
  if _deepEqual(actual, expected) {
    val detail = if message == "" { "values are equal" } else { message }
    _fail(detail + "\n  unexpected: " + _text(actual), actual, expected)
  }
  true
}

fn same(actual, expected, message = "") {
  _beginAssertion()
  if typeof(actual) != typeof(expected) || !_deepEqual(actual, expected) {
    val detail = if message == "" { "values are not the same" } else { message }
    _fail(detail + "\n  expected: " + _text(expected) + "\n  received: " + _text(actual), actual, expected)
  }
  true
}

fn contains(value, expected, message = "") {
  _beginAssertion()
  var found = false
  if typeof(value) == "string" {
    found = value.includes(str(expected))
  } elif typeof(value) == "array" {
    var i = 0
    while i < value.length {
      if _deepEqual(value[i], expected) {
        found = true
        break
      }
      i = i + 1
    }
  } elif typeof(value) == "object" && value != null {
    val key = str(expected)
    found = typeof(value[key]) != "undefined"
  }
  if !found {
    val detail = if message == "" { "value does not contain the expected item" } else { message }
    _fail(detail, value, expected)
  }
  true
}

fn type(value, expected, message = "") {
  _beginAssertion()
  if typeof(expected) != "string" || typeof(value) != expected {
    val detail = if message == "" { "unexpected value type" } else { message }
    _fail(detail + "\n  expected: " + _text(expected) + "\n  received: " + typeof(value), typeof(value), expected)
  }
  true
}

fn length(value, expected, message = "") {
  _beginAssertion()
  if typeof(value) != "string" && typeof(value) != "array" {
    _fail("length assertion expects a string or array", value, expected)
  }
  if value.length != expected {
    val detail = if message == "" { "unexpected length" } else { message }
    _fail(detail + "\n  expected: " + str(expected) + "\n  received: " + str(value.length), value.length, expected)
  }
  true
}

fn empty(value, message = "expected value to be empty") {
  _beginAssertion()
  var isEmpty = false
  if value == null || typeof(value) == "undefined" {
    isEmpty = true
  } elif typeof(value) == "string" || typeof(value) == "array" {
    isEmpty = value.length == 0
  } elif typeof(value) == "object" {
    isEmpty = Object.keys(value).length == 0
  }
  if !isEmpty {
    _fail(message, value, "empty")
  }
  true
}

fn approx(actual, expected, tolerance = 0.000001, message = "") {
  _beginAssertion()
  if typeof(actual) != "number" || typeof(expected) != "number" || typeof(tolerance) != "number" || tolerance < 0 || isNaN(actual) || isNaN(expected) || Math.abs(actual - expected) > tolerance {
    val detail = if message == "" { "values are not approximately equal" } else { message }
    _fail(detail + "\n  expected: " + str(expected) + " ± " + str(tolerance) + "\n  received: " + str(actual), actual, expected)
  }
  true
}

fn _inRange(value, minimum, maximum, message = "") {
  _beginAssertion()
  if typeof(value) != "number" || value < minimum || value > maximum {
    val detail = if message == "" { "value is outside the expected range" } else { message }
    _fail(detail + "\n  range: [" + str(minimum) + ", " + str(maximum) + "]\n  received: " + str(value), value, { min: minimum, max: maximum })
  }
  true
}

fn _matchesPattern(value, pattern, message = "") {
  _beginAssertion()
  if typeof(value) != "string" || typeof(pattern) != "string" || !_regex.test(value, pattern) {
    val detail = if message == "" { "value does not match the expected pattern" } else { message }
    _fail(detail + "\n  expected: " + _text(pattern) + "\n  received: " + _text(value), value, pattern)
  }
  true
}

fn raises(body, expected = undefined, message = "") {
  _beginAssertion()
  if typeof(body) != "function" {
    _fail("testing.raises expects a function")
  }
  var raised = false
  var error = undefined
  try {
    body()
  } catch e {
    raised = true
    error = e
  }
  if !raised {
    _fail(if message == "" { "expected function to raise" } else { message })
  }
  if !_matchError(error, expected) {
    _fail("raised error did not match expectation\n  expected: " + _text(expected) + "\n  received: " + _errorText(error), error, expected)
  }
  error
}

fn notRaises(body, message = "") {
  _beginAssertion()
  if typeof(body) != "function" {
    _fail("testing.notRaises expects a function")
  }
  try {
    body()
  } catch e {
    _fail(if message == "" { "expected function not to raise" } else { message }, e)
  }
  true
}

fn fail(message = "test failed") {
  _beginAssertion()
  _fail(message)
}

fn skip(reason = "skipped") {
  _skipFailure(_text(reason))
}

fn todo(reason = "todo") {
  _todoFailure(_text(reason))
}

fn note(message) {
  _noteValue(message)
}

fn plan(count) {
  if _current == null {
    _fail("testing.plan must be called inside a test")
  }
  if typeof(count) != "number" || count < 0 || count != parseInt(count) {
    _fail("testing.plan expects a non-negative integer")
  }
  _current.expectedAssertions = count
  true
}

fn snapshot(value, path, options = {}) {
  _beginAssertion()
  if typeof(path) != "string" || path == "" {
    _fail("testing.snapshot expects a file path")
  }
  val expected = _snapshotText(value)
  var update = false
  if typeof(options) == "object" {
    update = options.update == true
  }
  if update {
    if !_fs.writeFile(path, expected + "\n") {
      _fail("failed to write snapshot '" + path + "'")
    }
    true
  } else {
    val actual = _fs.readFile(path)
    if actual == null {
      _fail("snapshot does not exist: " + path, undefined, expected)
    }
    val normalized = actual.trimEnd()
    if normalized != expected {
      _fail("snapshot mismatch: " + path + "\n  expected: " + expected + "\n  received: " + normalized, normalized, expected)
    }
    true
  }
}

fn _runAttempt(entry) {
  val started = _os.hrtime()
  val state = {
    assertions: 0,
    expectedAssertions: undefined,
    notes: [],
    test: entry.name,
    group: entry.group
  }
  _current = state
  var bodyError = null
  var setupError = null
  var teardownError = null
  val beforeHooks = _collectBeforeEach(entry.groupState)
  val afterHooks = _collectAfterEach(entry.groupState)
  setupError = _runHooks(beforeHooks)
  if setupError == null {
    try {
      entry.body()
    } catch e {
      bodyError = e
    }
  }
  teardownError = _runHooks(afterHooks)
  _current = null
  val duration = _os.hrtime() - started
  var assertionError = null
  if bodyError == null && setupError == null && teardownError == null && typeof(state.expectedAssertions) == "number" && state.assertions != state.expectedAssertions {
    assertionError = {
      name: "AssertionPlanError",
      code: "E_PLAN",
      message: "assertion plan expected " + str(state.expectedAssertions) + " assertion(s), received " + str(state.assertions),
      actual: state.assertions,
      expected: state.expectedAssertions
    }
  }
  var error = bodyError
  if error == null {
    error = setupError
  }
  if error == null {
    error = teardownError
  }
  if error == null {
    error = assertionError
  }
  var status = "pass"
  if error != null {
    if typeof(error) == "object" && error != null && error.code == "E_SKIP" {
      status = "skip"
    } elif typeof(error) == "object" && error != null && error.code == "E_TODO" {
      status = "todo"
    } else {
      status = "fail"
    }
  }
  if entry.timeout > 0 && duration > entry.timeout && status == "pass" {
    status = "fail"
    error = {
      name: "TimeoutError",
      code: "E_TIMEOUT",
      message: "test exceeded " + str(entry.timeout) + "ms",
      actual: duration,
      expected: entry.timeout
    }
  }
  {
    status: status,
    duration: duration,
    assertions: state.assertions,
    expectedAssertions: state.expectedAssertions,
    notes: state.notes,
    setupError: setupError,
    cleanupError: teardownError,
    error: error
  }
}

fn _runEntry(entry, defaultRetries) {
  var retries = entry.retries
  if retries == 0 {
    retries = defaultRetries
  }
  var attempt = 0
  var totalDuration = 0
  var final = null
  while attempt <= retries {
    val result = _runAttempt(entry)
    totalDuration = totalDuration + result.duration
    final = result
    if result.status == "pass" || result.status == "skip" || result.status == "todo" {
      break
    }
    attempt = attempt + 1
  }
  final.attempts = attempt + 1
  final.duration = totalDuration
  final.flaky = final.status == "pass" && attempt > 0
  final
}

fn _result(entry, result) {
  {
    name: _displayName(entry),
    group: entry.group,
    tags: entry.tags,
    status: result.status,
    duration: result.duration,
    attempts: result.attempts,
    flaky: result.flaky,
    assertions: result.assertions,
    expectedAssertions: result.expectedAssertions,
    notes: result.notes,
    setupError: result.setupError,
    cleanupError: result.cleanupError,
    error: result.error
  }
}

fn list() {
  val result = []
  each entry in _tests {
    result.push({
      name: _displayName(entry),
      group: entry.group,
      tags: entry.tags,
      skip: !entry.enabled,
      todo: entry.todo,
      retries: entry.retries,
      timeout: entry.timeout
    })
  }
  result
}

fn run(options = {}) {
  if _running {
    _fail("testing.run cannot be called while a test run is active")
  }
  _running = true
  var filter = ""
  var requiredTags = []
  var excludedTags = []
  var failFast = false
  var quiet = false
  var failOnFlaky = false
  var retries = 0
  if typeof(options) == "object" {
    filter = options.filter ?? ""
    requiredTags = _normalizeTags(options.tags)
    excludedTags = _normalizeTags(options.excludeTags)
    failFast = options.failFast == true
    quiet = options.quiet == true
    failOnFlaky = options.failOnFlaky == true
    retries = _normalizeRetries(options.retries)
  }

  var total = 0
  var passed = 0
  var failed = 0
  var skipped = 0
  var todoCount = 0
  var unexpected = 0
  var flaky = 0
  var duration = 0
  var stopped = false
  val failures = []
  val results = []

  each entry in _tests {
    if stopped {
      break
    }
    if !_matchesFilter(entry, filter) || !_matchesTags(entry, requiredTags, excludedTags) {
      continue
    }
    total = total + 1
    if !entry.enabled {
      skipped = skipped + 1
      val skippedResult = {
        status: "skip",
        duration: 0,
        attempts: 0,
        flaky: false,
        assertions: 0,
        expectedAssertions: undefined,
        notes: [],
        setupError: null,
        cleanupError: null,
        error: { code: "E_SKIP", message: entry.reason }
      }
      results.push(_result(entry, skippedResult))
      if !quiet {
        _io.log("- " + _displayName(entry) + " [" + entry.reason + "]")
      }
      continue
    }

    val setupError = _ensureGroups(entry.groupState)
    if setupError != null {
      failed = failed + 1
      val setupResult = {
        status: "fail",
        duration: 0,
        attempts: 0,
        flaky: false,
        assertions: 0,
        expectedAssertions: undefined,
        notes: [],
        setupError: setupError,
        cleanupError: null,
        error: setupError
      }
      results.push(_result(entry, setupResult))
      failures.push({ test: _displayName(entry), error: setupError })
      _io.err("✗ " + _displayName(entry) + "\n  " + _errorText(setupError))
      if failFast {
        stopped = true
      }
      continue
    }

    val result = _runEntry(entry, retries)
    duration = duration + result.duration
    val stored = _result(entry, result)
    results.push(stored)

    if entry.todo {
      if result.status == "todo" || result.status == "fail" {
        todoCount = todoCount + 1
        if !quiet {
          val text = if result.status == "todo" { entry.reason } else { _errorText(result.error) }
          _io.log("○ " + _displayName(entry) + " [" + text + "]")
        }
      } elif result.status == "skip" {
        skipped = skipped + 1
        if !quiet {
          _io.log("- " + _displayName(entry) + " [skipped]")
        }
      } else {
        unexpected = unexpected + 1
        failed = failed + 1
        failures.push({ test: _displayName(entry), error: { code: "E_TODO_PASS", message: "todo test unexpectedly passed" } })
        if !quiet {
          _io.err("✗ " + _displayName(entry) + " [unexpected pass]")
        }
        if failFast {
          stopped = true
        }
      }
    } elif result.status == "pass" {
      passed = passed + 1
      if result.flaky {
        flaky = flaky + 1
      }
      if !quiet {
        val suffix = if result.flaky { " [flaky, passed after retry]" } else { "" }
        _io.log("✓ " + _displayName(entry) + suffix)
      }
      if failOnFlaky && result.flaky {
        failed = failed + 1
        failures.push({ test: _displayName(entry), error: { code: "E_FLAKY", message: "test passed only after retry" } })
        if !quiet {
          _io.err("✗ " + _displayName(entry) + " [flaky]")
        }
        if failFast {
          stopped = true
        }
      }
    } elif result.status == "skip" {
      skipped = skipped + 1
      if !quiet {
        _io.log("- " + _displayName(entry) + " [skipped]")
      }
    } else {
      failed = failed + 1
      failures.push({ test: _displayName(entry), error: result.error })
      _io.err("✗ " + _displayName(entry) + "\n  " + _errorText(result.error))
      if failFast {
        stopped = true
      }
    }
  }

  val afterAllError = _finishGroups()
  if afterAllError != null {
    failures.push({ test: "<afterAll>", error: afterAllError })
  }

  _running = false
  val ok = failed == 0 && unexpected == 0 && (afterAllError == null)
  if !quiet {
    _io.log("")
    _io.log("Tests: " + str(total) + " | passed: " + str(passed) + " | failed: " + str(failed) + " | skipped: " + str(skipped) + " | todo: " + str(todoCount) + " | flaky: " + str(flaky) + " | " + str(duration) + "ms")
  }
  {
    total: total,
    passed: passed,
    failed: failed,
    skipped: skipped,
    todo: todoCount,
    unexpected: unexpected,
    flaky: flaky,
    duration: duration,
    ok: ok,
    stopped: stopped,
    failures: failures,
    results: results,
    afterAllError: afterAllError
  }
}

fn clear() {
  _assertCollecting("clear")
  _tests = []
  _groups = []
  _root = {
    name: "",
    path: "",
    parent: null,
    tags: [],
    beforeAll: [],
    afterAll: [],
    beforeEach: [],
    afterEach: [],
    started: false,
    blocked: false,
    setupError: null
  }
  _groups.push(_root)
  _group = _root
  _running = false
  _current = null
  true
}

val __module__ = {
  test: test,
  cases: cases,
  group: group,
  beforeAll: beforeAll,
  afterAll: afterAll,
  beforeEach: beforeEach,
  afterEach: afterEach,
  ok: ok,
  falsey: falsey,
  null: _isNull,
  defined: defined,
  equal: equal,
  notEqual: notEqual,
  same: same,
  contains: contains,
  type: type,
  length: length,
  empty: empty,
  approx: approx,
  inRange: _inRange,
  matchesPattern: _matchesPattern,
  raises: raises,
  notRaises: notRaises,
  fail: fail,
  skip: skip,
  todo: todo,
  note: note,
  plan: plan,
  snapshot: snapshot,
  list: list,
  run: run,
  clear: clear
}
`

func TestingModule(interp interface {
	ExecAsModule(source, filename string) (*runtime.Value, error)
}) *runtime.Value {
	mod, err := interp.ExecAsModule(testingSource, "testing")
	if err != nil {
		panic(err)
	}
	return mod
}
