#!/usr/bin/env bash

set -uo pipefail

RED=$'\033[31m'
GREEN=$'\033[32m'
YELLOW=$'\033[33m'
CYAN=$'\033[36m'
GRAY=$'\033[90m'
BOLD=$'\033[1m'
RESET=$'\033[0m'

usage() {
  printf 'Usage: bash tests/run_all.sh ./path/to/lunex\n'
  printf 'Error: provide an explicit path starting with ./ or /\n'
}

print_error() {
  printf '  %bError:%b %s\n' "$RED" "$RESET" "$*" >&2
}

print_info() {
  printf '  %bInfo:%b %s\n' "$CYAN" "$RESET" "$*"
}

if [ "$#" -lt 1 ]; then
  usage
  exit 1
fi

LUNEX="$1"

case "$LUNEX" in
  ./*|/*) ;;
  *)
    print_error "invalid Lunex binary path '$LUNEX'"
    printf '  Use an explicit path such as ./lunex or /usr/local/bin/lunex\n'
    exit 1
    ;;
esac

if [ ! -f "$LUNEX" ]; then
  print_error "Lunex binary not found: $LUNEX"
  exit 1
fi

if [ ! -x "$LUNEX" ]; then
  print_info "adding execute permission to $LUNEX"
  if ! chmod +x "$LUNEX"; then
    print_error "unable to make Lunex executable: $LUNEX"
    exit 1
  fi
fi

if [ ! -x "$LUNEX" ]; then
  print_error "Lunex binary is not executable: $LUNEX"
  exit 1
fi

BASE="$(cd "$(dirname "$0")" && pwd)"
PASS=0
FAIL=0
TOTAL=0
FAILED_TESTS=()
LOG_DIR="$(mktemp -d "${TMPDIR:-/tmp}/lunex-tests.XXXXXX")"

cleanup() {
  rm -rf "$LOG_DIR"
}

trap cleanup EXIT INT TERM

run_test() {
  local file="$1"
  local name
  local log_file
  local exitcode

  name="$(basename "$file" .lx)"
  log_file="$LOG_DIR/$(printf '%06d' "$TOTAL").log"
  TOTAL=$((TOTAL + 1))

  if [[ "$file" == "$BASE/stdlib/ffi/01_ffi_disabled.lx" ]]; then
    env -u LUNEX_FFI "$LUNEX" run "$file" >"$log_file" 2>&1
    exitcode=$?
  elif [[ "$file" == "$BASE/stdlib/ffi/08_ffi_environment_is_ignored.lx" ]]; then
    LUNEX_FFI=on "$LUNEX" run "$file" >"$log_file" 2>&1
    exitcode=$?
  elif [[ "$file" == "$BASE/stdlib/ffi/"*.lx ]]; then
    "$LUNEX" ffi = on run "$file" >"$log_file" 2>&1
    exitcode=$?
  else
    "$LUNEX" run "$file" >"$log_file" 2>&1
    exitcode=$?
  fi

  if [ "$exitcode" -eq 0 ] && grep -Fq 'PASS' "$log_file"; then
    printf '  %b✓%b  %s\n' "$GREEN" "$RESET" "$name"
    PASS=$((PASS + 1))
    return 0
  fi

  printf '  %b✗%b  %s\n' "$RED" "$RESET" "$name"
  FAIL=$((FAIL + 1))
  FAILED_TESTS+=("$file")

  printf '        %bExit code:%b %s\n' "$GRAY" "$RESET" "$exitcode"
  printf '        %bFull Lunex output:%b\n' "$GRAY" "$RESET"

  if [ -s "$log_file" ]; then
    sed 's/^/        | /' "$log_file"
  else
    printf '        | %b<no output>%b\n' "$GRAY" "$RESET"
  fi

  printf '\n'
}

printf '\n'
printf '%bLunex Test Suite%b\n' "$BOLD" "$RESET"
printf '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n'

categories=(
  variables
  functions
  control_flow
  loops
  structs
  stdlib/io
  stdlib/math
  stdlib/utils
  stdlib/json
  stdlib/datetime
  stdlib/crypto
  stdlib/fs
  stdlib/os
  stdlib/regex
  stdlib/testing
  stdlib/ffi
  stdlib/db
  stdlib/http
  stdlib/ints
  stdlib/buffer
  concurrency
  advanced
)

for category in "${categories[@]}"; do
  dir="$BASE/$category"

  if [ ! -d "$dir" ]; then
    continue
  fi

  printf '\n%b%s%b\n' "$BOLD" "  $category" "$RESET"

  while IFS= read -r -d '' file; do
    run_test "$file"
  done < <(find "$dir" -maxdepth 1 -type f -name '*.lx' -print0 | sort -z)
done

printf '\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n'
printf '  passed : %b%d%b / %d\n' "$GREEN" "$PASS" "$RESET" "$TOTAL"
printf '  failed : %b%d%b / %d\n' "$RED" "$FAIL" "$RESET" "$TOTAL"
printf '\n'

if [ "$FAIL" -gt 0 ]; then
  printf '  %bFailed tests:%b\n' "$RED" "$RESET"
  for file in "${FAILED_TESTS[@]}"; do
    printf '    %s\n' "$file"
  done
  printf '\n'
  exit 1
fi

printf '  %b✓ All %d tests passed.%b\n' "$GREEN" "$PASS" "$RESET"
printf '\n'
