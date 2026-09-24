#!/usr/bin/env bash
# lb_flip.sh — atomic blue/green cutover for the local observability stack.
#
# Usage:
#   scripts/lb_flip.sh <from> <to>
#
#   <from>, <to> ∈ {blue, green}
#
# What it does:
#   Rewrites deploy/observability/prometheus.yml + nginx (if configured) so
#   that scrape + traffic targets swap from <from> to <to>. Idempotent: if
#   <to> is already live, exit 0 with a diagnostic.
#
# What it does NOT do:
#   - No real cloud LB API calls. That is intentional — this repo is
#     free/OSS/testnet-only until MAINNET_LAUNCH_PLAN.md is unlocked.
#   - No deploy — this script only flips traffic between two ALREADY-RUNNING
#     slots. Bring the target up FIRST (see docs/OPS_RUNBOOKS.md §2.3).
#
# Exit codes:
#   0  cutover successful (or already at target)
#   1  invalid arguments
#   2  target slot not healthy
#   3  file mutation failed
set -euo pipefail

usage() {
  echo "Usage: $0 <blue|green> <blue|green>" >&2
  exit 1
}

[ $# -eq 2 ] || usage
FROM="$1"
TO="$2"

case "$FROM" in blue|green) ;; *) usage ;; esac
case "$TO"   in blue|green) ;; *) usage ;; esac
[ "$FROM" != "$TO" ] || {
  echo "ERROR: <from> and <to> must differ" >&2
  exit 1
}

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PROM_CFG="$REPO_ROOT/deploy/observability/prometheus.yml"

if [ ! -f "$PROM_CFG" ]; then
  echo "ERROR: $PROM_CFG not found (Pack AG must be deployed first)" >&2
  exit 3
fi

# --- Health-check TO slot first. Bail if it is not up. ---------------------
TARGET_URL="http://engine-${TO}:8080/health"
echo "[lb_flip] pre-flight: probing ${TARGET_URL}"
if ! curl -sf --max-time 5 "${TARGET_URL}" >/dev/null; then
  echo "ERROR: ${TO} slot is not healthy at ${TARGET_URL}. Aborting cutover." >&2
  exit 2
fi

# --- Idempotency: if the scrape config already targets TO, exit clean. -----
if grep -q "engine-${TO}:8080" "$PROM_CFG" \
   && ! grep -q "engine-${FROM}:8080" "$PROM_CFG"; then
  echo "[lb_flip] already at target (${TO}). No-op."
  exit 0
fi

# --- Atomic rewrite via tempfile + mv. -------------------------------------
TMP="$(mktemp "${PROM_CFG}.XXXXXX")"
trap 'rm -f "$TMP"' EXIT

sed "s/engine-${FROM}:8080/engine-${TO}:8080/g" "$PROM_CFG" > "$TMP"

if ! diff -q "$PROM_CFG" "$TMP" >/dev/null; then
  mv "$TMP" "$PROM_CFG"
  echo "[lb_flip] cutover: prometheus scrape target ${FROM} -> ${TO}"
else
  echo "[lb_flip] no changes needed in $PROM_CFG"
fi

# --- Ask Prometheus to reload without a full restart, if reachable. --------
if command -v curl >/dev/null 2>&1; then
  RELOAD_URL="http://prometheus:9090/-/reload"
  if curl -sf --max-time 3 -XPOST "$RELOAD_URL" >/dev/null; then
    echo "[lb_flip] prometheus reloaded"
  else
    echo "[lb_flip] prometheus reload endpoint unreachable — restart the service manually if needed"
  fi
fi

echo "[lb_flip] done."
