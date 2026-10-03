#!/usr/bin/env bash

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'
BOLD='\033[1m'

step() { printf '%b==>%b %b%s%b\n' "$BLUE" "$NC" "$BOLD" "$*" "$NC"; }
ok()   { printf '%b[ok]%b %s\n' "$GREEN" "$NC" "$*"; }
warn() { printf '%b[!]%b %s\n' "$YELLOW" "$NC" "$*"; }
fail() { printf '%b[error]%b %s\n' "$RED" "$NC" "$*" >&2; exit 1; }
info() { printf '%b[info]%b %s\n' "$CYAN" "$NC" "$*"; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

NTL_VERSION="$(grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' "$SCRIPT_DIR/internal/app/version.json" | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')"

GO_MIN="1.25"

banner() {
    printf '\n  Lunex lang  v%s\n  Created by David Dev\n  GitHub: https://github.com/Megamexlevi2\n\n' "$NTL_VERSION"
}

android_runtime_detected() {
    if [ -n "${ANDROID_ROOT:-}" ] && [ -n "${ANDROID_DATA:-}" ]; then
        return 0
    fi
    case "${PREFIX:-}" in
        /data/data/com.termux/*|/data/user/*/com.termux/*|/data/user_de/*/com.termux/*)
            return 0
            ;;
    esac
    for path in \
        /system/bin/linker64 \
        /system/bin/linker \
        /apex/com.android.runtime/bin/linker64 \
        /apex/com.android.runtime/bin/linker; do
        if [ -x "$path" ]; then
            return 0
        fi
    done
    if [ -x /system/bin/getprop ] && [ -n "$(/system/bin/getprop ro.build.version.sdk 2>/dev/null)" ]; then
        return 0
    fi
    if command -v getprop >/dev/null 2>&1 && [ -n "$(getprop ro.build.version.sdk 2>/dev/null)" ]; then
        return 0
    fi
    if [ "$(uname -o 2>/dev/null || true)" = "Android" ]; then
        return 0
    fi
    if [ -f /proc/version ] && grep -qiE 'android|bionic' /proc/version 2>/dev/null; then
        return 0
    fi
    return 1
}

detect_os() {
    if android_runtime_detected; then
        echo "android"
        return
    fi
    case "$(uname -s)" in
        Linux*) echo "linux" ;;
        Darwin*)  echo "macos" ;;
        MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
        FreeBSD*) echo "freebsd" ;;
        *) echo "unknown" ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        riscv64)       echo "riscv64" ;;
        *)             echo "unknown" ;;
    esac
}

version_gte() {
    [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" = "$2" ]
}

normalize_arch() {
    case "$1" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        riscv64)       echo "riscv64" ;;
        *) echo "$1" ;;
    esac
}

normalize_host_go_os() {
    case "$1" in
        macos|darwin) echo "darwin" ;;
        android)      echo "linux" ;;
        *)            echo "$1" ;;
    esac
}

normalize_target_go_os() {
    case "$1" in
        macos|darwin) echo "darwin" ;;
        android)      echo "android" ;;
        *)            echo "$1" ;;
    esac
}

runtime_ext_for_os() {
    case "$1" in
        windows) echo ".exe" ;;
        *)       echo "" ;;
    esac
}

install_go() {
    if command -v go >/dev/null 2>&1 && version_gte "$(go version | awk '{print $3}' | sed 's/go//')" "$GO_MIN"; then
        ok "Go $(go version | awk '{print $3}') is already installed"
        return
    fi
    step "Installing Go $GO_MIN+"
    local arch
    arch=$(detect_arch)
    local goos
    goos=$(normalize_host_go_os "$(detect_os)")
    local ver="1.25.14"
    local name="go${ver}.${goos}-${arch}"
    local url="https://go.dev/dl/${name}.tar.gz"
    local tmp install_root
    tmp="$(mktemp -d)"
    install_root="$HOME/.local/go"
    curl -fsSL "$url" -o "$tmp/go.tar.gz"
    tar -C "$tmp" -xzf "$tmp/go.tar.gz"
    rm -rf "$install_root"
    mkdir -p "$HOME/.local" "$HOME/.local/bin"
    mv "$tmp/go" "$install_root"
    ln -sf "$install_root/bin/go" "$HOME/.local/bin/go"
    export PATH="$HOME/.local/bin:$install_root/bin:$PATH"
    rm -rf "$tmp"
    ok "Go $ver installed"
}

build_go_binary() {
    local os arch target_bin goos goarch ext log_file skip_uptodate
    os="${1:-$(detect_os)}"
    arch="${2:-$(detect_arch)}"
    target_bin="${3:-}"
    skip_uptodate="${4:-}"

    goos=$(normalize_target_go_os "$os")
    goarch=$(normalize_arch "$arch")
    ext="$(runtime_ext_for_os "$os")"

    if [ -z "$target_bin" ]; then
        target_bin="$SCRIPT_DIR/lunex$ext"
    fi

    mkdir -p "$(dirname "$target_bin")"

    local target_meta="${target_bin}.target"
    local target_id="${os}/${goarch}"
    if [ -z "$skip_uptodate" ] && [ -f "$target_bin" ] && [ -f "$target_meta" ] && [ "$(cat "$target_meta" 2>/dev/null || true)" = "$target_id" ]; then
        if ! find "$SCRIPT_DIR" -type f \( -name "*.go" -o -name "go.mod" -o -name "go.sum" -o -name "version.json" \) ! -path "$SCRIPT_DIR/.git/*" -newer "$target_bin" 2>/dev/null | grep -q .; then
            ok "Go binary is up to date"
            return
        fi
    fi

    step "Building Lunex for $goos/$goarch"
    cd "$SCRIPT_DIR"
    log_file="$(mktemp)"

    rm -f "$target_bin"

    local build_tags="netgo,osusergo"
    local build_mode=()
    if [ "$os" = "android" ]; then
        build_mode+=("-buildmode=pie")
    fi

    if ! env GONOSUMDB='*' GOFLAGS='-mod=mod' GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
        go build "${build_mode[@]}" -trimpath -tags "$build_tags" -ldflags="-s -w" -o "$target_bin" ./cmd/lunex >"$log_file" 2>&1; then
        sed '/^go: downloading/d' "$log_file" >&2
        rm -f "$log_file"
        fail "Build failed"
    fi

    rm -f "$log_file"
    [ -f "$target_bin" ] || fail "Build completed without producing $target_bin"
    printf '%s\n' "$target_id" > "$target_meta"
    if [ "$os" = "android" ]; then
        if command -v readelf >/dev/null 2>&1; then
            local machine interpreter
            machine=$(readelf -h "$target_bin" 2>/dev/null | awk -F: '/Class:/{c=$2} /Machine:/{m=$2} END{gsub(/^[[:space:]]+|[[:space:]]+$/, "", c); gsub(/^[[:space:]]+|[[:space:]]+$/, "", m); print c "|" m}')
            if [ "$goarch" = "arm64" ] && [ "$machine" != "ELF64|AArch64" ]; then
                fail "Android arm64 build has an unexpected ELF target: $machine"
            fi
            if [ "$goarch" = "amd64" ] && [ "$machine" != "ELF64|Advanced Micro Devices X86-64" ]; then
                fail "Android amd64 build has an unexpected ELF target: $machine"
            fi
        fi
    fi
    if [ "$os" = "android" ] && command -v readelf >/dev/null 2>&1; then
        local interp
        interp="$(readelf -lW "$target_bin" 2>/dev/null | awk '/Requesting program interpreter:/{gsub(/[\[\]]/, "", $NF); print $NF; exit}')"
        case "$goarch:$interp" in
            arm64:/system/bin/linker64|arm64:/apex/com.android.runtime/bin/linker64|amd64:/system/bin/linker|amd64:/apex/com.android.runtime/bin/linker) ;;
            *) fail "Android binary has an invalid ELF interpreter: ${interp:-missing}" ;;
        esac
    fi
    ok "$(basename "$target_bin") built: $(du -sh "$target_bin" | cut -f1)"
}

build_release() {
    step "Starting multi-platform release build"
    local out_dir="$SCRIPT_DIR/release"
    mkdir -p "$out_dir"

    local targets=(
        "linux:amd64"
        "linux:arm64"
        "windows:amd64"
        "windows:arm64"
        "darwin:amd64"
        "darwin:arm64"
        "android:arm64"
        "freebsd:amd64"
    )

    local t goos goarch ext release_name
    for t in "${targets[@]}"; do
        IFS=':' read -r goos goarch <<< "$t"
        info "Building $goos/$goarch"
        ext="$(runtime_ext_for_os "$goos")"
        release_name="lunex-${goos}-${goarch}${ext}"
        build_go_binary "$goos" "$goarch" "$out_dir/$release_name" "release"
        ok "Done: $out_dir/$release_name ($(du -sh "$out_dir/$release_name" | cut -f1))"
    done

    ok "All targets built successfully in ./release/"
}

run_tests() {
    local bin="$1"
    local runner="$SCRIPT_DIR/tests/run_all.sh"

    if [ ! -f "$runner" ]; then
        fail "Test runner not found: $runner"
    fi

    step "Running Lunex integration tests"
    if ! bash "$runner" "$bin"; then
        fail "Tests failed"
    fi
    ok "All tests passed"
}

clean() {
    step "Cleaning build artifacts"
    cd "$SCRIPT_DIR"
    rm -rf lunex lunex.exe lunex.target lunex.exe.target release
    find release -type f -name '*.target' -delete 2>/dev/null || true
    ok "Cleaned"
}

cmd="${1:-build}"
shift || true

case "$cmd" in
    build|"")
        banner
        target_os="${1:-$(detect_os)}"
        target_arch="${2:-$(detect_arch)}"
        install_go
        build_go_binary "$target_os" "$target_arch" \
            "$SCRIPT_DIR/lunex$(runtime_ext_for_os "$target_os")"
        echo
        ok "Lunex v${NTL_VERSION} built successfully"
        info "Run:  ./lunex run <file.lx>"
        ;;
    release|--release)
        banner
        install_go
        build_release
        ;;
    test)
        install_go
        target_bin="$SCRIPT_DIR/lunex$(runtime_ext_for_os "$(detect_os)")"
        build_go_binary "$(detect_os)" "$(detect_arch)" "$target_bin"
        run_tests "$target_bin"
        ;;
    clean)
        clean
        ;;
    *)
        echo "Usage: $0 [build|release|--release|test|clean]"
        echo "  build    — build for current platform (default)"
        echo "  release  — build for all platforms"
        echo "  test     — build + run tests"
        echo "  clean    — remove build artifacts"
        exit 1
        ;;
esac
