#!/usr/bin/env bash
# build-and-measure.sh — deterministic reproducer for the CP contract WASM
# size question: which contracts currently exceed the casper-js-sdk 5.0.12
# ~65 KB install/upgrade cap, and by how much?
#
# Usage:
#   scripts/build-and-measure.sh                # build & measure all workspace members
#   scripts/build-and-measure.sh proof-registry # build & measure a single member
#
# Requirements (host or Docker):
#   * rustup + wasm32-unknown-unknown target
#   * cargo (>= 1.74)
#   * wasm-opt (binaryen) — OPTIONAL but recommended; if missing we skip the
#     post-build shrink and emit sizes-before only.
#
# Output:
#   Line per contract:  <name>  <size_bytes>  <verdict>
#   Where verdict is "OK <=65KB" or "OVER +<N> bytes over 65536".
#   Exit status is 0 always (this script is a measurement tool, not a gate).
#
# We do NOT push anywhere; this is a local, reproducible build+measure.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONTRACTS_DIR="$REPO_ROOT/contracts"
LIMIT_BYTES=65536

if ! command -v rustup >/dev/null 2>&1; then
  echo "error: rustup not on PATH; install rustup then rerun." >&2
  exit 1
fi
if ! rustup target list --installed | grep -q wasm32-unknown-unknown; then
  echo "info: adding wasm32-unknown-unknown target..." >&2
  rustup target add wasm32-unknown-unknown
fi

TARGETS=("proof-registry" "verifier-gate" "defi-mock" "stake-slashing" \
         "model-registry" "proof-aggregation" "proof-of-inference" \
         "stake-slashing-session" "governance" "zk-verifier")
if [[ $# -gt 0 ]]; then
  TARGETS=("$@")
fi

WASM_OUT_DIR="$CONTRACTS_DIR/target/wasm32-unknown-unknown/release"

echo "==> building ${#TARGETS[@]} contract(s) with --release for wasm32"
pushd "$CONTRACTS_DIR" >/dev/null
CARGO_PACKAGE_ARGS=()
for t in "${TARGETS[@]}"; do
  CARGO_PACKAGE_ARGS+=(-p "$t")
done
cargo build --release --target wasm32-unknown-unknown "${CARGO_PACKAGE_ARGS[@]}" 2>&1 | tail -20
popd >/dev/null

echo
echo "==> raw sizes:"
printf "%-32s %10s %s\n" "contract" "bytes" "verdict"
printf "%-32s %10s %s\n" "--------" "-----" "-------"

has_wasm_opt=0
if command -v wasm-opt >/dev/null 2>&1; then
  has_wasm_opt=1
fi

for t in "${TARGETS[@]}"; do
  wasm_path="$WASM_OUT_DIR/${t//-/_}.wasm"
  if [[ ! -f "$wasm_path" ]]; then
    # fallback: some workspace members are named with hyphens; try both.
    alt="$WASM_OUT_DIR/${t}.wasm"
    if [[ -f "$alt" ]]; then wasm_path="$alt"; fi
  fi
  if [[ ! -f "$wasm_path" ]]; then
    printf "%-32s %10s %s\n" "$t" "-" "MISSING"
    continue
  fi
  size=$(stat -c%s "$wasm_path")
  if (( size <= LIMIT_BYTES )); then
    verdict="OK <=65KB"
  else
    over=$(( size - LIMIT_BYTES ))
    verdict="OVER +${over} bytes over 65536"
  fi
  printf "%-32s %10d %s\n" "$t" "$size" "$verdict"
done

if (( has_wasm_opt == 1 )); then
  echo
  echo "==> shrinking with wasm-opt -Oz --strip-debug --strip-producers"
  for t in "${TARGETS[@]}"; do
    wasm_path="$WASM_OUT_DIR/${t//-/_}.wasm"
    [[ -f "$wasm_path" ]] || wasm_path="$WASM_OUT_DIR/${t}.wasm"
    [[ -f "$wasm_path" ]] || continue
    opt_path="${wasm_path%.wasm}.opt.wasm"
    wasm-opt -Oz --strip-debug --strip-producers "$wasm_path" -o "$opt_path" 2>/dev/null || {
      echo "  wasm-opt failed for $t"; continue;
    }
    before=$(stat -c%s "$wasm_path")
    after=$(stat -c%s "$opt_path")
    saved=$(( before - after ))
    if (( after <= LIMIT_BYTES )); then
      verdict="OK <=65KB (was ${before}B, saved ${saved}B)"
    else
      over=$(( after - LIMIT_BYTES ))
      verdict="OVER +${over} bytes (was ${before}B, saved ${saved}B)"
    fi
    printf "%-32s %10d %s\n" "$t.opt" "$after" "$verdict"
  done
else
  echo
  echo "note: wasm-opt not on PATH; skipped post-build shrink."
  echo "      install binaryen (brew install binaryen) for --strip-debug pass."
fi

echo
echo "==> done. Interpret results in docs/WASM_SIZE_ANALYSIS.md."
