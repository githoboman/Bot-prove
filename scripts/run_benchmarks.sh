#!/usr/bin/env bash
# CI-runnable benchmark suite for CasperProver's hot paths (Merkle tree
# build/root/path, raw hashing, proof generate/verify).
#
# Usage:
#   scripts/run_benchmarks.sh                # run + print, don't persist
#   scripts/run_benchmarks.sh --baseline      # run + write baseline JSON
#
# Baseline metrics are written to benchmarks/baseline.json (ns/op + B/op +
# allocs/op per benchmark case), so regressions can be diffed by eye or by a
# future CI gate. This script never fails the build on drift by itself —
# it just produces the artifact.
#
# Benchmark hygiene note (R11 review): BenchmarkGenerateProof in
# engine/internal/verifier/verify_bench_test.go refreshes its ProofEngine
# every proofRefreshInterval iterations (with StopTimer/StartTimer) so
# the in-memory proof map never crosses prover.MaxProofs mid-run — that
# threshold otherwise turns evictIfNeeded's O(1) fast path into an O(n)
# map scan on every subsequent Generate, contaminating the reported
# ns/op with bookkeeping cost rather than measuring steady-state
# Generate. Reproduce the difference by removing that guard:
#   -benchtime=20000x  (below threshold) -> ~5µs/op
#   -benchtime=150000x (crosses threshold) -> ~540µs/op (108× slower)
# Both numbers are for BenchmarkGenerateProof/payload=64B on the same
# machine — the second is the artifact, not the reality.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${REPO_ROOT}/benchmarks"
RAW_OUT="${OUT_DIR}/last_run.txt"
BASELINE_OUT="${OUT_DIR}/baseline.json"

mkdir -p "${OUT_DIR}"

cd "${REPO_ROOT}/engine"
echo "Running Go benchmarks (prover + verifier)..."
go test -run '^$' -bench '.' -benchmem \
  ./internal/prover/... ./internal/verifier/... | tee "${RAW_OUT}"

if [[ "${1:-}" == "--baseline" ]]; then
  echo "Writing baseline metrics to ${BASELINE_OUT}"
  python3 "${REPO_ROOT}/scripts/bench_to_json.py" "${RAW_OUT}" "${BASELINE_OUT}"
fi

echo "Done. Raw output: ${RAW_OUT}"
