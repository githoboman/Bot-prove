#!/usr/bin/env bash
#
# contract-size-report.sh — Print .wasm size table for all contract crates
# and fail if any contract exceeds the hard ceiling.
#
# Usage:
#   scripts/contract-size-report.sh [--fail-over-kb N] [--warn-over-kb N] [--json]
#
# Defaults:
#   --fail-over-kb  200   # hard gas/deploy ceiling
#   --warn-over-kb   65   # historical installOrUpgrade limit (SDK 5.0.12);
#                          # keep as a warning, not a fail, because direct
#                          # deploys work fine above that.
#
# The script does NOT build. Run `cargo build --release --target
# wasm32-unknown-unknown --no-default-features` for each crate first (or use
# CI job that does).

set -euo pipefail

FAIL_KB=200
WARN_KB=65
JSON=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --fail-over-kb) FAIL_KB="$2"; shift 2 ;;
    --warn-over-kb) WARN_KB="$2"; shift 2 ;;
    --json) JSON=1; shift ;;
    -h|--help)
      grep '^#' "$0" | sed 's/^# \{0,1\}//'
      exit 0 ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WASM_DIR="$REPO_ROOT/contracts/target/wasm32-unknown-unknown/release"

if [[ ! -d "$WASM_DIR" ]]; then
  echo "wasm dir not found: $WASM_DIR" >&2
  echo "run cargo build --release --target wasm32-unknown-unknown --no-default-features per crate first" >&2
  exit 3
fi

# Deployed set (from docs/KNOWN_LIMITATIONS.md — everything NOT in the
# "undeployed" list). Update this list when a new contract goes live.
DEPLOYED=(proof-registry verifier-gate defi-mock stake-slashing stake-slashing-session)
UNDEPLOYED=(proof-of-inference model-registry proof-aggregation)

is_deployed() {
  local name="$1"
  for d in "${DEPLOYED[@]}"; do
    [[ "$d" == "$name" ]] && return 0
  done
  return 1
}

fail_kb_bytes=$((FAIL_KB * 1024))
warn_kb_bytes=$((WARN_KB * 1024))

any_fail=0
any_warn=0

rows=()

collect() {
  local name="$1"
  local wasm="$WASM_DIR/${name}.wasm"
  if [[ ! -f "$wasm" ]]; then
    echo "MISSING wasm for $name: $wasm" >&2
    any_fail=1
    rows+=("$name|MISSING|-|-")
    return
  fi
  local size
  size=$(stat -c '%s' "$wasm")
  local status="ok"
  if (( size > fail_kb_bytes )); then
    status="FAIL"
    any_fail=1
  elif (( size > warn_kb_bytes )); then
    status="warn"
    any_warn=1
  fi
  local deployed
  if is_deployed "$name"; then deployed="yes"; else deployed="no"; fi
  rows+=("$name|$size|$status|$deployed")
}

for c in "${DEPLOYED[@]}" "${UNDEPLOYED[@]}"; do
  collect "$c"
done

if (( JSON )); then
  printf '{\n  "fail_over_kb": %d,\n  "warn_over_kb": %d,\n  "contracts": [\n' "$FAIL_KB" "$WARN_KB"
  first=1
  for row in "${rows[@]}"; do
    IFS='|' read -r n s st d <<<"$row"
    (( first )) || printf ',\n'
    first=0
    printf '    {"name": "%s", "size_bytes": "%s", "status": "%s", "deployed": "%s"}' "$n" "$s" "$st" "$d"
  done
  printf '\n  ]\n}\n'
else
  printf '\n=== Contract .wasm size report ===\n'
  printf '  fail threshold: %d KB   warn threshold: %d KB\n\n' "$FAIL_KB" "$WARN_KB"
  printf '%-28s %10s  %6s  %-9s\n' 'CONTRACT' 'BYTES' 'STATUS' 'DEPLOYED'
  printf '%-28s %10s  %6s  %-9s\n' '--------' '-----' '------' '--------'
  for row in "${rows[@]}"; do
    IFS='|' read -r n s st d <<<"$row"
    if [[ "$s" != "MISSING" && "$s" != "-" ]]; then
      kb=$(awk "BEGIN{printf \"%.1f\", $s/1024}")
      printf '%-28s %10s  %6s  %-9s  (%s KB)\n' "$n" "$s" "$st" "$d" "$kb"
    else
      printf '%-28s %10s  %6s  %-9s\n' "$n" "$s" "$st" "$d"
    fi
  done
  echo
fi

if (( any_fail )); then
  echo "FAIL: at least one contract exceeds ${FAIL_KB}KB ceiling (or wasm missing)" >&2
  exit 1
fi
if (( any_warn )); then
  echo "WARN: at least one contract exceeds ${WARN_KB}KB (installOrUpgrade SDK 5.0.12 limit)" >&2
  # non-fatal
fi
exit 0
