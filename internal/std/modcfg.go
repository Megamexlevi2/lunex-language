package std

type Info struct {
	Name        string
	Description string
	Functions   map[string]string
}

var catalog = map[string]*Info{
	"io": {
		Name:        "io",
		Description: "Console I/O: log, warn, err, read, readLine, table, colors",
		Functions: map[string]string{
			"log":      "Print to stdout",
			"warn":     "Print to stderr in yellow",
			"err":      "Print to stderr in red",
			"info":     "Print in cyan",
			"success":  "Print in green with checkmark",
			"read":     "Read a line from stdin (prompt optional)",
			"readLine": "Alias for read",
			"table":    "Pretty-print an array of objects as a table",
		},
	},
	"fs": {
		Name:        "fs",
		Description: "Filesystem operations: read, write, list, stat, watch",
		Functions: map[string]string{
			"read":      "Read file as string",
			"readBytes": "Read file as byte array",
			"write":     "Write string to file",
			"append":    "Append string to file",
			"exists":    "Check if path exists",
			"stat":      "Get file/dir metadata",
			"rm":        "Delete file",
			"rmdir":     "Delete directory",
			"mkdir":     "Create directory",
			"ls":        "List directory contents",
			"copy":      "Copy file",
			"move":      "Move/rename file",
			"watch":     "Watch for file changes",
		},
	},
	"http": {
		Name:        "http",
		Description: "HTTP client and server primitives",
		Functions: map[string]string{
			"request":         "Generic request: request(method, url, options?)",
			"get":             "HTTP GET request",
			"post":            "HTTP POST request",
			"put":             "HTTP PUT request",
			"patch":           "HTTP PATCH request",
			"delete":          "HTTP DELETE request",
			"head":            "HTTP HEAD request",
			"createServer":    "Create an HTTP server: createServer(handler, options?)",
			"listen":          "Start a server: listen(server, port, host?, onReady?)",
			"close":           "Stop a server: close(server, timeoutMs?)",
			"json":            "Send a JSON response: json(res, value, status?)",
			"text":            "Send a text response: text(res, string, status?)",
			"html":            "Send an HTML response: html(res, string, status?)",
			"end":             "Send a raw string response: end(res, string?, status?)",
			"redirect":        "Send a redirect: redirect(res, url, status?)",
			"error":           "Create an HTTP error: error(status, message, code?)",
			"parseURL":        "Parse a URL into its parts",
			"buildURL":        "Append query parameters to a URL",
			"parseQuery":      "Parse a query string",
			"buildQuery":      "Build a query string from an object",
			"encode":          "Percent-encode a string",
			"decode":          "Percent-decode a string",
			"parseCookies":    "Parse a Cookie header",
			"serializeCookie": "Build a Set-Cookie header value",
			"statusText":      "Reason phrase for a status code",
			"status":          "Named status code constants",
		},
	},
	"http.router": {
		Name:        "http.router",
		Description: "Trie-based HTTP router built on std.http",
		Functions: map[string]string{
			"get":     "Define a GET route",
			"post":    "Define a POST route",
			"put":     "Define a PUT route",
			"patch":   "Define a PATCH route",
			"delete":  "Define a DELETE route",
			"head":    "Define a HEAD route",
			"options": "Define an OPTIONS route",
			"all":     "Define a route for every method",
			"create":  "Build a router from route definitions: create(routes, options?)",
		},
	},
	"http.static": {
		Name:        "http.static",
		Description: "Static file handler built on std.http",
		Functions: map[string]string{
			"create": "Create a handler that serves a directory: create(root, options?)",
		},
	},
	"crypto": {
		Name:        "crypto",
		Description: "Hashing, encoding, encryption, random number generation, and JWT",
		Functions: map[string]string{
			"hash":           "Hash a string: hash(text, algorithm?) — md5, sha1, sha256 (default), sha512",
			"hmac":           "HMAC signature: hmac(text, key, algorithm?)",
			"md5":            "MD5 hash shorthand",
			"sha1":           "SHA-1 hash shorthand",
			"sha256":         "SHA-256 hash shorthand",
			"sha512":         "SHA-512 hash shorthand",
			"randomBytes":    "Secure random bytes: randomBytes(length) → hex string",
			"randomHex":      "Secure random hex string",
			"randomUUID":     "Generate a UUID v4 string",
			"token":          "Generate a secure random token: token(length?)",
			"encrypt":        "AES-GCM encrypt: encrypt(text, key) → ciphertext",
			"decrypt":        "AES-GCM decrypt: decrypt(ciphertext, key) → plaintext",
			"toHex":          "Convert string to hex",
			"fromHex":        "Convert hex to string",
			"base64Encode":   "Base64 encode a string",
			"base64Decode":   "Base64 decode a string",
			"pbkdf2":         "PBKDF2 key derivation",
			"hashPassword":   "Bcrypt hash a password",
			"verifyPassword": "Verify bcrypt hash → bool",
			"compare":        "Constant-time string comparison",
		},
	},
	"db": {
		Name:        "db",
		Description: "Built-in document database backed by a real SQLite (.db) file on disk",
		Functions: map[string]string{
			"create":     "Open (or create) a named .db file, returns a database handle",
			"open":       "Alias for create",
			"connect":    "Alias for create",
			"drop":       "Delete a database's .db file from disk",
			"list":       "List the names of .db files in the project's data directory",
			"table":      "Get or create a table in the default database",
			"collection": "Alias for table",
		},
	},
	"ws": {
		Name:        "ws",
		Description: "WebSocket server and client",
	},
	"jwt": {
		Name:        "jwt",
		Description: "JSON Web Token sign and verify",
	},
	"math": {
		Name:        "math",
		Description: "Mathematical functions and constants",
		Functions: map[string]string{
			"abs":    "Absolute value",
			"ceil":   "Round up",
			"floor":  "Round down",
			"round":  "Round to nearest",
			"sqrt":   "Square root",
			"pow":    "Power",
			"log":    "Natural log",
			"log2":   "Log base 2",
			"log10":  "Log base 10",
			"sin":    "Sine",
			"cos":    "Cosine",
			"tan":    "Tangent",
			"min":    "Minimum of two values",
			"max":    "Maximum of two values",
			"clamp":  "Clamp value to range",
			"random": "Random float in [0,1)",
			"PI":     "Pi constant (3.14159...)",
			"E":      "Euler number (2.71828...)",
		},
	},
	"datetime": {
		Name:        "datetime",
		Description: "Date and time utilities",
		Functions: map[string]string{
			"now":    "Current timestamp (Unix ms)",
			"format": "Format a timestamp",
			"parse":  "Parse a date string",
			"add":    "Add duration to timestamp",
			"diff":   "Difference between two timestamps",
			"sleep":  "Sleep for N milliseconds",
		},
	},
	"os": {
		Name:        "os",
		Description: "OS interaction: exec, environment, platform info",
		Functions: map[string]string{
			"exec":     "Run a shell command",
			"spawn":    "Spawn a child process",
			"getenv":   "Get environment variable",
			"setenv":   "Set environment variable",
			"exit":     "Exit the process",
			"platform": "OS name string",
			"arch":     "CPU architecture string",
			"cwd":      "Current working directory",
			"chdir":    "Change working directory",
			"cpus":     "Number of CPU cores",
			"hostname": "Machine hostname",
		},
	},
	"regex": {
		Name:        "regex",
		Description: "Regular expression matching and replacement",
		Functions: map[string]string{
			"match":   "Test if string matches pattern",
			"find":    "Find first match",
			"findAll": "Find all matches",
			"replace": "Replace matches",
			"split":   "Split string by pattern",
		},
	},
	"env": {
		Name:        "env",
		Description: "Read and write environment variables",
	},
	"testing": {
		Name:        "testing",
		Description: "Lunex-native test registration, assertions, lifecycle hooks, diagnostics, snapshots, and execution",
		Functions: map[string]string{
			"test":           "Register a test function",
			"cases":          "Register parameterized tests from an array",
			"group":          "Register tests under a named group",
			"beforeAll":      "Register a group setup hook",
			"afterAll":       "Register a group cleanup hook",
			"beforeEach":     "Register a per-test setup hook",
			"afterEach":      "Register a per-test cleanup hook",
			"ok":             "Require a truthy value",
			"falsey":         "Require a falsy value",
			"null":           "Require a null value",
			"defined":        "Require a defined value",
			"equal":          "Require deep value equality",
			"notEqual":       "Require deep value inequality",
			"same":           "Require matching value types and deep equality",
			"contains":       "Require a string, array, or object to contain a value",
			"type":           "Require an exact Lunex value type",
			"length":         "Require an exact string or array length",
			"empty":          "Require an empty value",
			"approx":         "Require approximate numeric equality",
			"inRange":        "Require a numeric value inside an inclusive range",
			"matchesPattern": "Require a string to match a regular expression",
			"raises":         "Require a function to raise an expected error",
			"notRaises":      "Require a function not to raise an error",
			"fail":           "Fail the active test immediately",
			"skip":           "Skip the active test",
			"todo":           "Mark the active test as expected to fail",
			"note":           "Attach diagnostic text to the active test",
			"plan":           "Require an exact number of assertions in a test",
			"snapshot":       "Compare or update a deterministic value snapshot",
			"list":           "List registered tests without executing them",
			"run":            "Run the registered tests and return a structured summary",
			"clear":          "Clear the registered tests and group state",
		},
	},
	"utils": {
		Name:        "utils",
		Description: "String, array, and general utility helpers",
	},
}

func Get(name string) (*Info, bool) {
	m, ok := catalog[name]
	return m, ok
}

func All() map[string]*Info {
	result := make(map[string]*Info, len(catalog))
	for k, v := range catalog {
		result[k] = v
	}
	return result
}

func Describe(name string) string {
	m, ok := Get(name)
	if !ok {
		return ""
	}
	return m.Description
}
