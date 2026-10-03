package lunex

import (
	"fmt"
	"lunex/internal/pkg"
	"os"
	"path/filepath"
	"strings"
)

type templateFile struct {
	relPath string
	content string
}

type projectTemplate struct {
	id          string
	description string
	files       func(name string) []templateFile
}

var knownTemplates = map[string]*projectTemplate{
	"http_server": httpServerTemplate,
	"database":    databaseTemplate,
	"website":     websiteTemplate,
}

func templateNames() []string {
	return []string{"http_server", "database", "website"}
}

var httpServerTemplate = &projectTemplate{
	id:          "http_server",
	description: "A REST HTTP server with JSON routes",
	files: func(name string) []templateFile {
		main := `val http   = @import("std.http")
val router = @import("std.http.router")
val io     = @import("std.io")

val port = 3000

fn home(req, res) {
  res.json({ message: "` + name + ` is running", version: "1.0.0" })
}

fn health(req, res) {
  res.json({ status: "ok" })
}

fn echo(req, res) {
  res.json({ method: req.method, url: req.url, query: req.query })
}

val api = router.create([
  router.get("/", home),
  router.get("/health", health),
  router.get("/echo", echo)
])

fn main() {
  api.listen(port, "0.0.0.0", fn(actual) {
    io.log(io.green("` + name + ` listening on http://localhost:" + str(actual)))
    io.log("  GET /        -> server info")
    io.log("  GET /health  -> health check")
    io.log("  GET /echo    -> echo request info")
  })
}
`
		readme := `# ` + name + `

An HTTP server built with Lunex.

## Run

` + "```" + `
lunex run main.lx
` + "```" + `

## Routes

- ` + "`GET /`" + ` — server info
- ` + "`GET /health`" + ` — health check
- ` + "`GET /echo`" + ` — echoes method, url and query string back as JSON
`
		return []templateFile{
			{"main.lx", main},
			{"README.md", readme},
		}
	},
}

var databaseTemplate = &projectTemplate{
	id:          "database",
	description: "A project wired up to the built-in SQLite-backed document database",
	files: func(name string) []templateFile {
		main := `val io = @import("std.io")
val db = @import("std.db")

fn makeUser(name, email, age) {
  struct {
    name  = name
    email = email
    age   = age
  }
}

fn main() {
  val database = db.open("` + name + `")
  val users = database.table("users")

  users.schema({
    id:    { type: "string", default: "$uuid" }
    name:  { type: "string", required: true }
    email: { type: "string", required: true, unique: true }
    active:{ type: "boolean", default: true }
    age:   { type: "number", default: 0 }
  })

  io.log("Database file:", database.path)

  if users.count() == 0 {
    io.log("Seeding initial data...")
    users.insert(makeUser("Alice", "alice@example.com", 30))
    users.insert(makeUser("Bob", "bob@example.com", 25))
    users.insert(makeUser("Carol", "carol@example.com", 35))
  }

  io.log("Total users:", users.count())

  val adults = users.where("age", ">=", 18).orderBy("age").find()
  io.log("Users 18+:")
  each u in adults {
    io.log(" -", u.name, u.email, "age:", u.age)
  }

  database.close()
}
`
		readme := `# ` + name + `

A Lunex project using the built-in document database.

## Run

` + "```" + `
lunex run main.lx
` + "```" + `

The database is a real SQLite (.db) file, written to:

` + "```" + `
.lunex/data/` + name + `.db
` + "```" + `

You can open it with any standard SQLite tool (the ` + "`sqlite3`" + ` CLI,
DB Browser for SQLite, etc). Each table stores one row per document with
an ` + "`id`" + ` primary key and a ` + "`doc`" + ` column holding the JSON body, so plain
SQL and ` + "`json_extract()`" + ` also work directly against the file.

## API overview

- ` + "`db.open(name)`" + ` — open or create a database file
- ` + "`database.table(name)`" + ` — get or create a table
- ` + "`table.schema({...})`" + ` — declare field types, defaults, and constraints
- ` + "`table.insert(doc)`" + ` / ` + "`insertMany`" + ` / ` + "`upsert`" + `
- ` + "`table.find(filter)`" + ` / ` + "`findOne`" + ` / ` + "`findById`" + `
- ` + "`table.where(filter).orderBy(field).limit(n).find()`" + ` — query builder
- ` + "`table.update(filter, changes)`" + ` / ` + "`delete(filter)`" + `
- ` + "`table.index(fields)`" + ` — create a real SQL index
- ` + "`table.aggregate(pipeline)`" + ` — grouping and aggregation pipeline
`
		return []templateFile{
			{"main.lx", main},
			{"README.md", readme},
		}
	},
}

