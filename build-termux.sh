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

GOOS="android"

banner() {
    printf '\n  Lunex lang  v%s\n  Created by David Dev\n  GitHub: https://github.com/Megamexlevi2\n\n' "$NTL_VERSION"
}

detect_arch() {
    case "$(uname -m)" in
        aarch64|arm64) echo "arm64" ;;
        armv7l|armv8l) echo "arm" ;;
        x86_64)        echo "amd64" ;;
        *)             echo "arm64" ;;
    esac
}

normalize_go_arch() {
    case "$1" in
        arm64) echo "arm64" ;;
        arm)   echo "arm" ;;
        amd64) echo "amd64" ;;
        *)     echo "arm64" ;;
    esac
}

build_go() {
    local goarch="$1"
    local out="$SCRIPT_DIR/lunex"

    step "Building Lunex for android/$goarch"

    env CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$goarch" \
        go build -buildmode=pie -trimpath -tags netgo,osusergo -ldflags "-s -w" -o "$out" ./cmd/lunex

    chmod +x "$out"

    if [ "$goarch" = "arm64" ] && command -v readelf >/dev/null 2>&1; then
        local interp
        interp="$(readelf -lW "$out" 2>/dev/null | awk '/Requesting program interpreter:/{gsub(/[\[\]]/, "", $NF); print $NF; exit}')"
        case "$interp" in
            /system/bin/linker64|/apex/com.android.runtime/bin/linker64) ;;
            *) fail "Android arm64 binary has an invalid ELF interpreter: ${interp:-missing}" ;;
        esac
    fi

    ok "$(basename "$out") built: $(du -sh "$out" | cut -f1)"
}

build_all() {
    local archs=("arm64" "arm" "amd64")
    local a
    for a in "${archs[@]}"; do
        build_go "$(normalize_go_arch "$a")"
    done
}

clean() {
    step "Cleaning build artifacts"
    rm -f "$SCRIPT_DIR/lunex"
    ok "Cleaned"
}

cmd="${1:-build}"

case "$cmd" in
    build)
        banner
        build_go "$(normalize_go_arch "$(detect_arch)")"
        echo
        ok "Lunex v${NTL_VERSION} built successfully"
        info "Run:  ./lunex run <file.lx>"
        ;;
    clean)
        clean
        ;;
    *)
        echo "Usage: $0 [build|clean]"
        echo "  build  — build for current device"
        echo "  clean  — remove build artifact"
        exit 1
        ;;
esac