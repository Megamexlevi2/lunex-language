#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"
BUILD_DIR="${BUILD_DIR:-$ROOT_DIR/dist/wasm}"
GO_BIN="${GO:-go}"
TARGET="${1:-all}"
GOROOT_DIR="$("$GO_BIN" env GOROOT)"

build_browser() {
  mkdir -p "$BUILD_DIR"
  GOOS=js GOARCH=wasm CGO_ENABLED=0 "$GO_BIN" build -trimpath -ldflags='-s -w' -o "$BUILD_DIR/lunex-js.wasm" ./cmd/lunexwasm
  if [[ -f "$GOROOT_DIR/lib/wasm/wasm_exec.js" ]]; then
    cp "$GOROOT_DIR/lib/wasm/wasm_exec.js" "$BUILD_DIR/wasm_exec.js"
  elif [[ -f "$GOROOT_DIR/misc/wasm/wasm_exec.js" ]]; then
    cp "$GOROOT_DIR/misc/wasm/wasm_exec.js" "$BUILD_DIR/wasm_exec.js"
  else
    printf '%s\n' 'wasm_exec.js was not found in the Go installation' >&2
    return 1
  fi
  cat > "$BUILD_DIR/lunex-browser.js" <<'EOF'
async function loadRuntime() {
  if (typeof globalThis.Go === "function") return
  if (typeof document === "undefined") throw new Error("wasm_exec.js must be loaded in a browser")
  await new Promise((resolve, reject) => {
    const script = document.createElement("script")
    script.src = new URL("./wasm_exec.js", import.meta.url)
    script.onload = resolve
    script.onerror = () => reject(new Error("failed to load wasm_exec.js"))
    document.head.appendChild(script)
  })
}

function fileKey(url) {
  const key = decodeURIComponent(url.pathname).replace(/^\/+/, "")
  return key || "main.lx"
}

function arrayBufferToBase64(buffer) {
  const bytes = new Uint8Array(buffer)
  let binary = ""
  for (let offset = 0; offset < bytes.length; offset += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(offset, Math.min(offset + 0x8000, bytes.length)))
  }
  return btoa(binary)
}

function findFileImports(source) {
  const imports = []
  let index = 0
  while (index < source.length) {
    if (source.startsWith("//", index)) {
      const end = source.indexOf("\n", index + 2)
      index = end === -1 ? source.length : end + 1
      continue
    }
    if (source.startsWith("/*", index)) {
      const end = source.indexOf("*/", index + 2)
      index = end === -1 ? source.length : end + 2
      continue
    }
    const current = source[index]
    if (current === "\"" || current === "'" || current === "`") {
      const quote = current
      index++
      while (index < source.length) {
        if (source[index] === "\\") {
          index += 2
          continue
        }
        if (source[index] === quote) {
          index++
          break
        }
        index++
      }
      continue
    }
    if (source.startsWith("@fimport", index) && !/[A-Za-z0-9_$]/.test(source[index + 8] || "")) {
      let cursor = index + 8
      while (/\s/.test(source[cursor] || "")) cursor++
      if (source[cursor] === "(") {
        cursor++
        while (/\s/.test(source[cursor] || "")) cursor++
        const quote = source[cursor]
        if (quote === "\"" || quote === "'") {
          cursor++
          let specifier = ""
          while (cursor < source.length && source[cursor] !== quote) {
            if (source[cursor] === "\\" && cursor + 1 < source.length) {
              specifier += source[cursor + 1]
              cursor += 2
            } else {
              specifier += source[cursor]
              cursor++
            }
          }
          if (source[cursor] === quote) {
            cursor++
            while (/\s/.test(source[cursor] || "")) cursor++
            if (source[cursor] === ")") {
              imports.push(specifier)
              index = cursor + 1
              continue
            }
          }
        }
      }
    }
    index++
  }
  return imports
}

async function collectProject(entryURL, suppliedSource) {
  const files = Object.create(null)
  const binaryFiles = Object.create(null)
  const visited = new Map()

  async function loadURL(inputURL, inlineSource) {
    const url = new URL(inputURL.href)
    url.hash = ""
    if (visited.has(url.href)) return visited.get(url.href)
    visited.set(url.href, false)

    let response
    let source
    if (inlineSource !== undefined) {
      source = inlineSource
    } else {
      response = await fetch(url)
      if (!response.ok) return false
      if (url.pathname.toLowerCase().endsWith(".nax")) {
        binaryFiles[fileKey(url)] = arrayBufferToBase64(await response.arrayBuffer())
        visited.set(url.href, true)
        return true
      }
      source = await response.text()
    }

    files[fileKey(url)] = source
    visited.set(url.href, true)
    for (const specifier of findFileImports(source)) {
      const target = new URL(specifier, url)
      const hasExtension = /\.[^/]+$/.test(target.pathname)
      const candidates = hasExtension
        ? [target]
        : [target, new URL(target.href + ".lx"), new URL(target.href + ".nax")]
      let found = false
      for (const candidate of candidates) {
        if (await loadURL(candidate)) {
          found = true
          break
        }
      }
      if (!found) throw new Error(`cannot fetch @fimport dependency: ${specifier} (imported by ${url.href})`)
    }
    return true
  }

  const entry = new URL(entryURL)
  if (!(await loadURL(entry, suppliedSource))) throw new Error(`cannot fetch Lunex source: ${entry.href}`)
  return { filename: fileKey(entry), files, binaryFiles, source: files[fileKey(entry)] }
}

export async function loadLunex(wasmURL = new URL("./lunex-js.wasm", import.meta.url)) {
  await loadRuntime()
  const response = await fetch(wasmURL)
  if (!response.ok) throw new Error(`failed to load Lunex WebAssembly: ${response.status}`)
  const bytes = await response.arrayBuffer()
  const go = new globalThis.Go()
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject)
  const running = go.run(instance)
  running.catch(error => {
    globalThis.LunexWasmError = error
  })
  await new Promise(resolve => setTimeout(resolve, 0))
  if (!globalThis.Lunex || typeof globalThis.Lunex.run !== "function") {
    throw new Error("Lunex WebAssembly failed to initialize")
  }

  const api = globalThis.Lunex
  const runSource = api.run.bind(api)
  api.runWithImports = async (source, baseURL = document.baseURI) => {
    if (typeof source !== "string") throw new TypeError("source must be a string")
    const project = await collectProject(new URL(baseURL, import.meta.url), source)
    return runSource(project.source, project)
  }
  api.runFile = async url => {
    const project = await collectProject(new URL(url, document.baseURI))
    return runSource(project.source, project)
  }
  return api
}
EOF
}

build_wasi() {
  mkdir -p "$BUILD_DIR"
  GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 "$GO_BIN" build -trimpath -ldflags='-s -w' -o "$BUILD_DIR/lunex-wasi.wasm" ./cmd/lunex
}

case "$TARGET" in
  all)
    build_browser
    build_wasi
    ;;
  browser|js)
    build_browser
    ;;
  wasi)
    build_wasi
    ;;
  *)
    printf '%s\n' 'usage: ./build-webasembly.sh [all|browser|wasi]' >&2
    exit 2
    ;;
esac