var websiteTemplate = &projectTemplate{
	id:          "website",
	description: "A small static website served by the built-in HTTP server",
	files: func(name string) []templateFile {
		main := `val http   = @import("std.http")
val router = @import("std.http.router")
val fs     = @import("std.fs")
val io     = @import("std.io")

val port = 3000

fn sendPage(res, path, status) {
  if fs.exists(path) {
    res.html(fs.readFile(path), status)
  } else {
    res.text("Not found", 404)
  }
}

fn home(req, res) {
  sendPage(res, "./public/index.html", 200)
}

fn about(req, res) {
  sendPage(res, "./public/about.html", 200)
}

fn missing(req, res) {
  sendPage(res, "./public/404.html", 404)
}

val api = router.create([
  router.get("/", home),
  router.get("/about", about)
], { notFound: missing })

fn main() {
  api.listen(port, "0.0.0.0", fn(actual) {
    io.log(io.green("` + name + ` listening on http://localhost:" + str(actual)))
    io.log("  GET /       -> home page")
    io.log("  GET /about  -> about page")
  })
}
`
		style := `      body {
        font-family: system-ui, sans-serif;
        max-width: 640px;
        margin: 4rem auto;
        padding: 0 1.5rem;
        color: #1a1a1a;
        line-height: 1.6;
      }
      h1 { margin-bottom: 0.5rem; }
      nav a { margin-right: 1rem; }
      button {
        padding: 0.5rem 1rem;
        font-size: 1rem;
        cursor: pointer;
      }`
		indexHTML := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>` + name + `</title>
  <style>
` + style + `
  </style>
</head>
<body>
  <nav><a href="/">Home</a><a href="/about">About</a></nav>
  <main>
    <h1>` + name + `</h1>
    <p>This site is served by a Lunex HTTP server.</p>
    <button id="ping">Ping the server</button>
    <p id="result"></p>
  </main>
  <script>
    document.getElementById("ping").addEventListener("click", () => {
      document.getElementById("result").textContent =
        "Page rendered at " + new Date().toLocaleTimeString();
    });
  </script>
</body>
</html>
`
		aboutHTML := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>About - ` + name + `</title>
  <style>
` + style + `
  </style>
</head>
<body>
  <nav><a href="/">Home</a><a href="/about">About</a></nav>
  <main>
    <h1>About</h1>
    <p>` + name + ` is a small website built with the Lunex language and its built-in HTTP server.</p>
  </main>
</body>
</html>
`
		notFoundHTML := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>404 - Not Found</title>
  <style>
` + style + `
  </style>
</head>
<body>
  <main>
    <h1>404</h1>
    <p>Page not found.</p>
    <p><a href="/">Go home</a></p>
  </main>
</body>
</html>
`
		readme := `# ` + name + `

A static website served by the built-in Lunex HTTP server.

## Run

` + "```" + `
lunex run main.lx
` + "```" + `

Then open http://localhost:3000

## Structure

- ` + "`main.lx`" + ` — HTTP server and routing
- ` + "`public/index.html`" + ` — home page
- ` + "`public/about.html`" + ` — about page
- ` + "`public/404.html`" + ` — not found page

CSS and JavaScript are inlined directly in each HTML page, since they are
served with ` + "`res.html()`" + `, which always returns ` + "`text/html`" + ` responses.
`
		return []templateFile{
			{"main.lx", main},
			{"public/index.html", indexHTML},
			{"public/about.html", aboutHTML},
			{"public/404.html", notFoundHTML},
			{"README.md", readme},
		}
	},
}

func writeTemplateFiles(projectDir string, files []templateFile) error {
	for _, f := range files {
		fullPath := filepath.Join(projectDir, f.relPath)
		if _, err := os.Stat(fullPath); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return fmt.Errorf("could not create directory for %s: %w", f.relPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(f.content), 0644); err != nil {
			return fmt.Errorf("could not write %s: %w", f.relPath, err)
		}
	}
	return nil
}

func runInitTemplate(tmpl *projectTemplate, name string) {
	if name == "" {
		fmt.Fprintln(os.Stderr, "error: project name required")
		fmt.Fprintf(os.Stderr, "       usage: lunex init %s <project_name>\n", tmpl.id)
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	projectDir := filepath.Join(cwd, name)
	if st, err := os.Stat(projectDir); err == nil && !st.IsDir() {
		fmt.Fprintf(os.Stderr, "error: %s exists and is not a directory\n", projectDir)
		os.Exit(1)
	}
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if err := pkg.InitManifest(projectDir, name); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	}

	files := tmpl.files(name)
	if err := writeTemplateFiles(projectDir, files); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	gitignorePath := filepath.Join(projectDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		gitignoreContent := "dist/\n.lunex/\n*.nax\n"
		_ = os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644)
	}

	fmt.Printf("\n  ✓ Created project: %s\n", projectDir)
	fmt.Printf("  Template: %s (%s)\n\n", tmpl.id, tmpl.description)
	fmt.Printf("  Files created:\n")
	fmt.Printf("    %-30s  project manifest\n", name+"/lunex.toml")
	for _, f := range files {
		fmt.Printf("    %-30s\n", name+"/"+f.relPath)
	}
	fmt.Printf("    %-30s  git ignore rules\n", name+"/.gitignore")
	fmt.Printf("\n  Run your project:\n")
	fmt.Printf("    cd %s\n", name)
	fmt.Printf("    lunex start\n\n")
}
