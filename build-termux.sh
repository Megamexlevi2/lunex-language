#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="$SCRIPT_DIR"

GOOS="android"

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
    local arch="$1"
    local out="$OUT_DIR/$arch/lunex"
    mkdir -p "$(dirname "$out")"
    local goarch
    goarch="$(normalize_go_arch "$arch")"
    echo "Building lunex for android/$goarch..."
    GOOS="$GOOS" GOARCH="$goarch" CGO_ENABLED=0 \
        go build -buildmode=pie -trimpath -tags netgo,osusergo -ldflags "-s -w" -o "$out" ./cmd/lunex
    chmod +x "$out"
    if [ "$goarch" = "arm64" ] && command -v readelf >/dev/null 2>&1; then
        interp="$(readelf -lW "$out" 2>/dev/null | awk '/Requesting program interpreter:/{gsub(/[\[\]]/, "", $NF); print $NF; exit}')"
        case "$interp" in
            /system/bin/linker64|/apex/com.android.runtime/bin/linker64) ;;
            *) echo "Android arm64 binary has an invalid ELF interpreter: ${interp:-missing}" >&2; exit 1 ;;
        esac
    fi
    echo "  -> $out"
}

build_all() {
    local archs=("arm64" "arm" "amd64")
    for a in "${archs[@]}"; do
        build_go "$a"
    done
}

clean() {
    rm -rf "$OUT_DIR/arm64" "$OUT_DIR/arm" "$OUT_DIR/amd64"
}

case "${1:-build}" in
    build)
        mkdir -p "$OUT_DIR"
        build_all
        echo "Done. Binaries in $OUT_DIR"
        ;;
    native)

        mkdir -p "$OUT_DIR"
        build_go "$(detect_arch)"
        ;;
    clean)
        clean
        ;;
    *)
        echo "Usage: $0 [build|native|clean]"
        echo "  build   — build for arm64, arm, amd64"
        echo "  native  — build for current device only"
        echo "  clean   — remove release directory"
        ;;
esac
